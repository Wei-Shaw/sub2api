package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudflare/cloudflare-go/v6/option"
	"github.com/stretchr/testify/require"
)

// cloudflareEmailSettingRepo 是只读内存配置仓库，供 Cloudflare 通道的用例使用。
type cloudflareEmailSettingRepo struct {
	values map[string]string
}

func (r *cloudflareEmailSettingRepo) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}

func (r *cloudflareEmailSettingRepo) GetAll(context.Context) (map[string]string, error) {
	result := make(map[string]string, len(r.values))
	for key, value := range r.values {
		result[key] = value
	}
	return result, nil
}

func (r *cloudflareEmailSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	return r.values[key], nil
}

func (r *cloudflareEmailSettingRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func (r *cloudflareEmailSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		result[key] = r.values[key]
	}
	return result, nil
}

func (r *cloudflareEmailSettingRepo) SetMultiple(_ context.Context, values map[string]string) error {
	for key, value := range values {
		r.values[key] = value
	}
	return nil
}

func (r *cloudflareEmailSettingRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func TestNormalizeEmailProvider(t *testing.T) {
	for input, want := range map[string]string{
		"":            EmailProviderSMTP,
		"   ":         EmailProviderSMTP,
		"smtp":        EmailProviderSMTP,
		" SMTP ":      EmailProviderSMTP,
		" CloudFlare": EmailProviderCloudflare,
	} {
		provider, err := NormalizeEmailProvider(input)
		require.NoError(t, err)
		require.Equalf(t, want, provider, "input %q", input)
	}

	_, err := NormalizeEmailProvider("resend")
	require.ErrorIs(t, err, ErrInvalidEmailProvider)
}

func TestEmailServiceGetEmailProviderDefaultsToSMTP(t *testing.T) {
	svc := NewEmailService(&cloudflareEmailSettingRepo{values: map[string]string{}}, nil)
	provider, err := svc.GetEmailProvider(context.Background())
	require.NoError(t, err)
	require.Equal(t, EmailProviderSMTP, provider)

	repo := &cloudflareEmailSettingRepo{values: map[string]string{SettingKeyEmailProvider: " CloudFlare "}}
	provider, err = NewEmailService(repo, nil).GetEmailProvider(context.Background())
	require.NoError(t, err)
	require.Equal(t, EmailProviderCloudflare, provider)
}

func TestEmailServiceGetCloudflareConfig(t *testing.T) {
	repo := &cloudflareEmailSettingRepo{values: map[string]string{
		SettingKeyCloudflareAPIToken:  " token ",
		SettingKeyCloudflareAccountID: " account ",
		SettingKeyCloudflareFromEmail: " welcome@example.com ",
		SettingKeyCloudflareFromName:  " Sub2API ",
	}}
	svc := NewEmailService(repo, nil)

	config, err := svc.GetCloudflareConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "token", config.APIToken)
	require.Equal(t, "account", config.AccountID)
	require.Equal(t, "welcome@example.com", config.FromEmail)
	require.Equal(t, "Sub2API", config.FromName)
}

func TestEmailServiceGetCloudflareConfigRequiresCredentials(t *testing.T) {
	repo := &cloudflareEmailSettingRepo{values: map[string]string{
		SettingKeyCloudflareFromEmail: "welcome@example.com",
	}}
	_, err := NewEmailService(repo, nil).GetCloudflareConfig(context.Background())
	require.ErrorIs(t, err, ErrEmailNotConfigured)
}

func TestEmailServiceSendCloudflareUsesSDKPayload(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"delivered":["recipient@example.com"],"queued":[],"permanent_bounces":[]}`))
	}))
	defer server.Close()

	svc := NewEmailService(nil, nil)
	err := svc.SendEmailWithCloudflareConfig(context.Background(), &CloudflareConfig{
		APIToken:  "cf-token",
		AccountID: "account-id",
		FromEmail: "welcome@example.com",
		FromName:  "Sub2API",
	}, "recipient@example.com", "Welcome", "<h1>Hello</h1><p>World</p>",
		option.WithBaseURL(server.URL))
	require.NoError(t, err)

	require.Equal(t, "/accounts/account-id/email/sending/send", gotPath)
	require.True(t, strings.Contains(gotAuth, "cf-token"))
	require.Equal(t, "Welcome", gotBody["subject"])
	require.Equal(t, "recipient@example.com", gotBody["to"])
	require.Equal(t, "<h1>Hello</h1><p>World</p>", gotBody["html"])
	require.Equal(t, "Hello\nWorld", gotBody["text"])
	from, ok := gotBody["from"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "welcome@example.com", from["address"])
	require.Equal(t, "Sub2API", from["name"])
}

func TestEmailServiceSendCloudflareWithoutFromNameUsesPlainAddress(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"delivered":["recipient@example.com"],"queued":[],"permanent_bounces":[]}`))
	}))
	defer server.Close()

	svc := NewEmailService(nil, nil)
	err := svc.SendEmailWithCloudflareConfig(context.Background(), &CloudflareConfig{
		APIToken:  "cf-token",
		AccountID: "account-id",
		FromEmail: "welcome@example.com",
	}, "recipient@example.com", "Welcome", "<p>World</p>",
		option.WithBaseURL(server.URL))
	require.NoError(t, err)
	require.Equal(t, "welcome@example.com", gotBody["from"])
}

func TestEmailServiceSendCloudflareRejectsHeaderInjection(t *testing.T) {
	svc := NewEmailService(nil, nil)
	config := &CloudflareConfig{
		APIToken:  "cf-token",
		AccountID: "account-id",
		FromEmail: "welcome@example.com",
	}

	err := svc.SendEmailWithCloudflareConfig(context.Background(), config, "victim@example.com\nBcc: attacker@example.com", "Subject", "body")
	require.ErrorContains(t, err, "line breaks")

	err = svc.SendEmailWithCloudflareConfig(context.Background(), config, "victim@example.com", "Subject\r\nBcc: attacker@example.com", "body")
	require.ErrorContains(t, err, "line breaks")
}

func TestEmailServiceSendCloudflareRequiresConfig(t *testing.T) {
	svc := NewEmailService(nil, nil)
	err := svc.SendEmailWithCloudflareConfig(context.Background(), nil, "recipient@example.com", "Subject", "body")
	require.ErrorIs(t, err, ErrEmailNotConfigured)

	err = svc.SendEmailWithCloudflareConfig(context.Background(), &CloudflareConfig{AccountID: "account-id"}, "recipient@example.com", "Subject", "body")
	require.ErrorIs(t, err, ErrEmailNotConfigured)
}

func TestSendEmailFallsBackToSMTPWhenProviderUnset(t *testing.T) {
	repo := &cloudflareEmailSettingRepo{values: map[string]string{}}
	svc := NewEmailService(repo, nil)

	// 未配置 smtp_host 时应报「未配置」，证明分发落到了 SMTP 分支而不是 Cloudflare。
	err := svc.SendEmail(context.Background(), "recipient@example.com", "Subject", "body")
	require.ErrorIs(t, err, ErrEmailNotConfigured)
}

func TestSendEmailRejectsUnknownProvider(t *testing.T) {
	repo := &cloudflareEmailSettingRepo{values: map[string]string{SettingKeyEmailProvider: "resend"}}
	svc := NewEmailService(repo, nil)

	err := svc.SendEmail(context.Background(), "recipient@example.com", "Subject", "body")
	require.ErrorIs(t, err, ErrInvalidEmailProvider)
}
