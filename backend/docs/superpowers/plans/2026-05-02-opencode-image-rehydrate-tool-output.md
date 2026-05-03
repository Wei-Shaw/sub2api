# OpenCode 图片恢复工具输出化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 OpenCode 生成图片恢复从“尾部 synthetic user message”改成“原位置 synthetic tool output”，并避免历史 marker、压缩摘要、日志泄漏和 `-Sys` 尾部语义回归。

**Architecture:** 图片恢复在 `/v1/responses` 调度准备阶段完成，早于 target group 计算和 `-Sys` dummy pair 追加。恢复逻辑输出 synthetic `function_call` / `function_call_output` pair，marker 扫描使用高特异哨兵和机械 source 判定，所有 data URL 在 ops/request detail/runtime log 路径递归脱敏。

**Tech Stack:** Go 1.x, Gin, tidwall/gjson/sjson, existing sub2api OpenAI gateway service, existing Go unit tests with `C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe`.

---

## 前置约束

工作区：`C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\.worktrees\opencode-image-rehydrate-tool-output`

规格：`C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\.worktrees\opencode-image-rehydrate-tool-output\backend\docs\superpowers\specs\2026-05-02-opencode-image-rehydrate-tool-output-design.md`

本计划不要求自动提交。除非用户后续明确要求创建 git commit，否则所有实现子代理只能修改文件、运行测试并报告结果，不能提交。

基线说明：`go test ./internal/service ...` 在无 `-tags unit` 时当前基线会因既有测试 helper build tag 问题失败；已验证 `go test -tags unit ./internal/service -run "OpenCodeImage|GeneratedImage|ToolContinuation" -count=1` 可通过。实现任务中的 service 测试优先带 `-tags unit`。

## 文件结构

**Modify:** `backend/internal/service/openai_opencode_image_rehydrate.go`
- 负责 marker 扫描、source 判定、synthetic image tool pair 构造、原位置插入、不可用 TTL/LRU 去重。

**Modify:** `backend/internal/service/openai_opencode_image_rewrite.go`
- 负责新 marker 输出文本、生成图片 message 文案、服务端 continuation 输出形态复用。

**Modify:** `backend/internal/service/openai_opencode_image_ops_redaction.go`
- 负责新哨兵、旧 marker、下载 URL、`function_call_output.output[].image_url`、运行时日志文本中的图片数据脱敏。

**Modify:** `backend/internal/service/openai_gateway_service.go`
- 移除或跳过原 service 阶段 rehydrate 调用，确保不会在调度之后再改写 input。
- 为上游 array/string/auto 降级和 runtime log redaction 提供调用点。

**Modify:** `backend/internal/handler/openai_gateway_handler.go`
- 将 OpenCode image rehydrate 前移到调度准备阶段，缓存 `needsSysDummy`，再追加 sys dummy 并计算 target group。

**Modify:** `backend/internal/service/openai_opencode_image_rewrite_test.go`
- 覆盖 marker、rehydrate、插入位置、不可用去重、non-OpenCode 隔离相关 service tests。

**Modify:** `backend/internal/service/openai_gateway_service_test.go`
- 覆盖 transform 保真、fallback、gateway 层请求体和 non-OpenCode 行为。

**Modify:** `backend/internal/handler/openai_gateway_handler_test.go`
- 覆盖调度准备阶段、`needsSysDummy` 缓存、target group 与最终 tail 一致性。

**Modify:** ops 相关测试文件，优先复用现有文件
- `backend/internal/service/openai_opencode_image_rewrite_test.go`
- `backend/internal/service/openai_gateway_service_test.go`
- `backend/internal/handler/openai_gateway_handler_test.go`
- 如已有 ops logger 测试位置更合适，使用现有 `OpsErrorLogger` / request detail 测试文件。

---

### Task 1: 新 marker 与 redaction 基础

**Files:**
- Modify: `backend/internal/service/openai_opencode_image_rehydrate.go`
- Modify: `backend/internal/service/openai_opencode_image_rewrite.go`
- Modify: `backend/internal/service/openai_opencode_image_ops_redaction.go`
- Test: `backend/internal/service/openai_opencode_image_rewrite_test.go`

- [ ] **Step 1: 写失败测试，锁定新 marker 文案和 redaction**

在 `openai_opencode_image_rewrite_test.go` 增加测试：

```go
func TestOpenCodeGeneratedImageMessageUsesSpecificMarker(t *testing.T) {
	rec := OpenAIGeneratedImageRecord{ID: "img_abcdefghijklmnopqrstuvwxyzABCDEF", Filename: "img_abcdefghijklmnopqrstuvwxyzABCDEF.png"}
	msg := buildOpenCodeGeneratedImageMessage(rec, openCodeImageRewriteOptions{BaseURL: "https://example.com"})
	text := gjson.GetBytes(mustJSONBytes(t, msg), "content.0.text").String()
	require.Contains(t, text, "[[sub2api-generated-image:id=img_abcdefghijklmnopqrstuvwxyzABCDEF]]")
	require.NotContains(t, text, "sub2api-image://img_abcdefghijklmnopqrstuvwxyzABCDEF")
	require.Contains(t, text, "https://example.com/sub2api/generated-images/img_abcdefghijklmnopqrstuvwxyzABCDEF.png")
}

func TestRedactOpenCodeGeneratedImagesForOpsRedactsSpecificMarker(t *testing.T) {
	body := []byte(`{"input":[{"role":"assistant","content":[{"type":"output_text","text":"Image reference: [[sub2api-generated-image:id=img_abcdefghijklmnopqrstuvwxyzABCDEF]]\nTemporary download URL: https://example.com/sub2api/generated-images/img_abcdefghijklmnopqrstuvwxyzABCDEF.png"}]}]}`)
	redacted := string(redactOpenCodeGeneratedImagesForOps(body))
	require.NotContains(t, redacted, "img_abcdefghijklmnopqrstuvwxyzABCDEF")
	require.Contains(t, redacted, "[[sub2api-generated-image:id=[redacted]]]")
	require.Contains(t, redacted, "[redacted-generated-image-url]")
}
```

如果 `mustJSONBytes` 不存在，在测试文件内新增本地 helper：

```go
func mustJSONBytes(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}
```

- [ ] **Step 2: 运行失败测试**

Run:

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run "TestOpenCodeGeneratedImageMessageUsesSpecificMarker|TestRedactOpenCodeGeneratedImagesForOpsRedactsSpecificMarker" -count=1
```

Expected: FAIL，因为当前输出仍使用 `sub2api-image://`，redaction 不识别新哨兵。

- [ ] **Step 3: 实现最小 marker 输出和 redaction**

在 `openai_opencode_image_rehydrate.go` 增加新 pattern：

```go
var openCodeRehydrateSpecificMarkerPattern = regexp.MustCompile(`\[\[sub2api-generated-image:id=(img_[A-Za-z0-9_-]{32,})\]\]`)
```

在 `openai_opencode_image_rewrite.go` 的 `buildOpenCodeGeneratedImageMessage` 中把文本改为：

```go
text := "Generated image saved by sub2api.\nImage reference: [[sub2api-generated-image:id=" + rec.ID + "]]"
if baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/"); baseURL != "" {
	text += "\nTemporary download URL: " + baseURL + downloadPath
}
```

在 `openai_opencode_image_ops_redaction.go` 增加 pattern：

```go
openCodeSpecificImageMarkerForOpsPattern = regexp.MustCompile(`\[\[sub2api-generated-image:id=img_[A-Za-z0-9_-]{32,}\]\]`)
```

并在 `redactOpenCodeGeneratedImageTokensForOps` 中加入：

```go
redacted = openCodeSpecificImageMarkerForOpsPattern.ReplaceAllString(redacted, "[[sub2api-generated-image:id=[redacted]]]")
```

- [ ] **Step 4: 运行测试通过**

Run 同 Step 2。

Expected: PASS。

---

### Task 2: rehydrate 扫描模型与 source 过滤

**Files:**
- Modify: `backend/internal/service/openai_opencode_image_rehydrate.go`
- Test: `backend/internal/service/openai_opencode_image_rewrite_test.go`

- [ ] **Step 1: 写失败测试，覆盖压缩摘要、复读和 legacy source 判定**

新增测试：

```go
func TestScanOpenCodeGeneratedImageMarkersSkipsCompressedAndRepeatedText(t *testing.T) {
	id := "img_abcdefghijklmnopqrstuvwxyzABCDEF"
	input := []any{
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "[Compressed conversation section]\nGenerated image: sub2api-image://" + id}}},
		map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "I repeat [[sub2api-generated-image:id=" + id + "]]"}}},
		map[string]any{"id": "msg_sub2api_img_" + strings.TrimPrefix(id, "img_"), "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "Generated image saved by sub2api.\nImage reference: [[sub2api-generated-image:id=" + id + "]]"}}},
	}
	matches := scanOpenCodeGeneratedImageMarkerRefs(input)
	require.Len(t, matches, 1)
	require.Equal(t, id, matches[0].id)
	require.Equal(t, 2, matches[0].index)
}
```

- [ ] **Step 2: 运行失败测试**

Run:

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run "TestScanOpenCodeGeneratedImageMarkersSkipsCompressedAndRepeatedText" -count=1
```

Expected: FAIL，因为 `scanOpenCodeGeneratedImageMarkerRefs` 尚不存在。

- [ ] **Step 3: 增加扫描结构和过滤函数**

在 `openai_opencode_image_rehydrate.go` 增加：

```go
type openCodeImageMarkerRef struct {
	id          string
	index       int
	legacy      bool
	seq         int
	explicit    bool
	currentUser bool
}
```

实现 `scanOpenCodeGeneratedImageMarkerRefs(input any) []openCodeImageMarkerRef`：

```go
func scanOpenCodeGeneratedImageMarkerRefs(input any) []openCodeImageMarkerRef {
	items := normalizeOpenCodeRehydratedInput(input)
	matches := make([]openCodeImageMarkerRef, 0, 1)
	seq := 0
	for idx, item := range items {
		m, ok := item.(map[string]any)
		if !ok || isOpenCodeImageScanBlockedItem(m) || isOpenCodeSyntheticImageToolItem(m) || isOpenCodeSysDummyItem(m) {
			continue
		}
		isCurrentUser := isOpenCodeCurrentUserInput(items, idx, m)
		for _, text := range openCodeImageTextFields(m) {
			for _, id := range extractOpenCodeSpecificGeneratedImageIDs(text) {
				if isOpenCodeSpecificImageSource(m, isCurrentUser) {
					matches = append(matches, openCodeImageMarkerRef{id: id, index: idx, seq: seq, explicit: isOpenCodeUserRole(m), currentUser: isCurrentUser})
					seq++
				}
			}
			for _, id := range extractOpenCodeLegacyGeneratedImageIDs(text) {
				if isOpenCodeLegacyImageSource(m, text) {
					matches = append(matches, openCodeImageMarkerRef{id: id, index: idx, legacy: true, seq: seq})
					seq++
				}
			}
		}
	}
	return matches
}
```

Implement helper predicates exactly from the spec:

```go
func isOpenCodeImageScanBlockedItem(item map[string]any) bool {
	if typeValue, _ := item["type"].(string); strings.TrimSpace(typeValue) == "reasoning" {
		return true
	}
	for _, text := range openCodeImageTextFields(item) {
		if strings.Contains(text, "[Compressed conversation section]") || strings.Contains(text, "What did we do so far?") {
			return true
		}
	}
	return false
}

func isOpenCodeSyntheticImageToolItem(item map[string]any) bool {
	callID, _ := item["call_id"].(string)
	if !isOpenCodeImageCallID(callID) {
		return false
	}
	typeValue, _ := item["type"].(string)
	return typeValue == "function_call" || typeValue == "function_call_output"
}

func isOpenCodeSysDummyItem(item map[string]any) bool {
	typeValue, _ := item["type"].(string)
	callID, _ := item["call_id"].(string)
	if callID != sysDummyToolCallID {
		return false
	}
	if typeValue == "function_call_output" {
		return true
	}
	name, _ := item["name"].(string)
	return typeValue == "function_call" && name == sysDummyToolName
}

func isOpenCodeImageCallID(value any) bool {
	callID, _ := value.(string)
	return strings.HasPrefix(callID, "call_sub2api_image_img_")
}

func openCodeImageTextFields(item map[string]any) []string {
	texts := make([]string, 0, 2)
	appendText := func(v any) {
		if text, ok := v.(string); ok && strings.TrimSpace(text) != "" {
			texts = append(texts, text)
		}
	}
	appendText(item["text"])
	if content, ok := item["content"].([]any); ok {
		for _, part := range content {
			partMap, ok := part.(map[string]any)
			if !ok {
				continue
			}
			typeValue, _ := partMap["type"].(string)
			switch strings.TrimSpace(typeValue) {
			case "input_text", "output_text", "text":
				appendText(partMap["text"])
			}
		}
	}
	return texts
}

func isOpenCodeSpecificImageSource(item map[string]any, currentUser bool) bool {
	if isOpenCodeUserRole(item) {
		return currentUser
	}
	return isOpenCodeSub2APIImageAssistantMessage(item)
}

func isOpenCodeLegacyImageSource(item map[string]any, text string) bool {
	if !isOpenCodeSub2APIImageAssistantMessage(item) {
		return false
	}
	trimmed := strings.TrimSpace(text)
	return openCodeLegacyGeneratedImageLinePattern.MatchString(trimmed) ||
		openCodeLegacyDownloadLinePattern.MatchString(trimmed)
}

func isOpenCodeSub2APIImageAssistantMessage(item map[string]any) bool {
	if role, _ := item["role"].(string); strings.TrimSpace(role) != "assistant" {
		return false
	}
	if id, _ := item["id"].(string); strings.HasPrefix(strings.TrimSpace(id), "msg_sub2api_img_") {
		return true
	}
	for _, text := range openCodeImageTextFields(item) {
		trimmed := strings.TrimSpace(text)
		if strings.HasPrefix(trimmed, "Generated image saved by sub2api.\nImage reference: [[sub2api-generated-image:id=") ||
			strings.HasPrefix(trimmed, "Generated image: sub2api-image://") {
			return true
		}
	}
	return false
}

func isOpenCodeUserRole(item map[string]any) bool {
	role, _ := item["role"].(string)
	return strings.TrimSpace(role) == "user"
}

func isOpenCodeCurrentUserInput(items []any, idx int, item map[string]any) bool {
	if !isOpenCodeUserRole(item) {
		return false
	}
	for i := len(items) - 1; i >= 0; i-- {
		candidate, ok := items[i].(map[string]any)
		if !ok || isOpenCodeImageScanBlockedItem(candidate) {
			continue
		}
		if isOpenCodeUserRole(candidate) {
			return i == idx
		}
	}
	return false
}

func extractOpenCodeSpecificGeneratedImageIDs(text string) []string {
	return extractIDsWithRegex(text, openCodeSpecificGeneratedImagePattern, 1)
}

func extractOpenCodeLegacyGeneratedImageIDs(text string) []string {
	ids := extractIDsWithRegex(text, openCodeRehydrateImageMarkerPattern, 1)
	ids = append(ids, extractIDsWithRegex(text, openCodeRehydrateDownloadPathPattern, 1)...)
	ids = append(ids, extractIDsWithRegex(text, openCodeRehydrateAbsoluteDownloadURLPattern, 1)...)
	return ids
}

func extractIDsWithRegex(text string, re *regexp.Regexp, group int) []string {
	var ids []string
	for _, match := range re.FindAllStringSubmatch(text, -1) {
		if len(match) > group {
			ids = append(ids, match[group])
		}
	}
	return ids
}
```

Define these exact legacy source regexes near the existing marker regexes:

```go
var (
	openCodeSpecificGeneratedImagePattern   = regexp.MustCompile(`\[\[sub2api-generated-image:id=(img_[A-Za-z0-9_-]{32,})\]\]`)
	openCodeLegacyGeneratedImageLinePattern = regexp.MustCompile(`(?m)^Generated image: sub2api-image://img_[A-Za-z0-9_-]{32,}\s*$`)
	openCodeLegacyDownloadLinePattern       = regexp.MustCompile(`(?m)^(?:Download|I'll download from URL): (?:https?://[^\s]+)?/sub2api/generated-images/img_[A-Za-z0-9_-]{32,}\.(?:png|jpe?g|webp)\s*$`)
)
```

`isOpenCodeSpecificImageSource` permits assistant sub2api image messages and only the latest non-compaction user input item carrying the explicit new sentinel. Historical user items, user discussion of marker syntax, legacy URL/schema markers in user text, compressed summaries, compaction prompts, reasoning items, sys dummy items, and existing synthetic image tool items must not match. Add scan matrix rows for `type:"reasoning"` and for the exact old two-line template containing `I'll download from URL: /sub2api/generated-images/<id>.png`.

- [ ] **Step 4: 运行测试通过**

Run Step 2 command。

Expected: PASS。

---

### Task 3: synthetic image tool pair 构造与插入

**Files:**
- Modify: `backend/internal/service/openai_opencode_image_rehydrate.go`
- Test: `backend/internal/service/openai_opencode_image_rewrite_test.go`

- [ ] **Step 1: 写失败测试，覆盖成功图片、缺失图片和排序**

新增测试：

```go
func TestRehydrateOpenCodeGeneratedImageMarkersInsertsToolOutputNearMarker(t *testing.T) {
	ctx := context.Background()
	id := testImageID
	store := newTestStoreWithImage(t, id, "png", pngBytes)
	req := map[string]any{"input": []any{
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "before"}}},
		map[string]any{"id": "msg_sub2api_img_" + strings.TrimPrefix(id, "img_"), "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "Generated image saved by sub2api.\nImage reference: [[sub2api-generated-image:id=" + id + "]]"}}},
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "after"}}},
	}}
	changed, err := rehydrateOpenCodeGeneratedImageMarkers(ctx, req, store, openCodeImageRehydrateOptions{MaxImages: 3})
	require.NoError(t, err)
	require.True(t, changed)
	input := req["input"].([]any)
	require.Equal(t, "function_call", input[2].(map[string]any)["type"])
	require.Equal(t, "function_call_output", input[3].(map[string]any)["type"])
	output := input[3].(map[string]any)["output"].([]any)
	require.Equal(t, "input_image", output[1].(map[string]any)["type"])
	require.Contains(t, output[1].(map[string]any)["image_url"], "data:image/png;base64,")
	require.Equal(t, "after", gjson.GetBytes(mustJSONBytes(t, input[4]), "content.0.text").String())
}

func TestRehydrateOpenCodeGeneratedImageMarkersPreservesToolTailAndDisambiguatesCallID(t *testing.T) {
	ctx := context.Background()
	id := testImageID
	store := newTestStoreWithImage(t, id, "png", pngBytes)
	baseCallID := "call_sub2api_image_" + id
	req := map[string]any{"input": []any{
		map[string]any{"type": "function_call", "call_id": baseCallID, "name": "real_tool", "arguments": "{}"},
		map[string]any{"id": "msg_sub2api_img_" + strings.TrimPrefix(id, "img_"), "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "Generated image saved by sub2api.\nImage reference: [[sub2api-generated-image:id=" + id + "]]"}}},
		map[string]any{"type": "function_call_output", "call_id": "call_real_tail", "output": "real tail"},
	}}
	changed, err := rehydrateOpenCodeGeneratedImageMarkers(ctx, req, store, openCodeImageRehydrateOptions{MaxImages: 3})
	require.NoError(t, err)
	require.True(t, changed)
	input := req["input"].([]any)
	call := input[2].(map[string]any)
	out := input[3].(map[string]any)
	require.NotEqual(t, baseCallID, call["call_id"])
	require.Equal(t, call["call_id"], out["call_id"])
	require.Equal(t, "call_real_tail", input[len(input)-1].(map[string]any)["call_id"])
}

func TestRehydrateOpenCodeGeneratedImageMarkersSkipsSysDummyAndKeepsRecentThreeOrder(t *testing.T) {
	ctx := context.Background()
	ids := []string{
		"img_abcdefghijklmnopqrstuvwxyzABCDEF",
		"img_bcdefghijklmnopqrstuvwxyzABCDEFG",
		"img_cdefghijklmnopqrstuvwxyzABCDEFGH",
		"img_defghijklmnopqrstuvwxyzABCDEFGHI",
	}
	dummyOnlyID := "img_dummyonlydummyonlydummyonlydummyonly"
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	for _, id := range ids {
		_, err := store.saveDecodedForTest(id, "png", pngBytes)
		require.NoError(t, err)
	}
	req := map[string]any{"input": []any{
		map[string]any{"id": "msg_sub2api_img_" + strings.TrimPrefix(ids[0], "img_"), "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "Generated image saved by sub2api.\nImage reference: [[sub2api-generated-image:id=" + ids[0] + "]]"}}},
		map[string]any{"id": "msg_sub2api_img_" + strings.TrimPrefix(ids[1], "img_"), "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "Generated image saved by sub2api.\nImage reference: [[sub2api-generated-image:id=" + ids[1] + "]]"}}},
		map[string]any{"id": "msg_sub2api_img_" + strings.TrimPrefix(ids[2], "img_"), "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "Generated image saved by sub2api.\nImage reference: [[sub2api-generated-image:id=" + ids[2] + "]]"}}},
		map[string]any{"id": "msg_sub2api_img_" + strings.TrimPrefix(ids[0], "img_"), "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "Generated image saved by sub2api.\nImage reference: [[sub2api-generated-image:id=" + ids[0] + "]]"}}},
		map[string]any{"id": "msg_sub2api_img_" + strings.TrimPrefix(ids[3], "img_"), "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "Generated image saved by sub2api.\nImage reference: [[sub2api-generated-image:id=" + ids[3] + "]]"}}},
		map[string]any{"type": "function_call", "call_id": sysDummyToolCallID, "name": sysDummyToolName, "arguments": "{}"},
		map[string]any{"type": "function_call_output", "call_id": sysDummyToolCallID, "output": "[[sub2api-generated-image:id=" + dummyOnlyID + "]]"},
	}}
	changed, err := rehydrateOpenCodeGeneratedImageMarkers(ctx, req, store, openCodeImageRehydrateOptions{MaxImages: 3})
	require.NoError(t, err)
	require.True(t, changed)
	input := req["input"].([]any)
	require.Equal(t, sysDummyToolCallID, input[len(input)-1].(map[string]any)["call_id"])
	encoded := string(mustJSONBytes(t, req))
	require.Equal(t, 3, strings.Count(encoded, `"name":"sub2api_generated_image"`))
	require.NotContains(t, encoded, "call_sub2api_image_"+dummyOnlyID)
	var got []string
	for i := 0; i < len(input); i++ {
		item, ok := input[i].(map[string]any)
		if !ok || item["type"] != "function_call" || item["name"] != openCodeGeneratedImageToolName {
			continue
		}
		require.Less(t, i+1, len(input))
		output := input[i+1].(map[string]any)
		require.Equal(t, "function_call_output", output["type"])
		require.Equal(t, item["call_id"], output["call_id"])
		got = append(got, imageIDFromOpenCodeImageCallID(item["call_id"]))
	}
	require.Equal(t, []string{ids[2], ids[0], ids[3]}, got)
}

func TestRehydrateOpenCodeGeneratedImageMarkersIgnoresSourcesInsideExistingSysDummyTail(t *testing.T) {
	ctx := context.Background()
	id := testImageID
	store := newTestStoreWithImage(t, id, "png", pngBytes)
	req := map[string]any{"input": []any{
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "before dummy"}}},
		map[string]any{"type": "function_call", "call_id": sysDummyToolCallID, "name": sysDummyToolName, "arguments": "{}"},
		map[string]any{"type": "function_call_output", "call_id": sysDummyToolCallID, "output": sysDummyToolOutput + " [[sub2api-generated-image:id=" + id + "]]"},
	}}
	changed, err := rehydrateOpenCodeGeneratedImageMarkers(ctx, req, store, openCodeImageRehydrateOptions{MaxImages: 3})
	require.NoError(t, err)
	require.False(t, changed)
}
```

Use the existing helper already present near the top of `openai_opencode_image_rewrite_test.go`: `newTestStoreWithImage(t, id, "png", pngBytes)`.

- [ ] **Step 2: 运行失败测试**

Run:

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run "TestRehydrateOpenCodeGeneratedImageMarkers(InsertsToolOutputNearMarker|PreservesToolTailAndDisambiguatesCallID|SkipsSysDummyAndKeepsRecentThreeOrder|IgnoresSourcesInsideExistingSysDummyTail)" -count=1
```

Expected: FAIL，因为当前实现追加 user message 到末尾。

- [ ] **Step 3: 实现 tool pair 构造**

在 `openai_opencode_image_rehydrate.go` 增加：

```go
const openCodeGeneratedImageToolName = "sub2api_generated_image"

type openCodeImageToolInsert struct {
	index int
	items []any
}

func buildOpenCodeImageToolCall(id string, used map[string]struct{}) (map[string]any, string) {
	base := "call_sub2api_image_" + id
	callID := uniqueOpenCodeImageCallID(base, used)
	return map[string]any{"type": "function_call", "call_id": callID, "name": openCodeGeneratedImageToolName, "arguments": "{}"}, callID
}

func buildOpenCodeImageToolOutput(callID string, parts []any) map[string]any {
	return map[string]any{"type": "function_call_output", "call_id": callID, "output": parts}
}

func uniqueOpenCodeImageCallID(base string, used map[string]struct{}) string {
	if _, ok := used[base]; !ok {
		used[base] = struct{}{}
		return base
	}
	for i := 1; ; i++ {
		candidate := base + "_dup" + strconv.Itoa(i)
		if _, ok := used[candidate]; !ok {
			used[candidate] = struct{}{}
			return candidate
		}
	}
}

func imageIDFromOpenCodeImageCallID(value any) string {
	callID, _ := value.(string)
	id := strings.TrimPrefix(callID, "call_sub2api_image_")
	if idx := strings.Index(id, "_dup"); idx >= 0 {
		id = id[:idx]
	}
	return id
}

func findSysDummyTail(items []any) (int, bool) {
	if len(items) < 2 {
		return 0, false
	}
	call, ok := items[len(items)-2].(map[string]any)
	if !ok {
		return 0, false
	}
	output, ok := items[len(items)-1].(map[string]any)
	if !ok {
		return 0, false
	}
	if call["type"] != "function_call" || call["name"] != sysDummyToolName || call["call_id"] != sysDummyToolCallID {
		return 0, false
	}
	if output["type"] != "function_call_output" || output["call_id"] != sysDummyToolCallID {
		return 0, false
	}
	return len(items) - 2, true
}
```

Collect `used` from every existing top-level input item `call_id` before building inserts, including real user tool calls and sys dummy calls. The `_dupN` suffix is deterministic because inserts are processed in the selected marker order.

Build successful parts as `[]any{map[string]any{"type":"input_text",...}, map[string]any{"type":"input_image", "image_url":"data:"+mime+";base64,"+...}}`.

Build unavailable parts as `[]any{map[string]any{"type":"input_text", "text":"Generated image "+id+" is no longer available. Use the nearby marker only as historical context."}}` only for explicit current user markers.

- [ ] **Step 4: 实现基于原始 input 快照的一次性重建**

Replace `appendOpenCodeRehydratedMessages` usage with a new function:

```go
func insertOpenCodeImageToolPairs(input any, inserts []openCodeImageToolInsert) []any {
	items := normalizeOpenCodeRehydratedInput(input)
	byIndex := make(map[int][]any)
	for _, insert := range inserts {
		byIndex[insert.index] = append(byIndex[insert.index], insert.items...)
	}
	result := make([]any, 0, len(items)+len(inserts)*2)
	for idx, item := range items {
		result = append(result, item)
		if extra := byIndex[idx]; len(extra) > 0 {
			result = append(result, extra...)
		}
	}
	return result
}
```

Select candidates by filtering invalid sources, deduping by id using the last valid match, keeping the most recent `MaxImages`, then sorting selected refs by original `index` and `seq` before insertion.

When a valid existing sys dummy tail is present, compute `dummyStart` from `findSysDummyTail` before scanning. Any marker source inside the final dummy pair (`index >= dummyStart`) is invalid and must be ignored; never move a source that appears inside sys dummy to before the dummy pair.

- [ ] **Step 5: 运行测试通过**

Run Step 2 command。

Expected: PASS。

---

### Task 4: 不可用图片策略与 TTL/LRU 去重

**Files:**
- Modify: `backend/internal/service/openai_opencode_image_rehydrate.go`
- Test: `backend/internal/service/openai_opencode_image_rewrite_test.go`

- [ ] **Step 1: 写失败测试，历史缺失静默，当前 user 显式缺失提醒一次**

新增测试：

```go
func TestRehydrateOpenCodeGeneratedImageUnavailablePolicy(t *testing.T) {
	resetOpenCodeUnavailableImageReportsForTest(t)
	ctx := context.Background()
	store := newTestOpenAIGeneratedImageStore(t, fixedNow)
	id := "img_abcdefghijklmnopqrstuvwxyzABCDEF"
	history := map[string]any{"input": []any{map[string]any{"id": "msg_sub2api_img_" + strings.TrimPrefix(id, "img_"), "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "Generated image saved by sub2api.\nImage reference: [[sub2api-generated-image:id=" + id + "]]"}}}}}
	changed, err := rehydrateOpenCodeGeneratedImageMarkers(ctx, history, store, openCodeImageRehydrateOptions{MaxImages: 3})
	require.NoError(t, err)
	require.False(t, changed)

	current := map[string]any{"input": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "restore [[sub2api-generated-image:id=" + id + "]]"}}}}}
	changed, err = rehydrateOpenCodeGeneratedImageMarkers(ctx, current, store, openCodeImageRehydrateOptions{MaxImages: 3})
	require.NoError(t, err)
	require.True(t, changed)
	fresh := map[string]any{"input": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "restore [[sub2api-generated-image:id=" + id + "]]"}}}}}
	changed, err = rehydrateOpenCodeGeneratedImageMarkers(ctx, fresh, store, openCodeImageRehydrateOptions{MaxImages: 3})
	require.NoError(t, err)
	require.False(t, changed)
}
```

- [ ] **Step 2: Run failing test**

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run "TestRehydrateOpenCodeGeneratedImageUnavailablePolicy" -count=1
```

Expected: FAIL。

- [ ] **Step 3: 实现不可用策略**

Add process-local bounded TTL/LRU fields near rehydrate helpers:

```go
var openCodeUnavailableImageReports = newOpenCodeUnavailableImageReportCache(256, time.Hour)

func shouldReportOpenCodeUnavailableImage(id string, reason string, explicit bool) bool {
	if !explicit {
		return false
	}
	return openCodeUnavailableImageReports.Mark(id + ":" + reason)
}
```

Implement `openCodeUnavailableImageReportCache` with a mutex, `map[string]time.Time`, insertion order slice, capacity, TTL, and `Mark(key string) bool`. `Mark` returns false when an unexpired key already exists; on insert it evicts expired keys and then oldest keys until the map is within capacity.

Add cache-level tests with an injectable `now func() time.Time` field on the cache:

```go
func TestOpenCodeUnavailableImageReportCacheMarkTTLAndCapacity(t *testing.T) {
	now := fixedNow
	cache := newOpenCodeUnavailableImageReportCache(2, time.Hour)
	cache.now = func() time.Time { return now }
	require.True(t, cache.Mark("a"))
	require.False(t, cache.Mark("a"))
	now = now.Add(time.Hour + time.Second)
	require.True(t, cache.Mark("a"))
	require.True(t, cache.Mark("b"))
	require.True(t, cache.Mark("c"))
	require.True(t, cache.Mark("a"), "oldest key should have been evicted when capacity is exceeded")
}
```

Put `resetOpenCodeUnavailableImageReportsForTest` in `_test.go`; tests that reset this global cache must not call `t.Parallel()`.

Add a test helper:

```go
func resetOpenCodeUnavailableImageReportsForTest(t *testing.T) {
	t.Helper()
	old := openCodeUnavailableImageReports
	openCodeUnavailableImageReports = newOpenCodeUnavailableImageReportCache(256, time.Hour)
	t.Cleanup(func() { openCodeUnavailableImageReports = old })
}
```

- [ ] **Step 4: Run test passing**

Run Step 2 command。

Expected: PASS。

---

### Task 5: 调度前 rehydrate 与 `-Sys` tail 保护

**Files:**
- Modify: `backend/internal/handler/openai_gateway_handler.go`
- Modify: `backend/internal/service/openai_gateway_service.go`
- Test: `backend/internal/handler/openai_gateway_handler_test.go`
- Test: `backend/internal/service/openai_gateway_service_test.go`

- [ ] **Step 1: 写失败测试，`gpt-5.5-Sys + marker at last user item` 保持 sys dummy 尾部**

在 `openai_gateway_handler_test.go` 增加：

```go
func mustJSONBytesHandlerTest(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}

func TestPrepareResponsesRequestForSchedulingCachesNeedsSysDummyBeforeImageRehydrate(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.14.31")
	body := []byte(`{"model":"gpt-5.5-Sys","input":[{"role":"user","content":[{"type":"input_text","text":"restore [[sub2api-generated-image:id=img_abcdefghijklmnopqrstuvwxyzABCDEF]]"}]}]}`)
	hookCalled := false
	hook := func(_ *gin.Context, reqBody map[string]any) (bool, error) {
		hookCalled = true
		input := reqBody["input"].([]any)
		reqBody["input"] = append(input,
			map[string]any{"type": "function_call", "call_id": "call_sub2api_image_img_abcdefghijklmnopqrstuvwxyzABCDEF", "name": "sub2api_generated_image", "arguments": "{}"},
			map[string]any{"type": "function_call_output", "call_id": "call_sub2api_image_img_abcdefghijklmnopqrstuvwxyzABCDEF", "output": []any{map[string]any{"type": "input_text", "text": "restored"}}},
		)
		return true, nil
	}
	patchedBody, patchedModel, targetGroup, err := prepareResponsesRequestForScheduling(c, body, "gpt-5.5-Sys", hook)
	require.NoError(t, err)
	require.True(t, hookCalled)
	require.Equal(t, "gpt-5.5", patchedModel)
	require.Equal(t, service.TargetGroupExhausted, targetGroup)
	var patched map[string]any
	require.NoError(t, json.Unmarshal(patchedBody, &patched))
	input := patched["input"].([]any)
	require.Equal(t, "function_call", input[len(input)-4].(map[string]any)["type"])
	require.Equal(t, "function_call_output", input[len(input)-3].(map[string]any)["type"])
	require.Equal(t, "function_call", input[len(input)-2].(map[string]any)["type"])
	require.Equal(t, "function_call_output", input[len(input)-1].(map[string]any)["type"])
	require.Equal(t, "sys_dummy", input[len(input)-1].(map[string]any)["call_id"])
	cached, ok := c.Get(service.OpenAIParsedRequestBodyKey)
	require.True(t, ok)
	require.Len(t, cached.(map[string]any)["input"].([]any), len(input))
	require.JSONEq(t, string(patchedBody), string(mustJSONBytesHandlerTest(t, cached)))
}

func TestPrepareResponsesRequestForSchedulingRunsHookForActiveOpenCodeRequests(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.14.31")
	body := []byte(`{"model":"gpt-5.5","input":[{"role":"user","content":[{"type":"input_text","text":"restore [[sub2api-generated-image:id=img_abcdefghijklmnopqrstuvwxyzABCDEF]]"}]}]}`)
	hookCalled := false
	hook := func(_ *gin.Context, reqBody map[string]any) (bool, error) {
		hookCalled = true
		input := reqBody["input"].([]any)
		reqBody["input"] = append(input,
			map[string]any{"type": "function_call", "call_id": "call_sub2api_image_img_abcdefghijklmnopqrstuvwxyzABCDEF", "name": "sub2api_generated_image", "arguments": "{}"},
			map[string]any{"type": "function_call_output", "call_id": "call_sub2api_image_img_abcdefghijklmnopqrstuvwxyzABCDEF", "output": []any{map[string]any{"type": "input_text", "text": "restored"}}},
		)
		return true, nil
	}
	patchedBody, _, targetGroup, err := prepareResponsesRequestForScheduling(c, body, "gpt-5.5", hook)
	require.NoError(t, err)
	require.True(t, hookCalled)
	require.Equal(t, service.TargetGroupExhausted, targetGroup)
	var patched map[string]any
	require.NoError(t, json.Unmarshal(patchedBody, &patched))
	input := patched["input"].([]any)
	require.Equal(t, "function_call_output", input[len(input)-1].(map[string]any)["type"])
	cached, ok := c.Get(service.OpenAIParsedRequestBodyKey)
	require.True(t, ok)
	require.JSONEq(t, string(patchedBody), string(mustJSONBytesHandlerTest(t, cached)))
}
```

These tests deliberately use a lightweight scheduling hook rather than a generated image store. The hook mutates `reqBody` and returns `changed=true`; this proves hook execution happens before target-group calculation and that `needsSysDummy` is cached before image insertion.

- [ ] **Step 2: Run failing test**

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test ./internal/handler -run "TestPrepareResponsesRequestForScheduling(CachesNeedsSysDummyBeforeImageRehydrate|RunsHookForActiveOpenCodeRequests)" -count=1
```

Expected: FAIL until scheduling hook exists and `needsParsedBody := isSysModel || hook != nil` prevents the active-path early return.

- [ ] **Step 3: Refactor scheduling prepare signature minimally**

Change `prepareResponsesRequestForScheduling` to accept an optional callback without coupling handler tests to service internals:

```go
type responsesRequestPrepareHook func(*gin.Context, map[string]any) (bool, error)

func prepareResponsesRequestForScheduling(c *gin.Context, body []byte, reqModel string, hook responsesRequestPrepareHook) ([]byte, string, service.AccountTargetGroup, error) {
```

Update existing call sites and tests to pass `nil` where no rehydrate hook is needed.

At the top of the function, change the parse gate to ensure OpenCode hooks run even for active-looking requests:

```go
needsParsedBody := isSysModel || hook != nil
if !needsParsedBody {
	if cached, err := getResponsesRequestBody(c, body); err == nil {
		reqBody = cached
		needsParsedBody = service.GetRequestTargetGroup(cached) == service.TargetGroupExhausted
	}
}
```

Inside `-Sys` block:

```go
needsSysDummy := false
if isSysModel {
	// after input string normalization and model strip
	needsSysDummy = service.NeedsSysToolContinuation(reqBody)
}
if hook != nil {
	changed, err := hook(c, reqBody)
	if err != nil { return nil, "", targetGroup, err }
	rewriteBody = rewriteBody || changed
}
if validation := service.ValidateFunctionCallOutputContext(reqBody); validation.HasFunctionCallOutput && !validation.HasToolCallContext && !validation.HasItemReferenceForAllCallIDs {
	return nil, "", targetGroup, fmt.Errorf("%w: injected function_call_output lacks matching function_call context", errPrepareResponsesRequestRewrite)
}
if isSysModel && needsSysDummy {
	service.AppendMinimalSysToolContinuation(reqBody)
	rewriteBody = true
}
```

When `rewriteBody` is true, marshal exactly once after hook and sys dummy insertion, then call `c.Set(service.OpenAIParsedRequestBodyKey, reqBody)`. Any later handler body mutation, including channel/model mapping after this function returns, must update this same cached map or clear/rebuild `OpenAIParsedRequestBodyKey` before `Forward`. The session hash body must be assigned from the final marshaled `body` after prepare and channel mapping, not the pre-rehydrate body, so `GenerateSessionHash` and upstream forwarding see the same request shape.

Add one handler call-site test named `TestOpenAIGatewayResponsesUsesPreparedBodyForSessionHashAndForward`: introduce three package-local seams in `openai_gateway_handler.go`: `var prepareResponsesRequestForSchedulingFn = prepareResponsesRequestForScheduling`, `var generateOpenAISessionHash = func(s *service.OpenAIGatewayService, c *gin.Context, body []byte) string { return s.GenerateSessionHash(c, body) }`, and `var forwardOpenAIResponsesForTestableCallSite = func(s *service.OpenAIGatewayService, ctx context.Context, c *gin.Context, account *service.Account, body []byte) (*service.OpenAIForwardResult, error) { return s.Forward(ctx, c, account, body) }`. Use them at the existing call sites. The test temporarily replaces the seams to return/capture a known prepared body containing a synthetic image pair. Assert the bytes passed to `generateOpenAISessionHash`, `OpsUpstreamRequestBodyKey`, `OpenAIParsedRequestBodyKey`, and forward seam all JSON-equal the prepared body returned by the prepare seam.

```go
func TestOpenAIGatewayResponsesUsesPreparedBodyForSessionHashAndForward(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prepared := []byte(`{"model":"gpt-5.5","input":[{"role":"user","content":[{"type":"input_text","text":"restore"}]},{"type":"function_call","call_id":"call_sub2api_image_img_abcdefghijklmnopqrstuvwxyzABCDEF","name":"sub2api_generated_image","arguments":"{}"},{"type":"function_call_output","call_id":"call_sub2api_image_img_abcdefghijklmnopqrstuvwxyzABCDEF","output":[{"type":"input_text","text":"restored"}]}]}`)
	var parsedPrepared map[string]any
	require.NoError(t, json.Unmarshal(prepared, &parsedPrepared))

	oldPrepare := prepareResponsesRequestForSchedulingFn
	oldHash := generateOpenAISessionHash
	oldForward := forwardOpenAIResponsesForTestableCallSite
	var hashBody []byte
	var forwardedBody []byte
	prepareResponsesRequestForSchedulingFn = func(c *gin.Context, _ []byte, _ string, _ responsesRequestPrepareHook) ([]byte, string, service.AccountTargetGroup, error) {
		cloned := map[string]any{}
		require.NoError(t, json.Unmarshal(prepared, &cloned))
		c.Set(service.OpenAIParsedRequestBodyKey, cloned)
		return prepared, "gpt-5.5", service.TargetGroupExhausted, nil
	}
	generateOpenAISessionHash = func(_ *service.OpenAIGatewayService, _ *gin.Context, body []byte) string {
		hashBody = append([]byte(nil), body...)
		return ""
	}
	forwardOpenAIResponsesForTestableCallSite = func(_ *service.OpenAIGatewayService, _ context.Context, c *gin.Context, _ *service.Account, body []byte) (*service.OpenAIForwardResult, error) {
		forwardedBody = append([]byte(nil), body...)
		c.Set(service.OpsUpstreamRequestBodyKey, append([]byte(nil), body...))
		return &service.OpenAIForwardResult{Model: "gpt-5.5"}, nil
	}
	t.Cleanup(func() {
		prepareResponsesRequestForSchedulingFn = oldPrepare
		generateOpenAISessionHash = oldHash
		forwardOpenAIResponsesForTestableCallSite = oldForward
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.5-Sys","input":"restore"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "opencode/1.0")
	groupID := int64(12)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 101, GroupID: &groupID, User: &service.User{ID: 1}})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1, Concurrency: 1})

	gatewayService := &service.OpenAIGatewayService{}
	setUnexportedFieldForTest(t, gatewayService, "openaiScheduler", &openAIAccountSchedulerStub{selectFn: func(ctx context.Context, req service.OpenAIAccountScheduleRequest) (*service.AccountSelectionResult, service.OpenAIAccountScheduleDecision, error) {
		return &service.AccountSelectionResult{Account: &service.Account{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}, Acquired: true, ReleaseFunc: func() {}}, service.OpenAIAccountScheduleDecision{}, nil
	}})
	h := &OpenAIGatewayHandler{gatewayService: gatewayService, billingCacheService: service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeSimple}), apiKeyService: &service.APIKeyService{}, concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(&concurrencyCacheMock{acquireUserSlotFn: func(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) { return true, nil }}), SSEPingFormatNone, time.Second)}
	h.Responses(c)

	require.JSONEq(t, string(prepared), string(hashBody))
	require.JSONEq(t, string(prepared), string(forwardedBody))
	raw, ok := c.Get(service.OpsUpstreamRequestBodyKey)
	require.True(t, ok)
	require.JSONEq(t, string(prepared), string(raw.([]byte)))
	cached, ok := c.Get(service.OpenAIParsedRequestBodyKey)
	require.True(t, ok)
	require.JSONEq(t, string(prepared), string(mustJSONBytesHandlerTest(t, cached)))
}
```

The required assertions and `c.Set(service.OpenAIParsedRequestBodyKey, cloned)` side effect are not optional.

The forward seam in this test must set `OpsUpstreamRequestBodyKey` from the exact `body` argument it receives, mirroring the real `Forward` capture point while still letting the test avoid an actual upstream call. Task 7 separately verifies the real `Forward`/ops redaction path.

In the real handler call, pass a hook that calls gateway service image rehydrate only for OpenCode normal `/v1/responses`.

- [ ] **Step 4: Move service-stage rehydrate out of `Forward`**

Remove the existing block in `openai_gateway_service.go` that calls `rehydrateOpenCodeGeneratedImageMarkers` after model normalization. Replace it with a method callable from handler:

```go
func (s *OpenAIGatewayService) RehydrateOpenCodeGeneratedImagesForResponses(ctx context.Context, c *gin.Context, reqBody map[string]any) (bool, error) {
	if !isOpenCodeResponsesClient(c) || isOpenAIResponsesCompactPath(c) {
		return false, nil
	}
	return rehydrateOpenCodeGeneratedImageMarkers(ctx, reqBody, s.generatedImageStore, openCodeImageRehydrateOptions{MaxImages: 3})
}
```

The handler hook calls this method before target group calculation. `Forward` must not call `rehydrateOpenCodeGeneratedImageMarkers` again; scanner idempotency must skip existing synthetic image pairs, and a service test should preinsert a synthetic pair then call the forward/prep path to assert no second pair is added.

- [ ] **Step 5: Run handler and service tests**

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test ./internal/handler -run "PrepareResponsesRequestForScheduling|OpenAIGateway" -count=1
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run "OpenCode|Rehydrate|ToolContinuation" -count=1
```

Expected: PASS。

---

### Task 6: array/string/auto 降级与 transform 保真

**Files:**
- Modify: `backend/internal/service/openai_opencode_image_rehydrate.go`
- Modify: `backend/internal/service/openai_gateway_service.go`
- Test: `backend/internal/service/openai_gateway_service_test.go`

- [ ] **Step 1: 写失败测试，array output 经过 Codex transform 后仍是 array**

Add test:

```go
func TestOpenCodeImageToolOutputArraySurvivesCodexTransform(t *testing.T) {
	req := map[string]any{"model": "gpt-5.5", "input": []any{
		map[string]any{"type": "function_call", "call_id": "call_sub2api_image_img_abcdefghijklmnopqrstuvwxyzABCDEF", "name": "sub2api_generated_image", "arguments": "{}"},
		map[string]any{"type": "function_call_output", "call_id": "call_sub2api_image_img_abcdefghijklmnopqrstuvwxyzABCDEF", "output": []any{map[string]any{"type": "input_text", "text": "restored"}, map[string]any{"type": "input_image", "image_url": "data:image/png;base64,aGVsbG8="}}},
	}}
	result := applyCodexOAuthTransform(req, false, false)
	require.True(t, result.Modified)
	encoded := mustJSONBytes(t, req)
	require.True(t, gjson.GetBytes(encoded, "input.1.output").IsArray())
	require.Equal(t, "input_image", gjson.GetBytes(encoded, "input.1.output.1.type").String())
	require.Equal(t, gjson.GetBytes(encoded, "input.0.call_id").String(), gjson.GetBytes(encoded, "input.1.call_id").String())
}
```

- [ ] **Step 2: Run test**

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run "TestOpenCodeImageToolOutputArraySurvivesCodexTransform" -count=1
```

Expected: PASS if transform already preserves arrays; if FAIL, adjust transform to preserve `function_call_output.output` without stringifying.

- [ ] **Step 3: Add string fallback builder test**

Add test:

```go
func TestOpenCodeImageToolOutputStringFallbackDoesNotContainImageBytes(t *testing.T) {
	output := buildOpenCodeImageToolOutputStringFallback("img_abcdefghijklmnopqrstuvwxyzABCDEF", "https://example.com/sub2api/generated-images/img_abcdefghijklmnopqrstuvwxyzABCDEF.png")
	require.NotContains(t, output, "data:image")
	require.NotContains(t, output, "base64,")
	require.Contains(t, output, "[[sub2api-generated-image:id=img_abcdefghijklmnopqrstuvwxyzABCDEF]]")
}
```

Implement `buildOpenCodeImageToolOutputStringFallback(id, url string) string` in `openai_opencode_image_rehydrate.go`.

- [ ] **Step 4: Add explicit array incompatibility retry helper**

In `openai_opencode_image_rehydrate.go`, add a body mutator that converts only synthetic image tool outputs from array to safe string fallback:

```go
func rewriteOpenCodeImageToolOutputsToStringFallback(reqBody map[string]any) bool {
	input, ok := reqBody["input"].([]any)
	if !ok {
		return false
	}
	changed := false
	for _, item := range input {
		m, ok := item.(map[string]any)
		if !ok || m["type"] != "function_call_output" || !isOpenCodeImageCallID(m["call_id"]) {
			continue
		}
		if _, ok := m["output"].([]any); !ok {
			continue
		}
		id := imageIDFromOpenCodeImageCallID(m["call_id"])
		m["output"] = buildOpenCodeImageToolOutputStringFallback(id, "")
		changed = true
	}
	return changed
}

func cloneOpenAIRequestBodyMap(reqBody map[string]any) map[string]any {
	if reqBody == nil {
		return nil
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil
	}
	var cloned map[string]any
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil
	}
	return cloned
}
```

The fallback string must contain the new sentinel and text explanation only; it must not contain `data:image`, `base64,`, raw bytes, or a long download URL.

`cloneOpenAIRequestBodyMap` must be a JSON-compatible deep clone. A shallow map copy is forbidden because rewriting `retryReqBody["input"]` would share nested slices/maps and mutate the original parsed body cached under `OpenAIParsedRequestBodyKey`.

In `openai_gateway_service.go`, add a narrow retry decision helper used in the non-WS HTTP loop after reading a non-2xx response body and before `shouldFailoverOpenAIUpstreamResponse` / `handleErrorResponse` side effects:

```go
func shouldRetryOpenCodeImageOutputArrayCompatibility(c *gin.Context, status int, body []byte) bool {
	if c == nil || status != http.StatusBadRequest {
		return false
	}
	if c.Writer != nil && c.Writer.Written() {
		return false
	}
	if marked, _ := c.Get(OpenAIImageToolOutputArrayKey); marked != true {
		return false
	}
	text := strings.ToLower(string(body))
	return strings.Contains(text, "function_call_output") && strings.Contains(text, "output")
}
```

Use it in the existing service non-WS HTTP forward loop after reading a non-2xx body and before `shouldFailoverOpenAIUpstreamResponse` / `handleErrorResponse` side effects. Do not put this logic in handler account-switch/failover code.

```go
if !openCodeImageOutputArrayRetryTried && shouldRetryOpenCodeImageOutputArrayCompatibility(c, resp.StatusCode, respBody) {
	retryReqBody := cloneOpenAIRequestBodyMap(reqBody)
	if rewriteOpenCodeImageToolOutputsToStringFallback(retryReqBody) {
		retryBody, err := json.Marshal(retryReqBody)
		if err != nil { return nil, fmt.Errorf("serialize image output fallback retry body: %w", err) }
		body = retryBody
		setOpsUpstreamRequestBody(c, retryBody)
		openCodeImageOutputArrayRetryTried = true
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Retrying same account once with OpenCode image tool output string fallback (account: %s)", account.Name)
		continue
	}
}
```

This retry stays on the same account, does not call account failover or rate-limit side effects before retrying, and is attempted at most once. It uses a cloned request body and must not overwrite global `OpenAIParsedRequestBodyKey`; if this same-account retry also fails and the handler later switches accounts, the next account starts from the original array request rather than a polluted fallback cache. It only handles HTTP status-time 400 responses before any downstream body is written. Do not retry SSE `response.failed` or any error after body-level events have started; those paths only log redacted diagnostics and return the existing error.

Use a request context marker when array output is generated:

```go
const OpenAIImageToolOutputArrayKey = "openai_image_tool_output_array"
```

Set it in handler/service hook when array output is inserted. The retry check is only in the service non-WS HTTP loop's 4xx branch, before failover/handleErrorResponse side effects; it is not part of handler account-switch/failover logic.

Add these tests:

```go
func TestOpenCodeImageToolOutputAutoRetryRewritesArrayToStringFallback(t *testing.T) {
	upstream := &httpUpstreamSequenceRecorder{responses: []*http.Response{
		{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"function_call_output output must be a string"}}`))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"resp_ok","object":"response","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`))},
	}}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"function_call","call_id":"call_sub2api_image_img_abcdefghijklmnopqrstuvwxyzABCDEF","name":"sub2api_generated_image","arguments":"{}"},{"type":"function_call_output","call_id":"call_sub2api_image_img_abcdefghijklmnopqrstuvwxyzABCDEF","output":[{"type":"input_text","text":"restored"},{"type":"input_image","image_url":"data:image/png;base64,aGVsbG8="}]}]}`)
	c, _ := newOpenAITestContext(t, "/v1/responses", body)
	c.Set(OpenAIImageToolOutputArrayKey, true)
	var parsedOriginal map[string]any
	require.NoError(t, json.Unmarshal(body, &parsedOriginal))
	c.Set(OpenAIParsedRequestBodyKey, parsedOriginal)

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.bodies, 2)
	require.Contains(t, string(upstream.bodies[0]), "data:image/png;base64,aGVsbG8=")
	require.NotContains(t, string(upstream.bodies[1]), "data:image")
	require.NotContains(t, string(upstream.bodies[1]), "aGVsbG8=")
	require.Contains(t, string(upstream.bodies[1]), `"output":"`)
	cached, ok := c.Get(OpenAIParsedRequestBodyKey)
	require.True(t, ok)
	require.True(t, gjson.GetBytes(mustJSONBytes(t, cached), "input.1.output").IsArray(), "fallback retry must not mutate global parsed request cache")
}

func TestOpenCodeImageToolOutputAutoRetrySkipsNonOutputTypeErrors(t *testing.T) {
	bodyWithImageOutputArray := []byte(`{"model":"gpt-5.5","input":[{"type":"function_call","call_id":"call_sub2api_image_img_abcdefghijklmnopqrstuvwxyzABCDEF","name":"sub2api_generated_image","arguments":"{}"},{"type":"function_call_output","call_id":"call_sub2api_image_img_abcdefghijklmnopqrstuvwxyzABCDEF","output":[{"type":"input_text","text":"restored"},{"type":"input_image","image_url":"data:image/png;base64,aGVsbG8="}]}]}`)
	upstream := &httpUpstreamSequenceRecorder{responses: []*http.Response{{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"ordinary bad request"}}`))}}}
	svc := newOpenAITestGatewayService(upstream)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
	c, _ := newOpenAITestContext(t, "/v1/responses", bodyWithImageOutputArray)
	c.Set(OpenAIImageToolOutputArrayKey, true)
	_, err := svc.Forward(context.Background(), c, account, bodyWithImageOutputArray)
	require.Error(t, err)
	require.Len(t, upstream.bodies, 1)
}

func TestShouldRetryOpenCodeImageOutputArrayCompatibilitySkipsWrittenResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Set(OpenAIImageToolOutputArrayKey, true)
	_, _ = c.Writer.Write([]byte("started"))
	require.False(t, shouldRetryOpenCodeImageOutputArrayCompatibility(c, http.StatusBadRequest, []byte(`{"error":{"message":"function_call_output output"}}`)))
}
```

The helper-level written-response test is required; it protects future refactors that might move the retry decision after downstream output has started.

- [ ] **Step 5: Run fallback tests**

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run "ImageToolOutput(Array|String|Fallback)|OpenCodeImage" -count=1
```

Expected: PASS。

---

### Task 7: ops、request detail 和 runtime log redaction

**Files:**
- Modify: `backend/internal/service/openai_opencode_image_ops_redaction.go`
- Modify: `backend/internal/service/openai_gateway_service.go`
- Test: `backend/internal/service/openai_opencode_image_rewrite_test.go`
- Test: `backend/internal/service/openai_gateway_service_test.go`
- Test: `backend/internal/handler/openai_gateway_handler_test.go`

- [ ] **Step 1: 写失败测试，output array data URL 被递归 redacted**

```go
func TestRedactOpenCodeGeneratedImagesForOpsRedactsFunctionOutputArrayImage(t *testing.T) {
	body := []byte(`{"input":[{"type":"function_call_output","call_id":"call_1","output":[{"type":"input_text","text":"ok"},{"type":"input_image","image_url":"data:image/png;base64,aGVsbG8="}]}]}`)
	redacted := string(redactOpenCodeGeneratedImagesForOps(body))
	require.NotContains(t, redacted, "data:image")
	require.NotContains(t, redacted, "aGVsbG8=")
	require.Contains(t, redacted, "[redacted-input-image]")
}
```

- [ ] **Step 2: 运行失败测试**

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run "TestRedactOpenCodeGeneratedImagesForOpsRedactsFunctionOutputArrayImage" -count=1
```

Expected: FAIL only if recursive redaction misses output arrays。

- [ ] **Step 3: Ensure named runtime/ops paths use redaction helper**

Update these concrete `openai_gateway_service.go` paths before any log, ops event, or persisted request-detail write:

- Non-WS failover detail at `shouldFailoverOpenAIUpstreamResponse` caller around lines 3693-3710: sanitize `upstreamDetail` before `appendOpsUpstreamError`.
- `handleErrorResponse` around lines 4823-4842: sanitize `upstreamDetail` before `setOpsUpstreamError`, sanitize `truncateForLog(body, ...)` before `logger.LegacyPrintf`, and keep `logOpenAIInstructionsRequiredDebug` arguments redacted if it logs request/response bodies.
- Passthrough error helpers around lines 4128-4335 and 4990+: sanitize any `upstreamDetail`, `ErrorBody`, and `logger.LegacyPrintf` payloads.
- `setOpsUpstreamRequestBody(c, body)` callers must receive already-redacted bytes through `redactOpenCodeGeneratedImagesForOps(body)` when the body may include OpenCode image tool output arrays.
- Request-detail persistence that consumes `OpsUpstreamRequestBodyKey`, `UpstreamErrorsJSON`, `RequestBodyJSON`, or `UpstreamErrorDetail` must see redacted content. If redaction currently only happens in display helpers, move it earlier to the value stored in gin context / repository input.

Required pattern:

```go
safeDetail := redactOpenCodeGeneratedImageTokensForOps(rawDetail)
```

For JSON bytes:

```go
safeBody := redactOpenCodeGeneratedImagesForOps(respBody)
```

- [ ] **Step 4: Add end-to-end redaction test**

Add `TestOpenCodeImageToolOutputRedactsRuntimeAndOpsErrorBodies` in `openai_gateway_service_test.go`. Split the assertion into two explicit phases so it does not depend on handler middleware magically running during `svc.Forward`:

Phase A calls `svc.Forward` with `svc := newOpenAITestGatewayService(fakeHTTPUpstream)` and `svc.cfg.Gateway.LogUpstreamErrorBody = true`; fake upstream returns HTTP 400 with a response body containing `data:image/png;base64,aGVsbG8=`. Use an OpenCode request body containing a synthetic `function_call_output.output` array image. Assert gin context values and runtime logs are redacted.

Phase B constructs `OpsService{opsRepo: &opsRepoMock{...}}`, builds an `OpsInsertErrorLogInput` from the redacted context values produced in Phase A, then calls `opsSvc.RecordError(context.Background(), entry, rawRequestBody)` with a raw request body containing the same `function_call_output.output[].image_url` data URL. This exercises `PrepareOpsRequestBodyForQueue` and the repository input path. Add a second `opsSvc.RecordErrorBatch(context.Background(), []*OpsInsertErrorLogInput{entryWithUpstreamErrors})` assertion for `UpstreamErrorsJSON` sanitization. Do not expect `svc.Forward` alone to invoke `OpsErrorLoggerMiddleware` or repository writes.

Capture runtime logs with the existing helper from `openai_oauth_passthrough_test.go`; do not use bare `logger.SetSink` without initializing logger because `LegacyPrintf` only reaches the sink after logger initialization:

```go
logSink, restoreLogs := captureStructuredLog(t)
t.Cleanup(restoreLogs)
```

Use the existing service test mock `opsRepoMock` from `internal/service/ops_repo_mock_test.go`. Configure `InsertErrorLogFn` and `BatchInsertErrorLogsFn` to copy captured `*OpsInsertErrorLogInput` values into a local slice:

```go
var captured []*OpsInsertErrorLogInput
opsSvc := &OpsService{opsRepo: &opsRepoMock{
	InsertErrorLogFn: func(ctx context.Context, input *OpsInsertErrorLogInput) (int64, error) {
		captured = append(captured, input)
		return 1, nil
	},
	BatchInsertErrorLogsFn: func(ctx context.Context, inputs []*OpsInsertErrorLogInput) (int64, error) {
		captured = append(captured, inputs...)
		return int64(len(inputs)), nil
	},
}}
```

Assert these strings do not contain `data:image` or `aGVsbG8=`:

- `OpsUpstreamRequestBodyKey` from gin context.
- `UpstreamErrorDetail` from gin context / ops input.
- `UpstreamErrorsJSON` and `RequestBodyJSON` passed to repository/request detail persistence.
- every captured runtime log event `Message` and string-valued `Fields`.

Also assert `require.NotEmpty(t, logSink.events)` and at least one captured event message contains `OpenAI upstream error` or the upstream status code, so the runtime log assertion cannot pass vacuously.

Use this complete test shape:

```go
func TestOpenCodeImageToolOutputRedactsRuntimeAndOpsErrorBodies(t *testing.T) {
	rawRequestBody := []byte(`{"model":"gpt-5.5","input":[{"type":"function_call_output","call_id":"call_sub2api_image_img_abcdefghijklmnopqrstuvwxyzABCDEF","output":[{"type":"input_text","text":"ok"},{"type":"input_image","image_url":"data:image/png;base64,aGVsbG8="}]}]}`)
	upstream := &stubHTTPUpstream{response: &http.Response{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"bad","detail":"data:image/png;base64,aGVsbG8="}}`))}}
	svc := newOpenAITestGatewayService(upstream)
	svc.cfg.Gateway.LogUpstreamErrorBody = true
	svc.cfg.Gateway.LogUpstreamErrorBodyMaxBytes = 4096
	c, _ := newOpenAITestContext(t, "/v1/responses", rawRequestBody)
	logSink, restoreLogs := captureStructuredLog(t)
	t.Cleanup(restoreLogs)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}
	_, err := svc.Forward(context.Background(), c, account, rawRequestBody)
	require.Error(t, err)

	for _, key := range []string{OpsUpstreamRequestBodyKey, OpsUpstreamErrorDetailKey} {
		if value, ok := c.Get(key); ok {
			text := fmt.Sprint(value)
			require.NotContains(t, text, "data:image")
			require.NotContains(t, text, "aGVsbG8=")
		}
	}
	require.NotEmpty(t, logSink.events)
	foundErrorLog := false
	for _, event := range logSink.events {
		if strings.Contains(event.Message, "OpenAI upstream error") || strings.Contains(event.Message, "400") {
			foundErrorLog = true
		}
		require.NotContains(t, event.Message, "data:image")
		require.NotContains(t, event.Message, "aGVsbG8=")
		for _, field := range event.Fields {
			text := fmt.Sprint(field)
			require.NotContains(t, text, "data:image")
			require.NotContains(t, text, "aGVsbG8=")
		}
	}
	require.True(t, foundErrorLog)

	var captured []*OpsInsertErrorLogInput
	opsSvc := &OpsService{opsRepo: &opsRepoMock{InsertErrorLogFn: func(ctx context.Context, input *OpsInsertErrorLogInput) (int64, error) {
		captured = append(captured, input)
		return 1, nil
	}, BatchInsertErrorLogsFn: func(ctx context.Context, inputs []*OpsInsertErrorLogInput) (int64, error) {
		captured = append(captured, inputs...)
		return int64(len(inputs)), nil
	}}}
	detail := `data:image/png;base64,aGVsbG8=`
	require.NoError(t, opsSvc.RecordError(context.Background(), &OpsInsertErrorLogInput{ErrorPhase: "upstream", ErrorType: "upstream_error", UpstreamErrorDetail: &detail}, rawRequestBody))
	require.NoError(t, opsSvc.RecordErrorBatch(context.Background(), []*OpsInsertErrorLogInput{{ErrorPhase: "upstream", ErrorType: "upstream_error", UpstreamErrors: []*OpsUpstreamErrorEvent{{Kind: "http_error", Message: "bad", Detail: detail, UpstreamResponseBody: detail, UpstreamRequestBody: string(rawRequestBody)}}}}))
	require.NotEmpty(t, captured)
	for _, input := range captured {
		for _, value := range []*string{input.RequestBodyJSON, input.UpstreamErrorDetail, input.UpstreamErrorsJSON} {
			if value == nil { continue }
			require.NotContains(t, *value, "data:image")
			require.NotContains(t, *value, "aGVsbG8=")
		}
	}
}
```

The test should fail if only `redactOpenCodeGeneratedImagesForOps` helper output is tested but the real runtime log or ops path still stores raw base64.

- [ ] **Step 5: Run redaction tests**

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run "RedactOpenCode|OpenCodeImageToolOutputRedactsRuntimeAndOpsErrorBodies|OpsServiceRecordErrorBatch|PrepareOpsRequestBody|ImageToolOutput" -count=1
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test ./internal/handler -run "OpsErrorLogger|OpenAIGateway|GeneratedImage" -count=1
```

Expected: PASS。

---

### Task 8: captured-style fixture、non-OpenCode 隔离与最终验证

**Files:**
- Test: `backend/internal/service/openai_opencode_image_rewrite_test.go`
- Test: `backend/internal/service/openai_gateway_service_test.go`
- Test: `backend/internal/handler/openai_gateway_handler_test.go`
- Docs: no additional docs unless implementation changes plan assumptions

- [ ] **Step 1: 写 captured-style fixture 测试**

Create a sanitized payload in a test helper and a service-level test that proves compressed legacy markers are ignored while the valid assistant marker is inserted near its source:

```go
func openCodeImageRehydrateCapturedStyleBody() []byte {
	return []byte(`{"model":"gpt-5.5-Sys","store":false,"reasoning":{"effort":"xhigh","summary":"auto"},"include":["reasoning.encrypted_content"],"input":[{"role":"user","content":[{"type":"input_text","text":"[Compressed conversation section]\nGenerated image: sub2api-image://img_oldoldoldoldoldoldoldoldoldoldoldold"}]},{"id":"msg_sub2api_img_abcdefghijklmnopqrstuvwxyzABCDEF","role":"assistant","content":[{"type":"output_text","text":"Generated image saved by sub2api.\nImage reference: [[sub2api-generated-image:id=img_abcdefghijklmnopqrstuvwxyzABCDEF]]"}]},{"role":"user","content":[{"type":"input_text","text":"Continue"}]}]}`)
}

func TestRehydrateOpenCodeGeneratedImageMarkersCapturedStyleSkipsCompressedLegacy(t *testing.T) {
	var req map[string]any
	require.NoError(t, json.Unmarshal(openCodeImageRehydrateCapturedStyleBody(), &req))
	store := newTestStoreWithImage(t, testImageID, "png", pngBytes)
	changed, err := rehydrateOpenCodeGeneratedImageMarkers(context.Background(), req, store, openCodeImageRehydrateOptions{MaxImages: 3})
	require.NoError(t, err)
	require.True(t, changed)
	input := req["input"].([]any)
	require.Equal(t, "function_call", input[2].(map[string]any)["type"])
	require.Equal(t, "function_call_output", input[3].(map[string]any)["type"])
	require.Equal(t, "Continue", gjson.GetBytes(mustJSONBytes(t, input[4]), "content.0.text").String())
	encoded := string(mustJSONBytes(t, req))
	require.Contains(t, encoded, "call_sub2api_image_"+testImageID)
	require.NotContains(t, encoded, "call_sub2api_image_img_oldoldoldoldoldoldoldoldoldoldoldold")
}
```

Add a handler-level captured-style prepare test that uses the real service rehydrate hook and proves final tail/target group are consistent:

```go
func openCodeImageRehydrateCapturedStyleHandlerBody() []byte {
	return []byte(`{"model":"gpt-5.5-Sys","store":false,"reasoning":{"effort":"xhigh","summary":"auto"},"include":["reasoning.encrypted_content"],"input":[{"role":"user","content":[{"type":"input_text","text":"[Compressed conversation section]\nGenerated image: sub2api-image://img_oldoldoldoldoldoldoldoldoldoldoldold"}]},{"id":"msg_sub2api_img_abcdefghijklmnopqrstuvwxyzABCDEF","role":"assistant","content":[{"type":"output_text","text":"Generated image saved by sub2api.\nImage reference: [[sub2api-generated-image:id=img_abcdefghijklmnopqrstuvwxyzABCDEF]]"}]},{"role":"user","content":[{"type":"input_text","text":"Continue"}]}]}`)
}

func TestPrepareResponsesRequestForSchedulingCapturedStyleKeepsSysDummyTail(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "opencode/1.14.31")
	hook := func(_ *gin.Context, reqBody map[string]any) (bool, error) {
		input := reqBody["input"].([]any)
		// The service-level captured-style test verifies scanner/store behavior.
		// This handler test only proves an inserted image pair stays before sys dummy.
		reqBody["input"] = append(input[:2], append([]any{
			map[string]any{"type": "function_call", "call_id": "call_sub2api_image_img_abcdefghijklmnopqrstuvwxyzABCDEF", "name": "sub2api_generated_image", "arguments": "{}"},
			map[string]any{"type": "function_call_output", "call_id": "call_sub2api_image_img_abcdefghijklmnopqrstuvwxyzABCDEF", "output": []any{map[string]any{"type": "input_text", "text": "restored"}}},
		}, input[2:]...)...)
		return true, nil
	}
	body, model, targetGroup, err := prepareResponsesRequestForScheduling(c, openCodeImageRehydrateCapturedStyleHandlerBody(), "gpt-5.5-Sys", hook)
	require.NoError(t, err)
	require.Equal(t, "gpt-5.5", model)
	require.Equal(t, service.TargetGroupExhausted, targetGroup)
	var req map[string]any
	require.NoError(t, json.Unmarshal(body, &req))
	input := req["input"].([]any)
	require.Equal(t, "function_call_output", input[len(input)-1].(map[string]any)["type"])
	require.Equal(t, "sys_dummy", input[len(input)-1].(map[string]any)["call_id"])
	require.Equal(t, "function_call", input[2].(map[string]any)["type"])
	require.Equal(t, "function_call_output", input[3].(map[string]any)["type"])
	require.NotContains(t, string(body), "call_sub2api_image_img_oldoldoldoldoldoldoldoldoldoldoldold")
}
```

- [ ] **Step 2: 写 non-OpenCode 隔离测试**

Add a service method isolation test:

```go
func TestRehydrateOpenCodeGeneratedImagesForResponsesSkipsNonOpenCodeClient(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "curl/8.0")
	svc := &OpenAIGatewayService{generatedImageStore: newTestStoreWithImage(t, testImageID, "png", pngBytes)}
	req := map[string]any{"input": []any{map[string]any{"id": "msg_sub2api_img_" + strings.TrimPrefix(testImageID, "img_"), "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "Generated image saved by sub2api.\nImage reference: [[sub2api-generated-image:id=" + testImageID + "]]"}}}}}
	changed, err := svc.RehydrateOpenCodeGeneratedImagesForResponses(context.Background(), c, req)
	require.NoError(t, err)
	require.False(t, changed)
	require.NotContains(t, string(mustJSONBytes(t, req)), "sub2api_generated_image")
}
```

Keep or add the existing non-OpenCode image response rewrite test asserting standard `image_generation_call` output is still exposed; this is independent from marker rehydrate and protects non-OpenCode clients from the synthetic tool pair path.

- [ ] **Step 3: Run focused tests**

Before running the focused suite, update existing tests whose assertions intentionally change with this feature:

```text
backend/internal/service/openai_opencode_image_rewrite_test.go: update tests that expected tail `role:user` synthetic messages or `sub2api-image://` output so they now expect nearby `function_call` / `function_call_output` pairs and `[[sub2api-generated-image:id=...]]` sentinels.
backend/internal/service/openai_opencode_image_sse_test.go: update image-generation SSE response rewrite assertions so generated assistant text uses the new sentinel and no longer emits naked legacy marker strings.
backend/internal/service/openai_gateway_service_test.go: update `TestForwardResponsesRequest_OpenCode...` tests that assumed `Forward` performs rehydrate or that ops redaction sees tail synthetic user messages. New expectations: handler/prepare performs rehydrate, preinserted synthetic image pairs remain idempotent in `Forward`, and ops redaction sees `function_call_output.output` arrays.
```

Use the repository content search tool on `backend/internal/service/*opencode*test.go` and `backend/internal/service/openai_gateway_service_test.go` for `sub2api-image://|Attached generated image|role.*user|rehydrate` and update every assertion that refers to the old rehydrate shape. Do not weaken those tests by only removing assertions; replace them with assertions for the new tool-output shape or with a preinserted synthetic pair where the test is now about `Forward` idempotency.

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test ./internal/handler -run "PrepareResponsesRequestForScheduling|OpenAIGateway|GeneratedImage" -count=1
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run "OpenCode|GeneratedImage|Rehydrate|RedactOpenCode|ToolContinuation|ImageToolOutput" -count=1
```

Expected: PASS。

- [ ] **Step 4: Run final verification suite from spec**

```powershell
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test -tags unit ./internal/service -run "OpenCode|GeneratedImage|Rehydrate|RedactOpenCode|ToolContinuation|OpsServiceRecordErrorBatch|PrepareOpsRequestBody" -count=1
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test ./internal/handler -run "OpsErrorLogger|OpenAIGateway|GeneratedImage" -count=1
& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test ./internal/repository -run "Ops(ErrorLog|RepositoryListRequestDetails)|RequestType" -count=1
```

Expected: PASS. Before running, use `& "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe" test ./internal/repository -list "Ops(ErrorLog|RepositoryListRequestDetails)|RequestType"` to confirm the regex includes request-detail and request-type tests, not only error-log tests.

- [ ] **Step 5: Run formatting and diff checks**

```powershell
$goFiles = @((rtk git diff --name-only -- "*.go") + (rtk git diff --cached --name-only -- "*.go") + (rtk git ls-files --others --exclude-standard -- "*.go")) | Where-Object { $_ -ne "" } | Sort-Object -Unique
if ($goFiles.Count -gt 0) { & "C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\gofmt.exe" -w $goFiles }
rtk git diff --check
rtk git status --short
```

Expected: `git diff --check` has no output; `git status --short` lists only intended files.

---

## Plan Self-Review

**Spec coverage:**
- 工具输出化：Tasks 3, 6.
- 调度前 rehydrate 和 `-Sys` tail：Task 5.
- marker 特异性和 legacy 机械规则：Tasks 1, 2.
- 不可用去重：Task 4.
- ops/runtime redaction：Task 7.
- captured-style/non-OpenCode/final verification：Task 8.

**占位内容扫描:** 未留下未定标记或未定实现项。任何实现不确定性都已表达为测试或具体 fallback 规则。

**Type consistency:** Plan uses existing `map[string]any`, `[]any`, `OpenAIGeneratedImageStore`, `openCodeImageRehydrateOptions`, `function_call`, `function_call_output`, `input_text`, `input_image`, and existing target group helpers.
