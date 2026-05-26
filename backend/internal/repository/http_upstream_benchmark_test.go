package repository

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// httpClientSink 用于防止编译器优化掉基准测试中的赋值操作
// 这是 Go 基准测试的常见模式，确保测试结果准确
var httpClientSink *http.Client

// BenchmarkHTTPUpstreamProxyClient 对比重复创建与复用代理客户端的开销
//
// 测试目的：
// - 验证连接池复用相比每次新建的性能提升
// - 量化内存分配差异
//
// 预期结果：
// - "复用" 子测试应显著快于 "新建"
// - "复用" 子测试应零内存分配
func BenchmarkHTTPUpstreamProxyClient(b *testing.B) {
	// 创建测试配置
	cfg := &config.Config{
		Gateway: config.GatewayConfig{ResponseHeaderTimeout: 300},
	}
	upstream := NewHTTPUpstream(cfg)
	svc, ok := upstream.(*httpUpstreamService)
	if !ok {
		b.Fatalf("类型断言失败，无法获取 httpUpstreamService")
	}

	proxyURL := "http://127.0.0.1:8080"
	b.ReportAllocs() // 报告内存分配统计

	// 子测试：每次新建客户端
	// 模拟未优化前的行为，每次请求都创建新的 http.Client
	b.Run("新建", func(b *testing.B) {
		parsedProxy, err := url.Parse(proxyURL)
		if err != nil {
			b.Fatalf("解析代理地址失败: %v", err)
		}
		settings := defaultPoolSettings(cfg)
		for i := 0; i < b.N; i++ {
			// 每次迭代都创建新客户端，包含 Transport 分配
			transport, err := buildUpstreamTransport(settings, parsedProxy)
			if err != nil {
				b.Fatalf("创建 Transport 失败: %v", err)
			}
			httpClientSink = &http.Client{
				Transport: transport,
			}
		}
	})

	// 子测试：复用已缓存的客户端
	// 模拟优化后的行为，从缓存获取客户端
	b.Run("复用", func(b *testing.B) {
		// 预热：确保客户端已缓存
		entry, err := svc.getOrCreateClient(proxyURL, 1, 1)
		if err != nil {
			b.Fatalf("getOrCreateClient: %v", err)
		}
		client := entry.client
		b.ResetTimer() // 重置计时器，排除预热时间
		for i := 0; i < b.N; i++ {
			// 直接使用缓存的客户端，无内存分配
			httpClientSink = client
		}
	})
}
