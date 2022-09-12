package ingestion

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/cloudvelo/result_sets/simple"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self ElasticIngestor) HandleLogs(
	config_obj *config_proto.Config,
	message *crypto_proto.VeloMessage) error {

	row := message.LogMessage
	log_path_manager := paths.NewFlowPathManager(
		message.Source, message.SessionId).Log()

	file_store_factory := file_store.GetFileStore(config_obj)
	rs_writer, err := result_sets.NewResultSetWriter(
		file_store_factory, log_path_manager, json.NoEncOpts,
		utils.BackgroundWriter, result_sets.AppendMode)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	elastic_writer, ok := rs_writer.(*simple.ElasticSimpleResultSetWriter)
	if ok {
		elastic_writer.SetStartRow(row.Id)
	}

	rs_writer.Write(ordereddict.NewDict().
		Set("client_time", int64(row.Timestamp)/1000000).
		Set("level", row.Level).
		Set("message", row.Message))

	return nil
}

func (self ElasticIngestor) HandleResponses(
	ctx context.Context,
	config_obj *config_proto.Config,
	message *crypto_proto.VeloMessage) error {

	// Ignore messages without a destination Artifact
	if message.VQLResponse == nil || message.VQLResponse.Query == nil ||
		message.VQLResponse.Query.Name == "" {
		return nil
	}

	response := message.VQLResponse
	artifacts.Deobfuscate(config_obj, response)

	// Handle special types of responses
	switch message.VQLResponse.Query.Name {
	case "System.VFS.ListDirectory":
		_ = self.HandleSystemVfsListDirectory(ctx, config_obj, message)

	case "System.VFS.DownloadFile":
		_ = self.HandleSystemVfsUpload(ctx, config_obj, message)
	}

	path_manager, err := artifact_paths.NewArtifactPathManager(
		config_obj, message.Source,
		message.SessionId, message.VQLResponse.Query.Name)
	if err != nil {
		return err
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	rs_writer, err := result_sets.NewResultSetWriter(
		file_store_factory, path_manager.Path(), json.NoEncOpts,
		utils.BackgroundWriter, result_sets.AppendMode)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	elastic_writer, ok := rs_writer.(*simple.ElasticSimpleResultSetWriter)
	if ok {
		elastic_writer.SetStartRow(int64(message.VQLResponse.QueryStartRow))
	}

	rs_writer.WriteJSONL([]byte(message.VQLResponse.JSONLResponse),
		message.VQLResponse.TotalRows)

	return nil
}
