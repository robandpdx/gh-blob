package main

import (
	"fmt"
	"os"

	"github.com/robandpdx/gh-blob/cmd"
	"github.com/robandpdx/gh-blob/pkg/logger"
	"go.uber.org/zap"

	"github.com/spf13/cobra"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "gh-glx",
		Short: "GitHub GitLab Migration Tool",
	}

	// Add commands
	rootCmd.AddCommand(
		cmd.UploadBlob(),
		//cmd.QueryAllBlobs(),
		//cmd.QueryBlob(),
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

	required := []struct {
		name  string
		value string
	}{
		{"GITHUB_TOKEN", os.Getenv("GITHUB_TOKEN")},
	}

	var missing []string

	for _, r := range required {
		if r.value == "" {
			missing = append(missing, r.name)
		}
	}

	if len(missing) > 0 {
		logger.Logger.Error("Missing required environment variables",
			zap.Strings("missing", missing))
		os.Exit(1)
	}
}
