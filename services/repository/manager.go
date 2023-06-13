package repository

/*
  How are repositories arranged?

  - The velociraptor repository is an in-memory repository which
    caches all the built in artifacts.

  - The cloud repository reads artifact definitions from the cloud
    backend. Each org in cloud velo has a cloud repository.

  Cloud repositories set parents for lookup delegation - if an
  artifact is accessed in a cloud org's repository and it is not
  found, then it gets delegated to the parent repository.

  top level: Velociraptor Repository - in memory contains built ins.

  Cloud Root Org Repository: A Cloud Repository with the top level set
     as the parent.

  Cloud Org: A Cloud Repository with the root org's repository set as
  the parent.


  // If we call LoadYaml() on an artifact and set the options as
  ArtifactIsBuiltIn, then the cloud repository will delegate to its
  parent. This ensures that Built in artifacts can be overridden and
  bubble up into the in memory repository for setting.

*/

import (
	"context"
	"errors"
	"io/fs"
	"regexp"
	"strings"
	"sync"
	"time"

	"www.velocidex.com/golang/cloudvelo/artifact_definitions"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/velociraptor/artifacts/assets"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/repository"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type RepositoryManager struct {
	mu                sync.Mutex
	global_repository services.Repository

	config_obj *config_proto.Config
	ctx        context.Context
}

// New Repository is called to create a new temporary repository
func (self *RepositoryManager) NewRepository() services.Repository {
	return &repository.Repository{
		Data: make(map[string]*artifacts_proto.Artifact),
	}
}

func (self *RepositoryManager) BuildScope(builder services.ScopeBuilder) vfilter.Scope {
	return (&repository.RepositoryManager{}).BuildScope(builder)
}

func (self *RepositoryManager) BuildScopeFromScratch(
	builder services.ScopeBuilder) vfilter.Scope {
	return (&repository.RepositoryManager{}).BuildScopeFromScratch(builder)
}

func (self *RepositoryManager) GetGlobalRepository(
	config_obj *config_proto.Config) (services.Repository, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	return self.global_repository, nil
}

func (self *RepositoryManager) SetGlobalRepositoryForTests(
	config_obj *config_proto.Config, repository services.Repository) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.global_repository = repository.(*Repository)
}

func (self *RepositoryManager) SetParent(
	parent_config_obj *config_proto.Config, parent services.Repository) {
	child_repo, ok := self.global_repository.(*Repository)
	if ok {
		child_repo.SetParent(parent, parent_config_obj)
	}
}

func NewRepositoryManager(
	ctx context.Context,
	wg *sync.WaitGroup,
	config_obj *config_proto.Config) (*RepositoryManager, error) {

	// The root org gets an in memory repository which contains all
	// the built-in set. It will be reflected in all the child orgs
	// automatically and is immutable.
	if utils.IsRootOrg(config_obj.OrgId) {
		root_global_repo := NewRepository(ctx, config_obj)

		// Create an in memory repository to hold all built in
		// artifacts.
		built_in_repository := &repository.Repository{
			Data: make(map[string]*artifacts_proto.Artifact),
		}

		// Create a cloud repository for the root org. Set the parent
		// of this repository to be the in memory repo.
		root_global_repo.SetParent(built_in_repository, config_obj)

		return &RepositoryManager{
			global_repository: root_global_repo,
			config_obj:        config_obj,
			ctx:               ctx,
		}, nil
	}

	// Sub orgs get a new elastic based repository.
	return &RepositoryManager{
		global_repository: NewRepository(ctx, config_obj),
		config_obj:        config_obj,
		ctx:               ctx,
	}, nil
}

func (self *RepositoryManager) SetArtifactFile(
	ctx context.Context,
	config_obj *config_proto.Config, principal string,
	definition, required_prefix string) (*artifacts_proto.Artifact, error) {

	// Use regexes to force the artifact into the correct prefix.
	if required_prefix != "" {
		definition = ensureArtifactPrefix(definition, required_prefix)
	}

	// Ensure that the artifact is correct by parsing it.
	tmp_repository := self.NewRepository()
	artifact_definition, err := tmp_repository.LoadYaml(definition,
		services.ArtifactOptions{
			ValidateArtifact:  true,
			ArtifactIsBuiltIn: false,
		})
	if err != nil {
		return nil, err
	}

	// This should only be triggered if something weird happened.
	if !strings.HasPrefix(artifact_definition.Name, required_prefix) {
		return nil, errors.New(
			"Modified or custom artifacts must start with '" +
				required_prefix + "'")
	}

	// Load the new artifact into the global repo so it is
	// immediately available.
	global_repository, err := self.GetGlobalRepository(config_obj)
	if err != nil {
		return nil, err
	}

	// Load the artifact into the currently running repository.
	return global_repository.LoadYaml(definition,
		services.ArtifactOptions{
			ValidateArtifact:  true,
			ArtifactIsBuiltIn: false,
		})
}

func (self *RepositoryManager) DeleteArtifactFile(
	ctx context.Context, config_obj *config_proto.Config,
	principal, name string) error {
	err := cvelo_services.DeleteDocument(self.ctx, self.config_obj.OrgId,
		"repository", name, cvelo_services.Sync)
	if err != nil {
		return err
	}

	global_repository, err := self.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	// Also remove from the global repository
	global_repository.Del(name)
	return nil
}

func (self *RepositoryManager) LoadBuiltInArtifacts(
	ctx context.Context,
	config_obj *config_proto.Config) error {

	options := services.ArtifactOptions{
		ValidateArtifact:  false,
		ArtifactIsBuiltIn: true,
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	now := time.Now()

	defer func() {
		logger.Info("Built in artifacts loaded in %v", time.Now().Sub(now))
	}()

	assets.Init()

	files, err := assets.WalkDirs("", false)
	if err != nil {
		return err
	}

	// Load all the built in artifacts into this in-memory repository
	grepository, err := self.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	count := 0

	for _, file := range files {
		if strings.HasPrefix(file, "artifacts/definitions") &&
			strings.HasSuffix(file, "yaml") {
			data, err := assets.ReadFile(file)
			if err != nil {
				logger.Info("Cant read asset %s: %v", file, err)
				if options.ValidateArtifact {
					return err
				}
				continue
			}

			// Load the built in artifacts as a built in. NOTE: Built in
			// artifacts can not be overwritten!
			_, err = grepository.LoadYaml(
				string(data), options)
			if err != nil {
				logger.Info("Can't parse asset %s: %s", file, err)
				if options.ValidateArtifact {
					return err
				}
				continue
			}
			count += 1
		}
	}

	return nil
}

func LoadOverridenArtifacts(
	config_obj *config_proto.Config,
	self services.RepositoryManager) error {

	options := services.ArtifactOptions{
		ValidateArtifact:  true,
		ArtifactIsBuiltIn: true,
	}

	global_repository, err := self.GetGlobalRepository(config_obj)
	if err != nil {
		return err
	}

	files, err := artifact_definitions.FS.ReadDir(".")
	if err != nil {
		return err
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), "yaml") {
			data, err := fs.ReadFile(artifact_definitions.FS, file.Name())
			if err != nil {
				continue
			}

			// Load the built in artifacts as built in. NOTE: Built in
			// artifacts can not be overwritten!
			_, err = global_repository.LoadYaml(string(data), options)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// On the client the repository manager is in memory only.
func NewClientRepositoryManager(
	ctx context.Context, wg *sync.WaitGroup,
	config_obj *config_proto.Config) (services.RepositoryManager, error) {
	return repository.NewRepositoryManager(ctx, wg, config_obj)
}

var (
	name_regex = regexp.MustCompile("(?sm)^(name: *)(.+)$")
)

func ensureArtifactPrefix(definition, prefix string) string {
	return utils.ReplaceAllStringSubmatchFunc(
		name_regex, definition,
		func(matches []string) string {
			if !strings.HasPrefix(matches[2], prefix) {
				return matches[1] + prefix + matches[2]
			}
			return matches[1] + matches[2]
		})
}
