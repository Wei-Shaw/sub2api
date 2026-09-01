//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 智能路由成员配置校验回归：所有业务校验必须映射为 400（ApplicationError），
// 成员不存在透传底层 404。此前裸 errors.New 会被 ErrorFrom 兜底成 500。
func srValidationSvc(groups map[int64]*Group) *adminServiceImpl {
	return &adminServiceImpl{groupRepo: &groupRepoStubForAdmin{getByIDByID: groups}}
}

func TestValidateSmartRoutingConfig_EmptyMembersIs400(t *testing.T) {
	svc := srValidationSvc(nil)
	_, err := svc.validateSmartRoutingConfig(context.Background(), 0, nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	assert.Equal(t, "SMART_ROUTING_EMPTY_MEMBERS", infraerrors.Reason(err))
}

func TestValidateSmartRoutingConfig_SelfMemberIs400(t *testing.T) {
	svc := srValidationSvc(map[int64]*Group{10: {ID: 10, Platform: PlatformOpenAI, Status: StatusActive}})
	_, err := svc.validateSmartRoutingConfig(context.Background(), 10, []domain.SmartRoutingMember{
		{GroupID: 10, Priority: 1, Weight: 1},
	})
	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	assert.Equal(t, "SMART_ROUTING_SELF_MEMBER", infraerrors.Reason(err))
}

func TestValidateSmartRoutingConfig_NestedSmartRoutingMemberIs400(t *testing.T) {
	svc := srValidationSvc(map[int64]*Group{11: {ID: 11, Platform: PlatformSmartRouting, Status: StatusActive}})
	_, err := svc.validateSmartRoutingConfig(context.Background(), 0, []domain.SmartRoutingMember{
		{GroupID: 11, Priority: 1, Weight: 1},
	})
	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	assert.Equal(t, "SMART_ROUTING_NESTED_MEMBER", infraerrors.Reason(err))
}

func TestValidateSmartRoutingConfig_SubscriptionMemberIs400(t *testing.T) {
	svc := srValidationSvc(map[int64]*Group{12: {ID: 12, Platform: PlatformOpenAI, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}})
	_, err := svc.validateSmartRoutingConfig(context.Background(), 0, []domain.SmartRoutingMember{
		{GroupID: 12, Priority: 1, Weight: 1},
	})
	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	assert.Equal(t, "SMART_ROUTING_SUBSCRIPTION_MEMBER", infraerrors.Reason(err))
}

func TestValidateSmartRoutingConfig_MissingMemberIs404(t *testing.T) {
	svc := srValidationSvc(map[int64]*Group{})
	_, err := svc.validateSmartRoutingConfig(context.Background(), 0, []domain.SmartRoutingMember{
		{GroupID: 999, Priority: 1, Weight: 1},
	})
	require.Error(t, err)
	// 成员不存在：透传底层 ErrGroupNotFound 的 404 语义。
	assert.Equal(t, http.StatusNotFound, infraerrors.Code(err))
}

func TestValidateSmartRoutingConfig_ValidMembersNormalized(t *testing.T) {
	svc := srValidationSvc(map[int64]*Group{
		20: {ID: 20, Platform: PlatformOpenAI, Status: StatusActive},
		21: {ID: 21, Platform: PlatformAnthropic, Status: StatusActive},
	})
	out, err := svc.validateSmartRoutingConfig(context.Background(), 0, []domain.SmartRoutingMember{
		{GroupID: 21, Priority: 5, Weight: 0}, // 权重抬到下限 1，优先级靠后
		{GroupID: 20, Priority: 1, Weight: 3},
		{GroupID: 21, Priority: 9, Weight: 2}, // 去重：保留首见（权重 1、优先级 5）
	})
	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.Equal(t, int64(20), out[0].GroupID) // 优先级 1 排前
	assert.Equal(t, 1, out[0].Priority)
	assert.Equal(t, 3, out[0].Weight)
	assert.Equal(t, int64(21), out[1].GroupID)
	assert.Equal(t, 5, out[1].Priority)
	assert.Equal(t, 1, out[1].Weight)
}
