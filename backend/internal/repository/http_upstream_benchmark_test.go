package repository

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

var httpClientSink *http.Client

// BenchmarkHTTPUpstreamProxyClient 对比重复创建与复用代理客户端的开销。
func BenchmarkHTTPUpstreamProxyClient(b *testing.B) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{ResponseHeaderTimeout: 300REDACTED,
REDACTED
	upstream := NewHTTPUpstream(cfg)
	svc, ok := upstream.(*httpUpstreamService)
	if !ok {
		b.Fatalf("类型断言失败，无法获取 httpUpstreamService")
REDACTED

	proxyURL := "http://127.0.0.1:8080"
	b.ReportAllocs()

	b.Run("新建", func(b *testing.B) {
		parsedProxy, err := url.Parse(proxyURL)
		if err != nil {
			b.Fatalf("解析代理地址失败: %v", err)
	REDACTED
		for i := 0; i < b.N; i++ {
			httpClientSink = &http.Client{
				Transport: buildUpstreamTransport(cfg, parsedProxy),
		REDACTED
	REDACTED
REDACTED)

	b.Run("复用", func(b *testing.B) {
		client := svc.getOrCreateClient(proxyURL)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			httpClientSink = client
	REDACTED
REDACTED)
REDACTED
