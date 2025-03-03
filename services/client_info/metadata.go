package client_info

import (
	"context"

	"github.com/Velocidex/ordereddict"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

// We do not support custom metadata
type MetadataEntry struct {
	ClientId string `json:"client_id"`
	Metadata string `json:"metadata"`
}

func (self ClientInfoManager) GetMetadata(ctx context.Context,
	client_id string) (*ordereddict.Dict, error) {

	result := ordereddict.NewDict()
	message, err := cvelo_services.GetElasticRecord(ctx, self.config_obj.OrgId,
		"persisted", client_id+"_metadata")
	if err != nil || len(message) == 0 {
		// Metadata is not there return an empty one
		return result, nil
	}

	entry := &MetadataEntry{}
	err = json.Unmarshal(message, entry)
	if err != nil {
		return result, nil
	}

	err = result.UnmarshalJSON([]byte(entry.Metadata))
	if err != nil {
		return result, nil
	}

	return result, nil
}

func valueIsRemove(value vfilter.Any) bool {
	if utils.IsNil(value) {
		return true
	}

	value_str, ok := value.(string)
	if ok && value_str == "" {
		return true
	}

	return false
}

func (self ClientInfoManager) SetMetadata(ctx context.Context,
	client_id string, metadata *ordereddict.Dict, principal string) error {

	// Merge the old values with the new values
	old_metadata, err := self.GetMetadata(ctx, client_id)
	if err != nil {
		old_metadata = ordereddict.NewDict()
	}

	for _, k := range old_metadata.Keys() {
		new_value, pres := metadata.Get(k)
		if pres {
			// If the new metadata dict has an empty field then it
			// means to remove it from the old metadata.
			if valueIsRemove(new_value) {
				old_metadata.Delete(k)
			} else {
				old_metadata.Update(k, new_value)
			}
		}
	}

	// Add any missing fields
	for _, k := range metadata.Keys() {
		_, pres := old_metadata.Get(k)
		if !pres {
			value, _ := metadata.Get(k)
			old_metadata.Set(k, value)
		}
	}

	serialized, err := json.Marshal(old_metadata)
	if err != nil {
		return err
	}

	return cvelo_services.SetElasticIndex(ctx, self.config_obj.OrgId,
		"persisted", client_id+"_metadata", &MetadataEntry{
			ClientId: client_id,
			Metadata: string(serialized),
		})
}
