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

// openAIOAuthForeignModelPrefixes 列出明确属于其他厂商家族的模型名前缀。
// Codex 上游不可能服务这些模型：转发阶段 normalizeOpenAIModelForUpstream
// 对未知模型原样透传，上游必然返回不可重试的 400。
//
// 采用保守黑名单而非 Codex 模型白名单：未知/自定义别名保持「允许」，
// 以兼容渠道级模型映射等「账号选定之后才改写模型名」的部署方式
// （调度过滤看到的是改写前的原始模型名）。前缀分类的先例见
// ResolveThinkingProtocol（thinking_protocol.go）。
var openAIOAuthForeignModelPrefixes = []string{
	"deepseek",
	"glm-",
	"kimi-",
	"moonshot",
	"qwen",
	"qwq-",
	"minimax",
	"gemini-",
	"gemma-",
	"grok-",
	"doubao-",
	"hunyuan-",
	"llama",
	"meta-llama",
	"mistral",
	"mixtral",
	"baichuan",
	"ernie-",
	"step-",
	"seed-",
	"yi-",
REDACTED

// isOpenAIOAuthServableModel 判断「空 model_mapping 的 OpenAI OAuth 账号」能否
// 服务请求模型。空映射默认仍是「允许」，仅排除明确属于其他厂商家族的模型
// （deepseek-*/glm-* 等）——这类请求原样透传必然被 Codex 上游以不可重试的
// 400 拒绝，且不触发 failover，应在调度阶段就跳过该账号，把请求让给
// 显式声明支持该模型的账号（#3662）。
func isOpenAIOAuthServableModel(requestedModel string) bool {
	model := strings.ToLower(lastOpenAIModelSegment(requestedModel))
	if model == "" {
		return true // 空模型交由上层必填校验处理
REDACTED
	for _, prefix := range openAIOAuthForeignModelPrefixes {
		if strings.HasPrefix(model, prefix) {
			return false
	REDACTED
REDACTED
	return true
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
