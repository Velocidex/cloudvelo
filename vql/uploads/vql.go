// Override the upload() VQL function to push the data to S3

package uploads

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/uploads"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/velociraptor/vql/networking"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
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

	upload_response, err := Upload(
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

func init() {
	vql_subsystem.OverrideFunction(&UploadFunction{})
}
