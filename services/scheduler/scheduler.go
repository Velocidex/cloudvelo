package scheduler

import (
	"context"
	"errors"
	"time"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

/*
   This scheduler relies on Elastic:

   1. Jobs are written to the persistent index with field state = "available"

   2. Each worker updates a single element with its own state = worker
      ID - this means it leases the object which stops it from being
      leasted to another worker.

   3. Once the job is complete it is updated to have a type of
      scheduler_result and the job result is set to
      SchedulerResponseRecord.

   The caller which submits the job to the scheduler will wait for the
   appearance of the scheduler_result with the same ID and call its
   completion function.

   Note that currently calling the completion function is not strictly
   necessary. It is a way to dismiss the notebook calculation before
   the GUI poll time. If it is not properly done the GUI will have to
   wait an entire poll time (5 seconds) to update.

*/

// This is a wrapped version of services.SchedulerJob that will be
// serialized into the database/
type SchedulerRecordType struct {
	Timestamp int64 `json:"timestamp"`
	// A Unique ID for this job - will be placed into the result record.
	ID    string `json:"key"`
	Queue string `json:"name"`
	Job   string `json:"data"`
	OrgId string `json:"org_id"`
	State string `json:"state"` // "available" or ID of worker.
	Type  string `json:"type"`  // "scheduler"
}

type SchedulerResponseRecord struct {
	Timestamp int64  `json:"timestamp"`
	ID        string `json:"key"`
	Type      string `json:"type"` // "scheduler_result"
	Data      string `json:"data"`
	Error     string `json:"state"`
}

const (
	get_next_available_job = `{
"query": {
   "bool": {
     "must": [
       {"match": {"type": "scheduler"}},
       {"match": {"state": "available"}}
     ]
  }
},
"size": 1,
"_source": false
}`
)

type ElasticScheduler struct{}

// Worker loop - check for jobs, then try to lease them
func (self *ElasticScheduler) getOneJob(ctx context.Context,
	worker_id, queue string) (*services.SchedulerJob, error) {

	// Schedulers are global across all orgs.
	org_id := services.ROOT_ORG_ID

	ids, _, err := cvelo_services.QueryElasticIds(ctx, org_id,
		cvelo_services.PERSISTED, get_next_available_job)
	if err != nil || len(ids) == 0 {
		return nil, utils.NotFoundError
	}

	// Try to update it atomically - this will take ownership in a
	// race free manner.
	for _, doc_id := range ids {
		err = cvelo_services.UpdateIndex(
			ctx, org_id, cvelo_services.PERSISTED, doc_id,
			json.Format(`{
  "script": {
    "source": "if (ctx._source.state == 'available') { ctx._source.state = params.id; }",
    "lang": "painless",
    "params": {
       "key": %q
    }
  }
}`, worker_id))
		// We were unable to lease this id try another.
		if err != nil {
			continue
		}

		// Get the leased job
		hit, err := cvelo_services.GetElasticRecord(
			ctx, org_id, cvelo_services.PERSISTED, doc_id)
		if err != nil {
			continue
		}

		request := &SchedulerRecordType{}
		err = json.Unmarshal(hit, request)
		if err != nil {
			continue
		}

		// Create the job and pass it to the caller. When the caller
		// finished with it, we write the result record.
		return &services.SchedulerJob{
			Queue: request.Queue,
			Job:   request.Job,
			OrgId: request.OrgId,
			Done: func(result string, err error) {
				self.jobDone(ctx, result, err, org_id, doc_id, request)
			},
		}, nil
	}

	return nil, utils.NotFoundError
}

func (self *ElasticScheduler) jobDone(
	ctx context.Context,
	result string, err error, org_id, doc_id string, request *SchedulerRecordType) {
	// Write a result record and delete the old record.
	record := &SchedulerResponseRecord{
		Timestamp: utils.GetTime().Now().Unix(),
		ID:        request.ID,
		Type:      "scheduler_result",
		Data:      result,
	}

	if err != nil {
		record.Error = err.Error()
	}

	// Set and replace the old record with the
	// scheduler_result. The remote caller will find it
	// now.
	cvelo_services.SetElasticIndex(
		ctx, org_id, cvelo_services.PERSISTED, doc_id, record)
}

func (self *ElasticScheduler) RegisterWorker(ctx context.Context,
	name, queue string, priority int) (chan services.SchedulerJob, error) {

	worker_id := utils.ToString(utils.GetGUID())

	output_chan := make(chan services.SchedulerJob)
	go func() {
		defer close(output_chan)

		for {
			job, err := self.getOneJob(ctx, worker_id, queue)
			if err == nil {
				select {
				case <-ctx.Done():
					return
				case output_chan <- *job:
				}
				continue
			}

			select {
			case <-ctx.Done():
				return

			case <-time.After(utils.Jitter(500 * time.Millisecond)):
			}
		}
	}()

	return output_chan, nil
}

func (self *ElasticScheduler) Schedule(
	ctx context.Context, job services.SchedulerJob) (chan services.JobResponse, error) {

	// Currently we only support this queue
	if job.Queue != "Notebook" {
		return nil, errors.New("InlineScheduler: Queue not supported")
	}

	job_id := utils.ToString(utils.GetGUID())

	request := &SchedulerRecordType{
		Timestamp: utils.GetTime().Now().Unix(),
		ID:        job_id,
		Queue:     job.Queue,
		Job:       job.Job,
		OrgId:     job.OrgId,
		State:     "available",
		Type:      "scheduler",
	}

	// Schedulers are global across all orgs because we dont want each
	// org to have dedicated workers. Therefore we schedule all orgs
	// on the root org's index.
	org_id := services.ROOT_ORG_ID

	err := cvelo_services.SetElasticIndex(ctx, org_id, cvelo_services.PERSISTED,
		cvelo_services.DocIdRandom, request)
	if err != nil {
		return nil, err
	}

	output_chan := make(chan services.JobResponse)
	go func() {
		defer close(output_chan)

		// Now wait here for results.
		for i := 0; i < 1000; i++ {
			if i > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(utils.Jitter(500 * time.Millisecond)):
				}
			}

			hit, err := cvelo_services.GetElasticRecordByQuery(
				ctx, org_id, cvelo_services.PERSISTED,
				json.Format(`
{"query": {
   "bool": {
     "must": [
       {"match": {"type": "scheduler_result"}},
       {"match": {"key": %q}}
     ]
  }
 },
 "size": 1
}`, request.ID))
			if err != nil || len(hit) == 0 {
				continue
			}

			result := &SchedulerResponseRecord{}
			err = json.Unmarshal(hit, result)
			if err != nil {
				continue
			}

			job_response := services.JobResponse{
				Job: result.Data,
			}

			if result.Error != "" {
				job_response.Err = errors.New(result.Error)
			}

			select {
			case <-ctx.Done():
				return
			case output_chan <- job_response:
				return
			}
		}
	}()

	return output_chan, nil
}

func NewElasticScheduler() *ElasticScheduler {
	return &ElasticScheduler{}
}
