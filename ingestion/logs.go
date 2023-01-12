package ingestion

import (
	"context"
	"errors"

	"www.velocidex.com/golang/cloudvelo/result_sets/simple"
	cvelo_utils "www.velocidex.com/golang/cloudvelo/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

func (self Ingestor) HandleLogs(
	ctx context.Context,
	config_obj *config_proto.Config,
	message *crypto_proto.VeloMessage) error {

	msg := message.LogMessage
	if msg.Jsonl == "" {
		return errors.New("Invalid log messages response")
	}

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
		elastic_writer.SetStartRow(msg.Id)

		// Urgent messages are GUI driven so we need to see logs
		// immediately.
		if message.Urgent {
			elastic_writer.SetSync()
		}
	}

	payload := string(json.AppendJsonlItem([]byte(msg.Jsonl), "_ts",
		cvelo_utils.Clock.Now().UTC().Unix()))

	rs_writer.WriteJSONL([]byte(payload), uint64(msg.NumberOfRows))

	return nil
}
