package service

import (
	"runtime"
	"testing"
)

// 测量服务初始化常驻内存，覆盖尚未收到审计任务的启动阶段。
func BenchmarkContentModerationIdleService(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil, nil)
		runtime.KeepAlive(svc)
	}
}
