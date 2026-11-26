package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestGetClaudeTransformOptions_GroupMCPXMLInject(t *testing.T) {
	svc := &AntigravityGatewayService{}

	t.Run("defaults to enabled when group is absent", func(t *testing.T) {
		opts := svc.getClaudeTransformOptions(context.Background())
		require.True(t, opts.EnableMCPXML)
	})

	t.Run("defaults to enabled when group is typed nil", func(t *testing.T) {
		var nilGroup *Group
		ctx := context.WithValue(context.Background(), ctxkey.Group, nilGroup)
		opts := svc.getClaudeTransformOptions(ctx)
		require.True(t, opts.EnableMCPXML)
	})

	t.Run("honors group MCPXMLInject true", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{MCPXMLInject: true})
		opts := svc.getClaudeTransformOptions(ctx)
		require.True(t, opts.EnableMCPXML)
	})

	t.Run("honors group MCPXMLInject false even when settingService is nil", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ctxkey.Group, &Group{MCPXMLInject: false})
		opts := svc.getClaudeTransformOptions(ctx)
		require.False(t, opts.EnableMCPXML)

		req := &antigravity.ClaudeRequest{
			Model: "gemini-3-pro",
			Messages: []antigravity.ClaudeMessage{
				{Role: "user", Content: json.RawMessage(`"hi"`)},
			},
			Tools: []antigravity.ClaudeTool{
				{Name: "mcp__demo__ping", Description: "ping", InputSchema: map[string]any{"type": "object"}},
			},
		}
		wireDisabled, err := antigravity.TransformClaudeToGeminiWithOptions(req, "test-project", "gemini-3-pro", opts)
		require.NoError(t, err)
		require.NotContains(t, string(wireDisabled), "MCP XML 工具调用协议")

		ctxEnabled := context.WithValue(context.Background(), ctxkey.Group, &Group{MCPXMLInject: true})
		wireEnabled, err := antigravity.TransformClaudeToGeminiWithOptions(req, "test-project", "gemini-3-pro", svc.getClaudeTransformOptions(ctxEnabled))
		require.NoError(t, err)
		require.Contains(t, string(wireEnabled), "MCP XML 工具调用协议")
	})
}
