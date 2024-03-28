package notebooks

import (
	"context"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/velociraptor/acls"
	"www.velocidex.com/golang/velociraptor/json"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

const (
	all_notebook_items = `
{"query": {
    "bool": {
        "must": [
            {"match": {"notebook_id": %q}}
        ]}
}}
`
)

type DeleteNotebookArgs struct {
	NotebookId string `vfilter:"required,field=notebook_id"`
	ReallyDoIt bool   `vfilter:"optional,field=really_do_it"`
}

type DeleteNotebookPlugin struct{}

func (self *DeleteNotebookPlugin) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) <-chan vfilter.Row {

	output_chan := make(chan vfilter.Row)

	go func() {
		defer close(output_chan)

		arg := &DeleteNotebookArgs{}

		err := vql_subsystem.CheckAccess(scope, acls.SERVER_ADMIN)
		if err != nil {
			scope.Log("notebook_delete: %s", err)
			return
		}

		err = arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
		if err != nil {
			scope.Log("notebook_delete: %s", err.Error())
			return
		}

		config_obj, ok := vql_subsystem.GetServerConfig(scope)
		if !ok {
			scope.Log("notebook_delete: Command can only run on the server")
			return
		}

		if arg.ReallyDoIt {
			err := services.DeleteByQuery(
				ctx, config_obj.OrgId, "persisted",
				json.Format(all_notebook_items, arg.NotebookId))
			if err != nil {
				scope.Log("notebook_delete: %v", err)
			}
		}

	}()

	return output_chan
}

func (self DeleteNotebookPlugin) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.PluginInfo {
	return &vfilter.PluginInfo{
		Name:    "notebook_delete",
		Doc:     "Delete a notebook with all its cells. ",
		ArgType: type_map.AddType(scope, &DeleteNotebookArgs{}),
	}
}

func init() {
	vql_subsystem.OverridePlugin(&DeleteNotebookPlugin{})
}
