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
		Example: `gh blob upload --org my-org --archive-file-path /path/to/archive"`,
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
	orgInfo, err := github.GetOrgInfo(org)
	if err != nil {
		return fmt.Errorf("failed to fetch organization information: %w", err)
	}

	var orgDatabaseId = orgInfo.Organization.DatabaseId

	uploadArchiveInput := github.UploadArchiveInput{
		ArchiveFilePath: archiveFilePath,
		OrganizationId:  fmt.Sprintf("%d", orgDatabaseId),
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
	ghlog.Logger.Info("ID: " + uploadArchiveResponse.NodeID)
	ghlog.Logger.Info("URL: " + uploadArchiveResponse.URI)

	return nil
}

func DeleteBlob() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a blob from GitHub",
		Long: `Delete a blob from GitHub.
GitHub credentials must be configured via environment variables.`,
		Example: `gh blob delete --id <blob-id>`,
		RunE:    deleteBlob,
	}
	cmd.Flags().String("id", "", "ID of the blob to delete")
	err := cmd.MarkFlagRequired("id")
	if err != nil {
		ghlog.Logger.Error("failed to mark flag as required", zap.Error(err))
		return nil
	}
	return cmd
}

func deleteBlob(cmd *cobra.Command, args []string) error {
	ghlog.Logger.Info("Reading input values for deleting blob from GitHub")

	id, _ := cmd.Flags().GetString("id")

	if id == "" {
		return fmt.Errorf("ID is required")
	}

	// Create context with appropriate timeout
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Minute)
	defer cancel()

	err := github.DeleteBlobFromGitHub(ctx, id)
	if err != nil {
		ghlog.Logger.Error("failed to delete blob from GitHub", zap.Error(err))
		return fmt.Errorf("failed to delete blob from GitHub: %w", err)
	}
	ghlog.Logger.Info("Deleted blob from GitHub successfully")
	return nil
}

func QueryAllBlobs() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query-all",
		Short: "Query all blobs from GitHub",
		Long: `Query all blobs from GitHub.
GitHub credentials must be configured via environment variables.`,
		Example: `gh blob query-all --org my-org`,
		RunE:    queryAllBlobs,
	}
	cmd.Flags().String("org", "", "Owner of the repository")
	err := cmd.MarkFlagRequired("org")
	if err != nil {
		ghlog.Logger.Error("failed to mark flag as required", zap.Error(err))
		return nil
	}
	return cmd
}

func queryAllBlobs(cmd *cobra.Command, args []string) error {
	ghlog.Logger.Info("Reading input values for querying all blobs from GitHub")

	org, _ := cmd.Flags().GetString("org")

	if org == "" {
		return fmt.Errorf("organization is required")
	}

	_, err := github.QueryAllBlobsFromGitHub(org)
	if err != nil {
		ghlog.Logger.Error("failed to query blobs from GitHub", zap.Error(err))
		return fmt.Errorf("failed to query blobs from GitHub: %w", err)
	}
	ghlog.Logger.Info("Queried blobs from GitHub successfully")

	return nil
}

func QueryBlob() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query a blob from GitHub",
		Long: `Query a blob from GitHub.
GitHub credentials must be configured via environment variables.`,
		Example: `gh blob query --id <blob-id>`,
		RunE:    queryBlob,
	}
	cmd.Flags().String("id", "", "ID of the blob to query")
	err := cmd.MarkFlagRequired("id")
	if err != nil {
		ghlog.Logger.Error("failed to mark flag as required", zap.Error(err))
		return nil
	}
	return cmd
}
func queryBlob(cmd *cobra.Command, args []string) error {
	ghlog.Logger.Info("Reading input values for querying blob from GitHub")

	id, _ := cmd.Flags().GetString("id")

	if id == "" {
		return fmt.Errorf("ID is required")
	}

	_, err := github.QueryBlobFromGitHub(id)
	if err != nil {
		ghlog.Logger.Error("failed to query blob from GitHub", zap.Error(err))
		return fmt.Errorf("failed to query blob from GitHub: %w", err)
	}
	ghlog.Logger.Info("Queried blob from GitHub successfully")

	return nil
}
