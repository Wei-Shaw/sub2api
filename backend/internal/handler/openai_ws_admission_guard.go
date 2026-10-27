package handler

import "github.com/Wei-Shaw/sub2api/internal/service"

// openAIWSFirstAdmissionNonMigratable 判定连接首次准入是否把透传续聊帧搬到了活绑定之外的账号。
// 透传不解析帧，换账号后上下文无法恢复；ctx_pool 与 http_bridge 能重建上下文，允许迁移。
// 判据取连接建立时的原始首帧，不用 waitBudget.continuation：后续轮 failover 也会把它置真，
// 而那时载荷已由转发器重建成完整输入，可以安全换号；failover 重选一律不判（switchCount > 0）。
// lookupBound 须读到选号前的原绑定：openai 平台的 WS 入口置 DeferStickyBinding，
// 选号阶段（含加权粘性子模式负载层选中别的账号）不写绑定，统一在准入后按策略补写。
func openAIWSFirstAdmissionNonMigratable(switchCount int, firstFrameContinuation bool, admissionMode string, decision service.OpenAIAccountScheduleDecision, selectedAccountID int64, lookupBound func() int64) bool {
	if switchCount > 0 || !firstFrameContinuation || admissionMode != service.OpenAIWSIngressModePassthrough {
		return false
	}
	if decision.StickySessionHit || decision.StickyPreviousHit {
		return false
	}
	bound := lookupBound()
	return bound > 0 && bound != selectedAccountID
}

// openAIWSStickyBindPolicyFor 选出准入后写绑定的策略。
// failover 换号后新账号已过终检，绑定必须跟过去（Migrate）；调度器保留了绑定的选号
// （健康逃逸、队满溢出、guardian 父线程回退）与利润否决后的重选不改写已有异账号绑定（Preserve）；
// 其余首次准入沿用旧行为（Legacy），避免绑定卡在被跳过的不兼容账号上。非 openai 平台一律 Legacy。
func openAIWSStickyBindPolicyFor(requestPlatform string, admissionAfterFailover, profitVetoReselect bool, decision service.OpenAIAccountScheduleDecision) service.StickyBindPolicy {
	if requestPlatform != service.PlatformOpenAI {
		return service.StickyBindPolicyLegacy
	}
	if admissionAfterFailover {
		return service.StickyBindPolicyMigrate
	}
	if decision.StickyBindingPreserved || profitVetoReselect {
		return service.StickyBindPolicyPreserve
	}
	return service.StickyBindPolicyLegacy
}
