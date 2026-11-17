package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// OpenDesign 一键设备授权服务测试（2026-11-15 需求文档 §8 验收用例）。

func newOpenDesignAuthTestService(t *testing.T, handler http.Handler) (*OpenDesignDeviceAuthService, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("OPENDESIGN_AUTH_API_BASE_URL", server.URL)
	t.Setenv("OPENDESIGN_CLI_VERSION", "0.1.6")
	svc := NewOpenDesignDeviceAuthService()
	t.Cleanup(svc.Stop)
	return svc, server
}

// fakeUpstream 模拟官方设备授权 + 钱包接口，按次序返回脚本化响应。
type fakeUpstream struct {
	createStatus   int
	pollStatuses   []string // pending / authorized ...
	pollCount      int32
	walletStatus   int
	walletStatuses []int         // 按次覆盖 walletStatus（用于「先失败后成功」）
	walletBlock    chan struct{} // 非 nil 时 wallet 请求在此阻塞（并发窗口用例）
	workspaceID    string
	createHits     int32
	walletHits     int32
	lastCreateReq  map[string]any
	lastPollReq    map[string]any
	lastWalletAuth string
}

func (f *fakeUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/cli/device-authorizations"):
		atomic.AddInt32(&f.createHits, 1)
		body, _ := io.ReadAll(r.Body)
		var parsed map[string]any
		_ = json.Unmarshal(body, &parsed)
		f.lastCreateReq = parsed
		if f.createStatus != 0 && f.createStatus != http.StatusCreated {
			w.WriteHeader(f.createStatus)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"deviceId":"dev_1","userCode":"GQRSUT","status":"pending",
			"expiresAt":"2099-01-01T00:00:00Z","deviceSecret":"secret-1",
			"activationUrl":"https://open-design.ai/cloud/cli/activate?deviceId=dev_1&userCode=GQRSUT",
			"pollIntervalSeconds":2}`))

	case strings.HasSuffix(r.URL.Path, "/token"):
		idx := int(atomic.AddInt32(&f.pollCount, 1)) - 1
		body, _ := io.ReadAll(r.Body)
		var parsed map[string]any
		_ = json.Unmarshal(body, &parsed)
		f.lastPollReq = parsed
		status := "pending"
		if idx < len(f.pollStatuses) {
			status = f.pollStatuses[idx]
		}
		normalized := normalizeOpenDesignUpstreamStatus(status)
		if isOpenDesignAuthorizedStatus(normalized) {
			_, _ = w.Write([]byte(`{"status":"` + status + `","controlKey":"vela_ctrl_x","runtimeKey":"sk-amr-x",
				"user":{"email":"dev@example.com"}}`))
			return
		}
		if normalized != "pending" {
			_, _ = w.Write([]byte(`{"status":"` + status + `","controlKey":null,"runtimeKey":null,"user":null}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"pending","controlKey":null,"runtimeKey":null,"user":null}`))

	case strings.HasSuffix(r.URL.Path, "/wallet/balance"):
		hits := int(atomic.AddInt32(&f.walletHits, 1))
		f.lastWalletAuth = r.Header.Get("Authorization")
		if f.walletBlock != nil {
			<-f.walletBlock
		}
		status := f.walletStatus
		if hits-1 < len(f.walletStatuses) {
			status = f.walletStatuses[hits-1]
		}
		if status != 0 && status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		ws := f.workspaceID
		if ws == "" {
			ws = "wstestworkspace01"
		}
		_, _ = w.Write([]byte(`{"balanceUsd":"10.00","workspaceId":"` + ws + `","workspaceMemberId":"m1"}`))

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestOpenDesignDeviceAuthCreateReturnsNoSecrets(t *testing.T) {
	upstream := &fakeUpstream{}
	svc, _ := newOpenDesignAuthTestService(t, upstream)

	result, err := svc.InitiateDeviceAuth(context.Background(), "test-host")
	require.NoError(t, err)
	require.NotEmpty(t, result.SessionID)
	require.True(t, strings.HasPrefix(result.SessionID, "ods_"))
	require.Equal(t, "GQRSUT", result.UserCode)
	require.Contains(t, result.ActivationURL, "userCode=GQRSUT")
	require.Equal(t, 2, result.PollIntervalSeconds)
	require.False(t, result.ExpiresAt.IsZero())
	// body 必填 cliVersion/host/profile
	require.Equal(t, "0.1.6", upstream.lastCreateReq["cliVersion"])
	require.Equal(t, "test-host", upstream.lastCreateReq["host"])
	require.Equal(t, "prod", upstream.lastCreateReq["profile"])

	// 对外结构里不得出现任何敏感字段（deviceSecret / controlKey / runtimeKey）
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	for _, secret := range []string{"secret-1", "deviceSecret", "controlKey", "runtimeKey"} {
		require.NotContains(t, string(raw), secret)
	}
}

func TestOpenDesignDeviceAuthPendingThenAuthorizedFetchesWorkspace(t *testing.T) {
	upstream := &fakeUpstream{pollStatuses: []string{"pending", "authorized"}}
	svc, _ := newOpenDesignAuthTestService(t, upstream)
	ctx := context.Background()

	created, err := svc.InitiateDeviceAuth(ctx, "h")
	require.NoError(t, err)

	first, err := svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, "pending", first.Status)
	require.Empty(t, first.Email)
	require.Empty(t, first.WorkspaceID)

	// 绕过节流（生产上由前端 2s 轮询自然满足）
	svc.mu.Lock()
	svc.sessions[created.SessionID].LastPollAt = time.Time{}
	svc.mu.Unlock()

	second, err := svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, "authorized", second.Status)
	require.Equal(t, "dev@example.com", second.Email)
	require.Equal(t, "wstestworkspace01", second.WorkspaceID)
	require.EqualValues(t, 1, atomic.LoadInt32(&upstream.walletHits), "授权后必须用 controlKey 取 workspaceId")
}

func TestOpenDesignDeviceAuthPollThrottled(t *testing.T) {
	upstream := &fakeUpstream{pollStatuses: []string{"pending", "pending", "pending"}}
	svc, _ := newOpenDesignAuthTestService(t, upstream)
	ctx := context.Background()

	created, err := svc.InitiateDeviceAuth(ctx, "h")
	require.NoError(t, err)

	_, err = svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	// 2 秒内再次查询只读缓存，不得再打上游
	for i := 0; i < 3; i++ {
		res, err := svc.GetSessionStatus(ctx, created.SessionID)
		require.NoError(t, err)
		require.Equal(t, "pending", res.Status)
	}
	require.EqualValues(t, 1, atomic.LoadInt32(&upstream.pollCount), "节流期内不得重复请求上游")
}

func TestOpenDesignDeviceAuthUpstreamErrorSetsFailedAndRetries(t *testing.T) {
	upstream := &fakeUpstream{createStatus: http.StatusInternalServerError}
	svc, _ := newOpenDesignAuthTestService(t, upstream)

	_, err := svc.InitiateDeviceAuth(context.Background(), "h")
	require.ErrorIs(t, err, ErrOpenDesignUpstreamError)

	// 上游恢复后同一服务可继续创建工作
	upstream.createStatus = http.StatusCreated
	created, err := svc.InitiateDeviceAuth(context.Background(), "h")
	require.NoError(t, err)
	require.NotEmpty(t, created.SessionID)
}

func TestOpenDesignDeviceAuthConsumeIsOneShot(t *testing.T) {
	upstream := &fakeUpstream{pollStatuses: []string{"authorized"}}
	svc, _ := newOpenDesignAuthTestService(t, upstream)
	ctx := context.Background()

	created, err := svc.InitiateDeviceAuth(ctx, "h")
	require.NoError(t, err)
	status, err := svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, "authorized", status.Status)

	creds, err := svc.BeginSessionConsumption(created.SessionID)
	require.NoError(t, err)
	require.Equal(t, "vela_ctrl_x", creds.ControlKey)
	require.Equal(t, "sk-amr-x", creds.RuntimeKey)
	require.Equal(t, "wstestworkspace01", creds.WorkspaceID)
	require.Equal(t, "dev@example.com", creds.Email)

	// 重放必拒（§8 用例 6）
	_, err = svc.BeginSessionConsumption(created.SessionID)
	require.ErrorIs(t, err, ErrOpenDesignSessionConsumed)

	// 落库失败可回滚后重试
	svc.AbortSessionConsumption(created.SessionID)
	creds2, err := svc.BeginSessionConsumption(created.SessionID)
	require.NoError(t, err)
	require.Equal(t, "vela_ctrl_x", creds2.ControlKey)

	// finalize 后内存中的敏感值必须清空
	svc.FinalizeSessionConsumption(created.SessionID)
	svc.mu.Lock()
	sess := svc.sessions[created.SessionID]
	require.Empty(t, sess.ControlKey)
	require.Empty(t, sess.RuntimeKey)
	require.Empty(t, sess.DeviceSecret)
	svc.mu.Unlock()
}

func TestOpenDesignDeviceAuthConsumeRequiresAuthorized(t *testing.T) {
	upstream := &fakeUpstream{pollStatuses: []string{"pending"}}
	svc, _ := newOpenDesignAuthTestService(t, upstream)
	ctx := context.Background()

	created, err := svc.InitiateDeviceAuth(ctx, "h")
	require.NoError(t, err)

	_, err = svc.BeginSessionConsumption(created.SessionID)
	require.ErrorIs(t, err, ErrOpenDesignSessionNotAuthorized)
}

func TestOpenDesignDeviceAuthCancelAndNotFound(t *testing.T) {
	upstream := &fakeUpstream{}
	svc, _ := newOpenDesignAuthTestService(t, upstream)

	created, err := svc.InitiateDeviceAuth(context.Background(), "h")
	require.NoError(t, err)

	svc.CancelSession(created.SessionID)
	_, err = svc.GetSessionStatus(context.Background(), created.SessionID)
	require.ErrorIs(t, err, ErrOpenDesignSessionNotFound, "取消后再查必须 404")
}

func TestOpenDesignDeviceAuthExpiredSession(t *testing.T) {
	upstream := &fakeUpstream{pollStatuses: []string{"authorized"}}
	svc, _ := newOpenDesignAuthTestService(t, upstream)
	ctx := context.Background()

	created, err := svc.InitiateDeviceAuth(ctx, "h")
	require.NoError(t, err)

	// 先授权拿到凭据，再人为把过期时间提前（等价于 15 分钟未授权，§8 用例 7）
	status, err := svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, "authorized", status.Status)

	svc.mu.Lock()
	svc.sessions[created.SessionID].ExpiresAt = time.Time{}
	svc.mu.Unlock()

	res, err := svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, "expired", res.Status)

	// 过期必须当场抹掉三类敏感值（第二轮复核低 2）
	svc.mu.Lock()
	sess := svc.sessions[created.SessionID]
	require.Empty(t, sess.ControlKey)
	require.Empty(t, sess.RuntimeKey)
	require.Empty(t, sess.DeviceSecret)
	svc.mu.Unlock()

	_, err = svc.BeginSessionConsumption(created.SessionID)
	require.ErrorIs(t, err, ErrOpenDesignSessionExpired)
}

// 跨过期点返回的轮询结果不得写入 authorized（第二轮复核低 2）。
func TestOpenDesignDeviceAuthSettleRejectsPollCrossingExpiry(t *testing.T) {
	upstream := &gatedUpstream{started: make(chan struct{}), release: make(chan struct{})}
	svc, _ := newOpenDesignAuthTestService(t, upstream)
	ctx := context.Background()

	created, err := svc.InitiateDeviceAuth(ctx, "h")
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		_, gerr := svc.GetSessionStatus(ctx, created.SessionID)
		done <- gerr
	}()
	<-upstream.started

	// 轮询在飞期间会话过期
	svc.mu.Lock()
	svc.sessions[created.SessionID].ExpiresAt = time.Time{}
	svc.mu.Unlock()

	close(upstream.release)
	require.NoError(t, <-done)

	svc.mu.Lock()
	sess := svc.sessions[created.SessionID]
	require.Equal(t, openDesignSessionStatusExpired, sess.Status, "跨过期点的轮询不得写入 authorized")
	require.Empty(t, sess.ControlKey)
	require.Empty(t, sess.RuntimeKey)
	svc.mu.Unlock()
}

func TestOpenDesignDeviceAuthWalletFailureStaysRecoverable(t *testing.T) {
	upstream := &fakeUpstream{pollStatuses: []string{"authorized"}, walletStatus: http.StatusInternalServerError}
	svc, _ := newOpenDesignAuthTestService(t, upstream)
	ctx := context.Background()

	created, err := svc.InitiateDeviceAuth(ctx, "h")
	require.NoError(t, err)

	res, err := svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	// wallet 失败不再判死：保持 pending，等待下次只补 wallet（复核 H1）
	require.Equal(t, openDesignSessionStatusPending, res.Status)
	require.Equal(t, openDesignReasonWorkspaceReadFailed, res.Reason)

	// workspace 未就绪时仍不得把半成品凭据交给落库路径
	_, err = svc.BeginSessionConsumption(created.SessionID)
	require.ErrorIs(t, err, ErrOpenDesignSessionNotAuthorized)
}

// TestOpenDesignDeviceAuthWalletFailureKeepsIssuedCredentials 复核 H1：
// 一次性凭据已下发但 wallet 暂时失败时，凭据必须保留，下次只重试 wallet、不再 poll token。
func TestOpenDesignDeviceAuthWalletFailureKeepsIssuedCredentials(t *testing.T) {
	upstream := &fakeUpstream{
		pollStatuses:   []string{"approved"},
		walletStatuses: []int{http.StatusServiceUnavailable, http.StatusOK},
	}
	svc, _ := newOpenDesignAuthTestService(t, upstream)
	ctx := context.Background()

	created, err := svc.InitiateDeviceAuth(ctx, "h")
	require.NoError(t, err)

	first, err := svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, openDesignSessionStatusPending, first.Status)
	require.Equal(t, openDesignReasonWorkspaceReadFailed, first.Reason)

	// 凭据必须在第一次失败时就保留在会话里
	svc.mu.Lock()
	require.Equal(t, "vela_ctrl_x", svc.sessions[created.SessionID].ControlKey)
	require.Equal(t, "sk-amr-x", svc.sessions[created.SessionID].RuntimeKey)
	svc.sessions[created.SessionID].LastPollAt = time.Time{}
	svc.mu.Unlock()

	second, err := svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, openDesignSessionStatusAuthorized, second.Status)
	require.Equal(t, "wstestworkspace01", second.WorkspaceID)
	require.Equal(t, "dev@example.com", second.Email)
	require.EqualValues(t, 1, atomic.LoadInt32(&upstream.pollCount), "补 workspace 不得再次 poll token（凭据一次性）")
	require.EqualValues(t, 2, atomic.LoadInt32(&upstream.walletHits))
}

// TestOpenDesignDeviceAuthExpiredOverridesStaleReason 复核 L1：
// 本地过期是终态，不得把旧故障 reason 带到过期提示上。
func TestOpenDesignDeviceAuthExpiredOverridesStaleReason(t *testing.T) {
	upstream := &fakeUpstream{
		pollStatuses:   []string{"approved"},
		walletStatuses: []int{http.StatusServiceUnavailable},
	}
	svc, _ := newOpenDesignAuthTestService(t, upstream)
	ctx := context.Background()

	created, err := svc.InitiateDeviceAuth(ctx, "h")
	require.NoError(t, err)

	first, err := svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, openDesignReasonWorkspaceReadFailed, first.Reason)

	svc.mu.Lock()
	svc.sessions[created.SessionID].ExpiresAt = time.Now().Add(-time.Second)
	svc.mu.Unlock()

	res, err := svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, openDesignSessionStatusExpired, res.Status)
	require.Equal(t, openDesignReasonExpired, res.Reason)
	require.Empty(t, res.Message)
}

// TestOpenDesignDeviceAuthHoldsInFlightDuringWorkspaceFetch 复核第二轮并发窗口：
// approved 已取得凭据、workspace 尚未补到的这段时间里，并发查询不得再次 poll token。
func TestOpenDesignDeviceAuthHoldsInFlightDuringWorkspaceFetch(t *testing.T) {
	block := make(chan struct{})
	var releaseOnce sync.Once
	releaseWallet := func() { releaseOnce.Do(func() { close(block) }) }
	defer releaseWallet()
	upstream := &fakeUpstream{pollStatuses: []string{"approved"}, walletBlock: block}
	svc, _ := newOpenDesignAuthTestService(t, upstream)
	ctx := context.Background()

	created, err := svc.InitiateDeviceAuth(ctx, "h")
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = svc.GetSessionStatus(ctx, created.SessionID)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for atomic.LoadInt32(&upstream.walletHits) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	require.EqualValues(t, 1, atomic.LoadInt32(&upstream.walletHits), "首次请求应已进入 wallet 补取")

	// 绕过 2s 节流，确保命中的是 in-flight 守卫而不是节流
	svc.mu.Lock()
	svc.sessions[created.SessionID].LastPollAt = time.Time{}
	svc.mu.Unlock()

	concurrent, err := svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, openDesignSessionStatusPending, concurrent.Status, "补 workspace 期间只能回缓存")
	require.EqualValues(t, 1, atomic.LoadInt32(&upstream.pollCount), "不得再次 poll token（凭据一次性）")

	releaseWallet()
	<-done

	svc.mu.Lock()
	finalStatus := svc.sessions[created.SessionID].Status
	svc.mu.Unlock()
	require.Equal(t, openDesignSessionStatusAuthorized, finalStatus)
}

func TestOpenDesignDeviceAuthRejectsMalformedWorkspaceID(t *testing.T) {
	upstream := &fakeUpstream{pollStatuses: []string{"authorized"}, workspaceID: "../../etc/passwd"}
	svc, _ := newOpenDesignAuthTestService(t, upstream)

	created, err := svc.InitiateDeviceAuth(context.Background(), "h")
	require.NoError(t, err)

	res, err := svc.GetSessionStatus(context.Background(), created.SessionID)
	require.NoError(t, err)
	require.Equal(t, openDesignSessionStatusPending, res.Status, "含路径字符的 workspaceId 不得进入凭据")
	require.Equal(t, openDesignReasonWorkspaceReadFailed, res.Reason)
}

// gatedUpstream 让 /token 阻塞到 release 关闭，用于构造「慢轮询 + 快路径已消费」的交错。
type gatedUpstream struct {
	started    chan struct{}
	release    chan struct{}
	startOnce  sync.Once
	tokenCalls int32
}

func (g *gatedUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/cli/device-authorizations"):
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"deviceId":"dev_1","userCode":"GQRSUT","status":"pending",` +
			`"expiresAt":"2099-01-01T00:00:00Z","deviceSecret":"secret-1",` +
			`"activationUrl":"https://open-design.ai/cloud/cli/activate?userCode=GQRSUT","pollIntervalSeconds":2}`))
	case strings.HasSuffix(r.URL.Path, "/token"):
		atomic.AddInt32(&g.tokenCalls, 1)
		g.startOnce.Do(func() { close(g.started) })
		<-g.release
		_, _ = w.Write([]byte(`{"status":"authorized","controlKey":"vela_ctrl_x","runtimeKey":"sk-amr-x",` +
			`"user":{"email":"dev@example.com"}}`))
	case strings.HasSuffix(r.URL.Path, "/wallet/balance"):
		_, _ = w.Write([]byte(`{"workspaceId":"wstestworkspace01"}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// 反序并发回归（2026-11-15 复核阻断 1）：
// 一次慢轮询在会话已被消费并 finalize 之后返回 authorized，**不得**把 status / 敏感凭据
// 写回会话，否则同一会话可被二次消费（重复建号）。
func TestOpenDesignDeviceAuthLatePollCannotResurrectConsumedSession(t *testing.T) {
	upstream := &gatedUpstream{started: make(chan struct{}), release: make(chan struct{})}
	svc, _ := newOpenDesignAuthTestService(t, upstream)
	ctx := context.Background()

	created, err := svc.InitiateDeviceAuth(ctx, "h")
	require.NoError(t, err)

	slowDone := make(chan error, 1)
	go func() {
		_, gerr := svc.GetSessionStatus(ctx, created.SessionID)
		slowDone <- gerr
	}()
	<-upstream.started // 慢轮询已进入上游

	// 模拟并发的「快路径」：会话已授权 → 被消费 → finalize 抹除敏感值
	svc.mu.Lock()
	sess := svc.sessions[created.SessionID]
	sess.Status = openDesignSessionStatusAuthorized
	sess.ControlKey = "vela_ctrl_x"
	sess.RuntimeKey = "sk-amr-x"
	sess.WorkspaceID = "wstestworkspace01"
	svc.mu.Unlock()

	_, err = svc.BeginSessionConsumption(created.SessionID)
	require.NoError(t, err)
	svc.FinalizeSessionConsumption(created.SessionID)

	// 放行迟到的旧轮询
	close(upstream.release)
	require.NoError(t, <-slowDone)

	svc.mu.Lock()
	status := sess.Status
	controlKey := sess.ControlKey
	runtimeKey := sess.RuntimeKey
	inFlight := sess.PollInFlight
	svc.mu.Unlock()

	require.Equal(t, openDesignSessionStatusConsumed, status, "迟到的旧轮询不得把已消费会话改回 authorized")
	require.Empty(t, controlKey, "迟到的旧轮询不得写回 controlKey")
	require.Empty(t, runtimeKey, "迟到的旧轮询不得写回 runtimeKey")
	require.False(t, inFlight, "in-flight 标记必须被解除")

	_, err = svc.BeginSessionConsumption(created.SessionID)
	require.ErrorIs(t, err, ErrOpenDesignSessionConsumed, "会话仍必须是一次性的")
}

// 同一会话的上游轮询必须串行：第一次还在飞时，第二次只回缓存，不得重复打上游。
func TestOpenDesignDeviceAuthSerializesInFlightPoll(t *testing.T) {
	upstream := &gatedUpstream{started: make(chan struct{}), release: make(chan struct{})}
	svc, _ := newOpenDesignAuthTestService(t, upstream)
	ctx := context.Background()

	created, err := svc.InitiateDeviceAuth(ctx, "h")
	require.NoError(t, err)

	slowDone := make(chan error, 1)
	go func() {
		_, gerr := svc.GetSessionStatus(ctx, created.SessionID)
		slowDone <- gerr
	}()
	<-upstream.started

	// 轮询在飞期间再次查询：直接回缓存 pending，不产生第二次上游调用
	res, err := svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, openDesignSessionStatusPending, res.Status)
	require.EqualValues(t, 1, atomic.LoadInt32(&upstream.tokenCalls), "in-flight 期间不得重复轮询上游")

	close(upstream.release)
	require.NoError(t, <-slowDone)
	require.EqualValues(t, 1, atomic.LoadInt32(&upstream.tokenCalls))
}

// TestOpenDesignDeviceAuthApprovedStatusAuthorizes 覆盖上游实证状态 approved。
// 2026-11-16 生产故障根因：只认 authorized 会把已批准、已下发的凭据当未知状态丢弃。
func TestOpenDesignDeviceAuthApprovedStatusAuthorizes(t *testing.T) {
	upstream := &fakeUpstream{pollStatuses: []string{"approved"}}
	svc, _ := newOpenDesignAuthTestService(t, upstream)
	ctx := context.Background()

	created, err := svc.InitiateDeviceAuth(ctx, "h")
	require.NoError(t, err)

	status, err := svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, openDesignSessionStatusAuthorized, status.Status)
	require.Equal(t, "dev@example.com", status.Email)
	require.Equal(t, "wstestworkspace01", status.WorkspaceID)
	require.Empty(t, status.Message)
	require.Empty(t, status.Reason)
	require.EqualValues(t, 1, atomic.LoadInt32(&upstream.walletHits), "approved 必须用 controlKey 取 workspaceId")
}

// TestOpenDesignDeviceAuthApprovedStatusIsCaseInsensitive 上游状态大小写/空白不应影响判定。
func TestOpenDesignDeviceAuthApprovedStatusIsCaseInsensitive(t *testing.T) {
	upstream := &fakeUpstream{pollStatuses: []string{"  APPROVED "}}
	svc, _ := newOpenDesignAuthTestService(t, upstream)
	ctx := context.Background()

	created, err := svc.InitiateDeviceAuth(ctx, "h")
	require.NoError(t, err)

	status, err := svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, openDesignSessionStatusAuthorized, status.Status)
	require.Equal(t, "wstestworkspace01", status.WorkspaceID)
}

// TestOpenDesignDeviceAuthConsumedBecomesTerminalStatus 上游 consumed（凭据已被领取）
// 必须立即置终态并停止轮询，而不是每 2 秒空转到会话过期。
func TestOpenDesignDeviceAuthConsumedBecomesTerminalStatus(t *testing.T) {
	upstream := &fakeUpstream{pollStatuses: []string{"consumed", "consumed", "consumed"}}
	svc, _ := newOpenDesignAuthTestService(t, upstream)
	ctx := context.Background()

	created, err := svc.InitiateDeviceAuth(ctx, "h")
	require.NoError(t, err)

	first, err := svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, openDesignSessionStatusConsumed, first.Status)
	require.Contains(t, first.Message, "already issued")
	require.Equal(t, openDesignReasonCredentialConsumed, first.Reason)

	// 终态后不得再轮询上游
	svc.mu.Lock()
	svc.sessions[created.SessionID].LastPollAt = time.Time{}
	svc.mu.Unlock()
	second, err := svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, openDesignSessionStatusConsumed, second.Status)
	require.EqualValues(t, 1, atomic.LoadInt32(&upstream.pollCount), "consumed 终态必须停止轮询")
}

// TestOpenDesignDeviceAuthUnknownStatusStaysPending 未知状态不得判失败（防上游新增中间态误杀）。
func TestOpenDesignDeviceAuthUnknownStatusStaysPending(t *testing.T) {
	upstream := &fakeUpstream{pollStatuses: []string{"authorizing", "approved"}}
	svc, _ := newOpenDesignAuthTestService(t, upstream)
	ctx := context.Background()

	created, err := svc.InitiateDeviceAuth(ctx, "h")
	require.NoError(t, err)

	first, err := svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, openDesignSessionStatusPending, first.Status, "未知状态必须保持等待")
	require.Empty(t, first.Message)

	svc.mu.Lock()
	svc.sessions[created.SessionID].LastPollAt = time.Time{}
	svc.mu.Unlock()
	second, err := svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, openDesignSessionStatusAuthorized, second.Status, "后续 approved 应正常授权")
}

// TestOpenDesignDeviceAuthDeniedIsTerminal 浏览器拒绝是终态：停轮询并给重新发起出口。
func TestOpenDesignDeviceAuthDeniedIsTerminal(t *testing.T) {
	upstream := &fakeUpstream{pollStatuses: []string{"denied", "pending"}}
	svc, _ := newOpenDesignAuthTestService(t, upstream)
	ctx := context.Background()

	created, err := svc.InitiateDeviceAuth(ctx, "h")
	require.NoError(t, err)

	status, err := svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, openDesignSessionStatusDenied, status.Status)
	require.Contains(t, status.Message, "denied")
	require.Equal(t, openDesignReasonDenied, status.Reason)

	// 绕过缓存再查：不得再 poll token，也不得被后续 pending 覆盖
	svc.mu.Lock()
	svc.sessions[created.SessionID].LastPollAt = time.Time{}
	svc.mu.Unlock()
	again, err := svc.GetSessionStatus(ctx, created.SessionID)
	require.NoError(t, err)
	require.Equal(t, openDesignSessionStatusDenied, again.Status)
	require.EqualValues(t, 1, atomic.LoadInt32(&upstream.pollCount))
}
