package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// intelligenceCheckProbeMaxCaptureBytes 限制单次跑测在内存里捕获的 SSE 原始字节数。
// 跑测结果会被完整读进内存，若上游异常返回超大响应而没有任何上限，
// 会直接把进程内存打爆；作品本身另有落库大小上限（见 IntelligenceCheckService）。
const intelligenceCheckProbeMaxCaptureBytes = 8 << 20 // 8 MiB

// IntelligenceCheckProbeResult 是一次后台智力检测跑测的原始结果。
type IntelligenceCheckProbeResult struct {
	Status        string
	ResponseText  string
	UpstreamModel string
	ErrorMessage  string
	LatencyMs     int64
	StartedAt     time.Time
	FinishedAt    time.Time
	// Truncated 表示捕获内容触及上限被截断，此时作品不应判定为通过。
	Truncated bool
	// UpstreamResponded 表示本次跑测至少收到过一次上游 HTTP 响应。
	// 它是「这次跑测是否真的在别人家花了钱」的判据：没收到响应就不写使用记录。
	UpstreamResponded bool
	// UpstreamRequestIDs 是本次跑测所有上游响应头里读到的请求标识（去重、保序）。
	// 后台 A6 取数任务拿它按 ID 反查真实扣费；账号没配 upstream_request_id_header 时为空。
	UpstreamRequestIDs []string
	// FirstTokenMs 是流式跑测的首个内容块到达耗时（首字）。非流式、或流里没内容时为 nil。
	FirstTokenMs *int
	// Usage 是从上游响应里解析出的 token 用量（输入/输出/缓存）。
	Usage ProbeUsageTokens
}

// limitedResponseRecorder 是带字节上限的响应记录器。
// 超过上限后继续"接受"写入但不再累积内容，避免上游异常响应撑爆内存。
type limitedResponseRecorder struct {
	*httptest.ResponseRecorder
	limit     int
	truncated bool
}

func newLimitedResponseRecorder(limit int) *limitedResponseRecorder {
	return &limitedResponseRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		limit:            limit,
	}
}

func (w *limitedResponseRecorder) Write(p []byte) (int, error) {
	if w.limit <= 0 {
		return w.ResponseRecorder.Write(p)
	}
	remaining := w.limit - w.Body.Len()
	if remaining <= 0 {
		w.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		w.truncated = true
		if _, err := w.ResponseRecorder.Write(p[:remaining]); err != nil {
			return 0, err
		}
		return len(p), nil
	}
	return w.ResponseRecorder.Write(p)
}

// RunIntelligenceCheckProbe 在内存中执行一次智力检测跑测（不产生真实 HTTP 响应），
// 通过 httptest 记录 SSE 输出后解析。
//
// 与 RunTestBackground（官方定时连通性测试）的差异只有两点：
//   - 题面、输出上限与思考强度由参数注入（官方路径仍固定用 "hi" 与 1024）；
//   - 题面同时作为 prompt 参数下传，使 Chat Completions 分支直接生效。
//
// 平台分流、凭据、代理、错误处理与响应解析全部复用官方实现；
// 唯一的差异是流式与否由调用方决定，见下方覆盖值。
func (s *AccountTestService) RunIntelligenceCheckProbe(
	ctx context.Context,
	accountID int64,
	modelID string,
	prompt string,
	maxTokens int,
	reasoningEffort string,
	disableStream bool,
) (*IntelligenceCheckProbeResult, error) {
	startedAt := time.Now()

	recorder := newLimitedResponseRecorder(intelligenceCheckProbeMaxCaptureBytes)
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = (&http.Request{}).WithContext(ctx)
	withIntelligenceCheckOverride(ginCtx, intelligenceCheckOverride{
		Prompt:          prompt,
		MaxTokens:       maxTokens,
		ReasoningEffort: reasoningEffort,
		// 默认流式（disableStream=false）。非流式虽然能一次拿到完整响应，
		// 但上游网关按「整体响应」计时，长思考时更容易撞上网关超时。
		DisableStream: disableStream,
	})

	// 装上上游响应捕获器：平台测试拿到响应时会记下上游请求 ID（供 A6 反查成本）。
	// 官方连通性测试没有这个捕获器，相关钩子全部退化为空操作。
	capture := &intelligenceCheckUpstreamCapture{}
	withIntelligenceCheckUpstreamCapture(ginCtx, capture)

	testErr := s.TestAccountConnection(ginCtx, accountID, modelID, prompt, AccountTestModeDefault)

	finishedAt := time.Now()
	responseText, errMsg, upstreamModel := parseIntelligenceCheckSSEOutput(recorder.Body.String())

	status := IntelligenceCheckStatusCompleted
	if testErr != nil || errMsg != "" {
		status = IntelligenceCheckStatusFailed
		if errMsg == "" && testErr != nil {
			errMsg = testErr.Error()
		}
	}

	upstreamResponded, upstreamRequestIDs := capture.snapshot()
	// 首字只对流式跑测有意义：非流式下「第一个 content 事件」其实是响应结束时刻，
	// 拿它当首字会把总耗时冒充成首字。
	var firstTokenMs *int
	if !disableStream {
		if at := capture.firstContentTimestamp(); !at.IsZero() {
			ms := int(at.Sub(startedAt) / time.Millisecond)
			firstTokenMs = &ms
		}
	}
	usage := capture.usageSnapshot()

	return &IntelligenceCheckProbeResult{
		Status:             status,
		ResponseText:       responseText,
		UpstreamModel:      upstreamModel,
		ErrorMessage:       errMsg,
		LatencyMs:          finishedAt.Sub(startedAt).Milliseconds(),
		StartedAt:          startedAt,
		FinishedAt:         finishedAt,
		Truncated:          recorder.truncated,
		UpstreamResponded:  upstreamResponded,
		UpstreamRequestIDs: upstreamRequestIDs,
		FirstTokenMs:       firstTokenMs,
		Usage:              usage,
	}, nil
}

// parseIntelligenceCheckSSEOutput 从捕获的 SSE 输出里取出正文、错误与最后出现的模型名。
// 与 parseTestSSEOutput 的区别是多提取 upstream model，供「实际跑的哪个模型」展示。
func parseIntelligenceCheckSSEOutput(body string) (responseText, errMsg, upstreamModel string) {
	var texts []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		jsonStr := strings.TrimPrefix(line, "data: ")
		var event TestEvent
		if err := json.Unmarshal([]byte(jsonStr), &event); err != nil {
			continue
		}
		if model := strings.TrimSpace(event.Model); model != "" {
			upstreamModel = model
		}
		switch event.Type {
		case "content":
			if event.Text != "" {
				texts = append(texts, event.Text)
			}
		case "error":
			errMsg = event.Error
		}
	}
	responseText = strings.Join(texts, "")
	return
}
