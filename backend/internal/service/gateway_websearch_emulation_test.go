package service

import (
	"testing"

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
