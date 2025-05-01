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

type AllBlobsQuery struct {
	Organization struct {
		Login             string `graphql:"login"`
		ID                string `graphql:"id"`
		Name              string `graphql:"name"`
		DatabaseId        int    `graphql:"databaseId"`
		MigrationArchives struct {
			PageInfo struct {
				HasNextPage bool
				EndCursor   string
			}
			Nodes []struct {
				GUID      string `graphql:"guid"`
				ID        string `graphql:"id"`
				Name      string `graphql:"name"`
				Size      int    `graphql:"size"`
				URI       string `graphql:"uri"`
				CreatedAt string `graphql:"createdAt"`
			}
		} `graphql:"migrationArchives(first: $first, after: $endCursor)"`
	} `graphql:"organization(login: $login)"`
}
