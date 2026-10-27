package admin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 只覆盖 handler 自己的职责：body 绑定、HTTP 状态码与响应形状。
// 用卡流程、后处理与错误分类由 service 层测试覆盖。

type zhipuResetHandlerRepo struct {
	service.AccountRepository
	account *service.Account
}

func (r *zhipuResetHandlerRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	if r.account == nil || r.account.ID != id {
		return nil, service.ErrAccountNotFound
	}
	cloned := *r.account
	return &cloned, nil
}

func (r *zhipuResetHandlerRepo) UpdateExtra(context.Context, int64, map[string]any) error { return nil }

// zhipuResetHandlerUpstream 按顺序回放 200 响应体，记录请求。
type zhipuResetHandlerUpstream struct {
	mu        sync.Mutex
	responses []string
	requests  int
}

func (u *zhipuResetHandlerUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.requests++
	if len(u.responses) == 0 {
		return nil, errors.New("no scripted response")
	}
	body := u.responses[0]
	u.responses = u.responses[1:]
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
}

func (u *zhipuResetHandlerUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

const (
	zhipuResetHandlerList = `{"code":200,"success":true,"data":{
		"weekResets":[{"recordId":301,"grantType":"PACKAGE_GIFT","expireTime":"2026-10-05 23:59:59","available":true}],
		"fiveHourResets":[{"recordId":401,"grantType":"ACTIVITY","expireTime":"2026-10-10 12:00:00","available":true}]
	}}`
	zhipuResetHandlerEmptyList = `{"code":200,"success":true,"data":{"weekResets":[],"fiveHourResets":[]}}`
	zhipuResetHandlerUseOK     = `{"code":200,"success":true,"msg":"操作成功"}`
	zhipuResetHandlerQuota     = `{"success":true,"data":{"level":"pro","limits":[
		{"type":"TOKENS_LIMIT","unit":3,"percentage":0,"nextResetTime":1790000000000},
		{"type":"TOKENS_LIMIT","unit":6,"percentage":0,"nextResetTime":1790500000000}
	]}}`
)

func serveZhipuReset(t *testing.T, method, body string, responses ...string) (*httptest.ResponseRecorder, *zhipuResetHandlerUpstream) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	account := &service.Account{
		ID:          81,
		Platform:    service.PlatformZhipu,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Credentials: map[string]any{"api_key": "sk-test", "account_mode": service.AccountModeCoding},
	}
	upstream := &zhipuResetHandlerUpstream{responses: responses}
	h := NewCNProviderHandler(service.NewCNProviderQuotaService(&zhipuResetHandlerRepo{account: account}, nil, upstream, nil), nil)
	router := gin.New()
	router.GET("/admin/cn-providers/accounts/:id/reset-quota", h.ListResetCards)
	router.POST("/admin/cn-providers/accounts/:id/reset-quota", h.UseResetCard)

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, "/admin/cn-providers/accounts/81/reset-quota", reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w, upstream
}

func TestCNProviderHandler_UseResetCard(t *testing.T) {
	success := []string{zhipuResetHandlerList, zhipuResetHandlerUseOK, zhipuResetHandlerList, zhipuResetHandlerQuota}
	cases := []struct {
		name         string
		body         string
		responses    []string
		wantStatus   int
		wantReason   string
		wantType     string
		wantRecord   int64
		wantUpstream bool
	}{
		{name: "empty body uses the defaults", responses: success, wantStatus: http.StatusOK, wantType: "WEEK", wantRecord: 301, wantUpstream: true},
		{name: "body selects the card", body: `{"reset_type":"FIVE_HOUR","record_id":401}`, responses: success, wantStatus: http.StatusOK, wantType: "FIVE_HOUR", wantRecord: 401, wantUpstream: true},
		{name: "malformed json", body: `{"reset_type":`, wantStatus: http.StatusBadRequest},
		{name: "unknown reset_type", body: `{"reset_type":"MONTH"}`, wantStatus: http.StatusBadRequest, wantReason: service.ZhipuResetErrInvalidType},
		{name: "failed use is a 400 with reason", responses: []string{zhipuResetHandlerEmptyList}, wantStatus: http.StatusBadRequest, wantReason: service.ZhipuResetErrNoCard, wantUpstream: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, upstream := serveZhipuReset(t, http.MethodPost, tc.body, tc.responses...)
			require.Equal(t, tc.wantStatus, w.Code, w.Body.String())
			require.Equal(t, tc.wantUpstream, upstream.requests > 0)
			resp := gjson.Parse(w.Body.String())

			if tc.wantStatus != http.StatusOK {
				require.NotEmpty(t, resp.Get("message").String())
				if tc.wantReason != "" {
					require.Equal(t, tc.wantReason, resp.Get("reason").String())
				}
				return
			}
			data := resp.Get("data")
			require.True(t, data.Get("success").Bool())
			require.Equal(t, tc.wantType, data.Get("reset_type").String())
			require.Equal(t, tc.wantRecord, data.Get("record_id").Int())
			for _, field := range []string{"window", "week_resets_left", "five_hour_resets_left", "account_state_recovered", "cards.week_cards", "cards.five_hour_cards", "probe.success", "probe.tiers"} {
				require.Truef(t, data.Get(field).Exists(), "response is missing %s", field)
			}
			// 这里没有注入 recoverer：恢复失败以 warning_code 体现，HTTP 仍是 200。
			require.Equal(t, service.ZhipuResetWarningAccountRecoveryFailed, data.Get("warning_code").String())
		})
	}
}

func TestCNProviderHandler_ListResetCards(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		w, _ := serveZhipuReset(t, http.MethodGet, "", zhipuResetHandlerList)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		data := gjson.Get(w.Body.String(), "data")
		require.Equal(t, int64(301), data.Get("week_cards.0.record_id").Int())
		require.Equal(t, "PACKAGE_GIFT", data.Get("week_cards.0.grant_type").String())
		require.Equal(t, "2026-10-05 23:59:59", data.Get("week_cards.0.expire_time").String())
		require.Equal(t, int64(401), data.Get("five_hour_cards.0.record_id").Int())
		require.True(t, data.Get("persisted").Bool())
	})

	t.Run("upstream failure is a 502 with reason", func(t *testing.T) {
		w, _ := serveZhipuReset(t, http.MethodGet, "", `{"code":500,"success":false,"msg":"系统繁忙"}`)
		require.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())
		require.Equal(t, service.ZhipuResetErrRejected, gjson.Get(w.Body.String(), "reason").String())
		require.Contains(t, gjson.Get(w.Body.String(), "message").String(), "系统繁忙")
	})
}
