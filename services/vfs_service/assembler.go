package vfs_service

import (
	"context"

	"www.velocidex.com/golang/cloudvelo/services"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
)

const (
	// Here id is the hash of the VFS directory itself. The following
	// query will retrieve all VFS records related to the directory.
	// There are two types of records for the same directory:

	// 1. A Directory Listing record: contains a reference into the
	//    VFSListDirectory collection in the JSONData field.
	// 2. A Download record for each downloaded file in the directory:
	//    Contains a reference to the S3 object.

	// Since the VFS is written to a data stream we need to combine
	// all records into the same object.

	// This query will only retrieve the latest directory
	// listing. This is the latest document that contains the data
	// reference. The directory listing record has the same doc_id and
	// id members refering to the directory itself.
	vfsSidePanelRenderQuery = `
{
    "query": {
        "bool": {
            "must": [
                {
                    "match": {"doc_id": %q}
                }, {
                    "match": {"doc_type": "vfs"}
                }
            ]
        }
    },
    "sort": [
      {
        "timestamp": {
          "order": "desc"
        }
      }
    ],
    "size": 1
}
`

	// This query scans all the VFS documents relating to the
	// directory.
	queryAllVFSAttributes = `
{
    "query": {
        "bool": {
            "must": [
                {
                    "match": {"id": %q}
                },
                {
                    "match": {"doc_type": "vfs"}
                }
            ]
        }
    },
    "sort": [
      {
        "timestamp": {
          "order": "desc"
        }
      }
    ],
    "size": 10000
}
`
)

// A helper to rebuild a VFSListResponse from partial hits in the
// datastream.
type VFSAssembler struct {
	listing   string
	downloads map[string]string
}

// Build a full record with the full download list. This function
// scans all the records and assembles them into a coherent view.
func (self *VFSService) readDirectoryWithDownloads(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, components []string) (
	downloads []*DownloadRow,
	stat *api_proto.VFSListResponse, err error) {

	stat = &api_proto.VFSListResponse{}
	assembler := VFSAssembler{
		// Deduplicate downloads to the same file.
		downloads: make(map[string]string),
	}

	components = append([]string{client_id}, components...)
	id := cvelo_services.MakeId(utils.JoinComponents(components, "/"))

	hits, _, err := cvelo_services.QueryElasticRaw(ctx,
		config_obj.OrgId, "results", json.Format(queryAllVFSAttributes, id))
	if err != nil {
		return nil, nil, err
	}

	for _, hit := range hits {
		record := &VFSRecord{}
		err = json.Unmarshal(hit, record)
		if err != nil {
			continue
		}

		if record.JSONData != "" {
			assembler.listing = record.JSONData

		} else if len(record.Downloads) == 1 {
			download_record := record.Downloads[0]

			// This will replace the download record with the latest
			// one.
			assembler.downloads[record.DocId] = download_record
		}
	}

	if assembler.listing == "" {
		return downloads, stat, nil
	}

	err = json.Unmarshal([]byte(assembler.listing), stat)
	if err != nil {
		return downloads, stat, nil
	}

	// Now decode the latest download records.
	for _, download := range assembler.downloads {
		download_record := &DownloadRow{}
		err = json.Unmarshal([]byte(download), download_record)
		if err != nil {
			continue
		}

		// The stat contains the latest Mtime of the latest download.
		if download_record.Mtime > stat.DownloadVersion {
			stat.DownloadVersion = download_record.Mtime
		}

		// If the download failed, remove it from the list.
		if !download_record.InFlight && download_record.Sha256 == "" {
			continue
		}

		downloads = append(downloads, download_record)
	}

	return downloads, stat, nil
}

func getLatestVFSListResponse(
	ctx context.Context,
	config_obj *config_proto.Config,
	client_id string, components []string) (
	*api_proto.VFSListResponse, error) {

	result := &api_proto.VFSListResponse{}
	components = append([]string{client_id}, components...)

	id := services.MakeId(utils.JoinComponents(components, "/"))
	record := &VFSRecord{}
	serialized, err := services.GetElasticRecordByQuery(ctx,
		config_obj.OrgId, "results",
		json.Format(vfsSidePanelRenderQuery, id, id))
	if err != nil || len(serialized) == 0 {
		// Empty responses mean the directory is empty.
		return result, nil
	}

	err = json.Unmarshal(serialized, record)
	if err != nil {
		return result, nil
	}

	err = json.Unmarshal([]byte(record.JSONData), result)
	if err != nil {
		return nil, err
	}

	return result, nil
}
