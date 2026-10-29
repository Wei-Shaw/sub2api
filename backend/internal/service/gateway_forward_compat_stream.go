package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// anthropicCompatDisconnectDrainInterval 是 gateway.stream_data_interval_timeout
// 未启用时，客户端断开后继续读取上游的读间隔上限，与该配置项的默认值一致。
var anthropicCompatDisconnectDrainInterval = 180 * time.Second

// anthropicCompatStream 是 Anthropic 上游经 chat/responses 兼容入口流式转发时的
// 上游读取状态。上游请求 ctx 与客户端解绑，客户端断开后调用方停止向下游写入、
// 继续读取上游以合并最终 usage。逐行读间隔受 gateway.stream_data_interval_timeout
// 限制；该配置未启用时，客户端断开后以 anthropicCompatDisconnectDrainInterval 限制。
type anthropicCompatStream struct {
	pump        *anthropicNativeLinePump
	body        io.Closer
	maxLineSize int
	logPrefix   string
	requestID   string
	// clientGone 是请求 ctx 的 Done，客户端断开后为 nil。
	clientGone <-chan struct{}

	clientDisconnected bool
	sawMessageStart    bool
	// sawTerminal 表示上游已发出 message_stop、带 stop_reason 的 message_delta 或 [DONE]。
	sawTerminal bool
	// upstreamErr* 记录上游流内的 error 事件；收到后调用方结束读取。
	upstreamErrType    string
	upstreamErrMessage string
	upstreamErrPayload string
	// readErr 是上游流结束的原因（io.EOF、读错误或 errAnthropicNativeStreamIdle）。
	readErr error
}

// anthropicCompatStreamFailure 描述客户端在线时上游流未正常结束的原因：
// err 返回给调用方，errType / message 写入下游的终止错误事件。
type anthropicCompatStreamFailure struct {
	err     error
	errType string
	message string
}

// newAnthropicCompatStream 启动上游行泵；调用方 defer stop()。
func newAnthropicCompatStream(cfg *config.Config, c *gin.Context, resp *http.Response, logPrefix, requestID string) *anthropicCompatStream {
	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if cfg != nil && cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)
	stream := &anthropicCompatStream{
		pump:        newAnthropicNativeLinePump(scanner, anthropicNativeStreamInterval(cfg)),
		body:        resp.Body,
		maxLineSize: maxLineSize,
		logPrefix:   logPrefix,
		requestID:   requestID,
	}
	if c.Request != nil {
		stream.clientGone = c.Request.Context().Done()
	}
	return stream
}

func (st *anthropicCompatStream) stop() {
	st.pump.stop()
}

// markClientDisconnected 标记客户端已断开，此后调用方不再向下游写入；读间隔上限
// 未启用时以 anthropicCompatDisconnectDrainInterval 启用。
func (st *anthropicCompatStream) markClientDisconnected() {
	if st.clientDisconnected {
		return
	}
	st.clientDisconnected = true
	st.clientGone = nil
	st.pump.enableInterval(anthropicCompatDisconnectDrainInterval)
	logger.L().Info(st.logPrefix+" stream: client disconnected, continuing to drain upstream for billing",
		zap.String("request_id", st.requestID),
	)
}

// nextLine 返回上游下一行；ok=false 表示上游流已结束，原因记入 readErr。
// 读间隔超时时关闭 body，解除泵 goroutine 阻塞中的读并释放上游连接。
// 上游流结束时请求 ctx 已取消的，按客户端已断开收尾。
func (st *anthropicCompatStream) nextLine() (string, bool) {
	for {
		line, err := st.pump.nextOrClientGone(st.clientGone)
		if err == nil {
			if payload, ok := extractOpenAISSEDataLine(line); ok && strings.TrimSpace(payload) == "[DONE]" {
				st.sawTerminal = true
			}
			return line, true
		}
		if errors.Is(err, errAnthropicNativeClientGone) {
			st.markClientDisconnected()
			continue
		}
		if st.clientGone != nil {
			select {
			case <-st.clientGone:
				st.clientDisconnected = true
				st.clientGone = nil
			default:
			}
		}
		st.readErr = err
		switch {
		case errors.Is(err, errAnthropicNativeStreamIdle):
			_ = st.body.Close()
			logger.L().Warn(st.logPrefix+" stream: data interval timeout",
				zap.String("request_id", st.requestID),
				zap.Duration("interval", st.pump.interval),
				zap.Bool("client_disconnected", st.clientDisconnected),
			)
		case errors.Is(err, io.EOF), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			// 正常收流或取消：不记日志。
		default:
			logger.L().Warn(st.logPrefix+" stream: read error",
				zap.Error(err),
				zap.String("request_id", st.requestID),
			)
		}
		return "", false
	}
}

// observe 记录决定收尾方式的上游事件。收到上游 error 事件时关闭上游 body，
// 调用方不再处理该事件并结束读取。
func (st *anthropicCompatStream) observe(event *apicompat.AnthropicStreamEvent, payload string) {
	switch event.Type {
	case "message_start":
		st.sawMessageStart = true
	case "message_stop":
		st.sawTerminal = true
	case "message_delta":
		if event.Delta != nil && event.Delta.StopReason != "" {
			st.sawTerminal = true
		}
	case "error":
		st.upstreamErrPayload = payload
		st.upstreamErrType = firstNonEmpty(gjson.Get(payload, "error.type").String(), "upstream_error")
		st.upstreamErrMessage = firstNonEmpty(sanitizeUpstreamErrorMessage(gjson.Get(payload, "error.message").String()), "upstream stream error")
		_ = st.body.Close()
		logger.L().Warn(st.logPrefix+" stream: upstream error event",
			zap.String("request_id", st.requestID),
			zap.String("error_type", st.upstreamErrType),
			zap.String("error_message", st.upstreamErrMessage),
			zap.Bool("client_disconnected", st.clientDisconnected),
		)
	}
}

// failure 返回客户端在线、上游未发出结束事件即中断时的失败原因：上游流内
// error 事件、读间隔超时、读错误，或 message_start 之后正常关闭。此时调用方不补发
// 完成事件，而是写出协议内的终止错误。上游已发出结束事件、客户端已断开，或上游
// 未发出 message_start 即正常关闭时返回 nil，调用方按已累计的 usage 正常收尾。
// 读间隔超时时 err 为 errAnthropicNativeStreamIdle。
func (st *anthropicCompatStream) failure() *anthropicCompatStreamFailure {
	if st.clientDisconnected || st.sawTerminal {
		return nil
	}
	switch {
	case st.upstreamErrType != "":
		return &anthropicCompatStreamFailure{
			err:     &sseStreamErrorEventError{RawData: st.upstreamErrPayload},
			errType: st.upstreamErrType,
			message: st.upstreamErrMessage,
		}
	case errors.Is(st.readErr, errAnthropicNativeStreamIdle):
		return &anthropicCompatStreamFailure{
			err:     errAnthropicNativeStreamIdle,
			errType: "stream_timeout",
			message: fmt.Sprintf("upstream stream idle for %s", st.pump.interval),
		}
	case errors.Is(st.readErr, bufio.ErrTooLong):
		return &anthropicCompatStreamFailure{
			err:     fmt.Errorf("stream usage incomplete: %w", st.readErr),
			errType: "response_too_large",
			message: fmt.Sprintf("upstream SSE line exceeded %d bytes", st.maxLineSize),
		}
	case st.readErr != nil && !errors.Is(st.readErr, io.EOF):
		return &anthropicCompatStreamFailure{
			err:     fmt.Errorf("stream usage incomplete: %w", st.readErr),
			errType: "stream_read_error",
			message: "upstream stream disconnected: " + sanitizeStreamError(st.readErr),
		}
	case st.sawMessageStart:
		return &anthropicCompatStreamFailure{
			err:     errors.New("stream usage incomplete: missing terminal event"),
			errType: "stream_incomplete",
			message: "upstream stream ended without a terminal event",
		}
	default:
		return nil
	}
}

// finishAnthropicCompatStreamFailure 按失败原因收尾，返回交给调用方的结果与错误。
// 上游流内 error 事件与 /v1/messages 一致：不计费，返回 sseStreamErrorEventError
// 由调用方转为 UpstreamFailoverError；尚未向客户端写出内容时不写终止错误，由
// handler 换号重试。其余失败写出终止错误，并带上已观测到的 usage；未观测到任何
// usage 时结果为 nil。
func finishAnthropicCompatStreamFailure(c *gin.Context, writerSizeBeforeStream int, failure *anthropicCompatStreamFailure, result *ForwardResult, writeFailure func()) (*ForwardResult, error) {
	var sseErr *sseStreamErrorEventError
	if errors.As(failure.err, &sseErr) {
		if c.Writer.Size() != writerSizeBeforeStream {
			writeFailure()
		}
		return nil, failure.err
	}
	writeFailure()
	if !result.Usage.hasObservedTokens() {
		return nil, failure.err
	}
	return result, failure.err
}

// writeAnthropicCompatCCStreamFailure 向 Chat Completions 流写出终止错误帧，
// 并标记响应已提交，handler 不再追加兜底错误。
func writeAnthropicCompatCCStreamFailure(c *gin.Context, failure *anthropicCompatStreamFailure) {
	payload, _ := json.Marshal(map[string]any{
		"error": map[string]string{
			"type":    failure.errType,
			"message": failure.message,
		},
	})
	commitAnthropicCompatStreamFailure(c, failure, "data: "+string(payload)+"\n\n")
}

// writeAnthropicCompatResponsesStreamFailure 向 Responses 流写出 response.failed
// 终止事件，沿用流内的 response id；sequence_number 经 restorer 与已写出的事件
// 保持连续。写出后标记响应已提交。
func writeAnthropicCompatResponsesStreamFailure(c *gin.Context, state *apicompat.AnthropicEventToResponsesState, restorer *apicompat.ResponsesClientToolStreamRestorer, failure *anthropicCompatStreamFailure) {
	responseID := state.ResponseID
	if responseID == "" {
		responseID = "resp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	evt := apicompat.ResponsesStreamEvent{
		Type:           "response.failed",
		SequenceNumber: state.SequenceNumber,
		Response: &apicompat.ResponsesResponse{
			ID:        responseID,
			Object:    "response",
			CreatedAt: state.Created,
			Model:     state.Model,
			Status:    "failed",
			Output:    []apicompat.ResponsesOutput{},
			Error:     &apicompat.ResponsesError{Code: failure.errType, Message: failure.message},
		},
	}
	state.SequenceNumber++
	payload, err := json.Marshal(evt)
	if err != nil {
		return
	}
	payloads, _, err := restorer.RestoreEvent(payload)
	if err != nil {
		return
	}
	var frames strings.Builder
	for _, restored := range payloads {
		fmt.Fprintf(&frames, "event: %s\ndata: %s\n\n", gjson.GetBytes(restored, "type").String(), restored)
	}
	commitAnthropicCompatStreamFailure(c, failure, frames.String())
}

func commitAnthropicCompatStreamFailure(c *gin.Context, failure *anthropicCompatStreamFailure, frame string) {
	MarkOpsStreamError(c, failure.errType, failure.message, http.StatusBadGateway)
	MarkResponseCommitted(c)
	if _, err := io.WriteString(c.Writer, frame); err != nil {
		return
	}
	c.Writer.Flush()
}
