//go:build unit

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUserAvailableChannel_Unauthenticated401(t *testing.T) {
	// 没有 AuthSubject 注入时，handler 应返回 401 且不触达 service 依赖。
	gin.SetMode(gin.TestMode)
	h := &AvailableChannelHandler{REDACTED // nil services — 401 路径不会调用它们
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/channels/available", nil)

	h.List(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
REDACTED

func TestFilterUserVisibleGroups_IntersectionOnly(t *testing.T) {
	// 渠道挂在 {g1, g2, g3REDACTED，用户只允许 {g1, g3REDACTED —— 响应必须仅含 g1/g3。
	groups := []service.AvailableGroupRef{
		{ID: 1, Name: "g1", Platform: "anthropic"REDACTED,
		{ID: 2, Name: "g2", Platform: "anthropic"REDACTED,
		{ID: 3, Name: "g3", Platform: "openai"REDACTED,
REDACTED
	allowed := map[int64]struct{REDACTED{1: {REDACTED, 3: {REDACTEDREDACTED

	visible := filterUserVisibleGroups(groups, allowed)
	require.Len(t, visible, 2)
	ids := []int64{visible[0].ID, visible[1].IDREDACTED
	require.ElementsMatch(t, []int64{1, 3REDACTED, ids)
REDACTED

func TestToUserSupportedModels_FiltersByAllowedPlatforms(t *testing.T) {
	// 用户可访问分组只覆盖 anthropic；anthropic 平台的模型保留，openai 模型被剔除。
	src := []service.SupportedModel{
		{Name: "claude-sonnet-4-6", Platform: "anthropic", Pricing: nilREDACTED,
		{Name: "gpt-4o", Platform: "openai", Pricing: nilREDACTED,
REDACTED
	allowed := map[string]struct{REDACTED{"anthropic": {REDACTEDREDACTED
	out := toUserSupportedModels(src, allowed)
	require.Len(t, out, 1)
	require.Equal(t, "claude-sonnet-4-6", out[0].Name)
REDACTED

func TestToUserSupportedModels_NilAllowedPlatformsKeepsAll(t *testing.T) {
	// 显式传 nil allowedPlatforms 表示不做过滤。
	src := []service.SupportedModel{
		{Name: "a", Platform: "anthropic"REDACTED,
		{Name: "b", Platform: "openai"REDACTED,
REDACTED
	require.Len(t, toUserSupportedModels(src, nil), 2)
REDACTED

func TestUserAvailableChannel_FieldWhitelist(t *testing.T) {
	// 通过序列化 userAvailableChannel 结构体验证响应形状：
	// 只有 name / description / groups / supported_models；不含管理端字段。
	row := userAvailableChannel{
		Name:            "ch",
		Description:     "d",
		Groups:          []userAvailableGroup{{ID: 1, Name: "g1", Platform: "anthropic"REDACTEDREDACTED,
		SupportedModels: []userSupportedModel{REDACTED,
REDACTED
	raw, err := json.Marshal(row)
REDACTED
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	for _, key := range []string{"id", "status", "billing_model_source", "restrict_models"REDACTED {
		_, exists := decoded[key]
		require.Falsef(t, exists, "user DTO must not expose %q", key)
REDACTED
	for _, key := range []string{"name", "description", "groups", "supported_models"REDACTED {
		_, exists := decoded[key]
		require.Truef(t, exists, "user DTO must expose %q", key)
REDACTED

	// pricing interval 白名单：不应暴露 id / sort_order。
	pricing := toUserPricing(&service.ChannelModelPricing{
		BillingMode: service.BillingModeToken,
		Intervals: []service.PricingInterval{
			{ID: 7, MinTokens: 0, MaxTokens: nil, SortOrder: 3REDACTED,
	REDACTED,
REDACTED)
	require.NotNil(t, pricing)
	require.Len(t, pricing.Intervals, 1)
	rawIv, err := json.Marshal(pricing.Intervals[0])
REDACTED
	var ivDecoded map[string]any
	require.NoError(t, json.Unmarshal(rawIv, &ivDecoded))
	for _, key := range []string{"id", "pricing_id", "sort_order"REDACTED {
		_, exists := ivDecoded[key]
		require.Falsef(t, exists, "user pricing interval must not expose %q", key)
REDACTED
REDACTED
