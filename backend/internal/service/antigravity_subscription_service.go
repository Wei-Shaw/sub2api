package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

const antigravitySubscriptionAbnormal = "abnormal"

// AntigravitySubscriptionResult 表示订阅检测后的规范化结果。
type AntigravitySubscriptionResult struct {
	PlanType           string
	SubscriptionStatus string
	SubscriptionError  string
REDACTED

// NormalizeAntigravitySubscription 从 LoadCodeAssistResponse 提取 plan_type + 异常状态。
// 使用 GetTier()（返回 tier ID）+ TierIDToPlanType 映射。
func NormalizeAntigravitySubscription(resp *antigravity.LoadCodeAssistResponse) AntigravitySubscriptionResult {
	if resp == nil {
		return AntigravitySubscriptionResult{PlanType: "Free"REDACTED
REDACTED
	if len(resp.IneligibleTiers) > 0 {
		result := AntigravitySubscriptionResult{
			PlanType:           "Abnormal",
			SubscriptionStatus: antigravitySubscriptionAbnormal,
	REDACTED
		if resp.IneligibleTiers[0] != nil {
			result.SubscriptionError = strings.TrimSpace(resp.IneligibleTiers[0].ReasonMessage)
	REDACTED
		return result
REDACTED
	tierID := resp.GetTier()
	return AntigravitySubscriptionResult{
		PlanType: antigravity.TierIDToPlanType(tierID),
REDACTED
REDACTED

func applyAntigravitySubscriptionResult(account *Account, result AntigravitySubscriptionResult) (map[string]any, map[string]any) {
	credentials := make(map[string]any)
	for k, v := range account.Credentials {
		credentials[k] = v
REDACTED
	credentials["plan_type"] = result.PlanType

	extra := make(map[string]any)
	for k, v := range account.Extra {
		extra[k] = v
REDACTED
	if result.SubscriptionStatus != "" {
		extra["subscription_status"] = result.SubscriptionStatus
REDACTED else {
		delete(extra, "subscription_status")
REDACTED
	if result.SubscriptionError != "" {
		extra["subscription_error"] = result.SubscriptionError
REDACTED else {
		delete(extra, "subscription_error")
REDACTED
	return credentials, extra
REDACTED
