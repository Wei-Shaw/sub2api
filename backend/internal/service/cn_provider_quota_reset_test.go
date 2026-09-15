package service

// 智谱周额度重置（ResetUsage）链路测试：
//   - targetType 判定（团队凭据 → TEAM，个人 → PERSONAL）
//   - list → use → 再 list/探测 的完整成功链路（含 POST body 与 Extra 快照断言）
//   - 无可用重置次数时不发 use 请求；上游业务错误透传；非 zhipu 平台拒绝

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

// resetCall 记录一次上游调用的请求要素。
type resetCall struct {
	method string
	path   string
	query  string
	body   string
}

// stubResetUpstream 按调用顺序依次弹出预设响应，并记录请求。
type stubResetUpstream struct {
	queue []*http.Response
	calls []resetCall
}

func (u *stubResetUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	u.record(req)
	return u.next(), nil
}

func (u *stubResetUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	u.record(req)
	return u.next(), nil
}

func (u *stubResetUpstream) record(req *http.Request) {
	var body string
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		body = string(b)
	}
	u.calls = append(u.calls, resetCall{method: req.Method, path: req.URL.Path, query: req.URL.RawQuery, body: body})
}

func (u *stubResetUpstream) next() *http.Response {
	if len(u.queue) == 0 {
		return jsonResponse(`{"success":false,"msg":"no queued stub response"}`)
	}
	resp := u.queue[0]
	u.queue = u.queue[1:]
	return resp
}

func jsonResponse(body string) *http.Response {
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/json")
	rec.Write([]byte(body))
	return rec.Result()
}

// resetAccountRepo 仅实现 UpdateExtra，其余方法嵌入 nil 接口（被调用即 panic）。
type resetAccountRepo struct {
	AccountRepository
	extraUpdates []map[string]any
}

func (r *resetAccountRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.extraUpdates = append(r.extraUpdates, updates)
	return nil
}

func zhipuResetTestAccount() *Account {
	return &Account{ID: 10, Platform: PlatformZhipu, Type: AccountTypeAPIKey, Concurrency: 2,
		Credentials: map[string]any{
			"account_mode":       AccountModeCoding,
			"api_key":            "k",
			"zhipu_organization": "org-x",
			"zhipu_project":      "proj-y",
		}}
}

// targetType 判定：团队凭据存在 → TEAM，否则 PERSONAL。
func TestZhipuResetTargetType(t *testing.T) {
	t.Parallel()
	require.Equal(t, "TEAM", zhipuResetTargetType(zhipuResetTestAccount()))

	personal := &Account{ID: 3, Platform: PlatformZhipu, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"account_mode": AccountModeCoding, "api_key": "k"}}
	require.Equal(t, "PERSONAL", zhipuResetTargetType(personal))
}

// 完整成功链路：list（有可用次数）→ use（success）→ 再 list（空）→ 探测刷新。
func TestCNProviderQuotaResetUsage(t *testing.T) {
	upstream := &stubResetUpstream{queue: []*http.Response{
		// 1. 首次 list：1 条可用 + 1 条已用完（验证只取 available=true）。
		jsonResponse(`{"code":200,"msg":"操作成功","data":{"weekResets":[{"recordId":308564,"expireTime":"2026-10-01 23:59:59","available":true},{"recordId":308565,"expireTime":"2026-11-01 23:59:59","available":false}]},"success":true}`),
		// 2. use 成功。
		jsonResponse(`{"code":200,"msg":"操作成功","data":308564,"success":true}`),
		// 3. 再 list：已无可用次数。
		jsonResponse(`{"code":200,"msg":"操作成功","data":{"weekResets":[]},"success":true}`),
		// 4. 探测刷新（quota 主响应）。
		jsonResponse(`{"code":200,"data":{"limits":[{"type":"CREDIT_LIMIT","unit":3,"percentage":0}],"level":"max"},"success":true}`),
		// 5. 探测内嵌的重置次数 list：空。
		jsonResponse(`{"code":200,"msg":"操作成功","data":{"weekResets":[]},"success":true}`),
	}}
	repo := &resetAccountRepo{}
	svc := NewCNProviderQuotaService(repo, nil, upstream, &config.Config{})

	result, err := svc.ResetUsageForAccount(context.Background(), zhipuResetTestAccount())
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "weekly", result.Window)
	require.Equal(t, int64(308564), result.RecordID)
	require.Equal(t, 0, result.WeekResetsLeft)

	// 调用序列：list → use → list（剩余次数）→ quota 探测 → 探测内 list。
	require.Len(t, upstream.calls, 5)
	require.Equal(t, "/api/biz/customer-package-reset/list", upstream.calls[0].path)
	require.Equal(t, "targetType=TEAM", upstream.calls[0].query, "团队账号 list 必须带 targetType=TEAM")
	require.Equal(t, "/api/biz/customer-package-reset/use", upstream.calls[1].path)
	require.Equal(t, http.MethodPost, upstream.calls[1].method)

	// POST body 携带 targetType/resetType/recordId/requestId（官网同款）。
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(upstream.calls[1].body), &payload))
	require.Equal(t, "TEAM", payload["targetType"])
	require.Equal(t, "WEEK", payload["resetType"])
	require.Equal(t, float64(308564), payload["recordId"])
	require.NotEmpty(t, payload["requestId"])

	// 成功后带回刷新的探测结果，且重置可用性快照落为 false。
	require.NotNil(t, result.Probe)
	require.True(t, result.Probe.Success)
	require.False(t, result.Probe.ResetAvailable)
	require.NotEmpty(t, repo.extraUpdates)
	last := repo.extraUpdates[len(repo.extraUpdates)-1]
	require.Equal(t, false, last["zhipu_week_reset_available"])
}

// 无可用重置次数：不发 use 请求，返回 success=false + 明确错误。
func TestCNProviderQuotaResetUsage_NoAvailableRecords(t *testing.T) {
	upstream := &stubResetUpstream{queue: []*http.Response{
		jsonResponse(`{"code":200,"msg":"操作成功","data":{"weekResets":[]},"success":true}`),
	}}
	svc := NewCNProviderQuotaService(&resetAccountRepo{}, nil, upstream, &config.Config{})

	result, err := svc.ResetUsageForAccount(context.Background(), zhipuResetTestAccount())
	require.NoError(t, err)
	require.False(t, result.Success)
	require.NotEmpty(t, result.Error)
	require.Len(t, upstream.calls, 1, "无可用次数时不得发 use 请求")
	require.Equal(t, "/api/biz/customer-package-reset/list", upstream.calls[0].path)
}

// 上游业务错误（HTTP 200 + success=false）：错误信息透传，不误报成功。
func TestCNProviderQuotaResetUsage_UpstreamBusinessError(t *testing.T) {
	upstream := &stubResetUpstream{queue: []*http.Response{
		jsonResponse(`{"code":200,"msg":"操作成功","data":{"weekResets":[{"recordId":1,"available":true}]},"success":true}`),
		jsonResponse(`{"code":400,"msg":"重置次数已被使用","data":null,"success":false}`),
	}}
	svc := NewCNProviderQuotaService(&resetAccountRepo{}, nil, upstream, &config.Config{})

	result, err := svc.ResetUsageForAccount(context.Background(), zhipuResetTestAccount())
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Contains(t, result.Error, "重置次数已被使用")
	require.Len(t, upstream.calls, 2, "use 失败后不得继续刷新调用")
}

// 非 zhipu 平台直接拒绝。
func TestCNProviderQuotaResetUsage_RejectsNonZhipu(t *testing.T) {
	svc := NewCNProviderQuotaService(&resetAccountRepo{}, nil, &stubResetUpstream{}, &config.Config{})
	kimi := &Account{ID: 1, Platform: PlatformKimi, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"account_mode": AccountModeCoding, "api_key": "k"}}
	_, err := svc.ResetUsageForAccount(context.Background(), kimi)
	require.Error(t, err)
	require.Contains(t, err.Error(), "only supported for zhipu")
}
