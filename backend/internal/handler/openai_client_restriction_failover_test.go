package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newOpenAIClientRestrictionTestContext(t *testing.T) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("User-Agent", "Mozilla/5.0")
	return recorder, c
}

func TestOpenAIClientRestrictionFailoverState_ExcludesRestrictedAccount(t *testing.T) {
	_, c := newOpenAIClientRestrictionTestContext(t)
	releaseCount := 0
	selection := &service.AccountSelectionResult{
		Account: &service.Account{
			ID:       1001,
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": true},
		},
		Acquired: true,
		ReleaseFunc: func() {
			releaseCount++
		},
	}
	excludedIDs := make(map[int64]struct{})
	var state openAIClientRestrictionFailoverState

	result, excluded := state.excludeSelection(
		&service.OpenAIGatewayService{},
		c,
		[]byte(`{"model":"gpt-5.4"}`),
		selection,
		excludedIDs,
	)

	require.True(t, excluded)
	require.True(t, result.Enabled)
	require.False(t, result.Matched)
	require.Equal(t, service.CodexClientRestrictionReasonNotMatchedUA, result.Reason)
	require.Contains(t, excludedIDs, int64(1001))
	require.Equal(t, 1, releaseCount)
	require.False(t, selection.Acquired)
	require.Nil(t, selection.ReleaseFunc)
	require.NotNil(t, state.lastResult)
	require.Equal(t, int64(1001), state.lastAccountID)
	require.Equal(t, service.PlatformOpenAI, state.lastAccountPlatform)
}

func TestOpenAIClientRestrictionFailoverState_KeepsCompatibleAccount(t *testing.T) {
	_, c := newOpenAIClientRestrictionTestContext(t)
	releaseCount := 0
	selection := &service.AccountSelectionResult{
		Account: &service.Account{
			ID:       1002,
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeOAuth,
			Extra:    map[string]any{"codex_cli_only": false},
		},
		Acquired:    true,
		ReleaseFunc: func() { releaseCount++ },
	}
	excludedIDs := make(map[int64]struct{})
	var state openAIClientRestrictionFailoverState

	result, excluded := state.excludeSelection(
		&service.OpenAIGatewayService{},
		c,
		[]byte(`{"model":"gpt-5.4"}`),
		selection,
		excludedIDs,
	)

	require.False(t, excluded)
	require.False(t, result.Enabled)
	require.Empty(t, excludedIDs)
	require.Equal(t, 0, releaseCount)
	require.True(t, selection.Acquired)
	require.NotNil(t, selection.ReleaseFunc)
	require.Nil(t, state.lastResult)
}

func TestOpenAIClientRestrictionFailoverState_RejectsAfterExhaustion(t *testing.T) {
	recorder, c := newOpenAIClientRestrictionTestContext(t)
	state := openAIClientRestrictionFailoverState{
		lastResult: &service.CodexClientRestrictionDetectionResult{
			Enabled: true,
			Matched: false,
			Reason:  service.CodexClientRestrictionReasonNotMatchedUA,
		},
	}

	rejected := state.rejectIfExhausted(&OpenAIGatewayHandler{}, c, false)

	require.True(t, rejected)
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), service.CodexOfficialClientsOnlyMessage)
}
