package hunt_dispatcher

/*
func (self HuntDispatcher) XXXCreateHunt(
	ctx context.Context,
	config_obj *config_proto.Config,
	acl_manager vql_subsystem.ACLManager,
	hunt *api_proto.Hunt) (*api_proto.Hunt, error) {

	if hunt.StartRequest == nil || hunt.StartRequest.Artifacts == nil {
		return nil, errors.New("No artifacts to collect.")
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
		return nil, errors.New("Hunt expiry is in the past!")
	}

	// Set the artifacts information in the hunt object itself.
	hunt.Artifacts = hunt.StartRequest.Artifacts
	hunt.ArtifactSources = []string{}
	for _, artifact := range hunt.StartRequest.Artifacts {
		for _, source := range hunt_dispatcher.GetArtifactSources(
			ctx, config_obj, artifact) {
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
		return nil, err
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	laucher_manager, err := services.GetLauncher(config_obj)
	if err != nil {
		return nil, err
	}

	compiled, err := laucher_manager.CompileCollectorArgs(
		ctx, config_obj, acl_managers.NullACLManager{}, repository,
		services.CompilerOptions{
			ObfuscateNames: true,
		}, hunt.StartRequest)
	if err != nil {
		return nil, err
	}

	if len(compiled) == 0 {
		return nil, errors.New("No compiled requests!")
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
	hunt.StartRequest.FlowId = utils.CreateFlowIdFromHuntId(hunt.HuntId)
	hunt.StartRequest.CompiledCollectorArgs = compiled
	hunt.StartRequest.Creator = hunt.Creator

	serialized, err := protojson.Marshal(hunt)
	if err != nil {
		return nil, err
	}

	err = cvelo_services.SetElasticIndex(ctx,
		self.config_obj.OrgId,
		"persisted", hunt_id,
		&HuntEntry{
			HuntId:    hunt_id,
			Expires:   hunt.Expires,
			Timestamp: time.Now().Unix(),
			Hunt:      string(serialized),
			State:     hunt.State.String(),
			DocType:   "hunts",
		})

	// The actual hunt scheduling is done by the foreman.
	return hunt, nil
}
*/
