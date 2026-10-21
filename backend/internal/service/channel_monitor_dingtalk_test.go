//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type channelMonitorDingTalkSettingRepo struct {
	values         map[string]string
	getMultipleErr error
}

func (r *channelMonitorDingTalkSettingRepo) Get(_ context.Context, key string) (*Setting, error) {
	value, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}
func (r *channelMonitorDingTalkSettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	setting, err := r.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}
func (r *channelMonitorDingTalkSettingRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}
func (r *channelMonitorDingTalkSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	if r.getMultipleErr != nil {
		return nil, r.getMultipleErr
	}
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}
func (r *channelMonitorDingTalkSettingRepo) SetMultiple(_ context.Context, values map[string]string) error {
	for key, value := range values {
		r.values[key] = value
	}
	return nil
}
func (r *channelMonitorDingTalkSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	result := make(map[string]string, len(r.values))
	for key, value := range r.values {
		result[key] = value
	}
	return result, nil
}
func (r *channelMonitorDingTalkSettingRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

type channelMonitorRoundTripFunc func(*http.Request) (*http.Response, error)

func (f channelMonitorRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type channelMonitorFailingReadCloser struct{}

func (channelMonitorFailingReadCloser) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (channelMonitorFailingReadCloser) Close() error             { return nil }

func TestSignedDingTalkWebhook_ValidatesAndSigns(t *testing.T) {
	now := time.UnixMilli(1710000000123)
	signed, err := signedDingTalkWebhook(
		"https://oapi.dingtalk.com/robot/send?access_token=test-token",
		"SEC-test-secret",
		now,
	)
	if err != nil {
		t.Fatalf("signedDingTalkWebhook returned error: %v", err)
	}
	u, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("parse signed webhook: %v", err)
	}
	if got := u.Query().Get("timestamp"); got != "1710000000123" {
		t.Fatalf("unexpected timestamp: %q", got)
	}
	if u.Query().Get("sign") == "" {
		t.Fatal("expected non-empty signature")
	}
	if got := u.Query().Get("access_token"); got != "test-token" {
		t.Fatalf("access token changed: %q", got)
	}
}

func TestSignedDingTalkWebhook_AllowsUnsignedRobot(t *testing.T) {
	signed, err := signedDingTalkWebhook(
		"https://oapi.dingtalk.com/robot/send?access_token=test-token",
		"",
		time.Now(),
	)
	if err != nil {
		t.Fatalf("signedDingTalkWebhook returned error: %v", err)
	}
	u, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("parse webhook: %v", err)
	}
	if u.Query().Get("timestamp") != "" || u.Query().Get("sign") != "" {
		t.Fatalf("unsigned webhook unexpectedly contained signing parameters: %s", u.RawQuery)
	}
}

func TestSignedDingTalkWebhook_RejectsMalformedURL(t *testing.T) {
	if _, err := signedDingTalkWebhook("%", "", time.Now()); err == nil {
		t.Fatal("expected malformed webhook URL to be rejected")
	}
}

func TestValidateChannelMonitorDingTalkWebhook_RejectsUnsafeURLs(t *testing.T) {
	for _, webhook := range []string{
		"http://oapi.dingtalk.com/robot/send?access_token=x",
		"https://example.com/robot/send?access_token=x",
		"https://oapi.dingtalk.com/robot/send",
	} {
		if err := ValidateChannelMonitorDingTalkWebhook(webhook); err == nil {
			t.Fatalf("expected webhook to be rejected: %s", webhook)
		}
	}
}

func TestDingTalkChannelMonitorNotifier_SendsMarkdownAndChecksAPIResponse(t *testing.T) {
	repo := &channelMonitorDingTalkSettingRepo{values: map[string]string{
		SettingKeyChannelMonitorDingTalkEnabled: "true",
		SettingKeyChannelMonitorDingTalkWebhook: "https://oapi.dingtalk.com/robot/send?access_token=test-token",
		SettingKeyChannelMonitorDingTalkSecret:  "SEC-test-secret",
	}}
	notifier := &dingTalkChannelMonitorNotifier{settings: NewSettingService(repo, nil)}
	notifier.client = &http.Client{Transport: channelMonitorRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("timestamp") == "" || req.URL.Query().Get("sign") == "" {
			t.Fatal("signed query parameters are missing")
		}
		var payload struct {
			MsgType  string `json:"msgtype"`
			Markdown struct {
				Title string `json:"title"`
				Text  string `json:"text"`
			} `json:"markdown"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if payload.MsgType != "markdown" || payload.Markdown.Title != "渠道监控告警" {
			t.Fatalf("unexpected payload: %+v", payload)
		}
		if !strings.Contains(payload.Markdown.Text, "channel-a") || !strings.Contains(payload.Markdown.Text, "连续失败：3 次") {
			t.Fatalf("alert details missing: %s", payload.Markdown.Text)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"errcode":0,"errmsg":"ok"}`)),
			Header:     make(http.Header),
		}, nil
	})}

	sent, err := notifier.NotifyFailure(context.Background(), channelMonitorFailureAlert{
		MonitorID:           12,
		MonitorName:         "channel-a",
		Model:               "model-a",
		Status:              MonitorStatusFailed,
		Message:             "upstream unavailable",
		CheckedAt:           time.Now(),
		ConsecutiveFailures: 3,
	})
	if err != nil {
		t.Fatalf("NotifyFailure returned error: %v", err)
	}
	if !sent {
		t.Fatal("expected alert to be sent")
	}
}

func TestDingTalkChannelMonitorNotifier_FailsClosedWithoutUsableSettings(t *testing.T) {
	if notifier := newDingTalkChannelMonitorNotifier(nil); notifier != nil {
		t.Fatal("expected a nil notifier without a setting service")
	}

	for _, repo := range []*channelMonitorDingTalkSettingRepo{
		{values: map[string]string{}},
		{values: map[string]string{SettingKeyChannelMonitorDingTalkEnabled: "true"}},
		{values: map[string]string{}, getMultipleErr: errors.New("database unavailable")},
	} {
		notifier := &dingTalkChannelMonitorNotifier{
			settings: NewSettingService(repo, nil),
			client: &http.Client{Transport: channelMonitorRoundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("delivery should not be attempted")
				return nil, nil
			})},
		}
		sent, err := notifier.NotifyFailure(context.Background(), channelMonitorFailureAlert{})
		if err != nil || sent {
			t.Fatalf("expected notification to be skipped, got sent=%v err=%v", sent, err)
		}
	}
}

func TestNewDingTalkChannelMonitorNotifier_ConfiguresBoundedHTTPClient(t *testing.T) {
	repo := &channelMonitorDingTalkSettingRepo{values: map[string]string{}}
	notifier := newDingTalkChannelMonitorNotifier(NewSettingService(repo, nil))
	client := notifier.(*dingTalkChannelMonitorNotifier).client
	if client.Timeout != channelMonitorAlertTimeout {
		t.Fatalf("unexpected notifier timeout: %s", client.Timeout)
	}
	if client.CheckRedirect == nil {
		t.Fatal("expected notifier redirects to be disabled")
	}
}

func TestDingTalkChannelMonitorNotifier_RejectsInvalidConfigurationAndResponses(t *testing.T) {
	response := func(status int, body io.ReadCloser) *http.Response {
		return &http.Response{StatusCode: status, Body: body, Header: make(http.Header)}
	}
	tests := []struct {
		name      string
		webhook   string
		transport channelMonitorRoundTripFunc
	}{
		{name: "invalid stored webhook", webhook: "https://example.com/robot/send?access_token=x"},
		{name: "transport error", transport: func(*http.Request) (*http.Response, error) {
			return nil, errors.New("network down")
		}},
		{name: "body read error", transport: func(*http.Request) (*http.Response, error) {
			return response(http.StatusOK, channelMonitorFailingReadCloser{}), nil
		}},
		{name: "non success status", transport: func(*http.Request) (*http.Response, error) {
			return response(http.StatusBadGateway, io.NopCloser(strings.NewReader("bad gateway"))), nil
		}},
		{name: "malformed JSON", transport: func(*http.Request) (*http.Response, error) {
			return response(http.StatusOK, io.NopCloser(strings.NewReader("not-json"))), nil
		}},
		{name: "API rejection", transport: func(*http.Request) (*http.Response, error) {
			return response(http.StatusOK, io.NopCloser(strings.NewReader(`{"errcode":310000,"errmsg":"bad sign"}`))), nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := tt.webhook
			if webhook == "" {
				webhook = "https://oapi.dingtalk.com/robot/send?access_token=test-token"
			}
			repo := &channelMonitorDingTalkSettingRepo{values: map[string]string{
				SettingKeyChannelMonitorDingTalkEnabled: "true",
				SettingKeyChannelMonitorDingTalkWebhook: webhook,
			}}
			notifier := &dingTalkChannelMonitorNotifier{settings: NewSettingService(repo, nil)}
			if tt.transport != nil {
				notifier.client = &http.Client{Transport: tt.transport}
			} else {
				notifier.client = &http.Client{Transport: channelMonitorRoundTripFunc(func(*http.Request) (*http.Response, error) {
					t.Fatal("invalid webhook should fail before delivery")
					return nil, nil
				})}
			}
			sent, err := notifier.NotifyFailure(context.Background(), channelMonitorFailureAlert{})
			if err == nil || sent {
				t.Fatalf("expected notification failure, got sent=%v err=%v", sent, err)
			}
			if strings.Contains(err.Error(), "access_token") {
				t.Fatalf("error exposed webhook credentials: %v", err)
			}
		})
	}
}

func TestDingTalkFailurePayload_SanitizesAndTruncatesExternalText(t *testing.T) {
	payload := dingTalkFailurePayload(channelMonitorFailureAlert{
		MonitorName:         "channel[`name`]",
		Message:             strings.Repeat("错", channelMonitorAlertMessageRunes+20),
		ConsecutiveFailures: channelMonitorAlertThreshold,
	})
	markdown := payload["markdown"].(map[string]string)["text"]
	if strings.Contains(markdown, "[") || strings.Contains(markdown, "`") {
		t.Fatalf("markdown control characters were not sanitized: %s", markdown)
	}
	if !strings.Contains(markdown, "...") {
		t.Fatalf("long message was not truncated: %s", markdown)
	}
	fallback := dingTalkFailurePayload(channelMonitorFailureAlert{})["markdown"].(map[string]string)["text"]
	if !strings.Contains(fallback, "未返回错误详情") {
		t.Fatalf("missing empty-message fallback: %s", fallback)
	}
}

func TestDingTalkFailurePayload_TestMessageIsClearlyMarked(t *testing.T) {
	payload := dingTalkFailurePayload(channelMonitorFailureAlert{
		Message:   "configuration works",
		CheckedAt: time.Unix(1710000000, 0),
		IsTest:    true,
	})
	markdown := payload["markdown"].(map[string]string)
	if markdown["title"] != "渠道监控测试" {
		t.Fatalf("unexpected test title: %q", markdown["title"])
	}
	if !strings.Contains(markdown["text"], "配置有效") || strings.Contains(markdown["text"], "连续失败") {
		t.Fatalf("test message was ambiguous: %s", markdown["text"])
	}
}
