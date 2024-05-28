package scheduler

import (
	"context"
	"encoding/json"
	"errors"

	cloud_notebooks "www.velocidex.com/golang/cloudvelo/services/notebook"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/notebook"
)

type InlineScheduler struct{}

func (self *InlineScheduler) RegisterWorker(ctx context.Context,
	name, queue string, priority int) (chan services.SchedulerJob, error) {
	return nil, errors.New("InlineScheduler:RegisterWorker not implemented")
}

func (self *InlineScheduler) Schedule(
	ctx context.Context, job services.SchedulerJob) (chan services.JobResponse, error) {

	// Currently we only support this queue
	if job.Queue != "Notebook" {
		return nil, errors.New("InlineScheduler: Queue not supported")
	}

	request := &notebook.NotebookRequest{}
	err := json.Unmarshal([]byte(job.Job), request)
	if err != nil {
		return nil, err
	}

	// Get the config object for the correct org.
	org_manager, err := services.GetOrgManager()
	if err != nil {
		return nil, err
	}

	org_config_obj, err := org_manager.GetOrgConfig(job.OrgId)
	if err != nil {
		return nil, err
	}

	// Just run the update inline
	storage := cloud_notebooks.NewNotebookStore(ctx, org_config_obj)

	worker := &notebook.NotebookWorker{}
	response, err := worker.ProcessUpdateRequest(
		ctx, org_config_obj, request, storage)
	if err != nil {
		return nil, err
	}

	serialized, _ := json.Marshal(response)
	if response == nil {
		serialized = nil
	}

	output_chan := make(chan services.JobResponse)
	go func() {
		defer close(output_chan)

		output_chan <- services.JobResponse{
			Job: string(serialized),
			Err: err,
		}
	}()

	return output_chan, nil
}

func NewInlineScheduler() *InlineScheduler {
	return &InlineScheduler{}
}

func init() {
	services.RegisterScheduler(NewInlineScheduler())
}
