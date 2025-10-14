package hunts

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/acls"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type AddToHuntFunctionArg struct {
	ClientId string `vfilter:"required,field=client_id"`
	HuntId   string `vfilter:"required,field=hunt_id"`
	FlowId   string `vfilter:"optional,field=flow_id,doc=If a flow id is specified we do not create a new flow, but instead add this flow_id to the hunt."`
	Relaunch bool   `vfilter:"optional,field=relaunch,doc=If specified we relaunch the hunt on this client again."`
}

type AddToHuntFunction struct{}

func (self *AddToHuntFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	err := vql_subsystem.CheckAccess(scope, acls.START_HUNT)
	if err != nil {
		scope.Log("hunt_add: %v", err)
		return vfilter.Null{}
	}

	arg := &AddToHuntFunctionArg{}
	err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("hunt_add: %v", err)
		return vfilter.Null{}
	}

	err = services.RequireFrontend()
	if err != nil {
		scope.Log("hunt_add: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("hunt_add: Command can only run on the server")
		return vfilter.Null{}
	}

	hunt_dispatcher, err := services.GetHuntDispatcher(config_obj)
	if err != nil {
		return vfilter.Null{}
	}

	// Relaunch the collection.
	if arg.Relaunch || arg.FlowId == "" {
		hunt_obj, pres := hunt_dispatcher.GetHunt(ctx, arg.HuntId)
		if !pres || hunt_obj == nil ||
			hunt_obj.StartRequest == nil ||
			hunt_obj.StartRequest.CompiledCollectorArgs == nil {
			scope.Log("hunt_add: Hunt id not found %v", arg.HuntId)
			return vfilter.Null{}
		}

		launcher, err := services.GetLauncher(config_obj)
		if err != nil {
			return vfilter.Null{}
		}

		// Launch the collection against a client. We assume it is
		// already compiled because hunts always pre-compile their
		// artifacts.
		request := proto.Clone(hunt_obj.StartRequest).(*flows_proto.ArtifactCollectorArgs)
		request.ClientId = arg.ClientId

		// Generate a new flow id for each request
		request.FlowId = ""

		arg.FlowId, err = launcher.WriteArtifactCollectionRecord(
			ctx, config_obj, request, hunt_obj.StartRequest.CompiledCollectorArgs,
			func(task *crypto_proto.VeloMessage) {
				client_manager, err := services.GetClientInfoManager(config_obj)
				if err != nil {
					return
				}

				// Queue and notify the client about the new tasks
				_ = client_manager.QueueMessageForClient(
					ctx, arg.ClientId, task,
					services.NOTIFY_CLIENT, utils.BackgroundWriter)
			})
		if err != nil {
			scope.Log("hunt_add: %v", err)
			return vfilter.Null{}
		}

		err = hunt_dispatcher.MutateHunt(ctx, config_obj,
			&api_proto.HuntMutation{
				HuntId: arg.HuntId,
				Assignment: &api_proto.FlowAssignment{
					ClientId: arg.ClientId,
					FlowId:   arg.FlowId,
				}})
		if err != nil {
			scope.Log("hunt_add: %v", err)
			return vfilter.Null{}
		}

		return arg.FlowId
	}

	// Just assign the existing flow.
	err = hunt_dispatcher.MutateHunt(ctx, config_obj,
		&api_proto.HuntMutation{
			HuntId: arg.HuntId,
			Assignment: &api_proto.FlowAssignment{
				ClientId: arg.ClientId,
				FlowId:   arg.FlowId,
			}})
	if err != nil {
		scope.Log("hunt_add: %v", err)
		return vfilter.Null{}
	}

	return arg.FlowId
}

func (self AddToHuntFunction) Info(scope vfilter.Scope,
	type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:     "hunt_add",
		Doc:      "Assign a client to a hunt.",
		ArgType:  type_map.AddType(scope, &AddToHuntFunctionArg{}),
		Metadata: vql.VQLMetadata().Permissions(acls.START_HUNT).Build(),
	}
}

func init() {
	vql_subsystem.OverrideFunction(&AddToHuntFunction{})
}
