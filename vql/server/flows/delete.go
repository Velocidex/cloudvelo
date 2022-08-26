package flows

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/services"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type DeleteFlowArgs struct {
	ClientId   string `vfilter:"required,field=client_id"`
	FlowId     string `vfilter:"required,field=flow_id"`
	ReallyDoIt bool   `vfilter:"optional,field=really_do_it"`
}

type DeleteFlowPlugin struct{}

func (self *DeleteFlowPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &DeleteFlowArgs{}

		err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
		if err != nil {
			scope.Log("flow_delete: %v", err)
			return
		}

		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("flow_delete: %v", err)
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("Command can only run on the server")
			return
		}

		launcher, err := services.GetLauncher(config_obj)
		if err != nil {
			scope.Log("flow_delete: %v", err)
			return
		}

		results, err := launcher.DeleteFlow(ctx, config_obj,
			arg.ClientId, arg.FlowId, arg.ReallyDoIt)
		if err != nil {
			scope.Log("flow_delete: %v", err)
			return
		}

		for _, res := range results {
			select {
			case <-ctx.Done():
				return
			case output_chan <- res:
			}
		}

	}()

	return output_chan
}

func (self DeleteFlowPlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "flow_delete",
		Doc:     "Delete a flow with all its collections. ",
		ArgType: type_map.AddType(scope, &DeleteFlowArgs{}),
	}
}

func init() {
	vql_subsystem.OverridePlugin(&DeleteFlowPlugin{})
}
