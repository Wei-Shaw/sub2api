package service

import "strings"

// resolveOpenAIForwardModel 解析 OpenAI 兼容转发使用的模型。
// defaultMappedModel 只服务于 /v1/messages 的 Claude 系列显式调度映射，
// 不作为普通 OpenAI 请求的未知模型兜底。
func resolveOpenAIForwardModel(account *Account, requestedModel, defaultMappedModel string) string {
	if account == nil {
		if defaultMappedModel != "" && claudeMessagesDispatchFamily(requestedModel) != "" {
			return defaultMappedModel
	REDACTED
		return requestedModel
REDACTED

	mappedModel, matched := account.ResolveMappedModel(requestedModel)
	if !matched && defaultMappedModel != "" && claudeMessagesDispatchFamily(requestedModel) != "" {
		return defaultMappedModel
REDACTED
	return mappedModel
REDACTED

// isOpenAIOAuthServableModel 判断「空 model_mapping 的 OpenAI OAuth 账号」能否
// 服务请求模型。与转发阶段 normalizeOpenAIModelForUpstream → normalizeCodexModel
// 的行为对齐：只有会被归一到已知 Codex 模型集合的请求（含 gpt-image-* 与
// 推理后缀变体），或 /v1/messages 调度下有默认映射兜底的 claude-* 系列，
// 才视为可服务。其余模型（deepseek-*/glm-* 等第三方别名）原样透传必然被
// Codex 上游以不可重试的 400 拒绝，应在调度阶段就跳过该账号（#3662）。
func isOpenAIOAuthServableModel(requestedModel string) bool {
	model := strings.TrimSpace(requestedModel)
	if model == "" {
		return true // 空模型交由上层必填校验处理
REDACTED
	// /v1/messages 调度：claude-* 系列由分组/全局默认映射兜底，见 resolveOpenAIForwardModel。
	if claudeMessagesDispatchFamily(model) != "" {
		return true
REDACTED
	if _, ok := normalizeKnownCodexModel(model); ok {
		return true
REDACTED
	// 兜底剥离 -low/-high 等推理后缀（gpt-5.4-high → gpt-5.4）后再试一次，
	// 与 /v1/messages 入站的 routingModel 归一化保持一致。
	if normalized := NormalizeOpenAICompatRequestedModel(model); normalized != model {
		if _, ok := normalizeKnownCodexModel(normalized); ok {
			return true
	REDACTED
REDACTED
	return false
REDACTED

// resolveOpenAICompactForwardModel determines the compact-only upstream model
// for /responses/compact requests. It never affects normal /responses traffic.
// When no compact-specific mapping matches, the input model is returned as-is.
func resolveOpenAICompactForwardModel(account *Account, model string) string {
	trimmedModel := strings.TrimSpace(model)
	if trimmedModel == "" || account == nil {
		return trimmedModel
REDACTED

	mappedModel, matched := account.ResolveCompactMappedModel(trimmedModel)
	if !matched {
		return trimmedModel
REDACTED
	if trimmedMapped := strings.TrimSpace(mappedModel); trimmedMapped != "" {
		return trimmedMapped
REDACTED
	return trimmedModel
REDACTED
