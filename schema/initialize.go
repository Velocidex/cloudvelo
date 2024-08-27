package schema

import (
	"context"
	"embed"
	_ "embed"
	"os"
	"path"
	"strings"
	"www.velocidex.com/golang/cloudvelo/config"

	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"
	"github.com/pkg/errors"
	"www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

const (
	RESET_INDEX        = true
	DO_NOT_RESET_INDEX = false

	NO_FILTER = ""
)

var TRUE = true

//go:embed policies/*.json
//go:embed templates/*.json
var fs embed.FS

// Just remove the indexes for the org but do not create ones
func Delete(ctx context.Context,
	config_obj *config_proto.Config, org_id, filter string) error {

	client, err := services.GetElasticClient(org_id)
	if err != nil {
		return err
	}

	// Delete previously created index.
	_, err = opensearchapi.IndicesDeleteRequest{
		Index: []string{org_id + "*"},
	}.Do(ctx, client)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// Now roll over any data stream indexes to make sure they are
	// deleted.
	files, err := fs.ReadDir("templates")
	if err != nil {
		return err
	}

	for _, filename := range files {
		name := strings.Split(filename.Name(), ".")[0]
		data, err := fs.ReadFile(path.Join("templates", filename.Name()))
		if err != nil {
			return err
		}

		if strings.Contains(string(data), "data_stream") {
			_, err = opensearchapi.IndicesDeleteDataStreamRequest{
				Name: services.GetIndex(org_id, name),
			}.Do(ctx, client)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}

	return nil
}

func getAllOrgs(ctx context.Context) ([]string, error) {
	results := []string{"root"}
	indexes, err := services.ListIndexes(ctx)
	if err != nil {
		return nil, err
	}
	for _, index := range indexes {
		parts := strings.Split(index, "_results")
		if len(parts) == 2 {
			results = append(results, parts[0])
		}
	}
	return results, nil
}

func DeleteAllOrgs(ctx context.Context,
	config_obj *config_proto.Config, filter string) error {
	all_orgs, err := getAllOrgs(ctx)
	if err != nil {
		return err
	}
	orgs := append([]string{"root"}, all_orgs...)
	for _, org_id := range orgs {
		err := Delete(ctx, config_obj, org_id, filter)
		if err != nil {
			return err
		}
	}

	return nil
}

func InstallIndexTemplates(
	ctx context.Context,
	config_obj *config.Config) error {

	files, err := fs.ReadDir("templates")
	if err != nil {
		return err
	}
	logger := logging.GetLogger(config_obj.VeloConf(), &logging.FrontendComponent)

	for _, filename := range files {
		name := strings.Split(filename.Name(), ".")[0]

		// Check if the template is defined.
		err := services.DoesTemplateExist(ctx, name)
		if err == nil {
			continue
		}

		data, err := fs.ReadFile(path.Join("templates", filename.Name()))
		if err != nil {
			return err
		}

		logger.Info("Creating index template %v\n", name)
		err = services.PutTemplate(ctx, name, string(data), services.PrimaryOpenSearch)
		if err != nil {
			logger := logging.GetLogger(config_obj.VeloConf(), &logging.FrontendComponent)
			logger.Error("While creating index template %v: %v",
				name, err)
		}
		if config_obj.Cloud.SecondaryAddresses != nil {
			err = services.PutTemplate(ctx, name, string(data), services.SecondaryOpenSearch)
			if err != nil {
				logger := logging.GetLogger(config_obj.VeloConf(), &logging.FrontendComponent)
				logger.Error("While creating index template %v: %v",
					name, err)
			}
		}
	}

	return nil
}
