package service

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAITemperatureObservationContextKey = "openai_temperature_observation"

	openAIRequestedTemperatureHeader = "X-Sub2API-Requested-Temperature"
	openAIForwardedTemperatureHeader = "X-Sub2API-Forwarded-Temperature"
	openAITemperaturePolicyHeader    = "X-Sub2API-Temperature-Policy"
	openAITemperatureStatusHeader    = "X-Sub2API-Temperature-Status"

	openAITemperatureOmitted = "omitted"
)

type openAITemperatureObservation struct {
	requested   string
	policy      accountTemperatureMode
	afterPolicy string
	forwarded   string
}

func beginOpenAITemperatureObservation(c *gin.Context, account *Account, body []byte) {
	if c == nil {
		return
	}
	if account == nil || account.Platform != PlatformOpenAI {
		c.Set(openAITemperatureObservationContextKey, nil)
		clearOpenAITemperatureObservationHeaders(c.Writer.Header())
		return
	}
	if openAITemperatureObservationFromContext(c) != nil {
		return
	}
	c.Set(openAITemperatureObservationContextKey, &openAITemperatureObservation{
		requested: temperatureHeaderValue(body),
		policy:    accountTemperatureModeInherit,
		forwarded: openAITemperatureOmitted,
	})
}

func clearOpenAITemperatureObservationHeaders(headers http.Header) {
	if headers == nil {
		return
	}
	headers.Del(openAIRequestedTemperatureHeader)
	headers.Del(openAIForwardedTemperatureHeader)
	headers.Del(openAITemperaturePolicyHeader)
	headers.Del(openAITemperatureStatusHeader)
}

func setOpenAITemperaturePolicyObservation(c *gin.Context, policy accountTemperaturePolicy, body []byte) {
	observation := openAITemperatureObservationFromContext(c)
	if observation == nil {
		return
	}
	observation.policy = policy.mode
	observation.afterPolicy = temperatureHeaderValue(body)
	observation.forwarded = openAITemperatureOmitted
	writeOpenAITemperatureObservationHeaders(c.Writer.Header(), observation)
}

func observeOpenAIForwardedTemperature(c *gin.Context, body []byte) {
	observation := openAITemperatureObservationFromContext(c)
	if observation == nil {
		return
	}
	observation.forwarded = temperatureHeaderValue(body)
	writeOpenAITemperatureObservationHeaders(c.Writer.Header(), observation)
}

func openAITemperatureObservationFromContext(c *gin.Context) *openAITemperatureObservation {
	if c == nil {
		return nil
	}
	value, exists := c.Get(openAITemperatureObservationContextKey)
	if !exists {
		return nil
	}
	observation, _ := value.(*openAITemperatureObservation)
	return observation
}

func temperatureHeaderValue(body []byte) string {
	value := gjson.GetBytes(body, "temperature")
	if !value.Exists() || value.Type == gjson.Null || value.Type != gjson.Number {
		return openAITemperatureOmitted
	}
	return value.Raw
}

func writeOpenAITemperatureObservationHeaders(headers http.Header, observation *openAITemperatureObservation) {
	if headers == nil || observation == nil {
		return
	}
	headers.Set(openAIRequestedTemperatureHeader, observation.requested)
	headers.Set(openAIForwardedTemperatureHeader, observation.forwarded)
	headers.Set(openAITemperaturePolicyHeader, string(observation.policy))
	headers.Set(openAITemperatureStatusHeader, observation.status())
}

func (o *openAITemperatureObservation) status() string {
	if o.policy == accountTemperatureModeOmit {
		return "omitted-policy"
	}
	if o.forwarded != openAITemperatureOmitted {
		return "forwarded"
	}
	if o.afterPolicy != "" && o.afterPolicy != openAITemperatureOmitted {
		return "omitted-unsupported"
	}
	return openAITemperatureOmitted
}
