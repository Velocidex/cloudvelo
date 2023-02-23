package tools

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/services/server_artifacts"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type GetApprovedToolArgs struct {
	HuntId     string `vfilter:"required,field=hunt_id"`
	ReallyDoIt bool   `vfilter:"optional,field=really_do_it"`
}

type GetApprovedToolPlugin struct{}

func (self GetApprovedToolPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &GetApprovedToolArgs{}

		err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
		if err != nil {
			scope.Log("get_approved_tool: %s", err)
			return
		}

		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("get_approved_tool: %s", err)
			return
		}

		// Get the elastic config.
		server_artifacts_manager, err := services.GetServerArtifactService()
		if err != nil {
			scope.Log("get_approved_tool: %s", err)
			return
		}

		cloud_server_artifacts_manager, ok := server_artifacts_manager.(*server_artifacts.ServerArtifactsRunner)
		if !ok {
			scope.Log("get_approved_tool: Command can only run on a cloud server")
			return
		}

		json.Dump(cloud_server_artifacts_manager)

	}()

	return output_chan
}

func (self GetApprovedToolPlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "get_approved_tool",
		Doc:     "Get a tool as approved by the admin. ",
		ArgType: type_map.AddType(scope, &GetApprovedToolArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&GetApprovedToolPlugin{})
}
