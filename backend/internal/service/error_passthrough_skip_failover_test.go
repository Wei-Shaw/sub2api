//go:build unit

package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// bindSkipFailoverRule 构造一条只按关键词匹配的 openai 规则并绑定到请求上下文。
func bindSkipFailoverRule(c *gin.Context, keyword string, responseCode int, skipFailover bool) {
	svc := &ErrorPassthroughService{}
	code := responseCode
	svc.localCacheMu.Lock()
	svc.localCache = []*cachedPassthroughRule{{
		ErrorPassthroughRule: &model.ErrorPassthroughRule{
			ID:              1,
			Enabled:         true,
			Platforms:       []string{"openai"},
			MatchMode:       model.MatchModeAny,
			Keywords:        []string{keyword},
			ResponseCode:    &code,
			PassthroughBody: true,
			SkipFailover:    skipFailover,
		},
		lowerKeywords:  []string{strings.ToLower(keyword)},
		lowerPlatforms: []string{"openai"},
	}}
	svc.localCacheMu.Unlock()
	BindErrorPassthroughService(c, svc)
}

func newSkipFailoverTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, rec
}

func TestRuleSkipsFailoverTrueWhenRuleSet(t *testing.T) {
	c, _ := newSkipFailoverTestContext(t)
	bindSkipFailoverRule(c, "overloaded", 529, true)

	require.True(t, ruleSkipsFailover(c, "openai", http.StatusServiceUnavailable,
		[]byte(`{"error":{"message":"server overloaded now"}}`)))
}

func TestRuleSkipsFailoverFalseWhenRuleAllowsRetry(t *testing.T) {
	c, _ := newSkipFailoverTestContext(t)
	bindSkipFailoverRule(c, "overloaded", 529, false)

	require.False(t, ruleSkipsFailover(c, "openai", http.StatusServiceUnavailable,
		[]byte(`{"error":{"message":"server overloaded now"}}`)))
}

func TestRuleSkipsFailoverFalseWhenUnmatched(t *testing.T) {
	c, _ := newSkipFailoverTestContext(t)
	bindSkipFailoverRule(c, "overloaded", 529, true)

	require.False(t, ruleSkipsFailover(c, "openai", http.StatusServiceUnavailable,
		[]byte(`{"error":{"message":"unrelated failure"}}`)))
}

// 平台不匹配时规则不应生效，避免一个平台的规则拦下另一个平台的换号。
func TestRuleSkipsFailoverFalseWhenPlatformMismatch(t *testing.T) {
	c, _ := newSkipFailoverTestContext(t)
	bindSkipFailoverRule(c, "overloaded", 529, true)

	require.False(t, ruleSkipsFailover(c, "anthropic", http.StatusServiceUnavailable,
		[]byte(`{"error":{"message":"server overloaded now"}}`)))
}

func TestRuleSkipsFailoverFalseWhenNoServiceBound(t *testing.T) {
	c, _ := newSkipFailoverTestContext(t)

	require.False(t, ruleSkipsFailover(c, "openai", http.StatusServiceUnavailable, []byte(`{}`)))
}

// SSE 专用包装必须与规则匹配同口径：语义状态推断 + body 归一化。
func TestOpenAIStreamRuleSkipsFailoverUsesSemanticStatus(t *testing.T) {
	c, _ := newSkipFailoverTestContext(t)
	bindSkipFailoverRule(c, "server_is_overloaded", 529, true)

	payload := []byte(`{"type":"response.failed","response":{"status":"failed","error":{"code":"server_is_overloaded","message":"overloaded"}}}`)
	require.True(t, openAIStreamRuleSkipsFailover(c, "openai", payload, "overloaded"))
}

// 混合调度下 GeminiMessagesCompatService 可能选中 antigravity 平台的账号
// （见 isAccountValidForPlatform）。规则匹配必须用账号的真实平台，否则按
// 入口平台匹配会让配在 antigravity 上的规则静默失效。
func TestRuleSkipsFailoverMatchesAccountPlatformNotEntryPlatform(t *testing.T) {
	c, _ := newSkipFailoverTestContext(t)
	svc := &ErrorPassthroughService{}
	code := 529
	svc.localCacheMu.Lock()
	svc.localCache = []*cachedPassthroughRule{{
		ErrorPassthroughRule: &model.ErrorPassthroughRule{
			ID:           1,
			Enabled:      true,
			Platforms:    []string{model.PlatformAntigravity},
			MatchMode:    model.MatchModeAny,
			Keywords:     []string{"overloaded"},
			ResponseCode: &code,
			SkipFailover: true,
		},
		lowerKeywords:  []string{"overloaded"},
		lowerPlatforms: []string{model.PlatformAntigravity},
	}}
	svc.localCacheMu.Unlock()
	BindErrorPassthroughService(c, svc)

	body := []byte(`{"error":{"message":"server overloaded now"}}`)
	require.True(t, ruleSkipsFailover(c, PlatformAntigravity, http.StatusServiceUnavailable, body),
		"账号真实平台应命中规则")
	require.False(t, ruleSkipsFailover(c, PlatformGemini, http.StatusServiceUnavailable, body),
		"按入口平台匹配会漏掉该规则")
}
