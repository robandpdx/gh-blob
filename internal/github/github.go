package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/robandpdx/gh-blob/internal/clients"
	ghlog "github.com/robandpdx/gh-blob/pkg/logger"

	"github.com/shurcooL/graphql"

	"go.uber.org/zap"
)

const (
	DefaultPartSize           int64 = 100 * 1024 * 1024  // 100 MB
	DefaultMultipartThreshold int64 = 5000 * 1024 * 1024 // 5 GB
)

func GetOrgInfo(orgName string) (*OrgQuery, error) {
	opts := api.ClientOptions{
		Headers: map[string]string{"Accept": "application/json"},
	}

	client, err := api.NewGraphQLClient(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub client: %v", err)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub client: %v", err)
	}

	var query OrgQuery

	variables := map[string]interface{}{
		"login": graphql.String(orgName),
	}
	err = client.Query("GetOrganization", &query, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to query GitHub API: %v", err)
	}

	return &query, nil
}

func QueryBlobFromGitHub(blobId string) (*BlobQuery, error) {
	opts := api.ClientOptions{
		Headers: map[string]string{
			"Accept":           "application/json",
			"GraphQL-Features": "octoshift_github_owned_storage",
		},
	}

	client, err := api.NewGraphQLClient(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub client: %v", err)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub client: %v", err)
	}

	var query BlobQuery

	variables := map[string]interface{}{
		"id": graphql.ID(blobId),
	}
	err = client.Query("QueryBlob", &query, variables)
	if err != nil {
		return nil, fmt.Errorf("failed to query GitHub API: %v", err)
	}

	ghlog.Logger.Info("Blob ID: " + query.Node.MigrationArchive.ID)
	ghlog.Logger.Info("Blob GUID: " + query.Node.MigrationArchive.GUID)
	ghlog.Logger.Info("Blob Name: " + query.Node.MigrationArchive.Name)
	ghlog.Logger.Info("Blob Size: " + fmt.Sprintf("%d", query.Node.MigrationArchive.Size))
	ghlog.Logger.Info("Blob URI: " + query.Node.MigrationArchive.URI)
	ghlog.Logger.Info("Blob Created At: " + query.Node.MigrationArchive.CreatedAt)

	return &query, nil
}

func QueryAllBlobsFromGitHub(orgName string) (*AllBlobsQuery, error) {
	opts := api.ClientOptions{
		Headers: map[string]string{
			"Accept":           "application/json",
			"GraphQL-Features": "octoshift_github_owned_storage",
		},
	}

	client, err := api.NewGraphQLClient(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub client: %v", err)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub client: %v", err)
	}

	var query AllBlobsQuery

	variables := map[string]interface{}{
		"login":     graphql.String(orgName),
		"first":     graphql.Int(50),
		"endCursor": (*graphql.String)(nil),
	}

	page := 1
	blobCount := 0
	for {
		err = client.Query("AllBlobs", &query, variables)
		if err != nil {
			return nil, fmt.Errorf("failed to query GitHub API: %v", err)
		}
		ghlog.Logger.Info("Page: " + fmt.Sprintf("%d", page))
		for _, blob := range query.Organization.MigrationArchives.Nodes {
			ghlog.Logger.Info("Blob ID: " + blob.ID)
			ghlog.Logger.Info("Blob GUID: " + blob.GUID)
			ghlog.Logger.Info("Blob Name: " + blob.Name)
			ghlog.Logger.Info("Blob Size: " + fmt.Sprintf("%d", blob.Size))
			ghlog.Logger.Info("Blob URI: " + blob.URI)
			ghlog.Logger.Info("Blob Created At: " + blob.CreatedAt)
			ghlog.Logger.Info("==========================")
		}

		blobCount += len(query.Organization.MigrationArchives.Nodes)

		if !query.Organization.MigrationArchives.PageInfo.HasNextPage {
			ghlog.Logger.Info("Total blobs: " + fmt.Sprintf("%d", blobCount))
			break
		}
		variables["endCursor"] = graphql.String(query.Organization.MigrationArchives.PageInfo.EndCursor)
		page++
	}

	return &query, nil
}

func UploadArchiveToGitHub(ctx context.Context, input UploadArchiveInput) (*UploadArchiveResponse, error) {
	archiveFilePath := input.ArchiveFilePath
	orgId := input.OrganizationId

	// Open the file
	reader, err := os.Open(archiveFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	currentPos, err := reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, logAndReturnError(archiveFilePath, fmt.Errorf("failed to get current position: %w", err))
	}

	size, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, logAndReturnError(archiveFilePath, fmt.Errorf("failed to determine file size: %w", err))
	}

	_, err = reader.Seek(currentPos, io.SeekStart)
	if err != nil {
		return nil, logAndReturnError(archiveFilePath, fmt.Errorf("failed to reset file position: %w", err))
	}

	var uploadArchiveResponse *UploadArchiveResponse
	if size < DefaultMultipartThreshold {
		uploadArchiveResponse, err = simpleUpload(ctx, orgId, reader, size)
		if err != nil {
			return nil, err
		}
		return uploadArchiveResponse, nil
	} else {
		uploadArchiveResponse, err = multipartUpload(ctx, orgId, reader, size)
		if err != nil {
			return nil, err
		}
		return uploadArchiveResponse, nil
	}

	return nil, fmt.Errorf("multipart upload not implemented")
}

func simpleUpload(ctx context.Context, orgId string, reader io.ReadSeeker, size int64) (*UploadArchiveResponse, error) {
	ghlog.Logger.Info("Uploading file to GitHub",
		zap.String("orgId", fmt.Sprintf("%v", orgId)))

	blobName := filepath.Base(reader.(*os.File).Name())

	// Create a new GitHub client
	githubClient := clients.NewGitHubClient(os.Getenv("GITHUB_TOKEN"))
	client, err := githubClient.GitHubAuth()
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub client: %v", err)
	}

	// Upload the file
	url := fmt.Sprintf("https://uploads.github.com/organizations/%s/gei/archive?name=%s", orgId, blobName)
	req, err := http.NewRequestWithContext(ctx, "POST", url, reader)
	if err != nil {
		return nil, logAndReturnError(blobName, fmt.Errorf("failed to create HTTP request: %w", err))
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("User-Agent", "gh-blob")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("GITHUB_TOKEN")))
	req.ContentLength = size

	resp, err := client.Client().Do(req)
	if err != nil {
		return nil, logAndReturnError(blobName, fmt.Errorf("failed to upload file: %w", err))
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			ghlog.Logger.Error("failed to close response body", zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusCreated {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %v", err)
		}
		defer resp.Body.Close()
		return nil, fmt.Errorf("unexpected response status: %d, body: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		ghlog.Logger.Error("Failed to read response body", zap.Error(err))
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var uploadArchiveResponse UploadArchiveResponse

	// unmarshal the response
	if err := json.Unmarshal(body, &uploadArchiveResponse); err != nil {
		ghlog.Logger.Error("Failed to decode response", zap.Error(err))
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	return &uploadArchiveResponse, nil
}

func multipartUpload(ctx context.Context, orgId string, reader io.ReadSeeker, size int64) (*UploadArchiveResponse, error) {
	ghlog.Logger.Info("Uploading file to GitHub",
		zap.String("orgId", fmt.Sprintf("%v", orgId)))

	blobName := filepath.Base(reader.(*os.File).Name())

	// Create a new GitHub client
	githubClient := clients.NewGitHubClient(os.Getenv("GITHUB_TOKEN"))
	client, err := githubClient.GitHubAuth()
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub client: %v", err)
	}

	// Prepare JSON body
	bodyData := map[string]interface{}{
		"content_type": "application/octet-stream",
		"name":         blobName,
		"size":         size,
	}
	jsonBody, err := json.Marshal(bodyData)

	// Start the upload
	url := fmt.Sprintf("https://uploads.github.com/organizations/%s/gei/archive/blobs/uploads", orgId)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, logAndReturnError(blobName, fmt.Errorf("failed to create HTTP request: %w", err))
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "gh-blob")
	req.Header.Set("GraphQL-Features", "octoshift_github_owned_storage")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("GITHUB_TOKEN")))
	//req.ContentLength = size

	if err != nil {
		return nil, logAndReturnError(blobName, fmt.Errorf("failed to marshal JSON body: %w", err))
	}
	resp, err := client.Client().Do(req)
	if err != nil {
		return nil, logAndReturnError(blobName, fmt.Errorf("failed to upload file: %w", err))
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			ghlog.Logger.Error("failed to close response body", zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusAccepted {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %v", err)
		}
		defer resp.Body.Close()
		return nil, fmt.Errorf("unexpected response status: %d, body: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	// get the Location header from the response
	location := resp.Header.Get("Location")
	if location == "" {
		return nil, fmt.Errorf("missing Location header in response")
	}
	//ghlog.Logger.Info("Location header: " + location)
	// The location looks like this: /organizations/{organization_id}/gei/archive/blobs/uploads?part_number=1&guid=<guid>&upload_id=<upload_id>
	// Parse out the guid and upload_id
	uploadId := ""
	guid := ""
	for _, part := range []string{"guid", "upload_id"} {
		parts := strings.Split(location, part+"=")
		if len(parts) > 1 {
			parts = strings.Split(parts[1], "&")
			if len(parts) > 0 {
				if part == "guid" {
					guid = parts[0]
				} else if part == "upload_id" {
					uploadId = parts[0]
				}
			}
		}
	}

	ghlog.Logger.Info("Upload ID: " + uploadId)
	ghlog.Logger.Info("GUID: " + guid)

	// Upload file in parts of DefaultPartSize (100 MiB)
	partNumber := 1
	var lastLocation string = location
	var nextLocation string = location
	var uploadedBytes int64 = 0

	for uploadedBytes < size {
		ghlog.Logger.Info(fmt.Sprintf("Uploading part %d", partNumber))
		// Calculate the size of this part
		partSize := DefaultPartSize
		if size-uploadedBytes < partSize {
			partSize = size - uploadedBytes
		}

		// Read the part into memory
		partBuf := make([]byte, partSize)
		n, err := reader.Read(partBuf)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("failed to read file part: %v", err)
		}
		if int64(n) != partSize {
			partBuf = partBuf[:n]
		}

		// PATCH request to upload this part
		uploadURL := "https://uploads.github.com" + nextLocation
		req, err := http.NewRequestWithContext(ctx, "PATCH", uploadURL, bytes.NewReader(partBuf))
		if err != nil {
			return nil, fmt.Errorf("failed to create PATCH request: %v", err)
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("User-Agent", "gh-blob")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("GITHUB_TOKEN")))
		req.Header.Set("GraphQL-Features", "octoshift_github_owned_storage")
		req.ContentLength = int64(len(partBuf))

		resp, err := client.Client().Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to upload part %d: %v", partNumber, err)
		}
		if resp.StatusCode != http.StatusAccepted {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("unexpected response status for part %d: %d, body: %s", partNumber, resp.StatusCode, string(body))
		}
		// Save the previous location for the finalization step
		lastLocation = nextLocation
		// Get the next location from the response header
		nextLocation = resp.Header.Get("Location")
		resp.Body.Close()

		uploadedBytes += int64(n)
		partNumber++

		// If this is the last part, break the loop
		if uploadedBytes >= size || nextLocation == "" {
			break
		}
	}

	ghlog.Logger.Info("Finalizing upload...")
	// Finalize the upload by sending a POST to the last location
	finalizeURL := "https://uploads.github.com" + lastLocation
	finalizeReq, err := http.NewRequestWithContext(ctx, "PUT", finalizeURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create finalize request: %v", err)
	}
	finalizeReq.Header.Set("Content-Type", "application/octet-stream")
	finalizeReq.Header.Set("User-Agent", "gh-blob")
	finalizeReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("GITHUB_TOKEN")))
	finalizeReq.Header.Set("GraphQL-Features", "octoshift_github_owned_storage")

	finalizeResp, err := client.Client().Do(finalizeReq)
	if err != nil {
		return nil, fmt.Errorf("failed to finalize upload: %v", err)
	}
	defer finalizeResp.Body.Close()

	if finalizeResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(finalizeResp.Body)
		return nil, fmt.Errorf("unexpected finalize response status: %d, body: %s", finalizeResp.StatusCode, string(body))
	}

	body, err = io.ReadAll(finalizeResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read finalize response body: %v", err)
	}

	var uploadArchiveResponse UploadArchiveResponse

	if err := json.Unmarshal(body, &uploadArchiveResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	uploadArchiveResponse.URI = fmt.Sprintf("gei://archive/%s", guid)
	uploadArchiveResponse.GUID = guid
	uploadArchiveResponse.NodeID = "Not available"
	uploadArchiveResponse.Name = blobName
	uploadArchiveResponse.Size = int(size)
	uploadArchiveResponse.CreatedAt = finalizeResp.Header.Get("Date")

	return &uploadArchiveResponse, nil
}

func DeleteBlobFromGitHub(ctx context.Context, id string) error {
	ghlog.Logger.Info("Deleting blob from GitHub",
		zap.String("id", id))

	githubToken := os.Getenv("GITHUB_TOKEN")

	githubClient := clients.NewGitHubClient(githubToken)
	client, err := githubClient.GitHubAuth()
	if err != nil {
		return fmt.Errorf("failed to create GitHub client: %v", err)
	}

	mutation := `
	mutation deleteMigrationArchive(
		$migrationArchiveId: ID!
		) {
		deleteMigrationArchive(
			input: {
			migrationArchiveId: $migrationArchiveId
			}
		) {
			migrationArchive {
			id
			guid
			name
			size
			uri
			createdAt
			}
		}
	}`

	requestBody := map[string]interface{}{
		"query": mutation,
		"variables": map[string]interface{}{
			"migrationArchiveId": id,
		},
		"operationName": "deleteMigrationArchive",
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		ghlog.Logger.Error("Failed to marshal request body", zap.Error(err))
		return fmt.Errorf("failed to marshal request body: %v", err)
	}

	url := "https://api.github.com/graphql"

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		ghlog.Logger.Error("Failed to create HTTP request", zap.Error(err))
		return fmt.Errorf("failed to create HTTP request: %v", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", githubToken))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "gh-blob")
	req.Header.Set("GraphQL-Features", "octoshift_github_owned_storage")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Client().Do(req)
	if err != nil {
		ghlog.Logger.Error("Failed to make GraphQL request", zap.Error(err))
		return fmt.Errorf("failed to make GraphQL request: %v", err)
	}

	// if the response status is not 200, show error message
	// and the response body and return an error
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response status: %d, body: %s", resp.StatusCode, func() string {
			body, _ := io.ReadAll(resp.Body)
			return string(body)
		}())
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			ghlog.Logger.Error("failed to close response body", zap.Error(err))
		}
	}()

	// Parse the response
	var response GraphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("failed to decode response: %v", err)
	}

	// Check for GraphQL errors
	if len(response.Errors) > 0 {
		errMsg := response.Errors[0].Message
		return fmt.Errorf("GraphQL error: %s", errMsg)
	}

	return nil
}

func logAndReturnError(blobName string, err error) error {
	ghlog.Logger.Error("GitHub upload operation failed",
		zap.String("blobName", blobName),
		zap.Error(err))

	return fmt.Errorf("upload failed: %w", err)
}
