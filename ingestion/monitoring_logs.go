package ingestion

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/cloudvelo/result_sets/timed"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self ElasticIngestor) HandleMonitoringLogs(
	config_obj *config_proto.Config,
	message *crypto_proto.VeloMessage) error {

	row := message.LogMessage
	artifact_name := artifacts.DeobfuscateString(
		config_obj, row.Artifact)

	log_path_manager, err := artifact_paths.NewArtifactLogPathManager(
		config_obj, message.Source, message.SessionId, artifact_name)
	if err != nil {
		return err
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	rs_writer, err := timed.NewTimedResultSetWriter(
		file_store_factory, log_path_manager, json.NoEncOpts,
		utils.BackgroundWriter)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	rs_writer.Write(ordereddict.NewDict().
		Set("client_time", int64(row.Timestamp)/1000000).
		Set("level", row.Level).
		Set("message", row.Message))

	return nil
}

func (self ElasticIngestor) HandleMonitoringResponses(
	ctx context.Context, config_obj *config_proto.Config,
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

	// Automatically interrogate this client.
	case "Client.Info.Updates":
		err := self.HandleClientInfoUpdates(ctx, message)
		if err != nil {
			logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
			logger.Error("HandleClientInfoUpdates: %v", err)
		}
	}

	// Add the client id on the end of the record
	new_json_response := json.AppendJsonlItem(
		[]byte(message.VQLResponse.JSONLResponse), "ClientId", message.Source)

	path_manager, err := artifact_paths.NewArtifactPathManager(
		config_obj, message.Source,
		message.SessionId, message.VQLResponse.Query.Name)
	if err != nil {
		return err
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	rs_writer, err := timed.NewTimedResultSetWriter(
		file_store_factory, path_manager, json.NoEncOpts,
		utils.BackgroundWriter)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	rs_writer.WriteJSONL(new_json_response, int(message.VQLResponse.TotalRows))

	return nil
}
