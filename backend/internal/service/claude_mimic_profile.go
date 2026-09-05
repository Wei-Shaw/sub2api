package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
)

func (s *GatewayService) claudeCodeMimicProfile() claude.Profile {
	id := claude.DefaultProfileID
	if s != nil && s.cfg != nil && s.cfg.Gateway.ClaudeCodeMimicProfile != "" {
		id = s.cfg.Gateway.ClaudeCodeMimicProfile
	}
	return claude.ResolveProfile(id)
}

func (s *GatewayService) claudeCodeMimicMode(_ context.Context) claude.MimicMode {
	if s != nil && s.cfg != nil {
		return claude.NormalizeMimicMode(s.cfg.Gateway.ClaudeCodeMimicMode)
	}
	return claude.MimicModeCompatibility
}
