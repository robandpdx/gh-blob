package clients

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/go-github/v69/github"
	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/robandpdx/gh-blob/pkg/logger"
)

type S3Client interface {
	GetS3Client() (*s3.Client, error)
}

type GitLabClient interface {
	GitlabAuth() (*gitlab.Client, error)
}

type GitHubClient interface {
	GitHubAuth() (*github.Client, error)
}

type AwsClient struct {
}

type GitlabClientImpl struct {
	gitlabApiEndpoint string
	gitlabPAT         string
}

type GitHubClientImpl struct {
	githubPAT string
}

func NewAwsClient() S3Client {
	return &AwsClient{}
}

func NewGitHubClient(pat string) GitHubClient {
	return &GitHubClientImpl{
		githubPAT: pat,
	}
}

func (a *AwsClient) GetS3Client() (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(cfg), nil
}

func (g *GitlabClientImpl) GitlabAuth() (*gitlab.Client, error) {
	if g.gitlabPAT == "" {
		return nil, errors.New("GitLab PAT is required")
	}
	return gitlab.NewOAuthClient(g.gitlabPAT, gitlab.WithBaseURL(g.gitlabApiEndpoint))
}

func (g *GitHubClientImpl) GitHubAuth() (*github.Client, error) {
	if g.githubPAT == "" {
		logger.Logger.Error("GitHub PAT is not set")
		return nil, fmt.Errorf("GitHub token is required. Provide it via --token flag or GITHUB_TOKEN environment variable")
	}
	client := github.NewClient(nil).WithAuthToken(g.githubPAT)
	if client == nil {
		logger.Logger.Error("Failed to create GitHub client")
		return nil, fmt.Errorf("failed to initialize GitHub client")
	}
	return client, nil
}
