package github

import (
	"context"
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
