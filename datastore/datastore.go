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
	/**
	This query has been added for the UX to return the download file associated with the given it.
	*/
	list_children_query = `
{"sort": {"timestamp": {"order": "desc"}},
 "size": 1,
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
	/**This query has been updated due to the transient index being changed to a datastream. Previously the index item was updated in place until the zip was available for download.

	As we cannot update a datastream item in place, we have to take the most recent item (by setting size to 1 and order by timestamp) with the vfs path as the key**/
	get_datastore_doc_query = `
{"sort": {"timestamp": {"order": "desc"}},
 "size": 1,
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

	hits, _, err := services.QueryElasticRaw(self.ctx, config_obj.OrgId,
		"transient", json.Format(get_datastore_doc_query, id))

	if err != nil {
		return err
	}

	record := &DatastoreRecord{
		Timestamp: utils.GetTime().Now().UnixNano(),
	}
	for _, hit := range hits {
		err = json.Unmarshal(hit, &record)
		if err != nil {

			return err
		}

		return protojson.Unmarshal([]byte(record.JSONData), message)
	}
	return nil
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
		ID:        services.MakeId(path.AsClientPath()),
		Type:      "Generic",
		VFSPath:   path.AsClientPath(),
		JSONData:  string(serialized),
		DocType:   "datastore",
		Timestamp: utils.GetTime().Now().UnixNano(),
	}
	return services.SetElasticIndex(self.ctx, config_obj.OrgId, "transient", "", record)
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
		self.ctx, config_obj.OrgId, "transient", json.Format(delete_datastore_doc_query, id), services.SyncDelete)
}

func (self ElasticDatastore) DeleteSubjectWithCompletion(
	config_obj *config_proto.Config,
	urn api.DSPathSpec, completion func()) error {
	id := services.MakeId(urn.AsClientPath())
	return services.DeleteDocumentByQuery(
		self.ctx, config_obj.OrgId, "transient", json.Format(delete_datastore_doc_query, id), services.AsyncDelete)
}

func (self ElasticDatastore) ListChildren(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]api.DSPathSpec, error) {

	dir := urn.AsDatastoreDirectory(config_obj)
	hits, _, err := services.QueryElasticRaw(self.ctx, config_obj.OrgId,
		"transient", json.Format(list_children_query, dir))
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
