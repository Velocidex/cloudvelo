package notebook

import (
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/utils"
)

func checkNotebookAccess(notebook *api_proto.NotebookMetadata, user string) bool {
	if notebook.Public {
		return true
	}

	return notebook.Creator == user || utils.InString(notebook.Collaborators, user)
}
