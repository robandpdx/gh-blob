package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ghlog "github.com/robandpdx/gh-blob/pkg/logger"
	"go.uber.org/zap"
)

func TestDoWithRetry(t *testing.T) {
	originalLogger, originalDelay := ghlog.Logger, retryBaseDelay
	t.Cleanup(func() {
		ghlog.Logger, retryBaseDelay = originalLogger, originalDelay
	})
	ghlog.Logger = zap.NewNop()
	retryBaseDelay = time.Millisecond

	cases := []struct {
		name     string
		statuses []int
		want     int
		calls    int
	}{
		{"recovers after 502s", []int{502, 502, 202}, 202, 3},
		{"no retry on 4xx", []int{400, 202}, 400, 1},
		{"gives up after max attempts", []int{502, 502, 502, 502, 502, 202}, 502, 5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var calls int
			var bodies []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				bodies = append(bodies, string(b))
				w.WriteHeader(tc.statuses[calls])
				if tc.statuses[calls] == 502 {
					w.Write([]byte(`{"message":"Error creating policy"}`))
				}
				calls++
			}))
			defer srv.Close()

			resp, err := doWithRetry(context.Background(), srv.Client(), "part 129", http.StatusAccepted, func() (*http.Request, error) {
				return http.NewRequest("PATCH", srv.URL, strings.NewReader("part-bytes"))
			})
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != tc.want || calls != tc.calls {
				t.Fatalf("status %d calls %d, want %d/%d", resp.StatusCode, calls, tc.want, tc.calls)
			}
			for _, b := range bodies {
				if b != "part-bytes" {
					t.Fatalf("retry sent body %q, want full part", b)
				}
			}
		})
	}
}

func TestDoWithRetryRequestError(t *testing.T) {
	wantErr := errors.New("cannot construct request")
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("request construction failure must not reach transport")
		return nil, nil
	})}
	calls := 0
	response, err := doWithRetry(context.Background(), client, "test", http.StatusCreated, func() (*http.Request, error) {
		calls++
		return nil, wantErr
	})
	if response != nil || !errors.Is(err, wantErr) || calls != 1 {
		t.Fatalf("response = %v, error = %v, calls = %d; want request error without retries", response, err, calls)
	}
}

func TestDoWithRetryNetworkErrorExhaustion(t *testing.T) {
	mockHTTP(t, func(*http.Request) (*http.Response, error) {
		t.Fatal("unexpected use of default transport")
		return nil, nil
	})
	wantErr := errors.New("connection reset")
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		req.Body.Close()
		return nil, wantErr
	})}
	response, err := doWithRetry(context.Background(), client, "test", http.StatusCreated, func() (*http.Request, error) {
		return http.NewRequest(http.MethodPost, "https://uploads.example.com", strings.NewReader("archive"))
	})
	if response != nil || !errors.Is(err, wantErr) || calls != maxUploadAttempts {
		t.Fatalf("response = %v, error = %v, calls = %d; want network error after %d attempts", response, err, calls, maxUploadAttempts)
	}
}

func TestDoWithRetryCancellation(t *testing.T) {
	mockHTTP(t, func(*http.Request) (*http.Response, error) {
		t.Fatal("unexpected use of default transport")
		return nil, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		cancel()
		return testResponse(http.StatusServiceUnavailable, "unavailable"), nil
	})}
	response, err := doWithRetry(ctx, client, "test", http.StatusCreated, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodPost, "https://uploads.example.com", nil)
	})
	if response != nil || !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("response = %v, error = %v, calls = %d; want cancellation without retry", response, err, calls)
	}
}
