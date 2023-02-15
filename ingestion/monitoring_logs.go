package ingestion

import (
	"context"

	"www.velocidex.com/golang/cloudvelo/result_sets/timed"
	cvelo_utils "www.velocidex.com/golang/cloudvelo/utils"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	artifact_paths "www.velocidex.com/golang/velociraptor/paths/artifacts"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self Ingestor) HandleMonitoringLogs(
	ctx context.Context, config_obj *config_proto.Config,
	message *crypto_proto.VeloMessage) error {

	row := message.LogMessage
	artifact_name := artifacts.DeobfuscateString(
		config_obj, row.Artifact)

	// Suppress logging of some artifacts
	switch artifact_name {

	// Automatically interrogate this client.
	case "Client.Info.Updates":
		return nil
	}

	new_json_response := artifacts.DeobfuscateString(config_obj, row.Jsonl)

	log_path_manager := artifact_paths.NewArtifactLogPathManagerWithMode(
		config_obj, message.Source, message.SessionId, artifact_name,
		paths.MODE_CLIENT_EVENT)
	log_path_manager.Clock = cvelo_utils.Clock

	file_store_factory := file_store.GetFileStore(config_obj)
	rs_writer, err := timed.NewTimedResultSetWriter(
		file_store_factory, log_path_manager, json.DefaultEncOpts(),
		utils.BackgroundWriter)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	rs_writer.WriteJSONL([]byte(new_json_response), int(row.NumberOfRows))

	return nil
}

func (self Ingestor) HandleMonitoringResponses(
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
	case "Server.Internal.ClientInfo":
		return self.HandleClientInfoUpdates(ctx, message)
	}

	// Add the client id on the end of the record
	new_json_response := json.AppendJsonlItem(
		[]byte(message.VQLResponse.JSONLResponse), "ClientId", message.Source)

	path_manager := artifact_paths.NewArtifactPathManagerWithMode(
		config_obj, message.Source,
		message.SessionId, message.VQLResponse.Query.Name,
		paths.MODE_CLIENT_EVENT)
	path_manager.Clock = cvelo_utils.Clock

	file_store_factory := file_store.GetFileStore(config_obj)
	rs_writer, err := timed.NewTimedResultSetWriter(
		file_store_factory, path_manager, json.DefaultEncOpts(),
		utils.BackgroundWriter)
	if err != nil {
		return err
	}
	defer rs_writer.Close()

	rs_writer.WriteJSONL(new_json_response, int(message.VQLResponse.TotalRows))

	return nil
}
