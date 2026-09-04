package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// changeParentRepo 复用上游计费探测的轻量 fake repo，并额外实现 spark 影子
// 所需的一母一影查询，避免在核心测试里引入真实 DB。
type changeParentRepo struct {
	*upstreamBillingProbeAccountRepo
	shadowsByParent map[int64][]*Account
}

func (r *changeParentRepo) ListShadowsByParent(_ context.Context, parentID int64) ([]*Account, error) {
	return r.shadowsByParent[parentID], nil
}

func TestAdminUpdateAccountChangeShadowParentLinked(t *testing.T) {
	shadowID := int64(100)
	oldParentID := int64(10)
	newParentID := int64(20)
	oldProxy := int64(9)
	newProxy := int64(29)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		shadowID: {
			ID:              shadowID,
			Name:            "linked-shadow",
			Platform:        PlatformOpenAI,
			Type:            AccountTypeOAuth,
			ParentAccountID: &oldParentID,
			QuotaDimension:  QuotaDimensionLinked,
			ProxyID:         &oldProxy,
			Status:          StatusActive,
		},
		oldParentID: {
			ID:       oldParentID,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
		},
		newParentID: {
			ID:       newParentID,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
			ProxyID:  &newProxy,
		},
	}}
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.UpdateAccount(context.Background(), shadowID, &UpdateAccountInput{
		ParentAccountID: &newParentID,
	})
	require.NoError(t, err)
	require.NotNil(t, updated.ParentAccountID)
	require.Equal(t, newParentID, *updated.ParentAccountID)
	require.NotNil(t, updated.ProxyID)
	require.Equal(t, newProxy, *updated.ProxyID)
}

func TestAdminUpdateAccountChangeShadowParentErrors(t *testing.T) {
	shadowID := int64(100)
	oldParentID := int64(10)
	otherShadowID := int64(30)
	missingID := int64(999)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		shadowID: {
			ID:              shadowID,
			Name:            "linked-shadow",
			Platform:        PlatformOpenAI,
			Type:            AccountTypeOAuth,
			ParentAccountID: &oldParentID,
			QuotaDimension:  QuotaDimensionLinked,
			Status:          StatusActive,
		},
		oldParentID: {
			ID:       oldParentID,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
		},
		otherShadowID: {
			ID:              otherShadowID,
			Platform:        PlatformOpenAI,
			Type:            AccountTypeOAuth,
			ParentAccountID: &oldParentID,
			QuotaDimension:  QuotaDimensionLinked,
			Status:          StatusActive,
		},
	}}
	svc := &adminServiceImpl{accountRepo: repo}

	// 自关联
	selfID := shadowID
	_, err := svc.UpdateAccount(context.Background(), shadowID, &UpdateAccountInput{ParentAccountID: &selfID})
	require.ErrorContains(t, err, "cannot be linked to itself")

	// 目标不存在
	_, err = svc.UpdateAccount(context.Background(), shadowID, &UpdateAccountInput{ParentAccountID: &missingID})
	require.ErrorContains(t, err, "target parent account not found")

	// 目标是影子
	_, err = svc.UpdateAccount(context.Background(), shadowID, &UpdateAccountInput{ParentAccountID: &otherShadowID})
	require.ErrorContains(t, err, "not another shadow")

	// 平台不一致
	otherPlatformID := int64(40)
	repo.accounts[otherPlatformID] = &Account{
		ID:       otherPlatformID,
		Platform: PlatformDeepseek,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
	}
	_, err = svc.UpdateAccount(context.Background(), shadowID, &UpdateAccountInput{ParentAccountID: &otherPlatformID})
	require.ErrorContains(t, err, "platform must match")
}

func TestAdminUpdateAccountChangeSparkShadowParentConflict(t *testing.T) {
	shadowID := int64(100)
	oldParentID := int64(10)
	newParentID := int64(20)
	existingSparkID := int64(200)
	repo := &changeParentRepo{
		upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
			shadowID: {
				ID:              shadowID,
				Name:            "spark-shadow",
				Platform:        PlatformOpenAI,
				Type:            AccountTypeOAuth,
				ParentAccountID: &oldParentID,
				QuotaDimension:  QuotaDimensionSpark,
				Status:          StatusActive,
			},
			oldParentID: {
				ID:       oldParentID,
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Status:   StatusActive,
			},
			newParentID: {
				ID:       newParentID,
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Status:   StatusActive,
			},
			existingSparkID: {
				ID:              existingSparkID,
				Platform:        PlatformOpenAI,
				Type:            AccountTypeOAuth,
				ParentAccountID: &newParentID,
				QuotaDimension:  QuotaDimensionSpark,
				Status:          StatusActive,
			},
		}},
		shadowsByParent: map[int64][]*Account{
			newParentID: {
				{ID: existingSparkID, ParentAccountID: &newParentID, QuotaDimension: QuotaDimensionSpark},
			},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	_, err := svc.UpdateAccount(context.Background(), shadowID, &UpdateAccountInput{ParentAccountID: &newParentID})
	require.ErrorContains(t, err, "already has a spark shadow account")
}

func TestAdminUpdateAccountChangeSparkShadowParentOK(t *testing.T) {
	shadowID := int64(100)
	oldParentID := int64(10)
	newParentID := int64(20)
	newProxy := int64(29)
	repo := &changeParentRepo{
		upstreamBillingProbeAccountRepo: &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
			shadowID: {
				ID:              shadowID,
				Name:            "spark-shadow",
				Platform:        PlatformOpenAI,
				Type:            AccountTypeOAuth,
				ParentAccountID: &oldParentID,
				QuotaDimension:  QuotaDimensionSpark,
				Status:          StatusActive,
			},
			oldParentID: {
				ID:       oldParentID,
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Status:   StatusActive,
			},
			newParentID: {
				ID:       newParentID,
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Status:   StatusActive,
				ProxyID:  &newProxy,
			},
		}},
		shadowsByParent: map[int64][]*Account{},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.UpdateAccount(context.Background(), shadowID, &UpdateAccountInput{ParentAccountID: &newParentID})
	require.NoError(t, err)
	require.Equal(t, newParentID, *updated.ParentAccountID)
	require.Equal(t, newProxy, *updated.ProxyID)
}

func TestAdminUpdateAccountIgnoresParentForNonShadow(t *testing.T) {
	accountID := int64(100)
	parentID := int64(20)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:       accountID,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
		},
		parentID: {
			ID:       parentID,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Status:   StatusActive,
		},
	}}
	svc := &adminServiceImpl{accountRepo: repo}

	updated, err := svc.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{ParentAccountID: &parentID})
	require.NoError(t, err)
	require.Nil(t, updated.ParentAccountID)
}
