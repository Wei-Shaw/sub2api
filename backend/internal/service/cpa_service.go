package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// CPA (CLIProxyAPI) 只读管理面板客户端。
//
// 本文件只实现 CPA /v0/management/* 的**只读**调用：GET 列表/日志类接口，以及
// POST /api-call（语义上也是读取上游额度，CPA 侧不会改变任何调度状态）。
// 任何写操作（reset-quota、auth-files 的 PATCH/DELETE、config 的 PUT 等）都不
// 暴露给面板，避免误操作影响正在服务的凭据。
const (
	cpaRequestTimeout    = 20 * time.Second
	cpaMaxResponseBytes  = 8 << 20
	cpaCodexUsageURL     = "https://chatgpt.com/backend-api/wham/usage"
	cpaCodexUserAgent    = "codex-tui/0.149.1 (Mac OS 26.5.2; arm64) iTerm.app/3.6.11 (codex-tui; 0.149.1)"
	cpaManagementTimeout = 20 * time.Second
)

var (
	// ErrCpaNotConfigured 面板未启用或缺少 base URL / management key。
	ErrCpaNotConfigured = infraerrors.BadRequest("CPA_NOT_CONFIGURED", "CPA management is not configured or disabled")
	// ErrCpaKeyInvalid CPA 拒绝 management key（401/403）。
	ErrCpaKeyInvalid = infraerrors.BadRequest("CPA_MANAGEMENT_KEY_INVALID", "CPA rejected the management key")
	// ErrCpaUnreachable 网络层不可达。
	ErrCpaUnreachable = infraerrors.ServiceUnavailable("CPA_UNREACHABLE", "CPA is unreachable")
	// ErrCpaUpstream CPA 返回了非 2xx。
	ErrCpaUpstream = infraerrors.ServiceUnavailable("CPA_UPSTREAM_ERROR", "CPA management API returned an error")
)

// CpaService 负责与 CPA management API 通信。
type CpaService struct {
	settings *SettingService
	client   *http.Client
}

// NewCpaService 构造 CPA 客户端。settings 用于读取面板配置。
func NewCpaService(settings *SettingService) *CpaService {
	return &CpaService{
		settings: settings,
		client: &http.Client{
			Timeout: cpaRequestTimeout,
		},
	}
}

// Config 返回当前 CPA 面板配置（含明文 key，仅后端使用）。
func (s *CpaService) Config(ctx context.Context) *CpaConfig {
	if s == nil || s.settings == nil {
		return &CpaConfig{BaseURL: DefaultCpaManagementBaseURL}
	}
	return s.settings.GetCpaConfig(ctx)
}

// CpaVersionInfo 汇总 CPA 可达性、版本与 key 是否被接受。
type CpaVersionInfo struct {
	Reachable     bool   `json:"reachable"`
	HealthOK      bool   `json:"health_ok"`
	Version       string `json:"version,omitempty"`
	Commit        string `json:"commit,omitempty"`
	BuildDate     string `json:"build_date,omitempty"`
	SupportPlugin string `json:"support_plugin,omitempty"`
	KeyAccepted   bool   `json:"key_accepted"`
	Message       string `json:"message,omitempty"`
}

type cpaResponse struct {
	Status int
	Header http.Header
	Body   []byte
}

// Version 探测 CPA：/healthz 判断可达性，/v0/management/auth-files 的响应头
// 即使在 401 时也带 X-CPA-Version，因此可以同时判断 key 是否有效。
func (s *CpaService) Version(ctx context.Context) (*CpaVersionInfo, error) {
	cfg := s.Config(ctx)
	if cfg == nil || strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, ErrCpaNotConfigured
	}
	info := &CpaVersionInfo{}

	health, errHealth := s.do(ctx, cfg, http.MethodGet, "/healthz", nil, nil)
	if errHealth != nil {
		info.Message = errHealth.Error()
		return info, nil
	}
	info.Reachable = true
	info.HealthOK = health.Status == http.StatusOK

	if strings.TrimSpace(cfg.ManagementKey) == "" {
		info.Message = "management key not configured"
		return info, nil
	}
	probe, errProbe := s.do(ctx, cfg, http.MethodGet, "/v0/management/auth-files", nil, nil)
	if errProbe != nil {
		info.Message = errProbe.Error()
		return info, nil
	}
	info.Version = probe.Header.Get("X-CPA-Version")
	info.Commit = probe.Header.Get("X-CPA-Commit")
	info.BuildDate = probe.Header.Get("X-CPA-Build-Date")
	info.SupportPlugin = probe.Header.Get("X-CPA-Support-Plugin")
	info.KeyAccepted = probe.Status >= 200 && probe.Status < 300
	if !info.KeyAccepted {
		info.Message = cpaErrorMessage(probe)
	}
	return info, nil
}

// ListAuthFiles 返回 CPA 的 auth file 列表（含额度观测信号与成功/失败计数）。
func (s *CpaService) ListAuthFiles(ctx context.Context) (json.RawMessage, error) {
	return s.getJSON(ctx, "/v0/management/auth-files", nil)
}

// APIKeyUsage 返回 CPA 侧按上游 API key 聚合的最近请求桶。
func (s *CpaService) APIKeyUsage(ctx context.Context) (json.RawMessage, error) {
	return s.getJSON(ctx, "/v0/management/api-key-usage", nil)
}

// RequestLogs 返回 CPA 的文件日志（需要 CPA 开启 logging-to-file）。
func (s *CpaService) RequestLogs(ctx context.Context, query url.Values) (json.RawMessage, error) {
	return s.getJSON(ctx, "/v0/management/logs", query)
}

// RequestErrorLogs 返回 CPA 错误响应日志文件清单。
func (s *CpaService) RequestErrorLogs(ctx context.Context) (json.RawMessage, error) {
	return s.getJSON(ctx, "/v0/management/request-error-logs", nil)
}

// DownloadRequestErrorLog 下载单个错误日志文件内容。
func (s *CpaService) DownloadRequestErrorLog(ctx context.Context, name string) ([]byte, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || strings.ContainsAny(trimmed, "/\\") {
		return nil, infraerrors.BadRequest("CPA_INVALID_LOG_NAME", "invalid log file name")
	}
	cfg, err := s.managementConfig(ctx)
	if err != nil {
		return nil, err
	}
	resp, errDo := s.do(ctx, cfg, http.MethodGet, "/v0/management/request-error-logs/"+url.PathEscape(trimmed), nil, nil)
	if errDo != nil {
		return nil, errDo
	}
	if err := cpaRequireSuccess(resp); err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// AuthFileQuota 通过 CPA 的 api-call 用指定凭据请求 Codex 用量接口，拿到真实
// 额度窗口（5h / 周 / code review / 附加限额 / plan / 重置积分）。
// 这是只读操作：只替换 $TOKEN$ 并发起一次上游 GET。
func (s *CpaService) AuthFileQuota(ctx context.Context, authIndex string) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(authIndex)
	if trimmed == "" {
		return nil, infraerrors.BadRequest("CPA_AUTH_INDEX_REQUIRED", "auth_index is required")
	}
	payload := map[string]any{
		"auth_index": trimmed,
		"method":     http.MethodGet,
		"url":        cpaCodexUsageURL,
		"header": map[string]string{
			"Authorization": "Bearer $TOKEN$",
			"Content-Type":  "application/json",
			"User-Agent":    cpaCodexUserAgent,
		},
	}
	raw, err := s.postJSON(ctx, "/v0/management/api-call", payload)
	if err != nil {
		return nil, err
	}
	// api-call 返回 {status_code, header, body}，body 是上游响应的字符串。
	var envelope struct {
		StatusCode int             `json:"status_code"`
		Body       json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return raw, nil
	}
	if envelope.StatusCode != 0 && (envelope.StatusCode < 200 || envelope.StatusCode >= 300) {
		return nil, infraerrors.New(
			http.StatusBadGateway,
			"CPA_UPSTREAM_USAGE_FAILED",
			fmt.Sprintf("CPA upstream usage probe returned %d", envelope.StatusCode),
		)
	}
	if len(envelope.Body) == 0 {
		return json.RawMessage(`{}`), nil
	}
	// body 可能是字符串包裹的 JSON，尝试解一层。
	var asString string
	if err := json.Unmarshal(envelope.Body, &asString); err == nil {
		if json.Valid([]byte(asString)) {
			return json.RawMessage(asString), nil
		}
		return json.RawMessage(`{}`), nil
	}
	if json.Valid(envelope.Body) {
		return envelope.Body, nil
	}
	return json.RawMessage(`{}`), nil
}

func (s *CpaService) managementConfig(ctx context.Context) (*CpaConfig, error) {
	cfg := s.Config(ctx)
	if cfg == nil || !cfg.Enabled {
		return nil, ErrCpaNotConfigured
	}
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.ManagementKey) == "" {
		return nil, ErrCpaNotConfigured
	}
	return cfg, nil
}

func (s *CpaService) getJSON(ctx context.Context, path string, query url.Values) (json.RawMessage, error) {
	cfg, err := s.managementConfig(ctx)
	if err != nil {
		return nil, err
	}
	resp, errDo := s.do(ctx, cfg, http.MethodGet, path, query, nil)
	if errDo != nil {
		return nil, errDo
	}
	if err := cpaRequireSuccess(resp); err != nil {
		return nil, err
	}
	if !json.Valid(resp.Body) {
		return nil, infraerrors.ServiceUnavailable("CPA_INVALID_RESPONSE", "CPA returned a non-JSON response")
	}
	return json.RawMessage(resp.Body), nil
}

func (s *CpaService) postJSON(ctx context.Context, path string, payload any) (json.RawMessage, error) {
	cfg, err := s.managementConfig(ctx)
	if err != nil {
		return nil, err
	}
	resp, errDo := s.do(ctx, cfg, http.MethodPost, path, nil, payload)
	if errDo != nil {
		return nil, errDo
	}
	if err := cpaRequireSuccess(resp); err != nil {
		return nil, err
	}
	if !json.Valid(resp.Body) {
		return nil, infraerrors.ServiceUnavailable("CPA_INVALID_RESPONSE", "CPA returned a non-JSON response")
	}
	return json.RawMessage(resp.Body), nil
}

// do 执行一次 CPA 管理请求。返回错误仅表示网络/配置层失败；HTTP 非 2xx 由调用方判断。
func (s *CpaService) do(
	ctx context.Context,
	cfg *CpaConfig,
	method string,
	path string,
	query url.Values,
	payload any,
) (*cpaResponse, error) {
	if s == nil || s.client == nil {
		return nil, ErrCpaNotConfigured
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		return nil, ErrCpaNotConfigured
	}
	requestURL := base + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	parsed, errParse := url.Parse(requestURL)
	if errParse != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, infraerrors.BadRequest("CPA_INVALID_BASE_URL", "invalid CPA base URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, infraerrors.BadRequest("CPA_INVALID_BASE_URL", "CPA base URL must be http or https")
	}

	var bodyReader io.Reader
	if payload != nil {
		encoded, errEncode := json.Marshal(payload)
		if errEncode != nil {
			return nil, infraerrors.InternalServer("CPA_ENCODE_FAILED", "failed to encode CPA request")
		}
		bodyReader = bytes.NewReader(encoded)
	}

	reqCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cpaManagementTimeout)
	defer cancel()
	req, errReq := http.NewRequestWithContext(reqCtx, method, requestURL, bodyReader)
	if errReq != nil {
		return nil, infraerrors.InternalServer("CPA_REQUEST_FAILED", "failed to build CPA request")
	}
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(cfg.ManagementKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.ManagementKey))
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, errDo := s.client.Do(req)
	if errDo != nil {
		if errors.Is(errDo, context.DeadlineExceeded) {
			return nil, infraerrors.GatewayTimeout("CPA_TIMEOUT", "CPA request timed out")
		}
		return nil, infraerrors.New(http.StatusServiceUnavailable, "CPA_UNREACHABLE",
			fmt.Sprintf("CPA is unreachable: %s", cpaRedactError(errDo, cfg.ManagementKey)))
	}
	defer func() { _ = resp.Body.Close() }()

	body, errRead := io.ReadAll(io.LimitReader(resp.Body, cpaMaxResponseBytes))
	if errRead != nil {
		return nil, infraerrors.ServiceUnavailable("CPA_READ_FAILED", "failed to read CPA response")
	}
	return &cpaResponse{Status: resp.StatusCode, Header: resp.Header, Body: body}, nil
}

func cpaRequireSuccess(resp *cpaResponse) error {
	if resp == nil {
		return ErrCpaUpstream
	}
	if resp.Status >= 200 && resp.Status < 300 {
		return nil
	}
	if resp.Status == http.StatusUnauthorized || resp.Status == http.StatusForbidden {
		return ErrCpaKeyInvalid
	}
	message := cpaErrorMessage(resp)
	if message == "" {
		message = fmt.Sprintf("CPA management API returned HTTP %d", resp.Status)
	} else {
		message = fmt.Sprintf("CPA management API returned HTTP %d: %s", resp.Status, message)
	}
	return infraerrors.New(http.StatusServiceUnavailable, "CPA_UPSTREAM_ERROR", message)
}

func cpaErrorMessage(resp *cpaResponse) string {
	if resp == nil || len(resp.Body) == 0 {
		return ""
	}
	var payload struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return ""
	}
	if payload.Message != "" {
		return payload.Message
	}
	if len(payload.Error) > 0 {
		var asString string
		if err := json.Unmarshal(payload.Error, &asString); err == nil {
			return asString
		}
		var asObject struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(payload.Error, &asObject); err == nil && asObject.Message != "" {
			return asObject.Message
		}
	}
	return ""
}

// cpaRedactError 保证 management key 不会出现在错误信息/日志里。
func cpaRedactError(err error, key string) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if trimmed := strings.TrimSpace(key); trimmed != "" {
		message = strings.ReplaceAll(message, trimmed, "***")
	}
	return message
}
