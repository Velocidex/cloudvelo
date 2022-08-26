package repository

import (
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/repository"
)

func LoadArtifactsFromConfig(
	repo_manager services.RepositoryManager,
	config_obj *config_proto.Config) error {

	return repository.LoadArtifactsFromConfig(repo_manager, config_obj)
}
