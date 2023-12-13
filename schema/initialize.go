package schema

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"
	"github.com/pkg/errors"
	"os"
	"strings"
	"www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

const (
	RESET_INDEX        = true
	DO_NOT_RESET_INDEX = false

	NO_FILTER = ""
)

// Just remove the index but do not create one
func Delete(ctx context.Context,
	config_obj *config_proto.Config, org_id, filter string) error {

	client, err := services.GetElasticClient()
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
	if !reset {
		return nil
	}
	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)

	client, err := services.GetElasticClient()
	if err != nil {
		return err
	}

	// Delete previously created index.
	res, err := opensearchapi.IndicesDeleteRequest{
		Index: []string{org_id + "*"},
	}.Do(ctx, client)
	if err != nil {
		logger.Error("Initialize: %v", err)
		return nil
	}
	fmt.Printf("Deleted index %v: %v\n", org_id+"*", res)
	return nil
}
