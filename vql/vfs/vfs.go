package vfs

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type VFSListDirectoryPluginArgs struct {
	Components []string `vfilter:"required,field=components,doc=The directory to refresh."`
	Accessor   string   `vfilter:"optional,field=accessor,doc=An accessor to use."`
}

type VFSListDirectoryPlugin struct{}

func (self VFSListDirectoryPlugin) Call(
	ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {
	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &VFSListDirectoryPluginArgs{}
		err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("vfs_ls: %s", err.Error())
			return
		}

		err = vql_subsystem.CheckFilesystemAccess(scope, arg.Accessor)
		if err != nil {
			scope.Log("vfs_ls: %s", err.Error())
			return
		}

		if arg.Accessor == "" {
			arg.Accessor = "file"
		}

		accessor, err := accessors.GetAccessor(arg.Accessor, scope)
		if err != nil {
			scope.Log("vfs_ls: %v", err)
			return
		}

		ospath, err := accessor.ParsePath("")
		if err != nil {
			scope.Log("vfs_ls: %v", err)
			return
		}

		ospath.Components = append(ospath.Components, arg.Components...)

		files, err := accessor.ReadDirWithOSPath(ospath)
		if err != nil {
			scope.Log("vfs_ls: %v", err)
			return
		}

		rows := []*ordereddict.Dict{}
		for _, f := range files {
			rows = append(rows, ordereddict.NewDict().
				Set("_FullPath", f.FullPath()).
				Set("_Accessor", arg.Accessor).
				Set("_Data", f.Data()).
				Set("Name", f.Name()).
				Set("Size", f.Size()).
				Set("Mode", f.Mode().String()).
				Set("mtime", f.Mtime()).
				Set("atime", f.Atime()).
				Set("ctime", f.Ctime()).
				Set("btime", f.Btime()))
		}

		output_chan <- ordereddict.NewDict().
			Set("Directory", ospath.String()).
			Set("Components", ospath.Components).
			Set("Count", len(rows)).
			Set("_Accessor", arg.Accessor).
			Set("_JSON", json.MustMarshalString(rows))
	}()

	return output_chan
}
func (self VFSListDirectoryPlugin) Info(scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "vfs_ls",
		Doc:     "List directory and build a VFS object",
		ArgType: type_map.AddType(scope, &VFSListDirectoryPluginArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterPlugin(&VFSListDirectoryPlugin{})
}
