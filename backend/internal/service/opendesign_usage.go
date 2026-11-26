package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// OpenDesignUsageExtraKey 仅由专用用量接口维护，不能通过通用账号接口导入或导出。
const OpenDesignUsageExtraKey = "opendesign_usage_binding"

const (
	openDesignUsageBase    = "https://amr-api.open-design.ai/api"
	openDesignUsageTimeout = 10 * time.Second
	openDesignUsageMaxBody = 64 << 10
	openDesignControlAuth  = "control_token"
)

var (
	openDesignWorkspaceID          = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
	openDesignControlToken         = regexp.MustCompile(`^vela_ctrl_[A-Za-z0-9._~-]{1,512}$`)
	ErrOpenDesignUsageUnavailable  = infraerrors.ServiceUnavailable("OPENDESIGN_USAGE_UNAVAILABLE", "OpenDesign usage is unavailable")
	errOpenDesignUsageAccount      = infraerrors.BadRequest("OPENDESIGN_USAGE_ACCOUNT_INVALID", "Select an OpenDesign API key account")
	errOpenDesignUsageBinding      = infraerrors.BadRequest("OPENDESIGN_USAGE_BINDING_INVALID", "Enter an OpenDesign Control Key, not the inference API key or account password")
	errOpenDesignUsageSession      = infraerrors.BadRequest("OPENDESIGN_USAGE_SESSION_REQUIRED", "Bind an OpenDesign workspace session first")
	errOpenDesignUsageUnauthorized = infraerrors.BadRequest("OPENDESIGN_USAGE_UNAUTHORIZED", "OpenDesign rejected the session or workspace")
	errOpenDesignUsageChanged      = infraerrors.Conflict("OPENDESIGN_USAGE_CHANGED", "Account or usage binding changed; reload and try again")
	errOpenDesignUsageBusy         = infraerrors.TooManyRequests("OPENDESIGN_USAGE_BUSY", "Too many OpenDesign usage requests")
	openDesignUSD                  = regexp.MustCompile(`^-?[0-9]{1,20}(\.[0-9]{1,8})?$`)
)

// OpenDesignUsageWindow 为官方滚动窗口，只输出剩余百分比及重置时间。
type OpenDesignUsageWindow struct {
	RemainingPercent float64    `json:"remaining_percent"`   // 剩余额度百分比，范围 0–100。
	ResetsAt         *time.Time `json:"resets_at,omitempty"` // 上游提供的下次重置时间。
}

// OpenDesignUsageSnapshot 是允许展示的额度快照，不包含上游凭据。
type OpenDesignUsageSnapshot struct {
	FetchedAt  time.Time              `json:"fetched_at"`            // 快照采集时间。
	FiveHour   *OpenDesignUsageWindow `json:"five_hour,omitempty"`   // 仅套餐实际包含时返回。
	SevenDay   *OpenDesignUsageWindow `json:"seven_day,omitempty"`   // 仅套餐实际包含时返回。
	BalanceUSD *string                `json:"balance_usd,omitempty"` // 美元金额，查询不可用时缺省。
}

// OpenDesignUsageState 表示已绑定的工作区、最近一次刷新状态和最后成功快照。
type OpenDesignUsageState struct {
	WorkspaceID string                   `json:"workspace_id"`       // 会话实际访问的工作区。
	Status      string                   `json:"status"`             // ok、unauthorized 或 failed
	Snapshot    *OpenDesignUsageSnapshot `json:"snapshot,omitempty"` // 最近一次成功读取。
}

type openDesignUsageBinding struct {
	OpenDesignUsageState
	Ciphertext string `json:"ciphertext"`
	Auth       string `json:"auth,omitempty"` // 空值兼容历史 Cookie 绑定。
	MemberID   string `json:"member_id,omitempty"`
}

type openDesignUsageRepository interface {
	GetByID(context.Context, int64) (*Account, error)
	CompareAndSwapOpenDesignUsage(context.Context, *Account, json.RawMessage) (bool, error)
}

// OpenDesignUsageService 管理用量授权和快照，不接收上游密码或参与调度。
type OpenDesignUsageService struct {
	repo      openDesignUsageRepository
	upstream  HTTPUpstream
	encryptor SecretEncryptor
	keyReady  bool
	slots     chan struct{}
	devices   openDesignDeviceStore
}

// NewOpenDesignUsageService 复用服务的稳定加密密钥和外部 HTTP 客户端。
func NewOpenDesignUsageService(repo AccountRepository, upstream HTTPUpstream, encryptor SecretEncryptor, cfg *config.Config) *OpenDesignUsageService {
	storage, _ := repo.(openDesignUsageRepository)
	return &OpenDesignUsageService{
		repo: storage, upstream: upstream, encryptor: encryptor,
		keyReady: cfg != nil && cfg.Totp.EncryptionKeyConfigured, slots: make(chan struct{}, 4),
	}
}

// IsOpenDesignUsageAccount 限定用量绑定只适用于独立的 OpenDesign Key 账号。
func IsOpenDesignUsageAccount(a *Account) bool {
	return a != nil && a.Platform == PlatformOpenDesign && a.Type == AccountTypeAPIKey && !a.IsShadow()
}

func openDesignBindingFromAccount(a *Account) *openDesignUsageBinding {
	if !IsOpenDesignUsageAccount(a) || a.Extra[OpenDesignUsageExtraKey] == nil {
		return nil
	}
	raw, err := json.Marshal(a.Extra[OpenDesignUsageExtraKey])
	var binding openDesignUsageBinding
	if err != nil || json.Unmarshal(raw, &binding) != nil ||
		!openDesignWorkspaceID.MatchString(binding.WorkspaceID) || binding.Ciphertext == "" {
		return nil
	}
	return &binding
}

// OpenDesignUsageStateFromAccount 返回可用于账号列表的脱敏快照。
func OpenDesignUsageStateFromAccount(a *Account) *OpenDesignUsageState {
	if binding := openDesignBindingFromAccount(a); binding != nil {
		return &binding.OpenDesignUsageState
	}
	return nil
}

func (s *OpenDesignUsageService) account(ctx context.Context, id int64) (*Account, error) {
	if s == nil || s.repo == nil {
		return nil, ErrOpenDesignUsageUnavailable
	}
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !IsOpenDesignUsageAccount(a) {
		return nil, errOpenDesignUsageAccount
	}
	return a, nil
}

// Bind 验证授权后才保存密文；Control Key 绑定时自动解析工作区。
func (s *OpenDesignUsageService) Bind(ctx context.Context, id int64, workspaceID, rawCookie, rawToken string) (*OpenDesignUsageState, error) {
	a, err := s.account(ctx, id)
	if err != nil {
		return nil, err
	}
	if !s.keyReady || s.encryptor == nil {
		return nil, ErrOpenDesignUsageUnavailable
	}
	workspaceID, rawToken = strings.TrimSpace(workspaceID), strings.TrimSpace(rawToken)
	binding := &openDesignUsageBinding{OpenDesignUsageState: OpenDesignUsageState{WorkspaceID: workspaceID, Status: "ok"}}
	var credential string
	if rawToken != "" {
		if workspaceID != "" || strings.TrimSpace(rawCookie) != "" || !openDesignControlToken.MatchString(rawToken) {
			return nil, errOpenDesignUsageBinding
		}
		credential, binding.Auth = rawToken, openDesignControlAuth
	} else {
		credential, err = normalizeOpenDesignUsageCookie(rawCookie)
		if err != nil || !validOpenDesignWorkspace(workspaceID) {
			return nil, errOpenDesignUsageBinding
		}
	}
	return s.verifyUsageBinding(ctx, a, binding, credential)
}

func (s *OpenDesignUsageService) verifyUsageBinding(ctx context.Context, a *Account, binding *openDesignUsageBinding, credential string) (*OpenDesignUsageState, error) {
	var err error
	binding.Snapshot, err = s.fetch(ctx, a.ID, binding, credential)
	if err != nil {
		return nil, err
	}
	binding.Ciphertext, err = s.encryptor.Encrypt(credential)
	if err != nil {
		return nil, ErrOpenDesignUsageUnavailable
	}
	return s.persist(ctx, a, binding)
}

// Refresh 读取已绑定的工作区；失败时保留最后一次成功快照并标记状态。
func (s *OpenDesignUsageService) Refresh(ctx context.Context, id int64) (*OpenDesignUsageState, error) {
	a, err := s.account(ctx, id)
	if err != nil {
		return nil, err
	}
	binding := openDesignBindingFromAccount(a)
	if binding == nil {
		return nil, errOpenDesignUsageSession
	}
	if !s.keyReady || s.encryptor == nil {
		return nil, ErrOpenDesignUsageUnavailable
	}
	credential, err := s.encryptor.Decrypt(binding.Ciphertext)
	if err != nil {
		return nil, ErrOpenDesignUsageUnavailable
	}
	switch binding.Auth {
	case openDesignControlAuth:
		if !openDesignControlToken.MatchString(credential) || !openDesignWorkspaceID.MatchString(binding.MemberID) {
			return nil, ErrOpenDesignUsageUnavailable
		}
	case "":
		credential, err = normalizeOpenDesignUsageCookie(credential)
		if err != nil {
			return nil, ErrOpenDesignUsageUnavailable
		}
	default:
		return nil, ErrOpenDesignUsageUnavailable
	}
	snapshot, err := s.fetch(ctx, a.ID, binding, credential)
	switch err {
	case nil:
		binding.Status, binding.Snapshot = "ok", snapshot
	case errOpenDesignUsageUnauthorized:
		binding.Status = "unauthorized"
	case errOpenDesignUsageBusy:
		return nil, err
	default:
		binding.Status = "failed"
	}
	return s.persist(ctx, a, binding)
}

// Unbind 从账号移除会话密文和用量快照，不改变推理 Key。
func (s *OpenDesignUsageService) Unbind(ctx context.Context, id int64) error {
	a, err := s.account(ctx, id)
	if err != nil || a.Extra[OpenDesignUsageExtraKey] == nil {
		return err
	}
	_, err = s.persist(ctx, a, nil)
	return err
}

func (s *OpenDesignUsageService) persist(ctx context.Context, a *Account, value *openDesignUsageBinding) (*OpenDesignUsageState, error) {
	var payload json.RawMessage
	if value != nil {
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, ErrOpenDesignUsageUnavailable
		}
		payload = raw
	}
	ok, err := s.repo.CompareAndSwapOpenDesignUsage(ctx, a, payload)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errOpenDesignUsageChanged
	}
	if value == nil {
		return nil, nil
	}
	return &value.OpenDesignUsageState, nil
}

func normalizeOpenDesignUsageCookie(raw string) (string, error) {
	if len(raw) > 8192 || strings.ContainsAny(raw, "\r\n") {
		return "", errOpenDesignUsageBinding
	}
	cookies, err := http.ParseCookie(strings.TrimSpace(raw))
	if err != nil || len(cookies) == 0 || len(cookies) > 32 {
		return "", errOpenDesignUsageBinding
	}
	seen := make(map[string]bool, len(cookies))
	values := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		name := strings.ToLower(cookie.Name)
		switch name {
		case "domain", "path", "expires", "max-age", "samesite", "secure", "httponly", "partitioned":
			return "", errOpenDesignUsageBinding
		}
		if seen[name] || cookie.Value == "" {
			return "", errOpenDesignUsageBinding
		}
		seen[name] = true
		values = append(values, cookie.String())
	}
	return strings.Join(values, "; "), nil
}

type openDesignPlanResponse struct {
	WorkspaceID string `json:"workspaceId"`
	Eligible    *bool  `json:"eligible"`
	Windows     []struct {
		DurationSeconds int        `json:"durationSeconds"`
		LimitCredits    string     `json:"limitCredits"`
		Remaining       string     `json:"remainingCredits"`
		ResetsAt        *time.Time `json:"resetsAt"`
	} `json:"windows"`
}

func validOpenDesignWorkspace(id string) bool {
	return id != "current" && openDesignWorkspaceID.MatchString(id)
}

func (s *OpenDesignUsageService) fetch(ctx context.Context, accountID int64, binding *openDesignUsageBinding, credential string) (*OpenDesignUsageSnapshot, error) {
	if s.upstream == nil {
		return nil, ErrOpenDesignUsageUnavailable
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return nil, errOpenDesignUsageBusy
	}
	ctx, cancel := context.WithTimeout(WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamPublicHostsOnly(ctx)), openDesignUsageTimeout)
	defer cancel()
	var plan openDesignPlanResponse
	var balance string
	if binding.Auth == openDesignControlAuth {
		if binding.WorkspaceID == "" {
			var current struct {
				WorkspaceID string `json:"workspaceId"`
				MemberID    string `json:"workspaceMemberId"`
			}
			if err := s.readUsage(ctx, accountID, "", "/current", binding.Auth, credential, &current); err != nil {
				return nil, err
			}
			if !validOpenDesignWorkspace(current.WorkspaceID) || !openDesignWorkspaceID.MatchString(current.MemberID) {
				return nil, ErrOpenDesignUsageUnavailable
			}
			binding.WorkspaceID, binding.MemberID = current.WorkspaceID, current.MemberID
		}
		var preflight struct {
			SchemaVersion int                    `json:"schemaVersion"`
			WorkspaceID   string                 `json:"workspaceId"`
			MemberID      string                 `json:"workspaceMemberId"`
			BalanceUSD    string                 `json:"balanceUsd"`
			CodingPlan    openDesignPlanResponse `json:"codingPlan"`
		}
		if err := s.readUsage(ctx, accountID, binding.WorkspaceID, "/billing/preflight", binding.Auth, credential, &preflight); err != nil {
			return nil, err
		}
		if preflight.SchemaVersion != 1 || preflight.WorkspaceID != binding.WorkspaceID || preflight.MemberID != binding.MemberID {
			return nil, ErrOpenDesignUsageUnavailable
		}
		plan, balance = preflight.CodingPlan, preflight.BalanceUSD
	} else if err := s.readUsage(ctx, accountID, binding.WorkspaceID, "/billing/coding-plan-usage", binding.Auth, credential, &plan); err != nil {
		return nil, err
	}
	if plan.WorkspaceID != binding.WorkspaceID || plan.Eligible == nil || len(plan.Windows) > 16 {
		return nil, ErrOpenDesignUsageUnavailable
	}
	out := &OpenDesignUsageSnapshot{FetchedAt: time.Now().UTC()}
	if *plan.Eligible {
		for _, window := range plan.Windows {
			var target **OpenDesignUsageWindow
			switch window.DurationSeconds {
			case 18000:
				target = &out.FiveHour
			case 604800:
				target = &out.SevenDay
			default:
				continue
			}
			pct, ok := openDesignRemainingPercent(window.Remaining, window.LimitCredits)
			if !ok || *target != nil {
				return nil, ErrOpenDesignUsageUnavailable
			}
			*target = &OpenDesignUsageWindow{RemainingPercent: pct, ResetsAt: window.ResetsAt}
		}
		if out.FiveHour == nil && out.SevenDay == nil {
			return nil, ErrOpenDesignUsageUnavailable
		}
	}
	var wallet struct {
		WorkspaceID string `json:"workspaceId"`
		BalanceUSD  string `json:"balanceUsd"`
	}
	// 窗口读取是主目标；余额不可用时不能伪造 $0，也不能丢掉真实窗口。
	if binding.Auth == "" {
		if err := s.readUsage(ctx, accountID, binding.WorkspaceID, "/wallet/balance", binding.Auth, credential, &wallet); err == nil &&
			wallet.WorkspaceID == binding.WorkspaceID {
			balance = wallet.BalanceUSD
		}
	}
	if openDesignUSD.MatchString(balance) {
		out.BalanceUSD = &balance
	}
	if out.FiveHour == nil && out.SevenDay == nil && out.BalanceUSD == nil {
		return nil, ErrOpenDesignUsageUnavailable
	}
	return out, nil
}

func (s *OpenDesignUsageService) readUsage(ctx context.Context, accountID int64, workspaceID, suffix, auth, credential string, out any) error {
	target := openDesignUsageBase + "/v1/workspaces"
	if workspaceID != "" {
		target += "/" + workspaceID
	}
	target += suffix
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return ErrOpenDesignUsageUnavailable
	}
	req.Header.Set("Accept", "application/json")
	if auth == openDesignControlAuth {
		req.Header.Set("Authorization", "Bearer "+credential)
	} else {
		req.Header.Set("Cookie", credential)
	}
	if workspaceID != "" {
		req.Header.Set("X-Vela-Workspace-Id", workspaceID)
	}
	resp, err := s.upstream.Do(req, "", accountID, 1)
	if err != nil || resp == nil || resp.Body == nil {
		return ErrOpenDesignUsageUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.Request != nil && (resp.Request.URL == nil || resp.Request.URL.String() != target) {
		return ErrOpenDesignUsageUnavailable
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return errOpenDesignUsageUnauthorized
	case http.StatusOK:
	default:
		return ErrOpenDesignUsageUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, openDesignUsageMaxBody+1))
	if err != nil || len(body) > openDesignUsageMaxBody || json.Unmarshal(body, out) != nil {
		return ErrOpenDesignUsageUnavailable
	}
	return nil
}

func openDesignRemainingPercent(remaining, limit string) (float64, bool) {
	r, rerr := strconv.ParseUint(remaining, 10, 64)
	l, lerr := strconv.ParseUint(limit, 10, 64)
	if rerr != nil || lerr != nil || l == 0 {
		return 0, false
	}
	return min(100, 100*float64(r)/float64(l)), true
}
