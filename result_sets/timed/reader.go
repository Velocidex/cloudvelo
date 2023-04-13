package timed

import (
	"bufio"
	"context"
	"strings"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/cloudvelo/config"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
)

type TimedResultSetReader struct {
	start, end time.Time
	cancel     func()

	path_manager api.PathManager
	config_obj   *config.Config
}

func (self *TimedResultSetReader) GetAvailableFiles(
	ctx context.Context) []*api.ResultSetFileProperties {
	return nil
}

func (self *TimedResultSetReader) Debug() {}

func (self *TimedResultSetReader) SeekToTime(offset time.Time) error {
	self.start = offset
	return nil
}

func (self *TimedResultSetReader) SetMaxTime(end time.Time) {
	self.end = end
}

func (self *TimedResultSetReader) Close() {
	if self.cancel != nil {
		self.cancel()
	}
}

const getTimedRowsQuery = `
{
  "query": {
    "bool": {
      "must": [
         {"match": {"client_id": %q}},
         {"match": {"flow_id": %q}},
         {"match": {"artifact": %q}},
         {"match": {"type": %q}},
         {"range": {"timestamp": {"gte": %q}}},
         {"range": {"timestamp": {"lt": %q}}}
      ]
    }
  }
}
`

func (self *TimedResultSetReader) Rows(
	ctx context.Context) <-chan *ordereddict.Dict {
	output_chan := make(chan *ordereddict.Dict)

	go func() {
		defer close(output_chan)

		record := NewTimedResultSetRecord(self.path_manager)
		end := self.end
		if end.IsZero() {
			end = time.Unix(3000000000, 0)
		}
		start := self.start
		if start.IsZero() {
			start = time.Unix(0, 0)
		}

		// Client event artifacts always come from the monitoring
		// flow.
		if record.ClientId != "server" {
			record.FlowId = "F.Monitoring"
		}
		query := json.Format(getTimedRowsQuery,
			record.ClientId,
			record.FlowId,
			record.Artifact,
			record.Type,
			start.UnixNano(),
			end.UnixNano(),
		)
		subctx, cancel := context.WithCancel(ctx)
		defer cancel()

		self.cancel = cancel

		hits_chan, err := cvelo_services.QueryChan(
			subctx, self.config_obj.VeloConf(), 1000,
			self.config_obj.OrgId, "results", query,
			"timestamp")
		if err != nil {
			logger := logging.GetLogger(
				self.config_obj.VeloConf(), &logging.FrontendComponent)
			logger.Error("Reading %v: %v",
				self.path_manager, err)
			return
		}

		for hit := range hits_chan {
			record := &TimedResultSetRecord{}
			err = json.Unmarshal(hit, record)
			if err != nil {
				continue
			}

			reader := bufio.NewReader(strings.NewReader(record.JSONData))
			for {
				row_data, err := reader.ReadBytes('\n')
				if err != nil && len(row_data) == 0 {
					// Packet is exhausted, go get the next packet
					break
				}

				row := ordereddict.NewDict()
				err = row.UnmarshalJSON(row_data)
				if err != nil {
					continue
				}

				select {
				case <-ctx.Done():
					return

				case output_chan <- row.Set(
					"_ts", record.Timestamp/1000000):
				}
			}
		}
	}()

	return output_chan
}
