package service

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Image-input model rewrite: when a request carries image input (screenshots,
// pasted images, image blocks) for a text-only model deployment, the upstream
// rejects it with a 400 such as "not a multimodal model". The gateway-level
// mapping (gateway.image_input_model_map) rewrites the request's model to a
// vision-capable alternative before account selection, so the request can be
// served by a vision-capable account instead of failing. Pure-text requests
// are unaffected.

// imageInputContentTypes are the content-part type markers that carry image
// input across the supported API shapes:
//
//	Anthropic /v1/messages      -> messages[].content[].type == "image"
//	OpenAI /v1/chat/completions -> messages[].content[].type == "image_url"
//	OpenAI /v1/responses        -> input[].content[].type == "input_image"
var imageInputContentTypes = map[string]struct{}{
	"image":       {},
	"image_url":   {},
	"input_image": {},
}

// imageInputPayloadKeys are the field names an image part carries its payload
// under. Requiring a payload field avoids treating an element that only has a
// type marker (but no actual image content) as image input.
var imageInputPayloadKeys = []string{"source", "image_url", "image", "data"}

// RequestBodyHasImageInput reports whether the request body carries image
// input. Supports Anthropic messages, OpenAI chat.completions, and responses
// shapes; returns false for empty or invalid bodies.
func RequestBodyHasImageInput(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	for _, container := range [...]string{"messages", "input"} {
		if v := gjson.GetBytes(body, container); v.IsArray() {
			if gjsonValueHasImageInput(v, 0) {
				return true
			}
		}
	}
	return false
}

// gjsonValueHasImageInput recursively searches a JSON subtree for an image
// content part that carries a payload. The depth cap guards against
// pathological nesting.
func gjsonValueHasImageInput(v gjson.Result, depth int) bool {
	if depth > 16 {
		return false
	}
	switch {
	case v.IsArray():
		for _, item := range v.Array() {
			if gjsonValueHasImageInput(item, depth+1) {
				return true
			}
		}
	case v.IsObject():
		if t := v.Get("type"); t.Exists() && t.Type == gjson.String {
			if _, ok := imageInputContentTypes[strings.ToLower(strings.TrimSpace(t.String()))]; ok && objectHasAnyKey(v, imageInputPayloadKeys) {
				return true
			}
		}
		found := false
		v.ForEach(func(_, child gjson.Result) bool {
			if gjsonValueHasImageInput(child, depth+1) {
				found = true
				return false
			}
			return true
		})
		return found
	}
	return false
}

func objectHasAnyKey(v gjson.Result, keys []string) bool {
	for _, k := range keys {
		if v.Get(k).Exists() {
			return true
		}
	}
	return false
}

// RewriteImageInputModel returns the body with the `model` field rewritten to
// the mapped vision model when the request carries image input and modelMap
// covers the current model. Returns the input unchanged when any step does not
// apply.
func RewriteImageInputModel(body []byte, modelMap map[string]string) (newBody []byte, newModel string, changed bool) {
	if len(body) == 0 || len(modelMap) == 0 {
		return body, "", false
	}
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if model == "" {
		return body, "", false
	}
	target, ok := lookupImageInputModelMap(modelMap, model)
	if !ok || target == "" || target == model {
		return body, "", false
	}
	if !RequestBodyHasImageInput(body) {
		return body, "", false
	}
	updated, err := sjson.SetBytes(body, "model", target)
	if err != nil {
		return body, "", false
	}
	return updated, target, true
}

// lookupImageInputModelMap tries an exact match first, then a
// case-insensitive match, and tolerates the Claude Code long-context suffix
// (e.g. "xxx[1m]") by looking up both the normalized and the original name.
func lookupImageInputModelMap(modelMap map[string]string, model string) (string, bool) {
	normalized := normalizeClaudeCodeLongContextModel(strings.TrimSpace(model))
	candidates := []string{model, normalized}
	if normalized == model {
		candidates = []string{model}
	}
	for _, candidate := range candidates {
		if target, ok := modelMap[candidate]; ok {
			return strings.TrimSpace(target), true
		}
	}
	for _, candidate := range candidates {
		for k, v := range modelMap {
			if strings.EqualFold(strings.TrimSpace(k), candidate) {
				return strings.TrimSpace(v), true
			}
		}
	}
	return "", false
}
