package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/textproto"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"golang.org/x/net/http/httpguts"
)

const maxGatewayRuntimePreserveHeaders = 32

type GatewayOutboundPrivacySettings struct {
	Enabled                bool     `json:"enabled"`
	StrictAccountIsolation bool     `json:"strict_account_isolation"`
	PreserveHeaders        []string `json:"preserve_headers"`
}

type GatewayOpenAIWSBudgetSettings struct {
	MaxConnsPerAccount int `json:"max_conns_per_account"`
	MinIdlePerAccount  int `json:"min_idle_per_account"`
	MaxIdlePerAccount  int `json:"max_idle_per_account"`
}

type GatewayRuntimeSettings struct {
	ConnectionPoolIsolation string                         `json:"connection_pool_isolation"`
	OutboundPrivacy         GatewayOutboundPrivacySettings `json:"outbound_privacy"`
	OpenAIWS                GatewayOpenAIWSBudgetSettings  `json:"openai_ws"`
}

func gatewayRuntimeSettingsFromConfig(cfg *config.Config) GatewayRuntimeSettings {
	snapshot := cfg.GatewayRuntimeSettingsSnapshot()
	preserveHeaders := append([]string{}, snapshot.OutboundPrivacy.PreserveHeaders...)
	return GatewayRuntimeSettings{
		ConnectionPoolIsolation: snapshot.ConnectionPoolIsolation,
		OutboundPrivacy: GatewayOutboundPrivacySettings{
			Enabled:                snapshot.OutboundPrivacy.Enabled,
			StrictAccountIsolation: snapshot.OutboundPrivacy.StrictAccountIsolation,
			PreserveHeaders:        preserveHeaders,
		},
		OpenAIWS: GatewayOpenAIWSBudgetSettings{
			MaxConnsPerAccount: snapshot.OpenAIWS.MaxConnsPerAccount,
			MinIdlePerAccount:  snapshot.OpenAIWS.MinIdlePerAccount,
			MaxIdlePerAccount:  snapshot.OpenAIWS.MaxIdlePerAccount,
		},
	}
}

func gatewayRuntimeSettingsToConfig(settings GatewayRuntimeSettings) config.GatewayRuntimeSettings {
	return config.GatewayRuntimeSettings{
		ConnectionPoolIsolation: settings.ConnectionPoolIsolation,
		OutboundPrivacy: config.GatewayOutboundPrivacyConfig{
			Enabled:                settings.OutboundPrivacy.Enabled,
			StrictAccountIsolation: settings.OutboundPrivacy.StrictAccountIsolation,
			PreserveHeaders:        append([]string(nil), settings.OutboundPrivacy.PreserveHeaders...),
		},
		OpenAIWS: config.GatewayOpenAIWSRuntimeSettings{
			MaxConnsPerAccount: settings.OpenAIWS.MaxConnsPerAccount,
			MinIdlePerAccount:  settings.OpenAIWS.MinIdlePerAccount,
			MaxIdlePerAccount:  settings.OpenAIWS.MaxIdlePerAccount,
		},
	}
}

func normalizeGatewayRuntimeSettings(settings GatewayRuntimeSettings) (GatewayRuntimeSettings, error) {
	settings.ConnectionPoolIsolation = strings.ToLower(strings.TrimSpace(settings.ConnectionPoolIsolation))
	switch settings.ConnectionPoolIsolation {
	case config.ConnectionPoolIsolationProxy,
		config.ConnectionPoolIsolationAccount,
		config.ConnectionPoolIsolationAccountProxy:
	default:
		return GatewayRuntimeSettings{}, fmt.Errorf(
			"connection_pool_isolation must be one of %s, %s, or %s",
			config.ConnectionPoolIsolationProxy,
			config.ConnectionPoolIsolationAccount,
			config.ConnectionPoolIsolationAccountProxy,
		)
	}
	if settings.OutboundPrivacy.Enabled &&
		settings.OutboundPrivacy.StrictAccountIsolation &&
		settings.ConnectionPoolIsolation == config.ConnectionPoolIsolationProxy {
		settings.ConnectionPoolIsolation = config.ConnectionPoolIsolationAccountProxy
	}

	if len(settings.OutboundPrivacy.PreserveHeaders) > maxGatewayRuntimePreserveHeaders {
		return GatewayRuntimeSettings{}, fmt.Errorf(
			"outbound_privacy.preserve_headers must contain at most %d names",
			maxGatewayRuntimePreserveHeaders,
		)
	}
	seenHeaders := make(map[string]struct{}, len(settings.OutboundPrivacy.PreserveHeaders))
	normalizedHeaders := make([]string, 0, len(settings.OutboundPrivacy.PreserveHeaders))
	for _, header := range settings.OutboundPrivacy.PreserveHeaders {
		header = strings.TrimSpace(header)
		if !httpguts.ValidHeaderFieldName(header) {
			return GatewayRuntimeSettings{}, fmt.Errorf("invalid outbound privacy preserve header %q", header)
		}
		canonical := textproto.CanonicalMIMEHeaderKey(header)
		key := strings.ToLower(canonical)
		if _, exists := seenHeaders[key]; exists {
			continue
		}
		seenHeaders[key] = struct{}{}
		normalizedHeaders = append(normalizedHeaders, canonical)
	}
	settings.OutboundPrivacy.PreserveHeaders = normalizedHeaders

	if settings.OpenAIWS.MaxConnsPerAccount <= 0 {
		return GatewayRuntimeSettings{}, fmt.Errorf("openai_ws.max_conns_per_account must be positive")
	}
	if settings.OpenAIWS.MinIdlePerAccount < 0 {
		return GatewayRuntimeSettings{}, fmt.Errorf("openai_ws.min_idle_per_account must be non-negative")
	}
	if settings.OpenAIWS.MaxIdlePerAccount < 0 {
		return GatewayRuntimeSettings{}, fmt.Errorf("openai_ws.max_idle_per_account must be non-negative")
	}
	if settings.OpenAIWS.MinIdlePerAccount > settings.OpenAIWS.MaxIdlePerAccount {
		return GatewayRuntimeSettings{}, fmt.Errorf("openai_ws.min_idle_per_account must be <= max_idle_per_account")
	}
	if settings.OpenAIWS.MaxIdlePerAccount > settings.OpenAIWS.MaxConnsPerAccount {
		return GatewayRuntimeSettings{}, fmt.Errorf("openai_ws.max_idle_per_account must be <= max_conns_per_account")
	}
	return settings, nil
}

// LoadGatewayRuntimeSettings initializes the live snapshot from the deployment
// config and then applies a valid DB override when one exists.
func (s *SettingService) LoadGatewayRuntimeSettings(ctx context.Context) error {
	if s == nil || s.cfg == nil {
		return nil
	}
	fallback, err := normalizeGatewayRuntimeSettings(gatewayRuntimeSettingsFromConfig(s.cfg))
	if err != nil {
		return fmt.Errorf("normalize gateway runtime deployment settings: %w", err)
	}
	s.cfg.SetGatewayRuntimeSettings(gatewayRuntimeSettingsToConfig(fallback))
	if s.settingRepo == nil {
		return nil
	}
	values, err := s.settingRepo.GetMultiple(ctx, []string{SettingKeyGatewayRuntimeSettings})
	if err != nil {
		return fmt.Errorf("get gateway runtime settings: %w", err)
	}
	raw := strings.TrimSpace(values[SettingKeyGatewayRuntimeSettings])
	if raw == "" {
		return nil
	}
	var stored GatewayRuntimeSettings
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return fmt.Errorf("decode gateway runtime settings: %w", err)
	}
	stored, err = normalizeGatewayRuntimeSettings(stored)
	if err != nil {
		return fmt.Errorf("validate gateway runtime settings: %w", err)
	}
	s.cfg.SetGatewayRuntimeSettings(gatewayRuntimeSettingsToConfig(stored))
	return nil
}

func (s *SettingService) GetGatewayRuntimeSettings(context.Context) (*GatewayRuntimeSettings, error) {
	if s == nil || s.cfg == nil {
		return nil, fmt.Errorf("gateway runtime configuration is unavailable")
	}
	settings := gatewayRuntimeSettingsFromConfig(s.cfg)
	return &settings, nil
}

func (s *SettingService) SetGatewayRuntimeSettings(ctx context.Context, settings *GatewayRuntimeSettings) error {
	if s == nil || s.cfg == nil || s.settingRepo == nil {
		return fmt.Errorf("gateway runtime configuration is unavailable")
	}
	if settings == nil {
		return fmt.Errorf("gateway runtime settings are required")
	}
	normalized, err := normalizeGatewayRuntimeSettings(*settings)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("encode gateway runtime settings: %w", err)
	}
	if err := s.settingRepo.SetMultiple(ctx, map[string]string{SettingKeyGatewayRuntimeSettings: string(payload)}); err != nil {
		return fmt.Errorf("save gateway runtime settings: %w", err)
	}
	s.cfg.SetGatewayRuntimeSettings(gatewayRuntimeSettingsToConfig(normalized))
	return nil
}
