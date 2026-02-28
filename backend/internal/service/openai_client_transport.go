package service

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// OpenAIClientTransport 表示客户端入站协议类型。
type OpenAIClientTransport string

const (
	OpenAIClientTransportUnknown OpenAIClientTransport = ""
	OpenAIClientTransportHTTP    OpenAIClientTransport = "http"
	OpenAIClientTransportWS      OpenAIClientTransport = "ws"
)

const openAIClientTransportContextKey = "openai_client_transport"

// SetOpenAIClientTransport 标记当前请求的客户端入站协议。
func SetOpenAIClientTransport(c *gin.Context, transport OpenAIClientTransport) {
	if c == nil {
		return
REDACTED
	normalized := normalizeOpenAIClientTransport(transport)
	if normalized == OpenAIClientTransportUnknown {
		return
REDACTED
	c.Set(openAIClientTransportContextKey, string(normalized))
REDACTED

// GetOpenAIClientTransport 读取当前请求的客户端入站协议。
func GetOpenAIClientTransport(c *gin.Context) OpenAIClientTransport {
	if c == nil {
		return OpenAIClientTransportUnknown
REDACTED
	raw, ok := c.Get(openAIClientTransportContextKey)
	if !ok || raw == nil {
		return OpenAIClientTransportUnknown
REDACTED

	switch v := raw.(type) {
	case OpenAIClientTransport:
		return normalizeOpenAIClientTransport(v)
	case string:
		return normalizeOpenAIClientTransport(OpenAIClientTransport(v))
	default:
		return OpenAIClientTransportUnknown
REDACTED
REDACTED

func normalizeOpenAIClientTransport(transport OpenAIClientTransport) OpenAIClientTransport {
	switch strings.ToLower(strings.TrimSpace(string(transport))) {
	case string(OpenAIClientTransportHTTP), "http_sse", "sse":
		return OpenAIClientTransportHTTP
	case string(OpenAIClientTransportWS), "websocket":
		return OpenAIClientTransportWS
	default:
		return OpenAIClientTransportUnknown
REDACTED
REDACTED

func resolveOpenAIWSDecisionByClientTransport(
	decision OpenAIWSProtocolDecision,
	clientTransport OpenAIClientTransport,
) OpenAIWSProtocolDecision {
	if clientTransport == OpenAIClientTransportHTTP {
		return openAIWSHTTPDecision("client_protocol_http")
REDACTED
	return decision
REDACTED
