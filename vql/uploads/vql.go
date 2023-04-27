// Override the upload() VQL function to push the data to S3

package uploads

import (
	"context"
	"errors"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/artifacts"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/uploads"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/velociraptor/vql/networking"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

// S3 requires a minimum of 5mb per multi part upload
var (
	BUFF_SIZE       = 10 * 1024 * 1024
	MAX_FILE_LENGTH = uint64(10 * 1000000000) // 10 Gb
)

type UploadFunction struct{}

func (self *UploadFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &networking.UploadFunctionArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("upload: %v", err)
		return vfilter.Null{}
	}

	if arg.File == nil {
		return vfilter.Null{}
	}

	if arg.Accessor == "" {
		arg.Accessor = "auto"
	}

	err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
	if err != nil {
		scope.Log("upload: %v", err)
		return vfilter.Null{}
	}

	config_obj, ok := vql_subsystem.GetServerConfig(scope)
	if !ok {
		scope.Log("upload: unable to get config")
		return vfilter.Null{}
	}

	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		scope.Log("upload: %v", err)
		return &uploads.UploadResponse{
			Error: err.Error(),
		}
	}

	file, err := accessor.OpenWithOSPath(arg.File)
	if err != nil {
		scope.Log("upload: Unable to open %s: %s",
			arg.File, err.Error())
		return &uploads.UploadResponse{
			Error: err.Error(),
		}
	}
	defer file.Close()

	stat, err := accessor.LstatWithOSPath(arg.File)
	if err != nil {
		scope.Log("upload: Unable to stat %s: %v",
			arg.File, err)
		return vfilter.Null{}
	}

	mtime, err := functions.TimeFromAny(scope, arg.Mtime)
	if err != nil {
		mtime = stat.ModTime()
	}

	atime, _ := functions.TimeFromAny(scope, arg.Atime)
	ctime, _ := functions.TimeFromAny(scope, arg.Ctime)
	btime, _ := functions.TimeFromAny(scope, arg.Btime)

	upload_response, err := self.Upload(
		ctx, config_obj,
		scope, arg.File,
		arg.Accessor,
		arg.Name,
		stat.Size(), // Expected size.
		mtime, atime, ctime, btime,
		file)
	if err != nil {
		scope.Log("ERROR:upload: Unable to upload %v: %v", arg.File, err)
		return &uploads.UploadResponse{
			Error: err.Error(),
		}
	}
	return upload_response
}

func (self UploadFunction) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name: "upload",
		Doc: "Upload a file to the upload service. For a Velociraptor " +
			"client this will upload the file into the flow and store " +
			"it in the server's file store.",
		ArgType: type_map.AddType(scope, &networking.UploadFunctionArgs{}),
	}
}

// Upload the file possibly using the configured uploader factory.
func (self UploadFunction) Upload(
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
		// No uploader factory configured, Try to get an uploader from
		// the scope. This happens with command line tools etc.
		uploader, ok := artifacts.GetUploader(scope)
		if !ok {
			return nil, errors.New("Uploader not configured")
		}

		return uploader.Upload(ctx, scope, ospath, accessor, name,
			size, mtime, atime, ctime, btime, reader)
	}

	// If we get here we have a specialized uploader factory
	dest := ospath
	if name != nil {
		dest = name
	}

	// A regular uploader for bulk data.
	uploader, err := gUploaderFactory.New(
		ctx, scope, dest, accessor, dest,
		mtime, atime, ctime, btime, size, "")
	if err != nil {
		return nil, err
	}
	defer uploader.Close()

	// Try to upload a sparse file.
	range_reader, ok := reader.(uploads.RangeReader)
	if ok {
		// A new uploader for the index file.
		idx_name := dest.Dirname().Append(dest.Basename() + ".idx")
		idx_uploader, err := gUploaderFactory.New(
			ctx, scope, dest, accessor, idx_name, mtime,
			atime, ctime, btime, size, "idx")
		if err != nil {
			return nil, err
		}
		defer idx_uploader.Close()

		return UploadSparse(ctx, dest, idx_uploader, uploader, range_reader)
	}

	// We need to buffer writes untilt they reach 5mb before we can
	// send them. This is managed by the BufferedWriter object which
	// wraps the uploader.
	buffer := NewBufferWriter(uploader)
	err = buffer.Copy(reader, MAX_FILE_LENGTH)
	if err != nil {
		scope.Log("ERROR: Finalizing %v: %v", dest, err)
		return &uploads.UploadResponse{
			Error: err.Error(),
		}, nil
	}

	err = buffer.Close()
	if err != nil {
		scope.Log("ERROR: Finalizing %v: %v", dest, err)
		return &uploads.UploadResponse{
			Error: err.Error(),
		}, nil
	}

	return uploader.GetVQLResponse(), nil
}

func init() {
	vql_subsystem.OverrideFunction(&UploadFunction{})
}
