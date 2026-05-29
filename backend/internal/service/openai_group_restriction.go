package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
)

func sanitizeGroupCodexOfficialRestrictionFields(g *Group) {
	if g == nil || g.Platform == PlatformOpenAI {
		return
	}
	g.CodexOfficialOnly = false
}

func (s *OpenAIGatewayService) DetectGroupCodexOfficialRestriction(c *gin.Context, group *Group) CodexClientRestrictionDetectionResult {
	if group == nil || group.Platform != PlatformOpenAI || !group.CodexOfficialOnly {
		return CodexClientRestrictionDetectionResult{
			Enabled: false,
			Matched: false,
			Reason:  CodexClientRestrictionReasonDisabled,
		}
	}
	return s.getCodexClientRestrictionDetector().DetectPolicy(c, true, nil, s.codexRestrictionGlobalAllowedClients(c))
}

func (s *OpenAIGatewayService) codexRestrictionGlobalAllowedClients(c *gin.Context) []string {
	if s == nil || s.settingService == nil {
		return nil
	}
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	if s.settingService.IsOpenAIAllowClaudeCodeCodexPluginEnabled(ctx) {
		return []string{openai.AllowedClientClaudeCode}
	}
	return nil
}
