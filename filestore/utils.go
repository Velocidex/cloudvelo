package filestore

import "www.velocidex.com/golang/velociraptor/file_store/api"

func GetOrgId(file_store_obj api.FileStore) string {
	s3_filestore, ok := file_store_obj.(S3Filestore)
	if ok {
		return s3_filestore.config_obj.OrgId
	}
	return ""
}
