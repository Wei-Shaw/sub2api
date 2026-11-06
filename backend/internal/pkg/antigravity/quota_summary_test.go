//go:build unit

package antigravity

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type quotaTransport func(*http.Request) (*http.Response, error)

func (f quotaTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestRetrieveUserQuotaSummary(t *testing.T) {
	for _, tc := range []struct {
		name      string
		statuses  []int
		body      string
		limit     int64
		wantError bool
	}{
		{"production", []int{200}, `{"groups":[]}`, 1024, false},
		{"fallback on 404", []int{404, 200}, `{"groups":[]}`, 1024, false},
		{"no retry on 401", []int{401}, `{"error":"secret"}`, 1024, true},
		{"no retry on 429", []int{429}, `{"error":"secret"}`, 1024, true},
		{"no redirect", []int{302}, `{"groups":[]}`, 1024, true},
		{"no groups", []int{200}, `{}`, 1024, true},
		{"bounded response", []int{200}, `{"groups":[]}`, 5, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := &Client{httpClient: &http.Client{Transport: quotaTransport(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, "/v1internal:retrieveUserQuotaSummary", req.URL.Path)
				require.Equal(t, "https", req.URL.Scheme)
				require.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
				if calls == 0 {
					require.Equal(t, "cloudcode-pa.googleapis.com", req.URL.Host)
				}
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.JSONEq(t, `{}`, string(body))
				require.Less(t, calls, len(tc.statuses))
				status := tc.statuses[calls]
				calls++
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}}
			_, err := client.RetrieveUserQuotaSummary(context.Background(), "test-token", "", tc.limit)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, len(tc.statuses), calls)
		})
	}
}

func TestQuotaSummaryClientMetadata(t *testing.T) {
	client := &Client{httpClient: &http.Client{Transport: quotaTransport(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"project":"project-id"}`, string(body))
		require.Contains(t, req.Header.Get("User-Agent"), "hub/2.24.0 ")
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"groups":[]}`)), Header: make(http.Header)}, nil
	})}}
	_, err := client.RetrieveUserQuotaSummary(context.Background(), "token", "project-id", 1024)
	require.NoError(t, err)
	withTestUserAgentVersionResolver(t)
	ctx := context.WithValue(context.Background(), userAgentVersionTestContextKey{}, "3.4.5")
	require.Contains(t, quotaSummaryUserAgent(ctx), "hub/3.4.5 ")
}
