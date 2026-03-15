package service

import "time"

// ProviderRefreshErrorAction 定义 provider 在刷新失败时的处理动作。
type ProviderRefreshErrorAction int

const (
	// ProviderRefreshErrorReturn 失败即返回错误（不降级旧 token）。
	ProviderRefreshErrorReturn ProviderRefreshErrorAction = iota
	// ProviderRefreshErrorUseExistingToken 失败后继续使用现有 token。
	ProviderRefreshErrorUseExistingToken
)

// ProviderLockHeldAction 定义 provider 在刷新锁被占用时的处理动作。
type ProviderLockHeldAction int

const (
	// ProviderLockHeldUseExistingToken 直接使用现有 token。
	ProviderLockHeldUseExistingToken ProviderLockHeldAction = iota
	// ProviderLockHeldWaitForCache 等待后重试缓存读取。
	ProviderLockHeldWaitForCache
)

// ProviderRefreshPolicy 描述 provider 的平台差异策略。
type ProviderRefreshPolicy struct {
	OnRefreshError ProviderRefreshErrorAction
	OnLockHeld     ProviderLockHeldAction
	FailureTTL     time.Duration
}

func ClaudeProviderRefreshPolicy() ProviderRefreshPolicy {
	return ProviderRefreshPolicy{
		OnRefreshError: ProviderRefreshErrorUseExistingToken,
		OnLockHeld:     ProviderLockHeldWaitForCache,
		FailureTTL:     time.Minute,
	}
}

func OpenAIProviderRefreshPolicy() ProviderRefreshPolicy {
	return ProviderRefreshPolicy{
		OnRefreshError: ProviderRefreshErrorUseExistingToken,
		OnLockHeld:     ProviderLockHeldWaitForCache,
		FailureTTL:     time.Minute,
	}
}

func GeminiProviderRefreshPolicy() ProviderRefreshPolicy {
	return ProviderRefreshPolicy{
		OnRefreshError: ProviderRefreshErrorReturn,
		OnLockHeld:     ProviderLockHeldUseExistingToken,
		FailureTTL:     0,
	}
}

func AntigravityProviderRefreshPolicy() ProviderRefreshPolicy {
	return ProviderRefreshPolicy{
		OnRefreshError: ProviderRefreshErrorReturn,
		OnLockHeld:     ProviderLockHeldUseExistingToken,
		FailureTTL:     0,
	}
}

// BackgroundSkipAction 定义后台刷新服务在“未实际刷新”场景的计数方式。
type BackgroundSkipAction int

const (
	// BackgroundSkipAsSkipped 计入 skipped（保持当前默认行为）。
	BackgroundSkipAsSkipped BackgroundSkipAction = iota
	// BackgroundSkipAsSuccess 计入 success（仅用于兼容旧统计口径时可选）。
	BackgroundSkipAsSuccess
)

// BackgroundRefreshPolicy 描述后台刷新服务的调用侧策略。
type BackgroundRefreshPolicy struct {
	OnLockHeld       BackgroundSkipAction
	OnAlreadyRefresh BackgroundSkipAction
}

func DefaultBackgroundRefreshPolicy() BackgroundRefreshPolicy {
	return BackgroundRefreshPolicy{
		OnLockHeld:       BackgroundSkipAsSkipped,
		OnAlreadyRefresh: BackgroundSkipAsSkipped,
	}
}

func (p BackgroundRefreshPolicy) handleLockHeld() error {
	if p.OnLockHeld == BackgroundSkipAsSuccess {
		return nil
	}
	return errRefreshSkipped
}

func (p BackgroundRefreshPolicy) handleAlreadyRefreshed() error {
	if p.OnAlreadyRefresh == BackgroundSkipAsSuccess {
		return nil
	}
	return errRefreshSkipped
}
