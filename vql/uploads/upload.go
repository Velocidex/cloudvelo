package uploads

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"time"

	"www.velocidex.com/golang/velociraptor/accessors"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/uploads"
	"www.velocidex.com/golang/vfilter"
)

const BUFF_SIZE = 5 * 1024 * 1024

// TODO: Support sparse uploads
func Upload(
	ctx context.Context,
	config_obj *config_proto.Config,
	scope vfilter.Scope,
	ospath *accessors.OSPath,
	accessor string,
	name string,
	size int64, // Expected size.
	mtime, atime, ctime, btime time.Time,
	reader accessors.ReadSeekCloser) (*uploads.UploadResponse, error) {

	if gUploaderFactory == nil {
		return nil, errors.New("Uploader not configured")
	}

	dest := ospath
	if name != "" {
		accessor_obj, err := accessors.GetAccessor(accessor, scope)
		if err != nil {
			return nil, err
		}

		dest, err = accessor_obj.ParsePath(name)
		if err != nil {
			return nil, err
		}
	}

	// Get the session ID
	session_id_any, pres := scope.Resolve("_SessionId")
	if !pres {
		return nil, errors.New("Session ID not found")
	}

	session_id, ok := session_id_any.(string)
	if !ok {
		return nil, errors.New("Session ID not found")
	}

	uploader, err := gUploaderFactory.NewUploader(ctx, session_id, accessor, dest)
	if err != nil {
		return nil, err
	}
	defer func() {
		err := uploader.Close()
		if err != nil {
			scope.Log("ERROR: Finalizing %v: %v",
				dest, err)
		}
	}()

	uploader.session_id = session_id
	uploader.mtime = mtime
	uploader.atime = atime
	uploader.ctime = ctime
	uploader.btime = btime

	buf := make([]byte, BUFF_SIZE)
	for {
		n, err := reader.Read(buf)
		if err != nil && err != io.EOF && n == 0 {
			return nil, err
		}

		if n == 0 {
			break
		}

		err = uploader.Put(buf[:n])
		if err != nil {
			return nil, err
		}
	}

	// If we get here it all went well - commit the result.
	uploader.Commit()

	return &uploads.UploadResponse{
		Path:       ospath.String(),
		StoredName: name,
		Size:       uploader.offset,
		StoredSize: uploader.offset,
		Sha256:     hex.EncodeToString(uploader.sha_sum.Sum(nil)),
		Md5:        hex.EncodeToString(uploader.md5_sum.Sum(nil)),
		Reference:  uploader.file_store_path.AsClientPath(),
	}, nil
}
