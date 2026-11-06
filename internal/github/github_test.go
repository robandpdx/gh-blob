package github

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
	"go.uber.org/zap"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSimpleUpload(t *testing.T) {
	originalTransport := http.DefaultTransport
	originalDelay := retryBaseDelay
	originalLogger := ghlog.Logger
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
		retryBaseDelay = originalDelay
		ghlog.Logger = originalLogger
	})
	ghlog.Logger = zap.NewNop()
	retryBaseDelay = time.Millisecond

	cases := []struct {
		name         string
		statuses     []int
		networkError bool
		wantCalls    int
		wantError    string
	}{
		{name: "success", statuses: []int{201}, wantCalls: 1},
		{name: "transient responses", statuses: []int{502, 429, 201}, wantCalls: 3},
		{name: "network error", statuses: []int{0, 201}, networkError: true, wantCalls: 2},
		{name: "nonretryable response", statuses: []int{400}, wantCalls: 1, wantError: "unexpected response status: 400"},
		{name: "exhausted retries", statuses: []int{503, 503, 503, 503, 503}, wantCalls: 5, wantError: "unexpected response status: 503"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, err := os.CreateTemp(t.TempDir(), "archive-*.tar.gz")
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			const payload = "archive contents"
			if _, err := file.WriteString("prefix" + payload); err != nil {
				t.Fatal(err)
			}
			if _, err := file.Seek(int64(len("prefix")), io.SeekStart); err != nil {
				t.Fatal(err)
			}

			calls := 0
			ctx := context.Background()
			http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatal(err)
				}
				if err := req.Body.Close(); err != nil {
					t.Fatal(err)
				}
				if string(body) != payload {
					t.Fatalf("attempt %d body = %q, want %q", calls+1, body, payload)
				}
				if req.Method != http.MethodPost || req.ContentLength != int64(len(payload)) {
					t.Fatalf("unexpected method/length: %s/%d", req.Method, req.ContentLength)
				}
				if req.URL.Host != "uploads.github.com" || req.URL.Path != "/organizations/org-id/gei/archive" || req.URL.Query().Get("name") != filepath.Base(file.Name()) {
					t.Fatalf("unexpected upload URL: %s", req.URL)
				}
				if req.Header.Get("Content-Type") != "application/octet-stream" || req.Header.Get("User-Agent") != "gh-blob" || req.Header.Get("Authorization") != "Bearer test-token" {
					t.Fatal("unexpected request headers")
				}
				if req.Context() != ctx {
					t.Fatal("request did not preserve context")
				}
				calls++
				if tc.networkError && calls == 1 {
					return nil, errors.New("connection reset")
				}
				return &http.Response{
					StatusCode: tc.statuses[calls-1],
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"node_id":"archive-id"}`)),
				}, nil
			})

			result, err := simpleUpload(ctx, "org-id", file, int64(len(payload)), "github.com", "test-token")
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want %q", err, tc.wantError)
				}
			} else if err != nil {
				t.Fatal(err)
			} else if result.NodeID != "archive-id" {
				t.Fatalf("unexpected upload result: %+v", result)
			}
			if calls != tc.wantCalls {
				t.Fatalf("calls = %d, want %d", calls, tc.wantCalls)
			}
		})
	}
}
