package github

type GraphQLResponse struct {
	Data   interface{} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

type QueryVariables struct {
	Login string `json:"login"`
}

type OrgResponse struct {
	Organization struct {
		Login      string `json:"login"`
		ID         string `json:"id"`
		Name       string `json:"name"`
		DatabaseID int    `json:"databaseId"`
	} `json:"organization"`
}

type UploadArchiveInput struct {
	ArchiveFilePath string
	OrganizationId  string
}
type UploadArchiveResponse struct {
	GUID      string `json:"guid"`
	NodeID    string `json:"node_id"`
	Name      string `json:"name"`
	Size      int    `json:"size"`
	URI       string `json:"uri"`
	CreatedAt string `json:"created_at"`
}
type OrgQuery struct {
	Organization struct {
		Login      string `graphql:"login"`
		ID         string `graphql:"id"`
		Name       string `graphql:"name"`
		DatabaseId int    `graphql:"databaseId"`
	} `graphql:"organization(login: $login)"`
}
