//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	a6TestAccessToken = "test-system-token"
	a6TestUserID      = "1233"
)

// a6Recorder 在 handler goroutine 里记录观察到的请求，供测试主 goroutine 断言（race 安全）。
//
// handler 里不能用 require：它内部会 FailNow，而 FailNow 只允许在测试主 goroutine 调用。
type a6Recorder struct {
	mu      sync.Mutex
	queries []url.Values
	headers []http.Header
}

func (r *a6Recorder) add(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queries = append(r.queries, req.URL.Query())
	r.headers = append(r.headers, req.Header.Clone())
}

func (r *a6Recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.queries)
}

func (r *a6Recorder) lastQuery(t *testing.T) url.Values {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	require.NotEmpty(t, r.queries, "handler 没有被调用")
	return r.queries[len(r.queries)-1]
}

func (r *a6Recorder) lastHeader(t *testing.T) http.Header {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	require.NotEmpty(t, r.headers, "handler 没有被调用")
	return r.headers[len(r.headers)-1]
}

// newA6TestClient 起一个 httptest 服务并返回绑定其地址的客户端。
//
// /api/status 由本函数统一应答（quota_per_unit=500000），logHandler 只处理 /api/log/self。
func newA6TestClient(t *testing.T, quotaPerUnit string, logHandler http.HandlerFunc) (*A6Client, *a6Recorder) {
	t.Helper()
	recorder := &a6Recorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case a6StatusPath:
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":` + quotaPerUnit + `}}`))
		case a6SelfLogPath:
			recorder.add(r)
			logHandler(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := NewA6Client(ReconciliationA6Config{
		BaseURL:     server.URL,
		AccessToken: a6TestAccessToken,
		UserID:      a6TestUserID,
		Timeout:     10 * time.Second,
	})
	return client, recorder
}

func a6TestQuery() A6BillQuery {
	return A6BillQuery{
		TokenName: "chenshu-a",
		StartTime: time.Unix(1730000000, 0).UTC(),
		EndTime:   time.Unix(1730003600, 0).UTC(),
	}
}

func a6WriteJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func TestReconciliationA6ClientFetchBillsPageParsesRecordAndCost(t *testing.T) {
	client, recorder := newA6TestClient(t, "500000", func(w http.ResponseWriter, _ *http.Request) {
		a6WriteJSON(w, `{"success":true,"message":"","data":{
			"items":[{
				"request_id":"req-1",
				"created_at":1730000100,
				"model_name":"claude-sonnet-4-5",
				"token_name":"chenshu-a",
				"prompt_tokens":1200,
				"completion_tokens":340,
				"cache_read_tokens":800,
				"cache_creation_tokens":120,
				"quota":250000,
				"other":{"cache_tokens":920,"group_ratio":1.5}
			}],
			"total":1,"page":1,"page_size":100}}`)
	})

	page, err := client.FetchBillsPage(context.Background(), a6TestQuery(), 1)
	require.NoError(t, err)
	require.Equal(t, 1, page.Total)
	require.Equal(t, 1, page.Page)
	require.Equal(t, 100, page.PageSize)
	require.False(t, page.HasMore)
	require.Len(t, page.Items, 1)

	header := recorder.lastHeader(t)
	require.Equal(t, "Bearer "+a6TestAccessToken, header.Get("Authorization"))
	require.Equal(t, a6TestUserID, header.Get("New-API-User"), "New-API-User 必须带上用户标识")
	require.Equal(t, "no-store", header.Get("Cache-Control"))

	query := recorder.lastQuery(t)
	require.Equal(t, "1", query.Get("p"))
	require.Equal(t, "100", query.Get("page_size"))
	require.Equal(t, "2", query.Get("type"), "消费类日志固定传 2")
	require.Equal(t, "chenshu-a", query.Get("token_name"))
	require.Equal(t, "1730000000", query.Get("start_timestamp"))
	require.Equal(t, "1730003600", query.Get("end_timestamp"), "客户端不得自行放大窗口终点")
	require.Empty(t, query.Get("model_name"), "模型名为空时不下发该参数")

	bill := page.Items[0]
	require.Equal(t, "req-1", bill.RequestID)
	require.Equal(t, time.Date(2024, 10, 27, 3, 35, 0, 0, time.UTC), bill.OccurredAt)
	require.Equal(t, "claude-sonnet-4-5", bill.Model)
	require.Equal(t, "chenshu-a", bill.TokenName)
	require.Equal(t, 1200, bill.InputTokens)
	require.Equal(t, 340, bill.OutputTokens)
	require.Equal(t, 800, bill.CacheReadTokens)
	require.Equal(t, 120, bill.CacheCreationTokens)
	require.Equal(t, 920, bill.CacheTokensTotal)
	require.True(t, bill.Quota.Equal(decimal.NewFromInt(250000)))
	require.True(t, bill.QuotaPerUnit.Equal(decimal.NewFromInt(500000)))
	require.True(t, bill.CostUSD.Equal(decimal.NewFromFloat(0.5)), "cost=%s", bill.CostUSD)
	require.Equal(t, json.Number("1.5"), bill.Other["group_ratio"], "UseNumber 不得把小数退化成 float64")
	require.NotEmpty(t, bill.Raw)
}

func TestReconciliationA6ClientParsesDoubleEncodedOther(t *testing.T) {
	client, _ := newA6TestClient(t, "500000", func(w http.ResponseWriter, _ *http.Request) {
		a6WriteJSON(w, `{"success":true,"data":{"items":[
			{"request_id":"req-double","created_at":1730000100,"token_name":"t","quota":500000,
			 "other":"{\"cache_tokens\":777,\"billing_source\":\"api\"}"},
			{"request_id":"req-broken","created_at":1730000200,"token_name":"t","quota":500000,
			 "other":"{not-json"},
			{"request_id":"req-null","created_at":1730000300,"token_name":"t","quota":500000,"other":null}
		],"total":3}}`)
	})

	page, err := client.FetchBillsPage(context.Background(), a6TestQuery(), 1)
	require.NoError(t, err)
	require.Len(t, page.Items, 3, "other 解析失败不得丢掉整条记录")

	require.Equal(t, json.Number("777"), page.Items[0].Other["cache_tokens"])
	require.Equal(t, "api", page.Items[0].Other["billing_source"])
	require.Equal(t, 777, page.Items[0].CacheTokensTotal, "只给合并缓存值时落到 CacheTokensTotal")
	require.Nil(t, page.Items[1].Other, "坏 other 记 nil 而不是报错")
	require.Nil(t, page.Items[2].Other)
}

func TestReconciliationA6ClientAcceptsDataAsArray(t *testing.T) {
	client, _ := newA6TestClient(t, "500000", func(w http.ResponseWriter, _ *http.Request) {
		a6WriteJSON(w, `{"success":true,"data":[
			{"request_id":"req-arr","created_at":"1730000100","token_name":"t","quota":"500000"}
		]}`)
	})

	page, err := client.FetchBillsPage(context.Background(), a6TestQuery(), 1)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, a6TotalUnknown, page.Total, "数组形态没有 total")
	require.False(t, page.HasMore)
	require.True(t, page.Items[0].CostUSD.Equal(decimal.NewFromInt(1)), "cost=%s", page.Items[0].CostUSD)
}

func TestReconciliationA6ClientRetriesEachPageThreeTimes(t *testing.T) {
	var attempts int32
	client, _ := newA6TestClient(t, "500000", func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&attempts, 1) < a6MaxAttempts {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		a6WriteJSON(w, `{"success":true,"data":{"items":[
			{"request_id":"req-retry","created_at":1730000100,"token_name":"t","quota":500000}
		],"total":1}}`)
	})

	start := time.Now()
	page, err := client.FetchBillsPage(context.Background(), a6TestQuery(), 1)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.Equal(t, int32(a6MaxAttempts), atomic.LoadInt32(&attempts))
	require.Len(t, page.Items, 1)
	require.GreaterOrEqual(t, elapsed, 1500*time.Millisecond, "退避应为 0.5s + 1.0s")
}

func TestReconciliationA6ClientAuthFailureIsDistinguishable(t *testing.T) {
	var attempts int32
	client, _ := newA6TestClient(t, "500000", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := client.FetchBillsPage(context.Background(), a6TestQuery(), 1)
	require.Error(t, err)
	require.True(t, IsReconciliationA6AuthError(err))
	require.False(t, errors.Is(err, ErrReconciliationA6RequestFailed), "认证失败不得与普通请求失败混同")
	require.False(t, errors.Is(err, ErrReconciliationA6Timeout))
	require.Equal(t, "RECONCILIATION_A6_AUTH_FAILED", infraerrors.Reason(err))
	require.Equal(t, http.StatusBadGateway, infraerrors.Code(err), "对账接口不得向上返回 401/403")
	require.Equal(t, int32(a6MaxAttempts), atomic.LoadInt32(&attempts))

	forbidden, _ := newA6TestClient(t, "500000", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	_, err = forbidden.FetchBillsPage(context.Background(), a6TestQuery(), 1)
	require.True(t, IsReconciliationA6AuthError(err), "403 同样归入凭据类失败")
}

// TestReconciliationA6ClientAuthFailureCarriesUpstreamBodySnippet 验证 401 时不再丢弃 A6 的响应体。
//
// A6 是 new-api 部署，它把真正的原因写在 401 的 body 里；旧实现在判断状态码之前就把 body
// 丢进 io.Discard，于是线上只能看到笼统的 upstream 401，管理员只能靠猜。
func TestReconciliationA6ClientAuthFailureCarriesUpstreamBodySnippet(t *testing.T) {
	const upstreamBody = `{"message":"Unauthorized, invalid access token","success":false}`
	client, _ := newA6TestClient(t, "500000", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(upstreamBody))
	})

	_, err := client.FetchBillsPage(context.Background(), a6TestQuery(), 1)
	require.Error(t, err)
	require.True(t, IsReconciliationA6AuthError(err))

	appErr := infraerrors.FromError(err)
	// 契约不变：认证失败仍然映射成 502，绝不向上返回 401/403。
	require.Equal(t, http.StatusBadGateway, infraerrors.Code(err))
	require.Equal(t, "RECONCILIATION_A6_AUTH_FAILED", appErr.Reason)
	require.Equal(t, "401", appErr.Metadata["upstream_status"], "metadata.upstream_status 必须保留")
	require.Contains(t, appErr.Metadata["upstream_message"], "invalid access token",
		"401 的错误详情必须带上 A6 的原话")
	// err.Error() 就是 getJSONWithRetry 用 err=%v 打进服务端日志、以及 recordSyncError
	// 落进 reconciliation_sync_state 的那份文本，因此它也必须带原话。
	require.Contains(t, err.Error(), "invalid access token")
}

// TestReconciliationA6ClientErrorBodySnippetIsRedacted 验证响应体片段先脱敏再外流。
func TestReconciliationA6ClientErrorBodySnippetIsRedacted(t *testing.T) {
	const leakedSecret = "sk-abcdefghijklmnopqrstuvwxyz"
	body := `{"message":"invalid access token ` + a6TestAccessToken +
		`","access_token":"` + a6TestAccessToken +
		`","hint":"Authorization: Bearer ` + a6TestAccessToken +
		`","key":"` + leakedSecret + `"}`
	client, _ := newA6TestClient(t, "500000", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(body))
	})

	_, err := client.getJSON(context.Background(), client.Config(), a6SelfLogPath,
		a6BillQueryParams(a6TestQuery(), 1, a6DefaultPageSize))
	require.Error(t, err)

	appErr := infraerrors.FromError(err)
	snippet := appErr.Metadata["upstream_message"]
	require.NotEmpty(t, snippet, "401 的错误详情必须带响应体片段")
	require.Equal(t, "401", appErr.Metadata["upstream_status"])
	// 用 assert 而不是 require：任何一个脱敏口子破了都要看到全部结果，不能被 FailNow 截断。
	assert.NotContains(t, snippet, a6TestAccessToken, "本次请求使用的访问令牌绝不能外流")
	assert.NotContains(t, snippet, leakedSecret, "上游回显的 sk- 密钥必须被遮蔽")
	assert.Contains(t, snippet, `"access_token":"`+a6RedactedValue+`"`, "按键名遮蔽值、保留键名")
	assert.Contains(t, snippet, "Authorization: "+a6RedactedValue, "Authorization 回显必须被整段遮蔽")
}

// TestReconciliationA6ClientErrorBodySnippetIsTruncated 验证超大响应体只留一小段。
func TestReconciliationA6ClientErrorBodySnippetIsTruncated(t *testing.T) {
	client, _ := newA6TestClient(t, "500000", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"` + strings.Repeat("x", a6ErrorBodySnippetBytes*2) + `"}`))
	})

	_, err := client.getJSON(context.Background(), client.Config(), a6SelfLogPath,
		a6BillQueryParams(a6TestQuery(), 1, a6DefaultPageSize))
	require.Error(t, err)

	appErr := infraerrors.FromError(err)
	snippet := appErr.Metadata["upstream_message"]
	require.NotEmpty(t, snippet, "非认证类的非 2xx 同样要带上原话")
	require.Equal(t, "RECONCILIATION_A6_REQUEST_FAILED", appErr.Reason)
	require.Equal(t, "500", appErr.Metadata["upstream_status"])
	assert.True(t, len(snippet) <= a6ErrorBodySnippetBytes+len("..."),
		"片段必须被截断，避免超大 body 污染日志，实际长度=%d", len(snippet))
	assert.True(t, strings.HasSuffix(snippet, "..."), "截断后应带省略号")
}

// TestReconciliationA6ClientEmptyErrorBodyKeepsUpstreamStatus 验证空响应体不产生空片段。
func TestReconciliationA6ClientEmptyErrorBodyKeepsUpstreamStatus(t *testing.T) {
	client, _ := newA6TestClient(t, "500000", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	_, err := client.getJSON(context.Background(), client.Config(), a6SelfLogPath,
		a6BillQueryParams(a6TestQuery(), 1, a6DefaultPageSize))
	require.Error(t, err)

	appErr := infraerrors.FromError(err)
	require.Equal(t, "403", appErr.Metadata["upstream_status"])
	_, hasSnippet := appErr.Metadata["upstream_message"]
	assert.False(t, hasSnippet, "空 body 不该塞一个空的 upstream_message 进详情")
}

func TestReconciliationA6ClientUpstreamRejectionIsNotARetryableShape(t *testing.T) {
	client, _ := newA6TestClient(t, "500000", func(w http.ResponseWriter, _ *http.Request) {
		a6WriteJSON(w, `{"success":false,"message":"无效的令牌"}`)
	})

	_, err := client.FetchBillsPage(context.Background(), a6TestQuery(), 1)
	require.True(t, errors.Is(err, ErrReconciliationA6Rejected))
	require.Contains(t, err.Error(), "无效的令牌", "上游拒绝原因必须保留给运维排查")
}

func TestReconciliationA6ClientRejectsOversizedResponse(t *testing.T) {
	client, _ := newA6TestClient(t, "500000", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", a6MaxResponseBytes+1)))
	})

	_, err := client.FetchBillsPage(context.Background(), a6TestQuery(), 1)
	require.True(t, errors.Is(err, ErrReconciliationA6ResponseTooLarge), "err=%v", err)
}

func TestReconciliationA6ClientFetchBillsPagesUntilTotalReached(t *testing.T) {
	client, recorder := newA6TestClient(t, "500000", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("p") {
		case "1":
			a6WriteJSON(w, `{"success":true,"data":{"items":[
				{"request_id":"req-1","created_at":1730000100,"token_name":"t","quota":500000},
				{"request_id":"req-2","created_at":1730000200,"token_name":"t","quota":500000}
			],"total":3,"page":1,"page_size":2}}`)
		case "2":
			a6WriteJSON(w, `{"success":true,"data":{"items":[
				{"request_id":"req-3","created_at":1730000300,"token_name":"t","quota":500000}
			],"total":3,"page":2,"page_size":2}}`)
		default:
			t.Errorf("意外的页码 %q", r.URL.Query().Get("p"))
			http.NotFound(w, r)
		}
	})

	query := a6TestQuery()
	query.PageSize = 2
	bills, err := client.FetchBills(context.Background(), query)
	require.NoError(t, err)
	require.Len(t, bills, 3)
	require.Equal(t, []string{"req-1", "req-2", "req-3"}, []string{bills[0].RequestID, bills[1].RequestID, bills[2].RequestID})
	require.Equal(t, 2, recorder.count(), "两页取完即停")
}

func TestReconciliationA6ClientPageLimitReturnsPartialBills(t *testing.T) {
	client, _ := newA6TestClient(t, "500000", func(w http.ResponseWriter, _ *http.Request) {
		a6WriteJSON(w, `{"success":true,"data":{"items":[
			{"request_id":"req-limit","created_at":1730000100,"token_name":"t","quota":500000}
		],"total":10,"page":1,"page_size":1}}`)
	})

	query := a6TestQuery()
	query.PageSize = 1
	query.MaxPages = 1
	bills, err := client.FetchBills(context.Background(), query)
	require.True(t, errors.Is(err, ErrReconciliationA6PageLimitReached), "err=%v", err)
	require.Len(t, bills, 1, "达到上限也要把已取到的账单交回调用方")
}

func TestReconciliationA6ClientCachesQuotaPerUnitAndKeepsLastValue(t *testing.T) {
	var statusCalls int32
	var statusFailed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case a6StatusPath:
			atomic.AddInt32(&statusCalls, 1)
			if statusFailed.Load() {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			a6WriteJSON(w, `{"success":true,"data":{"quota_per_unit":"250000"}}`)
		case a6SelfLogPath:
			a6WriteJSON(w, `{"success":true,"data":{"items":[
				{"request_id":"req-q","created_at":1730000100,"token_name":"t","quota":250000}
			],"total":1}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := NewA6Client(ReconciliationA6Config{
		BaseURL: server.URL, AccessToken: a6TestAccessToken, UserID: a6TestUserID, Timeout: 5 * time.Second,
	})

	first, err := client.QuotaPerUnit(context.Background())
	require.NoError(t, err)
	require.True(t, first.Equal(decimal.NewFromInt(250000)), "字符串形态的 quota_per_unit 也要能解析")
	_, err = client.QuotaPerUnit(context.Background())
	require.NoError(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&statusCalls), "TTL 内不得重复请求 /api/status")

	// 让缓存过期且 /api/status 开始失败：应沿用上一次成功值，账单照常换算。
	statusFailed.Store(true)
	client.mu.Lock()
	client.quotaFetchedAt = client.quotaFetchedAt.Add(-2 * a6QuotaPerUnitTTL)
	client.mu.Unlock()

	kept, err := client.QuotaPerUnit(context.Background())
	require.NoError(t, err)
	require.True(t, kept.Equal(decimal.NewFromInt(250000)), "刷新失败必须保留上一次的值")

	page, err := client.FetchBillsPage(context.Background(), a6TestQuery(), 1)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.True(t, page.Items[0].CostUSD.Equal(decimal.NewFromInt(1)), "cost=%s", page.Items[0].CostUSD)
}

func TestReconciliationA6ClientQuotaPerUnitUnavailableWhenNeverFetched(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	client := NewA6Client(ReconciliationA6Config{
		BaseURL: server.URL, AccessToken: a6TestAccessToken, UserID: a6TestUserID, Timeout: 5 * time.Second,
	})

	_, err := client.QuotaPerUnit(context.Background())
	require.True(t, errors.Is(err, ErrReconciliationA6QuotaPerUnitUnavailable), "err=%v", err)
}

func TestReconciliationA6ClientRespectsContextCancellation(t *testing.T) {
	var attempts int32
	client, _ := newA6TestClient(t, "500000", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.FetchBillsPage(ctx, a6TestQuery(), 1)
	require.True(t, errors.Is(err, context.Canceled), "ctx 取消必须原样暴露，err=%v", err)
	require.Equal(t, int32(0), atomic.LoadInt32(&attempts), "ctx 已取消时不得发起账单请求")
}

func TestReconciliationA6ClientNormalizeBillSkipsUnusableRecords(t *testing.T) {
	client, _ := newA6TestClient(t, "500000", func(w http.ResponseWriter, _ *http.Request) {
		a6WriteJSON(w, `{"success":true,"data":{"items":[
			{"created_at":1730000100,"quota":500000},
			{"request_id":"req-bad-time","created_at":"not-a-time","quota":500000},
			{"request_id":"req-zero-time","created_at":0,"quota":500000},
			{"request_id":"req-missing-quota","created_at":1730000100},
			{"request_id":"req-negative-quota","created_at":1730000100,"quota":-5},
			{"request_id":"req-ok","created_at":1730000100,"quota":500000}
		],"total":6}}`)
	})

	page, err := client.FetchBillsPage(context.Background(), a6TestQuery(), 1)
	require.NoError(t, err)
	require.Len(t, page.Items, 1, "不可用的记录必须被跳过，而不是以 0 成本入账")
	require.Equal(t, "req-ok", page.Items[0].RequestID)
}

func TestReconciliationA6ClientNormalizeBillAcceptsMillisAndRFC3339(t *testing.T) {
	client, _ := newA6TestClient(t, "500000", func(w http.ResponseWriter, _ *http.Request) {
		a6WriteJSON(w, `{"success":true,"data":{"items":[
			{"request_id":"req-ms","created_at":1730000100000,"token_name":"t","quota":500000},
			{"request_id":"req-rfc","created_at":"2024-10-27T04:00:00Z","token_name":"t","quota":500000},
			{"request_id":"req-space","created_at":"2024-10-27 04:00:00","token_name":"t","quota":500000}
		]}}`)
	})

	page, err := client.FetchBillsPage(context.Background(), a6TestQuery(), 1)
	require.NoError(t, err)
	require.Len(t, page.Items, 3)
	require.Equal(t, time.Date(2024, 10, 27, 3, 35, 0, 0, time.UTC), page.Items[0].OccurredAt, "毫秒时间戳要按毫秒解释")
	require.Equal(t, time.Date(2024, 10, 27, 4, 0, 0, 0, time.UTC), page.Items[1].OccurredAt)
	require.Equal(t, time.Date(2024, 10, 27, 4, 0, 0, 0, time.UTC), page.Items[2].OccurredAt)
	require.Equal(t, time.UTC, page.Items[0].OccurredAt.Location())
}

func TestReconciliationA6ClientRejectsInvalidWindow(t *testing.T) {
	client, recorder := newA6TestClient(t, "500000", func(w http.ResponseWriter, _ *http.Request) {
		t.Error("窗口非法时不应发起请求")
	})

	query := a6TestQuery()
	query.EndTime = query.StartTime.Add(-time.Minute)
	_, err := client.FetchBillsPage(context.Background(), query, 1)
	require.True(t, errors.Is(err, ErrReconciliationA6InvalidWindow), "err=%v", err)
	require.Equal(t, 0, recorder.count())

	_, err = client.FetchBills(context.Background(), A6BillQuery{})
	require.True(t, errors.Is(err, ErrReconciliationA6InvalidWindow))
}

func TestReconciliationA6ClientUnconfiguredIsActionable(t *testing.T) {
	client := NewA6Client(ReconciliationA6Config{BaseURL: "https://a6.example.com"})
	require.False(t, client.Configured())

	_, err := client.FetchBillsPage(context.Background(), a6TestQuery(), 1)
	require.True(t, errors.Is(err, ErrReconciliationA6NotConfigured))

	configured := NewA6Client(ReconciliationA6Config{
		BaseURL: "https://a6.example.com/", AccessToken: a6TestAccessToken, UserID: a6TestUserID,
	})
	require.True(t, configured.Configured())
	require.Equal(t, "https://a6.example.com", configured.cfg.BaseURL, "基址末尾斜杠应被规整")
	require.Equal(t, a6DefaultTimeout, configured.httpClient.Timeout, "未注入 Timeout 时用默认 90 秒")

	badScheme := NewA6Client(ReconciliationA6Config{
		BaseURL: "a6.example.com", AccessToken: a6TestAccessToken, UserID: a6TestUserID,
	})
	require.False(t, badScheme.Configured(), "非绝对 HTTP(S) 基址应被判为未配置")
}

func TestReconciliationA6ClientNormalizeBillHandlesFloatNumbers(t *testing.T) {
	// 手工重放 raw jsonb 时数值可能是 float64（解码未开 UseNumber），也必须能解析。
	client := NewA6Client(ReconciliationA6Config{
		BaseURL: "https://a6.example.com", AccessToken: a6TestAccessToken, UserID: a6TestUserID,
	})
	bill, ok := client.NormalizeBill(map[string]any{
		"request_id":        "req-float",
		"created_at":        float64(1730000100),
		"model_name":        "gpt-5",
		"token_name":        "t",
		"prompt_tokens":     float64(10),
		"completion_tokens": 20,
		"quota":             float64(500000),
		"other":             "{\"cache_tokens\":42}",
	}, decimal.NewFromInt(500000))

	require.True(t, ok)
	require.Equal(t, time.Date(2024, 10, 27, 3, 35, 0, 0, time.UTC), bill.OccurredAt)
	require.Equal(t, 10, bill.InputTokens)
	require.Equal(t, 20, bill.OutputTokens)
	require.Equal(t, 42, bill.CacheTokensTotal)
	require.True(t, bill.CostUSD.Equal(decimal.NewFromInt(1)), "cost=%s", bill.CostUSD)

	// quotaPerUnit 传 0 时按 1 处理，并如实记录，便于发现误用。
	fallback, ok := client.NormalizeBill(map[string]any{
		"request_id": "req-fallback", "created_at": 1730000100, "quota": 7,
	}, decimal.Zero)
	require.True(t, ok)
	require.True(t, fallback.QuotaPerUnit.Equal(decimal.NewFromInt(1)))
	require.True(t, fallback.CostUSD.Equal(decimal.NewFromInt(7)))

	_, ok = client.NormalizeBill(nil, decimal.NewFromInt(1))
	require.False(t, ok, "空记录不可用")
}

// 上游首字/耗时的提取规则。
//
// 这一对的**核心语义是「缺失 ≠ 0」**：页面上一列显示「0ms」与显示「—」的排障结论
// 完全相反，所以本用例把「上游回了 0」与「上游没回」两条分支分别钉死。
func TestReconciliationA6ClientNormalizeBillExtractsUpstreamLatency(t *testing.T) {
	client := NewA6Client(ReconciliationA6Config{
		BaseURL: "https://a6.example.com", AccessToken: a6TestAccessToken, UserID: a6TestUserID,
	})
	base := map[string]any{
		"request_id": "req-latency",
		"created_at": 1730000100,
		"quota":      500000,
	}

	t.Run("frt 在 other 里、use_time 在顶层，且 use_time 由秒换算为毫秒", func(t *testing.T) {
		raw := map[string]any{}
		for k, v := range base {
			raw[k] = v
		}
		raw["other"] = "{\"frt\":1234}"
		raw["use_time"] = 5

		bill, ok := client.NormalizeBill(raw, decimal.NewFromInt(500000))
		require.True(t, ok)
		require.NotNil(t, bill.FirstTokenMs)
		require.Equal(t, 1234, *bill.FirstTokenMs)
		require.NotNil(t, bill.UpstreamDurationMs)
		require.Equal(t, 5000, *bill.UpstreamDurationMs, "use_time 是整秒，必须 ×1000 换成毫秒")
	})

	t.Run("上游回 0 与上游没回必须区分开", func(t *testing.T) {
		// 上游确实回了 0：指针非空、值为 0 —— 页面应显示「0ms」。
		zero := map[string]any{}
		for k, v := range base {
			zero[k] = v
		}
		zero["other"] = map[string]any{"frt": 0}
		zero["use_time"] = 0

		bill, ok := client.NormalizeBill(zero, decimal.NewFromInt(500000))
		require.True(t, ok)
		require.NotNil(t, bill.FirstTokenMs, "上游回 0 时指针必须非空，否则页面会误显示为「未取到」")
		require.Equal(t, 0, *bill.FirstTokenMs)
		require.NotNil(t, bill.UpstreamDurationMs)
		require.Equal(t, 0, *bill.UpstreamDurationMs)

		// 上游完全没回：指针必须为 nil —— 页面应显示「—」。
		absent, ok := client.NormalizeBill(base, decimal.NewFromInt(500000))
		require.True(t, ok)
		require.Nil(t, absent.FirstTokenMs, "字段缺失必须是 nil，不能折成 0")
		require.Nil(t, absent.UpstreamDurationMs, "字段缺失必须是 nil，不能折成 0")
	})

	t.Run("非法的 frt / use_time 一律降级为 nil，且不影响账单本身可用", func(t *testing.T) {
		raw := map[string]any{}
		for k, v := range base {
			raw[k] = v
		}
		raw["other"] = map[string]any{"frt": "not-a-number"}
		raw["use_time"] = -3

		bill, ok := client.NormalizeBill(raw, decimal.NewFromInt(500000))
		require.True(t, ok, "耗时取不到不该让整条账单作废——成本仍然是有效数据")
		require.Nil(t, bill.FirstTokenMs)
		require.Nil(t, bill.UpstreamDurationMs)
		require.True(t, bill.CostUSD.Equal(decimal.NewFromInt(1)), "金额不受影响")
	})

	t.Run("other 为被双重编码的 JSON 字符串时也能取到 frt", func(t *testing.T) {
		raw := map[string]any{}
		for k, v := range base {
			raw[k] = v
		}
		raw["other"] = "{\"frt\":2500,\"cache_tokens\":42}"

		bill, ok := client.NormalizeBill(raw, decimal.NewFromInt(500000))
		require.True(t, ok)
		require.NotNil(t, bill.FirstTokenMs)
		require.Equal(t, 2500, *bill.FirstTokenMs)
		require.Equal(t, 42, bill.CacheTokensTotal, "同一条 other 里的既有字段不受影响")
	})
}
