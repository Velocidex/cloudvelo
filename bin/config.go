package main

import (
	"io/ioutil"
	"os"

	"www.velocidex.com/golang/cloudvelo/config"
	velo_config "www.velocidex.com/golang/velociraptor/config"
)

var (
	override_flag = app.Flag("override", "A json object to override the config.").
			Short('o').String()

	override_file = app.Flag("override_file", "A json object to override the config.").
			String()
)

func loadConfig(velo_loader *velo_config.Loader) (*config.Config, error) {

	override := *override_flag
	if *override_file != "" {
		fd, err := os.Open(*override_file)
		if err != nil {
			return nil, err
		}

		data, err := ioutil.ReadAll(fd)
		if err != nil {
			return nil, err
		}

		override = string(data)
	}

	loader := &config.ConfigLoader{
		VelociraptorLoader: velo_loader,
		Filename:           *config_path,
		JSONPatch:          override,
	}

	return loader.Load()
}
