package hunt_dispatcher

import (
	"context"
	"errors"
	"path"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/hunt_dispatcher"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
)

func (self HuntDispatcher) CreateHunt(
	ctx context.Context,
	config_obj *config_proto.Config,
	acl_manager vql_subsystem.ACLManager,
	hunt *api_proto.Hunt) (string, error) {

	if hunt.StartRequest == nil || hunt.StartRequest.Artifacts == nil {
		return "", errors.New("No artifacts to collect.")
	}

	if hunt.Expires == 0 {
		default_expiry := config_obj.Defaults.HuntExpiryHours
		if default_expiry == 0 {
			default_expiry = 7 * 24
		}
		hunt.Expires = uint64(time.Now().Add(
			time.Duration(default_expiry)*time.Hour).
			UTC().UnixNano() / 1000)
	}

	if hunt.Expires < hunt.CreateTime {
		return "", errors.New("Hunt expiry is in the past!")
	}

	// Set the artifacts information in the hunt object itself.
	hunt.Artifacts = hunt.StartRequest.Artifacts
	hunt.ArtifactSources = []string{}
	for _, artifact := range hunt.StartRequest.Artifacts {
		for _, source := range hunt_dispatcher.GetArtifactSources(
			config_obj, artifact) {
			hunt.ArtifactSources = append(
				hunt.ArtifactSources, path.Join(artifact, source))
		}
	}

	hunt.CreateTime = uint64(time.Now().UTC().UnixNano() / 1000)

	// We allow our caller to determine if hunts are created in
	// the running state or the paused state.
	if hunt.State == api_proto.Hunt_UNSET {
		hunt.State = api_proto.Hunt_PAUSED

		// IF we are creating the hunt in the running state
		// set it started.
	} else if hunt.State == api_proto.Hunt_RUNNING {
		hunt.StartTime = hunt.CreateTime
	}

	// First compile the request to make sure it is valid.
	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		return "", err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return "", err
	}

	laucher_manager, err := services.GetLauncher(config_obj)
	if err != nil {
		return "", err
	}

	compiled, err := laucher_manager.CompileCollectorArgs(
		ctx, config_obj, acl_managers.NullACLManager{}, repository,
		services.CompilerOptions{
			ObfuscateNames: true,
		}, hunt.StartRequest)
	if err != nil {
		return "", err
	}

	if len(compiled) == 0 {
		return "", errors.New("No compiled requests!")
	}

	// Add a special hunt message to trigger stats update by the
	// client ingestor.
	compiled = append([]*actions_proto.VQLCollectorArgs{
		{
			QueryId:      -1,
			TotalQueries: 1 + int64(len(compiled)),
			Query: []*actions_proto.VQLRequest{
				{VQL: `SELECT log(message="Starting Hunt") FROM scope()`},
			},
		},
	}, compiled...)

	hunt_id := hunt_dispatcher.GetNewHuntId()
	hunt.HuntId = hunt_id
	hunt.StartRequest.CompiledCollectorArgs = compiled

	serialized, err := protojson.Marshal(hunt)
	if err != nil {
		return "", err
	}

	err = cvelo_services.SetElasticIndex(ctx,
		self.config_obj.OrgId,
		"hunts", hunt_id, &HuntEntry{
			HuntId:    hunt_id,
			Timestamp: time.Now().Unix(),
			Hunt:      string(serialized),
		})

	// The actual hunt scheduling is done by the foreman.
	/*
		if hunt.State == api_proto.Hunt_RUNNING {
			scheduleClientsForHunt(ctx, config_obj, hunt)
		}
	*/
	return hunt_id, nil
}

func XXXscheduleClientsForHunt(
	ctx context.Context,
	config_obj *config_proto.Config,
	hunt *api_proto.Hunt) error {

	if hunt.StartRequest == nil ||
		hunt.StartRequest.CompiledCollectorArgs == nil {
		return errors.New("Invalid hunt request!")
	}

	// Create a collection for each known client.
	indexer, err := services.GetIndexer(config_obj)
	if err != nil {
		return err
	}

	// TODO: I am not sure how performant this will be?
	scope := vql_subsystem.MakeScope()

	// TODO: filter by labels.
	client_chan, err := indexer.SearchClientsChan(ctx, scope,
		config_obj, "all", "")
	if err != nil {
		return err
	}

	laucher_manager, err := services.GetLauncher(config_obj)
	if err != nil {
		return err
	}

	for record := range client_chan {
		request := proto.Clone(
			hunt.StartRequest).(*flows_proto.ArtifactCollectorArgs)

		request.Creator = hunt.HuntId

		// Schedule the collection on each client
		request.ClientId = record.ClientId
		_, err := laucher_manager.ScheduleArtifactCollectionFromCollectorArgs(
			ctx, config_obj, request,
			hunt.StartRequest.CompiledCollectorArgs, utils.SyncCompleter)
		if err != nil {
			return err
		}
	}

	return nil
}
