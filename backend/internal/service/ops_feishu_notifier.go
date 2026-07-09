package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const opsFeishuWebhookTimeout = 5 * time.Second

type opsFeishuWebhookPayload struct {
	Timestamp string                  `json:"timestamp,omitempty"`
	Sign      string                  `json:"sign,omitempty"`
	MsgType   string                  `json:"msg_type"`
	Content   opsFeishuWebhookContent `json:"content"`
}

type opsFeishuWebhookContent struct {
	Text string `json:"text"`
}

type opsFeishuWebhookResponse struct {
	Code          *int   `json:"code,omitempty"`
	Msg           string `json:"msg,omitempty"`
	StatusCode    *int   `json:"StatusCode,omitempty"`
	StatusMessage string `json:"StatusMessage,omitempty"`
}

func sendOpsAlertFeishuWebhook(ctx context.Context, cfg OpsFeishuAlertConfig, rule *OpsAlertRule, event *OpsAlertEvent) error {
	webhookURL := strings.TrimSpace(cfg.WebhookURL)
	if webhookURL == "" {
		return fmt.Errorf("feishu webhook URL is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	payload := opsFeishuWebhookPayload{
		MsgType: "text",
		Content: opsFeishuWebhookContent{
			Text: buildOpsAlertFeishuText(rule, event),
		},
	}

	if secret := strings.TrimSpace(cfg.Secret); secret != "" {
		ts := fmt.Sprintf("%d", time.Now().Unix())
		payload.Timestamp = ts
		payload.Sign = signFeishuWebhook(ts, secret)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	reqCtx, cancel := context.WithTimeout(ctx, opsFeishuWebhookTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("feishu webhook returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed opsFeishuWebhookResponse
	if len(respBody) > 0 && json.Unmarshal(respBody, &parsed) == nil {
		if parsed.Code != nil && *parsed.Code != 0 {
			return fmt.Errorf("feishu webhook returned code %d: %s", *parsed.Code, parsed.Msg)
		}
		if parsed.StatusCode != nil && *parsed.StatusCode != 0 {
			return fmt.Errorf("feishu webhook returned status code %d: %s", *parsed.StatusCode, parsed.StatusMessage)
		}
	}
	return nil
}

func signFeishuWebhook(timestamp, secret string) string {
	stringToSign := timestamp + "\n" + secret
	mac := hmac.New(sha256.New, []byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func buildOpsAlertFeishuText(rule *OpsAlertRule, event *OpsAlertEvent) string {
	var b strings.Builder
	b.WriteString("[Sub2API Ops Alert]\n")
	if rule != nil {
		b.WriteString(fmt.Sprintf("Rule: %s\n", strings.TrimSpace(rule.Name)))
		b.WriteString(fmt.Sprintf("Severity: %s\n", strings.TrimSpace(rule.Severity)))
		b.WriteString(fmt.Sprintf("Metric: %s %s %.2f\n", strings.TrimSpace(rule.MetricType), strings.TrimSpace(rule.Operator), rule.Threshold))
	}
	if event != nil {
		b.WriteString(fmt.Sprintf("Status: %s\n", strings.TrimSpace(event.Status)))
		if event.MetricValue != nil {
			b.WriteString(fmt.Sprintf("Value: %.2f\n", *event.MetricValue))
		}
		if strings.TrimSpace(event.Title) != "" {
			b.WriteString(fmt.Sprintf("Title: %s\n", strings.TrimSpace(event.Title)))
		}
		if strings.TrimSpace(event.Description) != "" {
			b.WriteString(fmt.Sprintf("Description: %s\n", strings.TrimSpace(event.Description)))
		}
		if !event.FiredAt.IsZero() {
			b.WriteString(fmt.Sprintf("Fired at: %s\n", event.FiredAt.Format(time.RFC3339)))
		}
	}
	return strings.TrimSpace(b.String())
}
