package service

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/google/uuid"
	"github.com/tidwall/sjson"
)

const (
	GatewayKimiCodeResponsesProfile     = "kimi-code-global-responses-apikey-v1"
	GatewayKimiCodeResponsesBaseURL     = "https://api.kimi.ai/coding/v1"
	GatewayKimiCodeResponsesModel       = "k3-256k"
	GatewayGLMCodingResponsesProfile    = "glm-coding-global-responses-apikey-v1"
	GatewayGLMCodingResponsesBaseURL    = "https://api.z.ai/api/v1"
	GatewayGLMCodingResponsesModel      = "glm-5.3-flash"
	GatewayMiniMaxMPlanResponsesProfile = "minimax-mplan-global-responses-apikey-v1"
	GatewayMiniMaxMPlanResponsesBaseURL = "https://api.minimax.io/v1"
	GatewayMiniMaxMPlanResponsesModel   = "MiniMax-M3.1-Flash"
)

// New subscription tuples are immutable source definitions. A definition is
// fixture-capable; production enrollment remains a separate deployment gate.
func GatewayNativeCodingPlanProfileValid(profile, baseURL, model string) bool {
	switch profile {
	case GatewayKimiCodeResponsesProfile:
		return baseURL == GatewayKimiCodeResponsesBaseURL && model == GatewayKimiCodeResponsesModel
	case GatewayGLMCodingResponsesProfile:
		return baseURL == GatewayGLMCodingResponsesBaseURL && model == GatewayGLMCodingResponsesModel
	case GatewayMiniMaxMPlanResponsesProfile:
		return baseURL == GatewayMiniMaxMPlanResponsesBaseURL && model == GatewayMiniMaxMPlanResponsesModel
	}
	return false
}

func GatewayNativeCodingPlanProfileIDValid(profile string) bool {
	switch profile {
	case GatewayKimiCodeResponsesProfile, GatewayGLMCodingResponsesProfile, GatewayMiniMaxMPlanResponsesProfile:
		return true
	}
	return false
}

func gatewayNativeProviderProfile(profile string) bool {
	switch profile {
	case GatewayKimiCodeResponsesProfile, GatewayGLMCodingResponsesProfile, GatewayMiniMaxMPlanResponsesProfile, GatewayGrokOAuthResponsesProfile, GatewayNVIDIABuildChatProfile:
		return true
	}
	return false
}

// Reject lossy/unsupported input before admission and custody. Only text and
// client function/custom tools enter this slice. Schema and tool/user bytes are
// opaque and preserved; nested schema keys never select transport authority.
func ValidateGatewayNativeProviderResponsesBounded(profile string, body []byte, requestBytes int64) error {
	_, _, err := gatewayNativePrepareProviderResponses(profile, body, requestBytes)
	return err
}

func gatewayNativePrepareProviderResponses(profile string, body []byte, requestBytes int64) ([]byte, apicompat.ResponsesClientToolMapping, error) {
	empty := apicompat.ResponsesClientToolMapping{}
	deny := func() ([]byte, apicompat.ResponsesClientToolMapping, error) {
		return nil, empty, ErrGatewayNativeIdentity
	}
	if !gatewayNativeProviderProfile(profile) || requestBytes < 1 || requestBytes > 4<<20 || int64(len(body)) > requestBytes {
		return deny()
	}
	// These four qualified slices do not support a reasoning setting. Codex
	// serializes Option.None for reasoning/text as null; normalize only these
	// exact nulls, without changing configured values or the admitted input.
	policy, valid := GatewayNativePayloadFields(body, "reasoning", "text")
	if !valid {
		return deny()
	}
	copied := false
	for _, name := range []string{"reasoning", "text"} {
		raw := policy[name]
		if raw == nil {
			continue
		}
		null := bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
		if name == "reasoning" && !null {
			return deny()
		}
		if null {
			if !copied {
				body = append([]byte(nil), body...)
				copied = true
			}
			var err error
			body, err = sjson.DeleteBytes(body, name)
			if err != nil {
				return deny()
			}
		}
	}
	fields, ok := gatewayClaudeObject(body, "model", "input", "instructions", "stream", "store", "service_tier", "max_output_tokens", "tools", "tool_choice", "reasoning", "parallel_tool_calls", "temperature", "top_p", "text", "include", "prompt_cache_key", "client_metadata")
	if !ok || string(fields["stream"]) != "true" || string(fields["store"]) != "false" {
		return deny()
	}
	// Codex 0.133 emits a thread UUID and the single canonical installation
	// UUID (core/src/client.rs, installation_id.rs). These remain bounded body
	// metadata: they never supply routing, authentication or outgoing headers.
	if raw := fields["prompt_cache_key"]; raw != nil && !gatewayNativeProviderUUID(raw) {
		return deny()
	}
	if raw := fields["client_metadata"]; raw != nil {
		if len(raw) > 512 {
			return deny()
		}
		metadata, valid := gatewayClaudeObject(raw, "x-codex-installation-id")
		if !valid || len(metadata) != 1 || !gatewayNativeProviderUUID(metadata["x-codex-installation-id"]) {
			return deny()
		}
	}
	// The default tier is optional on the public Responses wire. Explicit
	// provider selection remains closed: no auto/priority/null coercion.
	if raw := fields["service_tier"]; raw != nil {
		var tier string
		if json.Unmarshal(raw, &tier) != nil || tier != "default" {
			return deny()
		}
	}
	var model string
	if json.Unmarshal(fields["model"], &model) != nil {
		return deny()
	}
	switch profile {
	case GatewayNVIDIABuildChatProfile:
		if !GatewayNVIDIABuildModelValid(model) {
			return deny()
		}
	case GatewayKimiCodeResponsesProfile, GatewayGLMCodingResponsesProfile, GatewayMiniMaxMPlanResponsesProfile:
		if !GatewayCodingPlanModelAdmitted(profile, model) {
			return deny()
		}
	case GatewayGrokOAuthResponsesProfile:
		if model != GatewayGrokOAuthModel {
			return deny()
		}
	}
	if _, ok := gatewayOAuthSeconds(fields["max_output_tokens"]); !ok || !gatewayNativeProviderTextInput(profile, fields["input"]) {
		return deny()
	}
	if raw := fields["instructions"]; raw != nil {
		var v string
		if json.Unmarshal(raw, &v) != nil {
			return deny()
		}
	}
	if raw := fields["include"]; raw != nil {
		var include []string
		if json.Unmarshal(raw, &include) != nil {
			return deny()
		}
		for _, value := range include {
			if value != "reasoning.encrypted_content" {
				return deny()
			}
		}
	}
	if raw := fields["text"]; raw != nil {
		text, valid := gatewayClaudeObject(raw, "format", "verbosity")
		if !valid {
			return deny()
		}
		if format := text["format"]; format != nil {
			format, valid := gatewayClaudeObject(format, "type", "name", "strict", "schema")
			var kind string
			if !valid || json.Unmarshal(format["type"], &kind) != nil {
				return deny()
			}
			switch kind {
			case "text":
				if len(format) != 1 {
					return deny()
				}
			case "json_schema":
				// Pinned Codex create_text_param_for_request uses this fixed name.
				// Keep the strictness and schema opaque rather than reinterpret them.
				var name string
				var strict bool
				if len(format) != 4 || json.Unmarshal(format["name"], &name) != nil || name != "codex_output_schema" ||
					json.Unmarshal(format["strict"], &strict) != nil {
					return deny()
				}
				if _, valid := gatewayNativeCanonicalObject(format["schema"]); !valid {
					return deny()
				}
			default:
				return deny()
			}
		}
		if verbosity := text["verbosity"]; verbosity != nil {
			var v string
			if json.Unmarshal(verbosity, &v) != nil || (v != "low" && v != "medium" && v != "high") {
				return deny()
			}
		}
	}
	if raw := fields["tools"]; raw != nil && !gatewayNativeProviderTools(raw) {
		return deny()
	}
	if raw := fields["tool_choice"]; raw != nil {
		var choice string
		if json.Unmarshal(raw, &choice) == nil {
			if choice != "auto" && choice != "none" && choice != "required" {
				return deny()
			}
		} else {
			choice, valid := gatewayClaudeObject(raw, "type", "name", "namespace")
			var kind, name string
			if !valid || json.Unmarshal(choice["type"], &kind) != nil || (kind != "function" && kind != "custom") || json.Unmarshal(choice["name"], &name) != nil || name == "" {
				return deny()
			}
			if namespace := choice["namespace"]; namespace != nil && (kind != "function" || !gatewayNativeProviderToolName(namespace)) {
				return deny()
			}
		}
	}
	// Use only the pure client-tool codec, never the ordinary Grok sanitizer
	// which drops unsupported tools/modalities and owns retry policy.
	wire, mapping, err := adaptResponsesClientToolsForFunctionUpstream(body, "qualified provider")
	if err != nil || int64(len(wire)) > requestBytes {
		return deny()
	}
	if !gatewayNativeProviderNamespaceReferences(fields["input"], fields["tool_choice"], mapping) {
		return deny()
	}
	return wire, mapping, nil
}

func gatewayNativeProviderTextInput(profile string, raw []byte) bool {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return true
	}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil || len(items) > 4096 {
		return false
	}
	for _, raw := range items {
		fields, ok := gatewayNativeCanonicalObject(raw, "type", "role", "content", "input", "output", "name", "call_id", "namespace")
		if !ok {
			return false
		}
		var kind string
		if t := fields["type"]; t != nil && json.Unmarshal(t, &kind) != nil {
			return false
		}
		switch kind {
		case "", "message":
			fields, ok = gatewayClaudeObject(raw, "type", "id", "role", "content", "status")
			var role string
			if !ok || json.Unmarshal(fields["role"], &role) != nil || (role != "user" && role != "assistant" && role != "system" && role != "developer") {
				return false
			}
			if json.Unmarshal(fields["content"], &text) == nil {
				continue
			}
			var parts []json.RawMessage
			if json.Unmarshal(fields["content"], &parts) != nil {
				return false
			}
			partKeys := []string{"type", "text", "annotations"}
			// OpenRouter returns an empty logprobs array on every output_text
			// part and a client replays the item unchanged (confirmed in
			// production: the only refused item of a replayed history).
			if profile == GatewayOpenRouterResponsesProfile {
				partKeys = append(partKeys, "logprobs")
			}
			for _, raw := range parts {
				part, ok := gatewayClaudeObject(raw, partKeys...)
				if !ok || (string(part["type"]) != `"input_text"` && string(part["type"]) != `"output_text"`) || json.Unmarshal(part["text"], &text) != nil {
					return false
				}
				if logprobs := part["logprobs"]; logprobs != nil {
					var entries []json.RawMessage
					if json.Unmarshal(logprobs, &entries) != nil {
						return false
					}
				}
			}
		case "function_call", "custom_tool_call":
			fields, ok = gatewayClaudeObject(raw, "type", "id", "status", "name", "call_id", "arguments", "input", "namespace")
			if !ok || !gatewayNativeProviderString(fields["call_id"]) || !gatewayNativeProviderString(fields["name"]) {
				return false
			}
			if namespace := fields["namespace"]; namespace != nil && (kind != "function_call" || !gatewayNativeProviderToolName(namespace)) {
				return false
			}
			if kind == "function_call" {
				if !gatewayNativeProviderString(fields["arguments"]) || fields["input"] != nil {
					return false
				}
			} else if !gatewayNativeProviderString(fields["input"]) || fields["arguments"] != nil {
				return false
			}
		case "function_call_output", "custom_tool_call_output":
			fields, ok = gatewayClaudeObject(raw, "type", "id", "call_id", "output")
			if !ok || !gatewayNativeProviderString(fields["call_id"]) || !gatewayNativeProviderString(fields["output"]) {
				return false
			}
		case "reasoning":
			keys := []string{"type", "id", "summary", "encrypted_content", "status"}
			// MiniMax and the open reasoning models served through OpenRouter
			// return clear reasoning content in their Responses output. Clients
			// replay output items unchanged on the next turn, so a text-only
			// reasoning_text content part must be accepted there; without it
			// every multi-turn review on such a model was refused at input
			// admission. It stays opaque through the existing client-tool codec.
			if profile == GatewayMiniMaxMPlanResponsesProfile || profile == GatewayOpenRouterResponsesProfile || profile == GatewayNVIDIABuildChatProfile {
				keys = append(keys, "content")
			}
			// OpenRouter tags every reasoning item with the format of its
			// reasoning data (observed in production: a replayed item with this
			// key was the only thing refused at input admission).
			if profile == GatewayOpenRouterResponsesProfile {
				keys = append(keys, "format")
			}
			fields, ok = gatewayClaudeObject(raw, keys...)
			if !ok {
				return false
			}
			if content := fields["content"]; content != nil {
				var parts []json.RawMessage
				if bytes.Equal(bytes.TrimSpace(content), []byte("null")) || json.Unmarshal(content, &parts) != nil {
					return false
				}
				for _, raw := range parts {
					part, valid := gatewayClaudeObject(raw, "type", "text")
					if !valid || string(part["type"]) != `"reasoning_text"` || !gatewayNativeProviderString(part["text"]) {
						return false
					}
				}
			}
			if value := fields["encrypted_content"]; value != nil && !gatewayNativeProviderString(value) {
				return false
			}
			if value := fields["format"]; value != nil && !gatewayNativeProviderString(value) {
				return false
			}
			if summary := fields["summary"]; summary != nil {
				var parts []json.RawMessage
				if json.Unmarshal(summary, &parts) != nil {
					return false
				}
				for _, raw := range parts {
					part, ok := gatewayClaudeObject(raw, "type", "text")
					if !ok || string(part["type"]) != `"summary_text"` || !gatewayNativeProviderString(part["text"]) {
						return false
					}
				}
			}
		default:
			return false
		}
	}
	return true
}

func gatewayNativeProviderString(raw []byte) bool {
	var v string
	return !bytes.Equal(raw, []byte("null")) && json.Unmarshal(raw, &v) == nil
}

func gatewayNativeProviderUUID(raw []byte) bool {
	if len(raw) > 256 {
		return false
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || len(value) != 36 {
		return false
	}
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}

// One explicit namespace level can contain only function children. The existing
// pure codec preserves their namespace identities and opaque schemas. It does
// not preserve custom children, so they are rejected instead of silently lost.
func gatewayNativeProviderTools(raw []byte) bool {
	var tools []json.RawMessage
	if json.Unmarshal(raw, &tools) != nil || len(tools) > 128 {
		return false
	}
	leaves := 0
	namespaces := map[string]bool{}
	for _, raw := range tools {
		kindFields, ok := GatewayNativePayloadFields(raw, "type")
		var kind string
		if !ok || json.Unmarshal(kindFields["type"], &kind) != nil {
			return false
		}
		if kind != "namespace" {
			if !gatewayNativeProviderLeafTool(raw) {
				return false
			}
			leaves++
			if leaves > 128 {
				return false
			}
			continue
		}
		namespace, ok := gatewayClaudeObject(raw, "type", "name", "description", "tools")
		var name, description string
		if !ok || !gatewayNativeProviderToolName(namespace["name"]) || json.Unmarshal(namespace["name"], &name) != nil || namespaces[name] {
			return false
		}
		namespaces[name] = true
		if raw := namespace["description"]; raw != nil && json.Unmarshal(raw, &description) != nil {
			return false
		}
		var children []json.RawMessage
		if json.Unmarshal(namespace["tools"], &children) != nil || len(children) == 0 || len(children) > 128 {
			return false
		}
		childNames := map[string]bool{}
		for _, raw := range children {
			child, ok := GatewayNativePayloadFields(raw, "type", "name")
			var childKind, childName string
			if !ok || json.Unmarshal(child["type"], &childKind) != nil || childKind != "function" || !gatewayNativeProviderLeafTool(raw) || json.Unmarshal(child["name"], &childName) != nil || childNames[childName] {
				return false
			}
			childNames[childName] = true
			leaves++
			if leaves > 128 {
				return false
			}
		}
	}
	return true
}

func gatewayNativeProviderLeafTool(raw []byte) bool {
	tool, valid := gatewayClaudeObject(raw, "type", "name", "description", "parameters", "strict", "format")
	var kind, description string
	if !valid || json.Unmarshal(tool["type"], &kind) != nil || !gatewayNativeProviderToolName(tool["name"]) {
		return false
	}
	if raw := tool["description"]; raw != nil && json.Unmarshal(raw, &description) != nil {
		return false
	}
	switch kind {
	case "function":
		if _, valid := gatewayNativeCanonicalObject(tool["parameters"]); !valid || tool["format"] != nil {
			return false
		}
		if raw := tool["strict"]; raw != nil {
			var strict bool
			if json.Unmarshal(raw, &strict) != nil {
				return false
			}
		}
		return true
	case "custom":
		return tool["parameters"] == nil && tool["strict"] == nil
	}
	return false
}

func gatewayNativeProviderToolName(raw []byte) bool {
	var name string
	return json.Unmarshal(raw, &name) == nil && name != "" && len(name) <= 128 && strings.TrimSpace(name) == name
}

// A namespace history/choice reference must name a declared function. Otherwise
// the pure codec would leave it unflattened and change the qualified protocol.
func gatewayNativeProviderNamespaceReferences(input, choice []byte, mapping apicompat.ResponsesClientToolMapping) bool {
	reference := func(raw []byte) bool {
		fields, ok := GatewayNativePayloadFields(raw, "namespace", "name", "type")
		if !ok {
			return false
		}
		if fields["namespace"] == nil {
			return true
		}
		var namespace, name string
		if json.Unmarshal(fields["namespace"], &namespace) != nil || json.Unmarshal(fields["name"], &name) != nil {
			return false
		}
		for _, entry := range mapping.NamespaceTools {
			if entry.Namespace == namespace && entry.Name == name {
				return true
			}
		}
		return false
	}
	var text string
	if json.Unmarshal(input, &text) != nil {
		var items []json.RawMessage
		if json.Unmarshal(input, &items) != nil {
			return false
		}
		for _, item := range items {
			if !reference(item) {
				return false
			}
		}
	}
	if len(choice) > 0 && json.Unmarshal(choice, &text) != nil && !reference(choice) {
		return false
	}
	return true
}
