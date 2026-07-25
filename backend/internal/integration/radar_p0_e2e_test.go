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
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/server/routes"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

const (
	radarP0PostgresImage     = "postgres:18.1-alpine3.23"
	radarP0RequestedModel    = "public-coder"
	radarP0UpstreamModel     = "claude-sonnet-4"
	radarP0UpstreamRequestID = "upstream-radar-request-123"
	radarP0AccountID         = int64(991)
	radarP0ChannelID         = int64(772)
)

type radarP0TestDatabase struct {
	SQL *sql.DB
	Ent *dbent.Client
}

type radarP0Fixture struct {
	router          http.Handler
	signer          *service.EvaluationContextSigner
	normalKey       string
	evaluationKey   string
	normalKeyID     int64
	evaluationKeyID int64
}

type radarP0Evidence struct {
	RouteTraceID   string
	RunID          string
	SampleID       string
	APIKeyID       int64
	RequestID      string
	RequestedModel string
	ResolvedModel  string
	RouteProfile   string
	Provider       string
	ChannelRef     string
	AccountRef     string
	Region         string
	Attempts       int
	FallbackJSON   []byte
	Status         string
	ErrorCode      string
	InputTokens    sql.NullInt64
	OutputTokens   sql.NullInt64
	TTFT           sql.NullInt64
	Latency        sql.NullInt64
	BilledAmount   sql.NullString
}

func TestRadarP0EvaluationIsolationAndEvidenceLifecycle(t *testing.T) {
	db := radarP0Database(t)
	gin.SetMode(gin.TestMode)

	var upstreamCalls atomic.Int64
	upstream := httptest.NewServer(radarP0AnthropicUpstream(&upstreamCalls))
	t.Cleanup(upstream.Close)

	fixture := newRadarP0Fixture(t, db, upstream.URL)
	copiedToken := radarP0Token(t, fixture.signer, fixture.evaluationKeyID, uuid.NewString(), uuid.NewString())

	normalResponse := radarP0Request(fixture.router, fixture.normalKey, copiedToken)
	require.Equal(t, http.StatusForbidden, normalResponse.Code)
	require.Zero(t, upstreamCalls.Load(), "normal keys with copied evaluation headers must not reach inference")
	requireRadarP0EvidenceCount(t, db.SQL, fixture.normalKeyID, 0)

	missingTokenResponse := radarP0Request(fixture.router, fixture.evaluationKey, "")
	require.Equal(t, http.StatusForbidden, missingTokenResponse.Code)
	require.Zero(t, upstreamCalls.Load(), "evaluation keys without a signed token must not reach inference")
	requireRadarP0EvidenceCount(t, db.SQL, fixture.evaluationKeyID, 0)

	runID := uuid.NewString()
	sampleID := uuid.NewString()
	validToken := radarP0Token(t, fixture.signer, fixture.evaluationKeyID, runID, sampleID)
	response := radarP0Request(fixture.router, fixture.evaluationKey, validToken)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, int64(1), upstreamCalls.Load(), "valid evaluation requests must traverse the real upstream transport")

	var responseBody struct {
		Model string `json:"model"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &responseBody))
	require.Equal(t, radarP0UpstreamModel, responseBody.Model)

	clientRequestID := response.Header().Get("X-Client-Request-ID")
	require.NotEmpty(t, clientRequestID)
	evidence := loadRadarP0Evidence(t, db.SQL, runID, fixture.evaluationKeyID)
	assertRadarP0Evidence(t, evidence, runID, sampleID, fixture.evaluationKeyID, clientRequestID)
	assertRadarP0UsageMatchesEvidence(t, db.SQL, fixture.evaluationKeyID, evidence)
}

func radarP0AnthropicUpstream(calls *atomic.Int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" || r.URL.Query().Get("beta") != "true" {
			http.Error(w, "unexpected upstream request", http.StatusBadRequest)
			return
		}
		if r.Header.Get("x-api-key") != "radar-upstream-key" {
			http.Error(w, "unexpected upstream credentials", http.StatusUnauthorized)
			return
		}

		var requestBody struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil || requestBody.Model != radarP0UpstreamModel {
			http.Error(w, "unexpected upstream model", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-request-id", radarP0UpstreamRequestID)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          "msg_radar_p0",
			"type":        "message",
			"role":        "assistant",
			"model":       radarP0UpstreamModel,
			"content":     []map[string]any{{"type": "text", "text": "ok"}},
			"stop_reason": "end_turn",
			"usage": map[string]any{
				"input_tokens":  101,
				"output_tokens": 37,
			},
		})
	})
}

func newRadarP0Fixture(
	t *testing.T,
	db *radarP0TestDatabase,
	upstreamURL string,
) *radarP0Fixture {
	t.Helper()

	const (
		normalKeyValue     = "sk-radar-p0-normal"
		evaluationKeyValue = "sk-radar-p0-evaluation"
	)
	userID, groupID, normalKeyID, evaluationKeyID := provisionRadarP0Principals(
		t,
		db.SQL,
		upstreamURL,
		normalKeyValue,
		evaluationKeyValue,
	)

	cfg := &config.Config{
		RunMode: config.RunModeStandard,
		Default: config.DefaultConfig{RateMultiplier: 1},
		Gateway: config.GatewayConfig{
			MaxBodySize:        1 << 20,
			TextMaxBodySize:    1 << 20,
			MaxAccountSwitches: 1,
		},
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:           false,
				AllowPrivateHosts: true,
				AllowInsecureHTTP: true,
			},
		},
		Radar: config.RadarConfig{
			Enabled:              true,
			SigningSecret:        strings.Repeat("s", 32),
			HashingSecret:        strings.Repeat("h", 32),
			MaxContextTTLSeconds: 300,
			Region:               "cn-east",
			RouteProfileVersion:  "route-v42",
		},
	}

	apiKeyRepo := repository.NewAPIKeyRepository(db.Ent, db.SQL)
	userRepo := repository.NewUserRepository(db.Ent, db.SQL)
	groupRepo := repository.NewGroupRepository(db.Ent, db.SQL)
	accountRepo := repository.NewAccountRepository(db.Ent, db.SQL, nil)
	userSubRepo := repository.NewUserSubscriptionRepository(db.Ent)
	userGroupRateRepo := repository.NewUserGroupRateRepository(db.SQL)
	usageLogRepo := repository.NewUsageLogRepository(db.Ent, db.SQL)
	usageBillingRepo := repository.NewUsageBillingRepository(db.Ent, db.SQL)
	channelRepo := repository.NewChannelRepository(db.SQL)
	evidenceRepo := repository.NewEvaluationRouteEvidenceRepository(db.SQL)

	apiKeyService := service.NewAPIKeyService(
		apiKeyRepo,
		userRepo,
		groupRepo,
		userSubRepo,
		userGroupRateRepo,
		nil,
		cfg,
	)
	channelService := service.NewChannelService(channelRepo, groupRepo, apiKeyService, nil)
	concurrencyService := service.NewConcurrencyService(nil)
	billingCacheService := service.NewBillingCacheService(
		nil,
		userRepo,
		userSubRepo,
		apiKeyRepo,
		nil,
		userGroupRateRepo,
		cfg,
		nil,
	)
	t.Cleanup(billingCacheService.Stop)
	billingService := service.NewBillingService(cfg, nil)
	rateLimitService := service.NewRateLimitService(accountRepo, usageLogRepo, cfg, nil, nil)
	deferredService := service.NewDeferredService(accountRepo, nil, time.Minute)
	gatewayService := service.NewGatewayService(
		accountRepo,
		groupRepo,
		usageLogRepo,
		usageBillingRepo,
		userRepo,
		userSubRepo,
		userGroupRateRepo,
		nil,
		cfg,
		nil,
		concurrencyService,
		billingService,
		rateLimitService,
		billingCacheService,
		nil,
		repository.NewHTTPUpstream(cfg),
		deferredService,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		channelService,
		nil,
		nil,
		nil,
		nil,
	)
	gatewayHandler := handler.NewGatewayHandler(
		gatewayService,
		nil,
		nil,
		nil,
		nil,
		concurrencyService,
		billingCacheService,
		nil,
		apiKeyService,
		nil,
		nil,
		nil,
		nil,
		cfg,
		nil,
	)

	router := gin.New()
	routes.RegisterGatewayRoutes(
		router,
		&handler.Handlers{
			Gateway:       gatewayHandler,
			OpenAIGateway: &handler.OpenAIGatewayHandler{},
		},
		servermiddleware.NewAPIKeyAuthMiddleware(apiKeyService, nil, cfg),
		servermiddleware.NewEvaluationEvidenceMiddleware(evidenceRepo),
		apiKeyService,
		nil,
		nil,
		nil,
		nil,
		cfg,
	)

	signer, err := service.NewEvaluationContextSigner([]byte(cfg.Radar.SigningSecret), 5*time.Minute)
	require.NoError(t, err)
	require.NotZero(t, userID)
	require.NotZero(t, groupID)

	return &radarP0Fixture{
		router:          router,
		signer:          signer,
		normalKey:       normalKeyValue,
		evaluationKey:   evaluationKeyValue,
		normalKeyID:     normalKeyID,
		evaluationKeyID: evaluationKeyID,
	}
}

func radarP0Database(t *testing.T) *radarP0TestDatabase {
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
	sqlDB, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.Eventually(t, func() bool { return sqlDB.PingContext(ctx) == nil }, 30*time.Second, 250*time.Millisecond)
	require.NoError(t, repository.ApplyMigrations(ctx, sqlDB))

	entClient := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, sqlDB)))
	t.Cleanup(func() { require.NoError(t, entClient.Close()) })
	return &radarP0TestDatabase{SQL: sqlDB, Ent: entClient}
}

func provisionRadarP0Principals(
	t *testing.T,
	db *sql.DB,
	upstreamURL string,
	normalKey string,
	evaluationKey string,
) (int64, int64, int64, int64) {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.NewString()

	var groupID int64
	var accountID int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO groups (name, platform, status, subscription_type, rate_multiplier)
		VALUES ($1, 'anthropic', 'active', 'standard', 1) RETURNING id`,
		"radar-p0-"+suffix,
	).Scan(&groupID))

	var userID int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO users (email, username, password_hash, role, balance, concurrency, status)
		VALUES ($1, $2, 'not-a-login-secret', 'user', 10, 0, 'active') RETURNING id`,
		"radar-p0-"+suffix+"@example.com",
		"radar-p0-"+suffix,
	).Scan(&userID))

	var normalKeyID int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO api_keys (user_id, key, name, group_id, status, is_evaluation)
		VALUES ($1, $2, 'normal', $3, 'active', false) RETURNING id`,
		userID,
		normalKey,
		groupID,
	).Scan(&normalKeyID))

	var evaluationKeyID int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO api_keys (user_id, key, name, group_id, status, is_evaluation)
		VALUES ($1, $2, 'evaluation', $3, 'active', true) RETURNING id`,
		userID,
		evaluationKey,
		groupID,
	).Scan(&evaluationKeyID))

	credentials, err := json.Marshal(map[string]any{
		"api_key":  "radar-upstream-key",
		"base_url": upstreamURL,
		"model_mapping": map[string]string{
			radarP0RequestedModel: radarP0UpstreamModel,
		},
	})
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO accounts (
			id, name, platform, type, credentials, extra, concurrency,
			priority, status, schedulable, rate_multiplier
		) VALUES (
			$1, $2, 'anthropic', 'apikey', $3, '{}'::jsonb, 0,
			1, 'active', true, 1
		) RETURNING id`,
		radarP0AccountID,
		"radar-p0-account-"+suffix,
		string(credentials),
	).Scan(&accountID))
	require.Equal(t, radarP0AccountID, accountID)
	_, err = db.ExecContext(ctx, `
		INSERT INTO account_groups (account_id, group_id, priority)
		VALUES ($1, $2, 1)`,
		radarP0AccountID,
		groupID,
	)
	require.NoError(t, err)

	channelMapping, err := json.Marshal(map[string]map[string]string{
		service.PlatformAnthropic: {
			radarP0RequestedModel: radarP0UpstreamModel,
		},
	})
	require.NoError(t, err)
	var channelID int64
	require.NoError(t, db.QueryRowContext(ctx, `
		INSERT INTO channels (
			id, name, description, status, model_mapping,
			billing_model_source, restrict_models
		) VALUES (
			$1, $2, '', 'active', $3, 'upstream', false
		) RETURNING id`,
		radarP0ChannelID,
		"radar-p0-channel-"+suffix,
		string(channelMapping),
	).Scan(&channelID))
	require.Equal(t, radarP0ChannelID, channelID)
	_, err = db.ExecContext(ctx, `
		INSERT INTO channel_groups (channel_id, group_id)
		VALUES ($1, $2)`,
		radarP0ChannelID,
		groupID,
	)
	require.NoError(t, err)

	return userID, groupID, normalKeyID, evaluationKeyID
}

func radarP0Token(
	t *testing.T,
	signer *service.EvaluationContextSigner,
	apiKeyID int64,
	runID string,
	sampleID string,
) string {
	t.Helper()
	now := time.Now().UTC()
	token, err := signer.Sign(service.EvaluationContext{
		RunID:                runID,
		SampleID:             sampleID,
		DatasetVersion:       "dataset-v1",
		ExpectedModelAlias:   radarP0RequestedModel,
		ExpectedRouteProfile: "route-v42",
		APIKeyID:             apiKeyID,
		IssuedAt:             now.Add(-time.Second),
		ExpiresAt:            now.Add(2 * time.Minute),
	})
	require.NoError(t, err)
	return token
}

func radarP0Request(router http.Handler, key string, token string) *httptest.ResponseRecorder {
	body := `{"model":"` + radarP0RequestedModel + `","max_tokens":32,"messages":[{"role":"user","content":"hello"}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
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
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM evaluation_route_evidence WHERE api_key_id = $1`,
		apiKeyID,
	).Scan(&count))
	require.Equal(t, want, count)
}

func loadRadarP0Evidence(t *testing.T, db *sql.DB, runID string, apiKeyID int64) radarP0Evidence {
	t.Helper()

	var count int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM evaluation_route_evidence WHERE evaluation_run_id = $1`,
		runID,
	).Scan(&count))
	require.Equal(t, 1, count, "one server-generated trace must produce one row")

	var evidence radarP0Evidence
	require.NoError(t, db.QueryRow(`
		SELECT
			route_trace_id::text,
			evaluation_run_id::text,
			sample_id::text,
			api_key_id,
			COALESCE(request_id, ''),
			requested_model,
			COALESCE(resolved_model, ''),
			route_profile_version,
			COALESCE(provider, ''),
			COALESCE(channel_ref, ''),
			COALESCE(account_pool_ref, ''),
			region,
			attempts,
			fallback_chain,
			transport_status,
			COALESCE(error_code, ''),
			input_tokens,
			output_tokens,
			ttft_ms,
			latency_ms,
			billed_amount::text
		FROM evaluation_route_evidence
		WHERE evaluation_run_id = $1 AND api_key_id = $2`,
		runID,
		apiKeyID,
	).Scan(
		&evidence.RouteTraceID,
		&evidence.RunID,
		&evidence.SampleID,
		&evidence.APIKeyID,
		&evidence.RequestID,
		&evidence.RequestedModel,
		&evidence.ResolvedModel,
		&evidence.RouteProfile,
		&evidence.Provider,
		&evidence.ChannelRef,
		&evidence.AccountRef,
		&evidence.Region,
		&evidence.Attempts,
		&evidence.FallbackJSON,
		&evidence.Status,
		&evidence.ErrorCode,
		&evidence.InputTokens,
		&evidence.OutputTokens,
		&evidence.TTFT,
		&evidence.Latency,
		&evidence.BilledAmount,
	))
	return evidence
}

func assertRadarP0Evidence(
	t *testing.T,
	evidence radarP0Evidence,
	runID string,
	sampleID string,
	apiKeyID int64,
	clientRequestID string,
) {
	t.Helper()

	_, err := uuid.Parse(evidence.RouteTraceID)
	require.NoError(t, err, "route_trace_id must be server-generated")
	require.NotEqual(t, runID, evidence.RouteTraceID)
	require.Equal(t, runID, evidence.RunID)
	require.Equal(t, sampleID, evidence.SampleID)
	require.Equal(t, apiKeyID, evidence.APIKeyID)
	require.Equal(t, "client:"+clientRequestID, evidence.RequestID)
	require.Equal(t, radarP0RequestedModel, evidence.RequestedModel)
	require.Equal(t, radarP0UpstreamModel, evidence.ResolvedModel)
	require.Equal(t, "route-v42", evidence.RouteProfile)
	require.Equal(t, service.PlatformAnthropic, evidence.Provider)
	require.Equal(t, "cn-east", evidence.Region)
	require.Equal(t, 1, evidence.Attempts)
	require.Equal(t, "succeeded", evidence.Status)
	require.Empty(t, evidence.ErrorCode)
	require.NotEmpty(t, evidence.AccountRef)
	require.NotEmpty(t, evidence.ChannelRef)
	require.NotEqual(t, strconv.FormatInt(radarP0AccountID, 10), evidence.AccountRef)
	require.NotEqual(t, strconv.FormatInt(radarP0ChannelID, 10), evidence.ChannelRef)

	var fallback []service.RouteFallbackEntry
	require.NoError(t, json.Unmarshal(evidence.FallbackJSON, &fallback))
	require.Len(t, fallback, 1)
	require.Equal(t, 1, fallback[0].Ordinal)
	require.Equal(t, service.PlatformAnthropic, fallback[0].Provider)
	require.Equal(t, evidence.AccountRef, fallback[0].AccountPoolRef)
	require.Equal(t, evidence.ChannelRef, fallback[0].ChannelRef)
	require.Equal(t, radarP0UpstreamModel, fallback[0].ResolvedModel)
	require.Equal(t, "cn-east", fallback[0].Region)
	require.Empty(t, fallback[0].ErrorCode)
	require.NotContains(t, string(evidence.FallbackJSON), `"account_pool_ref":"`+strconv.FormatInt(radarP0AccountID, 10)+`"`)
	require.NotContains(t, string(evidence.FallbackJSON), `"channel_ref":"`+strconv.FormatInt(radarP0ChannelID, 10)+`"`)
	require.NotContains(t, string(evidence.FallbackJSON), "account_id")
	require.NotContains(t, string(evidence.FallbackJSON), "channel_id")

	require.True(t, evidence.InputTokens.Valid)
	require.Equal(t, int64(101), evidence.InputTokens.Int64)
	require.True(t, evidence.OutputTokens.Valid)
	require.Equal(t, int64(37), evidence.OutputTokens.Int64)
	require.False(t, evidence.TTFT.Valid, "non-stream responses should not invent TTFT")
	require.True(t, evidence.Latency.Valid)
	require.GreaterOrEqual(t, evidence.Latency.Int64, int64(0))
	require.True(t, evidence.BilledAmount.Valid)
	require.True(t, decimal.RequireFromString(evidence.BilledAmount.String).IsPositive())
}

func assertRadarP0UsageMatchesEvidence(
	t *testing.T,
	db *sql.DB,
	apiKeyID int64,
	evidence radarP0Evidence,
) {
	t.Helper()

	var (
		requestID      string
		requestedModel string
		upstreamModel  string
		inputTokens    int64
		outputTokens   int64
		durationMS     int64
		actualCost     string
		channelID      sql.NullInt64
	)
	require.NoError(t, db.QueryRow(`
		SELECT
			request_id,
			requested_model,
			COALESCE(NULLIF(upstream_model, ''), model),
			input_tokens,
			output_tokens,
			COALESCE(duration_ms, 0),
			actual_cost::text,
			channel_id
		FROM usage_logs
		WHERE api_key_id = $1`,
		apiKeyID,
	).Scan(
		&requestID,
		&requestedModel,
		&upstreamModel,
		&inputTokens,
		&outputTokens,
		&durationMS,
		&actualCost,
		&channelID,
	))
	require.Equal(t, evidence.RequestID, requestID)
	require.Equal(t, evidence.RequestedModel, requestedModel)
	require.Equal(t, evidence.ResolvedModel, upstreamModel)
	require.Equal(t, evidence.InputTokens.Int64, inputTokens)
	require.Equal(t, evidence.OutputTokens.Int64, outputTokens)
	require.Equal(t, evidence.Latency.Int64, durationMS)
	require.True(t, channelID.Valid)
	require.Equal(t, radarP0ChannelID, channelID.Int64)
	require.True(t, decimal.RequireFromString(evidence.BilledAmount.String).Equal(decimal.RequireFromString(actualCost)))
}
