package labeler

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

var (
	label_regex = regexp.MustCompile(`^[a-zA-Z0-9_$-]+$`)
)

type Labeler struct {
	config_obj *config_proto.Config
}

// Get the last time any labeling operation modified the
// client's labels.
// TODO
func (self Labeler) LastLabelTimestamp(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) uint64 {
	return 0
}

// Is the label set for this client.
func (self Labeler) IsLabelSet(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, label string) bool {
	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return false
	}

	client_info, err := client_info_manager.Get(ctx, client_id)
	if err != nil {
		return false
	}

	return utils.InString(client_info.Labels, label)
}

// Set the label

const (
	// When we label a client, we zero out the last hunt and event
	// tables version to force the foreman to recalculate memberships.
	all_label_painless = `
if(!ctx._source.lower_labels.contains(params.lower_label)) {
  ctx._source.labels.add(params.label);
  ctx._source.lower_labels.add(params.lower_label);
  ctx._source.labels_timestamp = params.now;
  ctx._source.last_hunt_timestamp = 0;
  ctx._source.last_event_table_version = 0;
}
`

	label_update_query = `
{
    "script" : {
        "source": %q,
        "lang": "painless",
        "params": {
          "label": %q,
          "now": %q,
          "lower_label": %q
       }
    }
}
`
)

func (self Labeler) SetClientLabel(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, label string) error {

	label = strings.TrimSpace(label)

	if !label_regex.MatchString(label) {
		return errors.New("Invalid Label should use only A-Z and 0-9")
	}

	err := cvelo_services.UpdateIndex(ctx,
		self.config_obj.OrgId,
		"clients", client_id+"_labels",
		json.Format(label_update_query, all_label_painless, label,
			time.Now().UnixNano(), strings.ToLower(label)))
	if err == nil {
		return nil
	}

	if !strings.Contains(err.Error(), "document_missing_exception") {
		return err
	}

	return cvelo_services.SetElasticIndex(ctx,
		self.config_obj.OrgId, "clients", client_id+"_labels",
		api.ClientInfo{
			ClientId:    client_id,
			Labels:      []string{label},
			LowerLabels: []string{strings.ToLower(label)},
		})
}

const (
	remove_label_painless = `
for (int i=ctx._source.lower_labels.length-1; i>=0; i--) {
   if (ctx._source.lower_labels[i] == params.lower_label) {
      ctx._source.labels.remove(i);
      ctx._source.lower_labels.remove(i);
      ctx._source.labels_timestamp = params.now;
   }
}
`
)

// Remove the label from the client.
func (self Labeler) RemoveClientLabel(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id, label string) error {

	label = strings.TrimSpace(label)

	return cvelo_services.UpdateIndex(ctx, self.config_obj.OrgId,
		"clients", client_id+"_labels",
		json.Format(label_update_query, remove_label_painless, label,
			time.Now().UnixNano(), strings.ToLower(label)))
}

// Gets all the labels in a client.
func (self Labeler) GetClientLabels(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string) []string {
	client_info_manager, err := services.GetClientInfoManager(config_obj)
	if err != nil {
		return nil
	}

	client_info, err := client_info_manager.Get(ctx, client_id)
	if err != nil {
		return nil
	}

	return client_info.Labels
}

func NewLabelerService(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.Labeler, error) {

	labeler := &Labeler{
		config_obj: config_obj,
	}

	return labeler, nil
}
