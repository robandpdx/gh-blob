package main

import (
	"fmt"
	"os"

	"github.com/robandpdx/gh-blob/cmd"
	"github.com/robandpdx/gh-blob/pkg/logger"

	"github.com/spf13/cobra"
)

var (
	hostname string
	token    string
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "gh blob",
		Short: "GitHub GitLab Migration Tool",
	}

	// Add persistent flags
	rootCmd.PersistentFlags().StringVar(&hostname, "hostname", "github.com", "GitHub Enterprise Cloud with Data Residency hostname (e.g. enterprise.ghe.com)")
	rootCmd.PersistentFlags().StringVar(&token, "token", "", "GitHub Personal Access Token (defaults to GITHUB_TOKEN env var if not provided)")

	// Add commands
	rootCmd.AddCommand(
		cmd.UploadBlob(),
		cmd.QueryAllBlobs(),
		cmd.QueryBlob(),
		cmd.DeleteBlob(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

func init() {
	logger.InitLogger()
	defer logger.SyncLogger()
}
