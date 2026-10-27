package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// ---------------------------------------------------------------------------
// 测试桩
// ---------------------------------------------------------------------------

// zhipuResetEvents 按发生顺序记录 extra 写入与账号状态恢复，用于断言后处理顺序。
type zhipuResetEvents struct {
	mu  sync.Mutex
	log []string
}

func (e *zhipuResetEvents) add(event string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.log = append(e.log, event)
}

func (e *zhipuResetEvents) snapshot() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.log...)
}

// zhipuResetResponse 是一次脚本化的上游响应；err 非空模拟网络失败；
// hold 非空时请求到达后先关闭 arrived，再等 hold 关闭才返回（模拟慢请求）。
type zhipuResetResponse struct {
	status  int
	body    string
	err     error
	hold    <-chan struct{}
	arrived chan struct{}
}

func zhipuOK(body string) zhipuResetResponse {
	return zhipuResetResponse{status: http.StatusOK, body: body}
}

type zhipuResetRequest struct {
	method string
	url    string
	auth   string
	body   string
}

// zhipuResetUpstream 按顺序回放响应并记录每次请求（并发安全）。
type zhipuResetUpstream struct {
	mu        sync.Mutex
	responses []zhipuResetResponse
	requests  []zhipuResetRequest
}

func (u *zhipuResetUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	recorded := zhipuResetRequest{method: req.Method, url: req.URL.String(), auth: req.Header.Get("Authorization")}
	if req.Body != nil {
		raw, _ := io.ReadAll(req.Body)
		recorded.body = string(raw)
	}
	u.mu.Lock()
	u.requests = append(u.requests, recorded)
	if len(u.responses) == 0 {
		u.mu.Unlock()
		return nil, errors.New("no scripted response")
	}
	next := u.responses[0]
	u.responses = u.responses[1:]
	u.mu.Unlock()
	if next.hold != nil {
		close(next.arrived)
		<-next.hold
	}
	if next.err != nil {
		return nil, next.err
	}
	return &http.Response{StatusCode: next.status, Body: io.NopCloser(strings.NewReader(next.body)), Header: make(http.Header)}, nil
}

func (u *zhipuResetUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func (u *zhipuResetUpstream) recorded() []zhipuResetRequest {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]zhipuResetRequest(nil), u.requests...)
}

func (u *zhipuResetUpstream) countPath(path string) int {
	n := 0
	for _, req := range u.recorded() {
		if strings.Contains(req.url, path) {
			n++
		}
	}
	return n
}

// zhipuResetRepo 记录所有 extra 写入；写入按内容归类进事件日志（cards / quota）。
type zhipuResetRepo struct {
	AccountRepository
	account *Account
	events  *zhipuResetEvents

	mu     sync.Mutex
	writes []map[string]any
}

func (r *zhipuResetRepo) GetByID(context.Context, int64) (*Account, error) {
	if r.account == nil {
		return nil, ErrAccountNotFound
	}
	cloned := *r.account
	return &cloned, nil
}

func (r *zhipuResetRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.mu.Lock()
	r.writes = append(r.writes, updates)
	r.mu.Unlock()
	switch {
	case updates["zhipu_reset_cards_updated_at"] != nil:
		r.events.add("cards")
	case updates["zhipu_usage_updated_at"] != nil:
		r.events.add("quota")
	}
	return nil
}

func (r *zhipuResetRepo) extraWrites() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]map[string]any(nil), r.writes...)
}

// zhipuResetRecorder 同时充当后处理的 refresher 与 recoverer，把调用写进事件日志。
type zhipuResetRecorder struct {
	events     *zhipuResetEvents
	probe      *CNProviderQuotaProbeResult
	probeErr   error
	recoverErr error
}

func (r *zhipuResetRecorder) RefreshUsageAfterReset(context.Context, int64) (*CNProviderQuotaProbeResult, error) {
	r.events.add("refresh")
	return r.probe, r.probeErr
}

func (r *zhipuResetRecorder) RecoverAccountState(_ context.Context, _ int64, options AccountRecoveryOptions) (*SuccessfulTestRecoveryResult, error) {
	r.events.add("recover")
	if options != (AccountRecoveryOptions{}) {
		return nil, errors.New("api key accounts have no token cache to invalidate")
	}
	if r.recoverErr != nil {
		return nil, r.recoverErr
	}
	return &SuccessfulTestRecoveryResult{ClearedRateLimit: true}, nil
}

func zhipuResetAccount(id int64) *Account {
	return &Account{
		ID: id, Platform: PlatformZhipu, Type: AccountTypeAPIKey, Status: StatusActive, Concurrency: 1,
		Credentials: map[string]any{"account_mode": AccountModeCoding, "api_key": "sk-test"},
	}
}

type zhipuResetFixture struct {
	svc      *CNProviderQuotaService
	repo     *zhipuResetRepo
	upstream *zhipuResetUpstream
	events   *zhipuResetEvents
}

// newZhipuResetFixture 构造服务；注入的 recoverer 把恢复动作记进同一个事件日志。
func newZhipuResetFixture(account *Account, responses ...zhipuResetResponse) *zhipuResetFixture {
	events := &zhipuResetEvents{}
	repo := &zhipuResetRepo{account: account, events: events}
	upstream := &zhipuResetUpstream{responses: responses}
	svc := NewCNProviderQuotaService(repo, nil, upstream, nil)
	svc.SetZhipuResetRecoverer(&zhipuResetRecorder{events: events})
	return &zhipuResetFixture{svc: svc, repo: repo, upstream: upstream, events: events}
}

func requireZhipuResetReason(t *testing.T, err error, reason string, status int) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, reason, infraerrors.Reason(err))
	require.Equal(t, status, infraerrors.Code(err))
}

// ---------------------------------------------------------------------------
// 上游响应样本
// ---------------------------------------------------------------------------

// zhipuResetListBody：周卡两张可用（到期时间故意乱序）+ 一张已用；5h 卡一张可用 + 一张已过期。
const zhipuResetListBody = `{
	"code": 200,
	"success": true,
	"msg": "操作成功",
	"data": {
		"weekResets": [
			{"recordId": 302, "grantType": "PACKAGE_GIFT", "expireTime": "2026-10-20 23:59:59", "available": true},
			{"recordId": 301, "grantType": "PACKAGE_GIFT", "expireTime": "2026-10-05 23:59:59", "available": true},
			{"recordId": 300, "grantType": "PACKAGE_GIFT", "expireTime": "2026-09-01 23:59:59", "available": false}
		],
		"fiveHourResets": [
			{"recordId": 401, "grantType": "ACTIVITY", "expireTime": "2026-10-10 12:00:00", "available": true},
			{"recordId": 400, "grantType": "ACTIVITY", "expireTime": "2026-09-02 12:00:00", "available": false}
		],
		"lastWeekResetTime": "2026-09-20 10:00:00",
		"lastFiveHourResetTime": null
	}
}`

// zhipuResetListAfterWeekUse 是用掉 301 之后的 list。
const zhipuResetListAfterWeekUse = `{"code":200,"success":true,"data":{
	"weekResets":[
		{"recordId":302,"grantType":"PACKAGE_GIFT","expireTime":"2026-10-20 23:59:59","available":true},
		{"recordId":301,"grantType":"PACKAGE_GIFT","expireTime":"2026-10-05 23:59:59","available":false}
	],
	"fiveHourResets":[{"recordId":401,"grantType":"ACTIVITY","expireTime":"2026-10-10 12:00:00","available":true}]
}}`

// zhipuResetListAfterFiveHourUse 是用掉 401 之后的 list。
const zhipuResetListAfterFiveHourUse = `{"code":200,"success":true,"data":{
	"weekResets":[
		{"recordId":302,"grantType":"PACKAGE_GIFT","expireTime":"2026-10-20 23:59:59","available":true},
		{"recordId":301,"grantType":"PACKAGE_GIFT","expireTime":"2026-10-05 23:59:59","available":true}
	],
	"fiveHourResets":[{"recordId":401,"grantType":"ACTIVITY","expireTime":"2026-10-10 12:00:00","available":false}]
}}`

const zhipuResetEmptyListBody = `{"code":200,"success":true,"data":{
	"weekResets":[{"recordId":300,"grantType":"PACKAGE_GIFT","expireTime":"2026-09-01 23:59:59","available":false}],
	"fiveHourResets":[]
}}`

const zhipuResetUseOKBody = `{"code":200,"success":true,"msg":"操作成功","data":null}`

const zhipuResetQuotaBody = `{"success":true,"data":{"level":"pro","limits":[
	{"type":"TOKENS_LIMIT","unit":3,"percentage":0,"nextResetTime":1790000000000},
	{"type":"TOKENS_LIMIT","unit":6,"percentage":0,"nextResetTime":1790500000000}
]}}`

// ---------------------------------------------------------------------------
// list
// ---------------------------------------------------------------------------

func TestZhipuResetCards_ListPersistsSnapshot(t *testing.T) {
	t.Run("available cards", func(t *testing.T) {
		f := newZhipuResetFixture(zhipuResetAccount(51), zhipuOK(zhipuResetListBody))

		cards, err := f.svc.ListZhipuResetCards(context.Background(), 51)
		require.NoError(t, err)

		requests := f.upstream.recorded()
		require.Len(t, requests, 1)
		require.Equal(t, http.MethodGet, requests[0].method)
		require.Equal(t, "https://open.bigmodel.cn/api/biz/customer-package-reset/list?targetType=PERSONAL", requests[0].url)
		require.Equal(t, "sk-test", requests[0].auth, "zhipu data-plane auth carries the raw key without Bearer")

		// 只保留可用卡，按到期时间升序（301 比 302 早到期）。
		require.Equal(t, []ZhipuResetCard{
			{RecordID: 301, GrantType: "PACKAGE_GIFT", ExpireTime: "2026-10-05 23:59:59", Available: true},
			{RecordID: 302, GrantType: "PACKAGE_GIFT", ExpireTime: "2026-10-20 23:59:59", Available: true},
		}, cards.WeekCards)
		require.Equal(t, []ZhipuResetCard{
			{RecordID: 401, GrantType: "ACTIVITY", ExpireTime: "2026-10-10 12:00:00", Available: true},
		}, cards.FiveHourCards)
		require.Equal(t, "2026-09-20 10:00:00", cards.LastWeekResetTime)
		require.Empty(t, cards.LastFiveHourResetTime)
		require.True(t, cards.Persisted)

		writes := f.repo.extraWrites()
		require.Len(t, writes, 1)
		updates := writes[0]
		require.Equal(t, true, updates["zhipu_week_reset_available"])
		require.Equal(t, 2, updates["zhipu_week_reset_count"])
		require.Equal(t, "2026-10-05 23:59:59", updates["zhipu_week_reset_expire_at"])
		require.Equal(t, true, updates["zhipu_5h_reset_available"])
		require.Equal(t, 1, updates["zhipu_5h_reset_count"])
		require.Equal(t, "2026-10-10 12:00:00", updates["zhipu_5h_reset_expire_at"])
		updatedAt, ok := updates["zhipu_reset_cards_updated_at"].(string)
		require.True(t, ok)
		_, err = time.Parse(time.RFC3339, updatedAt)
		require.NoError(t, err)
		require.Len(t, updates, 7, "the card snapshot never touches quota window keys")
	})

	t.Run("no available cards writes explicit nulls", func(t *testing.T) {
		f := newZhipuResetFixture(zhipuResetAccount(52), zhipuOK(zhipuResetEmptyListBody))

		cards, err := f.svc.ListZhipuResetCards(context.Background(), 52)
		require.NoError(t, err)
		require.NotNil(t, cards.WeekCards, "empty lists serialize as [] rather than null")
		require.Empty(t, cards.WeekCards)
		require.Empty(t, cards.FiveHourCards)

		updates := f.repo.extraWrites()[0]
		require.Equal(t, false, updates["zhipu_week_reset_available"])
		require.Equal(t, false, updates["zhipu_5h_reset_available"])
		require.Equal(t, 0, updates["zhipu_week_reset_count"])
		require.Equal(t, 0, updates["zhipu_5h_reset_count"])
		// JSONB `||` 合并删不掉键：到期时间必须显式写 null。
		for _, key := range []string{"zhipu_week_reset_expire_at", "zhipu_5h_reset_expire_at"} {
			value, present := updates[key]
			require.Truef(t, present, "%s must be written explicitly", key)
			require.Nil(t, value)
		}
	})
}

// 上游失败与结构不可信都返回 502，且不落快照、保留原快照。
func TestZhipuResetCards_ListFailuresKeepSnapshot(t *testing.T) {
	cases := []struct {
		name     string
		response zhipuResetResponse
		reason   string
		contains string
	}{
		{name: "success=false", response: zhipuOK(`{"code":500,"success":false,"msg":"系统繁忙"}`), reason: ZhipuResetErrRejected, contains: "系统繁忙"},
		{name: "code is not 200", response: zhipuOK(`{"code":1001,"msg":"令牌已过期"}`), reason: ZhipuResetErrRejected, contains: "令牌已过期"},
		{name: "unauthorized", response: zhipuResetResponse{status: http.StatusUnauthorized, body: `{}`}, reason: ZhipuResetErrAuthFailed, contains: "HTTP 401"},
		{name: "forbidden", response: zhipuResetResponse{status: http.StatusForbidden, body: `{}`}, reason: ZhipuResetErrAuthFailed, contains: "HTTP 403"},
		{name: "server error", response: zhipuResetResponse{status: http.StatusBadGateway, body: `bad gateway`}, reason: ZhipuResetErrUpstream, contains: "HTTP 502"},
		{name: "network", response: zhipuResetResponse{err: errors.New("dial timeout")}, reason: ZhipuResetErrRequestFailed, contains: "dial timeout"},
		{name: "non json", response: zhipuOK(`<html>login</html>`), reason: ZhipuResetErrInvalidResponse, contains: "not valid JSON"},
		{name: "missing data", response: zhipuOK(`{"code":200,"success":true}`), reason: ZhipuResetErrInvalidResponse, contains: `"data"`},
		{name: "both lists missing", response: zhipuOK(`{"code":200,"success":true,"data":{"lastWeekResetTime":"2026-09-20 10:00:00"}}`), reason: ZhipuResetErrInvalidResponse, contains: "weekResets"},
		{name: "both lists null", response: zhipuOK(`{"code":200,"success":true,"data":{"weekResets":null,"fiveHourResets":null}}`), reason: ZhipuResetErrInvalidResponse, contains: "weekResets"},
		{name: "week list is an object", response: zhipuOK(`{"code":200,"success":true,"data":{"weekResets":{},"fiveHourResets":[]}}`), reason: ZhipuResetErrInvalidResponse, contains: "weekResets"},
		{name: "five hour list is a string", response: zhipuOK(`{"code":200,"success":true,"data":{"weekResets":[],"fiveHourResets":"none"}}`), reason: ZhipuResetErrInvalidResponse, contains: "fiveHourResets"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newZhipuResetFixture(zhipuResetAccount(54), tc.response)

			cards, err := f.svc.ListZhipuResetCards(context.Background(), 54)
			require.Nil(t, cards)
			requireZhipuResetReason(t, err, tc.reason, http.StatusBadGateway)
			require.Contains(t, infraerrors.Message(err), tc.contains)
			require.Empty(t, f.repo.extraWrites(), "a failed list must keep the previous card snapshot")
		})
	}
}

// 列表形状的合法边界：一个是数组、另一个缺失或为 null 仍然可用；数字时间戳归一化；
// 缺 recordId 的记录无法使用，跳过。
func TestParseZhipuResetCards_ValidShapes(t *testing.T) {
	for name, data := range map[string]string{
		"both empty arrays":        `{"weekResets":[],"fiveHourResets":[]}`,
		"one array, other missing": `{"weekResets":[]}`,
		"one array, other null":    `{"weekResets":[],"fiveHourResets":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			cards, failure := parseZhipuResetCards([]byte(`{"code":200,"success":true,"data":`+data+`}`), time.Unix(1_700_000_000, 0))
			require.Nil(t, failure)
			require.NotNil(t, cards.WeekCards)
			require.NotNil(t, cards.FiveHourCards)
			require.Equal(t, int64(1_700_000_000), cards.FetchedAt)
		})
	}

	cards, failure := parseZhipuResetCards([]byte(`{"code":200,"success":true,"data":{"weekResets":[
		{"recordId":"77","grantType":"G","expireTime":1790000000000,"available":true},
		{"grantType":"G","expireTime":"2026-10-01 00:00:00","available":true}
	]}}`), time.Now())
	require.Nil(t, failure)
	require.Len(t, cards.WeekCards, 1)
	require.Equal(t, int64(77), cards.WeekCards[0].RecordID)
	require.Equal(t, cnMillisToRFC3339(1790000000000), cards.WeekCards[0].ExpireTime)
}

// 选卡：默认最早到期（不带时区的时间按北京时间比较），解析不了的排最后。
func TestSelectZhipuResetCard_PicksEarliestExpiry(t *testing.T) {
	cards := &ZhipuResetCards{WeekCards: []ZhipuResetCard{
		{RecordID: 1, ExpireTime: "not-a-date"},
		{RecordID: 2, ExpireTime: "2026-10-05 23:59:59"},
		{RecordID: 3, ExpireTime: "2026-10-01T02:00:00Z"}, // = 2026-10-01 10:00 北京时间
		{RecordID: 4, ExpireTime: "2026-10-01 09:00:00"},  // 更早（北京时间）
		{RecordID: 5, ExpireTime: ""},
	}}
	card, failure := selectZhipuResetCard(cards, ZhipuResetTypeWeek, 0)
	require.Nil(t, failure)
	require.Equal(t, int64(4), card.RecordID)

	sorted := append([]ZhipuResetCard(nil), cards.WeekCards...)
	sortZhipuResetCards(sorted)
	ids := make([]int64, 0, len(sorted))
	for _, c := range sorted {
		ids = append(ids, c.RecordID)
	}
	require.Equal(t, []int64{4, 3, 2, 5, 1}, ids)

	// 指定 record_id 时不按到期时间选。
	card, failure = selectZhipuResetCard(cards, ZhipuResetTypeWeek, 2)
	require.Nil(t, failure)
	require.Equal(t, int64(2), card.RecordID)
}

// ---------------------------------------------------------------------------
// use
// ---------------------------------------------------------------------------

// 用卡成功：请求体字段、选卡、刷新列表，以及后处理「先落额度快照、再恢复账号状态」。
func TestZhipuResetCards_UseSucceeds(t *testing.T) {
	cases := []struct {
		name         string
		req          ZhipuResetCardUseRequest
		afterList    string
		wantType     string
		wantWindow   string
		wantRecord   int64
		wantGrant    string
		wantWeekLeft int
		wantFiveLeft int
	}{
		{
			name: "defaults to the earliest weekly card", req: ZhipuResetCardUseRequest{},
			afterList: zhipuResetListAfterWeekUse, wantType: "WEEK", wantWindow: "weekly",
			wantRecord: 301, wantGrant: "PACKAGE_GIFT", wantWeekLeft: 1, wantFiveLeft: 1,
		},
		{
			name: "explicit five hour card", req: ZhipuResetCardUseRequest{ResetType: "five_hour", RecordID: 401},
			afterList: zhipuResetListAfterFiveHourUse, wantType: "FIVE_HOUR", wantWindow: "5h",
			wantRecord: 401, wantGrant: "ACTIVITY", wantWeekLeft: 2, wantFiveLeft: 0,
		},
	}
	seenRequestIDs := map[string]bool{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newZhipuResetFixture(zhipuResetAccount(55),
				zhipuOK(zhipuResetListBody), zhipuOK(zhipuResetUseOKBody), zhipuOK(tc.afterList), zhipuOK(zhipuResetQuotaBody))

			result, err := f.svc.UseZhipuResetCard(context.Background(), 55, tc.req)
			require.NoError(t, err)
			require.True(t, result.Success, result.Error)
			require.Equal(t, tc.wantType, result.ResetType)
			require.Equal(t, tc.wantWindow, result.Window)
			require.Equal(t, tc.wantRecord, result.RecordID)
			require.Equal(t, tc.wantWeekLeft, result.WeekResetsLeft)
			require.Equal(t, tc.wantFiveLeft, result.FiveHourResetsLeft)
			require.Len(t, result.Cards.WeekCards, tc.wantWeekLeft)

			// 请求顺序：list → use → 刷新 list → 用卡后的额度探测。
			requests := f.upstream.recorded()
			require.Len(t, requests, 4)
			require.Equal(t, http.MethodGet, requests[0].method)
			require.Equal(t, http.MethodGet, requests[2].method)
			require.Equal(t, "https://open.bigmodel.cn/api/monitor/usage/quota/limit", requests[3].url)

			use := requests[1]
			require.Equal(t, http.MethodPost, use.method)
			require.Equal(t, "https://open.bigmodel.cn/api/biz/customer-package-reset/use", use.url)
			require.Equal(t, "sk-test", use.auth)
			require.Equal(t, "PERSONAL", gjson.Get(use.body, "targetType").String())
			require.Equal(t, tc.wantType, gjson.Get(use.body, "resetType").String())
			require.Equal(t, gjson.Number, gjson.Get(use.body, "recordId").Type, "recordId is sent as a number")
			require.Equal(t, tc.wantRecord, gjson.Get(use.body, "recordId").Int())
			require.Equal(t, tc.wantGrant, gjson.Get(use.body, "grantType").String())
			requestID := gjson.Get(use.body, "requestId").String()
			_, err = uuid.Parse(requestID)
			require.NoError(t, err, "requestId is a UUID")
			require.False(t, seenRequestIDs[requestID], "requestId is fresh per call")
			seenRequestIDs[requestID] = true

			// 用前卡列表 → 用后卡列表 → 额度快照 → 恢复账号状态。
			require.Equal(t, []string{"cards", "cards", "quota", "recover"}, f.events.snapshot())
			require.True(t, result.AccountStateRecovered)
			require.Empty(t, result.WarningCode)
			require.True(t, result.Probe.Success)
			require.True(t, result.Probe.Persisted)
		})
	}
}

// 用卡成功但刷新列表失败：按用前列表扣掉本次那张推算剩余并落快照，后处理照常执行。
func TestZhipuResetCards_UseRefreshFailureDerivesRemaining(t *testing.T) {
	f := newZhipuResetFixture(zhipuResetAccount(56),
		zhipuOK(zhipuResetListBody), zhipuOK(zhipuResetUseOKBody), zhipuResetResponse{err: errors.New("connection reset")}, zhipuOK(zhipuResetQuotaBody))

	result, err := f.svc.UseZhipuResetCard(context.Background(), 56, ZhipuResetCardUseRequest{})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 1, result.WeekResetsLeft)
	require.Equal(t, 1, result.FiveHourResetsLeft)
	require.Equal(t, int64(302), result.Cards.WeekCards[0].RecordID)
	require.True(t, result.Cards.Persisted)

	writes := f.repo.extraWrites()
	derived := writes[1]
	require.Equal(t, 1, derived["zhipu_week_reset_count"])
	require.Equal(t, "2026-10-20 23:59:59", derived["zhipu_week_reset_expire_at"])
	require.Equal(t, []string{"cards", "cards", "quota", "recover"}, f.events.snapshot())
}

// 用卡失败：返回业务失败结果，不做后处理（不重探、不恢复账号状态）；
// 发出过 use 请求的会再刷新一次列表，让快照跟上游对齐。
func TestZhipuResetCards_UseFailures(t *testing.T) {
	useThen := func(use zhipuResetResponse) []zhipuResetResponse {
		return []zhipuResetResponse{zhipuOK(zhipuResetListBody), use, zhipuOK(zhipuResetListBody)}
	}
	cases := []struct {
		name         string
		req          ZhipuResetCardUseRequest
		responses    []zhipuResetResponse
		wantCode     string
		wantStatus   int
		wantContains string
		wantRequests int
	}{
		// 选卡失败：不发 use。
		{name: "record already used", req: ZhipuResetCardUseRequest{RecordID: 300}, responses: []zhipuResetResponse{zhipuOK(zhipuResetListBody)}, wantCode: ZhipuResetErrCardNotAvailable, wantRequests: 1},
		{name: "five hour record under week type", req: ZhipuResetCardUseRequest{RecordID: 401}, responses: []zhipuResetResponse{zhipuOK(zhipuResetListBody)}, wantCode: ZhipuResetErrCardNotAvailable, wantRequests: 1},
		{name: "no weekly card", responses: []zhipuResetResponse{zhipuOK(zhipuResetEmptyListBody)}, wantCode: ZhipuResetErrNoCard, wantContains: "WEEK", wantRequests: 1},
		{name: "no five hour card", req: ZhipuResetCardUseRequest{ResetType: "FIVE_HOUR"}, responses: []zhipuResetResponse{zhipuOK(zhipuResetEmptyListBody)}, wantCode: ZhipuResetErrNoCard, wantContains: "FIVE_HOUR", wantRequests: 1},
		{name: "list forbidden before use", responses: []zhipuResetResponse{{status: http.StatusForbidden, body: `{}`}}, wantCode: ZhipuResetErrAuthFailed, wantStatus: http.StatusForbidden, wantRequests: 1},
		// 上游拒绝用卡：list → use → 刷新 list。
		{
			name: "upstream success=false", responses: useThen(zhipuOK(`{"code":500,"success":false,"msg":"指定的重置次数不可用，请刷新后重试"}`)),
			wantCode: ZhipuResetErrRejected, wantStatus: http.StatusOK, wantContains: "指定的重置次数不可用", wantRequests: 3,
		},
		{name: "code is not 200", responses: useThen(zhipuOK(`{"code":201,"success":true}`)), wantCode: ZhipuResetErrRejected, wantRequests: 3},
		{name: "success without code", responses: useThen(zhipuOK(`{"success":true}`)), wantCode: ZhipuResetErrInvalidResponse, wantRequests: 3},
		{name: "use unauthorized", responses: useThen(zhipuResetResponse{status: http.StatusUnauthorized, body: `{}`}), wantCode: ZhipuResetErrAuthFailed, wantStatus: http.StatusUnauthorized, wantContains: "HTTP 401", wantRequests: 3},
		{name: "use network error", responses: useThen(zhipuResetResponse{err: errors.New("i/o timeout")}), wantCode: ZhipuResetErrRequestFailed, wantContains: "i/o timeout", wantRequests: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newZhipuResetFixture(zhipuResetAccount(57), tc.responses...)

			result, err := f.svc.UseZhipuResetCard(context.Background(), 57, tc.req)
			require.NoError(t, err)
			require.False(t, result.Success)
			require.Equal(t, tc.wantCode, result.ErrorCode)
			require.NotEmpty(t, result.Error)
			if tc.wantContains != "" {
				require.Contains(t, result.Error, tc.wantContains)
			}
			if tc.wantStatus != 0 {
				require.Equal(t, tc.wantStatus, result.StatusCode)
			}
			require.Len(t, f.upstream.recorded(), tc.wantRequests)
			require.Equal(t, 0, f.upstream.countPath("/api/monitor/usage/quota/limit"), "no post-reset probe")
			require.NotContains(t, f.events.snapshot(), "recover", "a failed use never lifts the cooldown")
			require.False(t, result.AccountStateRecovered)
			require.Nil(t, result.Probe)
		})
	}
}

// 非智谱 / 国际站 / 团队版 / payg 账号与非法参数：直接拒绝，不发上游请求、不写快照。
func TestZhipuResetCards_RejectsUnsupportedRequests(t *testing.T) {
	withCredential := func(account *Account, key, value string) *Account {
		account.Credentials[key] = value
		return account
	}
	kimi := zhipuResetAccount(60)
	kimi.Platform = PlatformKimi

	cases := []struct {
		name    string
		account *Account
		req     ZhipuResetCardUseRequest
		reason  string
		useOnly bool
	}{
		{name: "kimi coding plan", account: kimi, reason: ZhipuResetErrUnsupported},
		{name: "zhipu behind a custom relay", account: withCredential(zhipuResetAccount(61), "base_url", "https://relay.example.com/v1"), reason: ZhipuResetErrUnsupported},
		{name: "zhipu international site", account: withCredential(zhipuResetAccount(62), "base_url", "https://api.z.ai/api/coding/paas/v4"), reason: ZhipuResetErrRegionUnsupported},
		{name: "zhipu team plan", account: withCredential(zhipuResetAccount(63), "zhipu_organization", "org-123"), reason: ZhipuResetErrTeamUnsupported},
		{name: "zhipu payg", account: withCredential(zhipuResetAccount(64), "account_mode", AccountModePayG), reason: "CN_QUOTA_NOT_CODING_PLAN"},
		{name: "unknown reset type", account: zhipuResetAccount(65), req: ZhipuResetCardUseRequest{ResetType: "MONTH"}, reason: ZhipuResetErrInvalidType, useOnly: true},
		{name: "negative record id", account: zhipuResetAccount(66), req: ZhipuResetCardUseRequest{RecordID: -1}, reason: ZhipuResetErrInvalidRecord, useOnly: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newZhipuResetFixture(tc.account, zhipuOK(zhipuResetListBody))

			if !tc.useOnly {
				_, err := f.svc.ListZhipuResetCards(context.Background(), tc.account.ID)
				requireZhipuResetReason(t, err, tc.reason, http.StatusBadRequest)
			}
			_, err := f.svc.UseZhipuResetCard(context.Background(), tc.account.ID, tc.req)
			requireZhipuResetReason(t, err, tc.reason, http.StatusBadRequest)

			require.Empty(t, f.upstream.recorded(), "rejected requests never reach the upstream")
			require.Empty(t, f.repo.extraWrites())
		})
	}
}

// 客户端在 use 请求进行中断开：调用方立即拿到 ctx.Err()，但卡已经在消耗，
// 快照刷新与账号状态恢复必须照常执行完。
func TestZhipuResetCards_UseCompletesAfterCallerCancels(t *testing.T) {
	release := make(chan struct{})
	arrived := make(chan struct{})
	f := newZhipuResetFixture(zhipuResetAccount(67),
		zhipuOK(zhipuResetListBody),
		zhipuResetResponse{status: http.StatusOK, body: zhipuResetUseOKBody, hold: release, arrived: arrived},
		zhipuOK(zhipuResetListAfterWeekUse),
		zhipuOK(zhipuResetQuotaBody))

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := f.svc.UseZhipuResetCard(ctx, 67, ZhipuResetCardUseRequest{})
		errCh <- err
	}()

	select {
	case <-arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("use request never reached the upstream")
	}
	cancel() // 客户端断开
	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("the caller should return as soon as its context is cancelled")
	}

	close(release) // 上游随后才处理完用卡
	require.Eventually(t, func() bool {
		events := f.events.snapshot()
		return len(events) > 0 && events[len(events)-1] == "recover"
	}, 5*time.Second, 10*time.Millisecond)
	require.Equal(t, []string{"cards", "cards", "quota", "recover"}, f.events.snapshot())
	require.Equal(t, 1, f.upstream.countPath("/customer-package-reset/use"))
}

// ---------------------------------------------------------------------------
// 后处理
// ---------------------------------------------------------------------------

// 后处理顺序固定为「先刷新快照、再恢复账号状态」；刷新失败不阻断恢复。
func TestRunZhipuResetCardPostProcess(t *testing.T) {
	okProbe := &CNProviderQuotaProbeResult{Provider: PlatformZhipu, Success: true, Persisted: true}
	cases := []struct {
		name          string
		recorder      zhipuResetRecorder
		noRecoverer   bool
		wantEvents    []string
		wantRecovered bool
		wantWarning   string
	}{
		{name: "refresh then recover", recorder: zhipuResetRecorder{probe: okProbe},
			wantEvents: []string{"refresh", "recover"}, wantRecovered: true},
		{name: "probe error still recovers", recorder: zhipuResetRecorder{probeErr: errors.New("timeout")},
			wantEvents: []string{"refresh", "recover"}, wantRecovered: true, wantWarning: ZhipuResetWarningQuotaRefreshFailed},
		{name: "unpersisted probe still recovers", recorder: zhipuResetRecorder{probe: &CNProviderQuotaProbeResult{Success: true}},
			wantEvents: []string{"refresh", "recover"}, wantRecovered: true, wantWarning: ZhipuResetWarningQuotaRefreshFailed},
		{name: "recovery failure", recorder: zhipuResetRecorder{probe: okProbe, recoverErr: errors.New("db down")},
			wantEvents: []string{"refresh", "recover"}, wantWarning: ZhipuResetWarningAccountRecoveryFailed},
		{name: "missing recoverer", recorder: zhipuResetRecorder{probe: okProbe}, noRecoverer: true,
			wantEvents: []string{"refresh"}, wantWarning: ZhipuResetWarningAccountRecoveryFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := tc.recorder
			recorder.events = &zhipuResetEvents{}
			var recoverer ZhipuResetAccountRecoverer = &recorder
			if tc.noRecoverer {
				recoverer = nil
			}
			result := runZhipuResetCardPostProcess(context.Background(), 1, &recorder, recoverer)
			require.Equal(t, tc.wantEvents, recorder.events.snapshot())
			require.Equal(t, tc.wantRecovered, result.AccountStateRecovered)
			require.Equal(t, tc.wantWarning, result.WarningCode)
		})
	}
}
