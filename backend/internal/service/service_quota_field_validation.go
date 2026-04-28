package service

import (
	"strconv"

	pkgerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ServiceQuotaValidationError* 是字段级校验错误码常量，与前端 ServiceQuotaErrorCode 枚举一一对应。
//
// 前端把这些 code 映射到 i18n key 渲染本地化文案；后端 message 是英文供开发者排查，
// 不参与最终用户展示（CLAUDE.md §"前端 API 错误处理"约定）。
const (
	// ReasonServiceQuotaValidationError 是顶层 ApplicationError.Reason，前端按这个值识别"字段级校验错误"。
	ReasonServiceQuotaValidationError = "SERVICE_QUOTA_VALIDATION_ERROR"

	// 字段错误码（与 frontend/src/utils/serviceQuotaError.ts 的 ServiceQuotaErrorCode 同步）。
	FieldErrCodeRequired                = "REQUIRED"
	FieldErrCodeMustBePositive          = "MUST_BE_POSITIVE"
	FieldErrCodeTargetUsersRequired     = "TARGET_USERS_REQUIRED"
	FieldErrCodeTokenComponentsRequired = "TOKEN_COMPONENTS_REQUIRED"
	FieldErrCodePlatformRequired        = "PLATFORM_REQUIRED"
	FieldErrCodeInvalidValue            = "INVALID_VALUE"
)

// validateRuleFields 是 normalizeAndValidate 之外的"字段级硬校验"补强：
// 收集所有字段错误一次性返回，让前端能同时高亮多个出错控件，而不是一个个 reject。
//
// 与 normalizeAndValidate 的关系：normalizeAndValidate 仍负责 enum 合法性 + 链路一致性
// （account ⊂ group / channel 服务范围等）这些"业务校验"；validateRuleFields 只负责
// "前端表单本应拦下、但请求绕过 UI 被直接 POST 时的兜底"。
//
// 实现委托给 pkgerrors.FieldErrorCollector（项目级共享基础设施），让 service quota 与
// 其它 admin 模块共用同一份"字段错误收集 + 序列化"机制；errors 为 nil 时 Build() 返回 nil
// 让调用方 if err != nil 判断保持惯性。
//
// 校验项（与前端 ServiceQuotaErrorCode 对齐）：
//   - counter_mode == 'user' 时 target_user_ids 不能为空 → TARGET_USERS_REQUIRED
//   - 每个 limiter：limit_value > 0（MUST_BE_POSITIVE）
//   - TPM/TPD 每个 limiter：token_components 至少 1 项（TOKEN_COMPONENTS_REQUIRED）
//   - 每个 path：platform 必须非空（PLATFORM_REQUIRED；存量 NULL 老规则保留读但不能再保存）
func validateRuleFields(input *ServiceQuotaRuleInput) error {
	c := pkgerrors.NewFieldErrorCollector(ReasonServiceQuotaValidationError).
		WithMessage("service quota validation failed")
	if input == nil {
		c.Add("", FieldErrCodeRequired)
		// Build 返回 *ApplicationError；外部签名 error，赋值时让 nil 走 nil 分支。
		if err := c.Build(); err != nil {
			return err
		}
		return nil
	}

	if input.CounterMode == ServiceQuotaCounterModeUser && len(input.TargetUserIDs) == 0 {
		c.Add("target_user_ids", FieldErrCodeTargetUsersRequired)
	}

	for i, l := range input.Limiters {
		base := "limiters[" + strconv.Itoa(i) + "]"
		if l.LimitValue <= 0 {
			c.Add(base+".limit_value", FieldErrCodeMustBePositive)
		}
		if ServiceQuotaLimiterTypeUsesTokenComponents(l.LimiterType) && len(l.TokenComponents) == 0 {
			c.Add(base+".token_components", FieldErrCodeTokenComponentsRequired)
		}
	}

	for i, p := range input.Paths {
		base := "paths[" + strconv.Itoa(i) + "]"
		// platform 必填：存量 NULL 老规则在保存（Update）时强制要求补齐——存量在 Read 路径仍然返回，
		// admin UI 列表会标 warning 提示用户编辑修复。
		if p.Platform == nil || *p.Platform == "" {
			c.Add(base+".platform", FieldErrCodePlatformRequired)
		}
	}

	if err := c.Build(); err != nil {
		return err
	}
	return nil
}
