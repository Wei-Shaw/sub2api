package websearch

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// hostStatusClient answers every request with the status and body configured for its host.
func hostStatusClient(byHost map[string]struct {
	status int
	body   string
}) *http.Client {
	return &http.Client{Transport: exaRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		reply := byHost[r.URL.Host]
		return &http.Response{
			StatusCode: reply.status,
			Body:       io.NopCloser(strings.NewReader(reply.body)),
			Header:     make(http.Header),
		}, nil
	})}
}

func TestManager_TestSearch_ReportsEveryProviderFailure(t *testing.T) {
	m := NewManager([]ProviderConfig{
		{Type: ProviderTypeTavily, APIKey: "tvly-bad"},
		{Type: ProviderTypeExa, APIKey: "exa-bad"},
	}, nil)
	m.clientCache[""] = hostStatusClient(map[string]struct {
		status int
		body   string
	}{
		"api.tavily.com": {http.StatusUnauthorized, `{"detail":{"error":"Unauthorized: missing or invalid API key."}}`},
		"api.exa.ai":     {http.StatusPaymentRequired, `{"error":"credits exhausted"}`},
	})

	_, _, err := m.TestSearch(context.Background(), SearchRequest{Query: "golang"})

	require.Error(t, err)
	msg := err.Error()
	require.Contains(t, msg, "all providers failed")
	require.Contains(t, msg, "tavily: status 401")
	require.Contains(t, msg, "invalid API key")
	require.Contains(t, msg, "exa: status 402")
	require.Contains(t, msg, "credits exhausted")
}

func TestManager_TestSearch_FallsThroughToWorkingProvider(t *testing.T) {
	m := NewManager([]ProviderConfig{
		{Type: ProviderTypeTavily, APIKey: "tvly-bad"},
		{Type: ProviderTypeExa, APIKey: "exa-good"},
	}, nil)
	m.clientCache[""] = hostStatusClient(map[string]struct {
		status int
		body   string
	}{
		"api.tavily.com": {http.StatusUnauthorized, `{"detail":{"error":"Unauthorized"}}`},
		"api.exa.ai":     {http.StatusOK, `{"results":[{"url":"https://go.dev","title":"Go","highlights":["Go"]}]}`},
	})

	resp, provider, err := m.TestSearch(context.Background(), SearchRequest{Query: "golang"})

	require.NoError(t, err)
	require.Equal(t, ProviderTypeExa, provider)
	require.Len(t, resp.Results, 1)
}

func TestManager_TestSearch_NoUsableProviderKeepsMessage(t *testing.T) {
	m := NewManager([]ProviderConfig{{Type: ProviderTypeTavily, APIKey: ""}}, nil)

	_, _, err := m.TestSearch(context.Background(), SearchRequest{Query: "golang"})

	require.EqualError(t, err, "websearch: no available provider")
}
