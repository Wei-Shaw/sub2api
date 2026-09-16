package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// Codex manifest 标准化后只保留 slug，测试弹窗用 display_name 当选项标签，
// 留空会让模型选择器渲染成一排空白项。已知 slug 由上游的标签表补成人类可读名
// （gpt-5.6-terra → "GPT-5.6 Terra"），因此这里只断言非空，不再要求与 ID 相等。
func TestFetchOpenAIAccountModelsFillsPickerLabels(t *testing.T) {
	newCodexModelsOAuthCacheServer(t, `{"models":[{"slug":"gpt-5.6-terra"},{"slug":"codex-auto-review"}]}`)
	svc := &AccountTestService{}
	svc.SetOpenAIGatewayService(&OpenAIGatewayService{})

	models, err := svc.FetchOpenAIAccountModels(context.Background(), newCodexModelsTestAccount())
	require.NoError(t, err)
	// manifest 里的 slug 排在最前，其后是上游内置的生图模型（它们自带人类可读标签）。
	require.GreaterOrEqual(t, len(models), 2)
	for _, model := range models {
		require.NotEmpty(t, model.DisplayName, "picker label must not be empty for %q", model.ID)
		require.Equal(t, "model", model.Type)
	}
	require.Equal(t, "gpt-5.6-terra", models[0].ID)
	require.Equal(t, "codex-auto-review", models[1].ID)
}
