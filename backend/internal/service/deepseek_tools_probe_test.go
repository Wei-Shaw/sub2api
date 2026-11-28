package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type deepseekProbeTestRepo struct{ snapshotUpdateAccountRepo }

func (r *deepseekProbeTestRepo) UpdateDeepseekToolsSupport(ctx context.Context, account *Account, supported *bool) error {
	var marker any
	if supported != nil {
		marker = *supported
	}
	return r.UpdateExtra(ctx, account.ID, map[string]any{deepseekToolsSupportedExtraKey: marker})
}

func newDeepseekToolsProbeTestService(account Account, resp *http.Response) (*AccountTestService, *deepseekProbeTestRepo, *httpUpstreamRecorder) {
	updateCalls := make(chan map[string]any, 1)
	repo := &deepseekProbeTestRepo{snapshotUpdateAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{accounts: []Account{account}},
		updateExtraCalls:      updateCalls,
	}}
	upstream := &httpUpstreamRecorder{resp: resp}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	return svc, repo, upstream
}

func TestProbeDeepseekToolsSupportThirdParty200(t *testing.T) {
	account := Account{
		ID:          903,
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-probe",
			"base_url": "https://relay.example.com/v1",
		},
	}
	svc, repo, upstream := newDeepseekToolsProbeTestService(account, &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"results":[]}`)),
	})
	svc.ProbeDeepseekToolsSupport(context.Background(), account.ID)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, "https://relay.example.com/v1/tools/search", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-probe", upstream.lastReq.Header.Get("Authorization"))
	updates := <-repo.updateExtraCalls
	require.Equal(t, true, updates[deepseekToolsSupportedExtraKey])
}

func TestProbeDeepseekToolsSupportThirdParty404(t *testing.T) {
	account := Account{
		ID:          904,
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-probe",
			"base_url": "https://relay.example.com/v1",
		},
	}
	svc, repo, _ := newDeepseekToolsProbeTestService(account, &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"Invalid URL"}}`)),
	})
	svc.ProbeDeepseekToolsSupport(context.Background(), account.ID)
	updates := <-repo.updateExtraCalls
	require.Equal(t, false, updates[deepseekToolsSupportedExtraKey])
}

func TestProbeDeepseekToolsSupportOfficialSkipsNetwork(t *testing.T) {
	account := Account{
		ID:          905,
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-probe",
			"base_url": "https://api.deepseek.com",
		},
	}
	svc, repo, upstream := newDeepseekToolsProbeTestService(account, nil)
	svc.ProbeDeepseekToolsSupport(context.Background(), account.ID)
	require.Nil(t, upstream.lastReq, "official accounts must not spend a billed probe call")
	require.Empty(t, repo.updateExtraCalls)
}

func TestProbeDeepseekToolsSupportSkipsForcedMode(t *testing.T) {
	account := Account{
		ID:          906,
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-probe",
			"base_url": "https://relay.example.com/v1",
		},
		Extra: map[string]any{deepseekToolsModeExtraKey: deepseekToolsModeForceSupported},
	}
	svc, repo, upstream := newDeepseekToolsProbeTestService(account, nil)
	svc.ProbeDeepseekToolsSupport(context.Background(), account.ID)
	require.Nil(t, upstream.lastReq)
	require.Len(t, repo.updateExtraCalls, 0, "forced mode must skip probing and marker writes")
}

func TestProbeDeepseekToolsSupportServerErrorKeepsUnknown(t *testing.T) {
	account := Account{
		ID:          907,
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-probe",
			"base_url": "https://relay.example.com/v1",
		},
	}
	svc, repo, _ := newDeepseekToolsProbeTestService(account, &http.Response{
		StatusCode: http.StatusInternalServerError,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`error`)),
	})
	svc.ProbeDeepseekToolsSupport(context.Background(), account.ID)
	require.Len(t, repo.updateExtraCalls, 1)
	require.Nil(t, (<-repo.updateExtraCalls)[deepseekToolsSupportedExtraKey])
}
