package simple

import (
	"context"
	"errors"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/cloudvelo/services"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/utils"
)

type ElasticSimpleResultSetWriter struct {
	log_path      api.FSPathSpec
	opts          *json.EncOpts
	buff          []byte
	buffered_rows int
	start_row     int64

	org_id string

	// Marks if the file is truncated or the offset was specifically
	// set. If it is not then we need to find the last start row
	// before writing anything (which is another database round trip
	// and can be expensive).
	truncated bool

	ctx        context.Context
	config_obj *config_proto.Config

	// If this is set writes will be syncrounous
	sync bool

	md *ResultSetMetadataRecord

	// Set once the result set is aborted. After this no further
	// writes or flushes are made.
	aborted bool

	version             string
	rows_per_result_set uint64
	max_size_per_packet uint64
}

// Not currently implemented but in future will be used to update
// result sets in the GUI
func (self *ElasticSimpleResultSetWriter) Update(uint64, *ordereddict.Dict) error {
	return errors.New("Updating result sets is not implemented yet.")
}

// Abort the result set: any buffered rows are discarded and the
// metadata is marked with TotalRows = -1 so readers refuse to open
// it. Abort does not go through Flush so it can be safely called
// from a failed write without re-entering the write path.
func (self *ElasticSimpleResultSetWriter) Abort() {
	if self.aborted {
		return
	}
	self.aborted = true

	self.buff = nil
	self.buffered_rows = 0

	self.md.TotalRows = -1
	self.md.EndRow = -1

	_ = SetResultSetMetadata(self.ctx, self.config_obj, self.log_path, self.md)
	_ = flushIndex(self.ctx, self.org_id, "transient")
}

func (self *ElasticSimpleResultSetWriter) WriteJSONL(
	serialized []byte, total_rows uint64) {
	if self.aborted {
		return
	}

	// Valid JSONL should be followed by \n already
	self.buff = append(self.buff, serialized...)
	self.buffered_rows += int(total_rows)

	// Flush depending on the total size of the buffer. If the rows
	// are large, we try to keep document size under 1mb.
	if uint64(self.buffered_rows) > self.rows_per_result_set ||
		uint64(len(self.buff)) > self.max_size_per_packet {
		self.Flush()
	}
}

// Write the JSONL record into a single document. The row counters
// are only advanced once the record is accepted.
func (self *ElasticSimpleResultSetWriter) writeJSONL(
	serialized []byte, total_rows uint64) error {

	record := NewSimpleResultSetRecord(self.log_path, self.version)
	record.JSONData = string(serialized)
	record.StartRow = self.start_row
	record.EndRow = self.start_row + int64(total_rows)
	record.Timestamp = utils.GetTime().Now().Unix()
	record.ID = self.version
	record.Type = "result_set"
	record.TotalRows = uint64(record.EndRow)

	if self.sync {
		err := setElasticIndex(
			self.ctx, self.org_id, "transient",
			services.DocIdRandom, record)
		if err != nil {
			return err
		}
	} else {
		// Async failures are reported by the bulk indexer.
		_ = setElasticIndexAsync(
			self.org_id, "transient", services.DocIdRandom,
			cvelo_services.BulkUpdateCreate, record)
	}

	self.start_row = record.EndRow
	self.md.EndRow = record.EndRow
	if self.md.EndByte != nil {
		*self.md.EndByte += int64(len(serialized))
	}

	return nil
}

// The Elastic backend stores plain JSONL rows so there is nowhere to keep
// a compressed blob and its chunk index. Inflate the batch and store it as
// normal rows rather than dropping it. The interface gives us no way to
// report an error, so a batch that is corrupt or out of sequence aborts
// the result set rather than storing rows in the wrong place.
func (self *ElasticSimpleResultSetWriter) WriteCompressedJSONL(
	serialized []byte, byte_offset uint64,
	uncompressed_size int, total_rows uint64) {
	if self.aborted {
		return
	}

	uncompressed, err := utils.Uncompress(self.ctx, serialized)
	if err != nil {
		self.abortWithError("can not decompress batch: %v", err)
		return
	}

	if len(uncompressed) != uncompressed_size {
		self.abortWithError(
			"batch decompressed to %v bytes, expected %v",
			len(uncompressed), uncompressed_size)
		return
	}

	next_byte, known := self.nextByte()
	if !known {
		// New result sets and ones written before the byte position
		// was tracked do not record it. Start from this batch's
		// offset so the batches after it are checked.
		if len(self.buff) == 0 {
			start := int64(byte_offset)
			self.md.EndByte = &start
		}

	} else if byte_offset != uint64(next_byte) {
		self.abortWithError("batch starts at byte %v, expected %v",
			byte_offset, next_byte)
		return
	}

	self.WriteJSONL(uncompressed, total_rows)
}

// The position in the uncompressed JSONL stream where the next batch
// starts: the bytes already stored plus the bytes still buffered.
func (self *ElasticSimpleResultSetWriter) nextByte() (int64, bool) {
	if self.md.EndByte == nil {
		return 0, false
	}
	return *self.md.EndByte + int64(len(self.buff)), true
}

func (self *ElasticSimpleResultSetWriter) abortWithError(
	format string, args ...interface{}) {
	logger := logging.GetLogger(self.config_obj, &logging.FrontendComponent)
	logger.Error("Aborting result set %v: "+format,
		append([]interface{}{self.log_path.AsClientPath()}, args...)...)
	self.Abort()
}

func (self *ElasticSimpleResultSetWriter) Write(row *ordereddict.Dict) {
	if self.aborted {
		return
	}

	serialized, err := json.MarshalWithOptions(row, self.opts)
	if err != nil {
		return
	}

	self.buff = append(self.buff, serialized...)
	self.buff = append(self.buff, '\n')
	self.buffered_rows++

	// Flush depending on the total size of the buffer. If the rows
	// are large, we try to keep document size under 1mb.
	if uint64(self.buffered_rows) > self.rows_per_result_set ||
		uint64(len(self.buff)) > self.max_size_per_packet {
		self.Flush()
	}
}

// Provide a hint to the writer that the next JSONL batch starts at
// this row count.
func (self *ElasticSimpleResultSetWriter) SetStartRow(start_row int64) error {
	self.start_row = start_row
	self.truncated = true

	return nil
}

func (self *ElasticSimpleResultSetWriter) Flush() {
	if self.aborted || self.buffered_rows == 0 {
		return
	}

	// Take ownership of the buffer before writing so a failed write
	// is never retried by a re-entrant Flush.
	buff := self.buff
	buffered_rows := self.buffered_rows
	self.buff = nil
	self.buffered_rows = 0

	err := self.writeJSONL(buff, uint64(buffered_rows))
	if err != nil {
		self.Abort()
		return
	}

	// Write a newer version of the MD record.
	_ = SetResultSetMetadata(self.ctx, self.config_obj, self.log_path, self.md)

	// Make sure the results are visible immediately
	_ = flushIndex(self.ctx, self.org_id, "transient")

	// No need to find the last start row as we assume we are the only
	// writers.
	self.truncated = true
}

func (self *ElasticSimpleResultSetWriter) Close() {
	self.Flush()
}

func (self *ElasticSimpleResultSetWriter) SetSync() {
	self.sync = true
}
