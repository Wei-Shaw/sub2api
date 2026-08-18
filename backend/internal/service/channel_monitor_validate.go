package service

import (
	"context"
	"net/url"
	"strings"
)

// 渠道监控参数校验与归一化辅助函数。
// 校验失败一律返回 channel_monitor_const.go 中预定义的 Err* 错误，错误信息不含具体 IP/hostname，避免泄露内网拓扑。

// monitorProviders 渠道监控支持的全部 provider（与迁移 226 的 CHECK 约束一致）。
// 不再以 adapter 表为唯一来源：antigravity 没有探活 adapter，但支持配额模式。
//
//nolint:gochecknoglobals // 静态查表，初始化后不变。
var monitorProviders = map[string]struct{REDACTED{
	MonitorProviderOpenAI:      {REDACTED,
	MonitorProviderAnthropic:   {REDACTED,
	MonitorProviderGemini:      {REDACTED,
	MonitorProviderGrok:        {REDACTED,
	MonitorProviderAntigravity: {REDACTED,
	MonitorProviderKimi:        {REDACTED,
	MonitorProviderZhipu:       {REDACTED,
	MonitorProviderDeepseek:    {REDACTED,
REDACTED

// probeCapableProviders 支持探活（probe / quota_probe）的 provider。
// antigravity 上游无 Chat/Responses 可打（仅 IDE 代理形态），只允许配额模式。
//
//nolint:gochecknoglobals // 静态查表，初始化后不变。
var probeCapableProviders = map[string]struct{REDACTED{
	MonitorProviderOpenAI:    {REDACTED,
	MonitorProviderAnthropic: {REDACTED,
	MonitorProviderGemini:    {REDACTED,
	MonitorProviderGrok:      {REDACTED,
	MonitorProviderKimi:      {REDACTED,
	MonitorProviderZhipu:     {REDACTED,
	MonitorProviderDeepseek:  {REDACTED,
REDACTED

// validateProvider 校验 provider 字符串。
func validateProvider(p string) error {
	if _, ok := monitorProviders[p]; !ok {
		return ErrChannelMonitorInvalidProvider
REDACTED
	return nil
REDACTED

// providerSupportsProbe 该 provider 是否注册了探活 adapter（antigravity 为 false）。
func providerSupportsProbe(p string) bool {
	_, ok := probeCapableProviders[p]
	return ok
REDACTED

// defaultCheckMode 空串归一为 probe，保证存量数据与旧客户端兼容。
func defaultCheckMode(checkMode string) string {
	if strings.TrimSpace(checkMode) == "" {
		return MonitorCheckModeProbe
REDACTED
	return strings.TrimSpace(checkMode)
REDACTED

// monitorCheckModeUsesQuota 该模式是否需要关联账号查配额。
func monitorCheckModeUsesQuota(checkMode string) bool {
	return checkMode == MonitorCheckModeQuota || checkMode == MonitorCheckModeQuotaProbe
REDACTED

// validateCheckMode 校验 check_mode 与 provider 的组合矩阵：
//
//	provider                | probe | quota | quota_probe
//	------------------------+-------+-------+------------
//	openai/anthropic/...    |  Y    |  Y    |  Y
//	antigravity（无 adapter）|  N    |  Y    |  N
func validateCheckMode(provider, checkMode string) error {
	checkMode = defaultCheckMode(checkMode)
	switch checkMode {
	case MonitorCheckModeProbe, MonitorCheckModeQuota, MonitorCheckModeQuotaProbe:
	default:
		return ErrChannelMonitorInvalidCheckMode
REDACTED
	if checkMode != MonitorCheckModeQuota && !providerSupportsProbe(provider) {
		return ErrChannelMonitorInvalidCheckMode
REDACTED
	return nil
REDACTED

// validateAPIMode 校验 provider 与 api_mode 的组合。
// responses 只对 OpenAI 有意义；其它 provider 使用 chat_completions 作为默认占位。
func validateAPIMode(provider, apiMode string) error {
	apiMode = defaultAPIMode(apiMode)
	switch apiMode {
	case MonitorAPIModeChatCompletions:
		return nil
	case MonitorAPIModeResponses:
		if provider == "" || provider == MonitorProviderOpenAI {
			return nil
	REDACTED
		return ErrChannelMonitorInvalidAPIMode
	default:
		return ErrChannelMonitorInvalidAPIMode
REDACTED
REDACTED

// validateInterval 校验 interval_seconds 范围。
func validateInterval(sec int) error {
	if sec < monitorMinIntervalSeconds || sec > monitorMaxIntervalSeconds {
		return ErrChannelMonitorInvalidInterval
REDACTED
	return nil
REDACTED

// validateJitter 校验 jitter_seconds（调度 ± 随机抖动）：
// 非负，且 interval - jitter 不得低于最小检测间隔，防止随机偏移后实际间隔过短打爆上游。
func validateJitter(jitterSec, intervalSec int) error {
	if jitterSec < 0 || intervalSec-jitterSec < monitorMinIntervalSeconds {
		return ErrChannelMonitorInvalidJitter
REDACTED
	return nil
REDACTED

// validateEndpoint 校验 endpoint：
//   - scheme 强制 https（拒绝 http，避免明文凭证 + 部分 SSRF 利用面）
//   - 必须为 origin（无 path/query/fragment），防止用户填 https://api.openai.com/v1
//     导致 joinURL 拼出 /v1/v1/chat/completions
//   - hostname 不能是 localhost/metadata 等已知元数据 hostname
//   - 解析所有 IP，任一落在 loopback/RFC1918/link-local/ULA 段即拒绝（防 SSRF）
//
// 错误信息不暴露具体 IP / hostname，避免泄露内网拓扑。
func validateEndpoint(ep string) error {
	ep = strings.TrimSpace(ep)
	if ep == "" {
		return ErrChannelMonitorInvalidEndpoint
REDACTED
	u, err := url.Parse(ep)
	if err != nil {
		return ErrChannelMonitorInvalidEndpoint
REDACTED
	if u.Scheme != "https" {
		return ErrChannelMonitorEndpointScheme
REDACTED
	if u.Host == "" {
		return ErrChannelMonitorInvalidEndpoint
REDACTED
	if u.Path != "" && u.Path != "/" {
		return ErrChannelMonitorEndpointPath
REDACTED
	if u.RawQuery != "" || u.Fragment != "" {
		return ErrChannelMonitorEndpointPath
REDACTED

	hostname := u.Hostname()
	ctx, cancel := context.WithTimeout(context.Background(), monitorEndpointResolveTimeout)
	defer cancel()
	blocked, err := isPrivateOrLoopbackHost(ctx, hostname)
	if err != nil {
		return ErrChannelMonitorEndpointUnreachable
REDACTED
	if blocked {
		return ErrChannelMonitorEndpointPrivate
REDACTED
	return nil
REDACTED

// normalizeEndpoint 去除前后空白与末尾 `/`，保证存储统一为 origin。
// validateEndpoint 已确保格式合法（仅 origin），这里只做最终归一化。
func normalizeEndpoint(ep string) string {
	ep = strings.TrimSpace(ep)
	ep = strings.TrimRight(ep, "/")
	return ep
REDACTED

// normalizeModels 去除空白、重复模型名。保留输入顺序（map 的迭代顺序无关）。
func normalizeModels(in []string) []string {
	if len(in) == 0 {
		return []string{REDACTED
REDACTED
	seen := make(map[string]struct{REDACTED, len(in))
	out := make([]string, 0, len(in))
	for _, m := range in {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
	REDACTED
		if _, ok := seen[m]; ok {
			continue
	REDACTED
		seen[m] = struct{REDACTED{REDACTED
		out = append(out, m)
REDACTED
	return out
REDACTED

// normalizeMonitorPrimaryModel applies provider/check_mode defaults while
// preserving the existing required-model behavior:
//   - Grok 探活默认轻量测活模型
//   - quota 模式占位 "quota"（primary_model 列 NotEmpty；历史行/时间线机制无需特判）
func normalizeMonitorPrimaryModel(provider, checkMode, model string) string {
	model = strings.TrimSpace(model)
	if model == "" && provider == MonitorProviderGrok {
		return MonitorDefaultGrokModel
REDACTED
	if model == "" && monitorCheckModeUsesQuota(defaultCheckMode(checkMode)) {
		return MonitorDefaultQuotaModel
REDACTED
	return model
REDACTED

// defaultAPIMode 空串归一为 chat_completions，保证历史数据与旧客户端兼容。
func defaultAPIMode(apiMode string) string {
	if strings.TrimSpace(apiMode) == "" {
		return MonitorAPIModeChatCompletions
REDACTED
	return strings.TrimSpace(apiMode)
REDACTED
