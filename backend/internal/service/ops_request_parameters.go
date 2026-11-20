package service

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const opsRequestParametersKey = "ops_request_parameters"

type opsRequestParameters struct {
	reasoningEffort string
	serviceTier     string
}

// SetOpsRequestParameters captures explicit client parameters before policy or
// upstream rewriting. Unknown string values are useful when diagnosing errors;
// never normalize them or infer a default from a model or missing field.
func SetOpsRequestParameters(c *gin.Context, body []byte) {
	if c == nil {
		return
	}
	params := opsRequestParameters{}
	if gjson.ValidBytes(body) {
		readString := func(path string) string {
			value := gjson.GetBytes(body, path)
			if value.Type != gjson.String {
				return ""
			}
			// Keep only compact metadata, not the request body, in the log queue.
			valueRunes := []rune(strings.TrimSpace(value.String()))
			if len(valueRunes) > 256 {
				valueRunes = valueRunes[:256]
			}
			return string(valueRunes)
		}
		for _, path := range []string{"reasoning.effort", "reasoning_effort", "output_config.effort"} {
			if params.reasoningEffort = readString(path); params.reasoningEffort != "" {
				break
			}
		}
		params.serviceTier = readString("service_tier")
	}
	c.Set(opsRequestParametersKey, params)
}

func OpsRequestParameters(c *gin.Context) (reasoningEffort, serviceTier string) {
	if c == nil {
		return "", ""
	}
	value, _ := c.Get(opsRequestParametersKey)
	params, _ := value.(opsRequestParameters)
	return params.reasoningEffort, params.serviceTier
}
