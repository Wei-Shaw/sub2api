//go:build unit

package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// TestNewGatewayHandler_StoresKiroGatewayService 验证 Task 18 新增的
// kiroGatewayService 依赖被 NewGatewayHandler 正确接住并保存到返回的
// *GatewayHandler 结构体上。
//
// 背景（task-18 corrected design，见 task-18-report.md）：
// GatewayHandler.Messages 内部的账号转发分流从「Antigravity/else 两路」
// 扩展为「Antigravity/Kiro/default 三路」，新增分支用
// h.kiroGatewayService.ForwardUpstream 转发 kiro 平台账号的请求。
//
// gatewayService / antigravityGatewayService / kiroGatewayService 都是
// 具体结构体指针字段（非接口），没法像接口依赖那样换成轻量 fake 来做端到端
// 分派测试；这个仓库里已有的 Antigravity 分支目前也没有任何测试覆盖同类
// 「按 account.Platform 选转发实现」的分派决策（controller 预检已确认，
// 见 task-18-report.md 的诚实测试覆盖说明）。因此这里只做诚实的构造期接线
// 校验：确认传入的 kiroGatewayService 参数值被正确存到了对应字段上，
// 不构造 gin.Context/httptest 去驱动 Messages() 走三路 switch 的具体某一支。
// 三路 switch 本身的正确性依赖代码评审 + go build/go vet 覆盖（三个比较、
// 三个方法调用的小改动，评审即可核实）。
func TestNewGatewayHandler_StoresKiroGatewayService(t *testing.T) {
	kiroSvc := &service.KiroGatewayService{}

	h := NewGatewayHandler(
		nil, // gatewayService
		nil, // openAIGatewayService
		nil, // geminiCompatService
		nil, // antigravityGatewayService
		kiroSvc,
		nil, // userService
		nil, // concurrencyService
		nil, // billingCacheService
		nil, // usageService
		nil, // apiKeyService
		nil, // usageRecordWorkerPool
		nil, // errorPassthroughService
		nil, // contentModerationService
		nil, // userMsgQueueService
		nil, // cfg
		nil, // settingService
	)

	require.NotNil(t, h)
	require.Same(t, kiroSvc, h.kiroGatewayService,
		"NewGatewayHandler 必须把 kiroGatewayService 参数存到同名字段，"+
			"否则 Messages() 里新增的 kiro 分支会拿到 nil 并 panic")
}
