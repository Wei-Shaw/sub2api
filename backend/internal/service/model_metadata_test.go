//go:build unit

package service

import (
	"sort"
	"strings"
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

func TestParseProjectModelMetadataDeduplicatesV7Catalog(t *testing.T) {
	records, err := parseProjectModelMetadata(modelMetadataFile)
	require.NoError(t, err)
	require.NotEmpty(t, records)

	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		key := strings.ToLower(record.CallName)
		if _, exists := seen[key]; exists {
			t.Fatalf("duplicate project metadata model: %s", record.CallName)
		}
		seen[key] = struct{}{}
		require.NotEmpty(t, record.AuthorizedGroups, record.CallName)
		require.NotEmpty(t, record.Categories, record.CallName)
	}
	require.Contains(t, seen, "gpt-5.5-pro")
}

func TestProjectModelNamesForPlatform_SeedanceUsesEmbeddedCatalog(t *testing.T) {
	names, err := ProjectModelNamesForPlatform(" Seedance ")
	require.NoError(t, err)
	require.Contains(t, names, "doubao-seedance-2-0-260128")
	require.Contains(t, names, "doubao-seedream-5-0-pro-260628")
	require.NotContains(t, names, "gpt-5.5-pro")
	require.True(t, sort.StringsAreSorted(names))
}

func TestProjectModelNamesForPlatform_RejectsUnsupportedPlatform(t *testing.T) {
	_, err := ProjectModelNamesForPlatform("openai")
	require.Error(t, err)
}

func TestParseAndValidateModelMetadata_AcceptsSeedanceV3PriceTable(t *testing.T) {
	raw := []byte(`{"模型元数据v3":[{
		"能力边界":"视频生成","适用场景":"短视频创作",
		"价格表":[{"分辨率":"480p","无视频输入（元/秒）":"0.46","有视频输入（元/个）":"2.53~5.62"}],
		"计价说明":["输出固定为5s"],"类别":"视频生成","上架日":"2026-02-12",
		"展示名":"Seedance 2.0","调用名":"doubao-seedance-2-0-260128","授权分组":"Seedance"
	}]}`)
	records, err := ParseAndValidateModelMetadata(raw, map[string]struct{}{"seedance": {}})
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.ElementsMatch(t, []string{"视频"}, records[0].Categories)
	require.Equal(t, "0.46", records[0].PriceTable[0]["无视频输入（元/秒）"])
	require.Equal(t, []string{"输出固定为5s"}, records[0].PricingNotes)
}

func TestParseAndNormalizeIncompleteModelMetadata_PreservesSeedancePriceTable(t *testing.T) {
	raw := []byte(`{"模型元数据v3":[{
		"能力边界":"视频生成","适用场景":"短视频创作",
		"价格表":[{"分辨率":"480p","无视频输入（元/秒）":"0.46"}],
		"计价说明":["输出固定为5s"],"类别":"视频生成","上架日":"2026-02-12",
		"展示名":"Seedance 2.0","调用名":"doubao-seedance-2-0-260128","授权分组":"Seedance"
	}]}`)
	records, err := ParseAndNormalizeIncompleteModelMetadata(raw)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "0.46", records[0].PriceTable[0]["无视频输入（元/秒）"])
	require.Equal(t, []string{"输出固定为5s"}, records[0].PricingNotes)
	require.Contains(t, records[0].Categories, "视频")
}
