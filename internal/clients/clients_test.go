package clients

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	ghlog "github.com/robandpdx/gh-blob/pkg/logger"
	"go.uber.org/zap"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGitHubAuth(t *testing.T) {
	originalLogger := ghlog.Logger
	ghlog.Logger = zap.NewNop()
	t.Cleanup(func() { ghlog.Logger = originalLogger })

	t.Run("missing token", func(t *testing.T) {
		client, err := NewGitHubClient("").GitHubAuth()
		if err == nil || !strings.Contains(err.Error(), "GitHub token is required") {
			t.Fatalf("error = %v, want missing token error", err)
		}
		if client != nil {
			t.Fatal("expected no client without a token")
		}
	})

	t.Run("authenticated client", func(t *testing.T) {
		originalTransport := http.DefaultTransport
		t.Cleanup(func() { http.DefaultTransport = originalTransport })
		wantErr := errors.New("stop before network")
		calls := 0
		http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			if got := req.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Fatalf("Authorization = %q, want Bearer test-token", got)
			}
			return nil, wantErr
		})
		client, err := NewGitHubClient("test-token").GitHubAuth()
		if err != nil {
			t.Fatal(err)
		}
		req, err := client.NewRequest(http.MethodGet, "user", nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Do(context.Background(), req, nil)
		if !errors.Is(err, wantErr) || calls != 1 {
			t.Fatalf("error = %v, calls = %d; want transport error and one call", err, calls)
		}
	})
}

func TestGitlabAuth(t *testing.T) {
	for _, tc := range []struct {
		name     string
		endpoint string
		token    string
		wantErr  string
	}{
		{name: "missing token", endpoint: "https://gitlab.example.com", wantErr: "GitLab PAT is required"},
		{name: "invalid endpoint", endpoint: "://invalid", token: "test-token", wantErr: "parse"},
		{name: "valid client", endpoint: "https://gitlab.example.com", token: "test-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			impl := &GitlabClientImpl{gitlabApiEndpoint: tc.endpoint, gitlabPAT: tc.token}
			client, err := impl.GitlabAuth()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || client != nil {
					t.Fatalf("client = %v, error = %v; want %q and no client", client, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := client.BaseURL().String(); got != "https://gitlab.example.com/api/v4/" {
				t.Fatalf("base URL = %q", got)
			}
		})
	}
}
