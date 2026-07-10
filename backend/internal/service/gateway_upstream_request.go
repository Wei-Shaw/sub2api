package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"

	"github.com/gin-gonic/gin"
)

func (s *GatewayService) buildUpstreamRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token, tokenType, modelID string, reqStream bool, mimicClaudeCode bool) (*http.Request, []byte, error) {
	if account.Platform == PlatformAnthropic && account.Type == AccountTypeServiceAccount {
		req, err := s.buildUpstreamRequestAnthropicVertex(ctx, c, account, body, token, modelID, reqStream)
		return req, body, err
REDACTED

	// 确定目标URL
	targetURL := claudeAPIURL
	if account.Type == AccountTypeAPIKey {
		baseURL := account.GetBaseURL()
		if baseURL != "" {
			validatedURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return nil, nil, err
		REDACTED
			targetURL = validatedURL + "/v1/messages?beta=true"
	REDACTED
REDACTED else if account.IsCustomBaseURLEnabled() {
		customURL := account.GetCustomBaseURL()
		if customURL == "" {
			return nil, nil, fmt.Errorf("custom_base_url is enabled but not configured for account %d", account.ID)
	REDACTED
		validatedURL, err := s.validateUpstreamBaseURL(customURL)
		if err != nil {
			return nil, nil, err
	REDACTED
		targetURL = s.buildCustomRelayURL(validatedURL, "/v1/messages", account)
REDACTED

	clientHeaders := http.Header{REDACTED
	if c != nil && c.Request != nil {
		clientHeaders = c.Request.Header
REDACTED

	// OAuth账号：应用统一指纹和metadata重写（受设置开关控制）
	var fingerprint *Fingerprint
	enableFP, enableMPT := true, false
	if s.settingService != nil {
		enableFP, enableMPT, _ = s.settingService.GetGatewayForwardingSettings(ctx)
REDACTED
	if account.IsOAuth() && s.identityService != nil {
		// 1. 获取或创建指纹（包含随机生成的ClientID）
		fp, err := s.identityService.GetOrCreateFingerprint(ctx, account.ID, clientHeaders)
		if err != nil {
			logger.LegacyPrintf("service.gateway", "Warning: failed to get fingerprint for account %d: %v", account.ID, err)
			// 失败时降级为透传原始headers
	REDACTED else {
			if enableFP {
				fingerprint = fp
		REDACTED

			// 2. 重写metadata.user_id（需要指纹中的ClientID和账号的account_uuid）
			// 如果启用了会话ID伪装，会在重写后替换 session 部分为固定值
			// 当 metadata 透传开启时跳过重写
			if !enableMPT {
				accountUUID := account.GetExtraString("account_uuid")
				if accountUUID != "" && fp.ClientID != "" {
					if newBody, err := s.identityService.RewriteUserIDWithMasking(ctx, body, account, accountUUID, fp.ClientID, fp.UserAgent); err == nil && len(newBody) > 0 {
						body = newBody
				REDACTED
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// 同步 billing header cc_version 与实际发送的 User-Agent 版本
	if fingerprint != nil {
		body = syncBillingHeaderVersion(body, fingerprint.UserAgent)
REDACTED

	// === 计算最终 anthropic-beta header（先于 body sanitize 与 CCH 签名）===
	//
	// 顺序约束：
	//   1) 算 finalBeta（纯函数，不依赖 req.Header；mimicry 路径会忽略客户端 beta，
	//      与原“OAuth + mimicClaudeCode 跳过白名单透传”行为对齐）
	//   2) 按 finalBeta 做能力维度 body sanitize（如 context-management beta 缺失 →
	//      strip body.context_management，与 Bedrock 路径对称）
	//   3) CCH 签名（必须使用 strip 后的 body，否则 hash 与最终 body 不一致 →
	//      被 Anthropic 判 third-party）
	//   4) NewRequest（body 至此最终敲定）
	//   5) 透传白名单 / fingerprint / mimic header / 写入 finalBeta
	policyFilterSet := s.getBetaPolicyFilterSet(ctx, c, account, modelID)
	effectiveDropSet := mergeDropSets(policyFilterSet)
	finalBetaHeader, finalBetaShouldSet := s.computeFinalAnthropicBeta(
		tokenType, mimicClaudeCode, modelID, clientHeaders, body, effectiveDropSet,
	)

	// 账号覆写了 anthropic-beta 时，覆写值即最终上游值（由下方 ApplyHeaderOverrides 写入）：
	// body 能力净化必须以覆写值为准，否则 header/body 不对称会被上游 400。
	if beta, ok := account.HeaderOverrideValue("anthropic-beta"); ok {
		finalBetaHeader, finalBetaShouldSet = beta, true
REDACTED

	// 能力维度 body sanitize：与最终 anthropic-beta header 对称
	if sanitized, changed := sanitizeAnthropicBodyForBetaTokens(body, finalBetaHeader); changed {
		body = sanitized
REDACTED

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
REDACTED

	// 设置认证头（保持原始大小写）
	if tokenType == "oauth" {
		setHeaderRaw(req.Header, "authorization", "Bearer "+token)
REDACTED else {
		setAnthropicAPIKeyAuthHeader(req.Header, account, token)
REDACTED

	// 白名单透传 headers
	// OAuth mimicry 路径：跳过客户端 header 透传，与 Parrot 对齐。
	// Parrot 的 build_upstream_headers 只发 9 个精确 header，不透传任何客户端 header。
	// 透传客户端 header 会引入不一致的 x-stainless-* / anthropic-beta / user-agent /
	// x-claude-code-session-id 等值，和我们注入的伪装 header 冲突，被 Anthropic 判 third-party。
	if tokenType != "oauth" || !mimicClaudeCode {
		for key, values := range clientHeaders {
			lowerKey := strings.ToLower(key)
			if allowedHeaders[lowerKey] {
				wireKey := resolveWireCasing(key)
				for _, v := range values {
					addHeaderRaw(req.Header, wireKey, v)
			REDACTED
		REDACTED
	REDACTED
REDACTED

	// OAuth账号：应用缓存的指纹到请求头（覆盖白名单透传的头）
	if fingerprint != nil {
		s.identityService.ApplyFingerprint(req, fingerprint)
REDACTED

	// 确保必要的headers存在（保持原始大小写）
	if getHeaderRaw(req.Header, "content-type") == "" {
		setHeaderRaw(req.Header, "content-type", "application/json")
REDACTED
	if getHeaderRaw(req.Header, "anthropic-version") == "" {
		setHeaderRaw(req.Header, "anthropic-version", "2023-06-01")
REDACTED
	if tokenType == "oauth" {
		applyClaudeOAuthHeaderDefaults(req)
REDACTED

	// OAuth + mimic Claude Code：强制注入 CLI 指纹相关 header
	// （user-agent/x-stainless-*/x-app/Accept/x-stainless-helper-method/x-client-request-id）
	if tokenType == "oauth" && mimicClaudeCode {
		applyClaudeCodeMimicHeaders(req, reqStream)
REDACTED

	// 写入最终 anthropic-beta header
	// 注：透传分支白名单可能写入了客户端 anthropic-beta，无条件 Del 一次再按 finalBeta
	// 决定是否 set，确保 dropSet 过滤后的结果一定覆盖客户端原始值。
	deleteHeaderAllForms(req.Header, "anthropic-beta")
	if finalBetaShouldSet {
		setHeaderRaw(req.Header, "anthropic-beta", finalBetaHeader)
REDACTED

	// 同步 X-Claude-Code-Session-Id 头：取 body 中已处理的 metadata.user_id 的 session_id 覆盖
	if sessionHeader := getHeaderRaw(req.Header, "X-Claude-Code-Session-Id"); sessionHeader != "" {
		if uid := gjson.GetBytes(body, "metadata.user_id").String(); uid != "" {
			if parsed := ParseMetadataUserID(uid); parsed != nil {
				setHeaderRaw(req.Header, "X-Claude-Code-Session-Id", parsed.SessionID)
		REDACTED
	REDACTED
REDACTED

	// 账号级请求头覆写（仅 anthropic/openai api_key 账号启用时生效；OAuth 路径 no-op）。
	// 放在所有 header 逻辑之后，确保配置值对同名头拥有最终决定权。
	account.ApplyHeaderOverrides(req.Header)

	// === DEBUG: 打印上游转发请求（headers + body 摘要），与 CLIENT_ORIGINAL 对比 ===
	s.debugLogGatewaySnapshot("UPSTREAM_FORWARD", req.Header, body, map[string]string{
		"url":                 req.URL.String(),
		"token_type":          tokenType,
		"mimic_claude_code":   strconv.FormatBool(mimicClaudeCode),
		"fingerprint_applied": strconv.FormatBool(fingerprint != nil),
		"enable_fp":           strconv.FormatBool(enableFP),
		"enable_mpt":          strconv.FormatBool(enableMPT),
REDACTED)

	// Always capture a compact fingerprint line for later error diagnostics.
	// We only print it when needed (or when the explicit debug flag is enabled).
	if c != nil && tokenType == "oauth" {
		c.Set(claudeMimicDebugInfoKey, buildClaudeMimicDebugLine(req, body, account, tokenType, mimicClaudeCode))
REDACTED
	if s.debugClaudeMimicEnabled() {
		logClaudeMimicDebug(req, body, account, tokenType, mimicClaudeCode)
REDACTED

	return req, body, nil
REDACTED

// vertexSupportedBetaTokens 是 Vertex AI 的 Anthropic 端点接受的 anthropic-beta
// 白名单。Vertex 对任何未知 token 直接 HTTP 400，故采用白名单（与 Bedrock 的
// bedrockSupportedBetaTokens 同思路）而非黑名单：未来 Claude Code 新增的、Vertex 尚未
// 支持的 token 天然被剥离。当 Vertex 新增支持某 beta 时在此补充。
//
// 明确排除（issue #3358 中 Vertex 报 400 的 token）：advisor-tool-2026-03-01、
// prompt-caching-scope-2026-01-05、redact-thinking-2026-02-12、
// thinking-token-count-2026-05-13；以及 claude-code-20250219 / oauth-2025-04-20 等
// 客户端身份 beta——Vertex service_account 走 Bearer 鉴权，不需要它们。
var vertexSupportedBetaTokens = map[string]bool{
	"context-1m-2025-08-07":                  true,
	"context-management-2025-06-27":          true,
	"fine-grained-tool-streaming-2025-05-14": true,
	"REDACTED":        true,
REDACTED

// filterVertexBetaTokens 解析 client 的 anthropic-beta header，先剔除 drop 集合中的
// token（BetaPolicy filter + 默认 drop），再只保留 Vertex 支持的 token，去重后逗号拼接。
// 返回最终 header（可能为空字符串）。
func filterVertexBetaTokens(header string, drop map[string]struct{REDACTED) string {
	tokens := parseAnthropicBetaHeader(header)
	if len(tokens) == 0 {
		return ""
REDACTED
	out := make([]string, 0, len(tokens))
	seen := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		if _, dropped := drop[t]; dropped {
			continue
	REDACTED
		if !vertexSupportedBetaTokens[t] {
			continue
	REDACTED
		if seen[t] {
			continue
	REDACTED
		seen[t] = true
		out = append(out, t)
REDACTED
	return strings.Join(out, ",")
REDACTED

func (s *GatewayService) buildUpstreamRequestAnthropicVertex(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
	modelID string,
	reqStream bool,
) (*http.Request, error) {
	vertexBody, err := buildVertexAnthropicRequestBody(body)
	if err != nil {
		return nil, err
REDACTED

	// 计算最终 outgoing anthropic-beta。Vertex AI 的 Anthropic 端点只接受一小撮
	// beta token，未知 token 会直接 HTTP 400——近期 Claude Code CLI 透传的
	// advisor-tool-2026-03-01 / prompt-caching-scope-2026-01-05 /
	// redact-thinking-2026-02-12 / thinking-token-count-2026-05-13 都不被 Vertex 接受
	// （issue #3358）。这里复用 BetaPolicy 的 block 检查（与 Bedrock 的
	// resolveBedrockBetaTokensForRequest 对称），再按 vertexSupportedBetaTokens 白名单
	// 剥离其余 token，使该路径与 Anthropic 直连 / Bedrock 路径行为一致。
	clientBeta := ""
	if c != nil && c.Request != nil {
		clientBeta = getHeaderRaw(c.Request.Header, "anthropic-beta")
REDACTED
	policy := s.evaluateBetaPolicy(ctx, clientBeta, account, modelID)
	if policy.blockErr != nil {
		return nil, policy.blockErr
REDACTED
	finalBeta := filterVertexBetaTokens(clientBeta, mergeDropSets(policy.filterSet))

	// 能力维度 sanitize：基于最终 beta（而非原始 client 值）决定是否保留 body 中的
	// context_management，与 Anthropic 直连 / Bedrock 路径对称。
	if sanitized, changed := sanitizeAnthropicBodyForBetaTokens(vertexBody, finalBeta); changed {
		vertexBody = sanitized
REDACTED
	fullURL, err := buildVertexAnthropicURL(account.VertexProjectID(), account.VertexLocation(modelID), modelID, reqStream)
	if err != nil {
		return nil, err
REDACTED
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(vertexBody))
	if err != nil {
		return nil, err
REDACTED

	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			lowerKey := strings.ToLower(strings.TrimSpace(key))
			if !allowedHeaders[lowerKey] || lowerKey == "anthropic-version" {
				continue
		REDACTED
			wireKey := resolveWireCasing(key)
			for _, v := range values {
				addHeaderRaw(req.Header, wireKey, v)
		REDACTED
	REDACTED
REDACTED

	req.Header.Del("authorization")
	req.Header.Del("x-api-key")
	req.Header.Del("x-goog-api-key")
	req.Header.Del("cookie")
	req.Header.Del("anthropic-version")
	setHeaderRaw(req.Header, "authorization", "Bearer "+token)
	setHeaderRaw(req.Header, "content-type", "application/json")

	// 覆盖上面白名单 loop 写入的原始 client anthropic-beta，使用过滤后的最终值。
	// finalBeta 为空（全部被剥离）时不下发该 header，与 Vertex 无 beta 请求一致。
	deleteHeaderAllForms(req.Header, "anthropic-beta")
	if finalBeta != "" {
		setHeaderRaw(req.Header, "anthropic-beta", finalBeta)
REDACTED

	s.debugLogGatewaySnapshot("UPSTREAM_FORWARD_VERTEX_ANTHROPIC", req.Header, vertexBody, map[string]string{
		"url":        req.URL.String(),
		"token_type": "service_account",
		"model":      modelID,
		"stream":     strconv.FormatBool(reqStream),
REDACTED)

	return req, nil
REDACTED

// getBetaHeader 处理anthropic-beta header
// 对于OAuth账号，需要确保包含oauth-2025-04-20
func (s *GatewayService) getBetaHeader(modelID string, clientBetaHeader string) string {
	// 如果客户端传了anthropic-beta
	if clientBetaHeader != "" {
		// 已包含oauth beta则直接返回
		if strings.Contains(clientBetaHeader, claude.BetaOAuth) {
			return clientBetaHeader
	REDACTED

		// 需要添加oauth beta
		parts := strings.Split(clientBetaHeader, ",")
		for i, p := range parts {
			parts[i] = strings.TrimSpace(p)
	REDACTED

		// 在claude-code-20250219后面插入oauth beta
		claudeCodeIdx := -1
		for i, p := range parts {
			if p == claude.BetaClaudeCode {
				claudeCodeIdx = i
				break
		REDACTED
	REDACTED

		if claudeCodeIdx >= 0 {
			// 在claude-code后面插入
			newParts := make([]string, 0, len(parts)+1)
			newParts = append(newParts, parts[:claudeCodeIdx+1]...)
			newParts = append(newParts, claude.BetaOAuth)
			newParts = append(newParts, parts[claudeCodeIdx+1:]...)
			return strings.Join(newParts, ",")
	REDACTED

		// 没有claude-code，放在第一位
		return claude.BetaOAuth + "," + clientBetaHeader
REDACTED

	// 客户端没传，根据模型生成
	// haiku 模型不需要 claude-code beta
	if strings.Contains(strings.ToLower(modelID), "haiku") {
		return claude.HaikuBetaHeader
REDACTED

	return claude.DefaultBetaHeader
REDACTED

func requestNeedsBetaFeatures(body []byte) bool {
	tools := gjson.GetBytes(body, "tools")
	if tools.Exists() && tools.IsArray() && len(tools.Array()) > 0 {
		return true
REDACTED
	thinkingType := gjson.GetBytes(body, "thinking.type").String()
	if strings.EqualFold(thinkingType, "enabled") || strings.EqualFold(thinkingType, "adaptive") {
		return true
REDACTED
	return false
REDACTED

func defaultAPIKeyBetaHeader(body []byte) string {
	modelID := gjson.GetBytes(body, "model").String()
	if strings.Contains(strings.ToLower(modelID), "haiku") {
		return claude.APIKeyHaikuBetaHeader
REDACTED
	return claude.APIKeyBetaHeader
REDACTED

func applyClaudeOAuthHeaderDefaults(req *http.Request) {
	if req == nil {
		return
REDACTED
	if getHeaderRaw(req.Header, "Accept") == "" {
		setHeaderRaw(req.Header, "Accept", "application/json")
REDACTED
	for key, value := range claude.DefaultHeaders {
		if value == "" {
			continue
	REDACTED
		if getHeaderRaw(req.Header, key) == "" {
			setHeaderRaw(req.Header, resolveWireCasing(key), value)
	REDACTED
REDACTED
REDACTED

func mergeAnthropicBeta(required []string, incoming string) string {
	seen := make(map[string]struct{REDACTED, len(required)+8)
	out := make([]string, 0, len(required)+8)

	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
	REDACTED
		if _, ok := seen[v]; ok {
			return
	REDACTED
		seen[v] = struct{REDACTED{REDACTED
		out = append(out, v)
REDACTED

	for _, r := range required {
		add(r)
REDACTED
	for _, p := range strings.Split(incoming, ",") {
		add(p)
REDACTED
	return strings.Join(out, ",")
REDACTED

func mergeAnthropicBetaDropping(required []string, incoming string, drop map[string]struct{REDACTED) string {
	merged := mergeAnthropicBeta(required, incoming)
	if merged == "" || len(drop) == 0 {
		return merged
REDACTED
	out := make([]string, 0, 8)
	for _, p := range strings.Split(merged, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
	REDACTED
		if _, ok := drop[p]; ok {
			continue
	REDACTED
		out = append(out, p)
REDACTED
	return strings.Join(out, ",")
REDACTED

// computeFinalAnthropicBeta 计算发往上游的最终 anthropic-beta header 值。
//
// 设计动机：将原本在 buildUpstreamRequest 内联在一起、依赖 req.Header 的
// anthropic-beta 计算逻辑抽成纯函数。这样调用方可以在 NewRequest 之前
// 就提前拿到最终 beta header，进而能按它对 body 做能力维度 sanitize 后再做
// CCH 签名——一举修复了以下之前由顺序依赖导致的能力维度 sanitize
// 无法部署的问题（签名与最终 body 不一致可以被判 third-party）。
//
// 返回 (value, shouldSet)：
//   - shouldSet=false 意为“不主动设置 anthropic-beta header”，与原代码“
//     API-key 账号 + 客户端未传 anthropic-beta + InjectBetaForAPIKey 未开启或
//     requestNeedsBetaFeatures=false”的行为对齐。
//   - shouldSet=true 时 value 可能为空字符串（例如客户端透传的 beta 被 dropSet
//     全部过滤掉），这与原代码中 setHeaderRaw 的结果一致。
//
// clientHeaders 是客户端原始 HTTP header（通常为 c.Request.Header）；nil 时按“客户端
// 未传”处理。body 是已经 metadata 重写 / billing version sync 之后但未 sanitize 上游
// 不兼容字段之前的版本。
func (s *GatewayService) computeFinalAnthropicBeta(
	tokenType string,
	mimicClaudeCode bool,
	modelID string,
	clientHeaders http.Header,
	body []byte,
	effectiveDropSet map[string]struct{REDACTED,
) (string, bool) {
	clientBeta := ""
	if clientHeaders != nil {
		clientBeta = getHeaderRaw(clientHeaders, "anthropic-beta")
REDACTED

	if tokenType == "oauth" {
		if mimicClaudeCode {
			// mimic 路径：原代码跳过白名单透传，incomingBeta 总是空字符串。
			// 这里传空 string 以严格对齐原行为。
			requiredBetas := []string{claude.BetaOAuth, claude.BetaInterleavedThinkingREDACTED
			if !strings.Contains(strings.ToLower(modelID), "haiku") {
				requiredBetas = claude.FullClaudeCodeMimicryBetas()
		REDACTED
			return mergeAnthropicBetaDropping(requiredBetas, "", effectiveDropSet), true
	REDACTED
		// 真 Claude Code 客户端透传路径
		return stripBetaTokensWithSet(s.getBetaHeader(modelID, clientBeta), effectiveDropSet), true
REDACTED

	// API-key accounts
	if clientBeta != "" {
		return stripBetaTokensWithSet(clientBeta, effectiveDropSet), true
REDACTED
	if s.cfg != nil && s.cfg.Gateway.InjectBetaForAPIKey {
		if requestNeedsBetaFeatures(body) {
			if beta := defaultAPIKeyBetaHeader(body); beta != "" {
				return beta, true
		REDACTED
	REDACTED
REDACTED
	return "", false
REDACTED

// computeFinalCountTokensAnthropicBeta 是 count_tokens 路径上 anthropic-beta header 的
// 计算纯函数。语义与 computeFinalAnthropicBeta 对齐，但备份了 count_tokens 独有的
// 两条特殊规则：
//
//   - OAuth mimic：requiredBetas 为 FullClaudeCodeMimicryBetas + BetaTokenCounting
//     （与 messages 不同的是：不按 haiku 排除；count_tokens 始终携带 token-counting beta）
//   - OAuth 透传 + 客户端未传 anthropic-beta：补齐 CountTokensBetaHeader
//   - OAuth 透传 + 客户端传了：补齐 BetaTokenCounting（如果未含）
//
// 返回语义同 computeFinalAnthropicBeta。
func (s *GatewayService) computeFinalCountTokensAnthropicBeta(
	tokenType string,
	mimicClaudeCode bool,
	modelID string,
	clientHeaders http.Header,
	body []byte,
	effectiveDropSet map[string]struct{REDACTED,
) (string, bool) {
	clientBeta := ""
	if clientHeaders != nil {
		clientBeta = getHeaderRaw(clientHeaders, "anthropic-beta")
REDACTED

	if tokenType == "oauth" {
		if mimicClaudeCode {
			// 与原代码严格等价：original buildCountTokensRequest 在 count_tokens mimic
			// 分支上**不**会跳过白名单透传（与 messages mimic 路径不同），所以
			// incomingBeta = req.Header[anthropic-beta] = 客户端透传过来的 client beta。
			// 重构后直接从 clientHeaders 拿同一个值，保持行为一致。
			requiredBetas := append(claude.FullClaudeCodeMimicryBetas(), claude.BetaTokenCounting)
			return mergeAnthropicBetaDropping(requiredBetas, clientBeta, effectiveDropSet), true
	REDACTED
		if clientBeta == "" {
			return claude.CountTokensBetaHeader, true
	REDACTED
		beta := s.getBetaHeader(modelID, clientBeta)
		if !strings.Contains(beta, claude.BetaTokenCounting) {
			beta = beta + "," + claude.BetaTokenCounting
	REDACTED
		return stripBetaTokensWithSet(beta, effectiveDropSet), true
REDACTED

	// API-key accounts
	if clientBeta != "" {
		return stripBetaTokensWithSet(clientBeta, effectiveDropSet), true
REDACTED
	if s.cfg != nil && s.cfg.Gateway.InjectBetaForAPIKey {
		if requestNeedsBetaFeatures(body) {
			if beta := defaultAPIKeyBetaHeader(body); beta != "" {
				return beta, true
		REDACTED
	REDACTED
REDACTED
	return "", false
REDACTED

// stripBetaTokens removes the given beta tokens from a comma-separated header value.
func stripBetaTokens(header string, tokens []string) string {
	if header == "" || len(tokens) == 0 {
		return header
REDACTED
	return stripBetaTokensWithSet(header, buildBetaTokenSet(tokens))
REDACTED

func stripBetaTokensWithSet(header string, drop map[string]struct{REDACTED) string {
	if header == "" || len(drop) == 0 {
		return header
REDACTED
	parts := strings.Split(header, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
	REDACTED
		if _, ok := drop[p]; ok {
			continue
	REDACTED
		out = append(out, p)
REDACTED
	if len(out) == len(parts) {
		return header // no change, avoid allocation
REDACTED
	return strings.Join(out, ",")
REDACTED

// BetaBlockedError indicates a request was blocked by a beta policy rule.
type BetaBlockedError struct {
	Message string
REDACTED

func (e *BetaBlockedError) Error() string { return e.Message REDACTED

// betaPolicyResult holds the evaluated result of beta policy rules for a single request.
type betaPolicyResult struct {
	blockErr  *BetaBlockedError   // non-nil if a block rule matched
	filterSet map[string]struct{REDACTED // tokens to filter (may be nil)
REDACTED

// evaluateBetaPolicy loads settings once and evaluates all rules against the given request.
func (s *GatewayService) evaluateBetaPolicy(ctx context.Context, betaHeader string, account *Account, model string) betaPolicyResult {
	if s.settingService == nil {
		return betaPolicyResult{REDACTED
REDACTED
	settings, err := s.settingService.GetBetaPolicySettings(ctx)
	if err != nil || settings == nil {
		return betaPolicyResult{REDACTED
REDACTED
	isOAuth := account.IsOAuth()
	isBedrock := account.IsBedrock()
	var result betaPolicyResult
	for _, rule := range settings.Rules {
		if !betaPolicyScopeMatches(rule.Scope, isOAuth, isBedrock) {
			continue
	REDACTED
		effectiveAction, effectiveErrMsg := resolveRuleAction(rule, model)
		switch effectiveAction {
		case BetaPolicyActionBlock:
			if result.blockErr == nil && betaHeader != "" && containsBetaToken(betaHeader, rule.BetaToken) {
				msg := effectiveErrMsg
				if msg == "" {
					msg = "beta feature " + rule.BetaToken + " is not allowed"
			REDACTED
				result.blockErr = &BetaBlockedError{Message: msgREDACTED
		REDACTED
		case BetaPolicyActionFilter:
			if result.filterSet == nil {
				result.filterSet = make(map[string]struct{REDACTED)
		REDACTED
			result.filterSet[rule.BetaToken] = struct{REDACTED{REDACTED
	REDACTED
REDACTED
	return result
REDACTED

// mergeDropSets merges the static defaultDroppedBetasSet with dynamic policy filter tokens.
// Returns defaultDroppedBetasSet directly when policySet is empty (zero allocation).
func mergeDropSets(policySet map[string]struct{REDACTED, extra ...string) map[string]struct{REDACTED {
	if len(policySet) == 0 && len(extra) == 0 {
		return defaultDroppedBetasSet
REDACTED
	m := make(map[string]struct{REDACTED, len(defaultDroppedBetasSet)+len(policySet)+len(extra))
	for t := range defaultDroppedBetasSet {
		m[t] = struct{REDACTED{REDACTED
REDACTED
	for t := range policySet {
		m[t] = struct{REDACTED{REDACTED
REDACTED
	for _, t := range extra {
		m[t] = struct{REDACTED{REDACTED
REDACTED
	return m
REDACTED

// betaPolicyFilterSetKey is the gin.Context key for caching the policy filter set within a request.
const betaPolicyFilterSetKey = "betaPolicyFilterSet"

// getBetaPolicyFilterSet returns the beta policy filter set, using the gin context cache if available.
// In the /v1/messages path, Forward() evaluates the policy first and caches the result;
// buildUpstreamRequest reuses it (zero extra DB calls). In the count_tokens path, this
// evaluates on demand (one DB call).
func (s *GatewayService) getBetaPolicyFilterSet(ctx context.Context, c *gin.Context, account *Account, model string) map[string]struct{REDACTED {
	if c != nil {
		if v, ok := c.Get(betaPolicyFilterSetKey); ok {
			if fs, ok := v.(map[string]struct{REDACTED); ok {
				return fs
		REDACTED
	REDACTED
REDACTED
	return s.evaluateBetaPolicy(ctx, "", account, model).filterSet
REDACTED

// betaPolicyScopeMatches checks whether a rule's scope matches the current account type.
func betaPolicyScopeMatches(scope string, isOAuth bool, isBedrock bool) bool {
	switch scope {
	case BetaPolicyScopeAll:
		return true
	case BetaPolicyScopeOAuth:
		return isOAuth
	case BetaPolicyScopeAPIKey:
		return !isOAuth && !isBedrock
	case BetaPolicyScopeBedrock:
		return isBedrock
	default:
		return true // unknown scope → match all (fail-open)
REDACTED
REDACTED

// matchModelWhitelist checks if a model matches any pattern in the whitelist.
// Reuses matchModelPattern from group.go which supports exact and wildcard prefix matching.
func matchModelWhitelist(model string, whitelist []string) bool {
	for _, pattern := range whitelist {
		if matchModelPattern(pattern, model) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

// resolveRuleAction determines the effective action and error message for a rule given the request model.
// When ModelWhitelist is empty, the rule's primary Action/ErrorMessage applies unconditionally.
// When non-empty, Action applies to matching models; FallbackAction/FallbackErrorMessage applies to others.
func resolveRuleAction(rule BetaPolicyRule, model string) (action, errorMessage string) {
	if len(rule.ModelWhitelist) == 0 {
		return rule.Action, rule.ErrorMessage
REDACTED
	if matchModelWhitelist(model, rule.ModelWhitelist) {
		return rule.Action, rule.ErrorMessage
REDACTED
	if rule.FallbackAction != "" {
		return rule.FallbackAction, rule.FallbackErrorMessage
REDACTED
	return BetaPolicyActionPass, "" // default fallback: pass (fail-open)
REDACTED

// droppedBetaSet returns claude.DroppedBetas as a set, with optional extra tokens.
func droppedBetaSet(extra ...string) map[string]struct{REDACTED {
	m := make(map[string]struct{REDACTED, len(defaultDroppedBetasSet)+len(extra))
	for t := range defaultDroppedBetasSet {
		m[t] = struct{REDACTED{REDACTED
REDACTED
	for _, t := range extra {
		m[t] = struct{REDACTED{REDACTED
REDACTED
	return m
REDACTED

// containsBetaToken checks if a comma-separated header value contains the given token.
func containsBetaToken(header, token string) bool {
	if header == "" || token == "" {
		return false
REDACTED
	for _, p := range strings.Split(header, ",") {
		if strings.TrimSpace(p) == token {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func filterBetaTokens(tokens []string, filterSet map[string]struct{REDACTED) []string {
	if len(tokens) == 0 || len(filterSet) == 0 {
		return tokens
REDACTED
	kept := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if _, filtered := filterSet[token]; !filtered {
			kept = append(kept, token)
	REDACTED
REDACTED
	return kept
REDACTED

func (s *GatewayService) resolveBedrockBetaTokensForRequest(
	ctx context.Context,
	account *Account,
	betaHeader string,
	body []byte,
	modelID string,
) ([]string, error) {
	// 1. 对原始 header 中的 beta token 做 block 检查（快速失败）
	policy := s.evaluateBetaPolicy(ctx, betaHeader, account, modelID)
	if policy.blockErr != nil {
		return nil, policy.blockErr
REDACTED

	// 2. 解析 header + body 自动注入 + Bedrock 转换/过滤
	betaTokens := ResolveBedrockBetaTokens(betaHeader, body, modelID)

	// 3. 对最终 token 列表再做 block 检查，捕获通过 body 自动注入绕过 header block 的情况。
	//    例如：管理员 block 了 interleaved-thinking，客户端不在 header 中带该 token，
	//    但请求体中包含 thinking 字段 → autoInjectBedrockBetaTokens 会自动补齐 →
	//    如果不做此检查，block 规则会被绕过。
	if blockErr := s.checkBetaPolicyBlockForTokens(ctx, betaTokens, account, modelID); blockErr != nil {
		return nil, blockErr
REDACTED

	return filterBetaTokens(betaTokens, policy.filterSet), nil
REDACTED

// checkBetaPolicyBlockForTokens 检查 token 列表中是否有被管理员 block 规则命中的 token。
// 用于补充 evaluateBetaPolicy 对 header 的检查，覆盖 body 自动注入的 token。
func (s *GatewayService) checkBetaPolicyBlockForTokens(ctx context.Context, tokens []string, account *Account, model string) *BetaBlockedError {
	if s.settingService == nil || len(tokens) == 0 {
		return nil
REDACTED
	settings, err := s.settingService.GetBetaPolicySettings(ctx)
	if err != nil || settings == nil {
		return nil
REDACTED
	isOAuth := account.IsOAuth()
	isBedrock := account.IsBedrock()
	tokenSet := buildBetaTokenSet(tokens)
	for _, rule := range settings.Rules {
		effectiveAction, effectiveErrMsg := resolveRuleAction(rule, model)
		if effectiveAction != BetaPolicyActionBlock {
			continue
	REDACTED
		if !betaPolicyScopeMatches(rule.Scope, isOAuth, isBedrock) {
			continue
	REDACTED
		if _, present := tokenSet[rule.BetaToken]; present {
			msg := effectiveErrMsg
			if msg == "" {
				msg = "beta feature " + rule.BetaToken + " is not allowed"
		REDACTED
			return &BetaBlockedError{Message: msgREDACTED
	REDACTED
REDACTED
	return nil
REDACTED

func buildBetaTokenSet(tokens []string) map[string]struct{REDACTED {
	m := make(map[string]struct{REDACTED, len(tokens))
	for _, t := range tokens {
		if t == "" {
			continue
	REDACTED
		m[t] = struct{REDACTED{REDACTED
REDACTED
	return m
REDACTED

var defaultDroppedBetasSet = buildBetaTokenSet(claude.DroppedBetas)

// applyClaudeCodeMimicHeaders forces "Claude Code-like" request headers.
// This mirrors opencode-anthropic-auth behavior: do not trust downstream
// headers when using Claude Code-scoped OAuth credentials.
func applyClaudeCodeMimicHeaders(req *http.Request, isStream bool) {
	if req == nil {
		return
REDACTED
	// Start with the standard defaults (fill missing).
	applyClaudeOAuthHeaderDefaults(req)
	// Then force key headers to match Claude Code fingerprint regardless of what the client sent.
	// 使用 resolveWireCasing 确保 key 与真实 wire format 一致（如 "x-app" 而非 "X-App"）
	for key, value := range claude.DefaultHeaders {
		if value == "" {
			continue
	REDACTED
		setHeaderRaw(req.Header, resolveWireCasing(key), value)
REDACTED
	// Real Claude CLI uses Accept: application/json (even for streaming).
	setHeaderRaw(req.Header, "Accept", "application/json")
	if isStream {
		setHeaderRaw(req.Header, "x-stainless-helper-method", "stream")
REDACTED
	// Real Claude CLI 每个请求都会生成一个新的 UUID 放在 x-client-request-id。
	// 上游会以此作为会话/请求指纹的一部分，缺失或重复都可能触发第三方判定。
	if getHeaderRaw(req.Header, "x-client-request-id") == "" {
		setHeaderRaw(req.Header, "x-client-request-id", uuid.NewString())
REDACTED
REDACTED

func truncateForLog(b []byte, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = 2048
REDACTED
	if len(b) > maxBytes {
		b = b[:maxBytes]
REDACTED
	s := string(b)
	// 保持一行，避免污染日志格式
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
REDACTED

// buildCustomRelayURL 构建自定义中继转发 URL
// 在 path 后附加 beta=true 和可选的 proxy 查询参数
func (s *GatewayService) buildCustomRelayURL(baseURL, path string, account *Account) string {
	u := strings.TrimRight(baseURL, "/") + path + "?beta=true"
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL := account.Proxy.URL()
		if proxyURL != "" {
			u += "&proxy=" + url.QueryEscape(proxyURL)
	REDACTED
REDACTED
	return u
REDACTED

func (s *GatewayService) validateUpstreamBaseURL(raw string) (string, error) {
	if s.cfg != nil && !s.cfg.Security.URLAllowlist.Enabled {
		normalized, err := urlvalidator.ValidateURLFormat(raw, s.cfg.Security.URLAllowlist.AllowInsecureHTTP)
		if err != nil {
			return "", fmt.Errorf("invalid base_url: %w", err)
	REDACTED
		return normalized, nil
REDACTED
	normalized, err := urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
		AllowedHosts:     s.cfg.Security.URLAllowlist.UpstreamHosts,
		RequireAllowlist: true,
		AllowPrivate:     s.cfg.Security.URLAllowlist.AllowPrivateHosts,
REDACTED)
	if err != nil {
		return "", fmt.Errorf("invalid base_url: %w", err)
REDACTED
	return normalized, nil
REDACTED
