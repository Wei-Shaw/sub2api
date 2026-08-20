package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *ModelAPIGatewayHandler) estimatePricing(c *gin.Context, path string) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil {
		h.jsonError(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	endpoint := strings.Trim(strings.TrimSuffix(path, "/estimate_pricing"), "/")
	if endpoint == "" {
		h.jsonError(c, http.StatusBadRequest, "invalid_request_error", "model endpoint is required")
		return
	}

	var params map[string]any
	decoder := json.NewDecoder(c.Request.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&params); err != nil {
		h.jsonError(c, http.StatusBadRequest, "invalid_request_error", "request parameters must be a JSON object")
		return
	}
	if input, ok := params["input"].(map[string]any); ok {
		params = input
	}
	dimensions, err := extractEstimateDimensions(params)
	if err != nil {
		h.jsonError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	count, err := extractEstimateImageCount(params)
	if err != nil {
		h.jsonError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	quality, _ := params["quality"].(string)
	estimate, err := h.gatewayService.EstimateImagePricing(c.Request.Context(), apiKey, endpoint, dimensions, quality, count)
	if err != nil {
		if errors.Is(err, service.ErrImagePricingModelUnsupported) {
			h.jsonError(c, http.StatusNotFound, "not_found_error", err.Error())
			return
		}
		h.jsonError(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, estimate)
}

func extractEstimateDimensions(params map[string]any) (service.ImageDimensions, error) {
	for _, key := range []string{"image_size", "size", "resolution"} {
		value, exists := params[key]
		if !exists {
			continue
		}
		switch typed := value.(type) {
		case string:
			if dimensions, ok := knownFALImageSize(typed); ok {
				return dimensions, nil
			}
			return service.ParseImageDimensions(typed)
		case map[string]any:
			return dimensionsFromMap(typed)
		}
	}
	return dimensionsFromMap(params)
}

func knownFALImageSize(value string) (service.ImageDimensions, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "square":
		return service.ImageDimensions{Width: 512, Height: 512}, true
	case "square_hd":
		return service.ImageDimensions{Width: 1024, Height: 1024}, true
	case "portrait_4_3":
		return service.ImageDimensions{Width: 768, Height: 1024}, true
	case "portrait_16_9":
		return service.ImageDimensions{Width: 576, Height: 1024}, true
	case "landscape_4_3":
		return service.ImageDimensions{Width: 1024, Height: 768}, true
	case "landscape_16_9":
		return service.ImageDimensions{Width: 1024, Height: 576}, true
	default:
		return service.ImageDimensions{}, false
	}
}

func dimensionsFromMap(values map[string]any) (service.ImageDimensions, error) {
	width, err := positiveJSONInt(values["width"], "width")
	if err != nil {
		return service.ImageDimensions{}, err
	}
	height, err := positiveJSONInt(values["height"], "height")
	if err != nil {
		return service.ImageDimensions{}, err
	}
	return service.ImageDimensions{Width: width, Height: height}, nil
}

func extractEstimateImageCount(params map[string]any) (int, error) {
	for _, key := range []string{"num_images", "n", "image_count"} {
		if value, exists := params[key]; exists {
			return positiveJSONInt(value, key)
		}
	}
	return 1, nil
}

func positiveJSONInt(value any, field string) (int, error) {
	if value == nil {
		return 0, fmt.Errorf("%s is required", field)
	}
	var number float64
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, fmt.Errorf("%s must be a positive integer", field)
		}
		number = parsed
	case float64:
		number = typed
	case int:
		number = float64(typed)
	default:
		return 0, fmt.Errorf("%s must be a positive integer", field)
	}
	if number <= 0 || number != math.Trunc(number) || number > float64(math.MaxInt) {
		return 0, fmt.Errorf("%s must be a positive integer", field)
	}
	return int(number), nil
}
