//go:build integration

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const radarP0PostgresImage = "postgres:18.1-alpine3.23"

type radarP0APIKeyRepo struct {
	service.APIKeyRepository
	keys map[string]*service.APIKey
}

func (r *radarP0APIKeyRepo) GetByKeyForAuth(_ context.Context, key string) (*service.APIKey, error) {
	apiKey, ok := r.keys[key]
	if !ok {
		return nil, service.ErrAPIKeyNotFound
	}
	clone := *apiKey
	return &clone, nil
}

func (r *radarP0APIKeyRepo) UpdateLastUsed(context.Context, int64, time.Time) error {
	return nil
}

func TestRadarP0EvaluationIsolationAndEvidenceLifecycle(t *testing.T) {
	db := radarP0Database(t)
	gin.SetMode(gin.TestMode)

	const (
		normalKeyValue     = "sk-radar-p0-normal"
		evaluationKeyValue = "sk-radar-p0-evaluation"
		accountID          = int64(991)
		channelID          = int64(772)
	)
	userID, groupID, normalKeyID, evaluationKeyID := provisionRadarP0Principals(t, db, normalKeyValue, evaluationKeyValue)
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
	user := &service.User{ID: userID, Role: service.RoleUser, Status: service.StatusActive, Balance: 10, Concurrency: 1}
	group := &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive}
	authRepo := &radarP0APIKeyRepo{keys: map[string]*service.APIKey{
		normalKeyValue: {
			ID: normalKeyID, UserID: user.ID, Key: normalKeyValue, GroupID: &groupID,
			Status: service.StatusActive, User: user, Group: group,
		},
		evaluationKeyValue: {
			ID: evaluationKeyID, UserID: user.ID, Key: evaluationKeyValue, GroupID: &groupID,
			Status: service.StatusActive, IsEvaluation: true, User: user, Group: group,
		},
	}}
	apiKeyService := service.NewAPIKeyService(authRepo, nil, nil, nil, nil, nil, cfg)
	evidenceRepo := repository.NewEvaluationRouteEvidenceRepository(db)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rate-limit" {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"code":"rate_limited"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"qwen3-coder-2026-07","usage":{"input_tokens":101,"output_tokens":37,"ttft_ms":123,"latency_ms":1250,"billed_amount":"0.00012345"}}`))
	}))
	t.Cleanup(upstream.Close)

	var inferenceCalls atomic.Int64
	router := gin.New()
	router.POST(
		"/v1/radar/:mode",
		gin.HandlerFunc(servermiddleware.NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg)),
		gin.HandlerFunc(servermiddleware.NewEvaluationEvidenceMiddleware(evidenceRepo)),
		func(c *gin.Context) {
			inferenceCalls.Add(1)
			ctx := service.WithCompositeRouteDecision(c.Request.Context(), service.CompositeRouteDecision{
				Matched: true, PublicModel: "public-coder", TargetPlatform: "qwen",
				UpstreamModel: "qwen3-coder-2026-07",
			})
			c.Request = c.Request.WithContext(ctx)

			switch c.Param("mode") {
			case "success", "upstream":
				trace, ok := service.RouteTraceFromContext(ctx)
				require.True(t, ok)
				attempt := service.RouteAttempt{
					Provider: "qwen", AccountID: accountID, ChannelID: channelID,
					ResolvedModel: "qwen3-coder-2026-07", Region: "cn-east",
				}
				if c.Param("mode") == "upstream" {
					attempt.ErrorCode = "rate_limited"
				}
				trace.RecordAttempt(attempt)
				path := "/success"
				if c.Param("mode") == "upstream" {
					path = "/rate-limit"
				}
				upstreamRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream.URL+path, nil)
				require.NoError(t, err)
				response, err := http.DefaultClient.Do(upstreamRequest)
				require.NoError(t, err)
				defer response.Body.Close()
				if response.StatusCode != http.StatusOK {
					c.JSON(http.StatusBadGateway, gin.H{"error": "upstream_failed"})
					return
				}
				var payload struct {
					Model string `json:"model"`
					Usage struct {
						InputTokens  int    `json:"input_tokens"`
						OutputTokens int    `json:"output_tokens"`
						TTFT         int    `json:"ttft_ms"`
						Latency      int    `json:"latency_ms"`
						BilledAmount string `json:"billed_amount"`
					} `json:"usage"`
				}
				require.NoError(t, json.NewDecoder(response.Body).Decode(&payload))
				evaluation, ok := service.EvaluationContextFromContext(ctx)
				require.True(t, ok)
				repo, ok := service.EvaluationEvidenceRepositoryFromContext(ctx)
				require.True(t, ok)
				require.NoError(t, repo.AttachBilling(ctx, evaluation.RouteTraceID, service.RouteUsageEvidence{
					InputTokens: payload.Usage.InputTokens, OutputTokens: payload.Usage.OutputTokens,
					TTFT: &payload.Usage.TTFT, Latency: &payload.Usage.Latency,
					BilledAmount: decimal.RequireFromString(payload.Usage.BilledAmount), FinishReason: "stop",
				}))
				c.JSON(http.StatusOK, gin.H{"model": payload.Model})
			case "protocol":
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				c.Request = c.Request.WithContext(cancelled)
				c.Status(499)
			case "gateway":
				c.JSON(http.StatusInternalServerError, gin.H{"error": "gateway_failed"})
			}
		},
	)

	signer, err := service.NewEvaluationContextSigner([]byte(secret), 5*time.Minute)
	require.NoError(t, err)
	copiedToken := radarP0Token(t, signer, evaluationKeyID, uuid.NewString(), uuid.NewString())

	normalResponse := radarP0Request(router, normalKeyValue, copiedToken, "success")
	require.Equal(t, http.StatusForbidden, normalResponse.Code)
	require.Zero(t, inferenceCalls.Load(), "normal keys with copied evaluation headers must not reach inference")
	requireRadarP0EvidenceCount(t, db, normalKeyID, 0)

	missingTokenResponse := radarP0Request(router, evaluationKeyValue, "", "success")
	require.Equal(t, http.StatusForbidden, missingTokenResponse.Code)
	require.Zero(t, inferenceCalls.Load(), "evaluation keys without a signed token must not reach inference")
	requireRadarP0EvidenceCount(t, db, evaluationKeyID, 0)

	terminalCases := []struct {
		mode       string
		wantStatus int
		wantClass  string
	}{
		{mode: "success", wantStatus: http.StatusOK, wantClass: "succeeded"},
		{mode: "upstream", wantStatus: http.StatusBadGateway, wantClass: "upstream_failed"},
		{mode: "protocol", wantStatus: http.StatusBadRequest, wantClass: "protocol_failed"},
		{mode: "cancelled", wantStatus: 499, wantClass: "client_cancelled"},
		{mode: "gateway", wantStatus: http.StatusInternalServerError, wantClass: "gateway_failed"},
	}
	for _, test := range terminalCases {
		t.Run(test.mode, func(t *testing.T) {
			runID := uuid.NewString()
			sampleID := uuid.NewString()
			token := radarP0Token(t, signer, evaluationKeyID, runID, sampleID)
			response := radarP0Request(router, evaluationKeyValue, token, test.mode)
			require.Equal(t, test.wantStatus, response.Code)
			assertRadarP0Evidence(t, db, runID, sampleID, evaluationKeyID, test.wantClass, test.mode == "success")
		})
	}
}

func radarP0Database(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "docker", "info")
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		t.Skip("docker is not available; skipping integration tests (start Docker to enable)")
	}

	container, err := tcpostgres.Run(
		ctx,
		radarP0PostgresImage,
		tcpostgres.WithDatabase("sub2api_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(context.Background())) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable", "TimeZone=UTC")
	require.NoError(t, err)
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.Eventually(t, func() bool { return db.PingContext(ctx) == nil }, 30*time.Second, 250*time.Millisecond)
	require.NoError(t, repository.ApplyMigrations(ctx, db))
	return db
}

func provisionRadarP0Principals(t *testing.T, db *sql.DB, normalKey, evaluationKey string) (int64, int64, int64, int64) {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.NewString()
	var groupID int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO groups (name, platform, status)
		VALUES ($1, 'openai', 'active') RETURNING id`, "radar-p0-"+suffix).Scan(&groupID))
	var userID int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO users (email, username, password_hash, role, balance, concurrency, status)
		VALUES ($1, $2, 'not-a-login-secret', 'user', 10, 1, 'active') RETURNING id`,
		"radar-p0-"+suffix+"@example.com", "radar-p0-"+suffix).Scan(&userID))
	var normalKeyID int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO api_keys (user_id, key, name, group_id, status, is_evaluation)
		VALUES ($1, $2, 'normal', $3, 'active', false) RETURNING id`,
		userID, normalKey, groupID).Scan(&normalKeyID))
	var evaluationKeyID int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO api_keys (user_id, key, name, group_id, status, is_evaluation)
		VALUES ($1, $2, 'evaluation', $3, 'active', true) RETURNING id`,
		userID, evaluationKey, groupID).Scan(&evaluationKeyID))
	return userID, groupID, normalKeyID, evaluationKeyID
}

func radarP0Token(t *testing.T, signer *service.EvaluationContextSigner, apiKeyID int64, runID, sampleID string) string {
	t.Helper()
	now := time.Now().UTC()
	token, err := signer.Sign(service.EvaluationContext{
		RunID: runID, SampleID: sampleID, DatasetVersion: "dataset-v1",
		ExpectedModelAlias: "public-coder", ExpectedRouteProfile: "route-v42",
		APIKeyID: apiKeyID, IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(2 * time.Minute),
	})
	require.NoError(t, err)
	return token
}

func radarP0Request(router http.Handler, key, token, mode string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/v1/radar/"+mode, strings.NewReader(`{"model":"public-coder"}`))
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("X-Sub2API-Evaluation-Token", token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func requireRadarP0EvidenceCount(t *testing.T, db *sql.DB, apiKeyID int64, want int) {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM evaluation_route_evidence WHERE api_key_id = $1`, apiKeyID).Scan(&count))
	require.Equal(t, want, count)
}

func assertRadarP0Evidence(t *testing.T, db *sql.DB, runID, sampleID string, apiKeyID int64, wantClass string, wantBilling bool) {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM evaluation_route_evidence WHERE evaluation_run_id = $1`, runID).Scan(&count))
	require.Equal(t, 1, count, "one server-generated trace must produce one row")

	var (
		storedSampleID, requestedModel, resolvedModel, routeProfile, status string
		fallbackJSON                                                        []byte
		inputTokens, outputTokens, ttft, latency                            sql.NullInt64
		billedAmount                                                        sql.NullString
	)
	require.NoError(t, db.QueryRow(`
		SELECT sample_id::text, requested_model, COALESCE(resolved_model, ''),
			route_profile_version, transport_status, fallback_chain,
			input_tokens, output_tokens, ttft_ms, latency_ms, billed_amount::text
		FROM evaluation_route_evidence
		WHERE evaluation_run_id = $1 AND api_key_id = $2`, runID, apiKeyID).Scan(
		&storedSampleID, &requestedModel, &resolvedModel, &routeProfile, &status, &fallbackJSON,
		&inputTokens, &outputTokens, &ttft, &latency, &billedAmount,
	))
	require.Equal(t, sampleID, storedSampleID)
	require.Equal(t, "public-coder", requestedModel)
	require.Equal(t, "qwen3-coder-2026-07", resolvedModel)
	require.Equal(t, "route-v42", routeProfile)
	require.Equal(t, wantClass, status)
	require.NotContains(t, string(fallbackJSON), "account_id")
	require.NotContains(t, string(fallbackJSON), "channel_id")
	var fallback []service.RouteFallbackEntry
	require.NoError(t, json.Unmarshal(fallbackJSON, &fallback))
	for _, attempt := range fallback {
		require.NotEqual(t, "991", attempt.AccountPoolRef)
		require.NotEqual(t, "772", attempt.ChannelRef)
	}
	if wantBilling {
		require.Equal(t, int64(101), inputTokens.Int64)
		require.Equal(t, int64(37), outputTokens.Int64)
		require.Equal(t, int64(123), ttft.Int64)
		require.Equal(t, int64(1250), latency.Int64)
		require.Equal(t, "0.00012345", billedAmount.String)
	}
}
