package cmd

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ghlog "github.com/robandpdx/gh-blob/pkg/logger"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func setupCommandTest(t *testing.T, transport roundTripFunc) {
	t.Helper()
	originalLogger, originalTransport := ghlog.Logger, http.DefaultTransport
	ghlog.Logger, http.DefaultTransport = zap.NewNop(), transport
	t.Cleanup(func() {
		ghlog.Logger, http.DefaultTransport = originalLogger, originalTransport
	})
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GH_HOST", "github.com")
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
}

func commandRoot(command *cobra.Command) *cobra.Command {
	root := &cobra.Command{Use: "blob", SilenceUsage: true, SilenceErrors: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.PersistentFlags().String("hostname", "github.com", "")
	root.PersistentFlags().String("token", "", "")
	root.AddCommand(command)
	return root
}

func commandResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestBlobCommandRequiredFlags(t *testing.T) {
	for _, tc := range []struct {
		name string
		new  func() *cobra.Command
		args []string
		want string
	}{
		{"upload org", UploadBlob, []string{"--archive-file-path", "archive.tar.gz"}, `"org"`},
		{"upload archive", UploadBlob, []string{"--org", "my-org"}, `"archive-file-path"`},
		{"delete", DeleteBlob, nil, `"id"`},
		{"query", QueryBlob, nil, `"id"`},
		{"query-all", QueryAllBlobs, nil, `"org"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupCommandTest(t, func(*http.Request) (*http.Response, error) {
				t.Fatal("invalid command must not make a network request")
				return nil, nil
			})
			command := tc.new()
			root := commandRoot(command)
			root.SetArgs(append([]string{command.Name()}, tc.args...))
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want required flag %s", err, tc.want)
			}
		})
	}
}

func TestUploadTimeoutFlag(t *testing.T) {
	command := UploadBlob()
	if got, err := command.Flags().GetDuration("timeout"); err != nil || got != time.Hour {
		t.Fatalf("timeout = %s, error = %v; want 1h", got, err)
	}
	if err := command.ParseFlags([]string{"-t", "1h15m"}); err != nil {
		t.Fatal(err)
	}
	if got, err := command.Flags().GetDuration("timeout"); err != nil || got != 75*time.Minute {
		t.Fatalf("timeout = %s, error = %v; want 1h15m", got, err)
	}
	if err := command.ParseFlags([]string{"--timeout", "invalid"}); err == nil {
		t.Fatal("invalid timeout must be rejected")
	}
}

func TestBlobCommandValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		new     func() *cobra.Command
		flags   map[string]string
		token   string
		wantErr string
	}{
		{"upload missing token", UploadBlob, nil, "", "GitHub token is required"},
		{"delete missing token", DeleteBlob, nil, "", "GitHub token is required"},
		{"query missing token", QueryBlob, nil, "", "GitHub token is required"},
		{"query-all missing token", QueryAllBlobs, nil, "", "GitHub token is required"},
		{"delete missing ID", DeleteBlob, nil, "test-token", "ID is required"},
		{"query missing ID", QueryBlob, nil, "test-token", "ID is required"},
		{"query-all missing organization", QueryAllBlobs, nil, "test-token", "organization is required"},
		{"upload missing file", UploadBlob, map[string]string{"archive-file-path": "missing"}, "test-token", "file does not exist"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupCommandTest(t, func(*http.Request) (*http.Response, error) {
				t.Fatal("invalid input must not make a network request")
				return nil, nil
			})
			t.Setenv("GITHUB_TOKEN", tc.token)
			command := tc.new()
			command.Flags().String("hostname", "github.com", "")
			command.Flags().String("token", "", "")
			for key, value := range tc.flags {
				if key == "archive-file-path" {
					value = filepath.Join(t.TempDir(), value)
				}
				if err := command.Flags().Set(key, value); err != nil {
					t.Fatal(err)
				}
			}
			// Call the handler directly to exercise validation beyond Cobra's required flags.
			if err := command.RunE(command, nil); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestBlobCommandExecution(t *testing.T) {
	for _, tc := range []struct {
		name     string
		new      func() *cobra.Command
		args     []string
		response string
		wantErr  string
	}{
		{"upload", UploadBlob, []string{"-o", "my-org"}, `{"data":{"organization":{"databaseId":42}}}`, "failed to fetch organization information"},
		{"delete", DeleteBlob, []string{"-i", "blob-id"}, `{"data":{}}`, "failed to delete blob from GitHub"},
		{"query", QueryBlob, []string{"-i", "blob-id"}, `{"data":{"node":{"id":"blob-id"}}}`, "failed to query blob from GitHub"},
		{"query-all", QueryAllBlobs, []string{"-o", "my-org"}, `{"data":{"organization":{"migrationArchives":{"pageInfo":{"hasNextPage":false},"nodes":[]}}}}`, "failed to query blobs from GitHub"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, mode := range []string{"environment token", "flag overrides environment", "API failure"} {
				t.Run(mode, func(t *testing.T) {
					calls := 0
					wantToken := "environment-token"
					if mode == "flag overrides environment" {
						wantToken = "flag-token"
					}
					setupCommandTest(t, func(req *http.Request) (*http.Response, error) {
						calls++
						wantAuthorization := "token " + wantToken
						if tc.name == "delete" || req.URL.Host == "uploads.github.com" {
							wantAuthorization = "Bearer " + wantToken
						}
						if req.Header.Get("Authorization") != wantAuthorization {
							t.Fatalf("unexpected Authorization: %q", req.Header.Get("Authorization"))
						}
						if mode == "API failure" {
							return commandResponse(http.StatusOK, `{"errors":[{"message":"access denied"}]}`), nil
						}
						if req.URL.Host == "uploads.github.com" {
							body, err := io.ReadAll(req.Body)
							if err != nil {
								t.Fatal(err)
							}
							if req.URL.Path != "/organizations/42/gei/archive" || string(body) != "archive contents" {
								t.Fatalf("unexpected upload: %s, body %q", req.URL, body)
							}
							return commandResponse(http.StatusCreated, `{"node_id":"blob-id","size":16}`), nil
						}
						if req.URL.String() != "https://api.github.com/graphql" {
							t.Fatalf("unexpected API URL: %s", req.URL)
						}
						return commandResponse(http.StatusOK, tc.response), nil
					})
					t.Setenv("GITHUB_TOKEN", "environment-token")
					command := tc.new()
					args := append([]string{command.Name()}, tc.args...)
					if tc.name == "upload" {
						path := filepath.Join(t.TempDir(), "archive.tar.gz")
						if err := os.WriteFile(path, []byte("archive contents"), 0600); err != nil {
							t.Fatal(err)
						}
						args = append(args, "-a", path)
					}
					if mode == "flag overrides environment" {
						args = append(args, "--token", "flag-token")
					}
					root := commandRoot(command)
					root.SetArgs(args)
					err := root.Execute()
					if mode == "API failure" {
						if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
							t.Fatalf("error = %v, want %q", err, tc.wantErr)
						}
					} else if err != nil {
						t.Fatal(err)
					}
					wantCalls := 1
					if tc.name == "upload" && mode != "API failure" {
						wantCalls = 2
					}
					if calls != wantCalls {
						t.Fatalf("calls = %d, want %d", calls, wantCalls)
					}
				})
			}
		})
	}
}

func TestUploadBlobTimeout(t *testing.T) {
	setupCommandTest(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "api.github.com" {
			return commandResponse(http.StatusOK, `{"data":{"organization":{"databaseId":42}}}`), nil
		}
		if !errors.Is(req.Context().Err(), context.DeadlineExceeded) {
			t.Fatal("upload request must use the expired timeout context")
		}
		return nil, req.Context().Err()
	})
	t.Setenv("GITHUB_TOKEN", "test-token")
	path := filepath.Join(t.TempDir(), "archive.tar.gz")
	if err := os.WriteFile(path, []byte("archive contents"), 0600); err != nil {
		t.Fatal(err)
	}
	root := commandRoot(UploadBlob())
	root.SetArgs([]string{"upload", "-o", "my-org", "-a", path, "--timeout", "0s"})
	err := root.Execute()
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "failed to upload to GitHub storage") {
		t.Fatalf("error = %v, want wrapped timeout error", err)
	}
}
