package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type openCodeImageRewriteOptions struct {
	BaseURL                string
	RewrittenImageMessages map[string]map[string]any
	GeneratedMessages      *[]openCodeImageGeneratedMessage
}

type openCodePublicSettingsProvider interface {
	GetPublicSettings(ctx context.Context) (*PublicSettings, error)
}

const openCodeImageServerContinuationToolName = "sub2api_image_generation_result"

func rewriteOpenCodeImageGenerationOutput(ctx context.Context, body []byte, store *OpenAIGeneratedImageStore, opts openCodeImageRewriteOptions) ([]byte, bool, error) {
	patched, _, changed, err := rewriteOpenCodeImageGenerationOutputWithGenerated(ctx, body, store, opts)
	return patched, changed, err
}

func rewriteOpenCodeImageGenerationOutputWithGenerated(ctx context.Context, body []byte, store *OpenAIGeneratedImageStore, opts openCodeImageRewriteOptions) ([]byte, []openCodeImageGeneratedMessage, bool, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body, nil, false, nil
	}
	output := gjson.GetBytes(body, "output")
	if !output.Exists() || !output.IsArray() {
		return body, nil, false, nil
	}

	rewritten, generated, changed, err := rewriteOpenCodeImageGenerationOutputItems(ctx, []byte(output.Raw), store, opts, false)
	if err != nil {
		return body, nil, false, err
	}
	if !changed {
		return body, nil, false, nil
	}

	outputJSON, err := json.Marshal(rewritten)
	if err != nil {
		return body, nil, false, err
	}
	patched, err := sjson.SetRawBytes(body, "output", outputJSON)
	if err != nil {
		return body, nil, false, err
	}
	return patched, generated, true, nil
}

type openCodeImageGeneratedMessage struct {
	OutputIndex int
	Message     map[string]any
	Record      *OpenAIGeneratedImageRecord
}

func rewriteOpenCodeImageGenerationOutputItems(ctx context.Context, outputRaw []byte, store *OpenAIGeneratedImageStore, opts openCodeImageRewriteOptions, safeFallback bool) ([]json.RawMessage, []openCodeImageGeneratedMessage, bool, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(outputRaw, &items); err != nil {
		return nil, nil, false, err
	}

	rewritten := make([]json.RawMessage, 0, len(items))
	generated := make([]openCodeImageGeneratedMessage, 0, 1)
	changed := false
	for idx, raw := range items {
		outputType := gjson.GetBytes(raw, "type").String()
		if isOpenCodeFilteredProviderBuiltInOutputType(outputType) {
			changed = true
			continue
		}
		if outputType != "image_generation_call" {
			rewritten = append(rewritten, raw)
			continue
		}

		sourceItemID := strings.TrimSpace(gjson.GetBytes(raw, "id").String())
		if sourceItemID != "" && opts.RewrittenImageMessages != nil {
			if message, ok := opts.RewrittenImageMessages[sourceItemID]; ok {
				messageJSON, err := json.Marshal(message)
				if err != nil {
					return nil, nil, false, err
				}
				rewritten = append(rewritten, messageJSON)
				changed = true
				continue
			}
		}

		message, rec, err := buildOpenCodeImageGenerationMessageForRawItem(ctx, raw, store, opts, safeFallback)
		if err != nil {
			return nil, nil, false, err
		}
		messageJSON, err := json.Marshal(message)
		if err != nil {
			return nil, nil, false, err
		}
		rewritten = append(rewritten, messageJSON)
		if sourceItemID != "" && opts.RewrittenImageMessages != nil {
			opts.RewrittenImageMessages[sourceItemID] = message
		}
		generated = append(generated, openCodeImageGeneratedMessage{
			OutputIndex: idx,
			Message:     message,
			Record:      rec,
		})
		if rec != nil && opts.GeneratedMessages != nil {
			*opts.GeneratedMessages = append(*opts.GeneratedMessages, openCodeImageGeneratedMessage{
				OutputIndex: idx,
				Message:     message,
				Record:      rec,
			})
		}
		changed = true
	}

	return rewritten, generated, changed, nil
}

func buildOpenCodeImageGenerationMessageForRawItem(ctx context.Context, raw json.RawMessage, store *OpenAIGeneratedImageStore, opts openCodeImageRewriteOptions, safeFallback bool) (map[string]any, *OpenAIGeneratedImageRecord, error) {
	result := strings.TrimSpace(gjson.GetBytes(raw, "result").String())
	if result == "" {
		return buildOpenCodeImageNoResultMessage(gjson.GetBytes(raw, "id").String()), nil, nil
	}
	if store == nil {
		if safeFallback {
			return buildOpenCodeImageNoResultMessage(gjson.GetBytes(raw, "id").String()), nil, nil
		}
		return nil, nil, fmt.Errorf("rewrite opencode image generation output: generated image store is nil")
	}
	rec, err := store.SaveBase64(ctx, OpenAIGeneratedImageSaveInput{
		Base64:       result,
		OutputFormat: gjson.GetBytes(raw, "output_format").String(),
		SourceItemID: gjson.GetBytes(raw, "id").String(),
	})
	if err != nil {
		if safeFallback {
			return buildOpenCodeImageNoResultMessage(gjson.GetBytes(raw, "id").String()), nil, nil
		}
		return nil, nil, err
	}
	return buildOpenCodeGeneratedImageMessage(rec, opts), &rec, nil
}

func buildOpenCodeGeneratedImageMessage(rec OpenAIGeneratedImageRecord, opts openCodeImageRewriteOptions) map[string]any {
	downloadPath := "/sub2api/generated-images/" + rec.Filename
	text := "Generated image: sub2api-image://" + rec.ID
	if baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/"); baseURL != "" {
		text += "\nI'll download from URL: " + baseURL + downloadPath
	}

	return map[string]any{
		"id":     "msg_sub2api_" + rec.ID,
		"type":   "message",
		"status": "completed",
		"role":   "assistant",
		"content": []any{
			map[string]any{
				"type":        "output_text",
				"text":        text,
				"annotations": []any{},
			},
		},
	}
}

func buildOpenCodeImageNoResultMessage(sourceItemID string) map[string]any {
	messageID := "msg_sub2api_no_image_result"
	if sourceItemID = strings.TrimSpace(sourceItemID); sourceItemID != "" {
		messageID += "_" + sourceItemID
	}
	return map[string]any{
		"id":     messageID,
		"type":   "message",
		"status": "completed",
		"role":   "assistant",
		"content": []any{
			map[string]any{
				"type":        "output_text",
				"text":        "OpenAI image generation completed with no image result.",
				"annotations": []any{},
			},
		},
	}
}

func buildOpenCodeImageServerContinuationBody(body []byte, generated []openCodeImageGeneratedMessage) ([]byte, bool, error) {
	if len(generated) == 0 || len(body) == 0 || !gjson.ValidBytes(body) {
		return body, false, nil
	}
	var reqBody map[string]any
	if err := json.Unmarshal(body, &reqBody); err != nil {
		return body, false, err
	}
	input, ok := normalizeOpenCodeImageContinuationInput(reqBody["input"])
	if !ok {
		return body, false, nil
	}
	appended := false
	for _, item := range generated {
		if item.Record == nil || item.Message == nil {
			continue
		}
		messageJSON, err := json.Marshal(item.Message)
		if err != nil {
			return body, false, err
		}
		messageText := strings.TrimSpace(gjson.GetBytes(messageJSON, "content.0.text").String())
		if messageText == "" {
			continue
		}
		callID := "call_sub2api_continue_" + item.Record.ID
		input = append(input,
			map[string]any{"type": "function_call", "call_id": callID, "name": openCodeImageServerContinuationToolName, "arguments": "{}"},
			map[string]any{"type": "function_call_output", "call_id": callID, "output": openCodeImageServerContinuationOutput(messageText)},
		)
		appended = true
	}
	if !appended {
		return body, false, nil
	}
	reqBody["input"] = input
	if isOpenCodeImageGenerationToolChoice(reqBody["tool_choice"]) {
		delete(reqBody, "tool_choice")
	}
	patched, err := json.Marshal(reqBody)
	if err != nil {
		return body, false, err
	}
	return patched, true, nil
}

func normalizeOpenCodeImageContinuationInput(raw any) ([]any, bool) {
	switch value := raw.(type) {
	case []any:
		return append([]any(nil), value...), true
	case string:
		return []any{map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": value}}}}, true
	case map[string]any:
		return []any{value}, true
	case nil:
		return []any{}, true
	default:
		return nil, false
	}
}

func openCodeImageServerContinuationOutput(messageText string) string {
	return messageText + "\n\nSynthetic image generation continuation inserted by sub2api for OpenCode. The image generation tool already completed and produced the real image result above. Do not treat this wrapper as the final answer. Do not stop after image generation. Immediately use the available shell, command-line, or network-access tool to download the image from the URL above, then respond to the user with the downloaded image or file result. Only if no tool can access the URL, say that explicitly and provide the URL and sub2api-image marker instead of silently finishing. Do not ask the user to regenerate the image unless the saved result is unavailable."
}

func isOpenCodeImageGenerationToolChoice(raw any) bool {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value) == "image_generation"
	case map[string]any:
		return strings.TrimSpace(asStringMaybe(value["type"])) == "image_generation"
	default:
		return false
	}
}

func (s *OpenAIGatewayService) resolveOpenCodeImageDownloadBaseURL(ctx context.Context, c *gin.Context) string {
	if ctx == nil {
		ctx = context.Background()
	}
	if s != nil && s.publicSettingsProvider != nil {
		settings, err := s.publicSettingsProvider.GetPublicSettings(ctx)
		if err == nil && settings != nil {
			if baseURL := normalizeOpenCodeConfiguredBaseURL(settings.APIBaseURL, true); baseURL != "" {
				return baseURL
			}
		}
	}
	var cfg *config.Config
	if s != nil {
		cfg = s.cfg
	}
	return resolveOpenCodeImageDownloadBaseURL(c, cfg)
}

func resolveOpenCodeImageDownloadBaseURL(c *gin.Context, cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	frontendURL := strings.TrimSpace(cfg.Server.FrontendURL)
	if frontendURL == "" {
		return resolveOpenCodeTrustedRequestBaseURL(c, cfg)
	}
	return normalizeOpenCodeConfiguredBaseURL(frontendURL, false)
}

func normalizeOpenCodeConfiguredBaseURL(raw string, stripV1Suffix bool) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	path := strings.TrimRight(parsed.Path, "/")
	if stripV1Suffix {
		switch {
		case path == "/v1":
			path = ""
		case strings.HasSuffix(path, "/v1"):
			path = strings.TrimSuffix(path, "/v1")
		}
	}
	parsed.Path = path
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/")
}

func resolveOpenCodeTrustedRequestBaseURL(c *gin.Context, cfg *config.Config) string {
	if !isOpenCodeRequestFromTrustedProxy(c, cfg) {
		return ""
	}
	proto := firstOpenCodeForwardedHeaderToken(c.Request.Header.Get("X-Forwarded-Proto"))
	if proto == "" && c.Request.TLS != nil {
		proto = "https"
	}
	proto = strings.ToLower(proto)
	if proto != "http" && proto != "https" {
		return ""
	}

	host := firstOpenCodeForwardedHeaderToken(c.Request.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(c.Request.Host)
	}
	return buildOpenCodeTrustedBaseURL(proto, host)
}

func isOpenCodeRequestFromTrustedProxy(c *gin.Context, cfg *config.Config) bool {
	if c == nil || c.Request == nil || cfg == nil || len(cfg.Server.TrustedProxies) == 0 {
		return false
	}
	remote := strings.TrimSpace(c.Request.RemoteAddr)
	if remote == "" {
		return false
	}
	var remoteAddr netip.Addr
	if addrPort, err := netip.ParseAddrPort(remote); err == nil {
		remoteAddr = addrPort.Addr()
	} else {
		addr, err := netip.ParseAddr(strings.Trim(remote, "[]"))
		if err != nil {
			return false
		}
		remoteAddr = addr
	}

	for _, trusted := range cfg.Server.TrustedProxies {
		trusted = strings.TrimSpace(trusted)
		if trusted == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(trusted); err == nil {
			if prefix.Contains(remoteAddr) {
				return true
			}
			continue
		}
		if addr, err := netip.ParseAddr(trusted); err == nil && addr == remoteAddr {
			return true
		}
	}
	return false
}

func firstOpenCodeForwardedHeaderToken(value string) string {
	for _, token := range strings.Split(value, ",") {
		if token = strings.TrimSpace(token); token != "" {
			return token
		}
	}
	return ""
}

func buildOpenCodeTrustedBaseURL(proto string, host string) string {
	if !isSafeOpenCodeRequestHost(host) {
		return ""
	}
	parsed, err := url.Parse(proto + "://" + host)
	if err != nil || parsed.Scheme != proto || parsed.Host != host || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}
	return proto + "://" + host
}

func isSafeOpenCodeRequestHost(host string) bool {
	if host == "" || strings.TrimSpace(host) != host {
		return false
	}
	for _, r := range host {
		if r <= ' ' || r == 0x7f {
			return false
		}
	}
	return true
}
