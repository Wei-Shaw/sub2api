//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func validModelMetadataJSON() []byte {
	return []byte(`[{
      "调用名":"vision-code-model","展示名":"Vision Code Model",
      "能力边界":"支持视觉理解和代码生成，不支持图像输出。",
      "适用场景":"看图编程","类别":["多模态"],"档位条件":"<272K",
      "输入价":"¥1","输出价":"¥2","缓存读":"¥0.1","缓存写":"¥0.2",
      "术语解释":"按 1M Token 计费","上架日":"2026-09-19","授权分组":["openai-default"]
    }]`)
}

func TestParseAndValidateModelMetadata_AppliesCategoryMapping(t *testing.T) {
	records, err := ParseAndValidateModelMetadata(validModelMetadataJSON(), map[string]struct{}{"openai-default": {}})
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.ElementsMatch(t, []string{"多模态", "代码"}, records[0].Categories)
}

func TestParseAndValidateModelMetadata_RejectsIncompleteRecord(t *testing.T) {
	_, err := ParseAndValidateModelMetadata([]byte(`[{"调用名":"broken","类别":["文本"]}]`), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "输出价")
	require.Contains(t, err.Error(), "授权分组")
}

func TestParseAndValidateModelMetadata_RejectsUnknownGroup(t *testing.T) {
	_, err := ParseAndValidateModelMetadata(validModelMetadataJSON(), map[string]struct{}{"another-group": {}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "授权分组「openai-default」不存在")
}

func TestParseAndValidateModelMetadata_RejectsStringCategory(t *testing.T) {
	raw := []byte(`[{"调用名":"broken","展示名":"Broken","能力边界":"x","适用场景":"x","类别":"文本","档位条件":"<272K","输入价":"¥1","输出价":"¥2","缓存读":"¥1","缓存写":"¥1","术语解释":"x","上架日":"2026-09-19","授权分组":["g"]}]`)
	_, err := ParseAndValidateModelMetadata(raw, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "JSON 格式错误")
}

func TestParseAndNormalizeIncompleteModelMetadata_FillsPendingFields(t *testing.T) {
	raw := []byte(`[{
		"调用名":"draft-image","展示名":"Draft Image","能力边界":"文生图","适用场景":"生成素材",
		"类别":"图像","档位条件":"<272K","输入价":"¥1","输出价":"","缓存读":"¥0.1",
		"上架日":"2026-09-19"
	}]`)

	records, err := ParseAndNormalizeIncompleteModelMetadata(raw)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, IncompleteMetadataPlaceholder, records[0].OutputPrice)
	require.Equal(t, IncompleteMetadataPlaceholder, records[0].CacheWritePrice)
	require.Equal(t, IncompleteMetadataPlaceholder, records[0].Glossary)
	require.Equal(t, []string{AllAuthorizedGroupsMarker}, records[0].AuthorizedGroups)
	require.Contains(t, records[0].Categories, "图像")
}

func TestMetadataAllowsGroup_AcceptsDraftWildcard(t *testing.T) {
	require.True(t, metadataAllowsGroup(ModelMetadata{AuthorizedGroups: []string{AllAuthorizedGroupsMarker}}, "any-group"))
}
