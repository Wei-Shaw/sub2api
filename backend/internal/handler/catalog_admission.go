package handler

import (
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// writeCatalogAdmissionError keeps catalog lifecycle failures fail-closed while
// preserving the protocol-neutral error envelope used by gateway adapters.
func writeCatalogAdmissionError(c *gin.Context, err error) {
	if c == nil {
		return
	}
	status := service.CatalogAdmissionHTTPStatus(err)
	message := service.CatalogAdmissionHTTPMessage(err)
	if strings.Contains(c.Request.URL.Path, "/messages") {
		c.AbortWithStatusJSON(status, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": message,
			},
		})
		return
	}
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{
			"type":    "invalid_request_error",
			"code":    catalogAdmissionErrorCode(err),
			"message": message,
		},
	})
}

func catalogAdmissionErrorCode(err error) string {
	switch {
	case errors.Is(err, service.ErrCatalogModelDisabled):
		return "model_disabled"
	case errors.Is(err, service.ErrCatalogModelRetired):
		return "model_retired"
	case errors.Is(err, service.ErrCatalogModelNotFound):
		return "model_not_found"
	case errors.Is(err, service.ErrCatalogPricingUnavailable):
		return "model_pricing_unavailable"
	case errors.Is(err, service.ErrCatalogSourceUnavailable):
		return "model_source_unavailable"
	default:
		return "model_catalog_unavailable"
	}
}

// catalogEffectiveModelAdmissionError admits the account/provider target before
// any upstream I/O. Channel-mapped model is the account mapping input;
// requested model is only the fallback for protocol paths without channel
// mapping.
func catalogEffectiveModelAdmissionError(mapping *service.ChannelMappingResult, account *service.Account, requestedModel string) error {
	return catalogEffectiveModelAdmissionErrorForBase(mapping, account, requestedModel, "")
}

// openAIWSCatalogAdmissionError validates each response.create turn against a
// fresh catalog decision while preserving the selected account for the
// lifetime of the WebSocket connection.
func openAIWSCatalogAdmissionError(mapping *service.ChannelMappingResult, account *service.Account, requestedModel string) error {
	return catalogEffectiveModelAdmissionError(mapping, account, requestedModel)
}

func catalogEffectiveModelAdmissionErrorForBase(mapping *service.ChannelMappingResult, account *service.Account, requestedModel, baseModel string) error {
	if mapping == nil || account == nil {
		return service.ErrCatalogDecisionInvalid
	}
	accountInputModel := strings.TrimSpace(baseModel)
	if accountInputModel == "" {
		accountInputModel = strings.TrimSpace(mapping.MappedModel)
		if accountInputModel == "" || !mapping.Mapped {
			accountInputModel = strings.TrimSpace(requestedModel)
		}
	}
	return mapping.FinalizeCatalogEffectiveModel(account.GetMappedModel(accountInputModel))
}

func finalizeCatalogEffectiveModelWithBase(c *gin.Context, mapping *service.ChannelMappingResult, account *service.Account, requestedModel, baseModel string, release func()) bool {
	if err := catalogEffectiveModelAdmissionErrorForBase(mapping, account, requestedModel, baseModel); err != nil {
		if release != nil {
			release()
		}
		writeCatalogAdmissionError(c, err)
		return false
	}
	return true
}

func finalizeCatalogEffectiveModel(c *gin.Context, mapping *service.ChannelMappingResult, account *service.Account, requestedModel string, release func()) bool {
	return finalizeCatalogEffectiveModelWithBase(c, mapping, account, requestedModel, "", release)
}
