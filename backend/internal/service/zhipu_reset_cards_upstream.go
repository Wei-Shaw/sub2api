package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ZCode 设备授权及重置卡管理使用官方控制面，不能从推理 base_url 推导。
const (
	zhipuControlOrigin = "https://zcode.z.ai"
	zhipuDevicePath    = "/api/v1/oauth/cli"
	zhipuResetPath     = "/api/v1/coding-plan/reset"
)

var (
	zhipuFlowID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	zhipuToken  = regexp.MustCompile(`^[A-Za-z0-9._~-]{16,}$`)

	errZhipuResetUpstream = infraerrors.New(http.StatusBadGateway, "ZHIPU_RESET_UPSTREAM", "GLM reset-card service could not be read")
	errZhipuResetUnknown  = infraerrors.New(http.StatusBadGateway, "ZHIPU_RESET_OUTCOME_UNKNOWN", "The action result is unknown; refresh reset cards and history before trying again")
	errZhipuResetAuth     = infraerrors.Conflict("ZHIPU_RESET_AUTH_REQUIRED", "Authorize your personal GLM Coding Plan again")
	errZhipuDeviceExpired = infraerrors.NotFound("ZHIPU_DEVICE_EXPIRED", "GLM authorization expired; start again")
	errZhipuResetLimited  = infraerrors.TooManyRequests("ZHIPU_RESET_THROTTLED", "GLM reset-card service is rate limited; try again later")
	errZhipuResetRejected = infraerrors.Conflict("ZHIPU_RESET_REJECTED", "GLM did not accept the reset-card action; refresh current status")
)

type zhipuDeviceInit struct {
	FlowID       string `json:"flow_id"`
	AuthorizeURL string `json:"authorize_url"`
	ExpiresAt    int64  `json:"expires_at"`
	ServerTime   int64  `json:"server_time"`
	PollInterval int    `json:"poll_interval_sec"`
}

type zhipuDeviceResult struct {
	Status string `json:"status"`
	Token  string `json:"token"`
	User   struct {
		ID string `json:"user_id"`
	} `json:"user"`
	BigModel struct {
		AccessToken string `json:"access_token"`
		LegacyToken string `json:"accessToken"`
	} `json:"bigmodel"`
}

type zhipuResetSession struct {
	ZCodeToken    string `json:"zcode"`
	BigModelToken string `json:"bigmodel"`
}

func (a zhipuResetSession) valid() bool {
	return len(a.ZCodeToken) <= 8192 && len(a.BigModelToken) <= 8192 &&
		zhipuToken.MatchString(a.ZCodeToken) && zhipuToken.MatchString(a.BigModelToken)
}

func safeZhipuAuthorizeURL(raw string) bool {
	if len(raw) > 4096 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "bigmodel.cn" || u.User != nil ||
		u.EscapedPath() != "/login" || u.Fragment != "" {
		return false
	}
	// 官方登录入口自带回调。即使上游误配，也不向浏览器转交非官方回调。
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q["redirect"]) != 1 {
		return false
	}
	cb, err := url.Parse(q.Get("redirect"))
	return err == nil && cb.Scheme == "https" && cb.Host == "zcode.z.ai" &&
		cb.User == nil && cb.Fragment == "" && cb.EscapedPath() == zhipuDevicePath+"/callback/bigmodel"
}

func (s *ZhipuResetCardsService) call(ctx context.Context, accountID int64, method, path, pollToken string, session *zhipuResetSession, body any, action bool) (int, json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamPublicHostsOnly(ctx)), 15*time.Second)
	defer cancel()
	failure := errZhipuResetUpstream
	if action {
		failure = errZhipuResetUnknown
	}
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return 0, nil, ErrZhipuResetUnavailable
		}
	}
	target := zhipuControlOrigin + path
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, ErrZhipuResetUnavailable
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if session != nil {
		req.Header.Set("Authorization", "Bearer "+session.ZCodeToken)
		req.Header.Set("X-Bigmodel-Authorization", session.BigModelToken)
		req.Header.Set("Bigmodel-Target-Type", "PERSONAL")
	} else {
		req.Header.Set("Authorization", "Bearer "+pollToken)
	}

	resp, err := s.upstream.Do(req, "", accountID, 1)
	if err != nil || resp == nil || resp.Body == nil {
		return 0, nil, failure
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.Request != nil && (resp.Request.URL == nil || resp.Request.URL.String() != target) {
		return 0, nil, failure
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return 0, nil, errZhipuResetAuth
	case http.StatusTooManyRequests:
		return 0, nil, errZhipuResetLimited
	}
	const maxBody = 64 << 10
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	var envelope struct {
		Code *int            `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err != nil || len(raw) > maxBody || json.Unmarshal(raw, &envelope) != nil || envelope.Code == nil {
		return 0, nil, failure
	}
	if *envelope.Code == 3004 && strings.HasPrefix(path, zhipuDevicePath+"/poll/") {
		return 0, nil, errZhipuDeviceExpired
	}
	// 业务拒绝可使用 HTTP 4xx，3301 仍须保留官方给出的冷却时间。
	if path == zhipuResetPath+"/opportunity" && *envelope.Code == 3301 &&
		(resp.StatusCode == http.StatusOK || resp.StatusCode >= 400 && resp.StatusCode < 500) {
		return *envelope.Code, envelope.Data, nil
	}
	if action && *envelope.Code == 2007 {
		return 0, nil, errZhipuResetUnknown
	}
	if resp.StatusCode != http.StatusOK {
		if action && resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusRequestTimeout {
			return 0, nil, errZhipuResetRejected
		}
		return 0, nil, failure
	}
	return *envelope.Code, envelope.Data, nil
}
