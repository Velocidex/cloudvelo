package ingestion

import (
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/cloudvelo/result_sets/simple"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self ElasticIngestor) HandleUploads(
	config_obj *config_proto.Config,
	message *crypto_proto.VeloMessage) error {

	if message.FileBuffer == nil || message.FileBuffer.Pathspec == nil {
		return nil
	}

	response := message.FileBuffer

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
			Set("Timestamp", time.Now().Unix()).
			Set("started", time.Now()).
			Set("vfs_path", response.Pathspec.Path).
			Set("file_size", response.Size).
			Set("uploaded_size", response.StoredSize))

	return nil
}
