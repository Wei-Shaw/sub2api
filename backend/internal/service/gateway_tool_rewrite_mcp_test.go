package service

import (
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBuildMCPToolMap_BoundariesAndStableCollisions(t *testing.T) {
	names := []string{"read", "read", "mcp__tools__read", "a/name", "a name", "a_name", "工具", "___", "", strings.Repeat("x", 52), strings.Repeat("x", 53), "mcp__invalid", "mcp____read"}
	mapping := buildMCPToolMap(names, "tools")
	reordered := make([]string, len(names))
	for i := range names {
		reordered[len(names)-1-i] = names[i]
	}
	require.Equal(t, mapping, buildMCPToolMap(reordered, "tools"))
	require.NotContains(t, mapping, "")
	require.NotContains(t, mapping, "mcp__tools__read")
	require.Equal(t, "mcp__tools__"+strings.Repeat("x", 52), mapping[strings.Repeat("x", 52)])
	require.Regexp(t, `^mcp__tools__x+_[0-9a-f]{8}$`, mapping[strings.Repeat("x", 53)])
	seen := map[string]bool{}
	for original, alias := range mapping {
		require.True(t, isMCPToolName(alias), alias)
		require.LessOrEqual(t, len(alias), 64)
		require.NotEqual(t, original, alias)
		require.NotContains(t, names, alias)
		require.False(t, seen[alias], alias)
		seen[alias] = true
	}
}

func TestBuildMCPToolMap_ReservesServerToolsAndExistingAliases(t *testing.T) {
	body := []byte(`{"tools":[{"name":"read","type":"function"},{"name":"write","type":"custom"},{"name":"mcp__tools__read","type":"computer_20250124"},{"name":"mcp__files__read"},{"name":"web_search","type":"web_search_20250305"}]}`)
	rw := buildTestToolNameRewrite(body)
	require.Len(t, rw.Forward, 2)
	require.NotEqual(t, "mcp__tools__read", rw.Forward["read"])
	out := applyToolNameRewriteToBody(body, rw)
	for _, path := range []string{"tools.2.name", "tools.3.name", "tools.4.name"} {
		require.Equal(t, gjson.GetBytes(body, path).Raw, gjson.GetBytes(out, path).Raw)
	}
}

func TestApplyToolNameRewriteToBody_ContextEditingExclusions(t *testing.T) {
	body := []byte(`{"tools":[{"name":"read_file"},{"type":"web_search_20250305","name":"web_search"}],"context_management":{"edits":[{"type":"clear_tool_uses_20250919","exclude_tools":["read_file","web_search","unknown",123],"trigger":{"type":"input_tokens","value":10000}},{"type":"unknown","exclude_tools":["read_file"]}]},"metadata":{"exclude_tools":["read_file"]}}`)
	rw := buildTestToolNameRewrite(body)
	out := applyToolNameRewriteToBody(body, rw)
	require.Equal(t, rw.Forward["read_file"], gjson.GetBytes(out, "context_management.edits.0.exclude_tools.0").String())
	for _, path := range []string{"context_management.edits.0.exclude_tools.1", "context_management.edits.0.exclude_tools.2", "context_management.edits.0.exclude_tools.3", "context_management.edits.0.trigger", "context_management.edits.1", "metadata"} {
		require.Equal(t, gjson.GetBytes(body, path).Raw, gjson.GetBytes(out, path).Raw, path)
	}
}

func TestBuildMCPToolMap_RejectsInvalidNamespace(t *testing.T) {
	for _, server := range []string{"", "not valid", strings.Repeat("x", 60)} {
		require.Empty(t, buildMCPToolMap([]string{"read"}, server))
	}
}

func TestBuildToolNameRewriteFromBody_UsesPersistentNamespace(t *testing.T) {
	body := []byte(`{"tools":[{"name":"read"}]}`)
	newGateway := func(name string) *GatewayService {
		return &GatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{AnthropicMCPServerName: name}}}
	}
	first := newGateway("helper")
	restarted := newGateway("helper")
	otherDeployment := newGateway("project")
	require.Equal(t, "mcp__helper__read", first.buildToolNameRewriteFromBody(body).Forward["read"])
	require.Equal(t, first.buildToolNameRewriteFromBody(body), restarted.buildToolNameRewriteFromBody(body))
	require.Equal(t, "mcp__project__read", otherDeployment.buildToolNameRewriteFromBody(body).Forward["read"])
	require.Nil(t, (&GatewayService{}).buildToolNameRewriteFromBody(body))
	require.Nil(t, newGateway("").buildToolNameRewriteFromBody(body), "never draw a process-local fallback")

	var wg sync.WaitGroup
	names := make([]string, 32)
	for i := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			names[i] = first.buildToolNameRewriteFromBody(body).Forward["read"]
		}()
	}
	wg.Wait()
	for _, name := range names {
		require.Equal(t, "mcp__helper__read", name)
	}
}
