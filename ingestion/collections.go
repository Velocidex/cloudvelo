package ingestion

import (
	"context"
	"fmt"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/cloudvelo/result_sets/simple"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	cvelo_utils "www.velocidex.com/golang/cloudvelo/utils"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	// Add an error status only if no other error statuses were
	// present. This allows us to fail a collection when an error
	// level message is received, but there could be many error level
	// messages, we only record the first one.
	logErrorPainlessScript = `
if(ctx._source.errored == 0) {
   ctx._source.query_stats.add(params.status);
}
ctx._source.errored++;
`
)

func (self Ingestor) HandleLogs(
	ctx context.Context,
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

	// If the log is of type error, update the collection status to
	// fail it.
	if row.Level == logging.ERROR {
		// Create a fake error status and append it to the collection
		// record. This will show the collection as failed and include
		// the log message as the reason in the GUI.
		query := json.Format(updateQuery, logErrorPainlessScript,
			cvelo_utils.Clock.Now().UnixNano()/1000,
			json.MustMarshalString(&crypto_proto.VeloStatus{
				Status:       crypto_proto.VeloStatus_GENERIC_ERROR,
				ErrorMessage: row.Message,
			}))
		return cvelo_services.UpdateIndex(ctx, config_obj.OrgId,
			"collections", message.SessionId, query)
	}

	return nil
}

func (self Ingestor) HandleResponses(
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

	// We do not verify that this is a real artifact in order to avoid
	// having to maintain a full artifact repository and lookups. We
	// just blindly write it in the client's space.
	pathspec := getFSPathSpec(message, message.VQLResponse.Query.Name)
	file_store_factory := file_store.GetFileStore(config_obj)
	rs_writer, err := result_sets.NewResultSetWriter(
		file_store_factory, pathspec, json.NoEncOpts,
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

// In the ingestor we only have to identify CLIENT_EVENT or CLIENT
// type artifacts. We use the fact that client events are always sent
// to FlowId "F.Monitoring"
func getFSPathSpec(
	message *crypto_proto.VeloMessage,
	full_artifact_name string) api.FSPathSpec {
	base_artifact_name, artifact_source := paths.SplitFullSourceName(full_artifact_name)

	if message.SessionId == "F.Monitoring" {
		if artifact_source != "" {
			return paths.CLIENTS_ROOT.AsFilestorePath().
				SetType(api.PATH_TYPE_FILESTORE_JSON).
				AddChild(
					message.Source, "monitoring",
					base_artifact_name, artifact_source,
					getDayName())
		} else {
			return paths.CLIENTS_ROOT.AsFilestorePath().
				SetType(api.PATH_TYPE_FILESTORE_JSON).
				AddChild(
					message.Source, "monitoring",
					base_artifact_name,
					getDayName())
		}
	}

	// Simple Client type artifact
	if artifact_source != "" {
		return paths.CLIENTS_ROOT.AsFilestorePath().
			SetType(api.PATH_TYPE_FILESTORE_JSON).
			AddChild(
				message.Source, "artifacts",
				base_artifact_name, message.SessionId,
				artifact_source)
	} else {
		return paths.CLIENTS_ROOT.AsFilestorePath().
			SetType(api.PATH_TYPE_FILESTORE_JSON).
			AddChild(
				message.Source, "artifacts",
				base_artifact_name,
				message.SessionId)
	}
}

func getDayName() string {
	now := cvelo_utils.Clock.Now().UTC()
	return fmt.Sprintf("%d-%02d-%02d", now.Year(), now.Month(), now.Day())
}
