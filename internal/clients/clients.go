package clients

import (
	"errors"
	"fmt"

	"github.com/google/go-github/v69/github"
	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/robandpdx/gh-blob/pkg/logger"
)

type GitLabClient interface {
	GitlabAuth() (*gitlab.Client, error)
}

type GitHubClient interface {
	GitHubAuth() (*github.Client, error)
}

type GitlabClientImpl struct {
	gitlabApiEndpoint string
	gitlabPAT         string
}

type GitHubClientImpl struct {
	githubPAT string
}

func NewGitHubClient(pat string) GitHubClient {
	return &GitHubClientImpl{
		githubPAT: pat,
	}
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
		return nil, fmt.Errorf("GITHUB_TOKEN environment variable is not set")
	}
	client := github.NewClient(nil).WithAuthToken(g.githubPAT)
	if client == nil {
		logger.Logger.Error("Failed to create GitHub client")
		return nil, fmt.Errorf("failed to initialize GitHub client")
	}
	return client, nil
}
