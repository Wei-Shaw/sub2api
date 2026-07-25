//go:build unit

package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const evaluationTokenHeader = "X-Sub2API-Evaluation-Token"

func TestAPIKeyAuthenticationEnforcesEvaluationContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, protocol := range []string{"default", "google"} {
		t.Run(protocol, func(t *testing.T) {
			tests := []struct {
				name         string
				isEvaluation bool
				headerName   string
				headerValue  string
				validToken   bool
				wantStatus   int
				wantCode     string
				wantContext  bool
			}{
				{name: "normal key without evaluation header", wantStatus: http.StatusOK},
				{
					name:        "normal key with evaluation header",
					headerName:  "X-Sub2API-Evaluation-Forged",
					headerValue: "untrusted",
					wantStatus:  http.StatusForbidden,
					wantCode:    "EVALUATION_CONTEXT_FORBIDDEN",
				},
				{
					name:         "evaluation key without token",
					isEvaluation: true,
					wantStatus:   http.StatusForbidden,
					wantCode:     "EVALUATION_CONTEXT_REQUIRED",
				},
				{
					name:         "evaluation key with invalid token",
					isEvaluation: true,
					headerName:   evaluationTokenHeader,
					headerValue:  "invalid-token",
					wantStatus:   http.StatusForbidden,
					wantCode:     "EVALUATION_CONTEXT_INVALID",
				},
				{
					name:         "evaluation key with valid token",
					isEvaluation: true,
					validToken:   true,
					wantStatus:   http.StatusOK,
					wantContext:  true,
				},
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					result := runEvaluationAuthRequest(t, protocol, test.isEvaluation, test.headerName, test.headerValue, test.validToken)
					require.Equal(t, test.wantStatus, result.response.Code)
					require.Equal(t, test.wantContext, result.hasContext)
					require.Empty(t, result.response.Header().Get("X-Sub2API-Route-Trace-ID"))

					if test.wantContext {
						require.Equal(t, testEvaluationRunID, result.evaluationContext.RunID)
						require.NotEmpty(t, result.evaluationContext.RouteTraceID)
					}
					if test.wantCode != "" {
						requireEvaluationAuthErrorCode(t, protocol, result.response, test.wantCode)
					}
				})
			}
		})
	}
}

const (
	testEvaluationRunID    = "018f4f20-3d12-7e50-9000-000000000001"
	testEvaluationSampleID = "018f4f20-3d12-7e50-9000-000000000002"
)

type evaluationAuthResult struct {
	response          *httptest.ResponseRecorder
	evaluationContext service.EvaluationContext
	hasContext        bool
}

func runEvaluationAuthRequest(t *testing.T, protocol string, isEvaluation bool, headerName, headerValue string, validToken bool) evaluationAuthResult {
	t.Helper()

	const apiKeyValue = "sk-evaluation-auth"
	const apiKeyID int64 = 41
	secret := strings.Repeat("s", 32)
	cfg := &config.Config{
		RunMode: config.RunModeSimple,
		Radar: config.RadarConfig{
			Enabled:              true,
			SigningSecret:        secret,
			HashingSecret:        strings.Repeat("h", 32),
			MaxContextTTLSeconds: 300,
			Region:               "cn-east",
			RouteProfileVersion:  "route-v42",
		},
	}
	user := &service.User{ID: 7, Role: service.RoleUser, Status: service.StatusActive, Balance: 10, Concurrency: 1}
	repo := &stubApiKeyRepo{getByKey: func(_ context.Context, key string) (*service.APIKey, error) {
		if key != apiKeyValue {
			return nil, service.ErrAPIKeyNotFound
		}
		return &service.APIKey{
			ID:           apiKeyID,
			UserID:       user.ID,
			Key:          apiKeyValue,
			Status:       service.StatusActive,
			IsEvaluation: isEvaluation,
			User:         user,
		}, nil
	}}
	apiKeyService := service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, cfg)

	result := evaluationAuthResult{response: httptest.NewRecorder()}
	router := gin.New()
	if protocol == "google" {
		router.Use(APIKeyAuthWithSubscriptionGoogle(apiKeyService, nil, cfg))
	} else {
		router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)))
	}
	router.GET("/test", func(c *gin.Context) {
		result.evaluationContext, result.hasContext = service.EvaluationContextFromContext(c.Request.Context())
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	if protocol == "google" {
		req.Header.Set("x-goog-api-key", apiKeyValue)
	} else {
		req.Header.Set("x-api-key", apiKeyValue)
	}
	if validToken {
		signer, err := service.NewEvaluationContextSigner([]byte(secret), 5*time.Minute)
		require.NoError(t, err)
		issuedAt := time.Now().UTC().Add(-time.Minute)
		token, err := signer.Sign(service.EvaluationContext{
			RunID:                testEvaluationRunID,
			SampleID:             testEvaluationSampleID,
			DatasetVersion:       "core-v1",
			ExpectedModelAlias:   "qwen3-coder",
			ExpectedRouteProfile: "route-v42",
			APIKeyID:             apiKeyID,
			IssuedAt:             issuedAt,
			ExpiresAt:            issuedAt.Add(5 * time.Minute),
		})
		require.NoError(t, err)
		headerName = evaluationTokenHeader
		headerValue = token
	}
	if headerName != "" {
		req.Header.Set(headerName, headerValue)
	}
	router.ServeHTTP(result.response, req)
	return result
}

func requireEvaluationAuthErrorCode(t *testing.T, protocol string, response *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	if protocol == "google" {
		var body googleErrorResponse
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
		require.Contains(t, body.Error.Message, wantCode)
		return
	}
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Equal(t, wantCode, body.Code)
}
