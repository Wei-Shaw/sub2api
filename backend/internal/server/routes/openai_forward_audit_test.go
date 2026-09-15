package routes

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/forwardaudit"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIForwardAuditCapturesClientBodyBeforeCompositeRewrite(t *testing.T) {
	gin.SetMode(gin.TestMode)

	directory := t.TempDir()
	recorder := forwardaudit.NewRecorder(forwardaudit.Options{Enabled: true, Directory: directory, QueueSize: 16})
	t.Cleanup(recorder.Close)

	resolver := service.NewCompositeRouteResolver(compositeRouteRepoStub{routes: []service.CompositeModelRoute{
		{
			ID:             1,
			GroupID:        9,
			PublicModel:    "openai-alias",
			MatchType:      service.CompositeRouteMatchExact,
			TargetPlatform: service.PlatformOpenAI,
			UpstreamModel:  "gpt-5.1",
			Endpoint:       service.CompositeRouteEndpointAny,
			Priority:       100,
			Enabled:        true,
		},
	}})

	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(9)
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
			ID:      12,
			UserID:  34,
			GroupID: &groupID,
			User:    &service.User{ID: 34},
			Group:   &service.Group{ID: groupID, Platform: service.PlatformComposite},
		})
		c.Next()
	})
	// The audit trace must be attached immediately after authentication and before
	// compositeTargetPlatformMiddleware mutates the request body.
	router.Use(openAIForwardAuditMiddleware(recorder))
	router.Use(compositeTargetPlatformMiddleware(resolver))
	router.POST("/v1/responses", func(c *gin.Context) {
		rewrittenBody, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"model":"gpt-5.1","input":"hello"}`, string(rewrittenBody))

		upstreamRequest, err := http.NewRequestWithContext(
			c.Request.Context(),
			http.MethodPost,
			"https://api.openai.example/v1/responses",
			bytes.NewReader(rewrittenBody),
		)
		require.NoError(t, err)
		upstreamRequest = forwardaudit.ActivateRequest(upstreamRequest)
		response, err := forwardaudit.WrapTransport(routeAuditRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			_, _ = io.Copy(io.Discard, request.Body)
			_ = request.Body.Close()
			return &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"id":"resp_1"}`)),
			}, nil
		}), 56).RoundTrip(upstreamRequest)
		require.NoError(t, err)
		_, err = io.Copy(io.Discard, response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/responses",
		bytes.NewBufferString(`{"model":"openai-alias","input":"hello"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Session-Id", "openai-session")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code)

	recorder.Close()
	records := readRouteAuditRecords(t, directory)
	require.Len(t, records, 3)
	require.JSONEq(t, `{"model":"openai-alias","input":"hello"}`, routeAuditBody(t, routeAuditRecordByStage(t, records, "client_request")))
	require.JSONEq(t, `{"model":"gpt-5.1","input":"hello"}`, routeAuditBody(t, routeAuditRecordByStage(t, records, "upstream_request")))
}

func TestOpenAIForwardAuditDormantTraceDoesNotRecordWithoutFinalActivation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	directory := t.TempDir()
	recorder := forwardaudit.NewRecorder(forwardaudit.Options{Enabled: true, Directory: directory, QueueSize: 16})
	t.Cleanup(recorder.Close)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
			ID:     12,
			UserID: 34,
			User:   &service.User{ID: 34},
			Group:  &service.Group{Platform: service.PlatformGrok},
		})
		c.Next()
	})
	router.Use(openAIForwardAuditMiddleware(recorder))
	router.POST("/v1/responses", func(c *gin.Context) {
		_, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		upstreamRequest, err := http.NewRequestWithContext(
			c.Request.Context(), http.MethodPost, "https://api.x.ai/v1/responses", strings.NewReader(`{"model":"grok-4"}`),
		)
		require.NoError(t, err)
		// Even an accidental activation downstream must be a no-op because the
		// ingress middleware must not attach traces to non-OpenAI/non-Composite groups.
		upstreamRequest = forwardaudit.ActivateRequest(upstreamRequest)
		response, err := forwardaudit.WrapTransport(routeAuditRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			_, _ = io.Copy(io.Discard, request.Body)
			_ = request.Body.Close()
			return &http.Response{Status: "200 OK", StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}, nil
		}), 56).RoundTrip(upstreamRequest)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"grok-4"}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code)

	recorder.Close()
	require.Empty(t, readRouteAuditRecords(t, directory))
}

type routeAuditRoundTripFunc func(*http.Request) (*http.Response, error)

func (f routeAuditRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func readRouteAuditRecords(t *testing.T, directory string) []map[string]any {
	t.Helper()
	var records []map[string]any
	require.NoError(t, filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range bytes.Split(bytes.TrimSpace(content), []byte("\n")) {
			if len(line) == 0 {
				continue
			}
			var record map[string]any
			if err := json.Unmarshal(line, &record); err != nil {
				return err
			}
			records = append(records, record)
		}
		return nil
	}))
	return records
}

func routeAuditRecordByStage(t *testing.T, records []map[string]any, stage string) map[string]any {
	t.Helper()
	for _, record := range records {
		if record["stage"] == stage {
			return record
		}
	}
	t.Fatalf("missing audit stage %q in %#v", stage, records)
	return nil
}

func routeAuditBody(t *testing.T, record map[string]any) string {
	t.Helper()
	body, ok := record["body"].(map[string]any)
	require.True(t, ok)
	content, ok := body["content"].(string)
	require.True(t, ok)
	return content
}
