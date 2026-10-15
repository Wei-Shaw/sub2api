package service

import (
	"context"
	"strings"
)

type relayOwnerContextKey struct{}

// Only authenticated relay routes set this scope; never infer ownership from JSON.
func WithRelayOwner(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, relayOwnerContextKey{}, userID)
}

func RelayOwnerFromContext(ctx context.Context) int64 {
	id, _ := ctx.Value(relayOwnerContextKey{}).(int64)
	return id
}

var relayEditableExtraKeys = strings.Fields(`email name email_address org_uuid account_uuid privacy_mode subscription_tier entitlement_status
	mixed_scheduling allow_overages anthropic_passthrough anthropic_apikey_auth_scheme
	openai_passthrough openai_oauth_passthrough openai_responses_flatten_namespaces
	openai_oauth_responses_websockets_v2_mode openai_oauth_responses_websockets_v2_enabled
	openai_apikey_responses_websockets_v2_mode openai_apikey_responses_websockets_v2_enabled
	openai_compact_mode openai_responses_mode codex_cli_only codex_cli_only_allow_app_server
	codex_fingerprint_mode images_url_to_b64_json web_search_emulation
	window_cost_limit window_cost_sticky_reserve max_sessions session_idle_timeout_minutes
	base_rpm rpm_strategy rpm_sticky_buffer user_msg_queue_mode session_id_masking_enabled
	cache_ttl_override_enabled cache_ttl_override_target custom_base_url_enabled custom_base_url
	upstream_request_id_header
	quota_limit quota_daily_limit quota_weekly_limit quota_daily_reset_mode quota_daily_reset_hour
	quota_weekly_reset_mode quota_weekly_reset_day quota_weekly_reset_hour quota_reset_timezone`)

// SanitizeRelayAccountExtra keeps account-level protocol controls, but not platform proxies, billing rates,
// parent-account links or system-managed state supplied by a client.
func SanitizeRelayAccountExtra(extra map[string]any) (map[string]any, error) {
	if extra == nil {
		return nil, nil
	}
	out := make(map[string]any)
	for _, key := range relayEditableExtraKeys {
		if value, ok := extra[key]; ok {
			out[key] = value
		}
	}
	if raw, ok := out["custom_base_url"]; ok && raw != "" && raw != nil {
		value, ok := raw.(string)
		if !ok {
			return nil, ErrGroupNotAllowed
		}
		if err := validateRelayURL(value); err != nil {
			return nil, err
		}
	}
	return out, nil
}
