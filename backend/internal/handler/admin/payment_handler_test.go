package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestPaymentHandlerListPlansIncludesGroupQuotaAndModels(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", "file:admin_payment_handler_list_plans?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	dailyLimit := 12.5
	weeklyLimit := 50.0
	monthlyLimit := 180.0

	group, err := client.Group.Create().
		SetName("Quota Group").
		SetPlatform("openai").
		SetDescription("group with limits").
		SetRateMultiplier(1.5).
		SetStatus("active").
		SetSubscriptionType("subscription").
		SetDailyLimitUsd(dailyLimit).
		SetWeeklyLimitUsd(weeklyLimit).
		SetMonthlyLimitUsd(monthlyLimit).
		SetSupportedModelScopes([]string{"claude", "gemini_text"}).
		Save(context.Background())
	require.NoError(t, err)

	_, err = client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("Quota Plan").
		SetDescription("plan description").
		SetPrice(29.9).
		SetValidityDays(30).
		SetValidityUnit("days").
		SetFeatures("Feature A\nFeature B").
		SetProductName("Quota Product").
		SetExternalSubscribeDialogText("联系客服开通").
		SetForSale(true).
		SetSortOrder(1).
		Save(context.Background())
	require.NoError(t, err)

	handler := NewPaymentHandler(nil, service.NewPaymentConfigService(client, nil, nil))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/payment/plans", nil)

	handler.ListPlans(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data []struct {
			Name                        string   `json:"name"`
			DailyLimitUSD               *float64 `json:"daily_limit_usd"`
			WeeklyLimitUSD              *float64 `json:"weekly_limit_usd"`
			MonthlyLimitUSD             *float64 `json:"monthly_limit_usd"`
			SupportedModelScopes        []string `json:"supported_model_scopes"`
			ExternalSubscribeDialogText string   `json:"external_subscribe_dialog_text"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data, 1)
	require.Equal(t, "Quota Plan", resp.Data[0].Name)
	require.NotNil(t, resp.Data[0].DailyLimitUSD)
	require.Equal(t, dailyLimit, *resp.Data[0].DailyLimitUSD)
	require.NotNil(t, resp.Data[0].WeeklyLimitUSD)
	require.Equal(t, weeklyLimit, *resp.Data[0].WeeklyLimitUSD)
	require.NotNil(t, resp.Data[0].MonthlyLimitUSD)
	require.Equal(t, monthlyLimit, *resp.Data[0].MonthlyLimitUSD)
	require.Equal(t, []string{"claude", "gemini_text"}, resp.Data[0].SupportedModelScopes)
	require.Equal(t, "联系客服开通", resp.Data[0].ExternalSubscribeDialogText)
}

func TestPaymentHandlerCreatePlanAllowsDialogOnlyExternalSubscribeTarget(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", "file:admin_payment_handler_create_dialog_plan?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	group, err := client.Group.Create().
		SetName("Dialog Group").
		SetPlatform("openai").
		SetDescription("subscription group").
		SetRateMultiplier(1).
		SetStatus("active").
		SetSubscriptionType("subscription").
		Save(context.Background())
	require.NoError(t, err)

	handler := NewPaymentHandler(nil, service.NewPaymentConfigService(client, nil, nil))
	body := `{
		"group_id": %d,
		"name": "Dialog Plan",
		"description": "dialog only",
		"price": 29.9,
		"original_price": 0,
		"validity_days": 30,
		"validity_unit": "days",
		"features": "Feature A",
		"product_name": "",
		"external_subscribe_enabled": true,
		"external_subscribe_url": "",
		"external_subscribe_dialog_text": "联系客服开通",
		"for_sale": true,
		"sort_order": 1
	}`

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/payment/plans",
		strings.NewReader(fmt.Sprintf(body, group.ID)),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	handler.CreatePlan(c)

	require.Equal(t, http.StatusCreated, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			ExternalSubscribeURL        string `json:"external_subscribe_url"`
			ExternalSubscribeDialogText string `json:"external_subscribe_dialog_text"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, "", resp.Data.ExternalSubscribeURL)
	require.Equal(t, "联系客服开通", resp.Data.ExternalSubscribeDialogText)
}

func TestPaymentHandlerUpdatePlanAllowsDialogOnlyExternalSubscribeTarget(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite", "file:admin_payment_handler_update_dialog_plan?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	group, err := client.Group.Create().
		SetName("Dialog Group").
		SetPlatform("openai").
		SetDescription("subscription group").
		SetRateMultiplier(1).
		SetStatus("active").
		SetSubscriptionType("subscription").
		Save(context.Background())
	require.NoError(t, err)

	plan, err := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("Existing Plan").
		SetDescription("existing").
		SetPrice(19.9).
		SetValidityDays(30).
		SetValidityUnit("days").
		SetFeatures("Feature A").
		SetProductName("").
		SetExternalSubscribeEnabled(false).
		SetExternalSubscribeURL("").
		SetExternalSubscribeDialogText("").
		SetForSale(true).
		SetSortOrder(1).
		Save(context.Background())
	require.NoError(t, err)

	handler := NewPaymentHandler(nil, service.NewPaymentConfigService(client, nil, nil))
	body := `{
		"group_id": %d,
		"name": "Existing Plan",
		"description": "dialog only",
		"price": 29.9,
		"original_price": 0,
		"validity_days": 30,
		"validity_unit": "days",
		"features": "Feature A",
		"product_name": "",
		"external_subscribe_enabled": true,
		"external_subscribe_url": "",
		"external_subscribe_dialog_text": "联系客服开通",
		"for_sale": true,
		"sort_order": 1
	}`

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", plan.ID)}}
	c.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/v1/admin/payment/plans/"+fmt.Sprintf("%d", plan.ID),
		strings.NewReader(fmt.Sprintf(body, group.ID)),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdatePlan(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			ExternalSubscribeEnabled    bool   `json:"external_subscribe_enabled"`
			ExternalSubscribeURL        string `json:"external_subscribe_url"`
			ExternalSubscribeDialogText string `json:"external_subscribe_dialog_text"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.True(t, resp.Data.ExternalSubscribeEnabled)
	require.Equal(t, "", resp.Data.ExternalSubscribeURL)
	require.Equal(t, "联系客服开通", resp.Data.ExternalSubscribeDialogText)
}
