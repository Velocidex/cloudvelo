package downloads

import (
	"context"

	cryptozip "github.com/Velocidex/cryptozip"
	"github.com/sirupsen/logrus"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/csv"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/result_sets"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

// Write a CSV file into the zip with the result set in it.
func writeCSVResultSetToFile(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, hostname, password string,
	zip_writer *cryptozip.Writer,
	result_set_path api.FSPathSpec) error {

	zip_file_name := path_specs.CleanPathForZip(result_set_path.
		SetType(api.PATH_TYPE_FILESTORE_CSV),
		client_id, hostname)
	writer, err := createZipMember(zip_writer, zip_file_name, password)
	if err != nil {
		return err
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	reader, err := result_sets.NewResultSetReader(
		file_store_factory, result_set_path)
	if err != nil {
		return err
	}
	scope := vql_subsystem.MakeScope()
	csv_writer := csv.GetCSVAppender(
		config_obj, scope, writer, true /* write_headers */)
	for row := range reader.Rows(ctx) {
		csv_writer.Write(row)
	}
	csv_writer.Close()
	return nil
}

// Write a JSONL file into the zip with the result set in it. FIXME:
// This can be further optimized to avoid parsing the JSONL in the
// opensearch index if needed.
func writeJSONLResultSetToFile(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, hostname, password string,
	zip_writer *cryptozip.Writer,
	result_set_path api.FSPathSpec) error {

	zip_file_name := path_specs.CleanPathForZip(result_set_path.
		SetType(api.PATH_TYPE_FILESTORE_JSON),
		client_id, hostname)
	writer, err := createZipMember(zip_writer, zip_file_name, password)
	if err != nil {
		return err
	}

	file_store_factory := file_store.GetFileStore(config_obj)
	reader, err := result_sets.NewResultSetReader(
		file_store_factory, result_set_path)
	if err != nil {
		return err
	}

	for row := range reader.Rows(ctx) {
		serialized, err := row.MarshalJSON()
		if err == nil {
			writer.Write(serialized)
			writer.Write([]byte("\n"))
		}
	}
	return nil
}

func copyUpload(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, flow_id, hostname, password string,
	zip_writer *cryptozip.Writer,
	upload_name api.FSPathSpec) error {

	file_store_factory := file_store.GetFileStore(config_obj)
	reader, err := file_store_factory.ReadFile(upload_name)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Clean the name so it makes a reasonable zip member.
	file_member_name := path_specs.CleanPathForZip(
		upload_name, client_id, hostname)
	f, err := createZipMember(zip_writer, file_member_name, password)
	if err != nil {
		return err
	}

	_, err = utils.Copy(ctx, f, reader)
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.GUIComponent)
		logger.WithFields(logrus.Fields{
			"flow_id":     flow_id,
			"client_id":   client_id,
			"upload_name": upload_name,
		}).Error("Download Flow")
	}
	return err

}
