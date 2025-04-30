package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/robandpdx/gh-blob/internal/github"
	ghlog "github.com/robandpdx/gh-blob/pkg/logger"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func UploadBlob() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload a blob to GitHub",
		Long: `Upload a blob to GitHub.
GitHub credentials must be configured via environment variables.`,
		Example: `gh glx upload-blob --org my-org --archive-file-path /path/to/archive"`,
		RunE:    uploadBlob,
	}

	cmd.Flags().String("org", "", "Owner of the repository")
	cmd.Flags().String("archive-file-path", "", "Path to the blob")

	err := cmd.MarkFlagRequired("org")
	if err != nil {
		ghlog.Logger.Error("failed to mark flag as required", zap.Error(err))
		return nil
	}
	err = cmd.MarkFlagRequired("archive-file-path")
	if err != nil {
		ghlog.Logger.Error("failed to mark flag as required", zap.Error(err))
		return nil
	}
	return cmd
}

func uploadBlob(cmd *cobra.Command, args []string) error {
	ghlog.Logger.Info("Reading input values for uploading blob to GitHub")

	org, _ := cmd.Flags().GetString("org")
	archiveFilePath, _ := cmd.Flags().GetString("archive-file-path")

	if _, err := os.Stat(archiveFilePath); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", archiveFilePath)
	}

	// Get the GitHub org id
	orgMap, err := fetchOrgInfo(org)
	if err != nil {
		return fmt.Errorf("failed to fetch organization information: %w", err)
	}

	var orgId = orgMap["id"]
	var orgDatabaseId = orgMap["databaseId"]
	// convert orgDatabaseId to int
	orgDatabaseId = int(orgDatabaseId.(float64))
	// convert orgDatabaseId to string
	orgDatabaseId = fmt.Sprintf("%v", orgDatabaseId)
	ghlog.Logger.Info("orgId: " + fmt.Sprintf("%v", orgId))

	uploadArchiveInput := github.UploadArchiveInput{
		ArchiveFilePath: archiveFilePath,
		OrganizationId:  orgDatabaseId.(string),
	}

	// Create context with appropriate timeout
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Minute)
	defer cancel()

	uploadArchiveResponse, err := github.UploadArchiveToGitHub(ctx, uploadArchiveInput)
	if err != nil {
		ghlog.Logger.Error("failed to upload to GitHub storage", zap.Error(err))
		return fmt.Errorf("failed to upload to GitHub storage: %w", err)
	}
	ghlog.Logger.Info("Uploaded archive to GitHub storage successfully")
	ghlog.Logger.Info("URL: " + uploadArchiveResponse.URI)

	return nil
}

// create a fetchOrgInfo function that takes org as input and return orgMap
func fetchOrgInfo(org string) (map[string]interface{}, error) {
	orgInfo, err := github.GetOrgInfo(org)
	if err != nil {
		ghlog.Logger.Debug("failed to get organization information from GitHub", zap.Error(err))
		return nil, fmt.Errorf("failed to get organization information from GitHub: %v", err)
	}

	ghlog.Logger.Info("Organization information from GitHub", zap.Any("orgInfo", orgInfo))

	// Handle the *interface{} case specifically
	var orgMap map[string]interface{}

	// First try to unwrap the pointer to interface{}
	if ptr, ok := orgInfo.(*interface{}); ok {
		// Then try to convert the unwrapped value to map[string]interface{}
		if unwrapped, ok := (*ptr).(map[string]interface{}); ok {
			if org, ok := unwrapped["organization"].(map[string]interface{}); ok {
				orgMap = org
			}
		}
	} else if direct, ok := orgInfo.(map[string]interface{}); ok {
		// Try direct assertion to map[string]interface{}
		if org, ok := direct["organization"].(map[string]interface{}); ok {
			orgMap = org
		}
	}

	if orgMap == nil {
		ghlog.Logger.Error("Could not parse organization data",
			zap.String("type", fmt.Sprintf("%T", orgInfo)))
		return nil, fmt.Errorf("failed to parse organization data")
	}

	return orgMap, nil
}
