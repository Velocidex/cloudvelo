package ingestion

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/cloudvelo/result_sets/simple"
	cvelo_api "www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	cvelo_utils "www.velocidex.com/golang/cloudvelo/utils"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	defaultLogErrorRegex = regexp.MustCompile(constants.VQL_ERROR_REGEX)

	// If the config file specifies the regex we compile it once and
	// cache in memory.
	mu            sync.Mutex
	logErrorRegex *regexp.Regexp
)

func getLogErrorRegex(config_obj *config_proto.Config) *regexp.Regexp {
	if config_obj.Frontend.CollectionErrorRegex != "" {
		mu.Lock()
		defer mu.Unlock()

		if logErrorRegex == nil {
			logErrorRegex = regexp.MustCompile(
				config_obj.Frontend.CollectionErrorRegex)
		}
		return logErrorRegex
	}

	return defaultLogErrorRegex
}

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
	}

	payload := string(json.AppendJsonlItem([]byte(msg.Jsonl), "_ts",
		cvelo_utils.Clock.Now().UTC().Unix()))

	rs_writer.WriteJSONL([]byte(payload), uint64(msg.NumberOfRows))

	// Update the last active time for this collection.
	err = cvelo_services.SetElasticIndexAsync(
		config_obj.OrgId, "collections",
		message.SessionId+"_last_active",
		cvelo_api.ArtifactCollectorContext{
			ClientId:   message.Source,
			SessionId:  message.SessionId,
			Timestamp:  cvelo_utils.Clock.Now().UnixNano(),
			LastActive: uint64(cvelo_utils.Clock.Now().UnixNano()),
		})

	// Client will tag the errored message if the log message was
	// written with ERROR level.
	error_message := msg.ErrorMessage

	// One of the messages triggered an error - we need to figure
	// out which so we parse the JSONL payload to lock in on the
	// first errored message.
	if error_message == "" &&
		getLogErrorRegex(config_obj).FindStringIndex(payload) != nil {
		for _, line := range strings.Split(payload, "\n") {
			if getLogErrorRegex(config_obj).FindStringIndex(line) != nil {
				msg := ordereddict.NewDict()
				err := json.Unmarshal([]byte(line), msg)
				if err == nil {
					error_message, _ = msg.GetString("message")
				}
			}
		}
	}

	if error_message != "" {
		// Create a fake error status and append it to the collection
		// record. This will show the collection as failed and include
		// the log message as the reason in the GUI.
		return cvelo_services.SetElasticIndexAsync(
			config_obj.OrgId, "collections",
			message.SessionId+"_status_"+cvelo_utils.GetId(),
			cvelo_api.ArtifactCollectorContext{
				ClientId:  message.Source,
				SessionId: message.SessionId,
				Timestamp: cvelo_utils.Clock.Now().UnixNano(),
				QueryStats: []string{
					json.MustMarshalString(&crypto_proto.VeloStatus{
						Status:       crypto_proto.VeloStatus_GENERIC_ERROR,
						ErrorMessage: error_message,
					}),
				},
			})
	}

	return nil
}
