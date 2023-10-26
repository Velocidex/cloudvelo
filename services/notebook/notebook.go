package notebook

import (
	"context"
	"sync"

	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services/notebook"
)

type NotebookManager struct {
	*notebook.NotebookManager
	config_obj *config_proto.Config
}

func NewNotebookManager(
	config_obj *config_proto.Config,
	storage notebook.NotebookStore) *NotebookManager {
	result := &NotebookManager{
		config_obj: config_obj,
		NotebookManager: notebook.NewNotebookManager(
			config_obj, storage),
	}
	return result
}

func NewNotebookManagerService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) *NotebookManager {

	service := NewNotebookManager(config_obj,
		NewNotebookStoreImpl(ctx, config_obj))
	return service
}

func NewNotebookStoreImpl(ctx context.Context,
	config_obj *config_proto.Config) *NotebookStoreImpl {
	return &NotebookStoreImpl{
		ctx:        ctx,
		config_obj: config_obj,
	}
}
