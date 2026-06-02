package admin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/testsupport"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestBotSalesFulfillmentHandlerRequiresIdempotencyKey(t *testing.T) {
	router, _, cleanup := newBotSalesFulfillmentHandlerTestRouter(t)
	defer cleanup()

	payload := map[string]any{
		"external_order_id": "bs-http-missing-key",
		"operation":         service.BotSalesFulfillmentOperationNew,
		"entitlement_kind":  service.BotSalesEntitlementSubscription,
		"plan_id":           1,
		"buyer": map[string]any{
			"external_user_id": "telegram:http-missing-key",
			"email":            "bot-http-missing-key@example.test",
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/bot-sales/token-fulfillments", jsonBody(t, payload))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var envelope response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Equal(t, "IDEMPOTENCY_KEY_REQUIRED", envelope.Reason)
}

func TestBotSalesFulfillmentHandlerFulfillsPayloadAndRejectsTargetGroupInput(t *testing.T) {
	router, client, cleanup := newBotSalesFulfillmentHandlerTestRouter(t)
	defer cleanup()
	ctx := context.Background()

	group := createBotSalesFulfillmentHandlerGroup(t, client, "bot-http-subscription", service.SubscriptionTypeSubscription)
	plan := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("Bot HTTP monthly").
		SetPrice(9.9).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetForSale(true).
		SaveX(ctx)

	validPayload := map[string]any{
		"external_system":        "bot-sales",
		"external_order_code":    "bs-http-order-1001",
		"external_order_item_id": "item-1001",
		"operation":              service.BotSalesFulfillmentOperationNew,
		"entitlement_kind":       service.BotSalesEntitlementSubscription,
		"plan_id":                plan.ID,
		"sku":                    "SUB2API_TOKEN_30D",
		"quantity":               1,
		"buyer": map[string]any{
			"external_user_id": "telegram:http-1001",
			"email":            "bot-http-user-1001@example.test",
			"display_name":     "HTTP Buyer 1001",
		},
		"affiliate": map[string]any{"aff_code": "AFFHTTP01"},
		"delivery_policy": map[string]any{
			"issue_api_key": "always",
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/bot-sales/token-fulfillments", jsonBody(t, validPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "bot-sales:bs-http-order-1001:1:new")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var data map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &data))
	require.Equal(t, service.BotSalesEntitlementSubscription, data["entitlement_kind"])
	delivery, ok := data["delivery"].(map[string]any)
	require.True(t, ok)
	apiKey, ok := delivery["api_key"].(map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, apiKey["key"])
	require.EqualValues(t, group.ID, apiKey["group_id"])

	replay := httptest.NewRecorder()
	replayReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/bot-sales/token-fulfillments", jsonBody(t, validPayload))
	replayReq.Header.Set("Content-Type", "application/json")
	replayReq.Header.Set("Idempotency-Key", "bot-sales:bs-http-order-1001:1:new")
	router.ServeHTTP(replay, replayReq)
	require.Equal(t, http.StatusOK, replay.Code, replay.Body.String())
	require.Equal(t, "true", replay.Header().Get("X-Idempotency-Replayed"))
	require.Equal(t, 1, client.UserSubscription.Query().CountX(ctx))
	require.Equal(t, 1, client.APIKey.Query().CountX(ctx))

	badPayload := map[string]any{
		"external_order_id": "bs-http-target-group",
		"operation":         service.BotSalesFulfillmentOperationNew,
		"entitlement_kind":  service.BotSalesEntitlementSubscription,
		"plan_id":           plan.ID,
		"target_group_id":   group.ID,
		"buyer": map[string]any{
			"external_user_id": "telegram:http-target-group",
			"email":            "bot-http-target-group@example.test",
		},
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/bot-sales/token-fulfillments", jsonBody(t, badPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "bot-sales:bs-http-target-group:1:new")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var envelope response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Equal(t, "UNSUPPORTED_FIELD", envelope.Reason)
}

func newBotSalesFulfillmentHandlerTestRouter(t *testing.T) (*gin.Engine, *dbent.Client, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	client, db := newBotSalesFulfillmentHandlerEntClient(t)
	fulfillmentSvc := newBotSalesFulfillmentHandlerService(t, client, db)
	handler := NewBotSalesFulfillmentHandler(fulfillmentSvc)

	cfg := service.DefaultIdempotencyConfig()
	cfg.ObserveOnly = false
	service.SetDefaultIdempotencyCoordinator(service.NewIdempotencyCoordinator(newBotSalesFulfillmentMemoryIdempotencyRepo(), cfg))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 1, Concurrency: 5})
		c.Set(string(middleware2.ContextKeyUserRole), service.RoleAdmin)
		c.Next()
	})
	router.POST("/api/v1/admin/bot-sales/token-fulfillments", handler.Create)

	cleanup := func() {
		service.SetDefaultIdempotencyCoordinator(nil)
	}
	return router, client, cleanup
}

func newBotSalesFulfillmentHandlerService(t *testing.T, client *dbent.Client, db *sql.DB) *service.BotSalesFulfillmentService {
	t.Helper()
	return testsupport.NewBotSalesFulfillmentService(client, db)
}

func newBotSalesFulfillmentHandlerEntClient(t *testing.T) (*dbent.Client, *sql.DB) {
	t.Helper()
	dbName := "file:" + strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + "?mode=memory&cache=shared&_fk=1"
	db, err := sql.Open("sqlite", dbName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client, db
}

func createBotSalesFulfillmentHandlerGroup(t *testing.T, client *dbent.Client, name string, subscriptionType string) *dbent.Group {
	t.Helper()
	return client.Group.Create().
		SetName(name).
		SetPlatform("claude").
		SetStatus(service.StatusActive).
		SetSubscriptionType(subscriptionType).
		SetRateMultiplier(1).
		SaveX(context.Background())
}

func jsonBody(t *testing.T, payload any) *bytes.Reader {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	return bytes.NewReader(body)
}

type botSalesFulfillmentMemoryIdempotencyRepo struct {
	mu     sync.Mutex
	nextID int64
	data   map[string]*service.IdempotencyRecord
}

func newBotSalesFulfillmentMemoryIdempotencyRepo() *botSalesFulfillmentMemoryIdempotencyRepo {
	return &botSalesFulfillmentMemoryIdempotencyRepo{nextID: 1, data: map[string]*service.IdempotencyRecord{}}
}

func (r *botSalesFulfillmentMemoryIdempotencyRepo) key(scope, keyHash string) string {
	return scope + "\x00" + keyHash
}

func (r *botSalesFulfillmentMemoryIdempotencyRepo) CreateProcessing(_ context.Context, record *service.IdempotencyRecord) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := r.key(record.Scope, record.IdempotencyKeyHash)
	if _, ok := r.data[k]; ok {
		return false, nil
	}
	rec := *record
	rec.ID = r.nextID
	r.nextID++
	r.data[k] = &rec
	record.ID = rec.ID
	return true, nil
}

func (r *botSalesFulfillmentMemoryIdempotencyRepo) GetByScopeAndKeyHash(_ context.Context, scope, keyHash string) (*service.IdempotencyRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rec, ok := r.data[r.key(scope, keyHash)]; ok {
		copy := *rec
		return &copy, nil
	}
	return nil, nil
}

func (r *botSalesFulfillmentMemoryIdempotencyRepo) TryReclaim(_ context.Context, id int64, fromStatus string, _ time.Time, newLockedUntil, newExpiresAt time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.data {
		if rec.ID == id && rec.Status == fromStatus {
			rec.Status = service.IdempotencyStatusProcessing
			rec.LockedUntil = &newLockedUntil
			rec.ExpiresAt = newExpiresAt
			return true, nil
		}
	}
	return false, nil
}

func (r *botSalesFulfillmentMemoryIdempotencyRepo) ExtendProcessingLock(_ context.Context, id int64, requestFingerprint string, newLockedUntil, newExpiresAt time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.data {
		if rec.ID == id && rec.RequestFingerprint == requestFingerprint && rec.Status == service.IdempotencyStatusProcessing {
			rec.LockedUntil = &newLockedUntil
			rec.ExpiresAt = newExpiresAt
			return true, nil
		}
	}
	return false, nil
}

func (r *botSalesFulfillmentMemoryIdempotencyRepo) MarkSucceeded(_ context.Context, id int64, responseStatus int, responseBody string, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.data {
		if rec.ID == id {
			rec.Status = service.IdempotencyStatusSucceeded
			rec.ResponseStatus = &responseStatus
			rec.ResponseBody = &responseBody
			rec.ExpiresAt = expiresAt
			return nil
		}
	}
	return nil
}

func (r *botSalesFulfillmentMemoryIdempotencyRepo) MarkFailedRetryable(_ context.Context, id int64, errorReason string, lockedUntil, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.data {
		if rec.ID == id {
			rec.Status = service.IdempotencyStatusFailedRetryable
			rec.ErrorReason = &errorReason
			rec.LockedUntil = &lockedUntil
			rec.ExpiresAt = expiresAt
			return nil
		}
	}
	return nil
}

func (r *botSalesFulfillmentMemoryIdempotencyRepo) DeleteExpired(_ context.Context, now time.Time, limit int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var deleted int64
	for k, rec := range r.data {
		if limit > 0 && int(deleted) >= limit {
			break
		}
		if !rec.ExpiresAt.After(now) {
			delete(r.data, k)
			deleted++
		}
	}
	return deleted, nil
}
