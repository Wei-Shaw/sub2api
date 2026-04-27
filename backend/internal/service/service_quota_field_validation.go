package service

import (
	"encoding/json"
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

// fieldError 描述单个字段的校验失败。Path 是 JSON 路径字符串（前端按此定位 UI 控件），
// Code 是 FieldErrCode* 之一。
type fieldError struct {
	Path string `json:"path"`
	Code string `json:"code"`
}

// serviceQuotaValidationError 收集多个 fieldError，构造统一的 ApplicationError。
//
// 设计：
//   - reason 固定为 ReasonServiceQuotaValidationError，前端按此识别"字段级校验错误"分支
//   - metadata.fields 是 fieldError 列表的 JSON 字符串（Metadata 是 map[string]string，
//     不能直接放数组对象，所以序列化为字符串；前端 parse JSON 后渲染）
//   - errors 为空时返回 nil，让调用方的 if err != nil 判断保持惯性
type validationFieldCollector struct {
	errors []fieldError
}

func (c *validationFieldCollector) add(path, code string) {
	c.errors = append(c.errors, fieldError{Path: path, Code: code})
}

func (c *validationFieldCollector) hasErrors() bool {
	return len(c.errors) > 0
}

// build 把收集到的字段错误打包成 ApplicationError。
//
// 没有错误时返回 nil（让调用方"err == nil 即通过"的检查继续工作）。
// JSON 序列化理论上不会失败（只含 string / 嵌套 string），失败也吞掉返回字段更少的同款错误，
// 让校验入口永远能给出可读响应。
func (c *validationFieldCollector) build() error {
	if !c.hasErrors() {
		return nil
	}
	payload, err := json.Marshal(c.errors)
	meta := map[string]string{"count": strconv.Itoa(len(c.errors))}
	if err == nil {
		meta["fields"] = string(payload)
	}
	return pkgerrors.BadRequest(
		ReasonServiceQuotaValidationError,
		"service quota validation failed",
	).WithMetadata(meta)
}

// validateRuleFields 是 normalizeAndValidate 之外的"字段级硬校验"补强：
// 收集所有字段错误一次性返回，让前端能同时高亮多个出错控件，而不是一个个 reject。
//
// 与 normalizeAndValidate 的关系：normalizeAndValidate 仍负责 enum 合法性 + 链路一致性
// （account ⊂ group / channel 服务范围等）这些"业务校验"；validateRuleFields 只负责
// "前端表单本应拦下、但请求绕过 UI 被直接 POST 时的兜底"。
//
// 校验项（与前端 ServiceQuotaErrorCode 对齐）：
//   - counter_mode == 'user' 时 target_user_ids 不能为空 → TARGET_USERS_REQUIRED
//   - 每个 limiter：limit_value > 0（MUST_BE_POSITIVE）
//   - TPM/TPD 每个 limiter：token_components 至少 1 项（TOKEN_COMPONENTS_REQUIRED）
//   - 每个 path：platform 必须非空（PLATFORM_REQUIRED；存量 NULL 老规则保留读但不能再保存）
func validateRuleFields(input *ServiceQuotaRuleInput) error {
	c := &validationFieldCollector{}
	if input == nil {
		c.add("", FieldErrCodeRequired)
		return c.build()
	}

	if input.CounterMode == ServiceQuotaCounterModeUser && len(input.TargetUserIDs) == 0 {
		c.add("target_user_ids", FieldErrCodeTargetUsersRequired)
	}

	for i, l := range input.Limiters {
		base := "limiters[" + strconv.Itoa(i) + "]"
		if l.LimitValue <= 0 {
			c.add(base+".limit_value", FieldErrCodeMustBePositive)
		}
		if ServiceQuotaLimiterTypeUsesTokenComponents(l.LimiterType) && len(l.TokenComponents) == 0 {
			c.add(base+".token_components", FieldErrCodeTokenComponentsRequired)
		}
	}

	for i, p := range input.Paths {
		base := "paths[" + strconv.Itoa(i) + "]"
		// platform 必填：存量 NULL 老规则在保存（Update）时强制要求补齐——存量在 Read 路径仍然返回，
		// admin UI 列表会标 warning 提示用户编辑修复。
		if p.Platform == nil || *p.Platform == "" {
			c.add(base+".platform", FieldErrCodePlatformRequired)
		}
	}

	return c.build()
}
