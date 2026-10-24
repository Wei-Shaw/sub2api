package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type transientCatalogAccountRepo struct {
	service.AccountRepository
	groupID  int64
	accounts []service.Account
}

func (r *transientCatalogAccountRepo) ListSchedulableByGroupID(_ context.Context, groupID int64) ([]service.Account, error) {
	var accounts []service.Account
	if groupID == r.groupID {
		for _, account := range r.accounts {
			if account.IsSchedulable() {
				accounts = append(accounts, account)
			}
		}
	}
	return accounts, nil
}

func (r *transientCatalogAccountRepo) ListModelAvailabilityCandidates(_ context.Context, groupID *int64, platforms []string, _ bool) ([]service.Account, error) {
	var accounts []service.Account
	if groupID == nil || *groupID != r.groupID {
		return accounts, nil
	}
	for _, account := range r.accounts {
		if account.Status != service.StatusActive || !account.Schedulable {
			continue
		}
		for _, platform := range platforms {
			if account.Platform == platform {
				accounts = append(accounts, account)
				break
			}
		}
	}
	return accounts, nil
}

func TestGatewayModelsPreserveCatalogAcrossTemporaryUnavailability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const groupID int64 = 7801
	until := time.Now().Add(time.Hour)
	account := service.Account{
		ID: 101, Platform: service.PlatformDeepseek, Type: service.AccountTypeAPIKey,
		Status: service.StatusActive, Schedulable: true,
		Credentials: map[string]any{"model_mapping": map[string]any{"deepseek-flash": "deepseek-flash"}},
	}
	account.SetUpstreamModelMetadataSnapshot(service.UpstreamModelMetadataSnapshot{Models: map[string]service.UpstreamModelMetadata{
		"deepseek-flash": {
			ID: "deepseek-flash", DisplayName: "Configured Flash", Description: "Upstream model description.",
			SupportedReasoningLevels: []string{"low", "high", "max"}, ContextWindow: 1_000_000,
		},
	}})
	for _, platform := range []string{service.PlatformComposite, service.PlatformDeepseek} {
		t.Run(platform, func(t *testing.T) {
			var baselineBody, baselineETag string
			for _, state := range []string{"healthy", "balance", "rate_limit", "overload", "recovered", "disabled"} {
				t.Run(state, func(t *testing.T) {
					current := account
					openAI := service.Account{ID: 102, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true}
					switch state {
					case "balance":
						current.TempUnschedulableUntil = &until
						current.TempUnschedulableReason = "cn_balance_low: insufficient balance"
						openAI.TempUnschedulableUntil = &until
					case "rate_limit":
						current.RateLimitResetAt = &until
						openAI.RateLimitResetAt = &until
					case "overload":
						current.OverloadUntil = &until
						openAI.OverloadUntil = &until
					case "disabled":
						current.Schedulable = false
					}
					repo := &transientCatalogAccountRepo{groupID: groupID, accounts: []service.Account{
						current, openAI,
						{ID: 103, Platform: service.PlatformDeepseek, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true,
							Credentials: map[string]any{"model_mapping": map[string]any{"other-model": "other-model"}}},
					}}
					// A fresh handler prevents a warm model-list cache from hiding the bug.
					h := newGatewayModelsHandlerForTest(repo)
					group := &service.Group{ID: groupID, Platform: platform}
					apiKey := &service.APIKey{GroupID: &group.ID, Group: group}
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodGet, "/models", nil)
					c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
					h.CodexModels(c)
					require.Equal(t, http.StatusOK, rec.Code)
					var manifest struct {
						Models []struct {
							Slug        string `json:"slug"`
							DisplayName string `json:"display_name"`
							Description string `json:"description"`
						} `json:"models"`
					}
					require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &manifest))
					found := false
					for _, model := range manifest.Models {
						if model.Slug == "deepseek-flash" {
							found = true
							require.Equal(t, "Configured Flash", model.DisplayName)
							require.Equal(t, "Upstream model description.", model.Description)
						}
					}
					require.Equal(t, state != "disabled", found)
					if state == "healthy" {
						baselineBody, baselineETag = rec.Body.String(), rec.Header().Get("ETag")
					} else if state != "disabled" {
						require.JSONEq(t, baselineBody, rec.Body.String())
						require.Equal(t, baselineETag, rec.Header().Get("ETag"))
					}
					standard := httptest.NewRecorder()
					c, _ = gin.CreateTestContext(standard)
					c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
					c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
					h.Models(c)
					require.Equal(t, http.StatusOK, standard.Code)
					var models gatewayModelsResponseForTest
					require.NoError(t, json.Unmarshal(standard.Body.Bytes(), &models))
					listed := false
					for _, model := range models.Data {
						listed = listed || model.ID == "deepseek-flash"
					}
					require.Equal(t, state != "disabled", listed)
				})
			}
		})
	}
}
