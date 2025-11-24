/*

Velociraptor result sets store arbitrary structured JSON data as
obtained from the VQL query. The client sends JSONL encoded packets
spanning multiple rows.

This implementation stores each raw packet in a separate elastic
document. The docuemnt also contains start_row and end_row which use
for seeking to the right document that spans the start row of
interest.

FAQ:
- Why not index the JSONL with elastic instead of keep it in a blob?
* The form of the JSONL is free form and depends on the VQL
  query. Indexing it into elastic is not really possible because it
  will change all the time. It is also more efficient for us to store
  larger chunks in reasonably sized elastic documented rather than
  lots of very small ones.

*/

package simple

import (
	"bufio"
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/cloudvelo/filestore"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/result_sets"
)

type SimpleResultSetReader struct {
	file_store_factory api.FileStore
	row                int64
	log_path           api.FSPathSpec
	opts               result_sets.ResultSetOptions
	base_record        *SimpleResultSetRecord
	mtime              time.Time
	md                 *ResultSetMetadataRecord

	stacker api.FSPathSpec
}

// TODO: for now seek position is approximate: we seek to the next
// packet with a start row after the desired position - refine that
// for next iteration so we can seek on sub packet row index.
func (self *SimpleResultSetReader) SeekToRow(start int64) error {
	self.row = start
	return nil
}

// Gets the elastic query for retriving the packet that has JSONL
// encompassing the required row.
func (self *SimpleResultSetReader) getPacket(
	ctx context.Context, row int64) (*SimpleResultSetRecord, error) {

	cvelo_services.Count("SimpleResultSet: getPacket")
	cvelo_services.Debug(
		cvelo_services.DEBUG_RESULT_SET, "getPacket: %v (%v)",
		self.log_path, row)()

	var artifact_clause, query string

	if self.base_record.VFSPath != "" {
		query = json.Format(`
{"query": {"bool": {"must": [
  {"match": {"vfs_path": %q}},
  {"match": {"id": %q}},
  {"match": {"type": "result_set"}},
  {"range": {"start_row": {"lte": %q}}},
  {"range": {"end_row": {"gt": %q}}}
]}}}`, self.base_record.VFSPath, self.base_record.ID, row, row)

	} else {
		if self.base_record.Artifact != "" {
			artifact_clause = json.Format(
				`,{"match": {"artifact": %q}}`, self.base_record.Artifact)
		}

		// No need to sort as we only get one result.
		query = json.Format(`
{"query": {"bool": {"must": [
  {"match": {"client_id": %q}},
  {"match": {"flow_id": %q}},
  {"match": {"type": %q}},
  {"match": {"id": %q}},
  {"match": {"type": "result_set"}},
  {"range": {"start_row": {"lte": %q}}},
  {"range": {"end_row": {"gt": %q}}}%s
]}}}`, self.base_record.ClientId,
			self.base_record.FlowId,
			self.base_record.Type,
			self.base_record.ID,
			row,
			row,
			artifact_clause)
	}

	org_id := filestore.GetOrgId(self.file_store_factory)
	hits, _, err := cvelo_services.QueryElasticRaw(ctx, org_id, "transient", query)
	if err != nil {
		return nil, err
	}

	if len(hits) == 0 {
		return nil, errors.New("Not found")
	}

	item := &SimpleResultSetRecord{}
	err = json.Unmarshal(hits[0], &item)
	if err != nil {
		return nil, err
	}

	return item, nil
}

func (self *SimpleResultSetReader) Rows(
	ctx context.Context) <-chan *ordereddict.Dict {
	output_chan := make(chan *ordereddict.Dict)

	last_row := int64(-1)

	go func() {
		defer close(output_chan)

		for {
			// No progress has been made something is wrong.
			if last_row == self.row {
				break
			}

			// We read enough rows
			if self.md.EndRow > 0 && self.row >= self.md.EndRow {
				break
			}

			packet, err := self.getPacket(ctx, self.row)
			if err != nil {
				return
			}
			last_row = self.row

			start_row := packet.StartRow
			reader := bufio.NewReader(strings.NewReader(packet.JSONData))
			for {
				row_data, err := reader.ReadBytes('\n')
				if err != nil && len(row_data) == 0 {
					// Packet is exhausted, go get the next packet
					break
				}

				// Consume the first few rows until we get to the one
				// we need.
				if start_row < self.row {
					start_row++
					continue
				}
				self.row++
				start_row++

				row := ordereddict.NewDict()
				err = row.UnmarshalJSON(row_data)
				if err != nil {
					continue
				}

				select {
				case <-ctx.Done():
					return

				case output_chan <- row:
				}
			}
		}

	}()

	return output_chan
}

func (self *SimpleResultSetReader) JSON(
	ctx context.Context) (<-chan []byte, error) {
	output_chan := make(chan []byte)

	last_row := int64(-1)

	go func() {
		defer close(output_chan)

		for {
			// No progress has been made something is wrong.
			if last_row == self.row {
				break
			}

			// We read enough rows
			if self.md.EndRow > 0 && self.row >= self.md.EndRow {
				break
			}

			packet, err := self.getPacket(ctx, self.row)
			if err != nil {
				return
			}
			last_row = self.row

			start_row := packet.StartRow
			reader := bufio.NewReader(strings.NewReader(packet.JSONData))
			for {
				row_data, err := reader.ReadBytes('\n')
				if err != nil && len(row_data) == 0 {
					// Packet is exhausted, go get the next packet
					break
				}

				// Consume the first few rows until we get to the one
				// we need.
				if start_row < self.row {
					start_row++
					continue
				}
				self.row++
				start_row++

				select {
				case <-ctx.Done():
					return

				case output_chan <- row_data:
				}
			}
		}

	}()

	return output_chan, nil
}

func (self *SimpleResultSetReader) MTime() time.Time {
	return self.mtime
}

func (self *SimpleResultSetReader) Stacker() api.FSPathSpec {
	return self.stacker
}

func (self *SimpleResultSetReader) SetStacker(s api.FSPathSpec) {
	self.stacker = s
}

func (self *SimpleResultSetReader) Close() {}

// Figure out how many rows are in this collection in total.
func (self *SimpleResultSetReader) TotalRows() int64 {
	return self.md.EndRow
}

// Deprecated: Current code sets the last row number in another
// metadata document in the EndRow field. This is here for backwards
// compatibility.
func getLastRecord(
	ctx context.Context,
	org_id string,
	base_record *SimpleResultSetRecord) (*SimpleResultSetRecord, error) {

	var artifact_clause, query string

	cvelo_services.Count("SimpleResultSet: getLastRecord")

	if base_record.VFSPath != "" {
		query = json.Format(`
{"sort": {"end_row": {"order": "desc"}},
 "size": 1,
 "query": {"bool": {"must": [
   {"match": {"id": %q}},
   {"match": {"type": "result_set"}},
   {"match": {"vfs_path": %q}}
 ]}}}`, base_record.ID, base_record.VFSPath)

	} else {
		if base_record.Artifact != "" {
			artifact_clause = json.Format(
				`,{"match": {"artifact": %q}}`, base_record.Artifact)
		}

		query = json.Format(`
{"sort": {"end_row": {"order": "desc"}},
 "size": 1,
 "query": {"bool": {"must": [
   {"match": {"id": %q}},
   {"match": {"client_id": %q}},
   {"match": {"type": "result_set"}},
   {"match": {"flow_id": %q}},
   {"match": {"type": %q}}%s
 ]}}}`, base_record.ID, base_record.ClientId,
			base_record.FlowId,
			base_record.Type,
			artifact_clause)
	}
	hits, _, err := cvelo_services.QueryElasticRaw(ctx, org_id,
		"transient", query)
	if err != nil {
		return nil, err
	}

	for _, hit := range hits {
		item := &SimpleResultSetRecord{}
		err = json.Unmarshal(hit, &item)
		if err != nil {
			continue
		}

		return item, nil
	}

	return nil, errors.New("Not found")
}
