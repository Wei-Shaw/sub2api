package service

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// OpenAICodexTicketRuntimeStatus exposes only the settings needed by the ticket page.
type OpenAICodexTicketRuntimeStatus struct {
	Enabled                bool     `json:"enabled"`
	RefreshBeforeSeconds   int      `json:"refresh_before_seconds"`
	TTLSeconds             int      `json:"ttl_seconds"`
	Models                 []string `json:"models"`
	HarvestProxyConfigured bool     `json:"harvest_proxy_configured"`
}

func (s *SettingService) GetOpenAICodexTicketRuntimeStatus(ctx context.Context) (*OpenAICodexTicketRuntimeStatus, error) {
	settings, err := s.GetAllSettings(ctx)
	if err != nil {
		return nil, err
	}
	cfg := config.OpenAICodexTicketConfig{}
	if s.cfg != nil {
		cfg = s.cfg.Gateway.OpenAICodexTicket
	}
	if cfg.RefreshBeforeSeconds <= 0 {
		cfg.RefreshBeforeSeconds = 600
	}
	if cfg.TTLSeconds <= 0 {
		cfg.TTLSeconds = 3600
	}
	if len(cfg.Models) == 0 {
		cfg.Models = []string{openAICodexTicketDefaultModel, openAICodexTicketDefaultSolModel}
	}
	return &OpenAICodexTicketRuntimeStatus{
		Enabled: settings.OpenAICodexTicketEnabled, RefreshBeforeSeconds: cfg.RefreshBeforeSeconds,
		TTLSeconds: cfg.TTLSeconds, Models: cfg.Models,
		HarvestProxyConfigured: strings.TrimSpace(settings.OpenAICodexTicketHarvestProxyURL) != "" || strings.TrimSpace(cfg.HarvestProxyURL) != "",
	}, nil
}
