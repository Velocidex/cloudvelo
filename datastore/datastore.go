package datastore

import (
	"context"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
)

type ElasticDatastore struct {
	ctx context.Context
}

const (
	list_children_query = `
{
    "query": {
        "bool": {
            "must": [
                {
                    "prefix": {
                        "vfs_path": %q
                    }
                },
                {
                    "match": {
                        "doc_type": "datastore"
                    }
                }
            ]
        }
    }
}
`
	delete_datastore_doc_query = `
{
    "query": {
        "bool": {
            "must": [
                {
                    "prefix": {
                        "id": %q
                    }
                },
                {
                    "match": {
                        "doc_type": "datastore"
                    }
                }
            ]
        }
    }
}
`
)

func (self ElasticDatastore) GetSubject(
	config_obj *config_proto.Config,
	path api.DSPathSpec,
	message proto.Message) error {

	id := services.MakeId(path.AsClientPath())
	data, err := services.GetElasticRecord(
		self.ctx, config_obj.OrgId, "datastore", id)
	if err != nil {
		return err
	}

	record := &DatastoreRecord{
		Timestamp: utils.GetTime().Now().UnixNano(),
	}
	err = json.Unmarshal(data, &record)
	if err != nil {
		return err
	}

	return protojson.Unmarshal([]byte(record.JSONData), message)
}

func (self ElasticDatastore) SetSubject(
	config_obj *config_proto.Config,
	path api.DSPathSpec,
	message proto.Message) error {

	serialized, err := json.Marshal(message)
	if err != nil {
		return err
	}

	record := &DatastoreRecord{
		ID:       services.MakeId(path.AsClientPath()),
		Type:     "Generic",
		VFSPath:  path.AsClientPath(),
		JSONData: string(serialized),
		DocType:  "datastore",
	}
	return services.SetElasticIndex(
		self.ctx, config_obj.OrgId, "datastore", "", record)
}

func (self ElasticDatastore) SetSubjectWithCompletion(
	config_obj *config_proto.Config,
	urn api.DSPathSpec,
	message proto.Message,
	completion func()) error {
	return self.SetSubject(config_obj, urn, message)
}

func (self ElasticDatastore) DeleteSubject(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) error {

	id := services.MakeId(urn.AsClientPath())
	return services.DeleteDocumentByQuery(
		self.ctx, config_obj.OrgId, "results", json.Format(delete_datastore_doc_query, id), services.SyncDelete)
}

func (self ElasticDatastore) DeleteSubjectWithCompletion(
	config_obj *config_proto.Config,
	urn api.DSPathSpec, completion func()) error {
	id := services.MakeId(urn.AsClientPath())
	return services.DeleteDocumentByQuery(
		self.ctx, config_obj.OrgId, "results", json.Format(delete_datastore_doc_query, id), services.AsyncDelete)
}

func (self ElasticDatastore) ListChildren(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]api.DSPathSpec, error) {

	dir := urn.AsDatastoreDirectory(config_obj)
	hits, _, err := services.QueryElasticRaw(self.ctx, config_obj.OrgId,
		"results", json.Format(list_children_query, dir))
	if err != nil {
		return nil, err
	}

	results := make([]api.DSPathSpec, 0, len(hits))
	for _, hit := range hits {
		record := &DatastoreRecord{}
		err = json.Unmarshal(hit, &record)
		if err != nil {
			continue
		}

		components := utils.SplitComponents(record.VFSPath)
		path_spec := path_specs.DSFromGenericComponentList(components)
		results = append(results, path_spec)
	}

	return results, nil
}

func (self ElasticDatastore) Debug(config_obj *config_proto.Config) {}

func (self ElasticDatastore) Close() {}
