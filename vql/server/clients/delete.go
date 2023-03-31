package clients

import (
	"context"

	"github.com/Velocidex/ordereddict"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/velociraptor/acls"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

const (
	all_client_items = `
{"query": {
    "bool": {
        "must": [
            {"match": {"client_id": %q}}
        ]}
}}
`
)

type DeleteClientArgs struct {
	ClientId   string `vfilter:"required,field=client_id"`
	ReallyDoIt bool   `vfilter:"optional,field=really_do_it"`
}

type DeleteClientPlugin struct{}

func (self DeleteClientPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &DeleteClientArgs{}

		err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
		if err != nil {
			scope.Log("client_delete: %s", err)
			return
		}

		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("client_delete: %s", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		// Delete flows and associated uploaded files
		launcher, err := services.GetLauncher(config_obj)
		if err != nil {
			scope.Log("client_delete:get_launcher: %s", err)
			return
		}

		flows, err := launcher.GetFlows(
			ctx, config_obj, arg.ClientId, true, nil, 0, 10000)
		if err != nil {
			scope.Log("client_delete:get_flows: %s", err)
			return
		}

		for _, f := range flows.Items {
			scope.Log("client_delete: deleting flow: %s", f.SessionId)
			_, err = launcher.Storage().DeleteFlow(
				ctx, config_obj, arg.ClientId, f.SessionId, arg.ReallyDoIt)
			if err != nil {
				scope.Log("client_delete:delete_flow: %s", err)
				return
			}

		}

		indexes := []string{"collections", "datastore", "results",
			"hunt_flows", "clients", "vfs", "tasks", "ping"}
		for _, index := range indexes {
			if arg.ReallyDoIt {
				err = removeClientDocs(ctx, config_obj, index, arg.ClientId)
				if err != nil {
					scope.Log("client_delete: %s : %s", index, err)
					return
				}
			}
		}

		// Send an event that the client was deleted.
		journal, err := services.GetJournal(config_obj)
		if err != nil {
			scope.Log("client_delete: %s", err)
			return
		}

		err = journal.PushRowsToArtifact(ctx, config_obj,
			[]*ordereddict.Dict{ordereddict.NewDict().
				Set("ClientId", arg.ClientId).
				Set("Principal", vql_subsystem.GetPrincipal(scope))},
			"Server.Internal.ClientDelete", "server", "")
		if err != nil {
			scope.Log("client_delete: %s", err)
			return
		}

		// Notify the client to force it to disconnect in case
		// it is already up.
		notifier, err := services.GetNotifier(config_obj)
		if err == nil {
			err = notifier.NotifyListener(
				ctx, config_obj, arg.ClientId, "DeleteClient")
			if err != nil {
				scope.Log("client_delete: %s", err)
				return
			}
		}
	}()

	return output_chan
}

func removeClientDocs(ctx context.Context,
	config_obj *config_proto.Config, index string, clientId string) error {

	return cvelo_services.DeleteByQuery(
		ctx, config_obj.OrgId, index,
		json.Format(all_client_items, clientId))
}

func (self DeleteClientPlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "client_delete",
		Doc:     "Delete all information related to a client. ",
		ArgType: type_map.AddType(scope, &DeleteClientArgs{}),
	}
}

func init() {
	vql_subsystem.OverridePlugin(&DeleteClientPlugin{})
}
