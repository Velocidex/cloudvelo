package schema

import (
	"context"
	"embed"
	_ "embed"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/opensearch-project/opensearch-go/opensearchapi"
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

//go:embed mappings/*.json
var fs embed.FS

// Just remove the index but do not create one
func Delete(ctx context.Context,
	config_obj *config_proto.Config, org_id, filter string) error {

	client, err := services.GetElasticClient()
	if err != nil {
		return err
	}

	files, err := fs.ReadDir("mappings")
	if err != nil {
		return err
	}
	for _, filename := range files {
		index_name := strings.Split(filename.Name(), ".")[0]

		if filter != "" && !strings.HasPrefix(index_name, filter) {
			continue
		}

		full_index_name := services.GetIndex(org_id, index_name)
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Info("Deleting index %v", full_index_name)

		// Delete previously created index.
		_, err := opensearchapi.IndicesDeleteRequest{
			Index: []string{full_index_name},
		}.Do(ctx, client)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
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

// Initialize and ensure elastic indexes exist.
func Initialize(ctx context.Context,
	config_obj *config_proto.Config,
	org_id, filter string, reset bool) error {

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	client, err := services.GetElasticClient()
	if err != nil {
		return err
	}

	files, err := fs.ReadDir("mappings")
	if err != nil {
		return err
	}

	wg := &sync.WaitGroup{}

	for _, filename := range files {
		wg.Add(1)
		go func(filename string) {
			defer wg.Done()

			data, err := fs.ReadFile(path.Join("mappings", filename))
			if err != nil {
				return
			}

			index_name := strings.Split(filename, ".")[0]

			if filter != "" && !strings.HasPrefix(index_name, filter) {
				return
			}

			full_index_name := services.GetIndex(org_id, index_name)

			response, err := client.Indices.Exists([]string{full_index_name})
			if err != nil {
				logger.Error("Initialize: %v", err)
				return
			}

			if response.StatusCode != 404 && reset {
				// Delete previously created index.
				res, err := opensearchapi.IndicesDeleteRequest{
					Index: []string{full_index_name},
				}.Do(ctx, client)
				if err != nil {
					logger.Error("Initialize: %v", err)
					return
				}
				fmt.Printf("Deleted index %v: %v\n", full_index_name, res)
			}

			response, err = opensearchapi.IndicesCreateRequest{
				Index: full_index_name,
				Body:  strings.NewReader(string(data)),
			}.Do(ctx, client)
			if err != nil {
				logger.Error("Initialize: %v", err)
				return
			}
			response_data, _ := ioutil.ReadAll(response.Body)
			if !strings.Contains(string(response_data), "resource_already_exists_exception") {
				fmt.Printf("Created index: %v\n", string(response_data))
			}
		}(filename.Name())
	}

	wg.Wait()
	return nil
}
