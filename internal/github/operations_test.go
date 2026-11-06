package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	ghlog "github.com/robandpdx/gh-blob/pkg/logger"
	"go.uber.org/zap"
)

func mockHTTP(t *testing.T, transport roundTripFunc) {
	t.Helper()
	originalTransport, originalLogger, originalDelay := http.DefaultTransport, ghlog.Logger, retryBaseDelay
	http.DefaultTransport, ghlog.Logger, retryBaseDelay = transport, zap.NewNop(), time.Millisecond
	t.Cleanup(func() {
		http.DefaultTransport, ghlog.Logger, retryBaseDelay = originalTransport, originalLogger, originalDelay
	})
}

func testResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func testArchive(t *testing.T) *os.File {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "archive-*.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	if _, err := file.WriteString("archive contents"); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	return file
}

type graphQLRequest struct {
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables"`
	OperationName string                 `json:"operationName"`
}

func decodeGraphQLRequest(t *testing.T, req *http.Request) graphQLRequest {
	t.Helper()
	if req.Method != http.MethodPost || req.URL.String() != "https://api.github.com/graphql" {
		t.Fatalf("unexpected GraphQL request: %s %s", req.Method, req.URL)
	}
	var body graphQLRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestGetOrgInfo(t *testing.T) {
	mockHTTP(t, func(req *http.Request) (*http.Response, error) {
		body := decodeGraphQLRequest(t, req)
		if !strings.HasPrefix(body.Query, "query GetOrganization(") || body.Variables["login"] != "my-org" {
			t.Fatalf("unexpected query: %+v", body)
		}
		if req.Header.Get("Authorization") != "token test-token" {
			t.Fatal("missing authentication")
		}
		return testResponse(http.StatusOK, `{"data":{"organization":{"login":"my-org","id":"org-id","name":"My Org","databaseId":42}}}`), nil
	})

	result, err := GetOrgInfo("my-org", "github.com", "test-token")
	if err != nil {
		t.Fatal(err)
	}
	if org := result.Organization; org.Login != "my-org" || org.ID != "org-id" || org.Name != "My Org" || org.DatabaseId != 42 {
		t.Fatalf("unexpected organization: %+v", org)
	}
}

func TestQueryBlobFromGitHub(t *testing.T) {
	mockHTTP(t, func(req *http.Request) (*http.Response, error) {
		body := decodeGraphQLRequest(t, req)
		if !strings.HasPrefix(body.Query, "query QueryBlob(") || body.Variables["id"] != "blob-id" || !strings.Contains(body.Query, "... on MigrationArchive") {
			t.Fatalf("unexpected query: %+v", body)
		}
		if req.Header.Get("GraphQL-Features") != "octoshift_github_owned_storage" {
			t.Fatal("missing storage feature header")
		}
		return testResponse(http.StatusOK, `{"data":{"node":{"id":"blob-id","guid":"guid","name":"archive.tar.gz","size":16,"uri":"https://storage.example/archive","createdAt":"2026-01-02T03:04:05Z"}}}`), nil
	})

	result, err := QueryBlobFromGitHub("blob-id", "github.com", "test-token")
	if err != nil {
		t.Fatal(err)
	}
	blob := result.Node.MigrationArchive
	if blob.ID != "blob-id" || blob.GUID != "guid" || blob.Name != "archive.tar.gz" || blob.Size != 16 || blob.URI != "https://storage.example/archive" || blob.CreatedAt != "2026-01-02T03:04:05Z" {
		t.Fatalf("unexpected blob: %+v", blob)
	}
}

func TestQueryAllBlobsFromGitHubPagination(t *testing.T) {
	for _, tc := range []struct {
		name     string
		bodies   []string
		wantErr  string
		wantNode string
	}{
		{
			name:   "empty organization",
			bodies: []string{`{"data":{"organization":{"migrationArchives":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}}}`},
		},
		{
			name: "multiple pages",
			bodies: []string{
				`{"data":{"organization":{"migrationArchives":{"pageInfo":{"hasNextPage":true,"endCursor":"cursor-1"},"nodes":[{"id":"first"}]}}}}`,
				`{"data":{"organization":{"migrationArchives":{"pageInfo":{"hasNextPage":false,"endCursor":"cursor-2"},"nodes":[{"id":"last"}]}}}}`,
			},
			wantNode: "last",
		},
		{
			name: "error on next page",
			bodies: []string{
				`{"data":{"organization":{"migrationArchives":{"pageInfo":{"hasNextPage":true,"endCursor":"cursor-1"},"nodes":[]}}}}`,
				`{"errors":[{"message":"access denied"}]}`,
			},
			wantErr: "failed to query GitHub API",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			mockHTTP(t, func(req *http.Request) (*http.Response, error) {
				body := decodeGraphQLRequest(t, req)
				if calls >= len(tc.bodies) {
					t.Fatal("unexpected extra page request")
				}
				if !strings.HasPrefix(body.Query, "query AllBlobs(") || body.Variables["login"] != "my-org" || body.Variables["first"] != float64(50) {
					t.Fatalf("unexpected query: %+v", body)
				}
				if calls == 0 && body.Variables["endCursor"] != nil || calls > 0 && body.Variables["endCursor"] != "cursor-1" {
					t.Fatalf("unexpected cursor on page %d: %v", calls+1, body.Variables["endCursor"])
				}
				if req.Header.Get("GraphQL-Features") != "octoshift_github_owned_storage" {
					t.Fatal("missing storage feature header")
				}
				response := testResponse(http.StatusOK, tc.bodies[calls])
				calls++
				return response, nil
			})
			result, err := QueryAllBlobsFromGitHub("my-org", "github.com", "test-token")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || result != nil {
					t.Fatalf("result = %v, error = %v; want %q", result, err, tc.wantErr)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				nodes := result.Organization.MigrationArchives.Nodes
				if tc.wantNode == "" && len(nodes) != 0 || tc.wantNode != "" && (len(nodes) != 1 || nodes[0].ID != tc.wantNode) {
					t.Fatalf("unexpected last page: %+v", nodes)
				}
			}
			if calls != len(tc.bodies) {
				t.Fatalf("calls = %d, want %d", calls, len(tc.bodies))
			}
		})
	}
}

func TestGraphQLQueryErrors(t *testing.T) {
	for _, query := range []struct {
		name string
		run  func() error
	}{
		{"organization", func() error { _, err := GetOrgInfo("org", "github.com", "test-token"); return err }},
		{"blob", func() error { _, err := QueryBlobFromGitHub("id", "github.com", "test-token"); return err }},
		{"all blobs", func() error { _, err := QueryAllBlobsFromGitHub("org", "github.com", "test-token"); return err }},
	} {
		t.Run(query.name, func(t *testing.T) {
			for _, tc := range []struct {
				name   string
				status int
				body   string
			}{
				{"HTTP failure", http.StatusUnauthorized, `{"message":"bad credentials"}`},
				{"GraphQL failure", http.StatusOK, `{"errors":[{"message":"access denied"}]}`},
				{"invalid JSON", http.StatusOK, `not JSON`},
			} {
				t.Run(tc.name, func(t *testing.T) {
					mockHTTP(t, func(*http.Request) (*http.Response, error) {
						return testResponse(tc.status, tc.body), nil
					})
					if err := query.run(); err == nil || !strings.Contains(err.Error(), "failed to query GitHub API") {
						t.Fatalf("error = %v, want query failure", err)
					}
				})
			}
		})
	}
}

func TestUploadArchiveToGitHub(t *testing.T) {
	file := testArchive(t)
	want := UploadArchiveResponse{
		GUID: "guid", NodeID: "node-id", Name: filepath.Base(file.Name()), Size: 16,
		URI: "https://storage.example/archive", CreatedAt: "2026-01-02T03:04:05Z",
	}
	for _, hostname := range []string{"github.com", "enterprise.ghe.com"} {
		t.Run(hostname, func(t *testing.T) {
			calls := 0
			mockHTTP(t, func(req *http.Request) (*http.Response, error) {
				calls++
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatal(err)
				}
				if req.URL.Host != "uploads."+hostname || req.URL.Path != "/organizations/42/gei/archive" || req.URL.Query().Get("name") != want.Name {
					t.Fatalf("unexpected upload URL: %s", req.URL)
				}
				if string(body) != "archive contents" || req.ContentLength != 16 {
					t.Fatalf("unexpected upload body %q, length %d", body, req.ContentLength)
				}
				response, err := json.Marshal(want)
				if err != nil {
					t.Fatal(err)
				}
				return testResponse(http.StatusCreated, string(response)), nil
			})
			result, err := UploadArchiveToGitHub(context.Background(), UploadArchiveInput{ArchiveFilePath: file.Name(), OrganizationId: "42"}, hostname, "test-token")
			if err != nil {
				t.Fatal(err)
			}
			if result == nil || *result != want || calls != 1 {
				t.Fatalf("result = %+v, calls = %d; want %+v and one call", result, calls, want)
			}
		})
	}
}

func TestUploadArchiveMultipartThreshold(t *testing.T) {
	for _, size := range []int64{DefaultMultipartThreshold - 1, DefaultMultipartThreshold, DefaultMultipartThreshold + 1} {
		t.Run(strconv.FormatInt(size, 10), func(t *testing.T) {
			file := testArchive(t)
			if err := file.Truncate(size); err != nil {
				t.Fatal(err)
			}
			calls := 0
			mockHTTP(t, func(req *http.Request) (*http.Response, error) {
				calls++
				wantPath := "/organizations/42/gei/archive"
				if size >= DefaultMultipartThreshold {
					wantPath += "/blobs/uploads"
					var body struct {
						Size int64 `json:"size"`
					}
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if body.Size != size {
						t.Fatalf("size = %d, want %d", body.Size, size)
					}
				}
				if req.URL.Path != wantPath {
					t.Fatalf("upload path = %q, want %q", req.URL.Path, wantPath)
				}
				return testResponse(http.StatusBadRequest, "stop before uploading large file"), nil
			})
			result, err := UploadArchiveToGitHub(context.Background(), UploadArchiveInput{ArchiveFilePath: file.Name(), OrganizationId: "42"}, "github.com", "test-token")
			if err == nil || !strings.Contains(err.Error(), "unexpected response status: 400") || result != nil || calls != 1 {
				t.Fatalf("result = %v, error = %v, calls = %d", result, err, calls)
			}
		})
	}
}

func TestUploadArchiveMissingFile(t *testing.T) {
	mockHTTP(t, func(*http.Request) (*http.Response, error) {
		t.Fatal("missing file must not make a network request")
		return nil, nil
	})
	result, err := UploadArchiveToGitHub(context.Background(), UploadArchiveInput{ArchiveFilePath: filepath.Join(t.TempDir(), "missing")}, "github.com", "test-token")
	if result != nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("result = %v, error = %v; want missing file error", result, err)
	}
}

func TestMissingUploadAndDeleteToken(t *testing.T) {
	mockHTTP(t, func(*http.Request) (*http.Response, error) {
		t.Fatal("missing token must not make a network request")
		return nil, nil
	})
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"simple upload", func() error {
			_, err := simpleUpload(context.Background(), "42", testArchive(t), 16, "github.com", "")
			return err
		}},
		{"multipart upload", func() error {
			_, err := multipartUpload(context.Background(), "42", testArchive(t), 16, "github.com", "")
			return err
		}},
		{"delete", func() error { return DeleteBlobFromGitHub("blob-id", "github.com", "") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil || !strings.Contains(err.Error(), "failed to create GitHub client") {
				t.Fatalf("error = %v, want authentication failure", err)
			}
		})
	}
}

type failingReadCloser struct{}

func (failingReadCloser) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (failingReadCloser) Close() error             { return nil }

func TestSimpleUploadResponseErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    io.ReadCloser
		wantErr string
	}{
		{"invalid JSON", io.NopCloser(strings.NewReader("invalid")), "failed to decode response"},
		{"unreadable body", failingReadCloser{}, "failed to read response body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockHTTP(t, func(*http.Request) (*http.Response, error) {
				response := testResponse(http.StatusCreated, "")
				response.Body = tc.body
				return response, nil
			})
			result, err := simpleUpload(context.Background(), "42", testArchive(t), 16, "github.com", "test-token")
			if result != nil || err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("result = %v, error = %v; want %q", result, err, tc.wantErr)
			}
		})
	}
}

func TestMultipartUpload(t *testing.T) {
	const location = "/organizations/42/gei/archive/blobs/uploads?part_number=1&guid=guid&upload_id=upload-id"
	for _, tc := range []struct {
		name         string
		hostname     string
		startStatus  int
		noLocation   bool
		partStatuses []int
		endStatuses  []int
		endBody      string
		wantErr      string
	}{
		{name: "success", hostname: "github.com"},
		{name: "data residency", hostname: "enterprise.ghe.com"},
		{name: "retry part and finalize", partStatuses: []int{502, 202}, endStatuses: []int{503, 201}},
		{name: "start failure", startStatus: 400, wantErr: "unexpected response status: 400"},
		{name: "missing location", noLocation: true, wantErr: "missing Location header"},
		{name: "part failure", partStatuses: []int{400}, wantErr: "unexpected response status for part 1: 400"},
		{name: "finalize failure", endStatuses: []int{400}, wantErr: "unexpected finalize response status: 400"},
		{name: "invalid finalize JSON", endBody: "invalid", wantErr: "failed to decode response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hostname := tc.hostname
			if hostname == "" {
				hostname = "github.com"
			}
			file := testArchive(t)
			starts, parts, ends := 0, 0, 0
			ctx := context.Background()
			mockHTTP(t, func(req *http.Request) (*http.Response, error) {
				if req.URL.Host != "uploads."+hostname || req.Context() != ctx {
					t.Fatalf("unexpected host/context: %s", req.URL)
				}
				if req.Header.Get("User-Agent") != "gh-blob" || req.Header.Get("GraphQL-Features") != "octoshift_github_owned_storage" {
					t.Fatal("missing upload headers")
				}
				switch req.Method {
				case http.MethodPost:
					starts++
					var body map[string]interface{}
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					want := map[string]interface{}{"content_type": "application/octet-stream", "name": filepath.Base(file.Name()), "size": float64(16)}
					if !reflect.DeepEqual(body, want) || req.URL.Path != "/organizations/42/gei/archive/blobs/uploads" {
						t.Fatalf("unexpected initiation: %s %+v", req.URL, body)
					}
					status := tc.startStatus
					if status == 0 {
						status = http.StatusAccepted
					}
					response := testResponse(status, "start response")
					if !tc.noLocation {
						response.Header.Set("Location", location)
					}
					return response, nil
				case http.MethodPatch:
					body, err := io.ReadAll(req.Body)
					if err != nil {
						t.Fatal(err)
					}
					if string(body) != "archive contents" || req.ContentLength != 16 || req.URL.RequestURI() != location {
						t.Fatalf("unexpected part: %s, body %q, size %d", req.URL, body, req.ContentLength)
					}
					status := http.StatusAccepted
					if len(tc.partStatuses) > 0 {
						if parts >= len(tc.partStatuses) {
							t.Fatal("too many part attempts")
						}
						status = tc.partStatuses[parts]
					}
					parts++
					response := testResponse(status, "part response")
					response.Header.Set("Location", strings.Replace(location, "part_number=1", "part_number=2", 1))
					return response, nil
				case http.MethodPut:
					if req.URL.RequestURI() != location {
						t.Fatalf("finalize URL = %s, want last uploaded location", req.URL)
					}
					status := http.StatusCreated
					if len(tc.endStatuses) > 0 {
						if ends >= len(tc.endStatuses) {
							t.Fatal("too many finalize attempts")
						}
						status = tc.endStatuses[ends]
					}
					ends++
					body := tc.endBody
					if body == "" {
						body = `{"node_id":"archive-id","guid":"guid","size":16}`
					}
					return testResponse(status, body), nil
				default:
					t.Fatalf("unexpected method %s", req.Method)
					return nil, nil
				}
			})
			result, err := multipartUpload(ctx, "42", file, 16, hostname, "test-token")
			if tc.wantErr != "" {
				if result != nil || err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("result = %v, error = %v; want %q", result, err, tc.wantErr)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if result.NodeID != "archive-id" || result.GUID != "guid" || result.Size != 16 {
					t.Fatalf("unexpected result: %+v", result)
				}
				wantParts, wantEnds := 1, 1
				if len(tc.partStatuses) > 0 {
					wantParts = len(tc.partStatuses)
				}
				if len(tc.endStatuses) > 0 {
					wantEnds = len(tc.endStatuses)
				}
				if parts != wantParts || ends != wantEnds {
					t.Fatalf("part/finalize calls = %d/%d, want %d/%d", parts, ends, wantParts, wantEnds)
				}
			}
			if starts != 1 {
				t.Fatalf("start calls = %d, want 1", starts)
			}
		})
	}
}

func TestDeleteBlobFromGitHub(t *testing.T) {
	for _, tc := range []struct {
		name     string
		hostname string
		status   int
		body     string
		wantErr  string
	}{
		{"success", "github.com", 200, `{"data":{"deleteMigrationArchive":{"migrationArchive":{"id":"blob-id"}}}}`, ""},
		{"data residency", "enterprise.ghe.com", 200, `{"data":{}}`, ""},
		{"HTTP failure", "github.com", 403, "permission denied", "unexpected response status: 403, body: permission denied"},
		{"GraphQL failure", "github.com", 200, `{"errors":[{"message":"not found"}]}`, "GraphQL error: not found"},
		{"invalid JSON", "github.com", 200, "invalid", "failed to decode response"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			mockHTTP(t, func(req *http.Request) (*http.Response, error) {
				calls++
				if req.Method != http.MethodPost || req.URL.String() != "https://api."+tc.hostname+"/graphql" {
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
				}
				var body graphQLRequest
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body.OperationName != "deleteMigrationArchive" || body.Variables["migrationArchiveId"] != "blob-id" || !strings.Contains(body.Query, "deleteMigrationArchive(") {
					t.Fatalf("unexpected mutation: %+v", body)
				}
				if req.Header.Get("GraphQL-Features") != "octoshift_github_owned_storage" || req.Header.Get("Content-Type") != "application/json" {
					t.Fatal("missing mutation headers")
				}
				return testResponse(tc.status, tc.body), nil
			})
			err := DeleteBlobFromGitHub("blob-id", tc.hostname, "test-token")
			if tc.wantErr == "" && err != nil || tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if calls != 1 {
				t.Fatalf("calls = %d, want 1", calls)
			}
		})
	}
}
