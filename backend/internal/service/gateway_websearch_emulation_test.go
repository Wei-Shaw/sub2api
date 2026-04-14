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
	require.True(t, isOnlyWebSearchToolInBody([]byte(`{"tools":[{"type":"web_search"REDACTED]REDACTED`)))
REDACTED

func TestIsOnlyWebSearchToolInBody_WebSearch2025Type(t *testing.T) {
	require.True(t, isOnlyWebSearchToolInBody([]byte(`{"tools":[{"type":"web_search_20250305"REDACTED]REDACTED`)))
REDACTED

func TestIsOnlyWebSearchToolInBody_GoogleSearchType(t *testing.T) {
	require.True(t, isOnlyWebSearchToolInBody([]byte(`{"tools":[{"type":"google_search"REDACTED]REDACTED`)))
REDACTED

func TestIsOnlyWebSearchToolInBody_NameWebSearch(t *testing.T) {
	require.True(t, isOnlyWebSearchToolInBody([]byte(`{"tools":[{"name":"web_search"REDACTED]REDACTED`)))
REDACTED

func TestIsOnlyWebSearchToolInBody_NameWebSearch2025(t *testing.T) {
	require.True(t, isOnlyWebSearchToolInBody([]byte(`{"tools":[{"name":"web_search_20250305"REDACTED]REDACTED`)))
REDACTED

func TestIsOnlyWebSearchToolInBody_NameGoogleSearch(t *testing.T) {
	require.True(t, isOnlyWebSearchToolInBody([]byte(`{"tools":[{"name":"google_search"REDACTED]REDACTED`)))
REDACTED

func TestIsOnlyWebSearchToolInBody_MultipleTools(t *testing.T) {
	require.False(t, isOnlyWebSearchToolInBody(
		[]byte(`{"tools":[{"type":"web_search"REDACTED,{"type":"text_editor"REDACTED]REDACTED`)))
REDACTED

func TestIsOnlyWebSearchToolInBody_NoTools(t *testing.T) {
	require.False(t, isOnlyWebSearchToolInBody([]byte(`{"model":"claude-3"REDACTED`)))
REDACTED

func TestIsOnlyWebSearchToolInBody_EmptyToolsArray(t *testing.T) {
	require.False(t, isOnlyWebSearchToolInBody([]byte(`{"tools":[]REDACTED`)))
REDACTED

func TestIsOnlyWebSearchToolInBody_NonWebSearchTool(t *testing.T) {
	require.False(t, isOnlyWebSearchToolInBody([]byte(`{"tools":[{"type":"text_editor"REDACTED]REDACTED`)))
REDACTED

func TestIsOnlyWebSearchToolInBody_ToolsNotArray(t *testing.T) {
	require.False(t, isOnlyWebSearchToolInBody([]byte(`{"tools":"web_search"REDACTED`)))
REDACTED

// --- extractSearchQueryFromBody ---

func TestExtractSearchQueryFromBody_StringContent(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"what is golang"REDACTED]REDACTED`
	require.Equal(t, "what is golang", extractSearchQueryFromBody([]byte(body)))
REDACTED

func TestExtractSearchQueryFromBody_ArrayContent(t *testing.T) {
	body := `{"messages":[{"role":"user","content":[{"type":"text","text":"search this"REDACTED]REDACTED]REDACTED`
	require.Equal(t, "search this", extractSearchQueryFromBody([]byte(body)))
REDACTED

func TestExtractSearchQueryFromBody_MultipleMessages(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"first"REDACTED,{"role":"assistant","content":"ok"REDACTED,{"role":"user","content":"second"REDACTED]REDACTED`
	require.Equal(t, "second", extractSearchQueryFromBody([]byte(body)))
REDACTED

func TestExtractSearchQueryFromBody_LastMessageNotUser(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"q"REDACTED,{"role":"assistant","content":"a"REDACTED]REDACTED`
	require.Equal(t, "", extractSearchQueryFromBody([]byte(body)))
REDACTED

func TestExtractSearchQueryFromBody_EmptyMessages(t *testing.T) {
	require.Equal(t, "", extractSearchQueryFromBody([]byte(`{"messages":[]REDACTED`)))
REDACTED

func TestExtractSearchQueryFromBody_NoMessages(t *testing.T) {
	require.Equal(t, "", extractSearchQueryFromBody([]byte(`{"model":"claude-3"REDACTED`)))
REDACTED

func TestExtractSearchQueryFromBody_ArrayContentSkipsEmptyText(t *testing.T) {
	body := `{"messages":[{"role":"user","content":[{"type":"image"REDACTED,{"type":"text","text":""REDACTED,{"type":"text","text":"real query"REDACTED]REDACTED]REDACTED`
	require.Equal(t, "real query", extractSearchQueryFromBody([]byte(body)))
REDACTED

func TestExtractSearchQueryFromBody_ArrayContentNoTextBlock(t *testing.T) {
	body := `{"messages":[{"role":"user","content":[{"type":"image","source":{REDACTEDREDACTED]REDACTED]REDACTED`
	require.Equal(t, "", extractSearchQueryFromBody([]byte(body)))
REDACTED

// --- buildSearchResultBlocks ---

func TestBuildSearchResultBlocks_WithResults(t *testing.T) {
	results := []websearch.SearchResult{
		{URL: "https://a.com", Title: "A", Snippet: "snippet a", PageAge: "2 days"REDACTED,
		{URL: "https://b.com", Title: "B", Snippet: "snippet b"REDACTED,
REDACTED
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
REDACTED

func TestBuildSearchResultBlocks_Empty(t *testing.T) {
	blocks := buildSearchResultBlocks(nil)
	require.Empty(t, blocks)
REDACTED

func TestBuildSearchResultBlocks_SnippetEmpty(t *testing.T) {
	blocks := buildSearchResultBlocks([]websearch.SearchResult{{URL: "https://x.com", Title: "X", Snippet: ""REDACTEDREDACTED)
	_, hasContent := blocks[0]["page_content"]
	require.False(t, hasContent)
REDACTED

// --- buildTextSummary ---

func TestBuildTextSummary_WithResults(t *testing.T) {
	results := []websearch.SearchResult{
		{URL: "https://a.com", Title: "A", Snippet: "desc a"REDACTED,
REDACTED
	summary := buildTextSummary("test query", results)
	require.Contains(t, summary, "test query")
	require.Contains(t, summary, "1. **A**")
	require.Contains(t, summary, "https://a.com")
REDACTED

func TestBuildTextSummary_NoResults(t *testing.T) {
	summary := buildTextSummary("test", nil)
	require.Contains(t, summary, "No search results found for: test")
REDACTED

// --- shouldEmulateWebSearch ---

// webSearchToolBody is a valid request body with exactly one web_search tool.
var webSearchToolBody = []byte(`{"tools":[{"type":"web_search"REDACTED],"messages":[{"role":"user","content":"test"REDACTED]REDACTED`)

// nonWebSearchToolBody is a request body without web_search tool.
var nonWebSearchToolBody = []byte(`{"tools":[{"type":"text_editor"REDACTED],"messages":[{"role":"user","content":"test"REDACTED]REDACTED`)

// newAnthropicAPIKeyAccount creates a test Account with the given web search emulation mode.
func newAnthropicAPIKeyAccount(mode string) *Account {
REDACTED
		ID:       1,
		Platform: PlatformAnthropic,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{featureKeyWebSearchEmulation: modeREDACTED,
REDACTED
REDACTED

// setGlobalWebSearchConfig stores a config in the global cache used by SettingService.IsWebSearchEmulationEnabled.
func setGlobalWebSearchConfig(cfg *WebSearchEmulationConfig) {
	webSearchEmulationCache.Store(&cachedWebSearchEmulationConfig{
		config:    cfg,
		expiresAt: time.Now().Add(10 * time.Minute).UnixNano(),
REDACTED)
REDACTED

// clearGlobalWebSearchConfig resets the global cache to force re-read.
func clearGlobalWebSearchConfig() {
	webSearchEmulationCache.Store((*cachedWebSearchEmulationConfig)(nil))
REDACTED

// newSettingServiceForWebSearchTest creates a SettingService with a mock repo pre-loaded with config.
func newSettingServiceForWebSearchTest(enabled bool) *SettingService {
	repo := newMockSettingRepo()
	cfg := &WebSearchEmulationConfig{
		Enabled:   enabled,
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "sk-test"REDACTEDREDACTED,
REDACTED
	data, _ := json.Marshal(cfg)
	repo.data[SettingKeyWebSearchEmulationConfig] = string(data)
	return NewSettingService(repo, &config.Config{REDACTED)
REDACTED

// newChannelServiceWithCache creates a ChannelService with a pre-built cache containing the channel.
func newChannelServiceWithCache(groupID int64, ch *Channel) *ChannelService {
	svc := &ChannelService{REDACTED
	cache := &channelCache{
		channelByGroupID: map[int64]*Channel{groupID: chREDACTED,
		byID:             map[int64]*Channel{ch.ID: chREDACTED,
		groupPlatform:    map[int64]string{REDACTED,
		loadedAt:         time.Now(),
REDACTED
	svc.cache.Store(cache)
	return svc
REDACTED

func TestShouldEmulateWebSearch_NilManager(t *testing.T) {
	SetWebSearchManager(nil)
	defer SetWebSearchManager(nil)

	settingSvc := newSettingServiceForWebSearchTest(true)
	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   true,
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "k"REDACTEDREDACTED,
REDACTED)
	defer clearGlobalWebSearchConfig()

	svc := &GatewayService{settingService: settingSvcREDACTED
	account := newAnthropicAPIKeyAccount(WebSearchModeEnabled)
	require.False(t, svc.shouldEmulateWebSearch(context.Background(), account, nil, webSearchToolBody))
REDACTED

func TestShouldEmulateWebSearch_NotOnlyWebSearchTool(t *testing.T) {
	mgr := websearch.NewManager([]websearch.ProviderConfig{{Type: "brave", APIKey: "k"REDACTEDREDACTED, nil)
	SetWebSearchManager(mgr)
	defer SetWebSearchManager(nil)

	settingSvc := newSettingServiceForWebSearchTest(true)
	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   true,
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "k"REDACTEDREDACTED,
REDACTED)
	defer clearGlobalWebSearchConfig()

	svc := &GatewayService{settingService: settingSvcREDACTED
	account := newAnthropicAPIKeyAccount(WebSearchModeEnabled)
	require.False(t, svc.shouldEmulateWebSearch(context.Background(), account, nil, nonWebSearchToolBody))
REDACTED

func TestShouldEmulateWebSearch_GlobalDisabled(t *testing.T) {
	mgr := websearch.NewManager([]websearch.ProviderConfig{{Type: "brave", APIKey: "k"REDACTEDREDACTED, nil)
	SetWebSearchManager(mgr)
	defer SetWebSearchManager(nil)

	// Global config disabled
	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   false,
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "k"REDACTEDREDACTED,
REDACTED)
	defer clearGlobalWebSearchConfig()

	settingSvc := newSettingServiceForWebSearchTest(false)
	svc := &GatewayService{settingService: settingSvcREDACTED
	account := newAnthropicAPIKeyAccount(WebSearchModeEnabled)
	require.False(t, svc.shouldEmulateWebSearch(context.Background(), account, nil, webSearchToolBody))
REDACTED

func TestShouldEmulateWebSearch_AccountDisabled(t *testing.T) {
	mgr := websearch.NewManager([]websearch.ProviderConfig{{Type: "brave", APIKey: "k"REDACTEDREDACTED, nil)
	SetWebSearchManager(mgr)
	defer SetWebSearchManager(nil)

	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   true,
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "k"REDACTEDREDACTED,
REDACTED)
	defer clearGlobalWebSearchConfig()

	settingSvc := newSettingServiceForWebSearchTest(true)
	svc := &GatewayService{settingService: settingSvcREDACTED
	account := newAnthropicAPIKeyAccount(WebSearchModeDisabled)
	require.False(t, svc.shouldEmulateWebSearch(context.Background(), account, nil, webSearchToolBody))
REDACTED

func TestShouldEmulateWebSearch_AccountEnabled(t *testing.T) {
	mgr := websearch.NewManager([]websearch.ProviderConfig{{Type: "brave", APIKey: "k"REDACTEDREDACTED, nil)
	SetWebSearchManager(mgr)
	defer SetWebSearchManager(nil)

	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   true,
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "k"REDACTEDREDACTED,
REDACTED)
	defer clearGlobalWebSearchConfig()

	settingSvc := newSettingServiceForWebSearchTest(true)
	svc := &GatewayService{settingService: settingSvcREDACTED
	account := newAnthropicAPIKeyAccount(WebSearchModeEnabled)
	require.True(t, svc.shouldEmulateWebSearch(context.Background(), account, nil, webSearchToolBody))
REDACTED

func TestShouldEmulateWebSearch_DefaultMode_ChannelEnabled(t *testing.T) {
	mgr := websearch.NewManager([]websearch.ProviderConfig{{Type: "brave", APIKey: "k"REDACTEDREDACTED, nil)
	SetWebSearchManager(mgr)
	defer SetWebSearchManager(nil)

	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   true,
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "k"REDACTEDREDACTED,
REDACTED)
	defer clearGlobalWebSearchConfig()

	settingSvc := newSettingServiceForWebSearchTest(true)
	ch := &Channel{
		ID:     10,
		Status: StatusActive,
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{PlatformAnthropic: trueREDACTED,
	REDACTED,
REDACTED
	channelSvc := newChannelServiceWithCache(42, ch)
	svc := &GatewayService{settingService: settingSvc, channelService: channelSvcREDACTED

	account := newAnthropicAPIKeyAccount(WebSearchModeDefault)
	groupID := int64(42)
	require.True(t, svc.shouldEmulateWebSearch(context.Background(), account, &groupID, webSearchToolBody))
REDACTED

func TestShouldEmulateWebSearch_DefaultMode_ChannelDisabled(t *testing.T) {
	mgr := websearch.NewManager([]websearch.ProviderConfig{{Type: "brave", APIKey: "k"REDACTEDREDACTED, nil)
	SetWebSearchManager(mgr)
	defer SetWebSearchManager(nil)

	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   true,
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "k"REDACTEDREDACTED,
REDACTED)
	defer clearGlobalWebSearchConfig()

	settingSvc := newSettingServiceForWebSearchTest(true)
	ch := &Channel{
		ID:     10,
		Status: StatusActive,
		FeaturesConfig: map[string]any{
			featureKeyWebSearchEmulation: map[string]any{PlatformAnthropic: falseREDACTED,
	REDACTED,
REDACTED
	channelSvc := newChannelServiceWithCache(42, ch)
	svc := &GatewayService{settingService: settingSvc, channelService: channelSvcREDACTED

	account := newAnthropicAPIKeyAccount(WebSearchModeDefault)
	groupID := int64(42)
	require.False(t, svc.shouldEmulateWebSearch(context.Background(), account, &groupID, webSearchToolBody))
REDACTED

func TestShouldEmulateWebSearch_DefaultMode_NilGroupID(t *testing.T) {
	mgr := websearch.NewManager([]websearch.ProviderConfig{{Type: "brave", APIKey: "k"REDACTEDREDACTED, nil)
	SetWebSearchManager(mgr)
	defer SetWebSearchManager(nil)

	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   true,
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "k"REDACTEDREDACTED,
REDACTED)
	defer clearGlobalWebSearchConfig()

	settingSvc := newSettingServiceForWebSearchTest(true)
	svc := &GatewayService{settingService: settingSvcREDACTED
	account := newAnthropicAPIKeyAccount(WebSearchModeDefault)
	// nil groupID + default mode → falls through to channel check → returns false
	require.False(t, svc.shouldEmulateWebSearch(context.Background(), account, nil, webSearchToolBody))
REDACTED

func TestShouldEmulateWebSearch_DefaultMode_NilChannelService(t *testing.T) {
	mgr := websearch.NewManager([]websearch.ProviderConfig{{Type: "brave", APIKey: "k"REDACTEDREDACTED, nil)
	SetWebSearchManager(mgr)
	defer SetWebSearchManager(nil)

	setGlobalWebSearchConfig(&WebSearchEmulationConfig{
		Enabled:   true,
		Providers: []WebSearchProviderConfig{{Type: "brave", APIKey: "k"REDACTEDREDACTED,
REDACTED)
	defer clearGlobalWebSearchConfig()

	settingSvc := newSettingServiceForWebSearchTest(true)
	svc := &GatewayService{settingService: settingSvc, channelService: nilREDACTED
	account := newAnthropicAPIKeyAccount(WebSearchModeDefault)
	groupID := int64(42)
	// nil channelService + default mode → returns false
	require.False(t, svc.shouldEmulateWebSearch(context.Background(), account, &groupID, webSearchToolBody))
REDACTED
