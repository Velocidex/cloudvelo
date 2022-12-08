package uploads

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/vfilter"
)

// S3 required a minimum of 5mb per multi part upload
var (
	BUFF_SIZE       = 10 * 1024 * 1024
	MAX_FILE_LENGTH = uint64(10 * 1000000000) // 10 Gb
)

func Upload(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	ospath *accessors.OSPath,
	accessor string,
	name *accessors.OSPath,
	size int64, // Expected size.
	mtime, atime, ctime, btime time.Time,
	reader accessors.ReadSeekCloser) (*uploads.UploadResponse, error) {

	if gUploaderFactory == nil {
		// Try to get an uploader from the scope.
		uploader, ok := artifacts.GetUploader(scope)
		if !ok {
			return nil, errors.New("Uploader not configured")
		}

		return uploader.Upload(ctx, scope, ospath, accessor, name,
			size, mtime, atime, ctime, btime, reader)
	}

	dest := ospath
	if name != nil {
		name = ospath
	}

	// A regular uploader for bulk data.
	uploader, err := makeUploader(
		ctx, scope, dest, accessor, name,
		mtime, atime, ctime, btime, "")
	if err != nil {
		return nil, err
	}

	// Try to upload a sparse file.
	range_reader, ok := reader.(uploads.RangeReader)
	if ok {
		// A new uploader for the index file.
		idx_uploader, err := makeUploader(
			ctx, scope, dest, accessor, name, mtime,
			atime, ctime, btime, "idx")
		if err != nil {
			return nil, err
		}

		return UploadSparse(ctx, dest, idx_uploader, uploader, range_reader)
	}

	buffer := NewBufferWriter(uploader)
	err = buffer.Copy(reader, MAX_FILE_LENGTH)
	if err != nil {
		scope.Log("ERROR: Finalizing %v: %v", dest, err)
		return &uploads.UploadResponse{
			Error: err.Error(),
		}, nil
	}

	err = buffer.Flush()
	if err != nil {
		scope.Log("ERROR: Finalizing %v: %v", dest, err)
		return &uploads.UploadResponse{
			Error: err.Error(),
		}, nil
	}

	// If we get here it all went well - commit the result.
	uploader.Commit()

	err = uploader.Close()
	if err != nil {
		scope.Log("ERROR: Finalizing %v: %v", dest, err)
		return &uploads.UploadResponse{
			Error: err.Error(),
		}, nil
	}

	result := &uploads.UploadResponse{
		Path:       dest.String(),
		Size:       uploader.offset,
		StoredSize: uploader.offset,
		Sha256:     hex.EncodeToString(uploader.sha_sum.Sum(nil)),
		Md5:        hex.EncodeToString(uploader.md5_sum.Sum(nil)),
	}

	if name != nil {
		result.StoredName = name.String()
	}
	return result, nil
}

func makeUploader(
	ctx context.Context,
	scope vfilter.Scope,
	dest *accessors.OSPath,
	accessor string,
	name *accessors.OSPath,
	mtime, atime, ctime, btime time.Time,
	uploader_type string) (*Uploader, error) {

	// Get the session ID
	session_id_any, pres := scope.Resolve("_SessionId")
	if !pres {
		return nil, errors.New("Session ID not found")
	}

	session_id, ok := session_id_any.(string)
	if !ok {
		return nil, errors.New("Session ID not found")
	}

	uploader, err := gUploaderFactory.NewUploader(
		ctx, session_id, accessor, uploader_type, dest)
	if err != nil {
		return nil, err
	}

	uploader.mtime = mtime
	uploader.atime = atime
	uploader.ctime = ctime
	uploader.btime = btime

	return uploader, nil
}
