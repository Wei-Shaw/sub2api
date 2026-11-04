package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ChatGPT Web 生图通道：把 /v1/images/generations 交给 chatgpt.com 网页链路执行，
// 再把网页产出的图片重新包装成 Responses 事件，复用 OAuth 分支既有的解析、落盘、
// 计费与错误分类逻辑——下游看到的行为与原生 OAuth 生图一致。

func chatGPTWebResolveImagesRequestModel(parsed *OpenAIImagesRequest, channelMappedModel string) string {
	requestModel := strings.TrimSpace(parsed.Model)
	if mapped := strings.TrimSpace(channelMappedModel); mapped != "" {
		requestModel = mapped
	}
	if requestModel == "" {
		requestModel = "gpt-image-2"
	}
	return requestModel
}

func chatGPTWebPromptWithCanvasSize(prompt, size string) string {
	trimmedSize := strings.ToLower(strings.TrimSpace(size))
	if trimmedSize == "" || trimmedSize == "auto" {
		return prompt
	}
	parts := strings.Split(trimmedSize, "x")
	if len(parts) != 2 {
		return prompt
	}
	width, widthErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, heightErr := strconv.Atoi(strings.TrimSpace(parts[1]))
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return prompt
	}

	orientation := "横向"
	switch {
	case height > width:
		orientation = "纵向"
	case height == width:
		orientation = "正方形"
	}
	resolution := "1K"
	if max(width, height) >= 3840 {
		resolution = "4K"
	} else if max(width, height) >= 2048 {
		resolution = "2K"
	}
	return fmt.Sprintf(
		"%s\n\n画布规格（必须遵循）：最终图像为 %d×%d 像素，%s画幅，%s 分辨率。不得裁切、拉伸或回退到默认正方形画布。",
		prompt,
		width,
		height,
		orientation,
		resolution,
	)
}

const (
	// 网页额度耗尽与短时速率限制在响应上都是 429，握手层面无法区分，
	// 因此冷却取一个折中值：短时限流恢复后仍会重新进入调度。
	chatGPTWebImagesQuotaCooldown = 10 * time.Minute
	chatGPTWebImagesQuotaReason   = "chatgpt_web_images_quota"
	chatGPTWebImagesAuthReason    = "chatgpt_web_images_auth"
)

// handleChatGPTWebImagesFailure 把网页链路的失败翻译成网关语义：
// 凭据失效/限额换号，限额额外写账号级冷却，Cloudflare 拦截页只报错（换号无意义）。
func (s *OpenAIGatewayService) handleChatGPTWebImagesFailure(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	err error,
) error {
	var upstreamErr *chatGPTWebUpstreamError
	if !errors.As(err, &upstreamErr) {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, http.StatusBadGateway, safeErr, "")
		return fmt.Errorf("chatgpt web image generation failed: %s", safeErr)
	}
	safeErr := sanitizeUpstreamErrorMessage(upstreamErr.Message)
	clientStatus := upstreamErr.clientStatusCode()
	setOpsUpstreamError(c, clientStatus, safeErr, "")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		ProxyID:            opsUpstreamProxyID(account),
		ProxyName:          opsUpstreamProxyName(account),
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: upstreamErr.StatusCode,
		UpstreamURL:        safeUpstreamURL(chatGPTWebBaseURL + "/backend-api/f/conversation"),
		Kind:               "failover",
		Message:            safeErr,
	})

	switch classifyChatGPTWebFailure(upstreamErr.StatusCode, upstreamErr.Challenge) {
	case chatGPTWebFailureQuota:
		s.coolChatGPTWebImagesAccount(ctx, account, chatGPTWebImagesQuotaReason)
		return s.newOpenAIAccountFailoverError(account, clientStatus, http.Header{}, upstreamErr.Body, safeErr, false, false)
	case chatGPTWebFailureAuth:
		s.coolChatGPTWebImagesAccount(ctx, account, chatGPTWebImagesAuthReason)
		return s.newOpenAIAccountFailoverError(account, clientStatus, http.Header{}, upstreamErr.Body, safeErr, false, false)
	case chatGPTWebFailureTransient:
		return s.newOpenAIAccountFailoverError(account, clientStatus, http.Header{}, upstreamErr.Body, safeErr, false, false)
	default:
		return fmt.Errorf("chatgpt web image generation failed: %s", safeErr)
	}
}

// coolChatGPTWebImagesAccount 给账号写模型级冷却，让后续生图请求换到别的账号。
func (s *OpenAIGatewayService) coolChatGPTWebImagesAccount(ctx context.Context, account *Account, reason string) {
	if s == nil || s.accountRepo == nil || account == nil {
		return
	}
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	resetAt := time.Now().Add(chatGPTWebImagesQuotaCooldown)
	if err := s.accountRepo.SetModelRateLimit(stateCtx, account.ID, openAIImageGenerationRateLimitKey, resetAt, reason); err != nil {
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] chatgpt-web images cooldown write failed account_id=%d error=%v", account.ID, err)
		return
	}
	logger.LegacyPrintf(
		"service.openai_gateway",
		"[OpenAI] chatgpt-web images account cooled account_id=%d reason=%s reset_in=%s",
		account.ID,
		reason,
		time.Until(resetAt).Truncate(time.Second),
	)
}

// buildChatGPTWebImagesCompletedEvent 合成 response.completed 事件，供现有解析器读取图片。
func buildChatGPTWebImagesCompletedEvent(model string, images [][]byte) []byte {
	output := make([]any, 0, len(images))
	for index, raw := range images {
		output = append(output, map[string]any{
			"id":     fmt.Sprintf("ig_chatgpt_web_%d", index),
			"type":   "image_generation_call",
			"status": "completed",
			"result": base64.StdEncoding.EncodeToString(raw),
		})
	}
	payload := map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id":         "resp_chatgpt_web_" + uuid.NewString(),
			"object":     "response",
			"model":      model,
			"created_at": time.Now().Unix(),
			"status":     "completed",
			"output":     output,
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return []byte(`{"type":"response.completed","response":{"output":[]}}`)
	}
	return encoded
}

// buildChatGPTWebImagesCompletedBody 把完成事件包成 SSE 体：既有解析器只消费 data: 行。
func buildChatGPTWebImagesCompletedBody(model string, images [][]byte) []byte {
	return []byte("event: response.completed\ndata: " + string(buildChatGPTWebImagesCompletedEvent(model, images)) + "\n\n")
}

func buildChatGPTWebImagesCompletedBodyFromResults(model string, results []string) []byte {
	output := make([]any, 0, len(results))
	for index, result := range results {
		output = append(output, map[string]any{
			"id":     fmt.Sprintf("ig_chatgpt_web_%d", index),
			"type":   "image_generation_call",
			"status": "completed",
			"result": result,
		})
	}
	payload := map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id":         "resp_chatgpt_web_" + uuid.NewString(),
			"object":     "response",
			"model":      model,
			"created_at": time.Now().Unix(),
			"status":     "completed",
			"output":     output,
		},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return []byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"output\":[]}}\n\n")
	}
	return []byte("event: response.completed\ndata: " + string(encoded) + "\n\n")
}

// buildChatGPTWebImagesStreamBody 合成 SSE 事件序列，事件顺序与真实上游一致。
func buildChatGPTWebImagesStreamBody(model string, images [][]byte) []byte {
	results := make([]string, 0, len(images))
	for _, raw := range images {
		results = append(results, base64.StdEncoding.EncodeToString(raw))
	}
	return buildChatGPTWebImagesStreamBodyFromResults(model, results)
}

func buildChatGPTWebImagesStreamBodyFromResults(model string, results []string) []byte {
	completed := buildChatGPTWebImagesCompletedBodyFromResults(model, results)
	completed = bytes.TrimPrefix(completed, []byte("event: response.completed\ndata: "))
	completed = bytes.TrimSuffix(completed, []byte("\n\n"))
	responseID := "resp_chatgpt_web_" + uuid.NewString()
	var builder strings.Builder
	created, _ := json.Marshal(map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":         responseID,
			"object":     "response",
			"model":      model,
			"created_at": time.Now().Unix(),
			"status":     "in_progress",
			"output":     []any{},
		},
	})
	_, _ = builder.WriteString("event: response.created\ndata: ")
	_, _ = builder.Write(created)
	_, _ = builder.WriteString("\n\n")
	for index, result := range results {
		item, _ := json.Marshal(map[string]any{
			"type":         "response.output_item.done",
			"output_index": index,
			"item": map[string]any{
				"id":     fmt.Sprintf("ig_chatgpt_web_%d", index),
				"type":   "image_generation_call",
				"status": "completed",
				"result": result,
			},
		})
		_, _ = builder.WriteString("event: response.output_item.done\ndata: ")
		_, _ = builder.Write(item)
		_, _ = builder.WriteString("\n\n")
	}
	_, _ = builder.WriteString("event: response.completed\ndata: ")
	_, _ = builder.Write(completed)
	_, _ = builder.WriteString("\n\n")
	return []byte(builder.String())
}

// forwardOpenAIImagesChatGPTWeb 走网页额度生图，并复用 OAuth 分支的下游处理。
func (s *OpenAIGatewayService) forwardOpenAIImagesChatGPTWeb(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	requestModel := chatGPTWebResolveImagesRequestModel(parsed, channelMappedModel)
	if err := validateOpenAIImagesModel(requestModel); err != nil {
		return nil, err
	}
	logger.LegacyPrintf(
		"service.openai_gateway",
		"[OpenAI] Images request routing (chatgpt-web) request_model=%s endpoint=%s account_id=%d uploads=%d",
		requestModel,
		parsed.Endpoint,
		account.ID,
		len(parsed.Uploads),
	)
	upstreamCtx, releaseUpstreamCtx := context.WithCancel(ctx)
	defer releaseUpstreamCtx()

	token, _, err := s.GetAccessToken(upstreamCtx, account)
	if err != nil {
		return nil, err
	}
	session := newChatGPTWebSession(s.httpUpstream, account, token, account.ChatGPTWebDevice())
	if parsed.IsEdits() && strings.TrimSpace(parsed.MaskImageURL) != "" {
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images chatgpt-web ignores mask, account_id=%d", account.ID)
	}
	requested := parsed.N
	if requested <= 0 {
		requested = 1
	}
	if requested > chatGPTWebMaxImagesPerRequest {
		return nil, &OpenAIImagesUpstreamError{
			StatusCode: http.StatusBadRequest,
			ErrorType:  "invalid_request_error",
			Message: fmt.Sprintf(
				"n must be between 1 and %d on the chatgpt web image channel",
				chatGPTWebMaxImagesPerRequest,
			),
			Param: "n",
		}
	}
	references, err := session.collectReferences(upstreamCtx, parsed, func(fetchCtx context.Context, rawURL string) ([]byte, string, error) {
		return s.fetchPublicImageBytes(fetchCtx, account, rawURL)
	})
	if err != nil {
		return nil, s.handleChatGPTWebImagesFailure(upstreamCtx, c, account, err)
	}
	upstreamPrompt := chatGPTWebPromptWithCanvasSize(parsed.Prompt, parsed.Size)
	urlOutput := strings.EqualFold(strings.TrimSpace(parsed.ResponseFormat), "url")
	var (
		images       [][]byte
		imageResults []string
	)
	if urlOutput {
		imageResults, err = session.GenerateImageResultBatch(
			upstreamCtx,
			upstreamPrompt,
			requestModel,
			account.ChatGPTWebImageUpstreamModel(),
			references,
			requested,
		)
	} else {
		images, err = session.GenerateImageBatch(
			upstreamCtx,
			upstreamPrompt,
			requestModel,
			account.ChatGPTWebImageUpstreamModel(),
			references,
			requested,
		)
	}
	if err != nil {
		return nil, s.handleChatGPTWebImagesFailure(upstreamCtx, c, account, err)
	}

	body := buildChatGPTWebImagesCompletedBody(requestModel, images)
	if urlOutput {
		body = buildChatGPTWebImagesCompletedBodyFromResults(requestModel, imageResults)
	}
	contentType := "application/json"
	if parsed.Stream {
		if urlOutput {
			body = buildChatGPTWebImagesStreamBodyFromResults(requestModel, imageResults)
		} else {
			body = buildChatGPTWebImagesStreamBody(requestModel, images)
		}
		contentType = "text/event-stream"
	}
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": []string{contentType}, "X-Request-Id": []string{uuid.NewString()}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}
	defer func() { _ = resp.Body.Close() }()

	var (
		usage            OpenAIUsage
		imageCount       int
		imageOutputSizes []string
		firstTokenMs     *int
	)
	if parsed.Stream {
		usage, imageCount, imageOutputSizes, firstTokenMs, err = s.handleOpenAIImagesOAuthStreamingResponse(
			resp,
			c,
			startTime,
			parsed.ResponseFormat,
			openAIImagesStreamPrefix(parsed),
			requestModel,
		)
		if err != nil {
			return nil, err
		}
	} else {
		usage, imageCount, imageOutputSizes, err = s.handleOpenAIImagesOAuthNonStreamingResponse(
			resp,
			c,
			parsed.ResponseFormat,
			requestModel,
		)
		if err != nil {
			return nil, err
		}
	}
	if imageCount <= 0 {
		if urlOutput {
			imageCount = len(imageResults)
		} else {
			imageCount = len(images)
		}
	}
	return &OpenAIForwardResult{
		RequestID:        resp.Header.Get("X-Request-Id"),
		UpstreamHeaders:  resp.Header,
		Usage:            usage,
		Model:            requestModel,
		UpstreamModel:    requestModel,
		Stream:           parsed.Stream,
		ResponseHeaders:  resp.Header.Clone(),
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ImageCount:       imageCount,
		ImageSize:        parsed.SizeTier,
		ImageInputSize:   parsed.Size,
		ImageOutputSizes: imageOutputSizes,
	}, nil
}
