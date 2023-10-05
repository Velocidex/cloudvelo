package notebook

import (
	"context"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/json"
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
              {"match": {"public": true}}
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
	ctx context.Context, user string, offset, count uint64) (
	[]*api_proto.NotebookMetadata, error) {
	hits, _, err := cvelo_services.QueryElasticRaw(
		ctx, self.config_obj.OrgId, "notebooks",
		json.Format(query, user, user, count, offset))
	if err != nil {
		return nil, err
	}

	result := []*api_proto.NotebookMetadata{}
	for _, hit := range hits {
		entry := &NotebookRecord{}
		err = json.Unmarshal(hit, entry)
		if err != nil {
			continue
		}

		item := &api_proto.NotebookMetadata{}
		err = json.Unmarshal([]byte(entry.Notebook), item)
		if err != nil {
			continue
		}

		if !item.Hidden {
			result = append(result, item)
		}

	}
	return result, nil
}
