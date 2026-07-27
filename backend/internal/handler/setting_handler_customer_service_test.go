//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type customerServicePublicSettingRepoStub struct {
	service.SettingRepository
	values map[string]string
}

func (r *customerServicePublicSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func TestSettingHandlerGetPublicSettingsMapsCustomerServiceFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &customerServicePublicSettingRepoStub{values: map[string]string{
		service.SettingKeyAfterSalesTitle:            "VIP Support",
		service.SettingKeyAfterSalesLink:             "https://example.com/support",
		service.SettingKeyAfterSalesLinkLabel:        "Start a chat",
		service.SettingKeyOfficialGroupTitle:         "Community",
		service.SettingKeyOfficialGroupLink:          "https://example.com/community",
		service.SettingKeyOfficialGroupLinkLabel:     "Join now",
		service.SettingKeyCustomerServiceTextEnabled: "true",
		service.SettingKeyCustomerServiceText:        "Support hours",
	}}
	handler := NewSettingHandler(service.NewSettingService(repo, &config.Config{}), "test")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/settings/public", nil)
	handler.GetPublicSettings(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Data dto.PublicSettings `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, "VIP Support", body.Data.AfterSalesTitle)
	require.Equal(t, "https://example.com/support", body.Data.AfterSalesLink)
	require.Equal(t, "Start a chat", body.Data.AfterSalesLinkLabel)
	require.Equal(t, "Community", body.Data.OfficialGroupTitle)
	require.Equal(t, "https://example.com/community", body.Data.OfficialGroupLink)
	require.Equal(t, "Join now", body.Data.OfficialGroupLinkLabel)
	require.True(t, body.Data.CustomerServiceTextEnabled)
	require.Equal(t, "Support hours", body.Data.CustomerServiceText)
}
