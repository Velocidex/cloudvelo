package simple

import (
	"context"
	"errors"
	"strconv"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/cloudvelo/services"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
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

	ctx context.Context

	// If this is set writes will be syncrounous
	sync bool
}

// Not currently implemented but in future will be used to update
// result sets in the GUI
func (self *ElasticSimpleResultSetWriter) Update(uint64, *ordereddict.Dict) error {
	return errors.New("Updating result sets is not implemented yet.")
}

func (self *ElasticSimpleResultSetWriter) WriteJSONL(
	serialized []byte, total_rows uint64) {

	record := NewSimpleResultSetRecord(self.log_path)
	record.JSONData = string(serialized)
	record.StartRow = self.start_row
	record.EndRow = self.start_row + int64(total_rows)
	record.Timestamp = utils.GetTime().Now().Unix()
	self.start_row = record.EndRow
	record.TotalRows = uint64(self.start_row)

	if self.sync {
		services.SetElasticIndex(
			self.ctx, self.org_id, "transient",
			services.DocIdRandom, record)
	} else {
		services.SetElasticIndexAsync(
			self.org_id, "transient", services.DocIdRandom,
			cvelo_services.BulkUpdateCreate, record)
	}
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
	self.truncated = true
}

const getLargestRowId = `
{
  "query": {
     "bool": {
       "must": [
            {"match": {"vfs_path": %q}}
       ]}
  },
  "size": 0,
  "aggs": {
    "genres": {
      "max": {"field": "end_row"}
    }
  }
}
`

func (self *ElasticSimpleResultSetWriter) getLastRow() error {
	ctx := context.Background()
	query := json.Format(getLargestRowId, self.log_path.AsClientPath())
	hits, err := services.QueryElasticAggregations(
		ctx, self.org_id, "transient", query)

	if err != nil {
		return err
	}

	for _, hit := range hits {
		end_row, err := strconv.ParseInt(hit, 10, 64)
		if err == nil {
			self.start_row = end_row
		}
		self.truncated = true
	}
	return nil
}

func (self *ElasticSimpleResultSetWriter) Flush() {
	if self.buffered_rows == 0 {
		return
	}

	if !self.truncated {
		self.getLastRow()
	}

	self.WriteJSONL(self.buff, uint64(self.buffered_rows))
	self.buff = nil
	self.buffered_rows = 0

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
