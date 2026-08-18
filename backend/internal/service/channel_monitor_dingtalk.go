package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	channelMonitorAlertThreshold    = 3
	channelMonitorAlertTimeout      = 8 * time.Second
	channelMonitorAlertMaxBody      = 64 * 1024
	channelMonitorAlertMessageRunes = 200
)

type channelMonitorFailureAlert struct {
	MonitorID           int64
	MonitorName         string
	Model               string
	Status              string
	Message             string
	CheckedAt           time.Time
	ConsecutiveFailures int
	IsTest              bool
}

type channelMonitorAlertNotifier interface {
	NotifyFailure(ctx context.Context, alert channelMonitorFailureAlert) (bool, error)
}

type dingTalkChannelMonitorNotifier struct {
	settings *SettingService
	client   *http.Client
}

func newDingTalkChannelMonitorNotifier(settings *SettingService) channelMonitorAlertNotifier {
	if settings == nil {
		return nil
	}
	return &dingTalkChannelMonitorNotifier{
		settings: settings,
		client:   newDingTalkHTTPClient(),
	}
}

func (n *dingTalkChannelMonitorNotifier) NotifyFailure(ctx context.Context, alert channelMonitorFailureAlert) (bool, error) {
	cfg := n.settings.GetChannelMonitorDingTalkRuntime(ctx)
	return n.notifyFailureWithConfig(ctx, cfg, alert)
}

func (n *dingTalkChannelMonitorNotifier) notifyFailureWithConfig(ctx context.Context, cfg ChannelMonitorDingTalkRuntime, alert channelMonitorFailureAlert) (bool, error) {
	if !cfg.Enabled || cfg.Webhook == "" {
		return false, nil
	}
	webhook, err := signedDingTalkWebhook(cfg.Webhook, cfg.Secret, time.Now())
	if err != nil {
		return false, err
	}
	payload := dingTalkFailurePayload(alert)
	body, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("marshal dingtalk alert: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook, bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("create dingtalk alert request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		if urlErr, ok := err.(*url.Error); ok {
			err = urlErr.Err
		}
		return false, fmt.Errorf("send dingtalk alert: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, channelMonitorAlertMaxBody))
	if err != nil {
		return false, fmt.Errorf("read dingtalk alert response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return false, fmt.Errorf("dingtalk alert returned HTTP %d", resp.StatusCode)
	}
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return false, fmt.Errorf("decode dingtalk alert response: %w", err)
	}
	if result.ErrCode != 0 {
		return false, fmt.Errorf("dingtalk alert rejected: code=%d message=%s", result.ErrCode, result.ErrMsg)
	}
	return true, nil
}

func newDingTalkHTTPClient() *http.Client {
	return &http.Client{
		Timeout: channelMonitorAlertTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// SendChannelMonitorDingTalkTest sends a clearly marked test message without
// reading or changing the persisted failure episode state.
func SendChannelMonitorDingTalkTest(ctx context.Context, webhook, secret string) error {
	notifier := &dingTalkChannelMonitorNotifier{client: newDingTalkHTTPClient()}
	sent, err := notifier.notifyFailureWithConfig(ctx, ChannelMonitorDingTalkRuntime{
		Enabled: true,
		Webhook: strings.TrimSpace(webhook),
		Secret:  strings.TrimSpace(secret),
	}, channelMonitorFailureAlert{
		Message:   "这是一条测试消息，当前填写的 Webhook 和加签密钥可正常发送。",
		CheckedAt: time.Now(),
		IsTest:    true,
	})
	if err != nil {
		return err
	}
	if !sent {
		return errors.New("dingtalk webhook is required")
	}
	return nil
}

func signedDingTalkWebhook(rawWebhook, secret string, now time.Time) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawWebhook))
	if err != nil {
		return "", errors.New("invalid dingtalk webhook URL")
	}
	if u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "oapi.dingtalk.com") || u.Path != "/robot/send" {
		return "", errors.New("dingtalk webhook must use https://oapi.dingtalk.com/robot/send")
	}
	if strings.TrimSpace(u.Query().Get("access_token")) == "" {
		return "", errors.New("dingtalk webhook is missing access_token")
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return u.String(), nil
	}
	timestamp := strconv.FormatInt(now.UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "\n" + secret))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	query := u.Query()
	query.Set("timestamp", timestamp)
	query.Set("sign", signature)
	u.RawQuery = query.Encode()
	return u.String(), nil
}

// ValidateChannelMonitorDingTalkWebhook validates a custom robot URL without
// exposing its access token in an error response.
func ValidateChannelMonitorDingTalkWebhook(rawWebhook string) error {
	_, err := signedDingTalkWebhook(rawWebhook, "", time.Now())
	return err
}

func dingTalkFailurePayload(alert channelMonitorFailureAlert) map[string]any {
	if alert.IsTest {
		checkedAt := alert.CheckedAt
		if checkedAt.IsZero() {
			checkedAt = time.Now()
		}
		message := sanitizeDingTalkText(alert.Message, channelMonitorAlertMessageRunes)
		text := fmt.Sprintf(
			"### 渠道监控测试\n\n- 状态：配置有效\n- 测试时间：%s\n- 说明：%s",
			checkedAt.Format("2006-01-02 15:04:05 MST"), message,
		)
		return map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]string{
				"title": "渠道监控测试",
				"text":  text,
			},
		}
	}

	name := sanitizeDingTalkText(alert.MonitorName, 100)
	model := sanitizeDingTalkText(alert.Model, 100)
	status := sanitizeDingTalkText(alert.Status, 30)
	message := sanitizeDingTalkText(alert.Message, channelMonitorAlertMessageRunes)
	if message == "" {
		message = "未返回错误详情"
	}
	checkedAt := alert.CheckedAt
	if checkedAt.IsZero() {
		checkedAt = time.Now()
	}
	text := fmt.Sprintf(
		"### 渠道监控告警\n\n- 渠道：%s（ID: %d）\n- 主模型：%s\n- 状态：%s\n- 连续失败：%d 次\n- 检测时间：%s\n- 错误：%s",
		name, alert.MonitorID, model, status, alert.ConsecutiveFailures,
		checkedAt.Format("2006-01-02 15:04:05 MST"), message,
	)
	return map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": "渠道监控告警",
			"text":  text,
		},
	}
}

func sanitizeDingTalkText(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	value = strings.NewReplacer("[", "(", "]", ")", "`", "'").Replace(value)
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes]) + "..."
	}
	return value
}
