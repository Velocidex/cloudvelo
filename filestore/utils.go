package filestore

import (
	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/velociraptor/file_store/api"
)

func GetOrgId(file_store_obj api.FileStore) string {
	config_obj := GetConfigObj(file_store_obj)
	if config_obj == nil {
		return ""
	}
	return config_obj.OrgId
}

func GetConfigObj(file_store_obj api.FileStore) *config.Config {
	switch t := file_store_obj.(type) {
	case *S3Filestore:
		return t.config_obj
	case S3Filestore:
		return t.config_obj
	default:
		return nil
	}
}
