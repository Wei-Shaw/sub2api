package typesafe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEvaluateContract(t *testing.T) {
	questions := map[string]Question{"sexual": {Type: "noul", Instructions: "Evaluate sexual content"}, "illicit": {Type: "noul", Instructions: "Evaluate illegal assistance"}}
	for _, tc := range []struct {
		name, response string
		valid          bool
	}{
		{"valid", `{"model":"jev-1.13.0","answers":{"sexual":{"type":"noul","noul":0},"illicit":{"type":"noul","noul":1}}}`, true},
		{"missing_answer", `{"model":"jev-1.13.0","answers":{"sexual":{"type":"noul","noul":0}}}`, false},
		{"missing_probability", `{"model":"jev-1.13.0","answers":{"sexual":{"type":"noul"},"illicit":{"type":"noul","noul":0}}}`, false},
		{"negative", `{"model":"jev-1.13.0","answers":{"sexual":{"type":"noul","noul":-0.1},"illicit":{"type":"noul","noul":0}}}`, false},
		{"above_one", `{"model":"jev-1.13.0","answers":{"sexual":{"type":"noul","noul":1.1},"illicit":{"type":"noul","noul":0}}}`, false},
		{"wrong_type", `{"model":"jev-1.13.0","answers":{"sexual":{"type":"score","noul":0},"illicit":{"type":"noul","noul":0}}}`, false},
		{"missing_model", `{"answers":{"sexual":{"type":"noul","noul":0},"illicit":{"type":"noul","noul":0}}}`, false},
		{"invalid_json", `{bad`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/v1/systemone", r.URL.Path)
				require.Equal(t, "Bearer test-secret", r.Header.Get("Authorization"))
				var input Request
				require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
				require.Equal(t, questions, input.Questions)
				require.Equal(t, "sample", input.State)
				_, err := w.Write([]byte(tc.response))
				require.NoError(t, err)
			}))
			defer server.Close()
			result, status, err := Evaluate(context.Background(), server.Client(), server.URL, "test-secret", Request{Model: "jev-latest", State: "sample", Questions: questions})
			require.Equal(t, 200, status)
			if tc.valid {
				require.NoError(t, err)
				require.Equal(t, "jev-1.13.0", result.Model)
				require.Equal(t, 1.0, result.Scores["illicit"])
			} else {
				require.Error(t, err)
				require.Nil(t, result)
			}
		})
	}
}

func TestEvaluateHTTPAndTimeoutDoNotExposePayload(t *testing.T) {
	for _, status := range []int{401, 429, 500, 529} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, err := w.Write([]byte("private request text and test-secret"))
			require.NoError(t, err)
		}))
		_, code, err := Evaluate(context.Background(), server.Client(), server.URL, "test-secret", Request{})
		require.Equal(t, status, code)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "private")
		require.NotContains(t, err.Error(), "test-secret")
		server.Close()
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, _, err := Evaluate(ctx, server.Client(), server.URL, "test-secret", Request{})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.False(t, strings.Contains(err.Error(), "test-secret"))
}
