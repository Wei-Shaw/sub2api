package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAnnotateOpenAISelectionFailure_UsesPlaceholderWhenNoAccountSelected(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)

	annotateOpenAISelectionFailure(c, nil, nil, "", nil, "Service temporarily unavailable: no available accounts")

	v, ok := c.Get(service.OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := v.([]*service.OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, int64(0), events[0].AccountID)
	require.Equal(t, noScheduledAccountLabel, events[0].AccountName)
	require.Equal(t, "selection", events[0].Kind)
	require.Equal(t, 503, events[0].UpstreamStatusCode)
}

func TestAnnotateOpenAISelectionFailure_PreservesSelectedAccountSnapshot(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	setOpsSelectedAccount(c, 42, "sticky-openai-account", service.PlatformOpenAI)

	annotateOpenAISelectionFailure(c, nil, nil, "", nil, "Service temporarily unavailable: context canceled")

	v, ok := c.Get(service.OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := v.([]*service.OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, int64(42), events[0].AccountID)
	require.Equal(t, "sticky-openai-account", events[0].AccountName)
}

func TestAnnotateOpenAISelectionFailure_CapturesStructuredDetail(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)

	err := &service.OpenAISelectionError{
		Phase:  "candidate_filtering",
		Cause:  "no available accounts remain after candidate filtering",
		Detail: "group_id=6 model=gpt-5.5 total=3 eligible=0 model_unsupported=2 channel_restricted=1",
		Err:    service.ErrNoAvailableAccounts,
	}
	annotateOpenAISelectionFailure(c, nil, nil, "", err, "Service temporarily unavailable: no available accounts remain after candidate filtering")

	v, ok := c.Get(service.OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := v.([]*service.OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Contains(t, events[0].Detail, "phase=candidate_filtering")
	require.Contains(t, events[0].Detail, "cause=no available accounts remain after candidate filtering")
	require.Contains(t, events[0].Detail, "group_id=6 model=gpt-5.5")
}
