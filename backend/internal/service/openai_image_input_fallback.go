package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/tidwall/gjson"
)

const (
	// openAIImageInputFallbackModeOff 关闭自动降级（保持现状，原样返回上游错误）。
	openAIImageInputFallbackModeOff = "off"
	// openAIImageInputFallbackModeStrip 删除图片内容后重试。
	openAIImageInputFallbackModeStrip = "strip"
	// openAIImageInputFallbackModeDescribe 调用视觉模型生成图片描述，用描述文本替换图片后重试。
	openAIImageInputFallbackModeDescribe = "describe"

	// openAIImageInputFallbackPlaceholderText 图片被移除后消息内容为空时的兜底文本。
	openAIImageInputFallbackPlaceholderText = "[image removed: the upstream model does not support image input]"

	// openAIImageInputVisionDefaultTimeout 视觉描述请求默认超时。
	openAIImageInputVisionDefaultTimeout = 60 * time.Second

	// openAIImageInputVisionDescribePrompt 视觉模型生成描述时使用的提示词。
	openAIImageInputVisionDescribePrompt = "请用简洁的中文描述这张图片的内容，包括其中的文字。只输出描述本身，不要任何前缀或解释。"
)

// openAIImageInputFallbackMode 返回配置的图片输入降级模式。
func openAIImageInputFallbackMode(cfg *config.Config) string {
	if cfg == nil {
		return openAIImageInputFallbackModeOff
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.Gateway.ImageInputFallback))
	switch mode {
	case openAIImageInputFallbackModeStrip, openAIImageInputFallbackModeDescribe:
		return mode
	}
	return openAIImageInputFallbackModeOff
}

// isOpenAIImageURLUnsupportedError 判断上游错误是否为“不支持图片输入(image_url)”导致的 400。
// 典型错误（Rust serde 上游）：
//
//	Failed to deserialize the JSON body into the target type: messages[139]:
//	unknown variant `image_url`, expected `text`
func isOpenAIImageURLUnsupportedError(statusCode int, upstreamMsg string, upstreamBody []byte) bool {
	if statusCode != http.StatusBadRequest {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(upstreamMsg))
	if message == "" {
		message = strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(upstreamBody)))
	}
	if message == "" {
		return false
	}
	if !strings.Contains(message, "image_url") && !strings.Contains(message, "input_image") {
		return false
	}
	return strings.Contains(message, "unknown variant") ||
		strings.Contains(message, "expected `text`") ||
		strings.Contains(message, "expected one of") ||
		strings.Contains(message, "does not support image") ||
		strings.Contains(message, "image is not supported")
}

// openAIImageContentRef 指向一个包含图片 content 数组的 message / responses item。
type openAIImageContentRef struct {
	owner  map[string]any
	isChat bool
}

// isOpenAIImageContentPart 判断 content part 是否为图片输入。
func isOpenAIImageContentPart(part map[string]any) bool {
	switch strings.ToLower(strings.TrimSpace(fmt.Sprint(part["type"]))) {
	case "image_url", "image", "input_image":
		return true
	}
	return false
}

func openAIImageTextPart(text string) map[string]any {
	return map[string]any{"type": "text", "text": text}
}

// openAIImageFallbackTextPartFor 按内容类型构造文本 part：chat 用 text，responses 用 input_text。
func openAIImageFallbackTextPartFor(ref openAIImageContentRef, text string) map[string]any {
	if ref.isChat {
		return openAIImageTextPart(text)
	}
	return map[string]any{"type": "input_text", "text": text}
}

// collectOpenAIImageContentParts 收集请求体中所有图片 content part，并按相同顺序
// 记录所属的 message / responses item，供后续统一替换或删除。
func collectOpenAIImageContentParts(root map[string]any, refs *[]openAIImageContentRef) []map[string]any {
	var parts []map[string]any
	collect := func(container map[string]any, isChat bool) {
		content, ok := container["content"].([]any)
		if !ok {
			return
		}
		hadImage := false
		for _, raw := range content {
			if part, ok := raw.(map[string]any); ok && isOpenAIImageContentPart(part) {
				parts = append(parts, part)
				hadImage = true
			}
		}
		if hadImage {
			*refs = append(*refs, openAIImageContentRef{owner: container, isChat: isChat})
		}
	}
	if messages, ok := root["messages"].([]any); ok {
		for _, raw := range messages {
			if msg, ok := raw.(map[string]any); ok {
				collect(msg, true)
			}
		}
	}
	if input, ok := root["input"].([]any); ok {
		for _, raw := range input {
			if item, ok := raw.(map[string]any); ok {
				collect(item, false)
			}
		}
	}
	return parts
}

// transformOpenAIImageInputBody 将请求体中的图片内容按 mode 处理：
//   - strip：删除图片 part；若消息内容被清空则写入占位文本。
//   - describe：调用 describeAll 为每张图片生成描述，用文本 part 替换图片 part；
//     描述失败或数量不匹配时整体退化为 strip。
//
// 返回修改后的请求体、处理原因、是否有修改及错误。
func transformOpenAIImageInputBody(
	body []byte,
	mode string,
	describeAll func(images []map[string]any) ([]string, error),
) ([]byte, string, bool, error) {
	if len(body) == 0 {
		return nil, "", false, nil
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, "", false, fmt.Errorf("parse request body for image input fallback: %w", err)
	}
	var refs []openAIImageContentRef
	images := collectOpenAIImageContentParts(root, &refs)
	if len(images) == 0 {
		return nil, "", false, nil
	}

	effectiveMode := mode
	var descriptions []string
	if effectiveMode == openAIImageInputFallbackModeDescribe {
		if describeAll == nil {
			effectiveMode = openAIImageInputFallbackModeStrip
		} else {
			descs, err := describeAll(images)
			if err != nil || len(descs) != len(images) {
				effectiveMode = openAIImageInputFallbackModeStrip
			} else {
				descriptions = descs
			}
		}
	}

	changed := false
	partIdx := 0
	for _, ref := range refs {
		content, ok := ref.owner["content"].([]any)
		if !ok {
			continue
		}
		filtered := make([]any, 0, len(content))
		localChanged := false
		for _, raw := range content {
			part, ok := raw.(map[string]any)
			if !ok || !isOpenAIImageContentPart(part) {
				filtered = append(filtered, raw)
				continue
			}
			changed = true
			localChanged = true
			if effectiveMode == openAIImageInputFallbackModeDescribe && partIdx < len(descriptions) {
				filtered = append(filtered, openAIImageFallbackTextPartFor(ref, descriptions[partIdx]))
			}
			partIdx++
		}
		if !localChanged {
			continue
		}
		if len(filtered) == 0 {
			if ref.isChat {
				ref.owner["content"] = openAIImageInputFallbackPlaceholderText
			} else {
				ref.owner["content"] = []any{openAIImageFallbackTextPartFor(ref, openAIImageInputFallbackPlaceholderText)}
			}
		} else {
			ref.owner["content"] = filtered
		}
	}
	if !changed {
		return nil, "", false, nil
	}
	out, err := marshalOpenAIUpstreamJSON(root)
	if err != nil {
		return nil, "", false, fmt.Errorf("marshal image input fallback body: %w", err)
	}
	if effectiveMode == openAIImageInputFallbackModeDescribe {
		return out, "replace image content with vision model description", true, nil
	}
	return out, "strip unsupported image content", true, nil
}

// imageInputFallbackEffectiveSettings 返回实际生效的图片输入降级配置：
// 数据库设置优先（管理后台可配置），未配置时回退到环境变量。
func (s *OpenAIGatewayService) imageInputFallbackEffectiveSettings(
	ctx context.Context,
	cfg *config.Config,
) *ImageInputFallbackSettings {
	if s.settingService != nil {
		if settings, err := s.settingService.GetImageInputFallbackEffectiveSettings(ctx); err == nil && settings != nil {
			return settings
		}
	}
	// 兜底：直接使用环境变量配置
	fallback := DefaultImageInputFallbackSettings()
	if cfg != nil {
		fallback.Mode = openAIImageInputFallbackMode(cfg)
		vision := cfg.Gateway.ImageInputVision
		fallback.VisionBaseURL = strings.TrimSpace(vision.BaseURL)
		fallback.VisionAPIKey = strings.TrimSpace(vision.APIKey)
		fallback.VisionModel = strings.TrimSpace(vision.Model)
		fallback.VisionTimeoutSeconds = vision.TimeoutSeconds
	}
	return fallback
}

// normalizeOpenAIImageInputUnsupportedRetryBody 检测上游“不支持图片输入”错误，
// 按配置对请求体做降级处理，返回可重试的新请求体。
func (s *OpenAIGatewayService) normalizeOpenAIImageInputUnsupportedRetryBody(
	ctx context.Context,
	cfg *config.Config,
	statusCode int,
	body []byte,
	responseBody []byte,
) ([]byte, string, bool, error) {
	if !isOpenAIImageURLUnsupportedError(statusCode, "", responseBody) {
		return nil, "", false, nil
	}
	effective := s.imageInputFallbackEffectiveSettings(ctx, cfg)
	mode := strings.ToLower(strings.TrimSpace(effective.Mode))
	if mode == "" || mode == openAIImageInputFallbackModeOff {
		return nil, "", false, nil
	}
	if mode == openAIImageInputFallbackModeDescribe &&
		(strings.TrimSpace(effective.VisionBaseURL) == "" ||
			strings.TrimSpace(effective.VisionAPIKey) == "" ||
			strings.TrimSpace(effective.VisionModel) == "") {
		mode = openAIImageInputFallbackModeStrip
	}
	var describeAll func(images []map[string]any) ([]string, error)
	if mode == openAIImageInputFallbackModeDescribe {
		vision := *effective
		describeAll = func(images []map[string]any) ([]string, error) {
			return s.describeOpenAIImageInputWithVision(ctx, vision, images)
		}
	}
	retryBody, reason, changed, err := transformOpenAIImageInputBody(body, mode, describeAll)
	if err != nil {
		return nil, "", false, err
	}
	return retryBody, reason, changed, nil
}

// describeOpenAIImageInputWithVision 调用配置的 OpenAI 兼容视觉模型为每张图片生成描述。
func (s *OpenAIGatewayService) describeOpenAIImageInputWithVision(
	ctx context.Context,
	settings ImageInputFallbackSettings,
	images []map[string]any,
) ([]string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(settings.VisionBaseURL), "/")
	timeout := openAIImageInputVisionDefaultTimeout
	if settings.VisionTimeoutSeconds > 0 {
		timeout = time.Duration(settings.VisionTimeoutSeconds) * time.Second
	}
	descriptions := make([]string, 0, len(images))
	for _, image := range images {
		part, ok := normalizeOpenAIImageVisionPart(image)
		if !ok {
			descriptions = append(descriptions, openAIImageInputFallbackPlaceholderText)
			continue
		}
		desc, err := callOpenAIVisionDescribe(ctx, baseURL, settings.VisionAPIKey, settings.VisionModel, part, timeout)
		if err != nil {
			return nil, err
		}
		descriptions = append(descriptions, desc)
	}
	return descriptions, nil
}

// normalizeOpenAIImageVisionPart 将请求中的图片 part 归一化为 OpenAI chat image_url part。
func normalizeOpenAIImageVisionPart(part map[string]any) (map[string]any, bool) {
	switch strings.ToLower(strings.TrimSpace(fmt.Sprint(part["type"]))) {
	case "image_url":
		imageURL, ok := part["image_url"]
		if !ok {
			return nil, false
		}
		switch v := imageURL.(type) {
		case map[string]any:
			return part, true
		case string:
			return map[string]any{"type": "image_url", "image_url": map[string]any{"url": v}}, true
		}
	case "input_image":
		imageURL, ok := part["image_url"]
		if !ok {
			return nil, false
		}
		return map[string]any{
			"type":      "image_url",
			"image_url": map[string]any{"url": fmt.Sprint(imageURL)},
		}, true
	case "image":
		// Claude 风格 source；尽量转换为 data URL。
		source, ok := part["source"].(map[string]any)
		if !ok {
			return nil, false
		}
		mediaType := strings.TrimSpace(fmt.Sprint(source["media_type"]))
		if mediaType == "" {
			mediaType = "image/png"
		}
		return map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": "data:" + mediaType + ";base64," + fmt.Sprint(source["data"]),
			},
		}, true
	}
	return nil, false
}

// callOpenAIVisionDescribe 调用 OpenAI 兼容 /chat/completions 端点生成单张图片描述。
func callOpenAIVisionDescribe(
	ctx context.Context,
	baseURL string,
	apiKey string,
	model string,
	part map[string]any,
	timeout time.Duration,
) (string, error) {
	payload, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": []any{part, openAIImageTextPart(openAIImageInputVisionDescribePrompt)},
			},
		},
		"max_tokens": 1024,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vision describe upstream returned %d: %s", resp.StatusCode, truncateString(string(respBody), 512))
	}
	content := strings.TrimSpace(gjson.GetBytes(respBody, "choices.0.message.content").String())
	if content == "" {
		// 部分兼容端点把多段内容放在 content 数组里。
		var parts []string
		gjson.GetBytes(respBody, "choices.0.message.content").ForEach(func(_, item gjson.Result) bool {
			if text := strings.TrimSpace(item.Get("text").String()); text != "" {
				parts = append(parts, text)
			}
			return true
		})
		content = strings.TrimSpace(strings.Join(parts, "\n"))
	}
	if content == "" {
		return "", fmt.Errorf("vision describe returned empty content")
	}
	return content, nil
}

// openAIBodyHasImageInput 快速检查 OpenAI 兼容请求体是否包含图片输入
// （chat messages / responses input 中的 image_url / input_image / image 类型 content part）。
func openAIBodyHasImageInput(body []byte) bool {
	if !gjson.ValidBytes(body) {
		return false
	}
	found := false
	scan := func(container gjson.Result) {
		container.ForEach(func(_, item gjson.Result) bool {
			content := item.Get("content")
			if content.IsArray() {
				content.ForEach(func(_, part gjson.Result) bool {
					switch strings.ToLower(part.Get("type").String()) {
					case "image_url", "input_image", "image":
						found = true
						return false
					}
					return true
				})
			}
			return !found
		})
	}
	scan(gjson.GetBytes(body, "messages"))
	scan(gjson.GetBytes(body, "input"))
	return found
}

// modelMatchesImageInputFallbackModels 判断模型名是否命中主动处理名单
// （逗号分隔，不区分大小写）。含 * 的条目按通配符匹配（* 匹配任意字符序列），
// 其余条目按子串匹配。
func modelMatchesImageInputFallbackModels(model, modelsCSV string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return false
	}
	for _, entry := range strings.Split(modelsCSV, ",") {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "*") {
			if globMatch(model, entry) {
				return true
			}
			continue
		}
		if strings.Contains(model, entry) {
			return true
		}
	}
	return false
}

// globMatch 判断字符串 s 是否匹配含 * 通配符的 pattern（* 匹配任意字符序列，可为空）。
func globMatch(s, pattern string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return s == pattern
	}
	pos := 0
	// 首段非空时必须从头匹配。
	if parts[0] != "" {
		if !strings.HasPrefix(s, parts[0]) {
			return false
		}
		pos = len(parts[0])
	}
	// 中间段按顺序查找。
	for _, part := range parts[1 : len(parts)-1] {
		if part == "" {
			continue
		}
		idx := strings.Index(s[pos:], part)
		if idx == -1 {
			return false
		}
		pos += idx + len(part)
	}
	// 末段非空时必须尾部匹配。
	last := parts[len(parts)-1]
	if last != "" && !strings.HasSuffix(s, last) {
		return false
	}
	return true
}

// maybeApplyImageInputFallbackBeforeUpstream 多模态模拟：
// 在请求发送到上游之前，对配置了主动处理模型名单（Models）且包含图片输入的请求体
// 执行图片降级（strip=删除图片，describe=用视觉模型描述替换）。
// Models 为空时不主动处理，仅依赖上游报错后的被动重试。
func (s *OpenAIGatewayService) maybeApplyImageInputFallbackBeforeUpstream(
	ctx context.Context,
	cfg *config.Config,
	body []byte,
) ([]byte, string, bool, error) {
	if len(body) == 0 {
		return nil, "", false, nil
	}
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if model == "" || !openAIBodyHasImageInput(body) {
		return nil, "", false, nil
	}
	effective := s.imageInputFallbackEffectiveSettings(ctx, cfg)
	if effective == nil {
		return nil, "", false, nil
	}
	mode := strings.ToLower(strings.TrimSpace(effective.Mode))
	if mode == "" || mode == openAIImageInputFallbackModeOff {
		return nil, "", false, nil
	}
	modelsCSV := strings.TrimSpace(effective.Models)
	if modelsCSV == "" || !modelMatchesImageInputFallbackModels(model, modelsCSV) {
		return nil, "", false, nil
	}
	if mode == openAIImageInputFallbackModeDescribe &&
		(strings.TrimSpace(effective.VisionBaseURL) == "" ||
			strings.TrimSpace(effective.VisionAPIKey) == "" ||
			strings.TrimSpace(effective.VisionModel) == "") {
		mode = openAIImageInputFallbackModeStrip
	}
	var describeAll func(images []map[string]any) ([]string, error)
	if mode == openAIImageInputFallbackModeDescribe {
		vision := *effective
		describeAll = func(images []map[string]any) ([]string, error) {
			return s.describeOpenAIImageInputWithVision(ctx, vision, images)
		}
	}
	retryBody, reason, changed, err := transformOpenAIImageInputBody(body, mode, describeAll)
	if err != nil {
		return nil, "", false, err
	}
	if !changed {
		return nil, "", false, nil
	}
	return retryBody, "proactively " + reason, true, nil
}

// ApplyImageInputFallbackBeforeUpstream 多模态模拟（供 handler 层在请求调度前
// 统一调用）：对配置了主动处理模型名单（Models）且包含图片输入的请求体
// 执行图片降级（strip=删除图片，describe=用视觉模型描述替换）。
// 命中模型时在发送到上游之前处理，避免上游返回 unknown variant `image_url`。
func (s *OpenAIGatewayService) ApplyImageInputFallbackBeforeUpstream(ctx context.Context, body []byte) ([]byte, string, bool, error) {
	return s.maybeApplyImageInputFallbackBeforeUpstream(ctx, s.cfg, body)
}
