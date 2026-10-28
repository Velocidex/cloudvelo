package repository

import (
	"context"
	"errors"
	"os"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/cloudvelo/schema/api"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/json"
)

// Artifact metadata (tags, hidden and basic flags) is stored in its
// own document in the persisted index rather than inside the artifact
// definition. This allows metadata to be attached to built in
// artifacts, which live in the in memory parent repository and are
// never written to the org's index. It also means re-saving an
// artifact definition does not lose its metadata.
const (
	metadataDocType = "artifact_metadata"

	// Same lifetime as the artifact LRU - eventually consistent
	// across frontends.
	metadataTTL = 10 * time.Second

	allMetadataQuery = `
{
    "query": {
        "bool": {
            "must": [
                {
                    "match": {
                        "doc_type": "artifact_metadata"
                    }
                }
            ]
		}
	}
}
`
)

func metadataDocId(name string) string {
	return metadataDocType + ":" + name
}

// Store the metadata for the named artifact. A nil metadata removes
// it.
func (self *Repository) SetMetadata(
	ctx context.Context, name string,
	metadata *artifacts_proto.ArtifactMetadata) error {
	defer self.invalidateMetadata()

	if metadata == nil {
		return cvelo_services.DeleteDocument(ctx, self.config_obj.OrgId,
			"persisted", metadataDocId(name), cvelo_services.SyncDelete)
	}

	return cvelo_services.SetElasticIndex(ctx, self.config_obj.OrgId,
		"persisted", metadataDocId(name), &api.RepositoryEntry{
			Name:       name,
			Definition: json.MustMarshalString(metadata),
			DocType:    metadataDocType,
		})
}

func (self *Repository) invalidateMetadata() {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.metadata = nil
}

// Returns all the metadata stored in this org keyed by artifact
// name. The returned map must not be modified.
func (self *Repository) getMetadata(
	ctx context.Context) map[string]*artifacts_proto.ArtifactMetadata {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.metadata != nil && time.Now().Before(self.metadata_expiry) {
		return self.metadata
	}

	hits, err := cvelo_services.QueryChan(ctx, self.config_obj, 1000,
		self.config_obj.OrgId, "persisted", allMetadataQuery, "name")
	if err != nil {
		// Serve stale data rather than nothing, and do not cache
		// the failure so the next call retries.
		if !errors.Is(err, os.ErrNotExist) && self.metadata != nil {
			return self.metadata
		}
		return nil
	}

	result := make(map[string]*artifacts_proto.ArtifactMetadata)
	for hit := range hits {
		record := &api.RepositoryEntry{}
		err = json.Unmarshal(hit, record)
		if err != nil {
			continue
		}

		metadata := &artifacts_proto.ArtifactMetadata{}
		err = json.Unmarshal([]byte(record.Definition), metadata)
		if err != nil {
			continue
		}
		result[record.Name] = metadata
	}

	self.metadata = result
	self.metadata_expiry = time.Now().Add(metadataTTL)

	return result
}

// Returns a copy of the artifact with this org's metadata attached. The
// artifact itself may be shared (e.g. a built in from the parent
// repository) so it is never modified in place.
func (self *Repository) decorateMetadata(
	ctx context.Context,
	artifact *artifacts_proto.Artifact) *artifacts_proto.Artifact {
	metadata, pres := self.getMetadata(ctx)[artifact.Name]
	if !pres {
		return artifact
	}

	artifact = proto.Clone(artifact).(*artifacts_proto.Artifact)
	artifact.Metadata = proto.Clone(metadata).(*artifacts_proto.ArtifactMetadata)
	return artifact
}

func (self *Repository) localTags(ctx context.Context) []string {
	var result []string
	for _, metadata := range self.getMetadata(ctx) {
		result = append(result, metadata.Tags...)
	}
	return result
}

func uniqueSorted(in []string) []string {
	lookup := make(map[string]bool)
	for _, i := range in {
		lookup[i] = true
	}

	result := make([]string, 0, len(lookup))
	for k := range lookup {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}
