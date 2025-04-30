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

	"github.com/robandpdx/gh-blob/internal/clients"
	ghlog "github.com/robandpdx/gh-blob/pkg/logger"

	"go.uber.org/zap"
)

const (
	DefaultPartSize           int64 = 100 * 1024 * 1024  // 100 MB
	DefaultMultipartThreshold int64 = 5000 * 1024 * 1024 // 5 GB
)

func GetOrgInfo(orgName string) (interface{}, error) {
	ghlog.Logger.Info("Getting organization information from GitHub")

	// Get environment variables
	githubToken := os.Getenv("GITHUB_PAT")
	githubHost := os.Getenv("GITHUB_API_ENDPOINT")

	if githubHost == "" {
		githubHost = "api.github.com"
	}

	githubClient := clients.NewGitHubClient(githubToken)
	client, err := githubClient.GitHubAuth()

	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub client: %v", err)
	}

	query := `
	query($login: String!) {
			organization(login: $login) {
					login
					id
					name
					databaseId
			}
	}`

	requestBody := map[string]interface{}{
		"query": query,
		"variables": QueryVariables{
			Login: orgName,
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %v", err)
	}

	url := fmt.Sprintf("https://%s/graphql", githubHost)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "gh-glx-migrator")

	resp, err := client.Client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make GraphQL request: %v", err)
	}

	// if the response status is not 200, show error message
	// and the response body and return an error
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response status: %d, body: %s", resp.StatusCode, func() string {
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
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	// Check for GraphQL errors
	if len(response.Errors) > 0 {
		errMsg := response.Errors[0].Message
		return nil, fmt.Errorf("GraphQL error: %s", errMsg)
	}

	ghlog.Logger.Info("Successfully retrieved organization information",
		zap.String("organization", orgName))

	return &response.Data, nil
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
	}
	//return multipartUpload(ctx, blobName, reader, size)
	return nil, fmt.Errorf("multipart upload not implemented")
}

func simpleUpload(ctx context.Context, orgId string, reader io.ReadSeeker, size int64) (*UploadArchiveResponse, error) {
	ghlog.Logger.Info("Uploading file to GitHub",
		zap.String("orgId", fmt.Sprintf("%v", orgId)))

	blobName := filepath.Base(reader.(*os.File).Name())

	// Create a new GitHub client
	githubClient := clients.NewGitHubClient(os.Getenv("GITHUB_PAT"))
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
	req.Header.Set("User-Agent", "gh-glx-migrator")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("GITHUB_PAT")))
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

	ghlog.Logger.Info("Successfully uploaded file to GitHub",
		zap.String("blobName", blobName),
		zap.String("orgId", orgId),
		zap.String("url", uploadArchiveResponse.URI),
		zap.Any("size", uploadArchiveResponse.Size))

	return &uploadArchiveResponse, nil
}

func logAndReturnError(blobName string, err error) error {
	ghlog.Logger.Error("GitHub upload operation failed",
		zap.String("blobName", blobName),
		zap.Error(err))

	return fmt.Errorf("upload failed: %w", err)
}
