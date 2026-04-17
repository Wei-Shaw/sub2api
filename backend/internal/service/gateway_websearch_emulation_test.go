//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/websearch"
	"github.com/stretchr/testify/require"
)

// --- isOnlyWebSearchToolInBody ---

func TestIsOnlyWebSearchToolInBody_WebSearchType(t *testing.T) {
	require.True(t, isOnlyWebSearchToolInBody([]byte(`{"tools":[{"type":"web_search"}]}`)))
}

func TestIsOnlyWebSearchToolInBody_WebSearch2025Type(t *testing.T) {
	require.True(t, isOnlyWebSearchToolInBody([]byte(`{"tools":[{"type":"web_search_20250305"}]}`)))
}

func TestIsOnlyWebSearchToolInBody_GoogleSearchType(t *testing.T) {
	require.True(t, isOnlyWebSearchToolInBody([]byte(`{"tools":[{"type":"google_search"}]}`)))
}

func TestIsOnlyWebSearchToolInBody_NameWebSearch(t *testing.T) {
	require.True(t, isOnlyWebSearchToolInBody([]byte(`{"tools":[{"name":"web_search"}]}`)))
}

func TestIsOnlyWebSearchToolInBody_NameWebSearch2025(t *testing.T) {
	require.True(t, isOnlyWebSearchToolInBody([]byte(`{"tools":[{"name":"web_search_20250305"}]}`)))
}

func TestIsOnlyWebSearchToolInBody_NameGoogleSearch(t *testing.T) {
	require.True(t, isOnlyWebSearchToolInBody([]byte(`{"tools":[{"name":"google_search"}]}`)))
}

func TestIsOnlyWebSearchToolInBody_MultipleTools(t *testing.T) {
	require.False(t, isOnlyWebSearchToolInBody(
		[]byte(`{"tools":[{"type":"web_search"},{"type":"text_editor"}]}`)))
}

func TestIsOnlyWebSearchToolInBody_NoTools(t *testing.T) {
	require.False(t, isOnlyWebSearchToolInBody([]byte(`{"model":"claude-3"}`)))
}

func TestIsOnlyWebSearchToolInBody_EmptyToolsArray(t *testing.T) {
	require.False(t, isOnlyWebSearchToolInBody([]byte(`{"tools":[]}`)))
}

func TestIsOnlyWebSearchToolInBody_NonWebSearchTool(t *testing.T) {
	require.False(t, isOnlyWebSearchToolInBody([]byte(`{"tools":[{"type":"text_editor"}]}`)))
}

func TestIsOnlyWebSearchToolInBody_ToolsNotArray(t *testing.T) {
	require.False(t, isOnlyWebSearchToolInBody([]byte(`{"tools":"web_search"}`)))
}

// --- extractSearchQueryFromBody ---

func TestExtractSearchQueryFromBody_StringContent(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"what is golang"}]}`
	require.Equal(t, "what is golang", extractSearchQueryFromBody([]byte(body)))
}

func TestExtractSearchQueryFromBody_ArrayContent(t *testing.T) {
	body := `{"messages":[{"role":"user","content":[{"type":"text","text":"search this"}]}]}`
	require.Equal(t, "search this", extractSearchQueryFromBody([]byte(body)))
}

func TestExtractSearchQueryFromBody_MultipleMessages(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"first"},{"role":"assistant","content":"ok"},{"role":"user","content":"second"}]}`
	require.Equal(t, "second", extractSearchQueryFromBody([]byte(body)))
}

func TestExtractSearchQueryFromBody_LastMessageNotUser(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"q"},{"role":"assistant","content":"a"}]}`
	require.Equal(t, "", extractSearchQueryFromBody([]byte(body)))
}

func TestExtractSearchQueryFromBody_EmptyMessages(t *testing.T) {
	require.Equal(t, "", extractSearchQueryFromBody([]byte(`{"messages":[]}`)))
}

func TestExtractSearchQueryFromBody_NoMessages(t *testing.T) {
	require.Equal(t, "", extractSearchQueryFromBody([]byte(`{"model":"claude-3"}`)))
}

func TestExtractSearchQueryFromBody_ArrayContentSkipsEmptyText(t *testing.T) {
	body := `{"messages":[{"role":"user","content":[{"type":"image"},{"type":"text","text":""},{"type":"text","text":"real query"}]}]}`
	require.Equal(t, "real query", extractSearchQueryFromBody([]byte(body)))
}

func TestExtractSearchQueryFromBody_ArrayContentNoTextBlock(t *testing.T) {
	body := `{"messages":[{"role":"user","content":[{"type":"image","source":{}}]}]}`
	require.Equal(t, "", extractSearchQueryFromBody([]byte(body)))
}

// --- buildSearchResultBlocks ---

func TestBuildSearchResultBlocks_WithResults(t *testing.T) {
	results := []websearch.SearchResult{
		{URL: "https://a.com", Title: "A", Snippet: "snippet a", PageAge: "2 days"},
		{URL: "https://b.com", Title: "B", Snippet: "snippet b"},
	}
	blocks := buildSearchResultBlocks(results)
	require.Len(t, blocks, 2)
	require.Equal(t, "web_search_result", blocks[0]["type"])
	require.Equal(t, "https://a.com", blocks[0]["url"])
	require.Equal(t, "snippet a", blocks[0]["page_content"])
	require.Equal(t, "2 days", blocks[0]["page_age"])
	// Second result has no PageAge
	require.Equal(t, "https://b.com", blocks[1]["url"])
	_, hasPageAge := blocks[1]["page_age"]
	require.False(t, hasPageAge)
}

func TestBuildSearchResultBlocks_Empty(t *testing.T) {
	blocks := buildSearchResultBlocks(nil)
	require.Empty(t, blocks)
}

func TestBuildSearchResultBlocks_SnippetEmpty(t *testing.T) {
	blocks := buildSearchResultBlocks([]websearch.SearchResult{{URL: "https://x.com", Title: "X", Snippet: ""}})
	_, hasContent := blocks[0]["page_content"]
	require.False(t, hasContent)
}

// --- buildTextSummary ---

func TestBuildTextSummary_WithResults(t *testing.T) {
	results := []websearch.SearchResult{
		{URL: "https://a.com", Title: "A", Snippet: "desc a"},
	}
	summary := buildTextSummary("test query", results)
	require.Contains(t, summary, "test query")
	require.Contains(t, summary, "1. **A**")
	require.Contains(t, summary, "https://a.com")
}

func TestBuildTextSummary_NoResults(t *testing.T) {
	summary := buildTextSummary("test", nil)
	require.Contains(t, summary, "No search results found for: test")
}

// --- shouldEmulateWebSearch ---

// webSearchToolBody is a valid request body with exactly one web_search tool.
var webSearchToolBody = []byte(`{"tools":[{"type":"web_search"}],"messages":[{"role":"user","content":"test"}]}`)

// nonWebSearchToolBody is a request body without web_search tool.
var nonWebSearchToolBody = []byte(`{"tools":[{"type":"text_editor"}],"messages":[{"role":"user","content":"test"}]}`)

// newAnthropicAPIKeyAccount creates a test Account with the given web search emulation mode.
func newAnthropicAPIKeyAccount(mode string) *Account {
	return &Account{
		ID:       1,
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyWebSearchEmulation: mode},
	}
}

// setGlobalWebSearchConfig stores a config in the global cache used by SettingService.IsWebSearchEmulationEnabled.
func setGlobalWebSearchConfig(cfg *WebSearchEmulationConfig) {
	webSearchEmulationCache.Store(&cachedWebSearchEmulationConfig{
		config:    cfg,
		expiresAt: time.Now().Add(10 * time.Minute).UnixNano(),
	})
}

// clearGlobalWebSearchConfig resets the global cache to force re-read.
func clearGlobalWebSearchConfig() {
	webSearchEmulationCache.Store((*cachedWebSearchEmulationConfig)(nil))
}

// newSettingServiceForWebSearchTest creates a SettingService with a mock repo pre-loaded with config.
func newSettingServiceForWebSearchTest(enabled bool) *SettingService {
	repo := newMockSettingRepo()
	cfg := &WebSearchEmulationConfig{
		Enabled:   enabled,
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "sk-test"}},
	}
	data, _ := json.Marshal(cfg)
	repo.data[SettingKeyWebSearchEmulationConfig] = string(data)
	return NewSettingService(repo, &config.Config{})
}

// newChannelServiceWithCache creates a ChannelService with a pre-built cache containing the channel.
func newChannelServiceWithCache(groupID int64, ch *Channel) *ChannelService {
	svc := &ChannelService{}
	cache := &channelCache{
		channelByGroupID: map[int64]*Channel{groupID: ch},
		byID:             map[int64]*Channel{ch.ID: ch},
		groupPlatform:    map[int64]string{},
		loadedAt:         time.Now(),
	}
	svc.cache.Store(cache)
	return svc
}

func TestShouldEmulateWebSearch_NilManager(t *testing.T) {
	SetWebSearchManager(nil)
	defer SetWebSearchManager(nil)

	settingSvc := newSettingServiceForWebSearchTest(true)
	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   true,
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "k"}},
	})
	defer clearGlobalWebSearchConfig()

	svc := &GatewayService{settingService: settingSvc}
	account := newAnthropicAPIKeyAccount(WebSearchModeEnabled)
	require.False(t, svc.shouldEmulateWebSearch(context.Background(), account, nil, webSearchToolBody))
}

func TestShouldEmulateWebSearch_NotOnlyWebSearchTool(t *testing.T) {
	mgr := websearch.NewManager([]websearch.ProviderConfig{{Type: "brave", APIKey: "k"}}, nil)
	SetWebSearchManager(mgr)
	defer SetWebSearchManager(nil)

	settingSvc := newSettingServiceForWebSearchTest(true)
	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   true,
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "k"}},
	})
	defer clearGlobalWebSearchConfig()

	svc := &GatewayService{settingService: settingSvc}
	account := newAnthropicAPIKeyAccount(WebSearchModeEnabled)
	require.False(t, svc.shouldEmulateWebSearch(context.Background(), account, nil, nonWebSearchToolBody))
}

func TestShouldEmulateWebSearch_GlobalDisabled(t *testing.T) {
	mgr := websearch.NewManager([]websearch.ProviderConfig{{Type: "brave", APIKey: "k"}}, nil)
	SetWebSearchManager(mgr)
	defer SetWebSearchManager(nil)

	// Global config disabled
	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   false,
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "k"}},
	})
	defer clearGlobalWebSearchConfig()

	settingSvc := newSettingServiceForWebSearchTest(false)
	svc := &GatewayService{settingService: settingSvc}
	account := newAnthropicAPIKeyAccount(WebSearchModeEnabled)
	require.False(t, svc.shouldEmulateWebSearch(context.Background(), account, nil, webSearchToolBody))
}

func TestShouldEmulateWebSearch_AccountDisabled(t *testing.T) {
	mgr := websearch.NewManager([]websearch.ProviderConfig{{Type: "brave", APIKey: "k"}}, nil)
	SetWebSearchManager(mgr)
	defer SetWebSearchManager(nil)

	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   true,
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "k"}},
	})
	defer clearGlobalWebSearchConfig()

	settingSvc := newSettingServiceForWebSearchTest(true)
	svc := &GatewayService{settingService: settingSvc}
	account := newAnthropicAPIKeyAccount(WebSearchModeDisabled)
	require.False(t, svc.shouldEmulateWebSearch(context.Background(), account, nil, webSearchToolBody))
}

func TestShouldEmulateWebSearch_AccountEnabled(t *testing.T) {
	mgr := websearch.NewManager([]websearch.ProviderConfig{{Type: "brave", APIKey: "k"}}, nil)
	SetWebSearchManager(mgr)
	defer SetWebSearchManager(nil)

	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   true,
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "k"}},
	})
	defer clearGlobalWebSearchConfig()

	settingSvc := newSettingServiceForWebSearchTest(true)
	svc := &GatewayService{settingService: settingSvc}
	account := newAnthropicAPIKeyAccount(WebSearchModeEnabled)
	require.True(t, svc.shouldEmulateWebSearch(context.Background(), account, nil, webSearchToolBody))
}

func TestShouldEmulateWebSearch_DefaultMode_ChannelEnabled(t *testing.T) {
	mgr := websearch.NewManager([]websearch.ProviderConfig{{Type: "brave", APIKey: "k"}}, nil)
	SetWebSearchManager(mgr)
	defer SetWebSearchManager(nil)

	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   true,
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "k"}},
	})
	defer clearGlobalWebSearchConfig()

	settingSvc := newSettingServiceForWebSearchTest(true)
	ch := &Channel{
		ID:     10,
		Status: StatusActive,
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{PlatformAnthropic: true},
		},
	}
	channelSvc := newChannelServiceWithCache(42, ch)
	svc := &GatewayService{settingService: settingSvc, channelService: channelSvc}

	account := newAnthropicAPIKeyAccount(WebSearchModeDefault)
	groupID := int64(42)
	require.True(t, svc.shouldEmulateWebSearch(context.Background(), account, &groupID, webSearchToolBody))
}

func TestShouldEmulateWebSearch_DefaultMode_ChannelDisabled(t *testing.T) {
	mgr := websearch.NewManager([]websearch.ProviderConfig{{Type: "brave", APIKey: "k"}}, nil)
	SetWebSearchManager(mgr)
	defer SetWebSearchManager(nil)

	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   true,
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "k"}},
	})
	defer clearGlobalWebSearchConfig()

	settingSvc := newSettingServiceForWebSearchTest(true)
	ch := &Channel{
		ID:     10,
		Status: StatusActive,
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{PlatformAnthropic: false},
		},
	}
	channelSvc := newChannelServiceWithCache(42, ch)
	svc := &GatewayService{settingService: settingSvc, channelService: channelSvc}

	account := newAnthropicAPIKeyAccount(WebSearchModeDefault)
	groupID := int64(42)
	require.False(t, svc.shouldEmulateWebSearch(context.Background(), account, &groupID, webSearchToolBody))
}

func TestShouldEmulateWebSearch_DefaultMode_NilGroupID(t *testing.T) {
	mgr := websearch.NewManager([]websearch.ProviderConfig{{Type: "brave", APIKey: "k"}}, nil)
	SetWebSearchManager(mgr)
	defer SetWebSearchManager(nil)

	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   true,
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "k"}},
	})
	defer clearGlobalWebSearchConfig()

	settingSvc := newSettingServiceForWebSearchTest(true)
	svc := &GatewayService{settingService: settingSvc}
	account := newAnthropicAPIKeyAccount(WebSearchModeDefault)
	// nil groupID + default mode → falls through to channel check → returns false
	require.False(t, svc.shouldEmulateWebSearch(context.Background(), account, nil, webSearchToolBody))
}

func TestShouldEmulateWebSearch_DefaultMode_NilChannelService(t *testing.T) {
	mgr := websearch.NewManager([]websearch.ProviderConfig{{Type: "brave", APIKey: "k"}}, nil)
	SetWebSearchManager(mgr)
	defer SetWebSearchManager(nil)

	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   true,
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "k"}},
	})
	defer clearGlobalWebSearchConfig()

	settingSvc := newSettingServiceForWebSearchTest(true)
	svc := &GatewayService{settingService: settingSvc, channelService: nil}
	account := newAnthropicAPIKeyAccount(WebSearchModeDefault)
	groupID := int64(42)
	// nil channelService + default mode → returns false
	require.False(t, svc.shouldEmulateWebSearch(context.Background(), account, &groupID, webSearchToolBody))
}
