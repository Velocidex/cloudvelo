package api

type RepositoryEntry struct {
	Name       string `json:"name,omitempty"`
	Definition string `json:"definition,omitempty"`
	DocType    string `json:"doc_type"`
}
