//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPropagateAccountProxyToShadows 外审第8轮:CRS/AdminService 改母账号 proxy 后,
// 影子 proxy 必须跟随(影子 proxy 恒继承母账号,否则出站漂移)。
func TestPropagateAccountProxyToShadows(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()

	oldProxy := int64(11)
	mother := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, ProxyID: &oldProxy}
	require.NoError(t, repo.Create(ctx, mother))
	parentID := mother.ID

	shadow := &Account{
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
		ProxyID:         &oldProxy,
	}
	require.NoError(t, repo.Create(ctx, shadow))

	newProxy := int64(22)
	require.NoError(t, propagateAccountProxyToShadows(ctx, repo, parentID, &newProxy))

	got, err := repo.GetByID(ctx, shadow.ID)
	require.NoError(t, err)
	require.NotNil(t, got.ProxyID)
	require.Equal(t, newProxy, *got.ProxyID, "shadow proxy must follow the parent's new proxy")

	// 清空母 proxy 也应传播为 nil。
	require.NoError(t, propagateAccountProxyToShadows(ctx, repo, parentID, nil))
	got, err = repo.GetByID(ctx, shadow.ID)
	require.NoError(t, err)
	require.Nil(t, got.ProxyID, "clearing parent proxy must clear the shadow proxy too")
}

// TestGuardCRSShadowParentInvariant 外审第8/9轮:有 spark 影子的母账号经 CRS 任意分支更新后,目标结果
// 必须仍是 OpenAI OAuth;否则(改 api_key 或跨平台 Anthropic/Gemini)影子读透母凭据失败、spark 全崩。
func TestGuardCRSShadowParentInvariant(t *testing.T) {
	ctx := context.Background()
	repo := newSparkShadowRepoStub()

	mother := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.NoError(t, repo.Create(ctx, mother))
	parentID := mother.ID

	// 无影子:任何目标都放行(含改离 OpenAI OAuth)。
	require.NoError(t, guardCRSShadowParentInvariant(ctx, repo, mother, PlatformOpenAI, AccountTypeAPIKey))
	require.NoError(t, guardCRSShadowParentInvariant(ctx, repo, mother, PlatformAnthropic, AccountTypeOAuth))

	// 建一个影子后:
	shadow := &Account{
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
	}
	require.NoError(t, repo.Create(ctx, shadow))

	// 翻成 OpenAI api_key 被拒。
	err := guardCRSShadowParentInvariant(ctx, repo, mother, PlatformOpenAI, AccountTypeAPIKey)
	require.Error(t, err, "must reject converting a shadow parent to openai api_key")
	require.Contains(t, err.Error(), "spark-shadow parent")

	// 跨平台改成 Anthropic OAuth(Type 仍 OAuth、仅 Platform 变)也被拒——第8轮只查 Type 的版本会漏。
	require.Error(t, guardCRSShadowParentInvariant(ctx, repo, mother, PlatformAnthropic, AccountTypeOAuth),
		"must reject moving a shadow parent to a non-OpenAI platform even if type stays oauth")

	// 改成 Gemini api_key 被拒。
	require.Error(t, guardCRSShadowParentInvariant(ctx, repo, mother, PlatformGemini, AccountTypeAPIKey))

	// 保持 OpenAI OAuth(重新同步母账号)放行,即便仍有影子。
	require.NoError(t, guardCRSShadowParentInvariant(ctx, repo, mother, PlatformOpenAI, AccountTypeOAuth))
}
