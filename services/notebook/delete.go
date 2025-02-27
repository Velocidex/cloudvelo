package notebook

import (
	"context"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/vfilter"
)

func (self *NotebookStoreImpl) DeleteNotebook(ctx context.Context,
	notebook_id string, progress chan vfilter.Row,
	really_do_it bool) error {
	err := self.NotebookStoreImpl.DeleteNotebook(ctx, notebook_id, progress, really_do_it)
	if err != nil {
		return err
	}

	if really_do_it {
		return cvelo_services.DeleteDocument(ctx, self.config_obj.OrgId,
			"persisted", notebook_id, cvelo_services.SyncDelete)
	}
	return nil
}
