package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestApplyCodexOAuthTransform_ToolContinuationPreservesInput(t *testing.T) {
	// 续链场景：保留 item_reference 与 id，但不再强制 store=true。
	setupCodexCache(t)

	reqBody := map[string]any{
		"model": "gpt-5.2",
		"input": []any{
			map[string]any{"type": "item_reference", "id": "ref1", "text": "x"REDACTED,
			map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "ok", "id": "o1"REDACTED,
	REDACTED,
		"tool_choice": "auto",
REDACTED

	applyCodexOAuthTransform(reqBody)

	// 未显式设置 store=true，默认为 false。
	store, ok := reqBody["store"].(bool)
	require.True(t, ok)
	require.False(t, store)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 2)

	// 校验 input[0] 为 map，避免断言失败导致测试中断。
	first, ok := input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "item_reference", first["type"])
	require.Equal(t, "ref1", first["id"])

	// 校验 input[1] 为 map，确保后续字段断言安全。
	second, ok := input[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "o1", second["id"])
REDACTED

func TestApplyCodexOAuthTransform_ExplicitStoreFalsePreserved(t *testing.T) {
	// 续链场景：显式 store=false 不再强制为 true，保持 false。
	setupCodexCache(t)

	reqBody := map[string]any{
		"model": "gpt-5.1",
		"store": false,
		"input": []any{
			map[string]any{"type": "function_call_output", "call_id": "call_1"REDACTED,
	REDACTED,
		"tool_choice": "auto",
REDACTED

	applyCodexOAuthTransform(reqBody)

	store, ok := reqBody["store"].(bool)
	require.True(t, ok)
	require.False(t, store)
REDACTED

func TestApplyCodexOAuthTransform_ExplicitStoreTrueForcedFalse(t *testing.T) {
	// 显式 store=true 也会强制为 false。
	setupCodexCache(t)

	reqBody := map[string]any{
		"model": "gpt-5.1",
		"store": true,
		"input": []any{
			map[string]any{"type": "function_call_output", "call_id": "call_1"REDACTED,
	REDACTED,
		"tool_choice": "auto",
REDACTED

	applyCodexOAuthTransform(reqBody)

	store, ok := reqBody["store"].(bool)
	require.True(t, ok)
	require.False(t, store)
REDACTED

func TestApplyCodexOAuthTransform_NonContinuationDefaultsStoreFalseAndStripsIDs(t *testing.T) {
	// 非续链场景：未设置 store 时默认 false，并移除 input 中的 id。
	setupCodexCache(t)

	reqBody := map[string]any{
		"model": "gpt-5.1",
		"input": []any{
			map[string]any{"type": "text", "id": "t1", "text": "hi"REDACTED,
	REDACTED,
REDACTED

	applyCodexOAuthTransform(reqBody)

	store, ok := reqBody["store"].(bool)
	require.True(t, ok)
	require.False(t, store)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 1)
	// 校验 input[0] 为 map，避免类型不匹配触发 errcheck。
	item, ok := input[0].(map[string]any)
	require.True(t, ok)
	_, hasID := item["id"]
	require.False(t, hasID)
REDACTED

func TestFilterCodexInput_RemovesItemReferenceWhenNotPreserved(t *testing.T) {
	input := []any{
		map[string]any{"type": "item_reference", "id": "ref1"REDACTED,
		map[string]any{"type": "text", "id": "t1", "text": "hi"REDACTED,
REDACTED

	filtered := filterCodexInput(input, false)
	require.Len(t, filtered, 1)
	// 校验 filtered[0] 为 map，确保字段检查可靠。
	item, ok := filtered[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "text", item["type"])
	_, hasID := item["id"]
	require.False(t, hasID)
REDACTED

func TestApplyCodexOAuthTransform_EmptyInput(t *testing.T) {
	// 空 input 应保持为空且不触发异常。
	setupCodexCache(t)

	reqBody := map[string]any{
		"model": "gpt-5.1",
		"input": []any{REDACTED,
REDACTED

	applyCodexOAuthTransform(reqBody)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 0)
REDACTED

func setupCodexCache(t *testing.T) {
REDACTED

	// 使用临时 HOME 避免触发网络拉取 header。
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	cacheDir := filepath.Join(tempDir, ".opencode", "cache")
	require.NoError(t, os.MkdirAll(cacheDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "opencode-codex-header.txt"), []byte("header"), 0o644))

	meta := map[string]any{
		"etag":        "",
		"lastFetch":   time.Now().UTC().Format(time.RFC3339),
		"lastChecked": time.Now().UnixMilli(),
REDACTED
	data, err := json.Marshal(meta)
REDACTED
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "opencode-codex-header-meta.json"), data, 0o644))
REDACTED
