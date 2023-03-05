package uploads

import (
	"context"
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/vfilter"
)

// An object that can do multipart uploading
type CloudUploader interface {
	// A constructor.
	New(ctx context.Context,
		scope vfilter.Scope,
		dest *accessors.OSPath,
		accessor string,
		name *accessors.OSPath,
		mtime, atime, ctime, btime time.Time,
		size int64, // Expected size.
		uploader_type string) (CloudUploader, error)

	// Upload the buffer as a single part upload. This is used for
	// files that are smaller than BUFF_SIZE.
	PutWhole(buf []byte) error

	// Upload the buffer as a multipart upload.  NOTE: This will be
	// called with minimum 5mb buffers for each part except for the
	// final part.
	Put(buf []byte) error

	// Once the upload is successfull this should be called. If not a
	// Close will cancel the upload.
	Commit()

	// Finalize the upload.
	Close() error

	SetIndex(index *actions_proto.Index)

	GetVQLResponse() *uploads.UploadResponse
}
