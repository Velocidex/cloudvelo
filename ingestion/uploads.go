package ingestion

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/cloudvelo/filestore"
	"www.velocidex.com/golang/cloudvelo/result_sets/simple"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	cvelo_utils "www.velocidex.com/golang/cloudvelo/utils"
	"www.velocidex.com/golang/cloudvelo/vql/uploads"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	updateUploadStatsScript = `
ctx._source.uploaded_files += params.uploaded_files;
ctx._source.uploaded_bytes += params.uploaded_bytes
`

	updateUploadStats = `
{
   "script": {
     "source": %q,
     "lang": "painless",
     "params": {
       "uploaded_files": %q,
       "uploaded_bytes": %q
     }
   }
}
`
)

// Uploads are being sent separately to the server handler by the
// client. The FileBuffer message only sends metadata about the
// upload.
func (self Ingestor) HandleUploads(
	ctx context.Context,
	config_obj *config_proto.Config,
	message *crypto_proto.VeloMessage) error {

	if message.FileBuffer == nil || message.FileBuffer.Pathspec == nil {
		return nil
	}

	response := message.FileBuffer

	// Figure out where in the filestore the server's
	// StartMultipartUpload placed it.
	components := filestore.S3ComponentsForClientUpload(&uploads.UploadRequest{
		ClientId:   message.Source,
		SessionId:  message.SessionId,
		Accessor:   message.FileBuffer.Pathspec.Accessor,
		Components: message.FileBuffer.Pathspec.Components,
	})

	path_manager := paths.NewFlowPathManager(
		message.Source, message.SessionId)
	file_store_factory := file_store.GetFileStore(config_obj)
	rs_writer, err := result_sets.NewResultSetWriter(
		file_store_factory, path_manager.UploadMetadata(), json.NoEncOpts,
		utils.BackgroundWriter, result_sets.AppendMode)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	elastic_writer, ok := rs_writer.(*simple.ElasticSimpleResultSetWriter)
	if ok {
		elastic_writer.SetStartRow(int64(response.UploadNumber))
	}

	rs_writer.Write(
		ordereddict.NewDict().
			Set("Timestamp", cvelo_utils.Clock.Now().Unix()).
			Set("started", cvelo_utils.Clock.Now()).
			Set("vfs_path", response.Pathspec.Path).
			Set("_Components", components).
			Set("file_size", response.Size).
			Set("uploaded_size", response.StoredSize))

	uploaded_files := 0
	if response.Offset == 0 {
		uploaded_files = 1
	}

	return cvelo_services.UpdateIndex(ctx, config_obj.OrgId, "collections",
		message.SessionId,
		json.Format(
			updateUploadStats, updateUploadStatsScript,
			uploaded_files, response.Size))

}
