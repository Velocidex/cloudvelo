package datastore

import (
	"context"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
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
	get_download_id = `
{"sort": {"timestamp": {"order": "desc"}},
 "size": 1,
    "query": {
        "bool": {
            "must": [
                {
                    "match": {
                        "vfs_path": %q
                    }
                },
                {
                    "match": {
                        "doc_type": "download"
                    }
                }
            ]
        }
    }
}
`
	get_datastore_doc_query = `
{"sort": {"timestamp": {"order": "desc"}},
 "size": 1,
    "query": {
        "bool": {
            "must": [
                {
                    "match": {
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

type hash struct {
	Hash string `json:"hash"`
}

type downloadid struct {
	Id        string `json:"id"`
	Timestamp int64  `json:"timestamp"`
	VFSPath   string `json:"vfs_path"`
	DocType   string `json:"doc_type"`
}

func (self ElasticDatastore) GetSubject(
	config_obj *config_proto.Config,
	path api.DSPathSpec,
	message proto.Message) error {

	id := cvelo_services.MakeId(path.AsClientPath())

	hits, _, err := cvelo_services.QueryElasticRaw(self.ctx, config_obj.OrgId,
		"transient", json.Format(get_datastore_doc_query, id))

	record := &DatastoreRecord{
		Timestamp: utils.GetTime().Now().UnixNano(),
	}
	for _, hit := range hits {
		err = json.Unmarshal(hit, &record)
		if err != nil {
			continue
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
		ID:        cvelo_services.MakeId(path.AsClientPath()),
		Type:      "Generic",
		VFSPath:   path.AsClientPath(),
		JSONData:  string(serialized),
		DocType:   "datastore",
		Timestamp: utils.GetTime().Now().UnixNano(),
	}

	return cvelo_services.SetElasticIndexAsync(config_obj.OrgId, "transient", "", cvelo_services.BulkUpdateCreate, record)

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

	id := cvelo_services.MakeId(urn.AsClientPath())
	return cvelo_services.DeleteDocumentByQuery(
		self.ctx, config_obj.OrgId, "transient", json.Format(delete_datastore_doc_query, id), cvelo_services.SyncDelete)
}

func (self ElasticDatastore) DeleteSubjectWithCompletion(
	config_obj *config_proto.Config,
	urn api.DSPathSpec, completion func()) error {
	id := cvelo_services.MakeId(urn.AsClientPath())
	return cvelo_services.DeleteDocumentByQuery(
		self.ctx, config_obj.OrgId, "transient", json.Format(delete_datastore_doc_query, id), cvelo_services.AsyncDelete)
}

func (self ElasticDatastore) ListChildren(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]api.DSPathSpec, error) {

	dir := urn.AsDatastoreDirectory(config_obj)
	hits, _, err := cvelo_services.QueryElasticRaw(self.ctx, config_obj.OrgId,
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
