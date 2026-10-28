package config_test

import (
	"testing"

	"github.com/alecthomas/assert"
	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/cloudvelo/testsuite"
	velo_config "www.velocidex.com/golang/velociraptor/config"
)

func TestDatastoreCompressionAccepted(t *testing.T) {
	for _, patch := range []string{
		`{"Datastore": {"compression": null}}`,
		`{"Datastore": {"compression": "none"}}`,
		`{"Datastore": {"compression": "NONE"}}`,
	} {
		config_obj, err := loadWithPatch(patch)
		assert.NoError(t, err, patch)
		assert.Equal(t, "none", config_obj.Datastore.Compression, patch)
	}
}

func TestDatastoreCompressionRefused(t *testing.T) {
	for _, value := range []string{"zlib", "ZLIB", "gzip"} {
		_, err := loadWithPatch(`{"Datastore": {"compression": "` + value + `"}}`)
		assert.Error(t, err, value)
		assert.Contains(t, err.Error(), "Datastore.compression", value)
	}
}

func loadWithPatch(patch string) (*config.Config, error) {
	loader := config.ConfigLoader{
		VelociraptorLoader: new(velo_config.Loader).
			WithRequiredFrontend().
			WithEnvLiteralLoader("VELOCONFIG"),
		ConfigText: testsuite.SERVER_CONFIG,
		JSONPatch:  patch,
	}
	return loader.Load()
}
