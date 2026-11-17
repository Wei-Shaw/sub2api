package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// OpenDesign 一键设备授权（OAuth 取凭据）服务：
// 服务端代理 vela CLI 的官方设备授权流，一次授权即可拿到 controlKey（探测用）、
// runtimeKey（= api_key，转发用）、email 与 workspaceId，彻底替代「运维本机跑
// vela.exe login + 手工抄 controlKey」的两步流程。
//
// 硬约束：账号 type 必须保持 apikey——用量探测与转发判定都要求
// platform=open_design && type=apikey，新增 AccountTypeOAuth 会同时打断两者。
// UI 上的「OAuth」只是取凭据方式，用 extra.opendesign_auth_source="oauth" 区分。
//
// 安全：deviceSecret / controlKey / runtimeKey 只在内存中转，不返回前端、
// 不写日志、不进错误信息；会话被消费或取消后立即清空敏感字段。

const (
	// OpenDesignAuthSourceExtraKey 标记账号凭据来源（oauth / apikey），仅用于 UI 展示。
	// OpenDesignUsageCredentialsKey / OpenDesignUsageWorkspaceKey are the probe
	// credential slots in credentials. Device auth writes them; the CN provider
	// quota probe reads them.
	OpenDesignUsageCredentialsKey = "opendesign_control_key"
	OpenDesignUsageWorkspaceKey   = "opendesign_workspace_id"
	OpenDesignAuthSourceExtraKey = "opendesign_auth_source"
	// OpenDesignAuthSourceOAuth 表示凭据由一键设备授权取得。
	OpenDesignAuthSourceOAuth = "oauth"

	openDesignDefaultCLIVersion = "0.1.6"
	openDesignDefaultProfile    = "prod"
	// openDesignAuthDefaultAPIBaseURL 是控制面（设备授权 + 钱包）基址；
	// 转发面仍是 amr-link.open-design.ai（见 DefaultOpenDesignBaseURL）。

	openDesignSessionDefaultTTL = 15 * time.Minute
	openDesignSessionMaxAge     = 20 * time.Minute
	openDesignMinPollInterval   = 2 * time.Second
	openDesignAuthHTTPTimeout   = 10 * time.Second

	openDesignSessionStatusPending    = "pending"
	openDesignSessionStatusAuthorized = "authorized"
	openDesignSessionStatusExpired    = "expired"
	openDesignSessionStatusFailed     = "failed"
	openDesignSessionStatusConsumed   = "consumed"
	// openDesignSessionStatusDenied 是本地的「浏览器拒绝」终态（上游 denied 的映射）。
	// 与 expired 分开，避免过期原因规范化覆盖拒绝原因。
	openDesignSessionStatusDenied = "denied"

	// openDesignUpstreamStatusApproved 是上游设备授权「已批准」状态。
	// 实证（2026-11-16 生产旁路轮询）：授权成功后上游返回 approved，并在同一响应里
	// 下发 controlKey / runtimeKey / controlKeyId / user；凭据被首个轮询领取后转为
	// consumed。设计文档当时推测的 authorized 并不存在，只认 authorized 会把到手的
	// 凭据当未知状态丢弃（2026-11-16 生产实弹故障根因）。
	openDesignUpstreamStatusApproved = "approved"
	// openDesignUpstreamStatusDenied 表示用户在浏览器拒绝了授权（RFC 8628 access_denied 同义）。
	openDesignUpstreamStatusDenied = "denied"

	// 以下 reason 为机器码，仅供前端映射中文提示；后端 message 保持英文用于日志排障，
	// 不再直接展示给管理员（2026-11-16 反馈：不要显示看不懂的英文代码串）。
	openDesignReasonDenied              = "denied"
	openDesignReasonCredentialConsumed  = "credential_consumed"
	openDesignReasonExpired             = "expired"
	openDesignReasonWorkspaceReadFailed = "workspace_read_failed"
	openDesignReasonUpstreamUnavailable = "upstream_unavailable"
	openDesignReasonUnknownStatus       = "unknown_status"
)

var (
	ErrOpenDesignSessionNotFound      = infraerrors.New(http.StatusNotFound, "OPENDESIGN_SESSION_NOT_FOUND", "OpenDesign device auth session not found or already cleaned up")
	ErrOpenDesignSessionNotAuthorized = infraerrors.New(http.StatusConflict, "OPENDESIGN_SESSION_NOT_AUTHORIZED", "OpenDesign device auth session is not authorized yet")
	ErrOpenDesignSessionConsumed      = infraerrors.New(http.StatusConflict, "OPENDESIGN_SESSION_CONSUMED", "OpenDesign device auth session has already been used")
	ErrOpenDesignSessionExpired       = infraerrors.New(http.StatusGone, "OPENDESIGN_SESSION_EXPIRED", "OpenDesign device auth session has expired")
	ErrOpenDesignUpstreamError        = infraerrors.New(http.StatusBadGateway, "OPENDESIGN_UPSTREAM_ERROR", "OpenDesign authorization service is temporarily unavailable")
	// ErrOpenDesignDeviceAuthAccountInvalid 表示重新授权时给的目标账号不是 open_design 账号。
	ErrOpenDesignDeviceAuthAccountInvalid = infraerrors.New(http.StatusBadRequest, "OPENDESIGN_ACCOUNT_INVALID", "target account is not an OpenDesign account")
	// ErrOpenDesignDeviceAuthAccountAmbiguous 表示授权身份命中多个 open_design 账号，
	// 必须由管理员显式指定 account_id（不允许平台猜测目标账号）。
	ErrOpenDesignDeviceAuthAccountAmbiguous = infraerrors.New(http.StatusConflict, "OPENDESIGN_ACCOUNT_AMBIGUOUS", "multiple OpenDesign accounts match this authorization, please specify account_id")
)

// openDesignDeviceSession 是单个设备授权会话（仅存进程内存，不落库）。
type openDesignDeviceSession struct {
	DeviceID      string
	DeviceSecret  string
	UserCode      string
	ActivationURL string
	ExpiresAt     time.Time
	Status        string
	ControlKey    string
	RuntimeKey    string
	WorkspaceID   string
	Email         string
	LastError     string
	Reason        string
	CreatedAt     time.Time
	LastPollAt    time.Time
	// PollInFlight 表示当前已有一次上游轮询在飞（锁外 HTTP），避免并发重复轮询。
	PollInFlight bool
}

// OpenDesignDeviceAuthService 管理设备授权会话与上游 HTTP 交互。
type OpenDesignDeviceAuthService struct {
	apiBaseURL string
	cliVersion string
	httpClient *http.Client

	mu       sync.Mutex
	sessions map[string]*openDesignDeviceSession

	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewOpenDesignDeviceAuthService 构造服务并启动过期会话清理循环。
// ErrOpenDesignUsageUnavailable is returned when the optional OpenDesign
// device-auth/usage wiring is not attached.
var ErrOpenDesignUsageUnavailable = infraerrors.ServiceUnavailable(
	"OPEN_DESIGN_USAGE_UNAVAILABLE", "OpenDesign usage is unavailable",
)

func NewOpenDesignDeviceAuthService() *OpenDesignDeviceAuthService {
	cliVersion := strings.TrimSpace(os.Getenv("OPENDESIGN_CLI_VERSION"))
	if cliVersion == "" {
		cliVersion = openDesignDefaultCLIVersion
	}
	apiBaseURL := strings.TrimSpace(os.Getenv("OPENDESIGN_AUTH_API_BASE_URL"))
	if apiBaseURL == "" {
		apiBaseURL = DefaultOpenDesignAuthAPIBaseURL
	}

	svc := &OpenDesignDeviceAuthService{
		apiBaseURL: strings.TrimRight(apiBaseURL, "/"),
		cliVersion: cliVersion,
		httpClient: &http.Client{Timeout: openDesignAuthHTTPTimeout},
		sessions:   make(map[string]*openDesignDeviceSession),
		stopCh:     make(chan struct{}),
	}
	go svc.cleanupLoop()
	return svc
}

// Stop 停止清理循环（应用关停时调用）。
func (s *OpenDesignDeviceAuthService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { close(s.stopCh) })
}

func (s *OpenDesignDeviceAuthService) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			s.mu.Lock()
			for id, sess := range s.sessions {
				if now.After(sess.ExpiresAt) || now.Sub(sess.CreatedAt) > openDesignSessionMaxAge {
					// 清理前先抹掉敏感值，避免 GC 前残留可读凭据。
					clearOpenDesignSessionSecrets(sess)
					delete(s.sessions, id)
				}
			}
			s.mu.Unlock()
		}
	}
}

func newOpenDesignDeviceSessionID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "ods_" + hex.EncodeToString(buf), nil
}

// OpenDesignDeviceAuthInitiateResult 是「创建会话」接口的对外响应，
// 刻意不含 deviceId / deviceSecret / controlKey。
type OpenDesignDeviceAuthInitiateResult struct {
	SessionID           string    `json:"sessionId"`
	UserCode            string    `json:"userCode"`
	ActivationURL       string    `json:"activationUrl"`
	ExpiresAt           time.Time `json:"expiresAt"`
	PollIntervalSeconds int       `json:"pollIntervalSeconds"`
}

// OpenDesignDeviceAuthStatusResult 是「查询会话状态」接口的对外响应。
type OpenDesignDeviceAuthStatusResult struct {
	Status      string `json:"status"`
	Email       string `json:"email,omitempty"`
	WorkspaceID string `json:"workspaceId,omitempty"`
	Message     string `json:"message,omitempty"`
	// Reason 是机器码（denied / credential_consumed / expired / workspace_read_failed /
	// upstream_unavailable / unknown_status），前端按它渲染中文提示，避免透出英文原文。
	Reason string `json:"reason,omitempty"`
}

// OpenDesignSessionCredentials 是消费会话后交给账号落库的敏感凭据（不返回前端）。
type OpenDesignSessionCredentials struct {
	Email       string
	ControlKey  string
	RuntimeKey  string
	WorkspaceID string
}

func (s *OpenDesignDeviceAuthService) statusSnapshot(sess *openDesignDeviceSession) *OpenDesignDeviceAuthStatusResult {
	res := &OpenDesignDeviceAuthStatusResult{
		Status:  sess.Status,
		Message: sess.LastError,
		Reason:  sess.Reason,
	}
	if sess.Status == openDesignSessionStatusAuthorized {
		res.Email = sess.Email
		res.WorkspaceID = sess.WorkspaceID
		res.Message = ""
		res.Reason = ""
	}
	// 过期是终态：无论此前遗留下什么故障原因，都统一按「已过期」呈现，
	// 避免把 workspace/unavailable 的旧 reason 带到过期提示上（复核 L1）。
	if sess.Status == openDesignSessionStatusExpired {
		res.Message = ""
		res.Reason = openDesignReasonExpired
	}
	return res
}

// InitiateDeviceAuth 调官方接口创建设备授权并登记内存会话。
func (s *OpenDesignDeviceAuthService) InitiateDeviceAuth(ctx context.Context, host string) (*OpenDesignDeviceAuthInitiateResult, error) {
	if strings.TrimSpace(host) == "" {
		if name, err := os.Hostname(); err == nil && strings.TrimSpace(name) != "" {
			host = name
		} else {
			host = "sub2api"
		}
	}

	// body 必须含 cliVersion（缺省会 500，实测），host/profile 一并带上。
	body, err := json.Marshal(map[string]string{
		"cliVersion": s.cliVersion,
		"host":       host,
		"profile":    openDesignDefaultProfile,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal device auth request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiBaseURL+"/api/v1/cli/device-authorizations", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build device auth request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "Go-http-client/2.0")

	startedAt := time.Now()
	resp, err := s.httpClient.Do(httpReq)
	elapsed := time.Since(startedAt)
	if err != nil {
		// 只记录状态与耗时，绝不记录 body / 凭据（§3.5）。
		slog.Error("opendesign_device_auth.create_failed", "duration_ms", elapsed.Milliseconds(), "err", err)
		return nil, ErrOpenDesignUpstreamError
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	slog.Info("opendesign_device_auth.create_response",
		"status", resp.StatusCode, "duration_ms", elapsed.Milliseconds(), "bytes", len(respBody))
	if readErr != nil {
		return nil, fmt.Errorf("read device auth response: %w", readErr)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, ErrOpenDesignUpstreamError
	}

	var payload struct {
		DeviceID            string `json:"deviceId"`
		UserCode            string `json:"userCode"`
		ExpiresAt           string `json:"expiresAt"`
		DeviceSecret        string `json:"deviceSecret"`
		ActivationURL       string `json:"activationUrl"`
		PollIntervalSeconds int    `json:"pollIntervalSeconds"`
	}
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, fmt.Errorf("decode device auth response: %w", err)
	}
	if strings.TrimSpace(payload.DeviceID) == "" || strings.TrimSpace(payload.DeviceSecret) == "" {
		slog.Error("opendesign_device_auth.create_incomplete_response")
		return nil, ErrOpenDesignUpstreamError
	}

	expiresAt := parseOpenDesignTimestamp(payload.ExpiresAt)
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(openDesignSessionDefaultTTL)
	}
	pollInterval := payload.PollIntervalSeconds
	if pollInterval <= 0 {
		pollInterval = int(openDesignMinPollInterval / time.Second)
	}

	sessionID, err := newOpenDesignDeviceSessionID()
	if err != nil {
		return nil, fmt.Errorf("generate device session id: %w", err)
	}

	now := time.Now()
	s.mu.Lock()
	s.sessions[sessionID] = &openDesignDeviceSession{
		DeviceID:      payload.DeviceID,
		DeviceSecret:  payload.DeviceSecret,
		UserCode:      payload.UserCode,
		ActivationURL: payload.ActivationURL,
		ExpiresAt:     expiresAt,
		Status:        openDesignSessionStatusPending,
		CreatedAt:     now,
	}
	s.mu.Unlock()

	return &OpenDesignDeviceAuthInitiateResult{
		SessionID:           sessionID,
		UserCode:            payload.UserCode,
		ActivationURL:       payload.ActivationURL,
		ExpiresAt:           expiresAt,
		PollIntervalSeconds: pollInterval,
	}, nil
}

func parseOpenDesignTimestamp(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if ts, err := time.Parse(layout, raw); err == nil {
			return ts
		}
	}
	return time.Time{}
}

// clearOpenDesignSessionSecrets 抹掉会话里的全部敏感值（取消 / 过期 / 消费共用）。
// 调用方必须已持有 s.mu。
func clearOpenDesignSessionSecrets(sess *openDesignDeviceSession) {
	if sess == nil {
		return
	}
	sess.DeviceSecret = ""
	sess.ControlKey = ""
	sess.RuntimeKey = ""
}

// GetSessionStatus 返回会话状态，必要时向上游轮询一次（2 秒节流）。
//
// 并发安全：上游 HTTP 在锁外执行，因此返回后必须重新确认「会话仍存在、仍是同一
// 会话对象、状态仍处于可轮询态」才允许回写。否则一次慢轮询可能在会话被消费
// （consumed）并抹除敏感值之后把 status/凭据写回去，让同一会话被二次消费
// （2026-11-15 复核阻断 1）。
func (s *OpenDesignDeviceAuthService) GetSessionStatus(ctx context.Context, sessionID string) (*OpenDesignDeviceAuthStatusResult, error) {
	s.mu.Lock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return nil, ErrOpenDesignSessionNotFound
	}

	now := time.Now()
	if now.After(sess.ExpiresAt) {
		sess.Status = openDesignSessionStatusExpired
		sess.PollInFlight = false
		clearOpenDesignSessionSecrets(sess)
		res := s.statusSnapshot(sess)
		s.mu.Unlock()
		return res, nil
	}

	// authorized / consumed / expired 直接返回；pending 与 failed 允许继续轮询。
	if sess.Status != openDesignSessionStatusPending && sess.Status != openDesignSessionStatusFailed {
		res := s.statusSnapshot(sess)
		s.mu.Unlock()
		return res, nil
	}

	// 已有一次轮询在飞：直接回缓存状态，既防重复上游调用，也避免并发交错。
	if sess.PollInFlight {
		res := s.statusSnapshot(sess)
		s.mu.Unlock()
		return res, nil
	}

	// 节流：距上次上游轮询不足 2 秒直接返回缓存状态，
	// 避免多个管理员同时开弹窗把上游打爆（§3.3）。
	if !sess.LastPollAt.IsZero() && now.Sub(sess.LastPollAt) < openDesignMinPollInterval {
		res := s.statusSnapshot(sess)
		s.mu.Unlock()
		return res, nil
	}
	sess.LastPollAt = now
	sess.PollInFlight = true
	deviceID := sess.DeviceID
	deviceSecret := sess.DeviceSecret
	// 一次性凭据已领到、只差 workspace：后续查询只补 wallet，绝不再 poll token
	// （token 已被 consumed，再 poll 只会确认凭据丢失）。
	resumeControlKey := sess.ControlKey
	resumeWorkspace := resumeControlKey != "" && sess.WorkspaceID == ""
	s.mu.Unlock()

	if resumeWorkspace {
		return s.completeWorkspace(ctx, sessionID, sess, resumeControlKey)
	}

	polled, err := s.pollUpstreamToken(ctx, deviceID, deviceSecret)
	if err != nil {
		// 上游临时故障：置 failed 并保留会话，下次查询自动重试。
		return s.settlePoll(sessionID, sess, func(target *openDesignDeviceSession) {
			target.Status = openDesignSessionStatusFailed
			target.LastError = "OpenDesign authorization service is temporarily unavailable, please retry"
			target.Reason = openDesignReasonUpstreamUnavailable
		})
	}

	switch polled.Status {
	case openDesignSessionStatusPending:
		return s.settlePoll(sessionID, sess, func(target *openDesignDeviceSession) {
			target.Status = openDesignSessionStatusPending
			target.LastError = ""
			target.Reason = ""
		})

	case openDesignUpstreamStatusApproved, openDesignSessionStatusAuthorized:
		// 上游实际用 approved 表示「已批准且凭据已下发」，authorized 作为兼容值保留。
		// 先确认会话仍可轮询，再做钱包查询（避免为已消费会话白跑一次上游）。
		if !s.isSessionPollable(sessionID, sess) {
			return s.settlePoll(sessionID, sess, nil)
		}
		// 一次性凭据先落库、再补 workspace：wallet 临时失败不得导致凭据丢失（复核 H1）。
		if !s.saveIssuedCredentials(sessionID, sess, polled) {
			return s.settlePoll(sessionID, sess, nil)
		}
		return s.completeWorkspace(ctx, sessionID, sess, polled.ControlKey)

	case openDesignSessionStatusConsumed:
		// 上游凭据已被首个轮询领取（一次性下发），本地没拿到即不可再取回：
		// 置终态，停止轮询，提示重新发起授权（不能像旧实现那样一直空转到过期）。
		slog.Info("opendesign_device_auth.upstream_consumed")
		return s.settlePoll(sessionID, sess, func(target *openDesignDeviceSession) {
			target.Status = openDesignSessionStatusConsumed
			target.LastError = "OpenDesign authorization credential was already issued and cannot be retrieved again; please start a new authorization"
			target.Reason = openDesignReasonCredentialConsumed
			clearOpenDesignSessionSecrets(target)
		})

	case openDesignSessionStatusExpired:
		slog.Info("opendesign_device_auth.upstream_expired")
		return s.settlePoll(sessionID, sess, func(target *openDesignDeviceSession) {
			target.Status = openDesignSessionStatusExpired
			target.LastError = "OpenDesign authorization expired, please start a new authorization"
			target.Reason = openDesignReasonExpired
			clearOpenDesignSessionSecrets(target)
		})

	case openDesignUpstreamStatusDenied:
		slog.Info("opendesign_device_auth.upstream_denied")
		return s.settlePoll(sessionID, sess, func(target *openDesignDeviceSession) {
			// 拒绝是终态：独立状态（非 pending/failed）从而停止轮询，并清掉任何半成品凭据。
			target.Status = openDesignSessionStatusDenied
			target.LastError = "OpenDesign authorization was denied in the browser"
			target.Reason = openDesignReasonDenied
			clearOpenDesignSessionSecrets(target)
		})

	default:
		// 未知状态不再判失败：保持等待并记录字面值，避免上游新增中间态时把已到手的
		// 授权误杀（本次生产故障的直接教训）。
		slog.Warn("opendesign_device_auth.unknown_upstream_status", "upstream_status", polled.Status)
		return s.settlePoll(sessionID, sess, func(target *openDesignDeviceSession) {
			target.Status = openDesignSessionStatusPending
			target.LastError = ""
			target.Reason = ""
		})
	}
}

// isSessionPollable 报告会话是否仍可接收轮询结果（存在、同一对象、status 可轮询）。
func (s *OpenDesignDeviceAuthService) isSessionPollable(sessionID string, sess *openDesignDeviceSession) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions[sessionID] != sess {
		return false
	}
	return sess.Status == openDesignSessionStatusPending || sess.Status == openDesignSessionStatusFailed
}

// settlePoll 在锁内落定一次轮询结果：先解除 in-flight 标记，再校验会话仍可轮询。
// 若会话已被删除 → 返回 not found；若已被消费/授权/过期（迟到的旧轮询）→ 只回
// 当前快照，**不回写任何状态或凭据**，防止消费后被复活。
func (s *OpenDesignDeviceAuthService) settlePoll(
	sessionID string,
	sess *openDesignDeviceSession,
	apply func(*openDesignDeviceSession),
) (*OpenDesignDeviceAuthStatusResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess.PollInFlight = false
	if s.sessions[sessionID] != sess {
		return nil, ErrOpenDesignSessionNotFound
	}
	// 跨过期点返回的轮询同样不得写入：统一置 expired 并抹除敏感值（低 2）。
	if time.Now().After(sess.ExpiresAt) {
		sess.Status = openDesignSessionStatusExpired
		clearOpenDesignSessionSecrets(sess)
		return s.statusSnapshot(sess), nil
	}
	if sess.Status != openDesignSessionStatusPending && sess.Status != openDesignSessionStatusFailed {
		return s.statusSnapshot(sess), nil
	}
	if apply != nil {
		apply(sess)
	}
	return s.statusSnapshot(sess), nil
}

// saveIssuedCredentials 把上游一次性下发的凭据先落进会话（状态保持 pending），
// 之后再单独补 workspace。返回 false 表示会话已不可写（被消费/过期/对象已换）。
func (s *OpenDesignDeviceAuthService) saveIssuedCredentials(
	sessionID string,
	sess *openDesignDeviceSession,
	polled *openDesignTokenPollResult,
) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions[sessionID] != sess {
		return false
	}
	if time.Now().After(sess.ExpiresAt) {
		sess.PollInFlight = false
		sess.Status = openDesignSessionStatusExpired
		clearOpenDesignSessionSecrets(sess)
		return false
	}
	if sess.Status != openDesignSessionStatusPending && sess.Status != openDesignSessionStatusFailed {
		sess.PollInFlight = false
		return false
	}
	// 成功路径**不清** PollInFlight：凭据已落库但 workspace 尚未就绪，
	// 继续保持「有请求在飞」可让并发查询只回缓存，避免在补 workspace 期间
	// 再次 poll token（复核第二轮）。in-flight 由 completeWorkspace 末尾的 settlePoll 统一清除。
	sess.ControlKey = polled.ControlKey
	sess.RuntimeKey = polled.RuntimeKey
	sess.Email = polled.Email
	sess.Status = openDesignSessionStatusPending
	sess.LastError = ""
	sess.Reason = ""
	return true
}

// completeWorkspace 用已落库的 controlKey 查询 workspaceId。
// 失败时保持 pending：下一次查询走 resumeWorkspace 分支只重试 wallet，不再 poll token，
// 因此本次已领取的一次性凭据不会丢失（复核 H1）。
func (s *OpenDesignDeviceAuthService) completeWorkspace(
	ctx context.Context,
	sessionID string,
	sess *openDesignDeviceSession,
	controlKey string,
) (*OpenDesignDeviceAuthStatusResult, error) {
	workspaceID, err := s.fetchWorkspaceID(ctx, controlKey)
	if err != nil {
		slog.Warn("opendesign_device_auth.fetch_workspace_failed", "err", err)
		return s.settlePoll(sessionID, sess, func(target *openDesignDeviceSession) {
			target.Status = openDesignSessionStatusPending
			target.LastError = "Failed to read workspace from OpenDesign, please retry"
			target.Reason = openDesignReasonWorkspaceReadFailed
		})
	}
	slog.Info("opendesign_device_auth.authorized", "workspace_resolved", true)
	return s.settlePoll(sessionID, sess, func(target *openDesignDeviceSession) {
		target.Status = openDesignSessionStatusAuthorized
		target.WorkspaceID = workspaceID
		target.LastError = ""
		target.Reason = ""
	})
}

type openDesignTokenPollResult struct {
	Status     string
	ControlKey string
	RuntimeKey string
	Email      string
}

// normalizeOpenDesignUpstreamStatus 归一化上游状态字面值（小写 + 去首尾空白）。
func normalizeOpenDesignUpstreamStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

// isOpenDesignAuthorizedStatus 判断上游状态是否代表「授权已批准、凭据已下发」。
// 上游实证值：approved（当前）；authorized 作为兼容值保留。
func isOpenDesignAuthorizedStatus(status string) bool {
	switch status {
	case openDesignUpstreamStatusApproved, openDesignSessionStatusAuthorized:
		return true
	default:
		return false
	}
}

func (s *OpenDesignDeviceAuthService) pollUpstreamToken(ctx context.Context, deviceID, deviceSecret string) (*openDesignTokenPollResult, error) {
	body, err := json.Marshal(map[string]string{"deviceSecret": deviceSecret})
	if err != nil {
		return nil, fmt.Errorf("marshal token poll request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/cli/device-authorizations/%s/token", s.apiBaseURL, deviceID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build token poll request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "Go-http-client/2.0")

	startedAt := time.Now()
	resp, err := s.httpClient.Do(httpReq)
	elapsed := time.Since(startedAt)
	if err != nil {
		slog.Error("opendesign_device_auth.poll_failed", "duration_ms", elapsed.Milliseconds(), "err", err)
		return nil, ErrOpenDesignUpstreamError
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	slog.Info("opendesign_device_auth.poll_response",
		"status", resp.StatusCode, "duration_ms", elapsed.Milliseconds(), "bytes", len(respBody))
	if readErr != nil {
		return nil, fmt.Errorf("read token poll response: %w", readErr)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, ErrOpenDesignUpstreamError
	}

	var payload struct {
		Status     string `json:"status"`
		ControlKey string `json:"controlKey"`
		RuntimeKey string `json:"runtimeKey"`
		User       *struct {
			Email string `json:"email"`
		} `json:"user"`
	}
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, fmt.Errorf("decode token poll response: %w", err)
	}
	// 上游状态字面值统一小写去空白，避免大小写/空格差异造成误判。
	status := normalizeOpenDesignUpstreamStatus(payload.Status)
	if isOpenDesignAuthorizedStatus(status) &&
		(strings.TrimSpace(payload.ControlKey) == "" || strings.TrimSpace(payload.RuntimeKey) == "") {
		// 已授权但缺凭据：视为上游异常，避免把空键写进账号。
		return nil, ErrOpenDesignUpstreamError
	}

	result := &openDesignTokenPollResult{
		Status:     status,
		ControlKey: payload.ControlKey,
		RuntimeKey: payload.RuntimeKey,
	}
	if payload.User != nil {
		result.Email = strings.TrimSpace(payload.User.Email)
	}
	return result, nil
}

func (s *OpenDesignDeviceAuthService) fetchWorkspaceID(ctx context.Context, controlKey string) (string, error) {
	if strings.TrimSpace(controlKey) == "" {
		return "", errors.New("empty control key")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, s.apiBaseURL+"/api/v1/wallet/balance", nil)
	if err != nil {
		return "", fmt.Errorf("build wallet request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+controlKey)
	httpReq.Header.Set("User-Agent", "Go-http-client/2.0")

	startedAt := time.Now()
	resp, err := s.httpClient.Do(httpReq)
	elapsed := time.Since(startedAt)
	if err != nil {
		slog.Error("opendesign_device_auth.wallet_failed", "duration_ms", elapsed.Milliseconds(), "err", err)
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	slog.Info("opendesign_device_auth.wallet_response",
		"status", resp.StatusCode, "duration_ms", elapsed.Milliseconds(), "bytes", len(respBody))
	if readErr != nil {
		return "", fmt.Errorf("read wallet response: %w", readErr)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("wallet balance returned status %d", resp.StatusCode)
	}

	var payload struct {
		WorkspaceID string `json:"workspaceId"`
	}
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return "", fmt.Errorf("decode wallet response: %w", err)
	}
	workspaceID := strings.TrimSpace(payload.WorkspaceID)
	if workspaceID == "" {
		return "", errors.New("workspaceId missing in balance response")
	}
	if !isValidOpenDesignWorkspaceID(workspaceID) {
		// 与探测侧同一份校验：workspaceId 会拼进探测 URL，必须剔除路径字符。
		return "", errors.New("workspaceId has unexpected format")
	}
	return workspaceID, nil
}

// CancelSession 取消会话并立即抹掉敏感值。
func (s *OpenDesignDeviceAuthService) CancelSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		return
	}
	clearOpenDesignSessionSecrets(sess)
	delete(s.sessions, sessionID)
}

// BeginSessionConsumption 校验会话状态并「预占」它（置 consumed），
// 返回本次落库需要的凭据。预占即防重放：并发第二次调用直接拿到
// OPENDESIGN_SESSION_CONSUMED。落库成功后调用 FinalizeSessionConsumption，
// 失败调用 AbortSessionConsumption 回滚到 authorized 允许重试（§3.2）。
func (s *OpenDesignDeviceAuthService) BeginSessionConsumption(sessionID string) (*OpenDesignSessionCredentials, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[sessionID]
	if !ok {
		return nil, ErrOpenDesignSessionNotFound
	}
	if time.Now().After(sess.ExpiresAt) {
		sess.Status = openDesignSessionStatusExpired
		clearOpenDesignSessionSecrets(sess)
		return nil, ErrOpenDesignSessionExpired
	}
	switch sess.Status {
	case openDesignSessionStatusConsumed:
		return nil, ErrOpenDesignSessionConsumed
	case openDesignSessionStatusAuthorized:
		// 允许继续
	default:
		return nil, ErrOpenDesignSessionNotAuthorized
	}
	if sess.ControlKey == "" || sess.RuntimeKey == "" || sess.WorkspaceID == "" {
		return nil, ErrOpenDesignSessionNotAuthorized.WithCause(errors.New("authorized session carries incomplete credentials"))
	}

	sess.Status = openDesignSessionStatusConsumed
	return &OpenDesignSessionCredentials{
		Email:       sess.Email,
		ControlKey:  sess.ControlKey,
		RuntimeKey:  sess.RuntimeKey,
		WorkspaceID: sess.WorkspaceID,
	}, nil
}

// FinalizeSessionConsumption 在账号成功落库后立即抹掉内存中的敏感值（§3.5 第 3 条）。
func (s *OpenDesignDeviceAuthService) FinalizeSessionConsumption(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		return
	}
	clearOpenDesignSessionSecrets(sess)
}

// AbortSessionConsumption 在账号落库失败时把会话回滚到 authorized，允许直接重试。
func (s *OpenDesignDeviceAuthService) AbortSessionConsumption(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sessionID]
	if !ok || sess.Status != openDesignSessionStatusConsumed {
		return
	}
	// 敏感值仍在（未 finalize）才能回滚；已 finalize 的会话敏感值为空，保持 consumed。
	if sess.ControlKey == "" || sess.RuntimeKey == "" || sess.WorkspaceID == "" {
		return
	}
	sess.Status = openDesignSessionStatusAuthorized
}

// isValidOpenDesignWorkspaceID constrains the workspace identifier carried in
// device-auth sessions: lowercase letters and digits only, 3-64 chars.
func isValidOpenDesignWorkspaceID(id string) bool {
	if len(id) < 3 || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
