//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestGeminiNewEndpointsRejectUnavailableProxyGroup(t *testing.T) {
	NewProxyGroupService(&proxyGroupServiceRepoStub{})
	defer SetDefaultProxyGroupResolver(nil)
	groupID := int64(7)
	account := &Account{ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey, ProxyGroupID: &groupID, Credentials: map[string]any{"api_key": "test"}}
	upstream := &httpUpstreamRecorder{}
	compat := newGeminiModelsEndpointsTestService(nil, upstream)
	_, err := compat.ForwardListModels(context.Background(), account)
	require.Equal(t, "PROXY_GROUP_NO_AVAILABLE_PROXY", infraerrors.Reason(err))
	_, err = compat.ForwardGetModel(context.Background(), account, "gemini-test")
	require.Equal(t, "PROXY_GROUP_NO_AVAILABLE_PROXY", infraerrors.Reason(err))
	cache := &GeminiCachedContentService{compat: compat}
	_, err = cache.doUpstream(context.Background(), account, http.MethodPost, "https://example.com/v1beta/cachedContents", nil, time.Second)
	require.ErrorIs(t, err, ErrGeminiCachedContentRequestNotSent)
	require.Nil(t, upstream.lastReq)
}
