package simple

import (
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/cloudvelo/services"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
)

type ElasticSimpleResultSetWriter struct {
	log_path      api.FSPathSpec
	opts          *json.EncOpts
	buff          []byte
	buffered_rows int
	start_row     int64

	org_id string
}

func (self *ElasticSimpleResultSetWriter) WriteJSONL(
	serialized []byte, total_rows uint64) {

	record := NewSimpleResultSetRecord(self.log_path)
	record.JSONData = string(serialized)
	record.StartRow = self.start_row
	record.EndRow = self.start_row + int64(total_rows)
	record.Timestamp = time.Now().Unix()
	self.start_row = record.EndRow
	record.TotalRows = uint64(self.start_row)

	services.SetElasticIndex(
		self.org_id, "results", "", record)
}

func (self *ElasticSimpleResultSetWriter) Write(row *ordereddict.Dict) {
	serialized, err := json.MarshalWithOptions(row, self.opts)
	if err != nil {
		return
	}

	self.buff = append(self.buff, serialized...)
	self.buff = append(self.buff, '\n')
	self.buffered_rows++

	if self.buffered_rows > 100 {
		self.Flush()
	}
}

// Provide a hint to the writer that the next JSONL batch starts at
// this row count.
func (self *ElasticSimpleResultSetWriter) SetStartRow(start_row int64) {
	self.start_row = start_row
}

func (self *ElasticSimpleResultSetWriter) Flush() {
	if self.buffered_rows == 0 {
		return
	}

	self.WriteJSONL(self.buff, uint64(self.buffered_rows))
	self.buff = nil
	self.buffered_rows = 0

	// Make sure the results are visible immediately
	cvelo_services.FlushIndex(self.org_id, "results")
}

func (self *ElasticSimpleResultSetWriter) Close() {
	self.Flush()
}

func (self ElasticSimpleResultSetWriter) SetSync() {}
