package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type codexTicketSettingRepo struct {
	*codexPolicyMigrationRepoStub
	err error
}

func (r *codexTicketSettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	return r.codexPolicyMigrationRepoStub.GetValue(ctx, key)
}

func TestCodexTicketProxyRuntimeSettingAndFallback(t *testing.T) {
	repo := &codexTicketSettingRepo{codexPolicyMigrationRepoStub: &codexPolicyMigrationRepoStub{values: map[string]string{}}}
	settings := NewSettingService(repo, &config.Config{})
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{HarvestProxyURL: "http://fallback.example.com:8080"}, nil)
	svc.settingService = settings
	require.Equal(t, "http://fallback.example.com:8080", svc.openAICodexTicketHarvestProxyURL())
	repo.values[SettingKeyOpenAICodexTicketHarvestProxyURL] = "socks5h://user:secret@first.example.com:1080"
	settings.InvalidateOpenAICodexTicketHarvestProxyCache()
	require.Equal(t, repo.values[SettingKeyOpenAICodexTicketHarvestProxyURL], svc.openAICodexTicketHarvestProxyURL())
	repo.values[SettingKeyOpenAICodexTicketHarvestProxyURL] = "http://second.example.com:8080"
	settings.InvalidateOpenAICodexTicketHarvestProxyCache()
	require.Equal(t, "http://second.example.com:8080", svc.openAICodexTicketHarvestProxyURL())
	// Simulate another instance's settings write after the local cache expires.
	repo.values[SettingKeyOpenAICodexTicketHarvestProxyURL] = "https://third.example.com:443"
	settings.openAICodexTicketHarvestProxyCache.Store(&cachedOpenAICodexTicketHarvestProxy{value: "http://second.example.com:8080", expiresAt: time.Now().Add(-time.Second).UnixNano()})
	require.Equal(t, "https://third.example.com:443", svc.openAICodexTicketHarvestProxyURL())
	repo.err = errors.New("database unavailable")
	settings.openAICodexTicketHarvestProxyCache.Store(&cachedOpenAICodexTicketHarvestProxy{value: "https://third.example.com:443", expiresAt: 0})
	require.Equal(t, "https://third.example.com:443", svc.openAICodexTicketHarvestProxyURL())
}

func TestCodexTicketProxyMaskAndValidation(t *testing.T) {
	for _, raw := range []string{"http://user:secret@proxy.example.com:8080", "socks5h://user:secret@proxy.example.com:1080", "https://user:secret@[::1]:443"} {
		require.NoError(t, ValidateOpenAICodexTicketHarvestProxyURL(raw))
		masked := MaskProxyURL(raw)
		require.NotContains(t, masked, "secret")
		require.True(t, IsMaskedProxyURL(masked))
	}
	require.True(t, IsMaskedProxyURL(""))
	require.False(t, IsMaskedProxyURL("http://user:secret***suffix@proxy.example.com:8080"))
	for _, raw := range []string{"user:secret@host:1234", "http://user:secret@", "ftp://user:secret@host:1234", "http://user:secret@host:99999", "http://host:1234/?password=secret", "http://host:1234/#secret", "http://user:secret%zz@host:1234"} {
		err := ValidateOpenAICodexTicketHarvestProxyURL(raw)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
		require.Empty(t, MaskProxyURL(raw))
	}
}
