package simple

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

// fakeElastic records documents written by the writer and can be
// told to fail result set writes.
type fakeElastic struct {
	rows     []*SimpleResultSetRecord
	md       []ResultSetMetadataRecord
	rs_calls int
	fail_rs  bool
}

func (self *fakeElastic) setElasticIndex(ctx context.Context,
	org_id, index, id string, record interface{}) error {
	switch t := record.(type) {
	case *ResultSetMetadataRecord:
		self.md = append(self.md, copyMetadata(*t))
	case *SimpleResultSetRecord:
		self.rs_calls++
		if self.rs_calls > 100 {
			panic("unbounded result set writes")
		}
		if self.fail_rs {
			return errors.New("injected elastic error")
		}
		self.rows = append(self.rows, t)
	}
	return nil
}

func (self *fakeElastic) setElasticIndexAsync(org_id, index, id string,
	action cvelo_services.BulkUpdateType, record interface{}) error {
	return self.setElasticIndex(context.Background(), org_id, index, id, record)
}

func (self *fakeElastic) lastMD() ResultSetMetadataRecord {
	if len(self.md) == 0 {
		return ResultSetMetadataRecord{}
	}
	return self.md[len(self.md)-1]
}

// Like GetResultSetMetadata, returns the record with the newest
// timestamp or an empty legacy record if there is none. Elastic does
// not order records with the same timestamp, so on a tie this returns
// the older record to make ties show up in tests.
func (self *fakeElastic) getResultSetMetadata(ctx context.Context,
	config_obj *config_proto.Config,
	log_path api.FSPathSpec) (*ResultSetMetadataRecord, error) {
	if len(self.md) == 0 {
		return &ResultSetMetadataRecord{Type: "rs_metadata"}, nil
	}
	newest := self.md[0]
	for _, md := range self.md[1:] {
		if md.Timestamp > newest.Timestamp {
			newest = md
		}
	}
	newest = copyMetadata(newest)
	return &newest, nil
}

// Elastic stores and returns serialized copies, so changes to a
// writer's record must not change a stored one, or the reverse.
func copyMetadata(md ResultSetMetadataRecord) ResultSetMetadataRecord {
	if md.EndByte != nil {
		end_byte := *md.EndByte
		md.EndByte = &end_byte
	}
	return md
}

func installFakeElastic(t *testing.T) *fakeElastic {
	fake := &fakeElastic{}

	old_set, old_async, old_flush, old_get := setElasticIndex,
		setElasticIndexAsync, flushIndex, getResultSetMetadata
	setElasticIndex = fake.setElasticIndex
	setElasticIndexAsync = fake.setElasticIndexAsync
	flushIndex = func(ctx context.Context, org_id, index string) error {
		return nil
	}
	getResultSetMetadata = fake.getResultSetMetadata
	t.Cleanup(func() {
		setElasticIndex, setElasticIndexAsync, flushIndex,
			getResultSetMetadata = old_set, old_async, old_flush, old_get
	})

	return fake
}

func newTestWriter(sync bool) *ElasticSimpleResultSetWriter {
	return &ElasticSimpleResultSetWriter{
		log_path:   path_specs.NewSafeFilestorePath("clients", "C.1", "F.1"),
		opts:       json.DefaultEncOpts(),
		ctx:        context.Background(),
		config_obj: &config_proto.Config{},
		sync:       sync,
		md: &ResultSetMetadataRecord{
			ID:   "v1",
			Type: "rs_metadata",
		},
		version:             "v1",
		rows_per_result_set: 1000,
		max_size_per_packet: 1024 * 1024,
	}
}

func TestAbortEmptyBufferPersistsSentinel(t *testing.T) {
	fake := installFakeElastic(t)

	writer := newTestWriter(false)
	writer.Abort()

	assert.Equal(t, 0, len(fake.rows))
	assert.Equal(t, 1, len(fake.md))
	assert.Equal(t, int64(-1), fake.lastMD().TotalRows)
}

func TestAbortDiscardsBufferedRows(t *testing.T) {
	fake := installFakeElastic(t)

	writer := newTestWriter(false)
	writer.Write(ordereddict.NewDict().Set("A", 1))
	writer.Write(ordereddict.NewDict().Set("A", 2))
	writer.Abort()

	// Nothing written after the abort either.
	writer.Write(ordereddict.NewDict().Set("A", 3))
	writer.Close()

	assert.Equal(t, 0, len(fake.rows))
	assert.Equal(t, 1, len(fake.md))
	assert.Equal(t, int64(-1), fake.lastMD().TotalRows)
}

func TestPersistentWriteErrorTerminates(t *testing.T) {
	fake := installFakeElastic(t)
	fake.fail_rs = true

	// Mirrors client log ingestion: a sync writer with a small batch
	// that is only written by the deferred Close.
	writer := newTestWriter(true)
	writer.WriteJSONL([]byte("{\"A\":1}\n{\"A\":2}\n"), 2)
	writer.Close()
	writer.Close()

	assert.Equal(t, 1, fake.rs_calls)
	assert.Equal(t, 0, len(fake.rows))
	assert.Equal(t, 1, len(fake.md))
	assert.Equal(t, int64(-1), fake.lastMD().TotalRows)
	assert.Equal(t, int64(-1), fake.lastMD().EndRow)
}

func TestFlushAdvancesRows(t *testing.T) {
	fake := installFakeElastic(t)

	writer := newTestWriter(true)
	writer.WriteJSONL([]byte("{\"A\":1}\n{\"A\":2}\n"), 2)
	writer.Close()

	assert.Equal(t, 1, len(fake.rows))
	assert.Equal(t, int64(0), fake.rows[0].StartRow)
	assert.Equal(t, int64(2), fake.rows[0].EndRow)
	assert.Equal(t, int64(2), fake.lastMD().EndRow)
	assert.Equal(t, int64(0), fake.lastMD().TotalRows)
}

var testLogPath = path_specs.NewSafeFilestorePath("clients", "C.1", "F.1")

// Opens a writer the way NewResultSetWriter does, starting a new
// version called new_id if the existing one can not be continued.
func openTestWriter(t *testing.T, sync bool,
	mode result_sets.WriteMode, new_id string) *ElasticSimpleResultSetWriter {
	md, err := openWriterMetadata(context.Background(),
		&config_proto.Config{}, testLogPath, mode,
		newWriterMetadata(testLogPath, new_id))
	assert.NoError(t, err)

	writer := newTestWriter(sync)
	writer.md = md
	writer.version = md.ID
	writer.start_row = md.EndRow
	return writer
}

func TestAppendContinuesExistingResultSet(t *testing.T) {
	fake := installFakeElastic(t)
	fake.md = []ResultSetMetadataRecord{{ID: "v1", EndRow: 2}}

	writer := openTestWriter(t, true, result_sets.AppendMode, "v2")
	assert.Equal(t, "v1", writer.version)
	assert.Equal(t, int64(2), writer.start_row)

	// The existing record is reused, not rewritten.
	assert.Equal(t, 1, len(fake.md))
}

func TestAppendAfterAbortStartsNewVersion(t *testing.T) {
	fake := installFakeElastic(t)
	fake.md = []ResultSetMetadataRecord{{ID: "v1", EndRow: -1, TotalRows: -1}}

	writer := openTestWriter(t, true, result_sets.AppendMode, "v2")
	assert.Equal(t, "v2", writer.version)
	assert.Equal(t, int64(0), writer.start_row)

	assert.Equal(t, 2, len(fake.md))
	assert.Equal(t, "v2", fake.lastMD().ID)
	assert.Equal(t, int64(0), fake.lastMD().TotalRows)
}

func TestClientLogRecoversAfterWriteError(t *testing.T) {
	fake := installFakeElastic(t)

	// A log batch fails to write so the result set is aborted.
	fake.fail_rs = true
	writer := openTestWriter(t, true, result_sets.AppendMode, "v1")
	writer.WriteJSONL([]byte("{\"A\":1}\n"), 1)
	writer.Close()
	assert.Equal(t, int64(-1), fake.lastMD().TotalRows)

	// The next batch for the same flow is written successfully and
	// the result set can be read again (readers refuse TotalRows < 0).
	fake.fail_rs = false
	writer = openTestWriter(t, true, result_sets.AppendMode, "v2")
	writer.WriteJSONL([]byte("{\"A\":2}\n{\"A\":3}\n"), 2)
	writer.Close()

	assert.Equal(t, 1, len(fake.rows))
	assert.Equal(t, "v2", fake.rows[0].ID)
	assert.Equal(t, int64(0), fake.rows[0].StartRow)
	assert.Equal(t, int64(2), fake.rows[0].EndRow)

	md := fake.lastMD()
	assert.Equal(t, "v2", md.ID)
	assert.Equal(t, int64(2), md.EndRow)
	assert.Equal(t, int64(0), md.TotalRows)

	// Later batches keep appending to the recovered version.
	writer = openTestWriter(t, true, result_sets.AppendMode, "v3")
	writer.WriteJSONL([]byte("{\"A\":4}\n"), 1)
	writer.Close()

	assert.Equal(t, "v2", fake.rows[1].ID)
	assert.Equal(t, int64(2), fake.rows[1].StartRow)
	assert.Equal(t, int64(3), fake.lastMD().EndRow)
}

// Freezes the clock for the rest of the test.
func freezeClock(t *testing.T) int64 {
	now := time.Unix(1661385600, 0)
	t.Cleanup(utils.MockTime(utils.NewMockClock(now)))
	return now.UnixNano()
}

func TestMetadataTimestampsIncreaseWithFrozenClock(t *testing.T) {
	fake := installFakeElastic(t)
	freezeClock(t)

	// Each batch is appended by a new writer, as in log ingestion.
	for i := 0; i < 3; i++ {
		writer := openTestWriter(t, true, result_sets.AppendMode, "v1")
		writer.WriteJSONL([]byte("{\"A\":1}\n"), 1)
		writer.Close()
	}

	for i := 1; i < len(fake.md); i++ {
		assert.Greater(t, fake.md[i].Timestamp, fake.md[i-1].Timestamp)
	}

	md, err := fake.getResultSetMetadata(context.Background(), nil, testLogPath)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), md.EndRow)
}

func TestAppendFromSlowClockReplacesRecord(t *testing.T) {
	fake := installFakeElastic(t)
	now := freezeClock(t)

	// The last record came from a frontend whose clock is an hour
	// ahead of ours.
	ahead := now + int64(time.Hour)
	fake.md = []ResultSetMetadataRecord{{ID: "v1", EndRow: 2, Timestamp: ahead}}

	writer := openTestWriter(t, true, result_sets.AppendMode, "v2")
	writer.WriteJSONL([]byte("{\"A\":3}\n"), 1)
	writer.Close()

	assert.Greater(t, fake.lastMD().Timestamp, ahead)

	md, err := fake.getResultSetMetadata(context.Background(), nil, testLogPath)
	assert.NoError(t, err)
	assert.Equal(t, "v1", md.ID)
	assert.Equal(t, int64(3), md.EndRow)
}

func TestRecoveryReplacesAbortedRecordFromFastClock(t *testing.T) {
	fake := installFakeElastic(t)
	now := freezeClock(t)

	// The write that failed came from a frontend whose clock is an
	// hour ahead of ours.
	ahead := now + int64(time.Hour)
	fake.md = []ResultSetMetadataRecord{
		{ID: "v1", EndRow: -1, TotalRows: -1, Timestamp: ahead}}

	writer := openTestWriter(t, true, result_sets.AppendMode, "v2")
	writer.WriteJSONL([]byte("{\"A\":1}\n"), 1)
	writer.Close()

	md, err := fake.getResultSetMetadata(context.Background(), nil, testLogPath)
	assert.NoError(t, err)
	assert.Equal(t, "v2", md.ID)
	assert.Equal(t, int64(0), md.TotalRows)
	assert.Equal(t, int64(1), md.EndRow)
}

// Compresses JSONL the way a client does for a compressed response.
func compressed(t *testing.T, jsonl string) []byte {
	data, err := utils.Compress([]byte(jsonl))
	assert.NoError(t, err)
	return data
}

const (
	batch1 = "{\"A\":1}\n{\"A\":2}\n"
	batch2 = "{\"A\":3}\n"
)

func TestCompressedJSONLWritesRowsInSequence(t *testing.T) {
	fake := installFakeElastic(t)

	writer := openTestWriter(t, true, result_sets.AppendMode, "v1")
	writer.WriteCompressedJSONL(compressed(t, batch1), 0, len(batch1), 2)
	writer.WriteCompressedJSONL(compressed(t, batch2), uint64(len(batch1)), len(batch2), 1)
	writer.Close()

	assert.Equal(t, 1, len(fake.rows))
	assert.Equal(t, batch1+batch2, fake.rows[0].JSONData)
	assert.Equal(t, int64(3), fake.rows[0].EndRow)

	md := fake.lastMD()
	assert.Equal(t, int64(3), md.EndRow)
	assert.Equal(t, int64(0), md.TotalRows)
	assert.Equal(t, int64(len(batch1+batch2)), *md.EndByte)
}

func TestCompressedJSONLOffsetContinuesAcrossWriters(t *testing.T) {
	fake := installFakeElastic(t)
	fake.md = []ResultSetMetadataRecord{{ID: "v1", EndByte: new(int64)}}

	// Each client response is written by a new writer, as in
	// ingestion, and a plain batch counts towards the offset too.
	writer := openTestWriter(t, true, result_sets.AppendMode, "v2")
	writer.WriteJSONL([]byte(batch1), 2)
	writer.Close()

	writer = openTestWriter(t, true, result_sets.AppendMode, "v3")
	writer.WriteCompressedJSONL(compressed(t, batch2), uint64(len(batch1)), len(batch2), 1)
	writer.Close()

	assert.Equal(t, 2, len(fake.rows))
	md := fake.lastMD()
	assert.Equal(t, "v1", md.ID)
	assert.Equal(t, int64(3), md.EndRow)
	assert.Equal(t, int64(0), md.TotalRows)
	assert.Equal(t, int64(len(batch1+batch2)), *md.EndByte)

	// A later writer is checked against the stored position.
	writer = openTestWriter(t, true, result_sets.AppendMode, "v4")
	writer.WriteCompressedJSONL(compressed(t, batch2), uint64(len(batch1)), len(batch2), 1)
	writer.Close()

	assert.Equal(t, 2, len(fake.rows))
	assert.Equal(t, int64(-1), fake.lastMD().TotalRows)
}

func TestCompressedJSONLCorruptDataAborts(t *testing.T) {
	fake := installFakeElastic(t)

	writer := openTestWriter(t, true, result_sets.AppendMode, "v1")
	writer.WriteCompressedJSONL([]byte("not zlib"), 0, len(batch1), 2)

	// Nothing more is written once aborted.
	writer.WriteCompressedJSONL(compressed(t, batch1), 0, len(batch1), 2)
	writer.Close()

	assert.Equal(t, 0, len(fake.rows))
	assert.Equal(t, int64(-1), fake.lastMD().TotalRows)
}

func TestCompressedJSONLLengthMismatchAborts(t *testing.T) {
	fake := installFakeElastic(t)

	writer := openTestWriter(t, true, result_sets.AppendMode, "v1")
	writer.WriteCompressedJSONL(compressed(t, batch1), 0, len(batch1)+1, 2)
	writer.Close()

	assert.Equal(t, 0, len(fake.rows))
	assert.Equal(t, int64(-1), fake.lastMD().TotalRows)
}

func TestCompressedJSONLOutOfSequenceOffsetAborts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		offset uint64
	}{
		{"repeated batch", 0},
		{"gap", uint64(len(batch1)) + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := installFakeElastic(t)

			writer := openTestWriter(t, true, result_sets.AppendMode, "v1")
			writer.WriteCompressedJSONL(compressed(t, batch1), 0, len(batch1), 2)
			writer.WriteCompressedJSONL(compressed(t, batch2), tc.offset, len(batch2), 1)
			writer.Close()

			// The first batch was still buffered, so it is discarded
			// with the rest of the result set.
			assert.Equal(t, 0, len(fake.rows))
			assert.Equal(t, int64(-1), fake.lastMD().TotalRows)
		})
	}
}

func TestCompressedJSONLStartsCheckingFromUnknownPosition(t *testing.T) {
	fake := installFakeElastic(t)

	// Records written before the byte position was tracked do not
	// have it, so the first batch's offset is taken as the position.
	fake.md = []ResultSetMetadataRecord{{ID: "v1", EndRow: 2}}

	writer := openTestWriter(t, true, result_sets.AppendMode, "v2")
	writer.WriteCompressedJSONL(compressed(t, batch2), 12345, len(batch2), 1)
	writer.Close()

	assert.Equal(t, 1, len(fake.rows))
	md := fake.lastMD()
	assert.Equal(t, int64(3), md.EndRow)
	assert.Equal(t, int64(0), md.TotalRows)
	assert.Equal(t, int64(12345+len(batch2)), *md.EndByte)

	// The next batch is checked against it.
	writer = openTestWriter(t, true, result_sets.AppendMode, "v3")
	writer.WriteCompressedJSONL(compressed(t, batch2), 12345, len(batch2), 1)
	writer.Close()

	assert.Equal(t, 1, len(fake.rows))
	assert.Equal(t, int64(-1), fake.lastMD().TotalRows)
}

func TestCompressedJSONLContinuesAfterRecovery(t *testing.T) {
	fake := installFakeElastic(t)

	// The client has already sent batch1 when a write fails and the
	// result set is aborted.
	fake.md = []ResultSetMetadataRecord{{ID: "v1", EndRow: 2,
		EndByte: new(int64)}}
	*fake.md[0].EndByte = int64(len(batch1))

	fake.fail_rs = true
	writer := openTestWriter(t, true, result_sets.AppendMode, "v2")
	writer.WriteCompressedJSONL(compressed(t, batch2), uint64(len(batch1)), len(batch2), 1)
	writer.Close()
	assert.Equal(t, int64(-1), fake.lastMD().TotalRows)

	// The new version does not know the client's position, so the
	// client's next batch is accepted rather than aborting again.
	fake.fail_rs = false
	offset := uint64(len(batch1 + batch2))
	writer = openTestWriter(t, true, result_sets.AppendMode, "v3")
	writer.WriteCompressedJSONL(compressed(t, batch2), offset, len(batch2), 1)
	writer.Close()

	md := fake.lastMD()
	assert.Equal(t, "v3", md.ID)
	assert.Equal(t, int64(1), md.EndRow)
	assert.Equal(t, int64(0), md.TotalRows)
	assert.Equal(t, int64(offset)+int64(len(batch2)), *md.EndByte)
}
