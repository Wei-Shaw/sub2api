//go:build unit

package adobe

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

// TestImageModelIDsAreAllInDefaultMapping 守「/v1/models 返回的模型客户端必然能请求」。
//
// ImageModelIDs() 出现在 /v1/models 和分组模型选择器里；DefaultAdobeModelMapping
// 是账号默认映射的严格白名单。两处不一致 = 用户看到一个模型名，请求上去被判「不支持」。
// 这条守死了这个一致性。
func TestImageModelIDsAreAllInDefaultMapping(t *testing.T) {
	mapping := domain.DefaultAdobeModelMapping
	require.NotEmpty(t, mapping)

	for _, id := range ImageModelIDs() {
		_, ok := mapping[id]
		require.True(t, ok,
			"/v1/models 会露出 %q，但 DefaultAdobeModelMapping 里没有它——用户请求会被 IsModelSupported 挡下", id)
	}
}

// TestImageModelIDsHaveNoInternalPrefix 守「用户面永远看不到 firefly-* 内部族 id」。
//
// firefly-* 是 imageFamilySpecs.familyID 内部路由用的键，不该泄漏到 /v1/models 或
// 分组选择器。Step 8 之前 ImageModelIDs 返回的正是这些内部 id。
func TestImageModelIDsHaveNoInternalPrefix(t *testing.T) {
	for _, id := range ImageModelIDs() {
		require.NotContains(t, id, ImageModelIDPrefix,
			"用户面 id %q 不该含 firefly- 前缀", id)
	}
}
