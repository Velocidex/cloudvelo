package schema

import (
	"context"
	"embed"
	_ "embed"
	"fmt"
	"io/ioutil"
	"path"
	"strings"

	"github.com/opensearch-project/opensearch-go/opensearchapi"
	"www.velocidex.com/golang/cloudvelo/services"
)

//go:embed mappings/*.json
var fs embed.FS

// Initialize and ensure elastic indexes exist.
func Initialize(ctx context.Context,
	org_id, filter string,
	reset bool) error {
	client, err := services.GetElasticClient()
	if err != nil {
		return err
	}

	files, err := fs.ReadDir("mappings")
	if err != nil {
		return err
	}
	for _, filename := range files {
		data, err := fs.ReadFile(path.Join("mappings", filename.Name()))
		if err != nil {
			continue
		}

		index_name := strings.Split(filename.Name(), ".")[0]

		if filter != "" && !strings.HasPrefix(index_name, filter) {
			continue
		}

		full_index_name := services.GetIndex(org_id, index_name)

		response, err := client.Indices.Exists([]string{full_index_name})
		if err != nil {
			return err
		}

		if response.StatusCode != 404 && reset {
			// Delete previously created index.
			res, err := opensearchapi.IndicesDeleteRequest{
				Index: []string{full_index_name},
			}.Do(ctx, client)
			if err != nil {
				return err
			}
			fmt.Printf("Deleted index: %v\n", res)
		}

		response, err = opensearchapi.IndicesCreateRequest{
			Index: full_index_name,
			Body:  strings.NewReader(string(data)),
		}.Do(ctx, client)
		if err != nil {
			return err
		}
		response_data, _ := ioutil.ReadAll(response.Body)
		if !strings.Contains(string(response_data), "resource_already_exists_exception") {
			fmt.Printf("Created index: %v\n", string(response_data))
		}
	}

	return nil
}
