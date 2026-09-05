//go:build unit

package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestRecordNoAvailableAccountsReasonForOps_KeepsReasonOffTheWire(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	recordNoAvailableAccountsReasonForOps(c, profitVetoExhaustedReason)

	v, ok := c.Get(service.OpsUpstreamErrorMessageKey)
	require.True(t, ok)
	require.Equal(t, "No available accounts: "+profitVetoExhaustedReason, v)

	_, hasStatus := c.Get(service.OpsUpstreamStatusCodeKey)
	require.False(t, hasStatus, "the request never reached upstream; no upstream status may be recorded")

	// The wire message must not describe the pool or its scheduler.
	require.NotContains(t, strings.ToLower(noAvailableAccountsClientMessage), "account")
	require.NotContains(t, strings.ToLower(noAvailableAccountsClientMessage), "profit")
}

func TestRecordNoAvailableAccountsErrorForOps_OnlyRecordsPoolErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("pool exhaustion is recorded with the scheduler's reason", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)

		recordNoAvailableAccountsErrorForOps(c, errors.New("no available accounts supporting model: gpt-5 (total=3 eligible=0 model_rate_limited=3)"))

		v, ok := c.Get(service.OpsUpstreamErrorMessageKey)
		require.True(t, ok)
		require.True(t, isOpsNoAvailableAccountMessage(v.(string)))
		require.Contains(t, v.(string), "model_rate_limited=3")
	})

	t.Run("unrelated selection failure is not attributed to the pool", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)

		recordNoAvailableAccountsErrorForOps(c, errors.New("redis: connection refused"))

		_, ok := c.Get(service.OpsUpstreamErrorMessageKey)
		require.False(t, ok)
	})
}

func TestOpsFilterTextWithContext_ExposesRecordedReasonToSettingsFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	require.Equal(t, noAvailableAccountsClientMessage, opsFilterTextWithContext(c, noAvailableAccountsClientMessage))

	recordNoAvailableAccountsReasonForOps(c, noAvailableAccountsReasonNoSlot)

	text := opsFilterTextWithContext(c, noAvailableAccountsClientMessage)
	require.True(t, strings.HasPrefix(text, noAvailableAccountsClientMessage))
	// The IgnoreNoAvailableAccounts filter keys on this phrase; it must survive the neutral body.
	require.Contains(t, strings.ToLower(text), opsErrNoAvailableAccounts)
}

func TestClassifyOpsErrorLog_NeutralBodyKeepsRoutingAttribution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	markOpsRoutingCapacityLimited(c)
	recordNoAvailableAccountsReasonForOps(c, noAvailableAccountsReasonNoSlot)

	phase, isBusinessLimited, errorOwner, errorSource := classifyOpsErrorLog(
		c,
		"api_error",
		noAvailableAccountsClientMessage,
		"",
		http.StatusServiceUnavailable,
	)

	// Same verdict as the former "No available accounts" body: the recorded
	// upstream message must not flip the entry to an upstream/provider failure.
	require.Equal(t, "routing", phase)
	require.True(t, isBusinessLimited)
	require.Equal(t, "platform", errorOwner)
	require.Equal(t, "gateway", errorSource)
}
