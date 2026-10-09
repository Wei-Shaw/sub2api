package service

import (
	"fmt"
	"time"
)

// GitHubAPIError 仅携带可公开的诊断信息，不包含请求 URL、凭据或原始响应体。
// 发布客户端保留这些字段，调用方才能区分权限失败与限流并尊重冷却时间。
type GitHubAPIError struct {
	StatusCode       int
	Code             string
	RateLimitResetAt *time.Time
	RetryAt          *time.Time
}

func (e *GitHubAPIError) Error() string {
	return fmt.Sprintf("GitHub API returned %d (%s)", e.StatusCode, e.Code)
}

func (e *GitHubAPIError) IsRateLimited() bool {
	return e.Code == "github_primary_rate_limit" || e.Code == "github_secondary_rate_limit" || e.Code == "github_rate_limit"
}
