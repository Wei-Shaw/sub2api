package service

import (
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// 智力检测的错误值。多数业务性失败（没配模型、没有 HTML、作品过大）不返回 error，
// 而是写进跑测记录的 error_code 字段；这里只放"整个功能不可用"这类需要 HTTP 状态码的错误。
var (
	// ErrIntelligenceCheckDisabled 功能总开关关闭时，用户侧作品墙整体不可用。
	ErrIntelligenceCheckDisabled = infraerrors.Forbidden(
		"INTELLIGENCE_CHECK_DISABLED",
		"intelligence check feature is disabled",
	)
	// ErrIntelligenceCheckInvalidVerdict 评审结论非法：只接受 pass / fail。
	ErrIntelligenceCheckInvalidVerdict = infraerrors.BadRequest(
		"INTELLIGENCE_CHECK_INVALID_VERDICT",
		"verdict must be either pass or fail",
	)
	// ErrIntelligenceCheckNotReviewable 该记录没有可评审的作品：
	// 跑测还在排队/执行中，或执行失败（超时、未配模型、没抽出 HTML）。
	ErrIntelligenceCheckNotReviewable = infraerrors.BadRequest(
		"INTELLIGENCE_CHECK_NOT_REVIEWABLE",
		"this run has no artwork to review",
	)
)
