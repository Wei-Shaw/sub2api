package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type emailProviderSettingRepoStub struct {
	values map[string]string
}

func (s *emailProviderSettingRepoStub) Get(context.Context, string) (*Setting, error) {
	return nil, errors.New("unexpected Get call")
}

func (s *emailProviderSettingRepoStub) GetValue(context.Context, string) (string, error) {
	return "", errors.New("unexpected GetValue call")
}

func (s *emailProviderSettingRepoStub) Set(context.Context, string, string) error {
	return errors.New("unexpected Set call")
}

func (s *emailProviderSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (s *emailProviderSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	return errors.New("unexpected SetMultiple call")
}

func (s *emailProviderSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return nil, errors.New("unexpected GetAll call")
}

func (s *emailProviderSettingRepoStub) Delete(context.Context, string) error {
	return errors.New("unexpected Delete call")
}

func TestNormalizeEmailProviderDefaultsToSMTP(t *testing.T) {
	provider, err := NormalizeEmailProvider("")
	require.NoError(t, err)
	require.Equal(t, EmailProviderSMTP, provider)
}

func TestNormalizeResendAPIBaseURL(t *testing.T) {
	baseURL, err := NormalizeResendAPIBaseURL("")
	require.NoError(t, err)
	require.Equal(t, defaultResendAPIBaseURL, baseURL)

	baseURL, err = NormalizeResendAPIBaseURL("https://api.resend.com/")
	require.NoError(t, err)
	require.Equal(t, "https://api.resend.com", baseURL)

	_, err = NormalizeResendAPIBaseURL("http://api.resend.com")
	require.Error(t, err)
}

func TestSendEmailWithResendConfigPostsExpectedPayload(t *testing.T) {
	var receivedAuth string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/emails", r.URL.Path)
		receivedAuth = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	svc := NewEmailService(&emailProviderSettingRepoStub{}, nil)
	err := svc.SendEmailWithResendConfig(context.Background(), &ResendConfig{
		APIKey:     "resend-token",
		FromEmail:  "noreply@example.com",
		FromName:   "Sub2API",
		APIBaseURL: server.URL,
	}, "user@example.com", "Hello", "<p>Hi</p>")
	require.NoError(t, err)
	require.Equal(t, "Bearer resend-token", receivedAuth)
	require.Equal(t, "Sub2API <noreply@example.com>", payload["from"])
	require.Equal(t, "Hello", payload["subject"])
	require.Equal(t, "<p>Hi</p>", payload["html"])
	require.Equal(t, []any{"user@example.com"}, payload["to"])
}

func TestCloudflareEmailConfigRequiresCredentials(t *testing.T) {
	svc := NewEmailService(&emailProviderSettingRepoStub{values: map[string]string{
		SettingKeyCloudflareAPIToken:  "token",
		SettingKeyCloudflareAccountID: "",
		SettingKeyCloudflareFromEmail: "noreply@example.com",
	}}, nil)

	_, err := svc.GetCloudflareEmailConfig(context.Background())
	require.ErrorIs(t, err, ErrEmailNotConfigured)
}

func TestPostEmailJSONRedactsAuthorizationFromError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad token"}`))
	}))
	defer server.Close()

	err := postEmailJSON(context.Background(), server.URL, "Bearer secret-token", map[string]string{"hello": "world"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "status 401")
	require.False(t, strings.Contains(err.Error(), "secret-token"))
}
