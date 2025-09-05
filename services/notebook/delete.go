package notebook

import (
	"context"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/vfilter"
)

func (self *NotebookStoreImpl) DeleteNotebook(ctx context.Context,
	notebook_id string, progress chan vfilter.Row,
	really_do_it bool) error {

	// TODO - recursively delete all the notebook items.

	if really_do_it {
		return cvelo_services.DeleteDocument(ctx, self.config_obj.OrgId,
			"persisted", notebook_id, cvelo_services.SyncDelete)
	}
	return nil
}
