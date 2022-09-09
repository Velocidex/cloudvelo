package datastore

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/file_store/api"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
)

type ElasticDatastore struct{}

func (self ElasticDatastore) GetSubject(
	config_obj *config_proto.Config,
	path api.DSPathSpec,
	message proto.Message) error {

	id := utils.JoinComponents(path.Components(), "_")
	if strings.Contains(id, "vfs") {
		utils.DlvBreak()
	}

	data, err := services.GetElasticRecord(
		context.Background(), config_obj.OrgId, "datastore", id)
	if err != nil {
		return err
	}

	record := &DatastoreRecord{}
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

	record, err := DSPathSpecToRecord(path)
	if err != nil {
		return err
	}

	serialized, err := json.Marshal(message)
	if err != nil {
		return err
	}

	record.JSONData = string(serialized)
	return services.SetElasticIndex(config_obj.OrgId, "datastore", "", record)
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
	return errors.New("ElasticDatastore.DeleteSubject Not implemented")
}

func (self ElasticDatastore) DeleteSubjectWithCompletion(
	config_obj *config_proto.Config,
	urn api.DSPathSpec, completion func()) error {
	return errors.New("ElasticDatastore.DeleteSubjectWithCompletion Not implemented")
}

func (self ElasticDatastore) ListChildren(
	config_obj *config_proto.Config,
	urn api.DSPathSpec) ([]api.DSPathSpec, error) {
	return nil, errors.New("ElasticDatastore.ListChildren Not implemented")
}

func (self ElasticDatastore) Debug(config_obj *config_proto.Config) {}

func (self ElasticDatastore) Close() {}
