package ingestion

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/Velocidex/ordereddict"
	opensearchapi "github.com/opensearch-project/opensearch-go/v2/opensearchapi"
	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/cloudvelo/services"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/networking"
)

const (
	incrementHuntCompleted = `
{
  "script" : {
    "source": "ctx._source.completed ++ ;",
    "lang": "painless"
  }
}
`

	incrementHuntError = `
{
  "script" : {
    "source": "ctx._source.errors ++ ;",
    "lang": "painless"
  }
}
`

	incrementHuntScheduled = `
{
  "script" : {
    "source": "ctx._source.scheduled ++ ;",
    "lang": "painless"
  }
}
`
)

type Script struct {
	Type     string            `json:"type"`
	IdOrCode string            `json:"idOrCode"`
	Params   *ordereddict.Dict `json:"params"`
}

type UpdatePlan struct {
	Type    string  `json:"type"`
	Index   string  `json:"index"`
	Doc     string  `json:"doc"`
	DocId   string  `json:"docId"`
	Timeout uint64  `json:"timeout"`
	Script  *Script `json:"script"`
	BaseDoc string  `json:"baseDoc"`
	Params  string  `json:"params"`
}

// A Mock ingestor that delegates to a http server. Used for testing!!!

type HTTPIngestor struct {
	config_obj *config.Config

	incoming_url string

	client networking.HTTPClient
}

func normalize_index(id string) string {
	return strings.Replace(id, "root_", "", -1)
}

// Execute the update
func (self *HTTPIngestor) Execute(
	ctx context.Context, plan *UpdatePlan) error {
	switch strings.ToUpper(plan.Type) {
	case "OVERWRITE", "NEW":
		fmt.Printf("%v: %v\n", plan.Type, plan.DocId)

		client, err := services.GetElasticClient()
		if err != nil {
			return err
		}

		es_req := opensearchapi.IndexRequest{
			Index:      normalize_index(plan.Index),
			DocumentID: plan.DocId,
			Body:       bytes.NewReader([]byte(plan.Doc)),
			Refresh:    "true",
		}

		res, err := es_req.Do(ctx, client)
		if err != nil {
			return err
		}
		defer res.Body.Close()

		data, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}

		// All is well we dont need to parse the results
		if !res.IsError() {
			return nil
		}

		return makeElasticError(data)

	case "SCRIPT":
		if plan.Script == nil {
			return errors.New("Plan operation invalid")
		}

		if plan.Script.Type == "STORED" {
			query := ""
			switch plan.Script.IdOrCode {
			case "increment-hunt-started-script":
				query = incrementHuntScheduled

			case "increment-hunt-errored-script":
				query = incrementHuntError

			case "increment-hunt-completed-script":
				query = incrementHuntCompleted

			case "update-last-interrogate-script":
				params := plan.Script.Params
				if params != nil {
					last_interrogate, _ := params.GetString("last_interrogate")
					query = json.Format(updateClientInterrogate, last_interrogate)
				}
			}

			if query == "" {
				return errors.New("Plan unknown stored query")
			}

			return services.UpdateIndex(ctx, "",
				normalize_index(plan.Index), plan.DocId, query)
		}

	default:
	}

	return errors.New("Plan operation not supported")
}

func (self *HTTPIngestor) Process(
	ctx context.Context, message *crypto_proto.VeloMessage) error {
	serialized, err := json.Marshal(message)
	if err != nil {
		return err
	}
	serialized = append(serialized, '\n')
	serialized = json.AppendJsonlItem(serialized, "orgId",
		utils.NormalizedOrgId(self.config_obj.OrgId))
	serialized = json.AppendJsonlItem(serialized, "client_id", message.Source)

	fmt.Printf("Sending %v\n", string(serialized))

	req, err := http.NewRequestWithContext(ctx, "PUT",
		self.incoming_url, bytes.NewReader(serialized))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := self.client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	serialized_response, err := ioutil.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return fmt.Errorf("Unable to connect: %v: %v\n",
			resp.Status, string(serialized_response))
	}

	fmt.Printf("Plan: %v\n", string(serialized_response))

	plans := []*UpdatePlan{}
	err = json.Unmarshal(serialized_response, &plans)
	if err != nil {
		return err
	}

	for _, plan := range plans {
		err = self.Execute(ctx, plan)
		if err != nil {
			fmt.Printf("%v: %v\n", err, string(serialized_response))
		}
	}

	return nil
}

func NewHTTPIngestor(config_obj *config.Config) (*HTTPIngestor, error) {
	ctx := context.Background()
	http_client, err := networking.GetDefaultHTTPClient(
		ctx, config_obj.Client, nil, "", nil)
	if err != nil {
		return nil, err
	}

	return &HTTPIngestor{
		config_obj:   config_obj,
		client:       http_client,
		incoming_url: "http://localhost:8080/v1/velociraptor/ingest",
	}, nil
}

func makeElasticError(data []byte) error {
	response := ordereddict.NewDict()
	err := response.UnmarshalJSON(data)
	if err != nil {
		return fmt.Errorf("Elastic Error: %v", string(data))
	}

	err_type := utils.GetString(response, "error.type")
	err_reason := utils.GetString(response, "error.reason")
	if false && err_type != "" && err_reason != "" {
		return fmt.Errorf("Elastic Error: %v: %v", err_type, err_reason)
	}

	return fmt.Errorf("Elastic Error: %v", response)
}
