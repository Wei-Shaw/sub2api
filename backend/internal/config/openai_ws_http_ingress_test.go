package config

import (
	"strings"
	"testing"
)

// TestOpenAIWSHTTPIngressWSUpstreamDefaultDisabled 确认新开关默认关闭：
// 未显式开启时 HTTP 入站必须保持原有 HTTP/SSE 行为。
func TestOpenAIWSHTTPIngressWSUpstreamDefaultDisabled(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("x", 32))
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Gateway.OpenAIWS.HTTPIngressWSUpstreamEnabled {
		t.Fatal("gateway.openai_ws.http_ingress_ws_upstream_enabled should default to false")
	}
}

// TestOpenAIWSHTTPIngressWSUpstreamEnvOverride 确认环境变量可开启该开关。
func TestOpenAIWSHTTPIngressWSUpstreamEnvOverride(t *testing.T) {
	t.Setenv("JWT_SECRET", strings.Repeat("x", 32))
	t.Setenv("GATEWAY_OPENAI_WS_HTTP_INGRESS_WS_UPSTREAM_ENABLED", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.Gateway.OpenAIWS.HTTPIngressWSUpstreamEnabled {
		t.Fatal("env GATEWAY_OPENAI_WS_HTTP_INGRESS_WS_UPSTREAM_ENABLED=true should enable the flag")
	}
}
