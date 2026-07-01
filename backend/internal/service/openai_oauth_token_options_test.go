package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

// stubTokenTLSRouterReader / stubTokenTLSProfileResolver are minimal test doubles
// for the ChatGPT OAuth token TLS dependencies (G5).
type stubTokenTLSRouterReader struct {
	router *model.TLSFingerprintRouter
}

func (s *stubTokenTLSRouterReader) GetRuntimeRouter(routerID int64) *model.TLSFingerprintRouter {
	return s.router
}

type stubTokenTLSProfileResolver struct {
	profiles map[int64]*tlsfingerprint.Profile
}

func (s *stubTokenTLSProfileResolver) ResolveTokenTLSProfileByID(id int64) (*tlsfingerprint.Profile, bool) {
	p, ok := s.profiles[id]
	return p, ok
}

func TestResolveChatGPTOAuthTokenRequestOptions(t *testing.T) {
	const profileID = int64(7)
	profile := &tlsfingerprint.Profile{Name: "codex"}

	// newSvc wires an OpenAIOAuthService with the given router (settingService left
	// nil — the UA fallback is guarded by a nil check).
	newSvc := func(router *model.TLSFingerprintRouter) *OpenAIOAuthService {
		return &OpenAIOAuthService{
			tlsFPRouterReader:    &stubTokenTLSRouterReader{router: router},
			tlsFPProfileResolver: &stubTokenTLSProfileResolver{profiles: map[int64]*tlsfingerprint.Profile{profileID: profile}},
		}
	}
	pid := profileID

	t.Run("routerID<=0 returns nil", func(t *testing.T) {
		require.Nil(t, newSvc(&model.TLSFingerprintRouter{Enabled: true, ChatGPTOAuthTokenUserAgent: "ua"}).
			resolveChatGPTOAuthTokenRequestOptions(context.Background(), 0, nil))
	})

	t.Run("router not found returns nil", func(t *testing.T) {
		svc := &OpenAIOAuthService{tlsFPRouterReader: &stubTokenTLSRouterReader{router: nil}}
		require.Nil(t, svc.resolveChatGPTOAuthTokenRequestOptions(context.Background(), 1, nil))
	})

	t.Run("disabled router returns nil", func(t *testing.T) {
		require.Nil(t, newSvc(&model.TLSFingerprintRouter{Enabled: false, ChatGPTOAuthTokenUserAgent: "ua"}).
			resolveChatGPTOAuthTokenRequestOptions(context.Background(), 1, nil))
	})

	t.Run("enabled but no UA and no profile returns nil", func(t *testing.T) {
		require.Nil(t, newSvc(&model.TLSFingerprintRouter{Enabled: true}).
			resolveChatGPTOAuthTokenRequestOptions(context.Background(), 1, nil))
	})

	t.Run("UA only", func(t *testing.T) {
		opts := newSvc(&model.TLSFingerprintRouter{Enabled: true, ChatGPTOAuthTokenUserAgent: "my-ua"}).
			resolveChatGPTOAuthTokenRequestOptions(context.Background(), 1, nil)
		require.Len(t, opts, 1)
		require.Equal(t, "my-ua", opts[0].UserAgent)
		require.Nil(t, opts[0].TLSProfile)
	})

	t.Run("profile only", func(t *testing.T) {
		opts := newSvc(&model.TLSFingerprintRouter{Enabled: true, ChatGPTOAuthTokenTLSFingerprintProfileID: &pid}).
			resolveChatGPTOAuthTokenRequestOptions(context.Background(), 1, nil)
		require.Len(t, opts, 1)
		require.Equal(t, "", opts[0].UserAgent)
		require.NotNil(t, opts[0].TLSProfile)
		require.Equal(t, "codex", opts[0].TLSProfile.Name)
	})

	t.Run("UA + profile + account", func(t *testing.T) {
		account := &Account{ID: 42, Concurrency: 5}
		opts := newSvc(&model.TLSFingerprintRouter{
			Enabled:                                  true,
			ChatGPTOAuthTokenUserAgent:               "ua2",
			ChatGPTOAuthTokenTLSFingerprintProfileID: &pid,
		}).resolveChatGPTOAuthTokenRequestOptions(context.Background(), 1, account)
		require.Len(t, opts, 1)
		require.Equal(t, "ua2", opts[0].UserAgent)
		require.NotNil(t, opts[0].TLSProfile)
		require.Equal(t, int64(42), opts[0].AccountID)
		require.Equal(t, 5, opts[0].AccountConcurrency)
	})
}
