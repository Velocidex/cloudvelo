package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/Velocidex/yaml/v2"
	jsonpatch "github.com/evanphx/json-patch/v5"
	errors "github.com/pkg/errors"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
)

type Tool struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type ElasticConfiguration struct {
	Username           string   `json:"username"`
	Password           string   `json:"password"`
	APIKey             string   `json:"api_key"`
	Addresses          []string `json:"addresses"`
	SecondaryAddresses []string `json:"secondary_addresses"`
	PrimaryOrgs        []string `json:"primary_orgs"`
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
	S3PartSize        uint64 `json:"s3_part_size"`

	ForemanIntervalSeconds int `json:"foreman_interval_seconds"`

	ApprovedTools       []Tool   `json:"approved_tools"`
	DedicatedForeman    bool     `json:"dedicated_foreman"`
	DedicatedForemanOrg string   `json:"dedicated_foreman_org"`
	ForemanExcludedOrgs []string `json:"foreman_excluded_orgs"`
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
	ConfigText         string
	JSONPatch          string
}

func (self *ConfigLoader) ApplyJsonPatch(config_obj *Config) (*Config, error) {
	serialized, err := json.Marshal(config_obj)
	if err != nil {
		return nil, err
	}

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

func (self *ConfigLoader) LoadFromText() (*Config, error) {
	config_obj := &Config{}
	err := yaml.UnmarshalStrict([]byte(self.ConfigText), config_obj)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if self.JSONPatch != "" {
		config_obj, err = self.ApplyJsonPatch(config_obj)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}

	return config_obj, self.validateVeloConfig(config_obj)
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
	return config_obj, self.validateVeloConfig(config_obj)
}

func (self *ConfigLoader) validateVeloConfig(config_obj *Config) error {
	// Serialized the velociraptor part of the config and validate it
	serialized, err := json.Marshal(config_obj.Config)
	if err != nil {
		return err
	}

	os.Setenv("VELOCONFIG", string(serialized))
	velo_config, err := self.VelociraptorLoader.LoadAndValidate()
	if err != nil {
		return err
	}
	config_obj.Config = *velo_config
	return nil
}

func (self *ConfigLoader) Load() (*Config, error) {
	if self.ConfigText != "" {
		return self.LoadFromText()
	} else if self.Filename != "" {
		return self.LoadFilename()
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
