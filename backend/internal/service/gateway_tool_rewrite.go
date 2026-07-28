package service

import (
	"fmt"
	"hash/fnv"
	"math/rand"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// toolNameRewriteKey 是 gin.Context 上存 ToolNameRewrite 映射的 key。
// 请求阶段写入，响应阶段读取，用于 bytes 级逆向还原假名 → 真名。
const toolNameRewriteKey = "claude_tool_name_rewrite"

// staticToolNameRewrites 是"静态前缀映射"，与 Parrot src/transform/cc_mimicry.py
// TOOL_NAME_REWRITES 完全一致。只有以这些前缀开头的工具会被重写。
var staticToolNameRewrites = map[string]string{
	"sessions_": "cc_sess_",
	"session_":  "cc_ses_",
REDACTED

// fakeToolNamePrefixes 是"动态映射"的前缀池，与 Parrot _FAKE_PREFIXES 一致。
// 当 tools 数量 > dynamicToolMapThreshold 时随机选用其中前缀生成可读假名。
var fakeToolNamePrefixes = []string{
	"analyze_", "compute_", "fetch_", "generate_", "lookup_", "modify_",
	"process_", "query_", "render_", "resolve_", "sync_", "update_",
	"validate_", "convert_", "extract_", "manage_", "monitor_", "parse_",
	"review_", "search_", "transform_", "handle_", "invoke_", "notify_",
REDACTED

// dynamicToolMapThreshold 与 Parrot 一致：tools 数量超过 5 才启用动态映射。
// 少量工具不需要混淆（一般是 Claude Code 自己的核心工具 bash/edit/read 等）。
const dynamicToolMapThreshold = 5

// ToolNameRewrite 是单次请求内的工具名混淆映射。
//   - Forward: real → fake，请求阶段在 body 上应用。
//   - Reverse: fake → real，响应阶段对每个 chunk 做 bytes.Replace 还原。
//
// ReverseOrdered 是按假名长度倒序的 (fake, real) 列表，用于防止短假名是长假名的
// 子串时 bytes.Replace 先被吃掉（对齐 Parrot _restore_tool_names_in_chunk 的
// `sorted(..., key=lambda x: len(x[1]), reverse=True)`）。
type ToolNameRewrite struct {
	Forward        map[string]string
	Reverse        map[string]string
	ReverseOrdered [][2]string
REDACTED

// buildDynamicToolMap 构造 tools 的动态假名映射。
//
// 与 Parrot _build_dynamic_tool_map 语义等价：
//   - tools 数量 ≤ dynamicToolMapThreshold 时返回 nil（不做动态映射，走静态 fallback）
//   - 同一组 tool_names 在同进程内映射稳定（保证 cache 命中）
//
// Parrot 用 `random.Random(hash(tuple(tool_names)))` 作 seed + shuffle 前缀池；
// Go 无法字节级复刻 Python hash，但"稳定性"和"前缀池打散"两个不变量都保留：
// 用 fnv64a(strings.Join(names, "\x00")) 作 seed 喂 math/rand.New。
// 字节级不同不影响上游判定（Anthropic 不会验证我们的随机种子算法）。
func buildDynamicToolMap(toolNames []string) map[string]string {
	if len(toolNames) <= dynamicToolMapThreshold {
		return nil
REDACTED
	h := fnv.New64a()
	for i, n := range toolNames {
		if i > 0 {
			_, _ = h.Write([]byte{0REDACTED)
	REDACTED
		_, _ = h.Write([]byte(n))
REDACTED
	rng := rand.New(rand.NewSource(int64(h.Sum64())))

	available := make([]string, len(fakeToolNamePrefixes))
	copy(available, fakeToolNamePrefixes)
	rng.Shuffle(len(available), func(i, j int) { available[i], available[j] = available[j], available[i] REDACTED)

	mapping := make(map[string]string, len(toolNames))
	for i, name := range toolNames {
		prefix := available[i%len(available)]
		headLen := 3
		if len(name) < 3 {
			headLen = len(name)
	REDACTED
		fake := fmt.Sprintf("%s%s%02d", prefix, name[:headLen], i)
		mapping[name] = fake
REDACTED
	return mapping
REDACTED

// sanitizeToolName 把真名转成假名。
// 与 Parrot _sanitize_tool_name 语义一致：动态映射优先，再走静态前缀映射。
func sanitizeToolName(name string, dynamic map[string]string) string {
	if dynamic != nil {
		if fake, ok := dynamic[name]; ok {
			return fake
	REDACTED
REDACTED
	for prefix, replacement := range staticToolNameRewrites {
		if strings.HasPrefix(name, prefix) {
			return replacement + name[len(prefix):]
	REDACTED
REDACTED
	return name
REDACTED

// shouldMimicToolName 指示某个 tool 是否需要重命名。
// server tool（type != "" 且不是 "function" / "custom"）是 Anthropic 协议语义的一部分，
// 比如 "web_search_20250305" / "computer_20250124"；误改会导致上游拒绝。
func shouldMimicToolName(toolType string) bool {
	if toolType == "" || toolType == "function" || toolType == "custom" {
		return true
REDACTED
	return false
REDACTED

// buildToolNameRewriteFromBody 扫描 body 的 tools[*].name，构造 ToolNameRewrite
// 并返回它。若不需要混淆（tools 数量不足 + 没有匹配静态前缀的工具）返回 nil。
//
// 注意：只扫描，不改 body。真正的 body 改写在 applyToolNameRewriteToBody。
func buildToolNameRewriteFromBody(body []byte) *ToolNameRewrite {
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return nil
REDACTED

	mimicableNames := make([]string, 0)
	toolsArr := tools.Array()
	for _, t := range toolsArr {
		if !shouldMimicToolName(t.Get("type").String()) {
			continue
	REDACTED
		name := t.Get("name").String()
		if name == "" {
			continue
	REDACTED
		mimicableNames = append(mimicableNames, name)
REDACTED

	dynamic := buildDynamicToolMap(mimicableNames)

	rw := &ToolNameRewrite{
		Forward: make(map[string]string),
		Reverse: make(map[string]string),
REDACTED
	for _, name := range mimicableNames {
		fake := sanitizeToolName(name, dynamic)
		if fake == name {
			continue
	REDACTED
		rw.Forward[name] = fake
		rw.Reverse[fake] = name
REDACTED
	if len(rw.Forward) == 0 {
		return nil
REDACTED

	rw.ReverseOrdered = make([][2]string, 0, len(rw.Reverse))
	for fake, real := range rw.Reverse {
		rw.ReverseOrdered = append(rw.ReverseOrdered, [2]string{fake, realREDACTED)
REDACTED
	sort.SliceStable(rw.ReverseOrdered, func(i, j int) bool {
		return len(rw.ReverseOrdered[i][0]) > len(rw.ReverseOrdered[j][0])
REDACTED)

	return rw
REDACTED

// applyToolNameRewriteToBody 把已构造的 ToolNameRewrite 应用到 body 上：
//
//   - 改写 $.tools[*].name（仅对 shouldMimicToolName 通过的 tool）
//   - 改写 $.tool_choice.name（仅当 $.tool_choice.type == "tool"）
//   - 改写 $.messages[*].content[*].name（仅当 type == "tool_use"）
//   - 在 $.tools[last].cache_control 上打 ephemeral 缓存断点
//
// 响应侧 bytes.Replace 会连带还原假名 → 真名。
func applyToolNameRewriteToBody(body []byte, rw *ToolNameRewrite) []byte {
	if rw == nil || len(rw.Forward) == 0 {
		body = applyToolsLastCacheBreakpoint(body)
		return body
REDACTED

	tools := gjson.GetBytes(body, "tools")
	if tools.IsArray() {
		idx := -1
		tools.ForEach(func(_, t gjson.Result) bool {
			idx++
			if !shouldMimicToolName(t.Get("type").String()) {
				return true
		REDACTED
			name := t.Get("name").String()
			if name == "" {
				return true
		REDACTED
			fake, ok := rw.Forward[name]
			if !ok {
				return true
		REDACTED
			if next, err := sjson.SetBytes(body, fmt.Sprintf("tools.%d.name", idx), fake); err == nil {
				body = next
		REDACTED
			return true
	REDACTED)
REDACTED

	if tc := gjson.GetBytes(body, "tool_choice"); tc.Exists() && tc.Get("type").String() == "tool" {
		name := tc.Get("name").String()
		if fake, ok := rw.Forward[name]; ok {
			if next, err := sjson.SetBytes(body, "tool_choice.name", fake); err == nil {
				body = next
		REDACTED
	REDACTED
REDACTED

	// 同步改写历史消息中的 tool_use.name，确保它和 tools[] 中的假名一致。
	// 否则 Anthropic 会因为 tool_use 引用了未声明的原始工具名而拒绝请求。
	messages := gjson.GetBytes(body, "messages")
	if messages.IsArray() {
		messages.ForEach(func(msgKey, msg gjson.Result) bool {
			msgIdx := int(msgKey.Num)
			content := msg.Get("content")
			if !content.IsArray() {
				return true
		REDACTED
			content.ForEach(func(blkKey, blk gjson.Result) bool {
				blkIdx := int(blkKey.Num)
				if blk.Get("type").String() != "tool_use" {
					return true
			REDACTED
				name := blk.Get("name").String()
				if name == "" {
					return true
			REDACTED
				if fake, ok := rw.Forward[name]; ok {
					path := fmt.Sprintf("messages.%d.content.%d.name", msgIdx, blkIdx)
					if next, err := sjson.SetBytes(body, path, fake); err == nil {
						body = next
				REDACTED
			REDACTED
				return true
		REDACTED)
			return true
	REDACTED)
REDACTED

	body = applyToolsLastCacheBreakpoint(body)
	return body
REDACTED

// applyToolsLastCacheBreakpoint 在最后一个非延迟加载工具上注入 cache_control
// 断点。Anthropic 不允许 custom.defer_loading=true 的工具携带 cache_control，
// 因此会先清理所有延迟加载工具上的客户端断点。其余行为对齐 Parrot
// `tools[-1]["cache_control"] = {"type":"ephemeral","ttl":"1h"REDACTED`，
// 但 ttl 按本仓规则：
//   - 客户端已为该 tool 显式设置 cache_control.ttl → 完全透传不覆盖
//   - 否则注入 {"type":"ephemeral","ttl": claude.DefaultCacheControlTTLREDACTED
//
// 纯副作用函数，tools 不存在或为空数组时 no-op。
func applyToolsLastCacheBreakpoint(body []byte) []byte {
	body = stripDeferredToolCacheControl(body)
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return body
REDACTED
	arr := tools.Array()
	if len(arr) == 0 {
		return body
REDACTED
	lastIdx := -1
	for idx, tool := range arr {
		if isDeferredLoadingTool(tool) {
			continue
	REDACTED
		lastIdx = idx
REDACTED
	if lastIdx == -1 {
		return body
REDACTED

	existingCC := arr[lastIdx].Get("cache_control")

	if existingCC.Exists() && existingCC.Get("ttl").String() != "" {
		return body
REDACTED

	if existingCC.Exists() {
		if next, err := sjson.SetBytes(body, fmt.Sprintf("tools.%d.cache_control.ttl", lastIdx), claude.DefaultCacheControlTTL); err == nil {
			body = next
	REDACTED
		return body
REDACTED

	raw := fmt.Sprintf(`{"type":"ephemeral","ttl":%qREDACTED`, claude.DefaultCacheControlTTL)
	if next, err := sjson.SetRawBytes(body, fmt.Sprintf("tools.%d.cache_control", lastIdx), []byte(raw)); err == nil {
		body = next
REDACTED
	return body
REDACTED

func isDeferredLoadingTool(tool gjson.Result) bool {
	return tool.Get("custom.defer_loading").Type == gjson.True
REDACTED

// stripDeferredToolCacheControl removes the cache marker Anthropic rejects on
// deferred tools. Only the literal JSON boolean true enables deferred loading.
func stripDeferredToolCacheControl(body []byte) []byte {
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return body
REDACTED
	for idx, tool := range tools.Array() {
		if !isDeferredLoadingTool(tool) || !tool.Get("cache_control").Exists() {
			continue
	REDACTED
		if next, err := sjson.DeleteBytes(body, fmt.Sprintf("tools.%d.cache_control", idx)); err == nil {
			body = next
	REDACTED
REDACTED
	return body
REDACTED

// restoreToolNamesInBytes 对 bytes chunk 做逆向还原：假名 → 真名。
// 按 ReverseOrdered 的假名长度倒序逐个 bytes.Replace，防止子串冲突
// （与 Parrot _restore_tool_names_in_chunk 的 sorted(..., reverse=True) 等价）。
// 再做静态前缀还原（cc_sess_ → sessions_ / cc_ses_ → session_）。
//
// rw 可为 nil；nil 时仍会做静态前缀还原。
func restoreToolNamesInBytes(data []byte, rw *ToolNameRewrite) []byte {
	if rw != nil {
		for _, pair := range rw.ReverseOrdered {
			fake, real := pair[0], pair[1]
			if fake == "" || fake == real {
				continue
		REDACTED
			data = replaceAllBytes(data, fake, real)
	REDACTED
REDACTED
	for prefix, replacement := range staticToolNameRewrites {
		data = replaceAllBytes(data, replacement, prefix)
REDACTED
	return data
REDACTED

// replaceAllBytes 是 bytes.ReplaceAll 的便捷封装，避免每个调用点各自做 []byte 转换。
func replaceAllBytes(data []byte, from, to string) []byte {
	if len(data) == 0 || from == to || !strings.Contains(string(data), from) {
		return data
REDACTED
	return []byte(strings.ReplaceAll(string(data), from, to))
REDACTED

// toolNameRewriteFromContext 从 gin.Context 取出请求阶段保存的工具名映射。
// 找不到（c==nil 或 key 不存在或类型不对）时返回 nil；调用方必须能处理 nil。
func toolNameRewriteFromContext(c interface {
	Get(string) (any, bool)
REDACTED) *ToolNameRewrite {
	if c == nil {
		return nil
REDACTED
	raw, ok := c.Get(toolNameRewriteKey)
	if !ok || raw == nil {
		return nil
REDACTED
	rw, _ := raw.(*ToolNameRewrite)
	return rw
REDACTED

// reverseToolNamesIfPresent 是响应侧 5 处注入点的统一封装：从 c 取出 mapping
// 并对 chunk 做 bytes 级假名→真名替换。c 没有 mapping 时仍会做静态前缀还原。
func reverseToolNamesIfPresent(c interface {
	Get(string) (any, bool)
REDACTED, chunk []byte) []byte {
	rw := toolNameRewriteFromContext(c)
	if rw == nil && len(staticToolNameRewrites) == 0 {
		return chunk
REDACTED
	return restoreToolNamesInBytes(chunk, rw)
REDACTED
