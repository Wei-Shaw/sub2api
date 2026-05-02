package service

import "context"

// OpenAI403CounterCache 追踪 OpenAI 账号连续 403 失败次数。
type OpenAI403CounterCache interface {
	// IncrementOpenAI403Count 原子递增 403 计数并返回当前值。
	IncrementOpenAI403Count(ctx context.Context, accountID int64, windowMinutes int) (int64, error)
	// ResetOpenAI403Count 成功后清零计数器。
	ResetOpenAI403Count(ctx context.Context, accountID int64) error
}
