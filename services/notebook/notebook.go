package notebook

import (
	"context"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/notebook"
)

// The NotebookManager service is the main entry point to the
// notebooks. It is composed by various storage related
// implementations which can be locally overriden for cloud
// environments.
type NotebookManager struct {
	*notebook.NotebookManager
	config_obj *config_proto.Config
}

func NewNotebookManagerService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) services.NotebookManager {

	timeline_storer := NewSuperTimelineStorer(config_obj)
	store := NewNotebookStore(ctx, wg, config_obj, timeline_storer)

	annotator := NewSuperTimelineAnnotator(config_obj, timeline_storer)

	notebook_service := notebook.NewNotebookManager(config_obj, store,
		timeline_storer, &SuperTimelineReader{}, &SuperTimelineWriter{},
		annotator, NewAttachmentManager(config_obj, store))

	return notebook_service
}
