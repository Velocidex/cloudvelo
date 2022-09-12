package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/Velocidex/yaml/v2"
	jsonpatch "github.com/evanphx/json-patch"
	errors "github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

type ElasticConfiguration struct {
	Username           string   `json:"username"`
	Password           string   `json:"password"`
	APIKey             string   `json:"api_key"`
	Addresses          []string `json:"addresses"`
	CloudID            string   `json:"cloud_id"`
	DisableSSLSecurity bool     `json:"disable_ssl_security"`
	RootCerts          string   `json:"root_cert"`

	// The name of the index we should use (default velociraptor)
	Index string `json:"index"`

	// AWS S3 settings
	AWSRegion         string `json:"aws_region"`
	CredentialsKey    string `json:"credentials_key"`
	CredentialsSecret string `json:"credentials_secret"`
	Endpoint          string `json:"endpoint"`
	NoVerifyCert      bool   `json:"no_verify_cert"`
	Bucket            string `json:"bucket"`
}

// Create a new cloud config object which contains the original
// Velociraptor server config as well as adiitioal cloud component.
type Config struct {
	config_proto.Config `json:",inline"`
	Cloud               ElasticConfiguration `json:"Cloud"`
}

func (self Config) VeloConf() *config_proto.Config {
	return &self.Config
}

type ConfigLoader struct {
	VelociraptorLoader *config.Loader
	Filename           string
	JSONPatch          string
}

func (self *ConfigLoader) ApplyJsonPatch(config_obj *Config) (*Config, error) {
	serialized, err := json.Marshal(config_obj)
	if err != nil {
		return nil, err
	}

	/*
		patch, err := jsonpatch.DecodePatch([]byte(self.JSONPatch))
		if err != nil {
			return nil, fmt.Errorf("Decoding JSON patch: %w", err)
		}

		patched, err := patch.Apply(serialized)
		if err != nil {
			return nil, fmt.Errorf("Invalid merge patch: %w", err)
		}
	*/

	patched, err := jsonpatch.MergePatch(serialized, []byte(self.JSONPatch))
	if err != nil {
		return nil, fmt.Errorf("Invalid merge patch: %w", err)
	}

	result := &Config{}
	err = json.Unmarshal(patched, result)
	if err != nil {
		return nil, fmt.Errorf(
			"Patched object produces an invalid config (%v): %w",
			self.JSONPatch, err)
	}

	return result, nil
}

func (self *ConfigLoader) LoadFilename() (*Config, error) {
	self.VelociraptorLoader.WithFileLoader("")
	config_obj, err := read_config_from_file(self.Filename)
	if err != nil {
		return nil, err
	}

	if self.JSONPatch != "" {
		config_obj, err = self.ApplyJsonPatch(config_obj)
		if err != nil {
			return nil, err
		}
	}

	// Now load and verify the velociraptor config
	return config_obj, nil
}

func (self *ConfigLoader) Load() (*Config, error) {
	if self.Filename != "" {
		config_obj, err := self.LoadFilename()
		if err != nil {
			return nil, err
		}

		// Now serialized the velociraptor part of the config and
		// validate it
		serialized, err := json.Marshal(config_obj.Config)
		if err != nil {
			return nil, err
		}

		os.Setenv("VELOCONFIG", string(serialized))

		velo_config, err := self.VelociraptorLoader.
			WithEnvLiteralLoader("VELOCONFIG").
			LoadAndValidate()
		if err != nil {
			return nil, err
		}
		config_obj.Config = *velo_config
		return config_obj, nil
	}
	return nil, errors.New("Unable to load config from anywhere")
}

func read_config_from_file(filename string) (*Config, error) {
	result := &Config{}

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	err = yaml.UnmarshalStrict(data, result)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return result, nil
}
