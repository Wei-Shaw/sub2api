//go:build unit

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// performProviderPricing 走真实的 gin 路由栈打一次请求，拿原始响应体。
func performProviderPricing(t *testing.T, filePath string) (int, string) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	h := NewProviderPricingHandler(service.NewProviderPricingService(filePath))
	engine.GET("/api/provider/pricing", h.GetPricing)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/provider/pricing", nil))
	return recorder.Code, recorder.Body.String()
}

// 这个接口是唯一刻意不用 {code,message,data} 信封的对外接口，
// 第三方已依赖旧 companion 的 {schema_version,success,message,data} 形状。
// 本用例把顶层结构逐键锁死，防止后人"顺手统一风格"而悄悄改坏第三方契约。
func TestProviderPricingHandler_TopLevelEnvelopeIsThirdPartyContract(t *testing.T) {
	status, body := performProviderPricing(t, "")
	require.Equal(t, http.StatusOK, status)

	var envelope map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &envelope))

	assert.Equal(t, []string{"data", "message", "schema_version", "success"}, sortedKeys(envelope),
		"顶层键集合与旧实现逐字一致，且必须没有 code 信封键")
	assert.Equal(t, true, envelope["success"])
	assert.Equal(t, "", envelope["message"])
	assert.Equal(t, "1.0", envelope["schema_version"])

	data, ok := envelope["data"].(map[string]any)
	require.True(t, ok, "data 必须是对象")
	assert.Equal(t, []string{"currency", "models", "price_unit", "site_domain", "site_name", "updated_at"},
		sortedKeys(data))

	assert.Equal(t, "chenshuapi.com", data["site_domain"], "站点域名已切换为正式域名")
	assert.Equal(t, "CNY", data["currency"])
	assert.Equal(t, "per_1m_tokens", data["price_unit"])

	models, ok := data["models"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, models, "价格列表不能为空，第三方靠它下单")

	// cache_create_price / cache_create_price_1h 必须在每个模型条目里出现，
	// 缺值时以 null 呈现——旧测试正是这么断言第三方契约的。
	for i, raw := range models {
		model, ok := raw.(map[string]any)
		require.True(t, ok, "第 %d 个模型条目必须是对象", i)
		assert.Contains(t, model, "cache_create_price", "第 %d 个模型缺少 cache_create_price 键", i)
		assert.Contains(t, model, "cache_create_price_1h", "第 %d 个模型缺少 cache_create_price_1h 键", i)
		assert.Contains(t, model, "model_name")
		assert.Contains(t, model, "group_name")
	}
}

// 旧实现用 json.Encoder 输出，体尾带一个换行；做过字符串级比对的第三方会依赖它。
func TestProviderPricingHandler_BodyKeepsTrailingNewline(t *testing.T) {
	_, body := performProviderPricing(t, "")
	assert.True(t, strings.HasSuffix(body, "\n"), "响应体必须以换行结尾")

	trimmed := strings.TrimSuffix(body, "\n")
	assert.True(t, json.Valid([]byte(trimmed)))
	assert.False(t, strings.Contains(trimmed, "\n"), "紧凑 JSON 不应有内部换行")
}

// 默认（未配置外部路径）必须走编译进二进制的数据。
//
// 这条用例守的是一个真实部署事故：正式镜像是多阶段构建，源码树里的数据文件
// 不会进运行层，如果默认依赖文件，容器里这个接口会永远 503。
func TestProviderPricingService_DefaultsToEmbeddedData(t *testing.T) {
	doc, err := service.NewProviderPricingService("").Load()
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, "1.0", doc.SchemaVersion)
	assert.Equal(t, "chenshuapi.com", doc.SiteDomain)
	assert.NotEmpty(t, doc.Models)
}

// 配了外部路径但文件不可用时，回落到内置数据而不是把 503 抛给第三方：
// 一处配置失误不该让外部集成整体失效。
func TestProviderPricingService_BrokenOverrideFallsBackToEmbedded(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "not-there.json")
	doc, err := service.NewProviderPricingService(missing).Load()
	require.NoError(t, err, "文件缺失时应回落到内置数据")
	assert.Equal(t, "chenshuapi.com", doc.SiteDomain)

	garbage := filepath.Join(t.TempDir(), "garbage.json")
	require.NoError(t, os.WriteFile(garbage, []byte("{ this is not json"), 0o600))
	doc, err = service.NewProviderPricingService(garbage).Load()
	require.NoError(t, err, "文件格式非法时应回落到内置数据")
	assert.Equal(t, "chenshuapi.com", doc.SiteDomain)
}

// 外部文件合法时必须优先采用它，这样运维改价无需重新构建镜像。
func TestProviderPricingService_ValidOverrideWins(t *testing.T) {
	override := filepath.Join(t.TempDir(), "override.json")
	require.NoError(t, os.WriteFile(override, []byte(`{
		"schema_version": "1.0",
		"currency": "CNY",
		"price_unit": "per_1m_tokens",
		"site_name": "覆盖站点",
		"site_domain": "override.example",
		"models": [{"model_name": "only-one", "group_name": "g", "enabled": true}]
	}`), 0o600))

	doc, err := service.NewProviderPricingService(override).Load()
	require.NoError(t, err)
	assert.Equal(t, "override.example", doc.SiteDomain)
	require.Len(t, doc.Models, 1)
	assert.Equal(t, "only-one", doc.Models[0].ModelName)
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
