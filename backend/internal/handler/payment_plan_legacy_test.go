//go:build unit

package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newPaymentPlanLegacyClient(t *testing.T) *dbent.Client {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func seedLegacyPlan(t *testing.T, client *dbent.Client, forSale bool) int64 {
	t.Helper()
	ctx := context.Background()
	group, err := client.Group.Create().
		SetName("Pro Group").
		SetPlatform(service.PlatformOpenAI).
		SetStatus(service.StatusActive).
		SetSubscriptionType(domain.SubscriptionTypeSubscription).
		SetRateMultiplier(2.5).
		SetSupportedModelScopes([]string{"gpt"}).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("Pro Plan").
		SetDescription("desc").
		SetPrice(99).
		SetOriginalPrice(129).
		SetValidityDays(1).
		SetValidityUnit("month").
		SetFeatures(`["fast","quota"]`).
		SetProductName("Pro Product").
		SetForSale(forSale).
		SetSortOrder(3).
		Save(ctx)
	require.NoError(t, err)
	return group.ID
}

func TestGetPlansLegacyReturnsSub2ApiPayShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := newPaymentPlanLegacyClient(t)
	groupID := seedLegacyPlan(t, client, true)
	configSvc := service.NewPaymentConfigService(client, nil, nil)
	h := NewPaymentHandler(nil, configSvc, nil)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/subscription-plans", nil)

	h.GetPlansLegacy(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data struct {
			Plans []map[string]any `json:"plans"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data.Plans, 1)
	plan := body.Data.Plans[0]
	require.Equal(t, float64(groupID), plan["groupId"])
	require.Equal(t, "Pro Group", plan["groupName"])
	require.Equal(t, "Pro Plan", plan["name"])
	require.Equal(t, 99.0, plan["price"])
	require.Equal(t, float64(1), plan["validityDays"])
	require.Equal(t, "month", plan["validityUnit"])
	require.Equal(t, service.PlatformOpenAI, plan["platform"])
}

func TestListPlansLegacyReturnsAdminSub2ApiPayShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := newPaymentPlanLegacyClient(t)
	groupID := seedLegacyPlan(t, client, false)
	configSvc := service.NewPaymentConfigService(client, nil, nil)
	h := adminhandler.NewPaymentHandler(nil, configSvc)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/subscription-plans", nil)

	h.ListPlansLegacy(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data struct {
			Plans []map[string]any `json:"plans"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data.Plans, 1)
	plan := body.Data.Plans[0]
	require.Equal(t, float64(groupID), plan["groupId"])
	require.Equal(t, "Pro Group", plan["groupName"])
	require.Equal(t, service.PlatformOpenAI, plan["platform"])
	require.Equal(t, false, plan["enabled"])
	require.Equal(t, float64(3), plan["sortOrder"])
	require.Equal(t, 2.5, plan["groupRateMultiplier"])
}

func TestCreatePlanLegacyAcceptsSub2ApiPayPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := newPaymentPlanLegacyClient(t)
	groupID := seedLegacyPlan(t, client, true)
	configSvc := service.NewPaymentConfigService(client, nil, nil)
	h := adminhandler.NewPaymentHandler(nil, configSvc)

	body := `{
		"group_id": ` + "1" + `,
		"name": "Legacy Create",
		"description": "legacy-desc",
		"price": 19.9,
		"original_price": 29.9,
		"validity_days": 2,
		"validity_unit": "week",
		"features": ["f1", "f2"],
		"sort_order": 9,
		"for_sale": true,
		"product_name": "Legacy Product"
	}`
	body = strings.Replace(body, `"group_id": 1`, `"group_id": `+strconv.FormatInt(groupID, 10), 1)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/subscription-plans", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.CreatePlanLegacy(c)

	require.Equal(t, http.StatusCreated, rec.Code)
	plans, err := configSvc.ListPlans(context.Background())
	require.NoError(t, err)
	require.Len(t, plans, 2)
	var created *dbent.SubscriptionPlan
	for _, plan := range plans {
		if plan.Name == "Legacy Create" {
			created = plan
			break
		}
	}
	require.NotNil(t, created)
	require.Equal(t, "f1\nf2", created.Features)
	require.Equal(t, "Legacy Product", created.ProductName)
	require.Equal(t, 9, created.SortOrder)
}

func TestUpdatePlanLegacyAcceptsSub2ApiPayPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := newPaymentPlanLegacyClient(t)
	groupID := seedLegacyPlan(t, client, true)
	configSvc := service.NewPaymentConfigService(client, nil, nil)
	h := adminhandler.NewPaymentHandler(nil, configSvc)

	plans, err := configSvc.ListPlans(context.Background())
	require.NoError(t, err)
	require.Len(t, plans, 1)
	planID := plans[0].ID

	body := `{
		"group_id": ` + strconv.FormatInt(groupID, 10) + `,
		"name": "Legacy Update",
		"features": ["u1", "u2"],
		"for_sale": false
	}`

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(planID, 10)}}
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/subscription-plans/"+strconv.FormatInt(planID, 10), strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.UpdatePlanLegacy(c)

	require.Equal(t, http.StatusOK, rec.Code)
	updated, err := configSvc.GetPlan(context.Background(), planID)
	require.NoError(t, err)
	require.Equal(t, "Legacy Update", updated.Name)
	require.Equal(t, "u1\nu2", updated.Features)
	require.False(t, updated.ForSale)
}

func TestDeletePlanLegacyDeletesPlan(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := newPaymentPlanLegacyClient(t)
	seedLegacyPlan(t, client, true)
	configSvc := service.NewPaymentConfigService(client, nil, nil)
	h := adminhandler.NewPaymentHandler(nil, configSvc)

	plans, err := configSvc.ListPlans(context.Background())
	require.NoError(t, err)
	require.Len(t, plans, 1)
	planID := plans[0].ID

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(planID, 10)}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/subscription-plans/"+strconv.FormatInt(planID, 10), nil)

	h.DeletePlanLegacy(c)

	require.Equal(t, http.StatusOK, rec.Code)
	remaining, err := configSvc.ListPlans(context.Background())
	require.NoError(t, err)
	require.Empty(t, remaining)
}
