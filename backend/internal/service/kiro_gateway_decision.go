package service

import "github.com/Wei-Shaw/sub2api/internal/pkg/kiro"

// kiroAction 是一次上游尝试失败后应采取的动作。
//
// 全部失败决策收口在本文件的纯函数里，HTTP 循环只负责执行。
// 这样设计文档 §7.2 的两条红线可以被表驱动测试穷举，而不必起真服务。
type kiroAction int

const (
	// kiroActionProceed 表示成功，继续处理响应。
	kiroActionProceed kiroAction = iota
	// kiroActionRefreshAndRetry 表示刷新 token 后重试同一端点。
	kiroActionRefreshAndRetry
	// kiroActionNextEndpoint 表示换下一个端点重试。
	kiroActionNextEndpoint
	// kiroActionFailoverAccount 表示换一个账号重试。
	kiroActionFailoverAccount
	// kiroActionAbort 表示直接把错误返回给客户端，不做任何重试。
	kiroActionAbort
)

// String 返回稳定的短名，用于日志与告警检索。
func (a kiroAction) String() string {
	switch a {
	case kiroActionProceed:
		return "proceed"
	case kiroActionRefreshAndRetry:
		return "refresh_and_retry"
	case kiroActionNextEndpoint:
		return "next_endpoint"
	case kiroActionFailoverAccount:
		return "failover_account"
	default:
		return "abort"
	}
}

// decideKiroAction 决定一次上游尝试之后该做什么。
//
// 三条不变式：
//
//  1. sawContent 为真时一律中止 —— 客户端已收到部分内容，任何重试都会产生重复输出。
//  2. SignalNetworkRegion（典型是 INVALID_MODEL_ID）永不触发账号转移 ——
//     它是网络/区域问题，大陆直连必现；换账号解决不了，只会把整池账号标记失败。
//  3. SignalBadRequest 永不重试也永不转移 —— 400 说明我们自己构造的请求不合法，
//     换账号一样失败，重试只会烧光整池配额。
func decideKiroAction(sig kiro.Signal, sawContent, alreadyRefreshed, hasMoreEndpoints bool) kiroAction {
	// 不变式 1：已经吐出内容就不能再重试。
	if sawContent && sig != kiro.SignalOK {
		return kiroActionAbort
	}

	switch sig {
	case kiro.SignalOK:
		return kiroActionProceed

	case kiro.SignalAuthExpired:
		if !alreadyRefreshed {
			return kiroActionRefreshAndRetry
		}
		return kiroActionFailoverAccount

	case kiro.SignalRateLimited:
		if hasMoreEndpoints {
			return kiroActionNextEndpoint
		}
		return kiroActionFailoverAccount

	case kiro.SignalCreditsExhausted:
		// 该账号额度已尽，换端点无用，直接换账号。
		return kiroActionFailoverAccount

	case kiro.SignalNetworkRegion:
		// 不变式 2：可以换端点碰运气，但绝不能归咎于账号。
		if hasMoreEndpoints {
			return kiroActionNextEndpoint
		}
		return kiroActionAbort

	case kiro.SignalBadRequest:
		// 不变式 3。
		return kiroActionAbort

	case kiro.SignalSuspended, kiro.SignalOverage:
		// 账号订阅/配置问题，换一个账号大概率能正常服务同一个请求
		// （kiro.Signal.Failoverable() 对这两个信号恒为 true，见其注释里的
		// Ruling I5）——之前这里直接 Abort，导致有问题的账号永远留在池子里
		// 且从不自愈，每个路由过去的请求都必然失败（C3）。
		return kiroActionFailoverAccount

	default:
		if hasMoreEndpoints {
			return kiroActionNextEndpoint
		}
		return kiroActionFailoverAccount
	}
}
