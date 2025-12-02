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

	version             string
	rows_per_result_set uint64
	max_size_per_packet uint64
}

// Not currently implemented but in future will be used to update
// result sets in the GUI
func (self *ElasticSimpleResultSetWriter) Update(uint64, *ordereddict.Dict) error {
	return errors.New("Updating result sets is not implemented yet.")
}

func (self *ElasticSimpleResultSetWriter) Abort() {
	self.md.TotalRows = -1
	self.md.EndRow = -1
	self.Close()
}

func (self *ElasticSimpleResultSetWriter) WriteJSONL(
	serialized []byte, total_rows uint64) {

	self.buff = append(self.buff, serialized...)
	self.buff = append(self.buff, '\n')
	self.buffered_rows += int(total_rows)

	// Flush depending on the total size of the buffer. If the rows
	// are large, we try to keep document size under 1mb.
	if uint64(self.buffered_rows) > self.rows_per_result_set ||
		uint64(len(self.buff)) > self.max_size_per_packet {
		self.Flush()
	}
}

// Write the JSONL record into a single document.
func (self *ElasticSimpleResultSetWriter) writeJSONL(
	serialized []byte, total_rows uint64) {

	record := NewSimpleResultSetRecord(self.log_path, self.version)
	record.JSONData = string(serialized)
	record.StartRow = self.start_row
	record.EndRow = self.start_row + int64(total_rows)
	record.Timestamp = utils.GetTime().Now().Unix()
	record.ID = self.version
	record.Type = "result_set"

	self.start_row = record.EndRow
	self.md.EndRow = record.EndRow

	record.TotalRows = uint64(self.start_row)

	if self.sync {
		err := services.SetElasticIndex(
			self.ctx, self.org_id, "transient",
			services.DocIdRandom, record)
		if err != nil {
			self.Abort()
		}
		return
	}

	services.SetElasticIndexAsync(
		self.org_id, "transient", services.DocIdRandom,
		cvelo_services.BulkUpdateCreate, record)
}

func (self *ElasticSimpleResultSetWriter) Write(row *ordereddict.Dict) {
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
	if self.buffered_rows == 0 {
		return
	}

	self.writeJSONL(self.buff, uint64(self.buffered_rows))
	self.buff = nil
	self.buffered_rows = 0

	// Write a newer version of the MD record.
	_ = SetResultSetMetadata(self.ctx, self.config_obj, self.log_path, self.md)

	// Make sure the results are visible immediately
	cvelo_services.FlushIndex(self.ctx, self.org_id, "transient")

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
