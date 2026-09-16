//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
	"github.com/stretchr/testify/require"
)

// 未显式配置 model_mapping 的 Adobe 账号应套用平台默认映射：
// 既把 OpenAI 侧的常见名字转成 Firefly 族级 id，也把不支持的模型挡在候选集之外。
func TestAccountAdobeDefaultMapping(t *testing.T) {
	account := &Account{Platform: PlatformAdobe}

	t.Run("OpenAI 侧名字映到 Firefly 族级 id", func(t *testing.T) {
		require.Equal(t, "firefly-gpt-image-2", account.GetMappedModel("gpt-image-2"))
		require.Equal(t, "firefly-gpt-image-1.5", account.GetMappedModel("gpt-image-1.5"))
		require.Equal(t, "firefly-nano-banana-pro", account.GetMappedModel("nano-banana-pro"))
		// Step 7：2.5 修回真族。sunburst 是 UI 展示名（对应上游 modelVersion=gpt-image-2.5-prism）。
		require.Equal(t, "firefly-gpt-image-2-5-prism", account.GetMappedModel("gpt-image-2.5-sunburst"))
		require.Equal(t, "firefly-gpt-image-2-5-prism", account.GetMappedModel("gpt-image-2.5-prism"))
		require.Equal(t, "firefly-gpt-image-2-5-flare", account.GetMappedModel("gpt-image-2.5-flare"))
	})

	// Step 8：内部族 id 不再出现在 DefaultAdobeModelMapping 的键里。
	// 用户请求 firefly-* 直接 IsModelSupported=false ——这是护栏：用户面看到的只有干净外部名，
	// 直接请求内部路由用的 id 就是「你试图用不该暴露的东西」，让它失败比模糊放行安全。
	t.Run("firefly-* 内部族名被挡掉", func(t *testing.T) {
		for _, familyID := range adobe.ImageFamilyModelIDs {
			require.False(t, account.IsModelSupported(familyID),
				"内部族 id %s 不应对外可请求", familyID)
		}
	})

	// 参考实现（GPT2Image-Pro 的 resolveAdobeFamilyFromModel）把一切非 firefly 名字
	// 归到 gpt-image-2；我们是严格白名单，这些名字必须显式列出才不会被挡成「无可用账号」。
	t.Run("更早的 gpt-image 名字有兜底", func(t *testing.T) {
		for _, requested := range []string{"gpt-image", "gpt-image-1", "gpt-image-1-mini"} {
			require.True(t, account.IsModelSupported(requested), "%s 应被支持", requested)
			require.Equal(t, "firefly-gpt-image-2", account.GetMappedModel(requested))
		}
	})

	t.Run("非图像模型被挡掉", func(t *testing.T) {
		require.False(t, account.IsModelSupported("gpt-4o"))
		require.False(t, account.IsModelSupported("claude-sonnet-4-6"))
		require.False(t, account.IsModelSupported("auto"))
	})
}

// 默认映射的每个目标都必须是 adobe 包能解析的族级 id，
// 否则请求会在网关侧才炸而不是在这里被发现。
func TestAdobeDefaultMappingTargetsResolve(t *testing.T) {
	account := &Account{Platform: PlatformAdobe}
	for _, requested := range []string{
		"gpt-image-2", "gpt-image-1.5", "gpt-image-2.5-sunburst", "gpt-image-2.5-flare", "gpt-image-2.5-prism",
		"nano-banana", "nano-banana2", "nano-banana-pro",
		"gpt-image", "gpt-image-1", "gpt-image-1-mini",
	} {
		mapped := account.GetMappedModel(requested)
		conf, err := adobe.ResolveImage(adobe.ImageRequest{ModelID: mapped, Size: "1024x1024"})
		require.NoError(t, err, "%s → %s 应可解析", requested, mapped)
		require.Equal(t, mapped, conf.Family)
	}
}

// 账号显式配置的 model_mapping 应完全覆盖默认表（含通配符）。
func TestAccountAdobeExplicitMappingOverridesDefault(t *testing.T) {
	account := &Account{
		Platform: PlatformAdobe,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"gpt-image-*": "firefly-nano-banana-pro",
			},
		},
	}
	require.Equal(t, "firefly-nano-banana-pro", account.GetMappedModel("gpt-image-2"))
	require.Equal(t, "firefly-nano-banana-pro", account.GetMappedModel("gpt-image-9-future"))
	require.False(t, account.IsModelSupported("nano-banana"))
}

// 默认表里的每个键都必须真的能走通网关：既被白名单放行，目标又能解析。
// 逐键遍历而非手写清单，新增键时不会漏测。
func TestAdobeDefaultMappingEveryKeyIsServable(t *testing.T) {
	account := &Account{Platform: PlatformAdobe}
	require.NotEmpty(t, domain.DefaultAdobeModelMapping)

	for requested, target := range domain.DefaultAdobeModelMapping {
		require.True(t, account.IsModelSupported(requested), "%s 应被白名单放行", requested)
		require.Equal(t, target, account.GetMappedModel(requested))

		conf, err := adobe.ResolveImage(adobe.ImageRequest{ModelID: target, Size: "1024x1024"})
		require.NoError(t, err, "%s → %s 应可解析", requested, target)
		require.Equal(t, target, conf.Family)
	}
}

// 视频协议层已就绪但网关未接线，58 个视频 id 不能出现在分组模型选择器里 ——
// 选了必然失败，且现象与「没有可用账号」一模一样、无从排查。
func TestAdobeModelsListCandidatesExcludeVideo(t *testing.T) {
	candidates := defaultModelsListCandidateIDs(PlatformAdobe)

	// Step 8：外露的是干净外部名（adobe.ImageModelIDs），不是内部族 id。
	require.Equal(t, adobe.ImageModelIDs(), candidates)
	for _, id := range candidates {
		require.NotContains(t, adobe.VideoModelIDs, id, "视频 id 不应出现在图像模型清单里")
		require.False(t, strings.HasPrefix(id, "firefly-"),
			"用户面清单不该含 firefly-* 前缀 id: %s", id)
	}
	require.NotEmpty(t, adobe.VideoModelIDs, "视频目录仍应存在，只是不对外列出")

	// 返回的必须是新切片：调用方会对它 append 账号自配的模型名。
	candidates = append(candidates, "sentinel")
	require.NotContains(t, adobe.ImageFamilyModelIDs, "sentinel")
}

// Step 10 守卫：别名表下沉到 adobe 包之后，两处必须逐条一致。
//
// 刻意没有把它们合并成一张表——domain.DefaultAdobeModelMapping 还承担「没配置时的
// 默认允许集」这个语义，合并会牵动 constants.go 的既有测试与
// defaultModelMappingForPlatform。代价就是漂移风险，由这条用例兜住。
func TestAdobeExternalAliasesMatchDefaultModelMapping(t *testing.T) {
	require.Equal(t, domain.DefaultAdobeModelMapping, adobe.ExternalImageModelAliases(),
		"adobe.externalImageModelAliases 与 domain.DefaultAdobeModelMapping 必须逐条一致")
}

// 白名单模式产出的是恒等对，GetMappedModel 原样返回对外名——ResolveImage 必须认得它。
// 这条路在 Step 10 之前必然 500（PickImageFamily 对无 firefly- 前缀的 id 直接 false）。
func TestAdobeWhitelistIdentityMappingResolves(t *testing.T) {
	account := &Account{
		Platform: PlatformAdobe,
		Credentials: map[string]any{
			"model_mapping": map[string]any{
				"imagen-4": "imagen-4",
				"flux-pro": "flux-pro",
			},
		},
	}

	for requested, wantFamily := range map[string]string{
		"imagen-4": "firefly-imagen-4",
		"flux-pro": "firefly-flux-pro",
	} {
		require.True(t, account.IsModelSupported(requested), requested)
		mapped := account.GetMappedModel(requested)
		require.Equal(t, requested, mapped, "白名单是恒等对，映射后应原样返回")

		conf, err := adobe.ResolveImage(adobe.ImageRequest{ModelID: mapped, Size: "1024x1024"})
		require.NoError(t, err, requested)
		require.Equal(t, wantFamily, conf.Family, requested)
	}

	// 别名表不能旁路严格白名单：没勾的模型仍然不可请求。
	require.False(t, account.IsModelSupported("nano-banana"),
		"别名表只负责翻译，准入仍由 model_mapping 的严格白名单决定")
	require.False(t, account.IsModelSupported("gpt-image-2"))
}
