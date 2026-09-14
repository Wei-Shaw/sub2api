//go:build unit

package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
	"github.com/stretchr/testify/require"
)

// validateOpenAIImagesModel 校验的是 **未经映射的原始请求模型**
// （parseOpenAIImagesRequest 在解析阶段就调它），而 Adobe 的
// /v1/images/generations 走的正是这个解析器。
//
// Step 8 把对外模型名改成干净外部名之后，13 个名字里只有 gpt-image-* 前缀那 4 个
// 能蒙混过 isOpenAIImageGenerationModel，其余 9 个（imagen-4 / flux-pro / …）
// 在解析阶段就被拒，报 "images endpoint requires an image model"。
// 这条用例把那个现网 bug 钉死。
func TestValidateOpenAIImagesModelAcceptsAdobeExternalNames(t *testing.T) {
	for _, model := range adobe.ImageModelIDs() {
		require.NoError(t, validateOpenAIImagesModel(model), "对外名 %s 必须放行", model)
	}
}

func TestValidateOpenAIImagesModelKeepsExistingBehaviour(t *testing.T) {
	// Firefly 内部族 id 与全量 id 照旧放行。
	require.NoError(t, validateOpenAIImagesModel("firefly-imagen-4"))
	require.NoError(t, validateOpenAIImagesModel("firefly-gpt-image-2-2k-16x9"))
	// OpenAI 原生生图模型照旧放行。
	require.NoError(t, validateOpenAIImagesModel("gpt-image-1"))
	require.NoError(t, validateOpenAIImagesModel("gpt-image-2.5-flare"))
	// Grok 生图照旧放行。
	require.NoError(t, validateOpenAIImagesModel("grok-imagine"))

	// 非生图模型与空值照旧拒绝——放行对外名不能顺带放宽这一层。
	require.Error(t, validateOpenAIImagesModel(""))
	require.Error(t, validateOpenAIImagesModel("gpt-4o"))
	require.Error(t, validateOpenAIImagesModel("claude-opus-5"))
}
