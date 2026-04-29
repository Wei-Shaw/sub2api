package service

import "github.com/Wei-Shaw/sub2api/internal/config"

// OpenAIUpstreamTransport 表示 OpenAI 上游传输协议。
type OpenAIUpstreamTransport string

const (
	OpenAIUpstreamTransportAny                  OpenAIUpstreamTransport = ""
	OpenAIUpstreamTransportHTTPSSE              OpenAIUpstreamTransport = "http_sse"
	OpenAIUpstreamTransportResponsesWebsocket   OpenAIUpstreamTransport = "responses_websockets"
	OpenAIUpstreamTransportResponsesWebsocketV2 OpenAIUpstreamTransport = "responses_websockets_v2"
)

// OpenAIWSProtocolDecision 表示协议决策结果。
type OpenAIWSProtocolDecision struct {
	Transport OpenAIUpstreamTransport
	Reason    string
REDACTED

// OpenAIWSProtocolResolver 定义 OpenAI 上游协议决策。
type OpenAIWSProtocolResolver interface {
	Resolve(account *Account) OpenAIWSProtocolDecision
REDACTED

type defaultOpenAIWSProtocolResolver struct {
	cfg *config.Config
REDACTED

// NewOpenAIWSProtocolResolver 创建默认协议决策器。
func NewOpenAIWSProtocolResolver(cfg *config.Config) OpenAIWSProtocolResolver {
	return &defaultOpenAIWSProtocolResolver{cfg: cfgREDACTED
REDACTED

func (r *defaultOpenAIWSProtocolResolver) Resolve(account *Account) OpenAIWSProtocolDecision {
	if account == nil {
		return openAIWSHTTPDecision("account_missing")
REDACTED
	if !account.IsOpenAI() {
		return openAIWSHTTPDecision("platform_not_openai")
REDACTED
	if account.IsOpenAIWSForceHTTPEnabled() {
		return openAIWSHTTPDecision("account_force_http")
REDACTED
	if r == nil || r.cfg == nil {
		return openAIWSHTTPDecision("config_missing")
REDACTED

	wsCfg := r.cfg.Gateway.OpenAIWS
	if wsCfg.ForceHTTP {
		return openAIWSHTTPDecision("global_force_http")
REDACTED
	if !wsCfg.Enabled {
		return openAIWSHTTPDecision("global_disabled")
REDACTED
	if account.IsOpenAIOAuth() {
		if !wsCfg.OAuthEnabled {
			return openAIWSHTTPDecision("oauth_disabled")
	REDACTED
REDACTED else if account.IsOpenAIApiKey() {
		if !wsCfg.APIKeyEnabled {
			return openAIWSHTTPDecision("apikey_disabled")
	REDACTED
REDACTED else {
		return openAIWSHTTPDecision("unknown_auth_type")
REDACTED
	if wsCfg.ModeRouterV2Enabled {
		mode := account.ResolveOpenAIResponsesWebSocketV2Mode(wsCfg.IngressModeDefault)
		switch mode {
		case OpenAIWSIngressModeOff:
			return openAIWSHTTPDecision("account_mode_off")
		case OpenAIWSIngressModeCtxPool, OpenAIWSIngressModePassthrough:
			// continue
		case OpenAIWSIngressModeHTTPBridge:
			return openAIWSHTTPDecision("ws_v2_mode_http_bridge")
		case OpenAIWSIngressModeShared, OpenAIWSIngressModeDedicated:
			// 历史值兼容：按 ctx_pool 处理。
			mode = OpenAIWSIngressModeCtxPool
		default:
			return openAIWSHTTPDecision("account_mode_off")
	REDACTED
		if account.Concurrency <= 0 {
			return openAIWSHTTPDecision("account_concurrency_invalid")
	REDACTED
		if wsCfg.ResponsesWebsocketsV2 {
			return OpenAIWSProtocolDecision{
				Transport: OpenAIUpstreamTransportResponsesWebsocketV2,
				Reason:    "ws_v2_mode_" + mode,
		REDACTED
	REDACTED
		if wsCfg.ResponsesWebsockets {
			return OpenAIWSProtocolDecision{
				Transport: OpenAIUpstreamTransportResponsesWebsocket,
				Reason:    "ws_v1_mode_" + mode,
		REDACTED
	REDACTED
		return openAIWSHTTPDecision("feature_disabled")
REDACTED
	if !account.IsOpenAIResponsesWebSocketV2Enabled() {
		return openAIWSHTTPDecision("account_disabled")
REDACTED
	if wsCfg.ResponsesWebsocketsV2 {
		return OpenAIWSProtocolDecision{
			Transport: OpenAIUpstreamTransportResponsesWebsocketV2,
			Reason:    "ws_v2_enabled",
	REDACTED
REDACTED
	if wsCfg.ResponsesWebsockets {
		return OpenAIWSProtocolDecision{
			Transport: OpenAIUpstreamTransportResponsesWebsocket,
			Reason:    "ws_v1_enabled",
	REDACTED
REDACTED
	return openAIWSHTTPDecision("feature_disabled")
REDACTED

func openAIWSHTTPDecision(reason string) OpenAIWSProtocolDecision {
	return OpenAIWSProtocolDecision{
		Transport: OpenAIUpstreamTransportHTTPSSE,
		Reason:    reason,
REDACTED
REDACTED
