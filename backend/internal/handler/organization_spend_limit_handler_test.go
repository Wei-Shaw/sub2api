//go:build unit

package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type spendLimitHandlerRepositoryStub struct {
	service.OrganizationRepository
	service.OrganizationSpendLimitRepository
	ownerID     int64
	memberIDs   []int64
	daily       *string
	monthly     *string
	alert       bool
	threshold   float64
	recipients  []string
	upsertCalls int
}

func (s *spendLimitHandlerRepositoryStub) UpsertSpendLimitRules(
	_ context.Context,
	ownerID int64,
	memberIDs []int64,
	daily, monthly *string,
	alertEnabled bool,
	threshold float64,
	recipients []string,
) ([]service.OrganizationSpendLimitRule, error) {
	s.upsertCalls++
	s.ownerID = ownerID
	s.memberIDs = memberIDs
	s.daily = daily
	s.monthly = monthly
	s.alert = alertEnabled
	s.threshold = threshold
	s.recipients = recipients
	return []service.OrganizationSpendLimitRule{}, nil
}

func TestUpsertSpendLimitsAcceptsNumericJSONAmounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repository := &spendLimitHandlerRepositoryStub{}
	handler := NewOrganizationHandler(service.NewOrganizationService(repository, nil, &config.Config{}), nil, nil, nil, nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/organization/spend-limits", bytes.NewBufferString(
		`{"target":"all","daily_limit_usd":10,"monthly_limit_usd":10,"alert_enabled":false,"alert_threshold_pct":80,"additional_recipients":[]}`,
	))
	request.Header.Set("Content-Type", "application/json")
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = request
	ctx.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 42})

	handler.UpsertSpendLimits(ctx)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, 1, repository.upsertCalls)
	require.Equal(t, int64(42), repository.ownerID)
	require.Empty(t, repository.memberIDs)
	require.NotNil(t, repository.daily)
	require.Equal(t, "10.0000000000", *repository.daily)
	require.NotNil(t, repository.monthly)
	require.Equal(t, "10.0000000000", *repository.monthly)
	require.False(t, repository.alert)
	require.Equal(t, float64(80), repository.threshold)
	require.Empty(t, repository.recipients)
}
