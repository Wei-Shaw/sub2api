//go:build integration

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

const (
	gatewayUsageBillingTestRedisDB = 15
	gatewayRequestedModel          = "catalog-public-chat"
	gatewayCatalogRequestedKey     = "catalog-requested-chat"
	gatewayMappedModel             = "gpt-4o"
	gatewayCatalogPricingSource    = "gateway-integration-catalog"
)

type gatewayIntegrationFixture struct {
	APIKeyID                  int64
	APIKeyValue               string
	CatalogEpoch              int64
	CatalogRevisionID         int64
	RequestedModelRevisionID  int64
	EffectiveModelRevisionID  int64
	CatalogInputCostPerToken  float64
	CatalogOutputCostPerToken float64
}

func TestGatewayHTTPUsageBilling_DurableSettlementAndUpstreamFailureBoundary(t *testing.T) {
	postgresDSN := strings.TrimSpace(os.Getenv("SUB2API_TEST_POSTGRES_DSN"))
	if postgresDSN == "" {
		t.Skip("SUB2API_TEST_POSTGRES_DSN is required for the real gateway integration test")
	}
	redisAddr := strings.TrimSpace(os.Getenv("SUB2API_TEST_REDIS_ADDR"))
	if redisAddr == "" {
		t.Skip("SUB2API_TEST_REDIS_ADDR is required for the real gateway integration test")
	}

	flushGatewayIntegrationRedis(t, redisAddr)

	pricingFixturePath := filepath.Join("..", "..", "resources", "model-pricing", "model_prices_and_context_window.json")
	pricingFixture, err := os.ReadFile(pricingFixturePath)
	require.NoError(t, err)
	pricingHash := sha256.Sum256(pricingFixture)

	var failUpstream atomic.Bool
	var upstreamRequests atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/model_pricing.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(pricingFixture)
			return
		case "/model_pricing.sha256":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, hex.EncodeToString(pricingHash[:]))
			return
		}

		upstreamRequests.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected fake upstream request: method=%s path=%s", r.Method, r.URL.Path)
			http.Error(w, "unexpected fake upstream request", http.StatusBadRequest)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read fake upstream request: %v", err)
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		var requestBody struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(body, &requestBody); err != nil {
			t.Errorf("decode fake upstream request: %v", err)
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if requestBody.Model != gatewayMappedModel {
			t.Errorf("unexpected fake upstream model: %q", requestBody.Model)
			http.Error(w, "unexpected model", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if failUpstream.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":{"message":"temporary local upstream failure"}}`)
			return
		}
		_, _ = io.WriteString(w, `{
			"id":"chatcmpl-local-gateway-integration",
			"object":"chat.completion",
			"created":1,
			"model":"gpt-4o",
			"choices":[{"index":0,"message":{"role":"assistant","content":"fake upstream ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}
		}`)
	}))
	t.Cleanup(upstream.Close)
	configureGatewayIntegrationEnvironment(t, postgresDSN, redisAddr, upstream.URL, pricingFixture)

	fixture := seedGatewayIntegrationFixture(t, postgresDSN, upstream.URL)
	apiKeyID := fixture.APIKeyID
	apiKeyValue := fixture.APIKeyValue

	app, err := initializeApplication(handler.BuildInfo{Version: "gateway-integration", BuildType: "test"})
	require.NoError(t, err)
	t.Cleanup(app.Cleanup)

	gateway := httptest.NewServer(app.Server.Handler)
	t.Cleanup(gateway.Close)

	client := &http.Client{Timeout: 5 * time.Second}
	var successStatus int
	var successBody []byte
	var lastRequestErr error
	require.Eventually(t, func() bool {
		successStatus, successBody, lastRequestErr = postGatewayChatCompletion(client, gateway.URL, apiKeyValue)
		return lastRequestErr == nil && successStatus == http.StatusOK
	}, 10*time.Second, 200*time.Millisecond, "gateway never became ready: status=%d body=%s err=%v", successStatus, successBody, lastRequestErr)

	var response struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	require.NoError(t, json.Unmarshal(successBody, &response))
	require.Equal(t, gatewayMappedModel, response.Model)
	require.Len(t, response.Choices, 1)
	require.Equal(t, "fake upstream ok", response.Choices[0].Message.Content)
	require.Equal(t, 10, response.Usage.PromptTokens)
	require.Equal(t, 4, response.Usage.CompletionTokens)
	require.Equal(t, 14, response.Usage.TotalTokens)
	require.EqualValues(t, 1, upstreamRequests.Load())

	queryDB, err := sql.Open("postgres", postgresDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = queryDB.Close() })

	type persistedUsage struct {
		requestID           string
		model               string
		requestedModel      string
		upstreamModel       sql.NullString
		modelMappingChain   sql.NullString
		inputTokens         int
		outputTokens        int
		totalTokens         int
		totalCost           float64
		actualCost          float64
		channelID           sql.NullInt64
		catalogEpoch        sql.NullInt64
		catalogRevisionID   sql.NullInt64
		requestedRevisionID sql.NullInt64
		effectiveRevisionID sql.NullInt64
		pricingSource       sql.NullString
		pricingSnapshot     []byte
	}
	var usage persistedUsage
	var settlementStatus string
	var balance float64
	var usageCount, settlementCount int
	deadline := time.Now().Add(10 * time.Second)
	for {
		_ = queryDB.QueryRow("SELECT COUNT(*) FROM usage_logs WHERE api_key_id = $1", apiKeyID).Scan(&usageCount)
		_ = queryDB.QueryRow("SELECT COUNT(*) FROM usage_billing_settlements WHERE api_key_id = $1", apiKeyID).Scan(&settlementCount)
		err = queryDB.QueryRow(`
			SELECT request_id, model, input_tokens, output_tokens,
				input_tokens + output_tokens + cache_creation_tokens + cache_read_tokens,
				total_cost, actual_cost, channel_id, requested_model, upstream_model,
				model_mapping_chain, catalog_epoch, catalog_revision_id,
				requested_model_revision_id, effective_model_revision_id,
				pricing_source, pricing_snapshot
			FROM usage_logs
			WHERE api_key_id = $1
			ORDER BY id DESC
			LIMIT 1
		`, apiKeyID).Scan(
			&usage.requestID,
			&usage.model,
			&usage.inputTokens,
			&usage.outputTokens,
			&usage.totalTokens,
			&usage.totalCost,
			&usage.actualCost,
			&usage.channelID,
			&usage.requestedModel,
			&usage.upstreamModel,
			&usage.modelMappingChain,
			&usage.catalogEpoch,
			&usage.catalogRevisionID,
			&usage.requestedRevisionID,
			&usage.effectiveRevisionID,
			&usage.pricingSource,
			&usage.pricingSnapshot,
		)
		if err == nil {
			err = queryDB.QueryRow(`
				SELECT status
				FROM usage_billing_settlements
				WHERE request_id = $1 AND api_key_id = $2
			`, usage.requestID, apiKeyID).Scan(&settlementStatus)
		}
		if err == nil && settlementStatus == "applied" {
			err = queryDB.QueryRow(`
				SELECT u.balance
				FROM users u
				JOIN api_keys k ON k.user_id = u.id
				WHERE k.id = $1
			`, apiKeyID).Scan(&balance)
			if err == nil {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("usage/settlement was not durably applied: err=%v status=%q usage_count=%d settlement_count=%d", err, settlementStatus, usageCount, settlementCount)
		}
		time.Sleep(100 * time.Millisecond)
	}

	require.NotEmpty(t, usage.requestID)
	require.Equal(t, gatewayRequestedModel, usage.model)
	require.Equal(t, gatewayRequestedModel, usage.requestedModel)
	require.True(t, usage.upstreamModel.Valid)
	require.Equal(t, gatewayMappedModel, usage.upstreamModel.String)
	require.True(t, usage.modelMappingChain.Valid)
	require.Equal(t, gatewayRequestedModel+"→"+gatewayMappedModel, usage.modelMappingChain.String)
	require.Equal(t, 10, usage.inputTokens)
	require.Equal(t, 4, usage.outputTokens)
	require.Equal(t, 14, usage.totalTokens)
	require.InDelta(t, 0.000065, usage.totalCost, 0.000000001)
	require.InDelta(t, 0.000065, usage.actualCost, 0.000000001)
	require.True(t, usage.channelID.Valid)
	require.True(t, usage.catalogEpoch.Valid)
	require.Equal(t, fixture.CatalogEpoch, usage.catalogEpoch.Int64)
	require.True(t, usage.catalogRevisionID.Valid)
	require.Equal(t, fixture.CatalogRevisionID, usage.catalogRevisionID.Int64)
	require.True(t, usage.requestedRevisionID.Valid)
	require.Equal(t, fixture.RequestedModelRevisionID, usage.requestedRevisionID.Int64)
	require.True(t, usage.effectiveRevisionID.Valid)
	require.Equal(t, fixture.EffectiveModelRevisionID, usage.effectiveRevisionID.Int64)
	require.True(t, usage.pricingSource.Valid)
	require.Equal(t, gatewayCatalogPricingSource, usage.pricingSource.String)

	var pricingSnapshot map[string]any
	require.NoError(t, json.Unmarshal(usage.pricingSnapshot, &pricingSnapshot))
	inputPrice, ok := pricingSnapshot["input_cost_per_token"].(float64)
	require.True(t, ok)
	outputPrice, ok := pricingSnapshot["output_cost_per_token"].(float64)
	require.True(t, ok)
	require.InDelta(t, fixture.CatalogInputCostPerToken, inputPrice, 0.000000000001)
	require.InDelta(t, fixture.CatalogOutputCostPerToken, outputPrice, 0.000000000001)
	settlementSnapshot, ok := pricingSnapshot["_settlement_cost"].(map[string]any)
	require.True(t, ok)
	pinnedActualCost, ok := settlementSnapshot["actual_cost"].(float64)
	require.True(t, ok)
	require.InDelta(t, usage.actualCost, pinnedActualCost, 0.000000001)
	catalogProjectedCost := float64(usage.inputTokens)*fixture.CatalogInputCostPerToken + float64(usage.outputTokens)*fixture.CatalogOutputCostPerToken
	require.NotEqual(t, catalogProjectedCost, usage.actualCost, "shadow mode must prove catalog snapshot is not silently treated as legacy billing authority")

	var commandPayload []byte
	require.NoError(t, queryDB.QueryRow(`
		SELECT command
		FROM usage_billing_settlements
		WHERE request_id = $1 AND api_key_id = $2
	`, usage.requestID, apiKeyID).Scan(&commandPayload))
	var command service.UsageBillingCommand
	require.NoError(t, json.Unmarshal(commandPayload, &command))
	require.Equal(t, fixture.CatalogEpoch, command.CatalogEpoch)
	require.Equal(t, fixture.CatalogRevisionID, command.CatalogRevisionID)
	require.Equal(t, fixture.RequestedModelRevisionID, command.RequestedModelRevisionID)
	require.Equal(t, fixture.EffectiveModelRevisionID, command.EffectiveModelRevisionID)
	require.Equal(t, gatewayCatalogPricingSource, command.PricingSource)
	require.NotEmpty(t, command.PricingSnapshotHash)
	require.InDelta(t, usage.actualCost, command.BalanceCost, 0.000000001)
	require.Equal(t, "applied", settlementStatus)
	require.InDelta(t, 9.999935, balance, 0.000000001)

	usageCountBefore := countGatewayRows(t, queryDB, "usage_logs", apiKeyID)
	settlementCountBefore := countGatewayRows(t, queryDB, "usage_billing_settlements", apiKeyID)
	require.Equal(t, 1, usageCountBefore)
	require.Equal(t, 1, settlementCountBefore)

	failUpstream.Store(true)
	failureStatus, failureBody, err := postGatewayChatCompletion(client, gateway.URL, apiKeyValue)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadGateway, failureStatus, "body=%s", failureBody)
	require.Eventually(t, func() bool {
		return upstreamRequests.Load() == 2
	}, 2*time.Second, 25*time.Millisecond)

	// Usage recording is asynchronous; hold the boundary long enough for an
	// accidental failure-path enqueue to become visible before asserting counts.
	time.Sleep(500 * time.Millisecond)
	require.Equal(t, usageCountBefore, countGatewayRows(t, queryDB, "usage_logs", apiKeyID))
	require.Equal(t, settlementCountBefore, countGatewayRows(t, queryDB, "usage_billing_settlements", apiKeyID))
}

func configureGatewayIntegrationEnvironment(t *testing.T, postgresDSN, redisAddr, fakeUpstreamURL string, pricingFixture []byte) {
	t.Helper()

	parsed, err := url.Parse(postgresDSN)
	require.NoError(t, err)
	require.Equal(t, "postgres", parsed.Scheme, "SUB2API_TEST_POSTGRES_DSN must use postgres:// URL form")
	require.NotNil(t, parsed.User)
	require.NotEmpty(t, parsed.User.Username())
	require.NotEmpty(t, strings.TrimPrefix(parsed.Path, "/"))

	databaseHost := parsed.Hostname()
	if databaseHost == "" {
		databaseHost = parsed.Query().Get("host")
	}
	require.NotEmpty(t, databaseHost)
	port := parsed.Port()
	if port == "" {
		port = parsed.Query().Get("port")
	}
	if port == "" {
		port = "5432"
	}
	password, _ := parsed.User.Password()
	sslMode := parsed.Query().Get("sslmode")
	if sslMode == "" {
		sslMode = "disable"
	}

	host, redisPort, err := net.SplitHostPort(redisAddr)
	require.NoError(t, err)
	pricingDir := t.TempDir()
	pricingFile := filepath.Join(pricingDir, "model_pricing.json")
	require.NoError(t, os.WriteFile(pricingFile, pricingFixture, 0o600))

	t.Setenv("DATABASE_HOST", databaseHost)
	t.Setenv("DATABASE_PORT", port)
	t.Setenv("DATABASE_USER", parsed.User.Username())
	t.Setenv("DATABASE_PASSWORD", password)
	t.Setenv("DATABASE_DBNAME", strings.TrimPrefix(parsed.Path, "/"))
	t.Setenv("DATABASE_SSLMODE", sslMode)
	t.Setenv("REDIS_HOST", host)
	t.Setenv("REDIS_PORT", redisPort)
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("REDIS_DB", strconv.Itoa(gatewayUsageBillingTestRedisDB))
	t.Setenv("RUN_MODE", config.RunModeStandard)
	t.Setenv("SERVER_MODE", "release")
	t.Setenv("JWT_SECRET", strings.Repeat("a", 64))
	t.Setenv("TOTP_ENCRYPTION_KEY", strings.Repeat("1", 64))
	t.Setenv("SECURITY_URL_ALLOWLIST_ENABLED", "false")
	t.Setenv("SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP", "true")
	t.Setenv("MODEL_CATALOG_MODE", "shadow")
	t.Setenv("MODEL_CATALOG_IMPORT_MODE", "off")
	t.Setenv("MODEL_CATALOG_LIST_READ_MODE", "legacy")
	t.Setenv("MODEL_CATALOG_PRICING_READ_MODE", "legacy")
	t.Setenv("MODEL_CATALOG_ADMISSION_MODE", "observe")
	t.Setenv("PRICING_DATA_DIR", pricingDir)
	t.Setenv("PRICING_FALLBACK_FILE", pricingFile)
	t.Setenv("PRICING_REMOTE_URL", fakeUpstreamURL+"/model_pricing.json")
	t.Setenv("PRICING_HASH_URL", fakeUpstreamURL+"/model_pricing.sha256")
}

func flushGatewayIntegrationRedis(t *testing.T, addr string) {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: addr, DB: gatewayUsageBillingTestRedisDB})
	t.Cleanup(func() {
		_ = client.FlushDB(context.Background()).Err()
		_ = client.Close()
	})
	require.NoError(t, client.Ping(context.Background()).Err())
	require.NoError(t, client.FlushDB(context.Background()).Err())
}

func seedGatewayIntegrationFixture(t *testing.T, postgresDSN, upstreamURL string) gatewayIntegrationFixture {
	t.Helper()

	cfg, err := config.LoadForBootstrap()
	require.NoError(t, err)
	client, sqlDB, err := repository.InitEnt(cfg)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	suffix := uuid.NewString()
	user, err := client.User.Create().
		SetEmail("gateway-integration-" + suffix + "@example.com").
		SetPasswordHash("test-password-hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		SetBalance(10).
		SetConcurrency(5).
		Save(ctx)
	require.NoError(t, err)

	group, err := client.Group.Create().
		SetName("gateway-integration-" + suffix).
		SetPlatform(service.PlatformDeepSeek).
		SetStatus(service.StatusActive).
		SetSubscriptionType(service.SubscriptionTypeStandard).
		SetRateMultiplier(1).
		SetSupportedModelScopes([]string{"deepseek"}).
		Save(ctx)
	require.NoError(t, err)

	account, err := client.Account.Create().
		SetName("gateway-integration-" + suffix).
		SetPlatform(service.PlatformDeepSeek).
		SetType(service.AccountTypeAPIKey).
		SetCredentials(map[string]any{
			"api_key":  "local-fake-upstream-key",
			"base_url": upstreamURL,
		}).
		SetExtra(map[string]any{}).
		SetConcurrency(3).
		SetPriority(50).
		SetStatus(service.StatusActive).
		SetSchedulable(true).
		AddGroupIDs(group.ID).
		Save(ctx)
	require.NoError(t, err)
	require.NotZero(t, account.ID)

	apiKeyValue := "sk-gateway-integration-" + suffix
	apiKey, err := client.APIKey.Create().
		SetUserID(user.ID).
		SetKey(apiKeyValue).
		SetName("gateway-integration").
		SetGroupID(group.ID).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	modelMapping, err := json.Marshal(map[string]map[string]string{
		service.PlatformDeepSeek: {
			gatewayRequestedModel: gatewayMappedModel,
		},
	})
	require.NoError(t, err)
	var channelID int64
	err = sqlDB.QueryRowContext(ctx, `
		INSERT INTO channels (name, description, status, model_mapping, billing_model_source, restrict_models)
		VALUES ($1, '', 'active', $2::jsonb, 'channel_mapped', false)
		RETURNING id
	`, "gateway-integration-"+suffix, string(modelMapping)).Scan(&channelID)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO channel_groups (channel_id, group_id)
		VALUES ($1, $2)
	`, channelID, group.ID)
	require.NoError(t, err)

	catalog := seedGatewayIntegrationCatalog(t, sqlDB, suffix)
	return gatewayIntegrationFixture{
		APIKeyID:                  apiKey.ID,
		APIKeyValue:               apiKeyValue,
		CatalogEpoch:              catalog.CatalogEpoch,
		CatalogRevisionID:         catalog.CatalogRevisionID,
		RequestedModelRevisionID:  catalog.RequestedModelRevisionID,
		EffectiveModelRevisionID:  catalog.EffectiveModelRevisionID,
		CatalogInputCostPerToken:  catalog.CatalogInputCostPerToken,
		CatalogOutputCostPerToken: catalog.CatalogOutputCostPerToken,
	}
}

func seedGatewayIntegrationCatalog(t *testing.T, db *sql.DB, suffix string) gatewayIntegrationFixture {
	t.Helper()

	ctx := context.Background()
	catalogRepo := repository.NewModelCatalogRepository(db)
	revision, err := catalogRepo.NextRevision(ctx, service.CatalogScopeGlobal)
	require.NoError(t, err)

	catalogInputCost := 0.000123
	catalogOutputCost := 0.000456
	normalizedHash := sha256.Sum256([]byte("gateway-catalog-normalized:" + suffix))
	requestedSourceHash := sha256.Sum256([]byte("gateway-catalog-requested:" + suffix))
	effectiveSourceHash := sha256.Sum256([]byte("gateway-catalog-effective:" + suffix))
	stage := service.CatalogRevisionStage{
		Revision:       revision,
		NormalizedHash: hex.EncodeToString(normalizedHash[:]),
		Normalizer:     service.CatalogNormalizerVersion,
		SyncRun: service.CatalogSyncRunSpec{
			SourceSet:       "gateway-integration",
			Trigger:         "test",
			Normalizer:      service.CatalogNormalizerVersion,
			SourceCount:     2,
			NormalizedCount: 2,
			AddedCount:      2,
		},
		Models: []service.CatalogSnapshotModelSpec{
			{
				ID:                   1,
				RevisionID:           1,
				CanonicalKey:         gatewayCatalogRequestedKey,
				OperatorState:        service.CatalogOperatorStateEnabled,
				SourceState:          service.CatalogSourceStatePresent,
				Provider:             service.PlatformDeepSeek,
				Platform:             service.PlatformDeepSeek,
				Mode:                 "chat",
				PricingSchemaVersion: 1,
				PricingValid:         true,
				PricingSource:        gatewayCatalogPricingSource,
				Pricing: &service.LiteLLMModelPricing{
					InputCostPerToken:  0.000111,
					OutputCostPerToken: 0.000222,
					LiteLLMProvider:    service.PlatformDeepSeek,
					Mode:               "chat",
				},
				SourceHash:     hex.EncodeToString(requestedSourceHash[:]),
				Capabilities:   json.RawMessage(`{"supports_chat":true}`),
				SourceMetadata: json.RawMessage(`{"fixture":"gateway"}`),
				Aliases: []service.CatalogAliasSpec{
					{Alias: gatewayRequestedModel, PlatformScope: service.PlatformDeepSeek},
				},
			},
			{
				ID:                   2,
				RevisionID:           2,
				CanonicalKey:         gatewayMappedModel,
				OperatorState:        service.CatalogOperatorStateEnabled,
				SourceState:          service.CatalogSourceStatePresent,
				Provider:             service.PlatformDeepSeek,
				Platform:             service.PlatformDeepSeek,
				Mode:                 "chat",
				PricingSchemaVersion: 1,
				PricingValid:         true,
				PricingSource:        gatewayCatalogPricingSource,
				Pricing: &service.LiteLLMModelPricing{
					InputCostPerToken:  catalogInputCost,
					OutputCostPerToken: catalogOutputCost,
					LiteLLMProvider:    service.PlatformDeepSeek,
					Mode:               "chat",
				},
				SourceHash:     hex.EncodeToString(effectiveSourceHash[:]),
				Capabilities:   json.RawMessage(`{"supports_chat":true}`),
				SourceMetadata: json.RawMessage(`{"fixture":"gateway"}`),
				Aliases: []service.CatalogAliasSpec{
					{Alias: gatewayMappedModel, PlatformScope: service.PlatformDeepSeek},
				},
			},
		},
	}

	catalogRevisionID, err := catalogRepo.StageRevision(ctx, stage)
	require.NoError(t, err)
	require.NoError(t, catalogRepo.ValidateRevision(ctx, catalogRevisionID))
	publication, err := catalogRepo.PublishRevision(ctx, service.CatalogPublishRequest{
		Scope:             service.CatalogScopeGlobal,
		CatalogRevisionID: catalogRevisionID,
		ActorType:         "integration-test",
		Reason:            "exercise gateway catalog model identity",
	})
	require.NoError(t, err)
	active, err := catalogRepo.LoadActiveSnapshot(ctx, service.CatalogScopeGlobal)
	require.NoError(t, err)

	var requestedRevisionID, effectiveRevisionID int64
	for _, model := range active.Models {
		switch model.CanonicalKey {
		case gatewayCatalogRequestedKey:
			requestedRevisionID = model.RevisionID
		case gatewayMappedModel:
			effectiveRevisionID = model.RevisionID
		}
	}
	require.NotZero(t, requestedRevisionID)
	require.NotZero(t, effectiveRevisionID)

	return gatewayIntegrationFixture{
		CatalogEpoch:              publication.Epoch,
		CatalogRevisionID:         publication.CatalogRevisionID,
		RequestedModelRevisionID:  requestedRevisionID,
		EffectiveModelRevisionID:  effectiveRevisionID,
		CatalogInputCostPerToken:  catalogInputCost,
		CatalogOutputCostPerToken: catalogOutputCost,
	}
}

func postGatewayChatCompletion(client *http.Client, baseURL, apiKey string) (int, []byte, error) {
	payload := []byte(`{"model":"catalog-public-chat","messages":[{"role":"user","content":"local integration test"}],"stream":false}`)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return resp.StatusCode, body, err
}

func countGatewayRows(t *testing.T, db *sql.DB, table string, apiKeyID int64) int {
	t.Helper()
	allowed := map[string]bool{
		"usage_logs":                true,
		"usage_billing_settlements": true,
	}
	require.True(t, allowed[table], "unexpected table %q", table)

	var count int
	query := map[string]string{
		"usage_logs":                "SELECT COUNT(*) FROM usage_logs WHERE api_key_id = $1",
		"usage_billing_settlements": "SELECT COUNT(*) FROM usage_billing_settlements WHERE api_key_id = $1",
	}[table]
	err := db.QueryRow(query, apiKeyID).Scan(&count)
	require.NoError(t, err)
	return count
}
