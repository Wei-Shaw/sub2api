package service

// This opt-in prototype keeps the client's Responses contract while executing
// its image tool on the server. It intentionally handles HTTP Responses only;
// WebSocket sessions require the corresponding ingress adapter before rollout.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptwebimage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/image/draw"
)

const webImageFunctionName = "sub2api_web_image"
const webImageResponsePrefix = "ig_web_"

var webImagePreviewPattern = regexp.MustCompile(`!\[Generated image\]\(data:image/jpeg;base64,[A-Za-z0-9+/=\r\n]+\)`)

type webImageBridgeContextKey struct{}

// ShouldBridgeChatGPTWebResponses includes disabled/unconfigured Free accounts
// so their image tools cannot fall back to native Codex generation.
func ShouldBridgeChatGPTWebResponses(ctx context.Context, c *gin.Context, account *Account) bool {
	if ctx.Value(webImageBridgeContextKey{}) != nil || !UsesChatGPTWebImagePolicy(account) || isOpenAIResponsesCompactPath(c) {
		return false
	}
	return true
}

func requestedChatGPTWebImageModel(body []byte) (string, bool, error) {
	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil || request == nil {
		return "", false, errors.New("responses request must be a JSON object")
	}
	model, explicit := "", false
	if topModel, _ := request["model"].(string); IsGPTImageGenerationModel(topModel) {
		model, explicit = strings.TrimSpace(topModel), true
	}
	tools, _ := request["tools"].([]any)
	if input, ok := request["input"].([]any); ok {
		for _, raw := range input {
			if item, ok := raw.(map[string]any); ok && item["type"] == "additional_tools" {
				additional, _ := item["tools"].([]any)
				tools = append(tools, additional...)
			}
		}
	}
	imageTools, otherTools := 0, 0
	for _, raw := range tools {
		if isOpenAIImageToolDeclaration(raw) {
			imageTools++
		} else {
			otherTools++
		}
		tool, ok := raw.(map[string]any)
		if !ok || tool["type"] != "image_generation" {
			continue
		}
		value, exists := tool["model"]
		if !exists || value == nil {
			continue
		}
		name, ok := value.(string)
		if !ok {
			return "", explicit, errors.New("image_generation model must be a string")
		}
		name = strings.TrimSpace(name)
		if name != "" && model != "" && name != model {
			return "", explicit, errors.New("multiple image compatibility models are not supported in one turn")
		}
		if name != "" {
			model = name
		}
	}
	if openAIJSONToolChoiceSelectsExplicitImageGeneration(gjson.GetBytes(body, "tool_choice")) {
		explicit = true
	}
	if request["tool_choice"] == "required" && imageTools > 0 && otherTools == 0 {
		explicit = true
	}
	return model, explicit, nil
}

func validateWebImageToolParameters(tool map[string]any) error {
	if tool["type"] != "image_generation" {
		return nil
	}
	for key, value := range tool {
		if value == nil {
			continue
		}
		switch key {
		case "type", "model":
		case "action":
			if value != "auto" && value != "generate" {
				return errors.New("experimental web images do not support image editing")
			}
		case "output_format":
			if value != "png" {
				return errors.New("experimental web images only support PNG output")
			}
		case "size", "quality":
			if value != "auto" {
				return fmt.Errorf("experimental web images do not support explicit %s", key)
			}
		case "background":
			if value != "auto" && value != "opaque" {
				return errors.New("experimental web images do not support transparent backgrounds")
			}
		default:
			return fmt.Errorf("experimental web images do not support image parameter %s", key)
		}
	}
	return nil
}

func prepareWebImageResponsesForAccount(body []byte, account *Account, groupAllowed bool) ([]byte, *chatGPTWebImageModel, error) {
	// Resolve a public image alias before deciding whether this is an explicit
	// image-only request. Leaving it for inner Forward would recreate a hosted
	// tool after this adapter has already selected the local web tool.
	topModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	imageAlias := ""
	if topModel != "" && !IsGPTImageGenerationModel(topModel) {
		if mapped, image := chatGPTWebImageModelID(account, topModel); image {
			var err error
			body, err = sjson.SetBytes(body, "model", mapped)
			if err != nil {
				return nil, nil, err
			}
			imageAlias = topModel
		}
	}
	requested, explicit, err := requestedChatGPTWebImageModel(body)
	if err != nil {
		return nil, nil, err
	}
	selection, policyErr := resolveChatGPTWebImageModel(account, requested)
	if policyErr == nil && imageAlias != "" {
		selection.RequestedModel = imageAlias
	}
	if !groupAllowed {
		policyErr = chatGPTWebImageRequestError("image_generation_disabled", ImageGenerationPermissionMessage(), "tools")
	}
	if explicit && policyErr != nil {
		return nil, nil, policyErr
	}
	// The web adapter cannot reconstruct previous_response_id history. Keep
	// ordinary text continuations working without offering an unusable tool.
	if !explicit && gjson.GetBytes(body, "previous_response_id").String() != "" {
		prepared, err := prepareWebImageResponses(body, false)
		return prepared, nil, err
	}
	prepared, err := prepareWebImageResponses(body, policyErr == nil)
	if err != nil || policyErr != nil {
		return prepared, nil, err
	}
	return prepared, &selection, nil
}

// webImageResponsesPlannerAccount isolates the temporary planner policy from
// the persisted account and its cached mappings. An image-only request uses a
// fixed text driver, even when a legacy wildcard maps every name to an image.
// Ordinary text requests retain their configured text-to-text mapping.
func webImageResponsesPlannerAccount(account *Account, originalBody, preparedBody []byte) (*Account, *OpenAIImagesUpstreamError) {
	cloned := *account
	cloned.Extra = make(map[string]any, len(account.Extra)+1)
	for key, value := range account.Extra {
		cloned.Extra[key] = value
	}
	cloned.Extra[featureKeyCodexImageGenerationBridge] = false
	originalModel := gjson.GetBytes(originalBody, "model").String()
	if _, imageOnly := chatGPTWebImageModelID(&cloned, originalModel); imageOnly {
		mapping := make(map[string]any, len(cloned.GetModelMapping())+1)
		for name, target := range cloned.GetModelMapping() {
			mapping[name] = target
		}
		plannerModel := strings.TrimSpace(gjson.GetBytes(preparedBody, "model").String())
		mapping[plannerModel] = plannerModel
		cloned.Credentials = make(map[string]any, len(account.Credentials)+1)
		for key, value := range account.Credentials {
			cloned.Credentials[key] = value
		}
		cloned.Credentials["model_mapping"] = mapping
		cloned.modelMappingCacheReady = false
	}
	if err := validateWebImageResponsesPlannerRequest(&cloned, preparedBody); err != nil {
		return nil, err
	}
	return &cloned, nil
}

// Reject a malformed internal planner before transport. In particular, an
// invalid driver override or a non-GPT media mapping cannot escape to the
// native image path merely because the bridge recursion marker is present.
func validateWebImageResponsesPlannerRequest(account *Account, body []byte) *OpenAIImagesUpstreamError {
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	billingModel, upstreamModel := resolveOpenAIForwardMappedModels(account, model, false)
	if model == "" || isOpenAIImageGenerationModel(model) || isOpenAIImageGenerationModel(billingModel) || isOpenAIImageGenerationModel(upstreamModel) ||
		openAIRequestBodyHasImageGenerationDeclaration(body) || openAIJSONToolChoiceSelectsExplicitImageGeneration(gjson.GetBytes(body, "tool_choice")) {
		return chatGPTWebImageRequestError("chatgpt_web_image_invalid_planner", "The web image planner requires a text model and cannot use native image tools", "model")
	}
	return nil
}

// prepareWebImageResponses preserves non-image tools. A declaration only makes
// the tool available; generation starts only after an actual model tool call.
func prepareWebImageResponses(body []byte, imagePermission ...bool) ([]byte, error) {
	allowImage := len(imagePermission) == 0 || imagePermission[0]
	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, errors.New("responses request must be a JSON object")
	}
	if topModel, _ := request["model"].(string); allowImage && IsGPTImageGenerationModel(topModel) {
		// Normalize the driver and parameter shape before replacing the hosted
		// tool below. Otherwise inner forwarding would reinsert a native image
		// tool merely because it still saw an image-only top-level model.
		normalizeOpenAIResponsesImageOnlyModel(request)
	}
	if id, _ := request["previous_response_id"].(string); id != "" && allowImage {
		return nil, errors.New("experimental web images require explicit Responses history; previous_response_id is not yet supported")
	}
	tools, _ := request["tools"].([]any)
	filtered := make([]any, 0, len(tools)+1)
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok {
			filtered = append(filtered, raw)
			continue
		}
		name, _ := tool["name"].(string)
		if name == webImageFunctionName {
			return nil, errors.New("client tool name conflicts with internal web image tool")
		}
		if isOpenAIImageToolDeclaration(tool) {
			if allowImage {
				if err := validateWebImageToolParameters(tool); err != nil {
					return nil, err
				}
			}
			continue
		}
		filtered = append(filtered, raw)
	}
	if allowImage {
		filtered = append(filtered, map[string]any{
			"type": "function", "name": webImageFunctionName,
			"description": "Generate one raster image from a text prompt using the server's configured image provider. Call only when the user actually requests an image. Not for editing existing images or local files.",
			"strict":      true,
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{"prompt": map[string]any{"type": "string"}}, "required": []string{"prompt"}, "additionalProperties": false},
		})
	}
	request["tools"] = filtered
	choiceJSON, _ := json.Marshal(request["tool_choice"])
	if openAIJSONToolChoiceSelectsExplicitImageGeneration(gjson.ParseBytes(choiceJSON)) {
		if allowImage {
			request["tool_choice"] = map[string]any{"type": "function", "name": webImageFunctionName}
		} else {
			delete(request, "tool_choice")
		}
	}
	instructions, _ := request["instructions"].(string)
	if allowImage {
		request["instructions"] = instructions + "\nThe gateway supplies the server-side sub2api_web_image tool for text-to-image generation. When the user asks for a raster image, call that tool with the requested visual description. The server executes it; do not use shell scripts, SVG, or unavailable local image tools as a substitute. For ordinary requests do not call it. Image editing is not supported by this experimental provider."
	} else {
		request["instructions"] = instructions + "\nWeb image generation is not enabled for this account or group. Answer ordinary requests normally. If asked to generate an image, explain that image generation is unavailable; do not use unavailable image tools or shell commands as a substitute."
	}
	// The provider is selected explicitly by the server operator. store controls
	// the Responses service, while regular web conversation consent is a separate
	// account policy checked by generateWebImage; never infer it from store=false.
	if input, ok := request["input"].([]any); ok {
		for i, raw := range input {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if item["type"] == "additional_tools" {
				declared, _ := item["tools"].([]any)
				kept := make([]any, 0, len(declared))
				for _, rawTool := range declared {
					tool, _ := rawTool.(map[string]any)
					name, _ := tool["name"].(string)
					if name == webImageFunctionName {
						return nil, errors.New("client tool name conflicts with internal web image tool")
					}
					if isOpenAIImageToolDeclaration(tool) {
						if allowImage {
							if err := validateWebImageToolParameters(tool); err != nil {
								return nil, err
							}
						}
						continue
					}
					kept = append(kept, rawTool)
				}
				item["tools"] = kept
			}
			id, _ := item["id"].(string)
			if item["type"] == "image_generation_call" && strings.HasPrefix(id, webImageResponsePrefix) {
				prompt, _ := item["revised_prompt"].(string)
				input[i] = map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "An image was generated in the preceding turn. Image description: " + prompt}}}
			}
			if item["role"] == "assistant" {
				parts, _ := item["content"].([]any)
				for _, part := range parts {
					p, _ := part.(map[string]any)
					if text, ok := p["text"].(string); ok {
						p["text"] = webImagePreviewPattern.ReplaceAllString(text, "[Generated image preview omitted from text history]")
					}
				}
				if text, ok := item["content"].(string); ok {
					item["content"] = webImagePreviewPattern.ReplaceAllString(text, "[Generated image preview omitted from text history]")
				}
			}
		}
	}
	request["stream"] = true
	return json.Marshal(request)
}

func (s *OpenAIGatewayService) forwardChatGPTWebResponses(ctx context.Context, c *gin.Context, account *Account, body []byte) (*OpenAIForwardResult, error) {
	started := time.Now()
	prepared, imageModel, err := prepareWebImageResponsesForAccount(body, account, GroupAllowsImageGeneration(apiKeyGroup(getAPIKeyFromContext(c))))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"type": "invalid_request_error", "message": err.Error()}})
		return nil, err
	}
	planner, plannerErr := webImageResponsesPlannerAccount(account, body, prepared)
	if plannerErr != nil {
		writeOpenAIImagesUpstreamErrorResponse(c, plannerErr)
		return nil, plannerErr
	}
	var original map[string]any
	_ = json.Unmarshal(body, &original)
	stream, _ := original["stream"].(bool)
	var live *webImageResponseStream
	if stream {
		live = newWebImageResponseStream(ctx, c, 10*time.Second)
		defer live.Close()
	}
	writeError := func(publicErr *OpenAIImagesUpstreamError) {
		if live != nil && live.Started() {
			live.Fail(publicErr)
		} else {
			writeOpenAIImagesUpstreamErrorResponse(c, publicErr)
		}
	}
	// Reuse the existing OAuth transforms, transport and text error handling
	// with the request-scoped planner account prepared above.
	capture := newWebImageCaptureWriter(ctx)
	defer capture.Close()
	capture.stream = live
	inner := c.Copy()
	inner.Writer = capture
	inner.Request = c.Request.Clone(ctx)
	result, err := s.Forward(context.WithValue(ctx, webImageBridgeContextKey{}, true), inner, planner, prepared)
	if err != nil {
		if live != nil && live.Started() {
			live.Fail(&OpenAIImagesUpstreamError{ErrorType: "upstream_error", Code: "upstream_response_failed", Message: "The upstream Responses turn did not complete"})
			// A streamed response cannot be replayed on another account after
			// its text or tool events have already reached the client.
			return result, errors.New(err.Error())
		} else if capture.Written() {
			copyWebImageCapturedResponse(c, capture)
		}
		return result, err
	}
	if capture.overflow {
		err = errors.New("experimental web image response exceeds buffer limit")
		writeError(&OpenAIImagesUpstreamError{StatusCode: http.StatusBadGateway, ErrorType: "upstream_error", Code: "response_buffer_limit", Message: err.Error()})
		return result, err
	}
	response, err := webImageCompletedResponse(capture.body.Bytes())
	if err != nil {
		if live != nil && live.Started() {
			live.Fail(&OpenAIImagesUpstreamError{ErrorType: "upstream_error", Code: "upstream_response_incomplete", Message: err.Error()})
		} else {
			copyWebImageCapturedResponse(c, capture)
		}
		return result, err
	}
	output, _ := response["output"].([]any)
	if live != nil {
		live.RememberResponse(response)
	}
	callCount := 0
	for _, raw := range output {
		item, _ := raw.(map[string]any)
		if item["type"] == "function_call" && item["name"] == webImageFunctionName {
			callCount++
		}
	}
	if callCount == 0 {
		// Do not generate merely because the request advertises image tools.
		// Retain the original upstream event stream for normal text/tool turns.
		if result != nil {
			result.Stream = stream
		}
		if stream {
			live.Complete(response)
		} else {
			delete(response, "tools")
			c.JSON(http.StatusOK, response)
		}
		return result, nil
	}
	if callCount != 1 {
		err = errors.New("experimental web images support one image call per turn")
		writeError(chatGPTWebImageRequestError("multiple_image_calls_unsupported", err.Error(), ""))
		return result, err
	}
	for i, raw := range output {
		item, _ := raw.(map[string]any)
		if item["type"] != "function_call" || item["name"] != webImageFunctionName {
			continue
		}
		if imageModel == nil {
			err := chatGPTWebImageRequestError("chatgpt_web_image_model_not_allowed", "Image generation is not enabled for this account", "model")
			writeError(err)
			return result, err
		}
		arguments, _ := item["arguments"].(string)
		var args struct {
			Prompt string `json:"prompt"`
		}
		if json.Unmarshal([]byte(arguments), &args) != nil || strings.TrimSpace(args.Prompt) == "" {
			err = errors.New("image tool returned invalid prompt arguments")
			writeError(chatGPTWebImageRequestError("invalid_image_tool_arguments", err.Error(), ""))
			return result, err
		}
		// The planner succeeded, but the independent web provider's outcome
		// must not change Codex scheduling health or clear its 429 state.
		c.Set(chatGPTWebImageRequestContextKey, true)
		imageID := webImageResponsePrefix + uuid.NewString()
		if live != nil {
			live.BeginImage(i, imageID)
		}
		generated, generateErr := s.generateWebImageOnce(ctx, c, account, body, args.Prompt, imageModel.RequestedModel)
		if generateErr != nil {
			// No failover after a potentially accepted generation: replaying the
			// turn could generate and charge for the same image twice.
			publicErr := chatGPTWebImagePublicError(generateErr)
			writeError(publicErr)
			return result, publicErr
		}
		if len(generated.Images) != 1 {
			err = errors.New("web provider did not return exactly one verified image")
			writeError(&OpenAIImagesUpstreamError{StatusCode: http.StatusBadRequest, ErrorType: "image_generation_error", Code: "image_generation_outcome_unknown", Message: err.Error()})
			return result, err
		}
		image := generated.Images[0]
		if image.Format != "png" {
			err = errors.New("web provider returned an unsupported image format; a new generation was not submitted")
			writeError(&OpenAIImagesUpstreamError{StatusCode: http.StatusBadRequest, ErrorType: "image_generation_error", Code: "image_generation_outcome_unknown", Message: err.Error()})
			return result, err
		}
		output[i] = map[string]any{"type": "image_generation_call", "id": imageID, "status": "completed", "result": base64.StdEncoding.EncodeToString(image.Bytes), "output_format": image.Format, "revised_prompt": args.Prompt}
		if enabled, _ := account.Extra["chatgpt_web_image_markdown_preview"].(bool); enabled {
			preview, previewErr := webImageMarkdownPreview(image)
			if previewErr == nil {
				output = append(output, map[string]any{"id": "msg_web_" + uuid.NewString(), "type": "message", "role": "assistant", "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": preview, "annotations": []any{}}}})
			}
		}
		if result == nil {
			result = &OpenAIForwardResult{}
		}
		result.ImageCount = 1
		applyChatGPTWebImageJobBilling(c, result)
		result.ImageOutputSize = fmt.Sprintf("%dx%d", image.Width, image.Height)
		result.ImageOutputSizes = []string{result.ImageOutputSize}
	}
	response["output"] = output
	response["status"] = "completed"
	delete(response, "tools") // Internal function declarations are not client tools.
	result.Duration = time.Since(started)
	result.Stream = stream
	if stream {
		live.Complete(response)
	} else {
		c.Header("X-Sub2API-Image-Provider", "chatgpt_web")
		c.JSON(http.StatusOK, response)
	}
	return result, nil
}

// A bounded preview uses the client's existing Markdown renderer. The original
// full-resolution image remains in the native image item. This is explicitly
// opt-in because older Codex clients do not render that native item themselves.
func webImageMarkdownPreview(img chatgptwebimage.Image) (string, error) {
	src, _, err := image.Decode(bytes.NewReader(img.Bytes))
	if err != nil {
		return "", err
	}
	bounds := src.Bounds()
	for _, edge := range []int{768, 512, 256} {
		width, height := bounds.Dx(), bounds.Dy()
		if width > edge || height > edge {
			if width >= height {
				height = max(1, height*edge/width)
				width = edge
			} else {
				width = max(1, width*edge/height)
				height = edge
			}
		}
		thumbnail := image.NewRGBA(image.Rect(0, 0, width, height))
		draw.CatmullRom.Scale(thumbnail, thumbnail.Bounds(), src, bounds, draw.Src, nil)
		var encoded bytes.Buffer
		if err := jpeg.Encode(&encoded, thumbnail, &jpeg.Options{Quality: 82}); err != nil {
			return "", err
		}
		if encoded.Len() <= 64<<10 {
			return "![Generated image](data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes()) + ")", nil
		}
	}
	return "", errors.New("image preview exceeds text response limit")
}

func webImageCompletedResponse(body []byte) (map[string]any, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64*1024), 32*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event struct {
			Type     string         `json:"type"`
			Response map[string]any `json:"response"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event) == nil && event.Type == "response.completed" {
			return event.Response, nil
		}
	}
	return nil, errors.New("upstream did not complete the Responses turn")
}

func writeWebImageResponseSSE(c *gin.Context, response map[string]any) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	sequence := 0
	send := func(kind string, event map[string]any) {
		event["type"], event["sequence_number"] = kind, sequence
		sequence++
		encoded, _ := json.Marshal(event)
		_, _ = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", kind, encoded)
		c.Writer.Flush()
	}
	initial := make(map[string]any, len(response))
	for k, v := range response {
		initial[k] = v
	}
	initial["status"], initial["output"] = "in_progress", []any{}
	send("response.created", map[string]any{"response": initial})
	send("response.in_progress", map[string]any{"response": initial})
	output, _ := response["output"].([]any)
	for index, raw := range output {
		item, _ := raw.(map[string]any)
		added := make(map[string]any, len(item))
		for k, v := range item {
			added[k] = v
		}
		added["status"] = "in_progress"
		if item["type"] == "image_generation_call" {
			added["result"] = ""
		}
		if item["type"] == "message" {
			added["content"] = []any{}
		}
		if item["type"] == "function_call" {
			added["arguments"] = ""
		}
		send("response.output_item.added", map[string]any{"output_index": index, "item": added})
		if item["type"] == "image_generation_call" {
			for _, state := range []string{"in_progress", "generating", "completed"} {
				send("response.image_generation_call."+state, map[string]any{"output_index": index, "item_id": item["id"]})
			}
		}
		if item["type"] == "message" {
			parts, _ := item["content"].([]any)
			for contentIndex, rawPart := range parts {
				part, _ := rawPart.(map[string]any)
				if part["type"] != "output_text" {
					continue
				}
				send("response.content_part.added", map[string]any{"output_index": index, "item_id": item["id"], "content_index": contentIndex, "part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}})
				send("response.output_text.delta", map[string]any{"output_index": index, "item_id": item["id"], "content_index": contentIndex, "delta": part["text"]})
				send("response.output_text.done", map[string]any{"output_index": index, "item_id": item["id"], "content_index": contentIndex, "text": part["text"]})
				send("response.content_part.done", map[string]any{"output_index": index, "item_id": item["id"], "content_index": contentIndex, "part": part})
			}
		}
		if item["type"] == "function_call" {
			send("response.function_call_arguments.delta", map[string]any{"output_index": index, "item_id": item["id"], "delta": item["arguments"]})
			send("response.function_call_arguments.done", map[string]any{"output_index": index, "item_id": item["id"], "arguments": item["arguments"]})
		}
		send("response.output_item.done", map[string]any{"output_index": index, "item": item})
	}
	send("response.completed", map[string]any{"response": response})
}

// webImageResponseStream forwards ordinary planner events immediately. Only
// the private server tool and the planner's terminal completion are withheld,
// so an image turn can finish with the same response ID after web generation.
// The mutex also serializes keepalives with response writes; Gin's writer is
// never accessed concurrently by the heartbeat and request goroutines.
type webImageResponseStream struct {
	mu             sync.Mutex
	ctx            context.Context
	c              *gin.Context
	header         http.Header
	sequence       int
	createdAt      responsesCreatedAtStamper
	started        bool
	terminal       bool
	writeErr       error
	response       map[string]any
	line           []byte
	data           [][]byte
	privateIDs     map[string]bool
	privateIndices map[int]bool
	startedImages  map[string]bool
	done           chan struct{}
	stopped        chan struct{}
	closeOnce      sync.Once
}

func newWebImageResponseStream(ctx context.Context, c *gin.Context, interval time.Duration) *webImageResponseStream {
	w := &webImageResponseStream{ctx: ctx, c: c, privateIDs: make(map[string]bool), privateIndices: make(map[int]bool), startedImages: make(map[string]bool), done: make(chan struct{}), stopped: make(chan struct{})}
	go func() {
		defer close(w.stopped)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				w.mu.Lock()
				if w.started && !w.terminal && w.writeErr == nil && w.ctx.Err() == nil {
					_, w.writeErr = w.c.Writer.WriteString(": keepalive\n\n")
					if w.writeErr == nil {
						w.c.Writer.Flush()
					}
				}
				w.mu.Unlock()
			case <-ctx.Done():
				return
			case <-w.done:
				return
			}
		}
	}()
	return w
}

func (w *webImageResponseStream) Close() {
	w.closeOnce.Do(func() { close(w.done) })
	<-w.stopped
}

func (w *webImageResponseStream) Started() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.started
}

func webImagePublicResponse(response map[string]any) map[string]any {
	public := make(map[string]any, len(response))
	for key, value := range response {
		if key != "tools" {
			public[key] = value
		}
	}
	return public
}

func (w *webImageResponseStream) RememberResponse(response map[string]any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.response = webImagePublicResponse(response)
	w.response["status"], w.response["output"] = "in_progress", []any{}
}

// Feed accepts arbitrary write boundaries, including a JSON event split over
// multiple writes. Multiline SSE data is joined only at its terminating blank.
func (w *webImageResponseStream) Feed(p []byte, header http.Header) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if w.writeErr != nil {
		return w.writeErr
	}
	if w.header == nil {
		w.header = header.Clone()
	}
	w.line = append(w.line, p...)
	for {
		end := bytes.IndexByte(w.line, '\n')
		if end < 0 {
			break
		}
		line := bytes.TrimSuffix(w.line[:end], []byte{'\r'})
		if len(line) == 0 {
			if len(w.data) > 0 {
				var event map[string]any
				if json.Unmarshal(bytes.Join(w.data, []byte{'\n'}), &event) == nil {
					w.forwardEventLocked(event)
				}
				w.data = nil
			}
		} else if bytes.HasPrefix(line, []byte("data:")) {
			w.data = append(w.data, bytes.Clone(bytes.TrimSpace(line[len("data:"):])))
		}
		w.line = w.line[end+1:]
	}
	return w.writeErr
}

func (w *webImageResponseStream) forwardEventLocked(event map[string]any) {
	kind, _ := event["type"].(string)
	if kind == "" || kind == "response.completed" || w.terminal {
		return
	}
	item, _ := event["item"].(map[string]any)
	if item["type"] == "function_call" && item["name"] == webImageFunctionName {
		id, _ := item["id"].(string)
		if id != "" {
			w.privateIDs[id] = true
		}
		if index, ok := event["output_index"].(float64); ok {
			w.privateIndices[int(index)] = true
		}
		return
	}
	if id, _ := event["item_id"].(string); w.privateIDs[id] {
		return
	}
	if index, ok := event["output_index"].(float64); ok && w.privateIndices[int(index)] {
		return
	}
	if response, ok := event["response"].(map[string]any); ok {
		response = webImagePublicResponse(response)
		event["response"] = response
		w.response = response
	}
	w.sendLocked(kind, event)
	if kind == "response.failed" || kind == "response.incomplete" {
		w.terminal = true
	}
}

func (w *webImageResponseStream) sendLocked(kind string, event map[string]any) {
	if w.terminal || w.writeErr != nil || w.ctx.Err() != nil {
		return
	}
	if !w.started {
		for key, values := range w.header {
			if !strings.EqualFold(key, "Content-Length") && !strings.EqualFold(key, "Content-Encoding") {
				w.c.Writer.Header()[key] = append([]string(nil), values...)
			}
		}
		w.c.Header("Content-Type", "text/event-stream")
		w.c.Header("Cache-Control", "no-cache")
		w.c.Header("X-Accel-Buffering", "no")
		w.started = true
	}
	event["type"], event["sequence_number"] = kind, w.sequence
	w.sequence++
	encoded, err := json.Marshal(event)
	if err != nil {
		w.writeErr = err
		return
	}
	encoded, _ = w.createdAt.Stamp(encoded, kind)
	_, w.writeErr = fmt.Fprintf(w.c.Writer, "event: %s\ndata: %s\n\n", kind, encoded)
	if w.writeErr == nil {
		w.c.Writer.Flush()
	}
}

func (w *webImageResponseStream) ensureStartedLocked() {
	if w.started {
		return
	}
	initial := webImagePublicResponse(w.response)
	initial["status"], initial["output"] = "in_progress", []any{}
	w.sendLocked("response.created", map[string]any{"response": initial})
	w.sendLocked("response.in_progress", map[string]any{"response": initial})
}

func (w *webImageResponseStream) BeginImage(index int, id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ensureStartedLocked()
	w.startedImages[id] = true
	w.sendLocked("response.output_item.added", map[string]any{"output_index": index, "item": map[string]any{"type": "image_generation_call", "id": id, "status": "in_progress", "result": ""}})
	for _, state := range []string{"in_progress", "generating"} {
		w.sendLocked("response.image_generation_call."+state, map[string]any{"output_index": index, "item_id": id})
	}
}

func (w *webImageResponseStream) Complete(response map[string]any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ensureStartedLocked()
	output, _ := response["output"].([]any)
	for index, raw := range output {
		item, _ := raw.(map[string]any)
		id, _ := item["id"].(string)
		if item["type"] == "image_generation_call" && strings.HasPrefix(id, webImageResponsePrefix) {
			if !w.startedImages[id] {
				w.sendLocked("response.output_item.added", map[string]any{"output_index": index, "item": map[string]any{"type": "image_generation_call", "id": id, "status": "in_progress", "result": ""}})
			}
			w.sendLocked("response.image_generation_call.completed", map[string]any{"output_index": index, "item_id": id})
			w.sendLocked("response.output_item.done", map[string]any{"output_index": index, "item": item})
		} else if item["type"] == "message" && strings.HasPrefix(id, "msg_web_") {
			w.sendLocked("response.output_item.added", map[string]any{"output_index": index, "item": map[string]any{"type": "message", "id": id, "role": "assistant", "status": "in_progress", "content": []any{}}})
			parts, _ := item["content"].([]any)
			for contentIndex, rawPart := range parts {
				part, _ := rawPart.(map[string]any)
				if part["type"] != "output_text" {
					continue
				}
				w.sendLocked("response.content_part.added", map[string]any{"output_index": index, "item_id": id, "content_index": contentIndex, "part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}})
				w.sendLocked("response.output_text.delta", map[string]any{"output_index": index, "item_id": id, "content_index": contentIndex, "delta": part["text"]})
				w.sendLocked("response.output_text.done", map[string]any{"output_index": index, "item_id": id, "content_index": contentIndex, "text": part["text"]})
				w.sendLocked("response.content_part.done", map[string]any{"output_index": index, "item_id": id, "content_index": contentIndex, "part": part})
			}
			w.sendLocked("response.output_item.done", map[string]any{"output_index": index, "item": item})
		}
	}
	w.sendLocked("response.completed", map[string]any{"response": webImagePublicResponse(response)})
	w.terminal = true
}

func (w *webImageResponseStream) Fail(publicErr *OpenAIImagesUpstreamError) {
	w.mu.Lock()
	defer w.mu.Unlock()
	response := webImagePublicResponse(w.response)
	response["status"], response["output"] = "failed", []any{}
	response["error"] = map[string]any{"type": publicErr.clientErrorType(), "code": publicErr.Code, "message": publicErr.clientMessage()}
	w.sendLocked("response.failed", map[string]any{"response": response})
	w.terminal = true
}

type webImageCaptureWriter struct {
	header            http.Header
	body              bytes.Buffer
	status            int
	written, overflow bool
	closed            chan bool
	stopNotify        func() bool
	stream            *webImageResponseStream
}

func newWebImageCaptureWriter(ctx context.Context) *webImageCaptureWriter {
	w := &webImageCaptureWriter{header: make(http.Header), status: http.StatusOK, closed: make(chan bool)}
	w.stopNotify = context.AfterFunc(ctx, func() { close(w.closed) })
	return w
}
func (w *webImageCaptureWriter) Close()              { w.stopNotify() }
func (w *webImageCaptureWriter) Header() http.Header { return w.header }
func (w *webImageCaptureWriter) WriteHeader(status int) {
	if !w.written {
		w.status = status
		w.written = true
	}
}
func (w *webImageCaptureWriter) Write(p []byte) (int, error) {
	w.WriteHeader(w.status)
	if w.body.Len()+len(p) > 32*1024*1024 {
		w.overflow = true
		return 0, errors.New("response capture size limit")
	}
	n, err := w.body.Write(p)
	if err == nil && w.stream != nil && w.status == http.StatusOK {
		err = w.stream.Feed(p, w.header)
	}
	return n, err
}
func (w *webImageCaptureWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
func (w *webImageCaptureWriter) Status() int                       { return w.status }
func (w *webImageCaptureWriter) Size() int {
	if !w.written {
		return -1
	}
	return w.body.Len()
}
func (w *webImageCaptureWriter) Written() bool   { return w.written }
func (w *webImageCaptureWriter) WriteHeaderNow() { w.WriteHeader(w.status) }
func (w *webImageCaptureWriter) Flush()          { w.WriteHeaderNow() }
func (w *webImageCaptureWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("buffered HTTP bridge cannot hijack")
}
func (w *webImageCaptureWriter) CloseNotify() <-chan bool { return w.closed }
func (w *webImageCaptureWriter) Pusher() http.Pusher      { return nil }

func copyWebImageCapturedResponse(c *gin.Context, capture *webImageCaptureWriter) {
	for key, values := range capture.header {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		c.Writer.Header()[key] = append([]string(nil), values...)
	}
	c.Status(capture.status)
	_, _ = c.Writer.Write(capture.body.Bytes())
}
