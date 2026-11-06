package websearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGoogleProvider_Search(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "/v1/responses", r.URL.Path)
		require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		resp := `{
			"id": "resp_123",
			"output": [
				{
					"type": "message",
					"role": "assistant",
					"content": [
						{
							"type": "output_text",
							"text": "今天是 2026年11月6日。\n\n---\nWeb search queries: current date\n\nSources:\n[1] [datetoday.net](https://example.com/date)\n[2] [time.gov](https://example.com/time)"
						}
					]
				}
			]
		}`
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(resp))
	}))
	defer server.Close()

	provider := NewGoogleProvider(server.URL, "test-token", server.Client())
	require.Equal(t, "google", provider.Name())

	res, err := provider.Search(context.Background(), SearchRequest{Query: "今天日期"})
	require.NoError(t, err)
	require.Equal(t, "今天日期", res.Query)
	require.Len(t, res.Results, 2)
	require.Equal(t, "datetoday.net", res.Results[0].Title)
	require.Equal(t, "https://example.com/date", res.Results[0].URL)
	require.Contains(t, res.Results[0].Snippet, "今天是 2026年11月6日。")
	require.Equal(t, "time.gov", res.Results[1].Title)
	require.Equal(t, "https://example.com/time", res.Results[1].URL)
}
