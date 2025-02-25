package notebook

import (
	"context"

	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	query = `
{
  "sort": [
  {
    "timestamp": {"order": "desc"}
  }],
  "query": {
    "bool": {
      "must": [
        {
          "bool": {
            "should": [
              {"match": {"creator" : %q}},
              {"match": {"shared": %q}},
              {"match": {"public": true}},
              {"match": {"doc_type" : "notebooks"}}
           ]}
        },
        {"match": {"type": "User"}}
      ]}
  },
  "size": %q,
  "from": %q
}
`
)

// Query the elastic backend to get all notebooks
func (self *NotebookManager) GetSharedNotebooks(
	ctx context.Context, user string) (api.FSPathSpec, error) {
	return nil, utils.NotImplementedError
}
