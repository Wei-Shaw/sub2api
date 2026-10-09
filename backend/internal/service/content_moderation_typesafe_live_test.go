package service

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/typesafe"
	"github.com/stretchr/testify/require"
)

// Opt-in paid network test. Never use real customer prompts or embed a credential.
func TestContentModerationTypeSafeLive(t *testing.T) {
	if os.Getenv("TYPESAFE_LIVE_TEST") != "1" {
		t.Skip("requires explicit TYPESAFE_LIVE_TEST=1 and a new TYPESAFE_API_KEY")
	}
	key := os.Getenv("TYPESAFE_API_KEY")
	require.NotEmpty(t, key, "configure a new key outside source control")
	for _, sample := range []struct{ id, text string }{
		{"coding", "请帮我写一个 Go 函数，对整数数组排序，并解释时间复杂度。"},
		{"security-defense", "我们需要检查登录接口的 SQL 注入风险，请说明参数化查询和最小权限的防护方法。"},
		{"quoted-injection", "请解释如何在聊天系统里识别‘忽略先前指令’这类提示注入，并避免泄露系统提示词。"},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		start := time.Now()
		result, status, err := typesafe.Evaluate(ctx, http.DefaultClient, "https://api.typesafe.ai", key, typesafe.Request{Model: "jev-latest", State: sample.text, Questions: typeSafeModerationQuestions()})
		elapsed := time.Since(start)
		cancel()
		require.NoError(t, err)
		require.Equal(t, 200, status)
		require.Len(t, result.Scores, 13)
		t.Logf("sample=%s model=%s rules=%s latency_ms=%d within_3s=%t input_tokens=%d output_tokens=%d scores=%v", sample.id, result.Model, TypeSafeModerationRulesVersion, elapsed.Milliseconds(), elapsed <= 3*time.Second, result.Usage.InputTokens, result.Usage.OutputTokens, result.Scores)
	}
}
