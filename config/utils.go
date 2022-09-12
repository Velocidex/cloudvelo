package config

import (
	"io/ioutil"
	"os"

	"github.com/Velocidex/yaml/v2"
)

func LoadConfig(path string) (*ElasticConfiguration, error) {
	fd, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	serialized, err := ioutil.ReadAll(fd)
	if err != nil {
		return nil, err
	}

	result := &ElasticConfiguration{}
	err = yaml.UnmarshalStrict(serialized, result)
	return result, err
}
