package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Velocidex/ttlcache/v2"
	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/repository"
)

// This repository is stateless - we store all the definitions and
// compiled data in the elastic index. We do have a time based LRU to
// limit the lifetime of cached data. Eventually consistent view of
// the database.
type Repository struct {
	mu sync.Mutex

	config_obj *config_proto.Config

	lru *ttlcache.Cache

	parent            services.Repository
	parent_config_obj *config_proto.Config
}

const (
	// We only need the names of the artifacts for listing.
	allNamesQuery = `
{
    "query" : {
        "match_all" : {}
    }
}
`
)

func (self *Repository) GetArtifactType(
	config_obj *config_proto.Config, artifact_name string) (string, error) {
	artifact, pres := self.Get(config_obj, artifact_name)
	if !pres {
		return "", fmt.Errorf("Artifact %s not known", artifact_name)
	}

	return artifact.Type, nil
}

func (self *Repository) GetSource(
	config_obj *config_proto.Config, name string) (*artifacts_proto.ArtifactSource, bool) {
	artifact_name, source_name := paths.SplitFullSourceName(name)
	artifact, pres := self.Get(config_obj, artifact_name)
	if !pres {
		return nil, false
	}
	for _, source := range artifact.Sources {
		if source.Name == source_name {
			return source, true
		}
	}

	return nil, false
}

func (self *Repository) SetParent(
	parent services.Repository, parent_config_obj *config_proto.Config) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.parent = parent
	self.parent_config_obj = parent_config_obj
}

func (self *Repository) List(
	ctx context.Context,
	config_obj *config_proto.Config) ([]string, error) {

	results := []string{}

	hits, err := cvelo_services.QueryChan(ctx, config_obj, 1000,
		self.config_obj.OrgId, "repository", allNamesQuery, "name")
	if err != nil {
		return nil, err
	}

	for hit := range hits {
		record := &api.RepositoryEntry{}
		err = json.Unmarshal(hit, record)
		if err != nil {
			continue
		}

		artifact := &artifacts_proto.Artifact{}
		err = json.Unmarshal([]byte(record.Definition), artifact)
		if err != nil {
			continue
		}

		// Refresh the TTL since this data is more recent.
		self.lru.Set(artifact.Name, artifact)
		results = append(results, artifact.Name)
	}

	// Merge with the parent's listing
	if self.parent != nil {
		names, err := self.parent.List(ctx, self.parent_config_obj)
		if err == nil {
			results = append(results, names...)
		}
	}

	sort.Strings(results)

	return results, nil

}

func (self *Repository) Copy() services.Repository {
	return self
}

func (self *Repository) LoadDirectory(
	config_obj *config_proto.Config, dirname string,
	override_builtins bool) (int, error) {
	return 0, errors.New("Not implemented")
}

func (self *Repository) LoadYaml(data string, validate, built_in bool) (
	*artifacts_proto.Artifact, error) {

	// Load into a dummy repo to check for syntax errors etc.
	dummy_repository := repository.Repository{Data: make(map[string]*artifacts_proto.Artifact)}
	artifact, err := dummy_repository.LoadYaml(
		data, validate, built_in)
	if err != nil {
		return nil, err
	}

	return artifact, self.saveArtifact(artifact)
}

func (self *Repository) LoadProto(
	artifact *artifacts_proto.Artifact, validate bool) (
	*artifacts_proto.Artifact, error) {
	dummy_repository := repository.Repository{Data: make(map[string]*artifacts_proto.Artifact)}
	artifact, err := dummy_repository.LoadProto(artifact, true)
	if err != nil {
		return nil, err
	}

	return artifact, self.saveArtifact(artifact)
}

func (self *Repository) Del(name string) {
	self.lru.Remove(name)
	cvelo_services.DeleteDocument(self.config_obj.OrgId,
		"repository", name, cvelo_services.SyncDelete)
}

func (self *Repository) Get(
	config_obj *config_proto.Config, name string) (*artifacts_proto.Artifact, bool) {
	// Strip off any source specification
	name, _ = paths.SplitFullSourceName(name)

	// Try to get it from the LRU
	artifact_any, err := self.lru.Get(name)
	if err == nil {
		artifact, ok := artifact_any.(*artifacts_proto.Artifact)
		if ok {
			return artifact, true
		}
	}

	// Try to get it from the parent first because the root org is
	// usually kept in memory.
	if self.parent != nil {
		artifact, pres := self.parent.Get(self.parent_config_obj, name)
		if pres {
			return artifact, true
		}
	}

	// Failing this we try to read from the backend.
	return self.getFromBackend(config_obj, name)
}

func (self *Repository) getFromBackend(
	config_obj *config_proto.Config, name string) (*artifacts_proto.Artifact, bool) {

	// Nope - get it from the backend.
	ctx := context.Background()
	serialized, err := cvelo_services.GetElasticRecord(ctx,
		self.config_obj.OrgId, "repository", name)
	if err != nil {
		return nil, false
	}

	record := &api.RepositoryEntry{}
	err = json.Unmarshal(serialized, record)
	if err != nil {
		return nil, false
	}

	artifact := &artifacts_proto.Artifact{}
	err = json.Unmarshal([]byte(record.Definition), artifact)
	if err != nil {
		return nil, false
	}

	// Remember it for next time.
	self.lru.Set(artifact.Name, artifact)

	return artifact, true
}

func (self *Repository) saveArtifact(artifact *artifacts_proto.Artifact) error {
	name := artifact.Name
	if self.parent != nil {
		_, pres := self.parent.Get(self.parent_config_obj, name)
		if pres {
			return fmt.Errorf(
				"Can not override an artifact from the root org: %s", name)
		}
	}

	// Set the artifact in the elastic index.
	err := cvelo_services.SetElasticIndex(
		self.config_obj.OrgId,
		"repository", artifact.Name,
		&api.RepositoryEntry{
			Name:       artifact.Name,
			Definition: json.MustMarshalString(artifact),
		})

	// Set the artifact in the LRU
	self.lru.Set(artifact.Name, artifact)
	return err
}

func NewRepository(config_obj *config_proto.Config) *Repository {
	result := &Repository{
		config_obj: config_obj,
		lru:        ttlcache.NewCache(),
	}
	result.lru.SetTTL(10 * time.Second)
	return result
}
