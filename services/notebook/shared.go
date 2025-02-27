package notebook

import (
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	query_for_shared_notebooks = `
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
              {"match": {"public": true}}
           ]}
        },
        {"match": {"doc_type" : "notebooks"}},
        {"match": {"type": "User"}}
      ]}
  },
  "size": %q,
  "from": %q
}
`

	// Very rarely used
	query_for_all_notebooks = `
{
  "sort": [
  {
    "timestamp": {"order": "desc"}
  }],
  "query": {
    "bool": {
      "must": [
        {"match": {"doc_type" : "notebooks"}},
        {"match": {"type": "User"}}
      ]}
  },
  "size": %q,
  "from": %q
}
`
)

func checkNotebookAccess(notebook *api_proto.NotebookMetadata, user string) bool {
	if notebook.Public {
		return true
	}

	return notebook.Creator == user || utils.InString(notebook.Collaborators, user)
}
