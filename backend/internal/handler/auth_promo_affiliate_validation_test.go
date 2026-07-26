//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type sharedCodeSettingRepoStub struct {
	service.SettingRepository
}

func (r *sharedCodeSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	switch key {
	case service.SettingKeyPromoCodeEnabled, service.SettingKeyAffiliateEnabled:
		return "true", nil
	default:
		return "", service.ErrSettingNotFound
	}
}

type sharedCodePromoRepoStub struct {
	service.PromoCodeRepository
	codes map[string]*service.PromoCode
}

func (r *sharedCodePromoRepoStub) GetByCode(_ context.Context, code string) (*service.PromoCode, error) {
	promoCode, ok := r.codes[code]
	if !ok {
		return nil, service.ErrPromoCodeNotFound
	}
	return promoCode, nil
}

type sharedCodeAffiliateRepoStub struct {
	service.AffiliateRepository
	codes map[string]*service.AffiliateSummary
}

func (r *sharedCodeAffiliateRepoStub) GetAffiliateByCode(_ context.Context, code string) (*service.AffiliateSummary, error) {
	affiliate, ok := r.codes[code]
	if !ok {
		return nil, service.ErrAffiliateProfileNotFound
	}
	return affiliate, nil
}

func TestValidatePromoCodeChecksPromoAndAffiliateCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settingService := service.NewSettingService(&sharedCodeSettingRepoStub{}, nil)
	promoService := service.NewPromoService(&sharedCodePromoRepoStub{codes: map[string]*service.PromoCode{
		"PROMO": {Code: "PROMO", BonusAmount: 20, Status: service.PromoCodeStatusActive},
		"DUAL":  {Code: "DUAL", BonusAmount: 5, Status: service.PromoCodeStatusActive},
	}}, nil, nil, nil, nil)
	affiliateService := service.NewAffiliateService(&sharedCodeAffiliateRepoStub{codes: map[string]*service.AffiliateSummary{
		"AFFILIATE": {UserID: 7, AffCode: "AFFILIATE"},
		"DUAL":      {UserID: 8, AffCode: "DUAL"},
	}}, settingService, nil, nil)
	authService := service.NewAuthService(nil, nil, nil, nil, nil, settingService, nil, nil, nil, promoService, nil, affiliateService, nil)
	handler := NewAuthHandler(nil, authService, nil, settingService, promoService, nil, nil, nil)

	tests := []struct {
		name           string
		code           string
		valid          bool
		promoValid     bool
		affiliateValid bool
		bonusAmount    float64
	}{
		{name: "promo only", code: "PROMO", valid: true, promoValid: true, bonusAmount: 20},
		{name: "affiliate only", code: "affiliate", valid: true, affiliateValid: true},
		{name: "same code matches both", code: "DUAL", valid: true, promoValid: true, affiliateValid: true, bonusAmount: 5},
		{name: "neither", code: "UNKNOWN", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(ValidatePromoCodeRequest{Code: tt.code})
			require.NoError(t, err)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/validate-promo-code", bytes.NewReader(body))
			ctx.Request.Header.Set("Content-Type", "application/json")

			handler.ValidatePromoCode(ctx)

			require.Equal(t, http.StatusOK, recorder.Code)
			var responseBody struct {
				Data ValidatePromoCodeResponse `json:"data"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &responseBody))
			require.Equal(t, tt.valid, responseBody.Data.Valid)
			require.Equal(t, tt.promoValid, responseBody.Data.PromoValid)
			require.Equal(t, tt.affiliateValid, responseBody.Data.AffiliateValid)
			require.Equal(t, tt.bonusAmount, responseBody.Data.BonusAmount)
		})
	}
}
