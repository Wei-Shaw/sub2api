package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Candy prompt adapted from haowang02/cpa-plugin-codex-candy-eval (MIT).
// See docs/ACCOUNT_QUALITY_LICENSES.md. The local final-answer parser is stricter.
const accountQualityCandyPrompt = "直接回答问题，不允许使用联网搜索、调用命令、历史记忆、代码文件：\r\n\r\n在一个黑色的袋子里放有三种口味的糖果，每种糖果有两种不同的形状（圆形和五角星形，不同的形状靠手感可以分辨）。现已知不同口味的糖和不同形状的数量统计如下表。参赛者需要在活动前决定摸出的糖果数目，那么，最少取出多少个糖果才能保证手中同时拥有不同形状的苹果味和桃子味的糖？（同时手中有圆形苹果味匹配五角星桃子味糖果，或者有圆形桃子味匹配五角星苹果味糖果都满足要求）\r\n\r\n        苹果味  桃子味  西瓜味\r\n圆形       7      9      8\r\n五角星形   7      6      4\r\n"

type qualityProbeContextKey struct{}
type qualityProbeSpec struct{ Prompt, Effort, Tier, Endpoint string }

func applyAccountQualityProbePayload(ctx context.Context, payload map[string]any) {
	spec, ok := ctx.Value(qualityProbeContextKey{}).(qualityProbeSpec)
	if !ok {
		return
	}
	payload["input"] = []map[string]any{{"role": "user", "content": []map[string]any{{"type": "input_text", "text": spec.Prompt}}}}
	if spec.Effort != "unspecified" {
		payload["reasoning"] = map[string]any{"effort": spec.Effort}
	}
	if spec.Tier != "" {
		payload["service_tier"] = spec.Tier
	}
}
func isAccountQualityProbe(ctx context.Context) bool {
	return ctx.Value(qualityProbeContextKey{}) != nil
}

var qualityAnswerPattern = regexp.MustCompile(`(?s)^\s*(?:<answer>\s*([0-9]{1,9})\s*</answer>|([0-9]{1,9}))\s*$`)

func judgeAccountQualityAnswer(answer, expected string) string {
	m := qualityAnswerPattern.FindStringSubmatch(answer)
	if m == nil {
		return "unparseable"
	}
	if m[1]+m[2] == expected {
		return "passed"
	}
	return "incorrect"
}
func (s *AccountTestService) RunQualityProbe(ctx context.Context, id int64, p AccountQualityPolicy, prompt string) (string, string, int64) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil || account == nil {
		return "", "account_unavailable", 0
	}
	if qualityMediaModel(p.Model) || qualityMediaModel(account.GetMappedModel(p.Model)) {
		return "", "text_benchmark_requires_text_model", 0
	}
	if !qualityAccountSupported(account.Platform, account.Type, p.Endpoint) || account.IsSyntheticUITest() {
		return "", "unsupported_account", 0
	}
	if !account.IsSchedulable() {
		return "", "account_not_schedulable", 0
	}
	if account.Type == AccountTypeAPIKey && account.Platform != PlatformOpenAI && account.Platform != PlatformAnthropic && account.Platform != PlatformGemini && strings.TrimSpace(account.GetCredential("base_url")) == "" {
		return "", "explicit_upstream_url_required", 0
	}
	ctx = context.WithValue(ctx, qualityProbeContextKey{}, qualityProbeSpec{Prompt: prompt, Effort: p.Effort, Tier: p.ServiceTier, Endpoint: p.Endpoint})
	rec := httptest.NewRecorder()
	writer := &qualityBoundedWriter{ResponseRecorder: rec, cancel: cancel}
	c, _ := gin.CreateTestContext(writer)
	c.Request = httptest.NewRequest(http.MethodPost, "/internal/account-quality", nil).WithContext(ctx)
	c.Set(accountTestBackgroundKey, true)
	if p.Endpoint == "chat_completions" {
		rawBase := strings.TrimSpace(account.GetCredential("base_url"))
		if rawBase == "" {
			if account.Platform != PlatformOpenAI {
				return "", "explicit_upstream_url_required", 0
			}
			rawBase = "https://api.openai.com"
		}
		base, baseErr := s.validateUpstreamBaseURL(rawBase)
		if baseErr != nil {
			return "", "invalid_upstream_url", 0
		}
		key := account.GetOpenAIProtocolAPIKey()
		if key == "" {
			key = account.GetCredential("api_key")
		}
		if key == "" {
			return "", "missing_api_key", 0
		}
		err = s.testOpenAIChatCompletionsConnection(c, account, account.GetMappedModel(p.Model), prompt, base, key)
	} else if p.Endpoint == "responses" {
		if account.Platform != PlatformOpenAI && strings.TrimSpace(account.GetCredential("base_url")) == "" {
			return "", "explicit_upstream_url_required", 0
		}
		err = s.testOpenAIAccountConnection(c, account, p.Model, prompt, AccountTestModeDefault)
	} else if p.Endpoint == "messages" {
		err = s.testClaudeAccountConnection(c, account, p.Model)
	} else if p.Endpoint == "generate_content" {
		if p.Effort != "unspecified" || p.ServiceTier != "" {
			return "", "use_unspecified_effort_for_native_gemini", 0
		}
		err = s.testGeminiAccountConnection(c, account, p.Model, prompt)
	} else {
		return "", "unsupported_endpoint", 0
	}
	elapsed := time.Since(start).Milliseconds()
	answer, message := parseTestSSEOutput(rec.Body.String())
	if err != nil || message != "" {
		return "", qualityProbeFailure(ctx, err, message), elapsed
	}
	completed := false
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if strings.HasPrefix(line, "data: ") {
			var ev TestEvent
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev) == nil && ev.Type == "test_complete" && ev.Success {
				completed = true
			}
		}
	}
	if !completed || ctx.Err() != nil || writer.overflow {
		return "", "incomplete_response", elapsed
	}
	if len(answer) > 32768 {
		return "", "response_too_large", elapsed
	}
	return answer, "", elapsed
}

type qualityBoundedWriter struct {
	*httptest.ResponseRecorder
	cancel   context.CancelFunc
	overflow bool
}

func (w *qualityBoundedWriter) Write(p []byte) (int, error) {
	if w.Body.Len()+len(p) > 131072 {
		w.overflow = true
		w.cancel()
		return 0, fmt.Errorf("probe response limit exceeded")
	}
	return w.ResponseRecorder.Write(p)
}

func (w *qualityBoundedWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

func qualityAccountSupported(platform, kind, endpoint string) bool {
	switch endpoint {
	case "responses":
		return platform == PlatformOpenAI || kind == AccountTypeAPIKey
	case "chat_completions":
		return kind == AccountTypeAPIKey
	case "messages":
		return platform == PlatformAnthropic || kind == AccountTypeAPIKey
	case "generate_content":
		return platform == PlatformGemini
	}
	return false
}
func applyAccountQualityNativePayload(ctx context.Context, payload map[string]any) {
	spec, ok := ctx.Value(qualityProbeContextKey{}).(qualityProbeSpec)
	if !ok || (spec.Endpoint != "messages" && spec.Endpoint != "chat_completions") {
		return
	}
	payload["messages"] = []map[string]any{{"role": "user", "content": spec.Prompt}}
	if spec.Endpoint == "messages" {
		payload["max_tokens"] = 8192
		if spec.Effort != "unspecified" {
			payload["output_config"] = map[string]any{"effort": spec.Effort}
		}
	} else if spec.Effort != "unspecified" {
		payload["reasoning_effort"] = spec.Effort
	}
	if spec.Tier != "" {
		payload["service_tier"] = spec.Tier
	}
}

func qualityMediaModel(model string) bool {
	m := strings.ToLower(model)
	return isOpenAIImageModel(model) || isImageGenerationModel(model) || strings.Contains(m, "imagine") || strings.HasPrefix(m, "sora") || strings.Contains(m, "video") || strings.HasPrefix(m, "tts-") || strings.HasPrefix(m, "whisper")
}

var qualityProbeStatusPattern = regexp.MustCompile(`(?i)(?:API returned|returned|status code|HTTP status|failed\s*\()\s*(\d{3})`)

func qualityProbeFailure(ctx context.Context, err error, message string) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	combined := message
	if err != nil {
		combined += " " + err.Error()
	}
	if match := qualityProbeStatusPattern.FindStringSubmatch(combined); match != nil {
		code, _ := strconv.Atoi(match[1])
		switch code {
		case 401:
			return "authentication_failed"
		case 402:
			return "upstream_balance_exhausted"
		case 403:
			return "access_denied"
		case 408, 504:
			return "timeout"
		case 429:
			return "rate_limited"
		case 400, 404, 413, 422:
			return "request_rejected"
		}
		if code >= 500 {
			return "upstream_server_error"
		}
	}
	if strings.Contains(strings.ToLower(combined), "incomplete") || strings.Contains(strings.ToLower(combined), "before response.completed") {
		return "incomplete_response"
	}
	return "upstream_or_transport_error"
}
