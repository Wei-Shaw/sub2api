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
)

const (
	opsWeComWebhookHost = "qyapi.weixin.qq.com"
	opsWeComWebhookPath = "/cgi-bin/webhook/send"
)

type OpsWeComNotifier interface {
	SendMarkdown(ctx context.Context, webhookURL, content string) error
}

type opsWeComHTTPNotifier struct {
	client *http.Client
}

func newOpsWeComHTTPNotifier() OpsWeComNotifier {
	return &opsWeComHTTPNotifier{
		client: &http.Client{
			Timeout: 8 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (n *opsWeComHTTPNotifier) SendMarkdown(ctx context.Context, webhookURL, content string) error {
	if err := validateOpsWeComWebhookURL(webhookURL); err != nil {
		return err
	}
	if n == nil || n.client == nil {
		return errors.New("wecom notifier not initialized")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return errors.New("wecom message is empty")
	}
	if len(content) > 4000 {
		content = content[:4000]
	}
	payload, err := json.Marshal(map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": content,
		},
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return errors.New("failed to build wecom request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return errors.New("failed to send wecom notification")
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return errors.New("failed to read wecom response")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("wecom returned HTTP %d", resp.StatusCode)
	}
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return errors.New("invalid wecom response")
	}
	if result.ErrCode != 0 {
		return fmt.Errorf("wecom returned error code %d", result.ErrCode)
	}
	return nil
}

func (s *OpsService) GetWeComNotificationConfig(ctx context.Context) (*OpsWeComNotificationConfig, error) {
	defaultCfg := defaultOpsWeComNotificationConfig()
	if s == nil || s.settingRepo == nil {
		return defaultCfg, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsWeComNotificationConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return defaultCfg, nil
		}
		return nil, err
	}
	cfg := &OpsWeComNotificationConfig{}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return defaultCfg, nil
	}
	normalizeOpsWeComNotificationConfig(cfg)
	return cfg, nil
}

func (s *OpsService) GetWeComNotificationConfigView(ctx context.Context) (*OpsWeComNotificationConfigView, error) {
	cfg, err := s.GetWeComNotificationConfig(ctx)
	if err != nil {
		return nil, err
	}
	return opsWeComNotificationConfigView(cfg), nil
}

func (s *OpsService) UpdateWeComNotificationConfig(ctx context.Context, req *OpsWeComNotificationConfigUpdateRequest) (*OpsWeComNotificationConfigView, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
	}
	if req == nil {
		return nil, errors.New("invalid request")
	}
	cfg, err := s.GetWeComNotificationConfig(ctx)
	if err != nil {
		return nil, err
	}
	cfg.Enabled = req.Enabled
	cfg.MinSeverity = strings.ToUpper(strings.TrimSpace(req.MinSeverity))
	cfg.RateLimitPerHour = req.RateLimitPerHour
	cfg.IncludeResolvedAlerts = req.IncludeResolvedAlerts
	if req.ClearWebhook {
		cfg.WebhookURL = ""
	} else if req.WebhookURL != nil {
		cfg.WebhookURL = strings.TrimSpace(*req.WebhookURL)
	}
	if err := validateOpsWeComNotificationConfig(cfg); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyOpsWeComNotificationConfig, string(encoded)); err != nil {
		return nil, err
	}
	return opsWeComNotificationConfigView(cfg), nil
}

func (s *OpsService) TestWeComNotification(ctx context.Context) error {
	if s == nil || s.weComNotifier == nil {
		return errors.New("wecom notifier not initialized")
	}
	cfg, err := s.GetWeComNotificationConfig(ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.WebhookURL) == "" {
		return errors.New("wecom webhook is not configured")
	}
	content := fmt.Sprintf("## Sub2API 企业微信通知测试\n> 配置验证成功\n> 时间：%s", time.Now().UTC().Format(time.RFC3339))
	return s.weComNotifier.SendMarkdown(ctx, cfg.WebhookURL, content)
}

func defaultOpsWeComNotificationConfig() *OpsWeComNotificationConfig {
	return &OpsWeComNotificationConfig{
		Enabled:               false,
		MinSeverity:           "",
		RateLimitPerHour:      0,
		IncludeResolvedAlerts: true,
	}
}

func normalizeOpsWeComNotificationConfig(cfg *OpsWeComNotificationConfig) {
	if cfg == nil {
		return
	}
	cfg.WebhookURL = strings.TrimSpace(cfg.WebhookURL)
	cfg.MinSeverity = strings.ToUpper(strings.TrimSpace(cfg.MinSeverity))
}

func validateOpsWeComNotificationConfig(cfg *OpsWeComNotificationConfig) error {
	if cfg == nil {
		return errors.New("invalid config")
	}
	if cfg.RateLimitPerHour < 0 {
		return errors.New("rate_limit_per_hour must be >= 0")
	}
	switch cfg.MinSeverity {
	case "", "P0", "P1", "P2", "P3":
	default:
		return errors.New("min_severity must be one of: P0, P1, P2, P3")
	}
	if cfg.WebhookURL != "" {
		if err := validateOpsWeComWebhookURL(cfg.WebhookURL); err != nil {
			return err
		}
	}
	if cfg.Enabled && cfg.WebhookURL == "" {
		return errors.New("webhook_url is required when enabled")
	}
	return nil
}

func validateOpsWeComWebhookURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return errors.New("invalid wecom webhook URL")
	}
	if parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), opsWeComWebhookHost) || parsed.Port() != "" {
		return errors.New("webhook_url must use the official WeCom HTTPS host")
	}
	if parsed.Path != opsWeComWebhookPath || parsed.User != nil || parsed.Fragment != "" || strings.TrimSpace(parsed.Query().Get("key")) == "" {
		return errors.New("invalid wecom webhook URL")
	}
	return nil
}

func opsWeComNotificationConfigView(cfg *OpsWeComNotificationConfig) *OpsWeComNotificationConfigView {
	if cfg == nil {
		cfg = defaultOpsWeComNotificationConfig()
	}
	return &OpsWeComNotificationConfigView{
		Enabled:               cfg.Enabled,
		WebhookConfigured:     strings.TrimSpace(cfg.WebhookURL) != "",
		WebhookURLMasked:      maskOpsWeComWebhookURL(cfg.WebhookURL),
		MinSeverity:           cfg.MinSeverity,
		RateLimitPerHour:      cfg.RateLimitPerHour,
		IncludeResolvedAlerts: cfg.IncludeResolvedAlerts,
	}
}

func maskOpsWeComWebhookURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Query().Get("key") == "" {
		return ""
	}
	key := parsed.Query().Get("key")
	suffix := key
	if len(suffix) > 4 {
		suffix = suffix[len(suffix)-4:]
	}
	return "https://" + opsWeComWebhookHost + opsWeComWebhookPath + "?key=****" + suffix
}
