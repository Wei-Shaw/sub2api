package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	openDesignDevicePath    = "/v1/cli/device-authorizations"
	openDesignDeviceVersion = "0.1.10" // 已与官方 Vela 发起/轮询接口实测的协议版本。
	openDesignDeviceMax     = 32
	openDesignDeviceTTL     = 10 * time.Minute
	openDesignAttemptIDSize = 26 // crypto/rand.Text 的当前最小长度；同时限制异常请求。
)

var (
	openDesignDeviceUserCode = regexp.MustCompile(`^[A-Za-z0-9-]{4,16}$`)
	errOpenDesignDeviceGone  = infraerrors.NotFound("OPENDESIGN_DEVICE_EXPIRED", "OpenDesign authorization expired; start again")
)

// OpenDesignDevicePrompt 只返回管理员完成浏览器授权所需的信息。
type OpenDesignDevicePrompt struct {
	AttemptID    string    `json:"attempt_id"`    // Sub2API 本次授权的标识，不是上游设备密钥。
	ActivateURL  string    `json:"activate_url"`  // 固定在 OpenDesign 官方页面的授权链接。
	UserCode     string    `json:"user_code"`     // 供用户在官方页对照的验证码。
	ExpiresAt    time.Time `json:"expires_at"`    // 本次授权失效时间。
	PollInterval int       `json:"poll_interval"` // 客户端轮询间隔（秒）。
}

// OpenDesignDeviceStatus 表示轮询阶段；成功仅返回脱敏额度状态。
type OpenDesignDeviceStatus struct {
	Status string                `json:"status"`          // pending、bound、expired 或 rejected。
	Usage  *OpenDesignUsageState `json:"usage,omitempty"` // 绑定成功后的额度快照。
}

type openDesignDeviceAttempt struct {
	prompt       OpenDesignDevicePrompt
	actor        int64
	deviceID     string
	deviceSecret string
	accountStamp [sha256.Size]byte
	nextPoll     time.Time
	busy         bool
}

type openDesignDeviceStore struct {
	mu       sync.Mutex
	attempts map[int64]*openDesignDeviceAttempt
}

// StartDeviceAuthorization 发起官方设备授权，不保存密码；同一账号复用未过期的请求。
func (s *OpenDesignUsageService) StartDeviceAuthorization(ctx context.Context, id, actor int64) (*OpenDesignDevicePrompt, error) {
	a, err := s.account(ctx, id)
	if err != nil {
		return nil, err
	}
	if actor <= 0 || !s.keyReady || s.encryptor == nil || s.upstream == nil {
		return nil, ErrOpenDesignUsageUnavailable
	}
	stamp, err := openDesignDeviceAccountStamp(a)
	if err != nil {
		return nil, ErrOpenDesignUsageUnavailable
	}
	now := time.Now().UTC()
	d := &s.devices
	d.mu.Lock()
	for id, attempt := range d.attempts {
		if !attempt.busy && !now.Before(attempt.prompt.ExpiresAt) {
			delete(d.attempts, id)
		}
	}
	if previous := d.attempts[id]; previous != nil && !previous.busy && previous.accountStamp != stamp {
		delete(d.attempts, id)
	}
	if pending := d.attempts[id]; pending != nil {
		if pending.actor != actor || pending.accountStamp != stamp || pending.prompt.ActivateURL == "" {
			d.mu.Unlock()
			return nil, errOpenDesignUsageChanged
		}
		prompt := pending.prompt
		d.mu.Unlock()
		return &prompt, nil
	}
	if len(d.attempts) >= openDesignDeviceMax {
		d.mu.Unlock()
		return nil, errOpenDesignUsageBusy
	}
	if d.attempts == nil {
		d.attempts = make(map[int64]*openDesignDeviceAttempt)
	}
	attempt := &openDesignDeviceAttempt{actor: actor, accountStamp: stamp, busy: true}
	d.attempts[id] = attempt
	d.mu.Unlock()

	var upstream struct {
		DeviceID     string    `json:"deviceId"`
		DeviceSecret string    `json:"deviceSecret"`
		UserCode     string    `json:"userCode"`
		ActivateURL  string    `json:"activationUrl"`
		ExpiresAt    time.Time `json:"expiresAt"`
		PollInterval int       `json:"pollIntervalSeconds"`
	}
	payload := struct {
		Version string `json:"cliVersion"`
		Host    string `json:"host"`
		Profile string `json:"profile"`
	}{openDesignDeviceVersion, "Sub2API", "prod"}
	err = s.openDesignDeviceRequest(ctx, id, openDesignDevicePath, payload, &upstream, http.StatusCreated)
	activate, valid := openDesignActivationURL(upstream.ActivateURL, upstream.DeviceID, upstream.UserCode)
	now = time.Now().UTC()
	d.mu.Lock()
	defer d.mu.Unlock()
	if err != nil || !valid || len(upstream.DeviceSecret) == 0 || len(upstream.DeviceSecret) > 1024 ||
		!upstream.ExpiresAt.After(now) || upstream.PollInterval < 1 || upstream.PollInterval > 60 {
		delete(d.attempts, id)
		if err != nil {
			return nil, err
		}
		return nil, ErrOpenDesignUsageUnavailable
	}
	expires := minTime(upstream.ExpiresAt, now.Add(openDesignDeviceTTL))
	attempt.prompt = OpenDesignDevicePrompt{rand.Text(), activate, upstream.UserCode, expires, max(2, upstream.PollInterval)}
	attempt.deviceID, attempt.deviceSecret = upstream.DeviceID, upstream.DeviceSecret
	attempt.nextPoll = now.Add(time.Duration(attempt.prompt.PollInterval) * time.Second)
	attempt.busy = false
	prompt := attempt.prompt
	return &prompt, nil
}

// PollDeviceAuthorization 交换已批准的授权；作用域固定为发起人和目标账号。
func (s *OpenDesignUsageService) PollDeviceAuthorization(ctx context.Context, id, actor int64, attemptID string) (*OpenDesignDeviceStatus, error) {
	if s == nil || actor <= 0 || len(attemptID) < openDesignAttemptIDSize || len(attemptID) > 128 {
		return nil, errOpenDesignDeviceGone
	}
	d := &s.devices
	now := time.Now().UTC()
	d.mu.Lock()
	attempt := d.attempts[id]
	if attempt == nil || attempt.actor != actor || attempt.prompt.AttemptID != attemptID {
		d.mu.Unlock()
		return nil, errOpenDesignDeviceGone
	}
	if !now.Before(attempt.prompt.ExpiresAt) {
		if !attempt.busy {
			delete(d.attempts, id)
		}
		d.mu.Unlock()
		return &OpenDesignDeviceStatus{Status: "expired"}, nil
	}
	if attempt.busy || now.Before(attempt.nextPoll) {
		d.mu.Unlock()
		return &OpenDesignDeviceStatus{Status: "pending"}, nil
	}
	attempt.busy = true
	d.mu.Unlock()
	done := false
	defer func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		if done {
			delete(d.attempts, id)
		} else {
			attempt.busy = false
			attempt.nextPoll = time.Now().Add(time.Duration(attempt.prompt.PollInterval) * time.Second)
		}
	}()

	a, err := s.account(ctx, id)
	if err != nil {
		return nil, err
	}
	stamp, err := openDesignDeviceAccountStamp(a)
	if err != nil || stamp != attempt.accountStamp {
		done = true
		return nil, errOpenDesignUsageChanged
	}
	// 仅解析计费所需的 Control Key，不接收、保存或覆盖推理 Key。
	var upstream struct {
		DeviceID   string `json:"deviceId"`
		Status     string `json:"status"`
		ControlKey string `json:"controlKey"`
	}
	payload := struct {
		DeviceSecret string `json:"deviceSecret"`
	}{attempt.deviceSecret}
	if err := s.openDesignDeviceRequest(ctx, id, openDesignDevicePath+"/"+attempt.deviceID+"/token", payload, &upstream, http.StatusOK); err != nil {
		if err == errOpenDesignDeviceGone {
			done = true
			return &OpenDesignDeviceStatus{Status: "expired"}, nil
		}
		return nil, err
	}
	if upstream.DeviceID != attempt.deviceID {
		done = true
		return nil, ErrOpenDesignUsageUnavailable
	}
	switch upstream.Status {
	case "pending":
		return &OpenDesignDeviceStatus{Status: "pending"}, nil
	case "expired":
		done = true
		return &OpenDesignDeviceStatus{Status: "expired"}, nil
	case "approved":
		done = true
		if !time.Now().Before(attempt.prompt.ExpiresAt) || !openDesignControlToken.MatchString(upstream.ControlKey) {
			return nil, ErrOpenDesignUsageUnavailable
		}
		binding := &openDesignUsageBinding{OpenDesignUsageState: OpenDesignUsageState{Status: "ok"}, Auth: openDesignControlAuth}
		state, err := s.verifyUsageBinding(ctx, a, binding, upstream.ControlKey)
		if err != nil {
			return nil, err
		}
		return &OpenDesignDeviceStatus{Status: "bound", Usage: state}, nil
	default:
		done = true
		return &OpenDesignDeviceStatus{Status: "rejected"}, nil
	}
}

func openDesignActivationURL(raw, id, code string) (string, bool) {
	if !openDesignWorkspaceID.MatchString(id) || !openDesignDeviceUserCode.MatchString(code) || len(raw) > 2048 {
		return "", false
	}
	link, err := url.Parse(raw)
	if err != nil || link.Scheme != "https" || link.Host != "open-design.ai" ||
		link.EscapedPath() != "/cloud/cli/activate" || link.User != nil {
		return "", false
	}
	params, err := url.ParseQuery(link.RawQuery)
	if err != nil || params.Get("deviceId") != id || params.Get("userCode") != code {
		return "", false
	}
	// 仅把设备 ID 和可见验证码交给浏览器，其余上游参数不透传。
	link.RawQuery = url.Values{"deviceId": {id}, "userCode": {code}}.Encode()
	link.Fragment = ""
	return link.String(), true
}

func openDesignDeviceAccountStamp(a *Account) ([sha256.Size]byte, error) {
	// 用量刷新只改变快照，不会取消待批准的授权；换 Key、换绑和解绑则会取消。
	binding := openDesignBindingFromAccount(a)
	var cipher string
	if binding != nil {
		cipher = binding.Ciphertext
	}
	b, err := json.Marshal([]any{a.Credentials, a.ProxyID, cipher})
	return sha256.Sum256(b), err
}

func (s *OpenDesignUsageService) openDesignDeviceRequest(ctx context.Context, id int64, path string, body, result any, expected int) error {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return errOpenDesignUsageBusy
	}
	ctx, cancel := context.WithTimeout(WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamPublicHostsOnly(ctx)), openDesignUsageTimeout)
	defer cancel()
	payload, err := json.Marshal(body)
	if err != nil {
		return ErrOpenDesignUsageUnavailable
	}
	target := openDesignUsageBase + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return ErrOpenDesignUsageUnavailable
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.upstream.Do(req, "", id, 1)
	if err != nil || resp == nil || resp.Body == nil {
		return ErrOpenDesignUsageUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.Request != nil && (resp.Request.URL == nil || resp.Request.URL.String() != target) {
		return ErrOpenDesignUsageUnavailable
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		return errOpenDesignDeviceGone
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return errOpenDesignUsageBusy
	}
	if resp.StatusCode != expected {
		return ErrOpenDesignUsageUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, openDesignUsageMaxBody+1))
	if err != nil || len(data) > openDesignUsageMaxBody || json.Unmarshal(data, result) != nil {
		return ErrOpenDesignUsageUnavailable
	}
	return nil
}
