package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/imroc/req/v3"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/crypto/sha3"
)

type openAIResponsesImageResult struct {
	Result        string
	RevisedPrompt string
	OutputFormat  string
	Size          string
	Background    string
	Quality       string
	Model         string
}

func openAIResponsesImageResultKey(itemID string, result openAIResponsesImageResult) string {
	if strings.TrimSpace(result.Result) != "" {
		return strings.TrimSpace(result.OutputFormat) + "|" + strings.TrimSpace(result.Result)
	}
	return "item:" + strings.TrimSpace(itemID)
}

func appendOpenAIResponsesImageResultDedup(results *[]openAIResponsesImageResult, seen map[string]struct{}, itemID string, result openAIResponsesImageResult) bool {
	if results == nil {
		return false
	}
	key := openAIResponsesImageResultKey(itemID, result)
	if key != "" {
		if _, exists := seen[key]; exists {
			return false
		}
		seen[key] = struct{}{}
	}
	*results = append(*results, result)
	return true
}

func mergeOpenAIResponsesImageMeta(dst *openAIResponsesImageResult, src openAIResponsesImageResult) {
	if dst == nil {
		return
	}
	if trimmed := strings.TrimSpace(src.OutputFormat); trimmed != "" {
		dst.OutputFormat = trimmed
	}
	if trimmed := strings.TrimSpace(src.Size); trimmed != "" {
		dst.Size = trimmed
	}
	if trimmed := strings.TrimSpace(src.Background); trimmed != "" {
		dst.Background = trimmed
	}
	if trimmed := strings.TrimSpace(src.Quality); trimmed != "" {
		dst.Quality = trimmed
	}
	if trimmed := strings.TrimSpace(src.Model); trimmed != "" {
		dst.Model = trimmed
	}
}

func extractOpenAIResponsesImageMetaFromLifecycleEvent(payload []byte) (openAIResponsesImageResult, int64, bool) {
	switch gjson.GetBytes(payload, "type").String() {
	case "response.created", "response.in_progress", "response.completed":
	default:
		return openAIResponsesImageResult{}, 0, false
	}

	response := gjson.GetBytes(payload, "response")
	if !response.Exists() {
		return openAIResponsesImageResult{}, 0, false
	}

	meta := openAIResponsesImageResult{
		OutputFormat: strings.TrimSpace(response.Get("tools.0.output_format").String()),
		Size:         strings.TrimSpace(response.Get("tools.0.size").String()),
		Background:   strings.TrimSpace(response.Get("tools.0.background").String()),
		Quality:      strings.TrimSpace(response.Get("tools.0.quality").String()),
		Model:        strings.TrimSpace(response.Get("tools.0.model").String()),
	}
	return meta, response.Get("created_at").Int(), true
}

func buildOpenAIImagesStreamPartialPayload(
	eventType string,
	b64 string,
	partialImageIndex int64,
	responseFormat string,
	createdAt int64,
	meta openAIResponsesImageResult,
) []byte {
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
	}

	payload := []byte(`{"type":"","created_at":0,"partial_image_index":0,"b64_json":""}`)
	payload, _ = sjson.SetBytes(payload, "type", eventType)
	payload, _ = sjson.SetBytes(payload, "created_at", createdAt)
	payload, _ = sjson.SetBytes(payload, "partial_image_index", partialImageIndex)
	payload, _ = sjson.SetBytes(payload, "b64_json", b64)
	if strings.EqualFold(strings.TrimSpace(responseFormat), "url") {
		payload, _ = sjson.SetBytes(payload, "url", "data:"+openAIImageOutputMIMEType(meta.OutputFormat)+";base64,"+b64)
	}
	if meta.Background != "" {
		payload, _ = sjson.SetBytes(payload, "background", meta.Background)
	}
	if meta.OutputFormat != "" {
		payload, _ = sjson.SetBytes(payload, "output_format", meta.OutputFormat)
	}
	if meta.Quality != "" {
		payload, _ = sjson.SetBytes(payload, "quality", meta.Quality)
	}
	if meta.Size != "" {
		payload, _ = sjson.SetBytes(payload, "size", meta.Size)
	}
	if meta.Model != "" {
		payload, _ = sjson.SetBytes(payload, "model", meta.Model)
	}
	return payload
}

func buildOpenAIImagesStreamCompletedPayload(
	eventType string,
	img openAIResponsesImageResult,
	responseFormat string,
	createdAt int64,
	usageRaw []byte,
) []byte {
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
	}

	payload := []byte(`{"type":"","created_at":0,"b64_json":""}`)
	payload, _ = sjson.SetBytes(payload, "type", eventType)
	payload, _ = sjson.SetBytes(payload, "created_at", createdAt)
	payload, _ = sjson.SetBytes(payload, "b64_json", img.Result)
	if strings.EqualFold(strings.TrimSpace(responseFormat), "url") {
		payload, _ = sjson.SetBytes(payload, "url", "data:"+openAIImageOutputMIMEType(img.OutputFormat)+";base64,"+img.Result)
	}
	if img.Background != "" {
		payload, _ = sjson.SetBytes(payload, "background", img.Background)
	}
	if img.OutputFormat != "" {
		payload, _ = sjson.SetBytes(payload, "output_format", img.OutputFormat)
	}
	if img.Quality != "" {
		payload, _ = sjson.SetBytes(payload, "quality", img.Quality)
	}
	if img.Size != "" {
		payload, _ = sjson.SetBytes(payload, "size", img.Size)
	}
	if img.Model != "" {
		payload, _ = sjson.SetBytes(payload, "model", img.Model)
	}
	if len(usageRaw) > 0 && gjson.ValidBytes(usageRaw) {
		payload, _ = sjson.SetRawBytes(payload, "usage", usageRaw)
	}
	return payload
}

func openAIImageOutputMIMEType(outputFormat string) string {
	if outputFormat == "" {
		return "image/png"
	}
	if strings.Contains(outputFormat, "/") {
		return outputFormat
	}
	switch strings.ToLower(strings.TrimSpace(outputFormat)) {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func openAIImageUploadToDataURL(upload OpenAIImagesUpload) (string, error) {
	if len(upload.Data) == 0 {
		return "", fmt.Errorf("upload %q is empty", strings.TrimSpace(upload.FileName))
	}
	contentType := strings.TrimSpace(upload.ContentType)
	if contentType == "" {
		contentType = http.DetectContentType(upload.Data)
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(upload.Data), nil
}

func buildOpenAIImagesResponsesRequest(parsed *OpenAIImagesRequest, toolModel string) ([]byte, error) {
	if parsed == nil {
		return nil, fmt.Errorf("parsed images request is required")
	}
	prompt := strings.TrimSpace(parsed.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	inputImages := make([]string, 0, len(parsed.InputImageURLs)+len(parsed.Uploads))
	for _, imageURL := range parsed.InputImageURLs {
		if trimmed := strings.TrimSpace(imageURL); trimmed != "" {
			inputImages = append(inputImages, trimmed)
		}
	}
	for _, upload := range parsed.Uploads {
		dataURL, err := openAIImageUploadToDataURL(upload)
		if err != nil {
			return nil, err
		}
		inputImages = append(inputImages, dataURL)
	}
	if parsed.IsEdits() && len(inputImages) == 0 {
		return nil, fmt.Errorf("image input is required")
	}

	req := []byte(`{"instructions":"","stream":true,"reasoning":{"effort":"medium","summary":"auto"},"parallel_tool_calls":true,"include":["reasoning.encrypted_content"],"model":"","store":false,"tool_choice":{"type":"image_generation"}}`)
	req, _ = sjson.SetBytes(req, "model", openAIImagesResponsesMainModel)

	input := []byte(`[{"type":"message","role":"user","content":[{"type":"input_text","text":""}]}]`)
	input, _ = sjson.SetBytes(input, "0.content.0.text", prompt)
	for index, imageURL := range inputImages {
		part := []byte(`{"type":"input_image","image_url":""}`)
		part, _ = sjson.SetBytes(part, "image_url", imageURL)
		input, _ = sjson.SetRawBytes(input, fmt.Sprintf("0.content.%d", index+1), part)
	}
	req, _ = sjson.SetRawBytes(req, "input", input)

	action := "generate"
	if parsed.IsEdits() {
		action = "edit"
	}
	tool := []byte(`{"type":"image_generation","action":"","model":""}`)
	tool, _ = sjson.SetBytes(tool, "action", action)
	tool, _ = sjson.SetBytes(tool, "model", strings.TrimSpace(toolModel))

	for _, field := range []struct {
		path  string
		value string
	}{
		{path: "size", value: parsed.Size},
		{path: "quality", value: parsed.Quality},
		{path: "background", value: parsed.Background},
		{path: "output_format", value: parsed.OutputFormat},
		{path: "moderation", value: parsed.Moderation},
		{path: "style", value: parsed.Style},
	} {
		if trimmed := strings.TrimSpace(field.value); trimmed != "" {
			tool, _ = sjson.SetBytes(tool, field.path, trimmed)
		}
	}
	if parsed.OutputCompression != nil {
		tool, _ = sjson.SetBytes(tool, "output_compression", *parsed.OutputCompression)
	}
	if parsed.PartialImages != nil {
		tool, _ = sjson.SetBytes(tool, "partial_images", *parsed.PartialImages)
	}

	maskImageURL := strings.TrimSpace(parsed.MaskImageURL)
	if parsed.MaskUpload != nil {
		dataURL, err := openAIImageUploadToDataURL(*parsed.MaskUpload)
		if err != nil {
			return nil, err
		}
		maskImageURL = dataURL
	}
	if maskImageURL != "" {
		tool, _ = sjson.SetBytes(tool, "input_image_mask.image_url", maskImageURL)
	}

	req, _ = sjson.SetRawBytes(req, "tools", []byte(`[]`))
	req, _ = sjson.SetRawBytes(req, "tools.-1", tool)
	return req, nil
}

const (
	openAIChatGPTConversationPrepareURL = "https://chatgpt.com/backend-api/f/conversation/prepare"
	openAIChatGPTConversationURL        = "https://chatgpt.com/backend-api/f/conversation"
	openAIChatGPTRequirementsURL        = "https://chatgpt.com/backend-api/sentinel/chat-requirements"
	openAIChatGPTDefaultPOWScript       = "https://chatgpt.com/backend-api/sentinel/sdk.js"
)

type openAIChatRequirements struct {
	Token       string
	ProofToken  string
	Turnstile   string
	SOToken     string
	RawFinalize []byte
}

func openAIImagesChatGPTModelSlug(model string) string {
	switch strings.TrimSpace(model) {
	case "":
		return "auto"
	case "gpt-image-2":
		return "gpt-5-3"
	case "codex-gpt-image-2":
		return "codex-gpt-image-2"
	default:
		return "auto"
	}
}

func buildOpenAIImagesChatGPTPrepareRequest(parsed *OpenAIImagesRequest, requestModel string) ([]byte, error) {
	if parsed == nil {
		return nil, fmt.Errorf("parsed images request is required")
	}
	prompt := strings.TrimSpace(parsed.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	payload := []byte(`{"action":"next","fork_from_shared_post":false,"parent_message_id":"","model":"","client_prepare_state":"success","timezone_offset_min":-480,"timezone":"Asia/Shanghai","conversation_mode":{"kind":"primary_assistant"},"system_hints":["picture_v2"],"partial_query":{"id":"","author":{"role":"user"},"content":{"content_type":"text","parts":[""]}},"supports_buffering":true,"supported_encodings":["v1"],"client_contextual_info":{"app_name":"chatgpt.com"}}`)
	payload, _ = sjson.SetBytes(payload, "parent_message_id", uuid.NewString())
	payload, _ = sjson.SetBytes(payload, "model", openAIImagesChatGPTModelSlug(requestModel))
	payload, _ = sjson.SetBytes(payload, "partial_query.id", uuid.NewString())
	payload, _ = sjson.SetBytes(payload, "partial_query.content.parts.0", prompt)
	return payload, nil
}

func buildOpenAIImagesChatGPTConversationRequest(parsed *OpenAIImagesRequest, requestModel string, references []map[string]any) ([]byte, error) {
	if parsed == nil {
		return nil, fmt.Errorf("parsed images request is required")
	}
	prompt := strings.TrimSpace(parsed.Prompt)
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	parts := make([]any, 0, len(references)+1)
	attachments := make([]any, 0, len(references))
	for _, ref := range references {
		fileID := strings.TrimSpace(fmt.Sprint(ref["file_id"]))
		if fileID == "" {
			continue
		}
		parts = append(parts, map[string]any{
			"content_type":  "image_asset_pointer",
			"asset_pointer": "file-service://" + fileID,
			"width":         ref["width"],
			"height":        ref["height"],
			"size_bytes":    ref["file_size"],
		})
		attachments = append(attachments, map[string]any{
			"id":       fileID,
			"mimeType": ref["mime_type"],
			"name":     ref["file_name"],
			"size":     ref["file_size"],
			"width":    ref["width"],
			"height":   ref["height"],
		})
	}
	parts = append(parts, prompt)
	contentType := "text"
	if len(parts) > 1 {
		contentType = "multimodal_text"
	}
	metadata := map[string]any{
		"developer_mode_connector_ids": []any{},
		"selected_github_repos":        []any{},
		"selected_all_github_repos":    false,
		"system_hints":                 []string{"picture_v2"},
		"serialization_metadata":       map[string]any{"custom_symbol_offsets": []any{}},
	}
	if len(attachments) > 0 {
		metadata["attachments"] = attachments
	}
	payload := map[string]any{
		"action": "next",
		"messages": []any{map[string]any{
			"id":          uuid.NewString(),
			"author":      map[string]any{"role": "user"},
			"create_time": float64(time.Now().UnixNano()) / float64(time.Second),
			"content":     map[string]any{"content_type": contentType, "parts": parts},
			"metadata":    metadata,
		}},
		"parent_message_id":                    uuid.NewString(),
		"model":                                openAIImagesChatGPTModelSlug(requestModel),
		"client_prepare_state":                 "sent",
		"timezone_offset_min":                  -480,
		"timezone":                             "Asia/Shanghai",
		"conversation_mode":                    map[string]any{"kind": "primary_assistant"},
		"enable_message_followups":             true,
		"system_hints":                         []string{"picture_v2"},
		"supports_buffering":                   true,
		"supported_encodings":                  []string{"v1"},
		"client_contextual_info":               map[string]any{"is_dark_mode": false, "time_since_loaded": 1200, "page_height": 1072, "page_width": 1724, "pixel_ratio": 1.2, "screen_height": 1440, "screen_width": 2560, "app_name": "chatgpt.com"},
		"paragen_cot_summary_display_override": "allow",
		"force_parallel_switch":                "auto",
	}
	return json.Marshal(payload)
}

func openAIImagesChatGPTHeaders(account *Account, token, path string, extra http.Header) http.Header {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)
	headers.Set("User-Agent", openAIImageBackendUserAgent)
	headers.Set("Origin", "https://chatgpt.com")
	headers.Set("Referer", "https://chatgpt.com/")
	headers.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Pragma", "no-cache")
	headers.Set("OAI-Language", "zh-CN")
	headers.Set("OAI-Device-Id", uuid.NewString())
	headers.Set("OAI-Session-Id", uuid.NewString())
	headers.Set("X-OpenAI-Target-Path", path)
	headers.Set("X-OpenAI-Target-Route", path)
	if account != nil {
		if chatgptAccountID := account.GetChatGPTAccountID(); chatgptAccountID != "" {
			headers.Set("chatgpt-account-id", chatgptAccountID)
		}
		if ua := account.GetOpenAIUserAgent(); ua != "" {
			headers.Set("User-Agent", ua)
		}
	}
	for key, values := range extra {
		headers.Del(key)
		for _, value := range values {
			headers.Add(key, value)
		}
	}
	return headers
}

func buildOpenAIImagesLegacyRequirementsToken(userAgent string) string {
	seed := fmt.Sprintf("%0.16f", rand.Float64())
	config := []any{
		3000,
		time.Now().UTC().Format("Mon Jan 02 2006 15:04:05") + " GMT-0500 (Eastern Standard Time)",
		4294705152,
		0,
		userAgent,
		openAIChatGPTDefaultPOWScript,
		"",
		"en-US",
		"en-US,es-US,en,es",
		0,
		"hardwareConcurrency-32",
		"location",
		"navigator",
		float64(time.Now().UnixMilli()),
		uuid.NewString(),
		"",
		32,
		float64(time.Now().UnixMilli()),
	}
	answer, _ := generateOpenAIImagesPOW(seed, "0fffff", config, 500000)
	return "gAAAAAC" + answer
}

func buildOpenAIImagesProofToken(seed, difficulty, userAgent string) (string, error) {
	if strings.TrimSpace(seed) == "" || strings.TrimSpace(difficulty) == "" {
		return "", fmt.Errorf("missing proof token challenge")
	}
	config := []any{
		3000,
		time.Now().UTC().Format("Mon Jan 02 2006 15:04:05") + " GMT-0500 (Eastern Standard Time)",
		4294705152,
		0,
		userAgent,
		openAIChatGPTDefaultPOWScript,
		"",
		"en-US",
		"en-US,es-US,en,es",
		0,
		"hardwareConcurrency-32",
		"location",
		"navigator",
		float64(time.Now().UnixMilli()),
		uuid.NewString(),
		"",
		32,
		float64(time.Now().UnixMilli()),
	}
	answer, solved := generateOpenAIImagesPOW(strings.TrimSpace(seed), strings.TrimSpace(difficulty), config, 500000)
	if !solved {
		return "", fmt.Errorf("failed to solve proof token challenge")
	}
	return "gAAAAAB" + answer, nil
}

func generateOpenAIImagesPOW(seed string, difficulty string, config []any, limit int) (string, bool) {
	target, err := hexStringBytes(difficulty)
	if err != nil || len(target) == 0 {
		return "", false
	}
	diffLen := len(target)
	seedBytes := []byte(seed)
	static1 := mustMarshalOpenAIPOWJSONPrefix(config[:3], true)
	static2 := mustMarshalOpenAIPOWJSONMiddle(config[4:9])
	static3 := mustMarshalOpenAIPOWJSONSuffix(config[10:])
	for i := 0; i < limit; i++ {
		finalJSON := append([]byte{}, static1...)
		finalJSON = append(finalJSON, []byte(fmt.Sprintf("%d", i))...)
		finalJSON = append(finalJSON, static2...)
		finalJSON = append(finalJSON, []byte(fmt.Sprintf("%d", i>>1))...)
		finalJSON = append(finalJSON, static3...)
		encoded := []byte(base64.StdEncoding.EncodeToString(finalJSON))
		h := sha3.New512()
		_, _ = h.Write(seedBytes)
		_, _ = h.Write(encoded)
		if bytes.Compare(h.Sum(nil)[:diffLen], target) <= 0 {
			return string(encoded), true
		}
	}
	return "wQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D" + base64.StdEncoding.EncodeToString([]byte(`"`+seed+`"`)), false
}

func mustMarshalOpenAIPOWJSONPrefix(values []any, trailingComma bool) []byte {
	raw, _ := json.Marshal(values)
	if trailingComma && len(raw) > 0 {
		raw = raw[:len(raw)-1]
		raw = append(raw, ',')
	}
	return raw
}

func mustMarshalOpenAIPOWJSONMiddle(values []any) []byte {
	raw, _ := json.Marshal(values)
	if len(raw) >= 2 {
		raw = raw[1 : len(raw)-1]
	}
	out := []byte{','}
	out = append(out, raw...)
	return append(out, ',')
}

func mustMarshalOpenAIPOWJSONSuffix(values []any) []byte {
	raw, _ := json.Marshal(values)
	if len(raw) >= 1 {
		raw = raw[1:]
	}
	out := []byte{','}
	return append(out, raw...)
}

func hexStringBytes(raw string) ([]byte, error) {
	if len(raw)%2 != 0 {
		raw = "0" + raw
	}
	out := make([]byte, len(raw)/2)
	for i := 0; i < len(out); i++ {
		var b byte
		for j := 0; j < 2; j++ {
			ch := raw[i*2+j]
			switch {
			case ch >= '0' && ch <= '9':
				b = b*16 + ch - '0'
			case ch >= 'a' && ch <= 'f':
				b = b*16 + ch - 'a' + 10
			case ch >= 'A' && ch <= 'F':
				b = b*16 + ch - 'A' + 10
			default:
				return nil, fmt.Errorf("invalid hex")
			}
		}
		out[i] = b
	}
	return out, nil
}

func extractOpenAIImagesConversationID(payload []byte) string {
	if !gjson.ValidBytes(payload) {
		return ""
	}
	for _, path := range []string{"conversation_id", "conversation.id", "message.conversation_id"} {
		if value := strings.TrimSpace(gjson.GetBytes(payload, path).String()); value != "" {
			return value
		}
	}
	return ""
}

func extractOpenAIImagesPOWResources(html string) ([]string, string) {
	matches := regexp.MustCompile(`<script[^>]+src=["']([^"']+)["']`).FindAllStringSubmatch(html, -1)
	sources := make([]string, 0, len(matches))
	dataBuild := ""
	for _, match := range matches {
		if len(match) < 2 || strings.TrimSpace(match[1]) == "" {
			continue
		}
		sources = append(sources, strings.TrimSpace(match[1]))
		if dataBuild == "" {
			if hit := regexp.MustCompile(`c/[^/]*/_`).FindString(match[1]); hit != "" {
				dataBuild = hit
			}
		}
	}
	if len(sources) == 0 {
		sources = []string{openAIChatGPTDefaultPOWScript}
	}
	if dataBuild == "" {
		if match := regexp.MustCompile(`<html[^>]*data-build=["']([^"']*)["']`).FindStringSubmatch(html); len(match) > 1 {
			dataBuild = match[1]
		}
	}
	return sources, dataBuild
}

func (s *OpenAIGatewayService) doOpenAIImagesChatGPTRequest(ctx context.Context, account *Account, method, targetURL, path string, body []byte, token string, headers http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Host = "chatgpt.com"
	for key, values := range openAIImagesChatGPTHeaders(account, token, path, headers) {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	proxyURL := ""
	if account != nil && account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	return s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
}

func (s *OpenAIGatewayService) getOpenAIImagesChatRequirements(ctx context.Context, account *Account, token string) (openAIChatRequirements, error) {
	bootstrapResp, err := s.doOpenAIImagesChatGPTRequest(ctx, account, http.MethodGet, openAIChatGPTStartURL, "/", nil, token, http.Header{
		"Accept": {"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"},
	})
	if err != nil {
		return openAIChatRequirements{}, err
	}
	bootstrapBody, _ := io.ReadAll(io.LimitReader(bootstrapResp.Body, 2<<20))
	_ = bootstrapResp.Body.Close()
	if bootstrapResp.StatusCode >= 400 {
		return openAIChatRequirements{}, fmt.Errorf("chatgpt bootstrap failed: status %d", bootstrapResp.StatusCode)
	}
	_, _ = extractOpenAIImagesPOWResources(string(bootstrapBody))

	userAgent := openAIImagesChatGPTHeaders(account, token, "/", nil).Get("User-Agent")
	requirementsBody, _ := json.Marshal(map[string]string{"p": buildOpenAIImagesLegacyRequirementsToken(userAgent)})
	resp, err := s.doOpenAIImagesChatGPTRequest(ctx, account, http.MethodPost, openAIChatGPTRequirementsURL, "/backend-api/sentinel/chat-requirements", requirementsBody, token, http.Header{
		"Content-Type": {"application/json"},
		"Accept":       {"application/json"},
	})
	if err != nil {
		return openAIChatRequirements{}, err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		return openAIChatRequirements{}, fmt.Errorf("chat requirements failed: status %d", resp.StatusCode)
	}
	requirements := openAIChatRequirements{
		Token:       strings.TrimSpace(gjson.GetBytes(body, "token").String()),
		SOToken:     strings.TrimSpace(gjson.GetBytes(body, "so_token").String()),
		RawFinalize: append([]byte(nil), body...),
	}
	if requirements.Token == "" {
		return openAIChatRequirements{}, fmt.Errorf("missing chat requirements token")
	}
	if gjson.GetBytes(body, "arkose.required").Bool() {
		return openAIChatRequirements{}, fmt.Errorf("chat requirements requires arkose token")
	}
	if gjson.GetBytes(body, "turnstile.required").Bool() {
		return openAIChatRequirements{}, fmt.Errorf("chat requirements requires turnstile token")
	}
	if gjson.GetBytes(body, "proofofwork.required").Bool() {
		proofToken, err := buildOpenAIImagesProofToken(
			gjson.GetBytes(body, "proofofwork.seed").String(),
			gjson.GetBytes(body, "proofofwork.difficulty").String(),
			userAgent,
		)
		if err != nil {
			return openAIChatRequirements{}, err
		}
		requirements.ProofToken = proofToken
	}
	return requirements, nil
}

func (r openAIChatRequirements) headers(accept string) http.Header {
	if accept == "" {
		accept = "*/*"
	}
	headers := http.Header{
		"Content-Type": {"application/json"},
		"Accept":       {accept},
		"OpenAI-Sentinel-Chat-Requirements-Token": {r.Token},
	}
	if r.ProofToken != "" {
		headers.Set("OpenAI-Sentinel-Proof-Token", r.ProofToken)
	}
	if r.Turnstile != "" {
		headers.Set("OpenAI-Sentinel-Turnstile-Token", r.Turnstile)
	}
	if r.SOToken != "" {
		headers.Set("OpenAI-Sentinel-SO-Token", r.SOToken)
	}
	if accept == "text/event-stream" {
		headers.Set("X-Oai-Turn-Trace-Id", uuid.NewString())
	}
	return headers
}

func (s *OpenAIGatewayService) prepareOpenAIImagesChatGPTConversation(
	ctx context.Context,
	account *Account,
	token string,
	parsed *OpenAIImagesRequest,
	requestModel string,
	requirements openAIChatRequirements,
) (string, []byte, error) {
	body, err := buildOpenAIImagesChatGPTPrepareRequest(parsed, requestModel)
	if err != nil {
		return "", nil, err
	}
	resp, err := s.doOpenAIImagesChatGPTRequest(ctx, account, http.MethodPost, openAIChatGPTConversationPrepareURL, "/backend-api/f/conversation/prepare", body, token, requirements.headers("*/*"))
	if err != nil {
		return "", body, err
	}
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", body, fmt.Errorf("prepare image conversation failed: status %d: %s", resp.StatusCode, sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(respBody)))
	}
	conduitToken := strings.TrimSpace(gjson.GetBytes(respBody, "conduit_token").String())
	if conduitToken == "" {
		return "", body, fmt.Errorf("prepare image conversation did not return conduit token")
	}
	return conduitToken, body, nil
}

func (s *OpenAIGatewayService) startOpenAIImagesChatGPTConversation(
	ctx context.Context,
	account *Account,
	token string,
	parsed *OpenAIImagesRequest,
	requestModel string,
	requirements openAIChatRequirements,
	conduitToken string,
) (*http.Response, []byte, error) {
	body, err := buildOpenAIImagesChatGPTConversationRequest(parsed, requestModel, nil)
	if err != nil {
		return nil, nil, err
	}
	headers := requirements.headers("text/event-stream")
	headers.Set("X-Conduit-Token", conduitToken)
	resp, err := s.doOpenAIImagesChatGPTRequest(ctx, account, http.MethodPost, openAIChatGPTConversationURL, "/backend-api/f/conversation", body, token, headers)
	if err != nil {
		return nil, body, err
	}
	return resp, body, nil
}

func (s *OpenAIGatewayService) collectOpenAIImagesFromChatGPTConversationBody(
	ctx context.Context,
	body []byte,
	account *Account,
	token string,
	fallbackPrompt string,
) ([]openAIResponsesImageResult, int64, OpenAIUsage, error) {
	var (
		pointers       []openAIImagePointerInfo
		conversationID string
		usage          OpenAIUsage
		createdAt      int64
	)
	forEachOpenAISSEDataPayload(string(body), func(payload []byte) {
		if !gjson.ValidBytes(payload) {
			return
		}
		mergeOpenAIUsage(&usage, payload)
		if conversationID == "" {
			conversationID = extractOpenAIImagesConversationID(payload)
		}
		if createdAt <= 0 {
			if t := gjson.GetBytes(payload, "message.create_time").Float(); t > 0 {
				createdAt = int64(t)
			}
		}
		pointers = mergeOpenAIImagePointerInfos(pointers, collectOpenAIImagePointers(payload))
	})
	if len(pointers) == 0 && gjson.ValidBytes(body) {
		conversationID = extractOpenAIImagesConversationID(body)
		pointers = collectOpenAIImagePointers(body)
		mergeOpenAIUsage(&usage, body)
	}
	hasUsablePointer := false
	for _, pointer := range pointers {
		if strings.TrimSpace(pointer.Pointer) != "" ||
			strings.TrimSpace(pointer.DownloadURL) != "" ||
			strings.TrimSpace(pointer.B64JSON) != "" {
			hasUsablePointer = true
			break
		}
	}
	if !hasUsablePointer && gjson.ValidBytes(body) {
		if b64 := strings.TrimSpace(gjson.GetBytes(body, "b64_json").String()); b64 != "" {
			pointers = append(pointers, openAIImagePointerInfo{
				B64JSON:  b64,
				MimeType: strings.TrimSpace(gjson.GetBytes(body, "mime_type").String()),
				Prompt:   strings.TrimSpace(gjson.GetBytes(body, "revised_prompt").String()),
			})
		}
	}
	if len(pointers) == 0 {
		return nil, createdAt, usage, fmt.Errorf("upstream did not return image output")
	}

	headers := openAIImagesChatGPTHeaders(account, token, "/", http.Header{"Accept": {"application/json"}})
	client := req.C()
	results := make([]openAIResponsesImageResult, 0, len(pointers))
	for _, pointer := range pointers {
		if strings.TrimSpace(pointer.Pointer) == "" &&
			strings.TrimSpace(pointer.DownloadURL) == "" &&
			strings.TrimSpace(pointer.B64JSON) == "" {
			continue
		}
		if b64 := strings.TrimSpace(pointer.B64JSON); b64 != "" {
			revisedPrompt := strings.TrimSpace(pointer.Prompt)
			if revisedPrompt == "" {
				revisedPrompt = strings.TrimSpace(fallbackPrompt)
			}
			results = append(results, openAIResponsesImageResult{
				Result:        b64,
				RevisedPrompt: revisedPrompt,
				OutputFormat:  strings.TrimPrefix(openAIImageOutputMIMEType(pointer.MimeType), "image/"),
			})
			continue
		}
		data, err := resolveOpenAIImageBytes(ctx, client, headers, conversationID, pointer)
		if err != nil {
			return nil, createdAt, usage, err
		}
		revisedPrompt := strings.TrimSpace(pointer.Prompt)
		if revisedPrompt == "" {
			revisedPrompt = strings.TrimSpace(fallbackPrompt)
		}
		result := openAIResponsesImageResult{
			Result:        base64.StdEncoding.EncodeToString(data),
			RevisedPrompt: revisedPrompt,
			OutputFormat:  strings.TrimPrefix(openAIImageOutputMIMEType(pointer.MimeType), "image/"),
		}
		results = append(results, result)
	}
	if len(results) == 0 {
		return nil, createdAt, usage, fmt.Errorf("upstream did not return image output")
	}
	return results, createdAt, usage, nil
}

func (s *OpenAIGatewayService) handleOpenAIImagesChatGPTNonStreamingResponse(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	token string,
	parsed *OpenAIImagesRequest,
	fallbackModel string,
) (OpenAIUsage, int, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return OpenAIUsage{}, 0, err
	}
	results, createdAt, usage, err := s.collectOpenAIImagesFromChatGPTConversationBody(ctx, body, account, token, parsed.Prompt)
	if err != nil {
		return OpenAIUsage{}, 0, err
	}
	firstMeta := openAIResponsesImageResult{Model: strings.TrimSpace(fallbackModel)}
	if len(results) > 0 {
		firstMeta = results[0]
		firstMeta.Model = strings.TrimSpace(fallbackModel)
	}
	responseBody, err := buildOpenAIImagesAPIResponse(results, createdAt, nil, firstMeta, parsed.ResponseFormat)
	if err != nil {
		return OpenAIUsage{}, 0, err
	}
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.Data(resp.StatusCode, "application/json; charset=utf-8", responseBody)
	return usage, len(results), nil
}

func (s *OpenAIGatewayService) handleOpenAIImagesChatGPTStreamingResponse(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	token string,
	parsed *OpenAIImagesRequest,
	startTime time.Time,
	streamPrefix string,
	fallbackModel string,
) (OpenAIUsage, int, *int, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return OpenAIUsage{}, 0, nil, err
	}
	firstTokenMs := int(time.Since(startTime).Milliseconds())
	results, createdAt, usage, err := s.collectOpenAIImagesFromChatGPTConversationBody(ctx, body, account, token, parsed.Prompt)
	if err != nil {
		return usage, 0, &firstTokenMs, err
	}
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(resp.StatusCode)
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return usage, 0, &firstTokenMs, fmt.Errorf("streaming is not supported by response writer")
	}
	eventName := streamPrefix + ".completed"
	for _, img := range results {
		img.Model = strings.TrimSpace(fallbackModel)
		if err := s.writeOpenAIImagesStreamEvent(c, flusher, eventName, buildOpenAIImagesStreamCompletedPayload(eventName, img, parsed.ResponseFormat, createdAt, nil)); err != nil {
			return usage, 0, &firstTokenMs, err
		}
	}
	return usage, len(results), &firstTokenMs, nil
}

func extractOpenAIImagesFromResponsesCompleted(payload []byte) ([]openAIResponsesImageResult, int64, []byte, openAIResponsesImageResult, error) {
	if gjson.GetBytes(payload, "type").String() != "response.completed" {
		return nil, 0, nil, openAIResponsesImageResult{}, fmt.Errorf("unexpected event type")
	}

	createdAt := gjson.GetBytes(payload, "response.created_at").Int()
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
	}

	var (
		results   []openAIResponsesImageResult
		firstMeta openAIResponsesImageResult
	)
	output := gjson.GetBytes(payload, "response.output")
	if output.IsArray() {
		for _, item := range output.Array() {
			if item.Get("type").String() != "image_generation_call" {
				continue
			}
			result := strings.TrimSpace(item.Get("result").String())
			if result == "" {
				continue
			}
			entry := openAIResponsesImageResult{
				Result:        result,
				RevisedPrompt: strings.TrimSpace(item.Get("revised_prompt").String()),
				OutputFormat:  strings.TrimSpace(item.Get("output_format").String()),
				Size:          strings.TrimSpace(item.Get("size").String()),
				Background:    strings.TrimSpace(item.Get("background").String()),
				Quality:       strings.TrimSpace(item.Get("quality").String()),
			}
			if len(results) == 0 {
				firstMeta = entry
			}
			results = append(results, entry)
		}
	}

	var usageRaw []byte
	if usage := gjson.GetBytes(payload, "response.tool_usage.image_gen"); usage.Exists() && usage.IsObject() {
		usageRaw = []byte(usage.Raw)
	}
	return results, createdAt, usageRaw, firstMeta, nil
}

func extractOpenAIImageFromResponsesOutputItemDone(payload []byte) (openAIResponsesImageResult, string, bool, error) {
	if gjson.GetBytes(payload, "type").String() != "response.output_item.done" {
		return openAIResponsesImageResult{}, "", false, fmt.Errorf("unexpected event type")
	}

	item := gjson.GetBytes(payload, "item")
	if !item.Exists() || item.Get("type").String() != "image_generation_call" {
		return openAIResponsesImageResult{}, "", false, nil
	}

	result := strings.TrimSpace(item.Get("result").String())
	if result == "" {
		return openAIResponsesImageResult{}, "", false, nil
	}

	entry := openAIResponsesImageResult{
		Result:        result,
		RevisedPrompt: strings.TrimSpace(item.Get("revised_prompt").String()),
		OutputFormat:  strings.TrimSpace(item.Get("output_format").String()),
		Size:          strings.TrimSpace(item.Get("size").String()),
		Background:    strings.TrimSpace(item.Get("background").String()),
		Quality:       strings.TrimSpace(item.Get("quality").String()),
	}
	return entry, strings.TrimSpace(item.Get("id").String()), true, nil
}

func collectOpenAIImagesFromResponsesBody(body []byte) ([]openAIResponsesImageResult, int64, []byte, openAIResponsesImageResult, bool, error) {
	var (
		fallbackResults []openAIResponsesImageResult
		fallbackSeen    = make(map[string]struct{})
		finalResults    []openAIResponsesImageResult
		finalMeta       openAIResponsesImageResult
		collectErr      error
		createdAt       int64
		usageRaw        []byte
		foundFinal      bool
		responseMeta    openAIResponsesImageResult
	)

	forEachOpenAISSEDataPayload(string(body), func(payload []byte) {
		if collectErr != nil || len(finalResults) > 0 {
			return
		}
		if !gjson.ValidBytes(payload) {
			return
		}
		if meta, eventCreatedAt, ok := extractOpenAIResponsesImageMetaFromLifecycleEvent(payload); ok {
			mergeOpenAIResponsesImageMeta(&responseMeta, meta)
			if eventCreatedAt > 0 {
				createdAt = eventCreatedAt
			}
		}

		switch gjson.GetBytes(payload, "type").String() {
		case "response.output_item.done":
			result, itemID, ok, err := extractOpenAIImageFromResponsesOutputItemDone(payload)
			if err != nil {
				collectErr = err
				return
			}
			if ok {
				mergeOpenAIResponsesImageMeta(&result, responseMeta)
				appendOpenAIResponsesImageResultDedup(&fallbackResults, fallbackSeen, itemID, result)
			}
		case "response.completed":
			results, completedAt, completedUsageRaw, firstMeta, err := extractOpenAIImagesFromResponsesCompleted(payload)
			if err != nil {
				collectErr = err
				return
			}
			foundFinal = true
			if completedAt > 0 {
				createdAt = completedAt
			}
			if len(completedUsageRaw) > 0 {
				usageRaw = completedUsageRaw
			}
			if len(results) > 0 {
				mergeOpenAIResponsesImageMeta(&firstMeta, responseMeta)
				finalResults = results
				finalMeta = firstMeta
				return
			}
			if len(fallbackResults) > 0 {
				firstMeta = fallbackResults[0]
				mergeOpenAIResponsesImageMeta(&firstMeta, responseMeta)
				finalResults = fallbackResults
				finalMeta = firstMeta
				return
			}
		}
	})
	if collectErr != nil {
		return nil, 0, nil, openAIResponsesImageResult{}, false, collectErr
	}
	if len(finalResults) > 0 {
		return finalResults, createdAt, usageRaw, finalMeta, true, nil
	}

	if len(fallbackResults) > 0 {
		firstMeta := fallbackResults[0]
		mergeOpenAIResponsesImageMeta(&firstMeta, responseMeta)
		return fallbackResults, createdAt, usageRaw, firstMeta, foundFinal, nil
	}
	return nil, createdAt, usageRaw, openAIResponsesImageResult{}, foundFinal, nil
}

func buildOpenAIImagesAPIResponse(
	results []openAIResponsesImageResult,
	createdAt int64,
	usageRaw []byte,
	firstMeta openAIResponsesImageResult,
	responseFormat string,
) ([]byte, error) {
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
	}
	out := []byte(`{"created":0,"data":[]}`)
	out, _ = sjson.SetBytes(out, "created", createdAt)

	format := strings.ToLower(strings.TrimSpace(responseFormat))
	if format == "" {
		format = "b64_json"
	}
	for _, img := range results {
		item := []byte(`{}`)
		if format == "url" {
			item, _ = sjson.SetBytes(item, "url", "data:"+openAIImageOutputMIMEType(img.OutputFormat)+";base64,"+img.Result)
		} else {
			item, _ = sjson.SetBytes(item, "b64_json", img.Result)
		}
		if img.RevisedPrompt != "" {
			item, _ = sjson.SetBytes(item, "revised_prompt", img.RevisedPrompt)
		}
		out, _ = sjson.SetRawBytes(out, "data.-1", item)
	}
	if firstMeta.Background != "" {
		out, _ = sjson.SetBytes(out, "background", firstMeta.Background)
	}
	if firstMeta.OutputFormat != "" {
		out, _ = sjson.SetBytes(out, "output_format", firstMeta.OutputFormat)
	}
	if firstMeta.Quality != "" {
		out, _ = sjson.SetBytes(out, "quality", firstMeta.Quality)
	}
	if firstMeta.Size != "" {
		out, _ = sjson.SetBytes(out, "size", firstMeta.Size)
	}
	if firstMeta.Model != "" {
		out, _ = sjson.SetBytes(out, "model", firstMeta.Model)
	}
	if len(usageRaw) > 0 && gjson.ValidBytes(usageRaw) {
		out, _ = sjson.SetRawBytes(out, "usage", usageRaw)
	}
	return out, nil
}

func openAIImagesStreamPrefix(parsed *OpenAIImagesRequest) string {
	if parsed != nil && parsed.IsEdits() {
		return "image_edit"
	}
	return "image_generation"
}

func buildOpenAIImagesStreamErrorBody(message string) []byte {
	body := []byte(`{"type":"error","error":{"type":"upstream_error","message":""}}`)
	if strings.TrimSpace(message) == "" {
		message = "upstream request failed"
	}
	body, _ = sjson.SetBytes(body, "error.message", message)
	return body
}

func (s *OpenAIGatewayService) writeOpenAIImagesStreamEvent(c *gin.Context, flusher http.Flusher, eventName string, payload []byte) error {
	if strings.TrimSpace(eventName) != "" {
		if _, err := fmt.Fprintf(c.Writer, "event: %s\n", eventName); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", payload); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func (s *OpenAIGatewayService) tryWriteOpenAIImagesStreamEvent(
	c *gin.Context,
	flusher http.Flusher,
	clientDisconnected *bool,
	lastWriteAt *time.Time,
	eventName string,
	payload []byte,
) bool {
	if clientDisconnected != nil && *clientDisconnected {
		return false
	}
	if err := s.writeOpenAIImagesStreamEvent(c, flusher, eventName, payload); err != nil {
		if clientDisconnected != nil {
			*clientDisconnected = true
		}
		logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images stream client disconnected, continue draining upstream for billing")
		return false
	}
	if lastWriteAt != nil {
		*lastWriteAt = time.Now()
	}
	return true
}

func (s *OpenAIGatewayService) handleOpenAIImagesOAuthNonStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	responseFormat string,
	fallbackModel string,
) (OpenAIUsage, int, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return OpenAIUsage{}, 0, err
	}

	var usage OpenAIUsage
	forEachOpenAISSEDataPayload(string(body), func(data []byte) {
		s.parseSSEUsageBytes(data, &usage)
	})
	results, createdAt, usageRaw, firstMeta, _, err := collectOpenAIImagesFromResponsesBody(body)
	if err != nil {
		return OpenAIUsage{}, 0, err
	}
	if len(results) == 0 {
		return OpenAIUsage{}, 0, fmt.Errorf("upstream did not return image output")
	}
	if strings.TrimSpace(firstMeta.Model) == "" {
		firstMeta.Model = strings.TrimSpace(fallbackModel)
	}

	responseBody, err := buildOpenAIImagesAPIResponse(results, createdAt, usageRaw, firstMeta, responseFormat)
	if err != nil {
		return OpenAIUsage{}, 0, err
	}
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.Data(resp.StatusCode, "application/json; charset=utf-8", responseBody)
	return usage, len(results), nil
}

func (s *OpenAIGatewayService) handleOpenAIImagesOAuthStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	startTime time.Time,
	responseFormat string,
	streamPrefix string,
	fallbackModel string,
) (OpenAIUsage, int, *int, error) {
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(resp.StatusCode)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return OpenAIUsage{}, 0, nil, fmt.Errorf("streaming is not supported by response writer")
	}

	format := strings.ToLower(strings.TrimSpace(responseFormat))
	if format == "" {
		format = "b64_json"
	}

	usage := OpenAIUsage{}
	imageCount := 0
	var firstTokenMs *int
	emitted := make(map[string]struct{})
	pendingResults := make([]openAIResponsesImageResult, 0, 1)
	pendingSeen := make(map[string]struct{})
	streamMeta := openAIResponsesImageResult{Model: strings.TrimSpace(fallbackModel)}
	var createdAt int64
	clientDisconnected := false
	lastDownstreamWriteAt := time.Now()
	var sseData openAISSEDataAccumulator
	var processDataErr error
	processDataDone := false

	processData := func(dataBytes []byte) {
		if processDataDone || processDataErr != nil {
			return
		}
		if firstTokenMs == nil {
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}
		s.parseSSEUsageBytes(dataBytes, &usage)
		if !gjson.ValidBytes(dataBytes) {
			return
		}
		if meta, eventCreatedAt, ok := extractOpenAIResponsesImageMetaFromLifecycleEvent(dataBytes); ok {
			mergeOpenAIResponsesImageMeta(&streamMeta, meta)
			if eventCreatedAt > 0 {
				createdAt = eventCreatedAt
			}
		}
		switch gjson.GetBytes(dataBytes, "type").String() {
		case "response.image_generation_call.partial_image":
			b64 := strings.TrimSpace(gjson.GetBytes(dataBytes, "partial_image_b64").String())
			if b64 == "" {
				return
			}
			eventName := streamPrefix + ".partial_image"
			partialMeta := streamMeta
			mergeOpenAIResponsesImageMeta(&partialMeta, openAIResponsesImageResult{
				OutputFormat: strings.TrimSpace(gjson.GetBytes(dataBytes, "output_format").String()),
				Background:   strings.TrimSpace(gjson.GetBytes(dataBytes, "background").String()),
			})
			payload := buildOpenAIImagesStreamPartialPayload(
				eventName,
				b64,
				gjson.GetBytes(dataBytes, "partial_image_index").Int(),
				format,
				createdAt,
				partialMeta,
			)
			s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, eventName, payload)
		case "response.output_item.done":
			img, itemID, ok, extractErr := extractOpenAIImageFromResponsesOutputItemDone(dataBytes)
			if extractErr != nil {
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(extractErr.Error()))
				processDataErr = extractErr
				processDataDone = true
				return
			}
			if !ok {
				return
			}
			mergeOpenAIResponsesImageMeta(&streamMeta, img)
			mergeOpenAIResponsesImageMeta(&img, streamMeta)
			key := openAIResponsesImageResultKey(itemID, img)
			if _, exists := emitted[key]; exists {
				return
			}
			if _, exists := pendingSeen[key]; exists {
				return
			}
			pendingSeen[key] = struct{}{}
			pendingResults = append(pendingResults, img)
		case "response.completed":
			results, _, usageRaw, firstMeta, extractErr := extractOpenAIImagesFromResponsesCompleted(dataBytes)
			if extractErr != nil {
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(extractErr.Error()))
				processDataErr = extractErr
				processDataDone = true
				return
			}
			mergeOpenAIResponsesImageMeta(&streamMeta, firstMeta)
			finalResults := make([]openAIResponsesImageResult, 0, len(results)+len(pendingResults))
			finalSeen := make(map[string]struct{})
			for _, img := range results {
				mergeOpenAIResponsesImageMeta(&img, streamMeta)
				appendOpenAIResponsesImageResultDedup(&finalResults, finalSeen, "", img)
			}
			for _, img := range pendingResults {
				mergeOpenAIResponsesImageMeta(&img, streamMeta)
				appendOpenAIResponsesImageResultDedup(&finalResults, finalSeen, "", img)
			}
			if len(finalResults) == 0 {
				outputErr := fmt.Errorf("upstream did not return image output")
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(outputErr.Error()))
				processDataErr = outputErr
				processDataDone = true
				return
			}
			eventName := streamPrefix + ".completed"
			for _, img := range finalResults {
				key := openAIResponsesImageResultKey("", img)
				if _, exists := emitted[key]; exists {
					continue
				}
				payload := buildOpenAIImagesStreamCompletedPayload(eventName, img, format, createdAt, usageRaw)
				emitted[key] = struct{}{}
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, eventName, payload)
			}
			imageCount = len(emitted)
			processDataDone = true
		}
	}

	processLine := func(line []byte) (bool, error) {
		if len(line) == 0 {
			return false, nil
		}
		sseData.AddLine(string(line), processData)
		if processDataErr != nil {
			return true, processDataErr
		}
		return processDataDone, nil
	}

	flushData := func() (bool, error) {
		sseData.Flush(processData)
		if processDataErr != nil {
			return true, processDataErr
		}
		return processDataDone, nil
	}

	finalizePending := func() error {
		if imageCount > 0 {
			return nil
		}
		if len(pendingResults) > 0 {
			eventName := streamPrefix + ".completed"
			for _, img := range pendingResults {
				mergeOpenAIResponsesImageMeta(&img, streamMeta)
				key := openAIResponsesImageResultKey("", img)
				if _, exists := emitted[key]; exists {
					continue
				}
				payload := buildOpenAIImagesStreamCompletedPayload(eventName, img, format, createdAt, nil)
				emitted[key] = struct{}{}
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, eventName, payload)
			}
			imageCount = len(emitted)
			return nil
		}

		streamErr := fmt.Errorf("stream disconnected before image generation completed")
		s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(streamErr.Error()))
		return streamErr
	}

	streamInterval := s.openAIImageStreamDataInterval()
	keepaliveInterval := s.openAIImageStreamKeepaliveInterval()
	if streamInterval <= 0 && keepaliveInterval <= 0 {
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadBytes('\n')
			done, processErr := processLine(line)
			if processErr != nil {
				return usage, imageCount, firstTokenMs, processErr
			}
			if done {
				return usage, imageCount, firstTokenMs, nil
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				if done, processErr := flushData(); processErr != nil {
					return usage, imageCount, firstTokenMs, processErr
				} else if done {
					return usage, imageCount, firstTokenMs, nil
				}
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(err.Error()))
				return usage, imageCount, firstTokenMs, err
			}
		}
		if done, processErr := flushData(); processErr != nil {
			return usage, imageCount, firstTokenMs, processErr
		} else if done {
			return usage, imageCount, firstTokenMs, nil
		}
		if err := finalizePending(); err != nil {
			return usage, imageCount, firstTokenMs, err
		}
		return usage, imageCount, firstTokenMs, nil
	}

	type readEvent struct {
		line []byte
		err  error
	}
	events := make(chan readEvent, 16)
	done := make(chan struct{})
	sendEvent := func(ev readEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
		}
	}
	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
	go func() {
		defer close(events)
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
			}
			if len(line) > 0 && !sendEvent(readEvent{line: line}) {
				return
			}
			if err == io.EOF {
				return
			}
			if err != nil {
				_ = sendEvent(readEvent{err: err})
				return
			}
		}
	}()
	defer close(done)

	var intervalTicker *time.Ticker
	if streamInterval > 0 {
		intervalTicker = time.NewTicker(streamInterval)
		defer intervalTicker.Stop()
	}
	var intervalCh <-chan time.Time
	if intervalTicker != nil {
		intervalCh = intervalTicker.C
	}

	var keepaliveTicker *time.Ticker
	if keepaliveInterval > 0 {
		keepaliveTicker = time.NewTicker(keepaliveInterval)
		defer keepaliveTicker.Stop()
	}
	var keepaliveCh <-chan time.Time
	if keepaliveTicker != nil {
		keepaliveCh = keepaliveTicker.C
	}

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				if done, processErr := flushData(); processErr != nil {
					return usage, imageCount, firstTokenMs, processErr
				} else if done {
					return usage, imageCount, firstTokenMs, nil
				}
				if err := finalizePending(); err != nil {
					return usage, imageCount, firstTokenMs, err
				}
				return usage, imageCount, firstTokenMs, nil
			}
			if ev.err != nil {
				if done, processErr := flushData(); processErr != nil {
					return usage, imageCount, firstTokenMs, processErr
				} else if done {
					return usage, imageCount, firstTokenMs, nil
				}
				s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(ev.err.Error()))
				return usage, imageCount, firstTokenMs, ev.err
			}
			done, processErr := processLine(ev.line)
			if processErr != nil {
				return usage, imageCount, firstTokenMs, processErr
			}
			if done {
				return usage, imageCount, firstTokenMs, nil
			}
		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
			}
			if clientDisconnected {
				return usage, imageCount, firstTokenMs, fmt.Errorf("image stream incomplete after timeout")
			}
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images responses stream data interval timeout: interval=%s", streamInterval)
			s.tryWriteOpenAIImagesStreamEvent(c, flusher, &clientDisconnected, &lastDownstreamWriteAt, "error", buildOpenAIImagesStreamErrorBody(fmt.Sprintf("upstream image stream idle for %s", streamInterval)))
			return usage, imageCount, firstTokenMs, fmt.Errorf("image stream data interval timeout")
		case <-keepaliveCh:
			if clientDisconnected || time.Since(lastDownstreamWriteAt) < keepaliveInterval {
				continue
			}
			if _, writeErr := io.WriteString(c.Writer, ":\n\n"); writeErr != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Images responses stream client disconnected during keepalive, continue draining upstream for billing")
				continue
			}
			flusher.Flush()
			lastDownstreamWriteAt = time.Now()
		}
	}
}

func (s *OpenAIGatewayService) forwardOpenAIImagesOAuthResponses(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIImagesRequest,
	requestModel string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, parsed.Stream)
	defer releaseUpstreamCtx()

	token, _, err := s.GetAccessToken(upstreamCtx, account)
	if err != nil {
		return nil, err
	}

	responsesBody, err := buildOpenAIImagesResponsesRequest(parsed, requestModel)
	if err != nil {
		return nil, err
	}
	setOpsUpstreamRequestBody(c, responsesBody)

	upstreamReq, err := s.buildUpstreamRequest(upstreamCtx, c, account, responsesBody, token, true, parsed.StickySessionSeed(), false)
	if err != nil {
		return nil, err
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Accept", "text/event-stream")

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
			Kind:               "request_error",
			Message:            safeErr,
		})
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			s.handleFailoverSideEffects(upstreamCtx, resp, account)
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return s.handleErrorResponse(upstreamCtx, resp, c, account, responsesBody)
	}
	defer func() { _ = resp.Body.Close() }()

	var (
		usage        OpenAIUsage
		imageCount   int
		firstTokenMs *int
	)
	if parsed.Stream {
		usage, imageCount, firstTokenMs, err = s.handleOpenAIImagesOAuthStreamingResponse(resp, c, startTime, parsed.ResponseFormat, openAIImagesStreamPrefix(parsed), requestModel)
		if err != nil {
			if imageCount > 0 {
				return &OpenAIForwardResult{
					RequestID:       resp.Header.Get("x-request-id"),
					Usage:           usage,
					Model:           requestModel,
					UpstreamModel:   requestModel,
					Stream:          parsed.Stream,
					ResponseHeaders: resp.Header.Clone(),
					Duration:        time.Since(startTime),
					FirstTokenMs:    firstTokenMs,
					ImageCount:      imageCount,
					ImageSize:       parsed.SizeTier,
				}, err
			}
			return nil, err
		}
	} else {
		usage, imageCount, err = s.handleOpenAIImagesOAuthNonStreamingResponse(resp, c, parsed.ResponseFormat, requestModel)
		if err != nil {
			return nil, err
		}
	}
	if imageCount <= 0 {
		imageCount = parsed.N
	}
	return &OpenAIForwardResult{
		RequestID:       resp.Header.Get("x-request-id"),
		Usage:           usage,
		Model:           requestModel,
		UpstreamModel:   requestModel,
		Stream:          parsed.Stream,
		ResponseHeaders: resp.Header.Clone(),
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
		ImageCount:      imageCount,
		ImageSize:       parsed.SizeTier,
	}, nil
}

func (s *OpenAIGatewayService) forwardOpenAIImagesOAuth(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	requestModel := strings.TrimSpace(parsed.Model)
	if mapped := strings.TrimSpace(channelMappedModel); mapped != "" {
		requestModel = mapped
	}
	if requestModel == "" {
		requestModel = "gpt-image-2"
	}
	if err := validateOpenAIImagesModel(requestModel); err != nil {
		return nil, err
	}
	logger.LegacyPrintf(
		"service.openai_gateway",
		"[OpenAI] Images request routing request_model=%s endpoint=%s account_type=%s uploads=%d",
		requestModel,
		parsed.Endpoint,
		account.Type,
		len(parsed.Uploads),
	)
	if parsed.N > 1 {
		logger.LegacyPrintf(
			"service.openai_gateway",
			"[Warning] Codex /responses image tool requested n=%d; falling back to n=1 request_model=%s endpoint=%s",
			parsed.N,
			requestModel,
			parsed.Endpoint,
		)
	}
	if parsed.Stream || parsed.IsEdits() {
		return s.forwardOpenAIImagesOAuthResponses(ctx, c, account, parsed, requestModel, startTime)
	}

	upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, parsed.Stream)
	defer releaseUpstreamCtx()

	token, _, err := s.GetAccessToken(upstreamCtx, account)
	if err != nil {
		return nil, err
	}

	requirements, err := s.getOpenAIImagesChatRequirements(upstreamCtx, account, token)
	if err != nil {
		return nil, err
	}
	conduitToken, prepareBody, err := s.prepareOpenAIImagesChatGPTConversation(upstreamCtx, account, token, parsed, requestModel, requirements)
	if err != nil {
		return nil, err
	}
	setOpsUpstreamRequestBody(c, prepareBody)

	upstreamStart := time.Now()
	resp, startBody, err := s.startOpenAIImagesChatGPTConversation(upstreamCtx, account, token, parsed, requestModel, requirements, conduitToken)
	if len(startBody) > 0 {
		setOpsUpstreamRequestBody(c, startBody)
	}
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			UpstreamURL:        safeUpstreamURL(openAIChatGPTConversationURL),
			Kind:               "request_error",
			Message:            safeErr,
		})
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				UpstreamURL:        safeUpstreamURL(openAIChatGPTConversationURL),
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			s.handleFailoverSideEffects(upstreamCtx, resp, account)
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return s.handleErrorResponse(upstreamCtx, resp, c, account, startBody)
	}
	defer func() { _ = resp.Body.Close() }()

	var (
		usage        OpenAIUsage
		imageCount   int
		firstTokenMs *int
	)
	if parsed.Stream {
		usage, imageCount, firstTokenMs, err = s.handleOpenAIImagesChatGPTStreamingResponse(upstreamCtx, resp, c, account, token, parsed, startTime, openAIImagesStreamPrefix(parsed), requestModel)
		if err != nil {
			if imageCount > 0 {
				return &OpenAIForwardResult{
					RequestID:       resp.Header.Get("x-request-id"),
					Usage:           usage,
					Model:           requestModel,
					UpstreamModel:   requestModel,
					Stream:          parsed.Stream,
					ResponseHeaders: resp.Header.Clone(),
					Duration:        time.Since(startTime),
					FirstTokenMs:    firstTokenMs,
					ImageCount:      imageCount,
					ImageSize:       parsed.SizeTier,
				}, err
			}
			return nil, err
		}
	} else {
		usage, imageCount, err = s.handleOpenAIImagesChatGPTNonStreamingResponse(upstreamCtx, resp, c, account, token, parsed, requestModel)
		if err != nil {
			return nil, err
		}
	}
	if imageCount <= 0 {
		imageCount = parsed.N
	}
	return &OpenAIForwardResult{
		RequestID:       resp.Header.Get("x-request-id"),
		Usage:           usage,
		Model:           requestModel,
		UpstreamModel:   requestModel,
		Stream:          parsed.Stream,
		ResponseHeaders: resp.Header.Clone(),
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
		ImageCount:      imageCount,
		ImageSize:       parsed.SizeTier,
	}, nil
}
