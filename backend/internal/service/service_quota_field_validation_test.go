//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	pkgerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// extractFieldErrors 把 ApplicationError.Metadata.fields（JSON 字符串）解析回 pkgerrors.FieldError 切片，
// 让测试断言"哪个 path 触发哪个 code"，而不是依赖底层 metadata 序列化细节。
func extractFieldErrors(t *testing.T, err error) []pkgerrors.FieldError {
	t.Helper()
	require.Error(t, err)
	var appErr *pkgerrors.ApplicationError
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, ReasonServiceQuotaValidationError, appErr.Reason,
		"应是统一的字段级校验错误 reason")
	require.NotEmpty(t, appErr.Metadata["fields"], "metadata.fields 必须存在")
	var fields []pkgerrors.FieldError
	require.NoError(t, json.Unmarshal([]byte(appErr.Metadata["fields"]), &fields))
	return fields
}

// containsFieldError 断言指定 path + code 出现在 pkgerrors.FieldError 列表中。
func containsFieldError(t *testing.T, fields []pkgerrors.FieldError, path, code string) {
	t.Helper()
	for _, fe := range fields {
		if fe.Path == path && fe.Code == code {
			return
		}
	}
	t.Fatalf("expected pkgerrors.FieldError {path=%q, code=%q} not found in %+v", path, code, fields)
}

// validInput 构造一份完全合法的 input fixture，让各 case 仅修改触发错误的字段。
func validInput() *ServiceQuotaRuleInput {
	platform := "openai"
	enabled := true
	return &ServiceQuotaRuleInput{
		Enabled:     &enabled,
		CounterMode: ServiceQuotaCounterModeShared,
		Limiters: []ServiceQuotaLimiterInput{
			{LimiterType: ServiceQuotaLimiterRPM, WindowMode: ServiceQuotaWindowFixed, LimitValue: 60},
		},
		Paths: []ServiceQuotaPathInput{
			{Platform: &platform},
		},
	}
}

func TestValidateRuleFields_HappyPath(t *testing.T) {
	t.Parallel()
	require.NoError(t, validateRuleFields(validInput()))
}

func TestValidateRuleFields_LimitValueMustBePositive(t *testing.T) {
	t.Parallel()
	in := validInput()
	in.Limiters[0].LimitValue = 0
	fields := extractFieldErrors(t, validateRuleFields(in))
	containsFieldError(t, fields, "limiters[0].limit_value", FieldErrCodeMustBePositive)
}

func TestValidateRuleFields_NegativeLimitValue(t *testing.T) {
	t.Parallel()
	in := validInput()
	in.Limiters[0].LimitValue = -1
	fields := extractFieldErrors(t, validateRuleFields(in))
	containsFieldError(t, fields, "limiters[0].limit_value", FieldErrCodeMustBePositive)
}

func TestValidateRuleFields_TPMRequiresTokenComponents(t *testing.T) {
	t.Parallel()
	in := validInput()
	in.Limiters = []ServiceQuotaLimiterInput{
		{LimiterType: ServiceQuotaLimiterTPM, WindowMode: ServiceQuotaWindowFixed, LimitValue: 1000, TokenComponents: nil},
	}
	fields := extractFieldErrors(t, validateRuleFields(in))
	containsFieldError(t, fields, "limiters[0].token_components", FieldErrCodeTokenComponentsRequired)
}

func TestValidateRuleFields_TPDRequiresTokenComponents(t *testing.T) {
	t.Parallel()
	in := validInput()
	in.Limiters = []ServiceQuotaLimiterInput{
		{LimiterType: ServiceQuotaLimiterTPD, WindowMode: ServiceQuotaWindowFixed, LimitValue: 50000, TokenComponents: []string{}},
	}
	fields := extractFieldErrors(t, validateRuleFields(in))
	containsFieldError(t, fields, "limiters[0].token_components", FieldErrCodeTokenComponentsRequired)
}

// 非 TPM/TPD（rpm/concurrency/daily_usd）不应触发 TOKEN_COMPONENTS_REQUIRED，
// 即使 TokenComponents 是空切片。
func TestValidateRuleFields_RPMTokenComponentsNotRequired(t *testing.T) {
	t.Parallel()
	in := validInput()
	in.Limiters[0].TokenComponents = nil
	require.NoError(t, validateRuleFields(in))
}

func TestValidateRuleFields_PathPlatformRequired(t *testing.T) {
	t.Parallel()
	in := validInput()
	in.Paths[0].Platform = nil
	fields := extractFieldErrors(t, validateRuleFields(in))
	containsFieldError(t, fields, "paths[0].platform", FieldErrCodePlatformRequired)
}

func TestValidateRuleFields_PathPlatformEmptyString(t *testing.T) {
	t.Parallel()
	in := validInput()
	empty := ""
	in.Paths[0].Platform = &empty
	fields := extractFieldErrors(t, validateRuleFields(in))
	containsFieldError(t, fields, "paths[0].platform", FieldErrCodePlatformRequired)
}

func TestValidateRuleFields_TargetUsersRequiredWhenCounterModeUser(t *testing.T) {
	t.Parallel()
	in := validInput()
	in.CounterMode = ServiceQuotaCounterModeUser
	in.TargetUserIDs = nil
	fields := extractFieldErrors(t, validateRuleFields(in))
	containsFieldError(t, fields, "target_user_ids", FieldErrCodeTargetUsersRequired)
}

// counter_mode=shared 时不要求 target_user_ids，即使为空也应 happy path。
func TestValidateRuleFields_TargetUsersOptionalForSharedMode(t *testing.T) {
	t.Parallel()
	in := validInput() // counter_mode=shared
	require.NoError(t, validateRuleFields(in))
}

// 多个字段同时错应一次性收集，不只报第一个。
func TestValidateRuleFields_CollectsAllErrors(t *testing.T) {
	t.Parallel()
	in := validInput()
	in.CounterMode = ServiceQuotaCounterModeUser
	in.TargetUserIDs = nil
	in.Limiters[0].LimitValue = -1
	in.Paths[0].Platform = nil
	fields := extractFieldErrors(t, validateRuleFields(in))
	containsFieldError(t, fields, "target_user_ids", FieldErrCodeTargetUsersRequired)
	containsFieldError(t, fields, "limiters[0].limit_value", FieldErrCodeMustBePositive)
	containsFieldError(t, fields, "paths[0].platform", FieldErrCodePlatformRequired)
	require.Len(t, fields, 3, "三个字段错误应该都被收集")
}

func TestValidateRuleFields_NilInput(t *testing.T) {
	t.Parallel()
	fields := extractFieldErrors(t, validateRuleFields(nil))
	containsFieldError(t, fields, "", FieldErrCodeRequired)
}

// TestNormalizeAndValidate_FieldErrorsBubbleUp 验证 field 校验集成进顶层入口：
// validateRuleFields 失败时 normalizeAndValidate 必须立即返回，不会再走到链路一致性校验。
func TestNormalizeAndValidate_FieldErrorsBubbleUp(t *testing.T) {
	t.Parallel()
	repo := &fakeServiceQuotaRepo{}
	svc := &serviceQuotaService{repo: repo}

	// 没 platform → 应触发 PLATFORM_REQUIRED 字段错误（而不是后续的 mismatch 错误）。
	in := validInput()
	in.Paths[0].Platform = nil
	err := svc.normalizeAndValidate(context.Background(), in)
	fields := extractFieldErrors(t, err)
	containsFieldError(t, fields, "paths[0].platform", FieldErrCodePlatformRequired)
}
