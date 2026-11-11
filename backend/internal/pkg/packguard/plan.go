package packguard

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	SeedanceModel    = "dreamina-seedance-2-5-260628"
	SeedModel        = "dola-seed-2-1-turbo-260628"
	MaxBodyBytes     = 1 << 20
	MaxResponseBytes = 8 << 20
	VideoLifetime    = time.Hour
)

func permittedKeys(v map[string]any, list string) error {
	keys := map[string]bool{}
	for _, key := range strings.Fields(list) {
		keys[key] = true
	}
	for key := range v {
		if !keys[key] {
			return fmt.Errorf("pack-only mode does not support this request field")
		}
	}
	return nil
}
func integer(v any, fallback int64) (int64, error) {
	if v == nil {
		return fallback, nil
	}
	n, ok := v.(float64)
	if !ok || math.IsNaN(n) || n != math.Trunc(n) || n < 0 || n > 1e9 {
		return 0, fmt.Errorf("invalid integer in pack-only request")
	}
	return int64(n), nil
}
func stringField(v map[string]any, key, fallback string) (string, error) {
	if v[key] == nil {
		return fallback, nil
	}
	value, ok := v[key].(string)
	if !ok {
		return "", fmt.Errorf("invalid string in pack-only request")
	}
	return value, nil
}

// BuildPlan canonicalizes JSON before dispatch, rejects unpriced features, and
// reserves a conservative ceiling. Only qualified, pinned model versions pass.
func BuildPlan(path string, body []byte) (Plan, []byte, error) {
	var p Plan
	if len(body) == 0 || len(body) > MaxBodyBytes {
		return p, nil, fmt.Errorf("pack-only request exceeds the supported body size")
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil || raw == nil {
		return p, nil, fmt.Errorf("pack-only requests require a JSON object")
	}
	model, err := stringField(raw, "model", "")
	if err != nil {
		return p, nil, err
	}
	p.Model = model
	switch path {
	case "/contents/generations/tasks":
		if model != SeedanceModel {
			return p, nil, fmt.Errorf("this video model has no qualified prepaid funding")
		}
		if err = permittedKeys(raw, "model content duration resolution ratio generate_audio seed watermark return_last_frame execution_expires_after service_tier priority safety_identifier"); err != nil {
			return p, nil, err
		}
		lifetime, e := integer(raw["execution_expires_after"], 3600)
		if e != nil || lifetime != 3600 {
			return p, nil, fmt.Errorf("pack-only video jobs require a one-hour execution lifetime")
		}
		raw["execution_expires_after"] = int64(3600)
		tier, e := stringField(raw, "service_tier", "")
		if e != nil || (tier != "" && tier != "default" && tier != "standard") {
			return p, nil, fmt.Errorf("unqualified video service tier")
		}
		priority, e := integer(raw["priority"], 0)
		if e != nil || priority != 0 {
			return p, nil, fmt.Errorf("priority video pricing is not qualified for pack-only mode")
		}
		seconds := int64(30)
		if raw["duration"] != nil && raw["duration"] != float64(-1) {
			seconds, e = integer(raw["duration"], 30)
			if e != nil || seconds < 4 || seconds > 30 {
				return p, nil, fmt.Errorf("video duration must be 4 to 30 seconds")
			}
		}
		resolution, e := stringField(raw, "resolution", "1080p")
		if e != nil {
			return p, nil, e
		}
		var pixels int64
		switch resolution {
		case "480p":
			pixels = 500000
		case "720p":
			pixels = 1100000
		case "1080p":
			pixels = 2400000
		default:
			return p, nil, fmt.Errorf("unqualified video resolution")
		}
		ratio, e := stringField(raw, "ratio", "adaptive")
		if e != nil {
			return p, nil, e
		}
		switch ratio {
		case "adaptive", "16:9", "4:3", "1:1", "3:4", "9:16", "21:9":
		default:
			return p, nil, fmt.Errorf("unqualified video aspect ratio")
		}
		for _, key := range []string{"generate_audio", "watermark", "return_last_frame"} {
			if raw[key] != nil {
				if _, ok := raw[key].(bool); !ok {
					return p, nil, fmt.Errorf("invalid boolean in pack-only video request")
				}
			}
		}
		content, ok := raw["content"].([]any)
		if !ok || len(content) == 0 || len(content) > 50 {
			return p, nil, fmt.Errorf("invalid video reference content")
		}
		withVideo := false
		for _, item := range content {
			object, ok := item.(map[string]any)
			if !ok {
				return p, nil, fmt.Errorf("invalid video reference")
			}
			kind, _ := object["type"].(string)
			switch kind {
			case "text":
				if err = permittedKeys(object, "type text"); err != nil {
					return p, nil, err
				}
				text, ok := object["text"].(string)
				if !ok {
					return p, nil, fmt.Errorf("invalid video text")
				}
				if strings.Contains(text, "--") {
					return p, nil, fmt.Errorf("use JSON video settings; inline parameter overrides are disabled in pack-only mode")
				}
			case "image_url", "video_url":
				if err = permittedKeys(object, "type "+kind+" role"); err != nil {
					return p, nil, err
				}
				media, ok := object[kind].(map[string]any)
				if !ok {
					return p, nil, fmt.Errorf("invalid media reference")
				}
				if err = permittedKeys(media, "url"); err != nil {
					return p, nil, err
				}
				link, _ := media["url"].(string)
				allowed := strings.HasPrefix(link, "https://")
				if kind == "image_url" && strings.HasPrefix(link, "data:image/") {
					allowed = true
				}
				if !allowed {
					return p, nil, fmt.Errorf("asset licensing and non-HTTPS references are not qualified")
				}
				if kind == "video_url" {
					withVideo = true
				}
			default:
				return p, nil, fmt.Errorf("unqualified video reference type")
			}
		}
		inputSeconds := int64(0)
		if withVideo {
			inputSeconds = 30
		}
		// Qualified output is 24 fps and <=30s. Pixel ceilings exceed the
		// documented dimensions; 4x headroom also covers rounding/minimum usage.
		ceiling := (pixels*24*(seconds+inputSeconds+2)*4 + 1023) / 1024
		p.Ratio = 107
		if resolution == "1080p" {
			p.Ratio = 117
		}
		if withVideo {
			p.Ratio = 64
			if resolution == "1080p" {
				p.Ratio = 70
			}
		}
		p.Units = ceiling * p.Ratio
		p.Lifetime = VideoLifetime
		p.Async = true
	case "/chat/completions", "/responses":
		if model != SeedModel {
			return p, nil, fmt.Errorf("this text model has no qualified free-token funding")
		}
		if err = permittedKeys(raw, "model messages input instructions temperature top_p seed max_tokens max_completion_tokens max_output_tokens stream stream_options frequency_penalty presence_penalty n stop tools tool_choice parallel_tool_calls response_format thinking reasoning_effort reasoning text store service_tier user prompt_cache_key metadata"); err != nil {
			return p, nil, err
		}
		if raw["service_tier"] != nil && raw["service_tier"] != "default" {
			return p, nil, fmt.Errorf("unqualified text service tier")
		}
		n, e := integer(raw["n"], 1)
		if e != nil || n != 1 {
			return p, nil, fmt.Errorf("pack-only text supports one output")
		}
		maximum := int64(0)
		for _, key := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
			if raw[key] != nil {
				value, e := integer(raw[key], 8192)
				if e != nil || value < 1 || value > 32768 {
					return p, nil, fmt.Errorf("text output must be bounded to 1..32768 tokens")
				}
				maximum = max(maximum, value)
			}
		}
		if maximum == 0 {
			maximum = 8192
		}
		if v, ok := raw["thinking"].(map[string]any); ok {
			if err = permittedKeys(v, "type"); err != nil {
				return p, nil, err
			}
		} else if raw["thinking"] != nil {
			return p, nil, fmt.Errorf("invalid thinking controls")
		}
		if v, ok := raw["reasoning"].(map[string]any); ok {
			if err = permittedKeys(v, "effort"); err != nil {
				return p, nil, err
			}
		} else if raw["reasoning"] != nil {
			return p, nil, fmt.Errorf("invalid reasoning controls")
		}
		// An inclusive output limit covers both reasoning and visible output.
		delete(raw, "max_tokens")
		delete(raw, "max_completion_tokens")
		delete(raw, "max_output_tokens")
		if path == "/responses" {
			raw["max_output_tokens"] = maximum
			raw["store"] = false
		} else {
			raw["max_completion_tokens"] = maximum
			if raw["stream"] == true {
				raw["stream_options"] = map[string]any{"include_usage": true}
			}
		}
		if tools, ok := raw["tools"].([]any); ok {
			for _, tool := range tools {
				v, ok := tool.(map[string]any)
				if !ok || v["type"] != "function" {
					return p, nil, fmt.Errorf("paid provider tools are disabled in pack-only mode")
				}
			}
		} else if raw["tools"] != nil {
			return p, nil, fmt.Errorf("invalid function tools")
		}
		if choice, ok := raw["tool_choice"].(map[string]any); ok && choice["type"] != "function" {
			return p, nil, fmt.Errorf("unqualified tool choice")
		}
		if err = validateTextInput(raw["messages"]); err != nil {
			return p, nil, err
		}
		if err = validateTextInput(raw["input"]); err != nil {
			return p, nil, err
		}
		p.Ratio = UnitsPerToken
		p.Lifetime = 5 * time.Minute
		canonical, e := json.Marshal(raw)
		if e != nil {
			return p, nil, e
		}
		// UTF-8 bytes plus JSON framing overestimates text BPE input. Remote
		// context, files, images and paid auxiliary features are rejected.
		p.Units = (int64(len(canonical)) + maximum + 1024) * UnitsPerToken
	default:
		return p, nil, fmt.Errorf("this endpoint is not qualified for pack-only dispatch")
	}
	canonical, err := json.Marshal(raw)
	return p, canonical, err
}

func validateTextInput(value any) error {
	switch v := value.(type) {
	case nil, string:
		return nil
	case []any:
		for _, item := range v {
			if err := validateTextInput(item); err != nil {
				return err
			}
		}
	case map[string]any:
		if t, ok := v["type"].(string); ok && t != "message" && t != "text" && t != "input_text" && t != "output_text" && t != "function_call" && t != "function_call_output" {
			return fmt.Errorf("pack-only text rejects remote context and non-text content")
		}
		if err := permittedKeys(v, "type role content text name tool_calls tool_call_id id call_id output arguments status function"); err != nil {
			return err
		}
		for _, key := range []string{"content", "output"} {
			if err := validateTextInput(v[key]); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("invalid text content")
	}
	return nil
}
