package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
	"github.com/Wei-Shaw/sub2api/internal/pkg/gemini"
	"github.com/tidwall/gjson"
)

const adobeGeminiMaxCandidateCount = adobeMaxBatchCount

var adobeGeminiGenerationMethods = []string{"generateContent", "streamGenerateContent"}

// ParseAdobeGeminiImageRequest 把 Gemini generateContent 生图请求翻译成
// OpenAI Images 形状 + AdobeImageCall 的比例/档位。模型名来自 URL，不读 body.model。
// 模型是否可用由调用方在渠道映射后用 ValidateAdobeGeminiModel 判定，这里不查目录。
func ParseAdobeGeminiImageRequest(model string, body []byte) (*OpenAIImagesRequest, *AdobeImageCall, error) {
	model = stripGeminiModelsPrefix(model)
	if model == "" {
		return nil, nil, fmt.Errorf("model is required")
	}
	if len(body) == 0 {
		return nil, nil, fmt.Errorf("request body is empty")
	}
	if !gjson.ValidBytes(body) {
		return nil, nil, fmt.Errorf("failed to parse request body")
	}

	if tools := gjson.GetBytes(body, "tools"); tools.Exists() {
		if (tools.IsArray() && len(tools.Array()) > 0) || tools.IsObject() {
			return nil, nil, fmt.Errorf("Adobe generateContent does not support tools")
		}
	}

	if err := validateAdobeGeminiResponseModalities(gjson.GetBytes(body, "generationConfig.responseModalities")); err != nil {
		return nil, nil, err
	}

	var texts []string
	texts = append(texts, collectGeminiPartTexts(gjson.GetBytes(body, "systemInstruction"))...)
	texts = append(texts, collectGeminiPartTexts(gjson.GetBytes(body, "system_instruction"))...)

	var uploads []OpenAIImagesUpload
	var imageURLs []string
	contents := gjson.GetBytes(body, "contents")
	if contents.Exists() && !contents.IsArray() {
		return nil, nil, fmt.Errorf("contents must be an array")
	}
	for _, content := range contents.Array() {
		role := strings.ToLower(strings.TrimSpace(content.Get("role").String()))
		skipText := role == "model" || role == "assistant"
		parts := content.Get("parts")
		if !parts.Exists() {
			continue
		}
		if !parts.IsArray() {
			return nil, nil, fmt.Errorf("contents[].parts must be an array")
		}
		for _, part := range parts.Array() {
			if part.Get("functionCall").Exists() || part.Get("function_call").Exists() {
				return nil, nil, fmt.Errorf("Adobe generateContent does not support function calls")
			}
			if !skipText {
				if text := strings.TrimSpace(part.Get("text").String()); text != "" {
					texts = append(texts, text)
				}
			}
			upload, imageURL, err := parseAdobeGeminiImagePart(part)
			if err != nil {
				return nil, nil, err
			}
			if upload != nil {
				uploads = append(uploads, *upload)
			}
			if imageURL != "" {
				imageURLs = append(imageURLs, imageURL)
			}
		}
	}

	prompt := strings.TrimSpace(strings.Join(texts, "\n"))
	if prompt == "" && len(uploads) == 0 && len(imageURLs) == 0 {
		return nil, nil, fmt.Errorf("Adobe generateContent requires a text prompt or a reference image")
	}

	n := 1
	if count := gjson.GetBytes(body, "generationConfig.candidateCount"); count.Exists() {
		if count.Type != gjson.Number {
			return nil, nil, fmt.Errorf("invalid candidateCount field type")
		}
		n = int(count.Int())
		if n <= 0 {
			return nil, nil, fmt.Errorf("candidateCount must be greater than 0")
		}
		if n > adobeGeminiMaxCandidateCount {
			return nil, nil, fmt.Errorf("Adobe generateContent candidateCount must be between 1 and %d, got %d", adobeGeminiMaxCandidateCount, n)
		}
	}

	aspectRatio := strings.TrimSpace(gjson.GetBytes(body, "generationConfig.imageConfig.aspectRatio").String())
	imageSize, err := normalizeAdobeGeminiImageSize(gjson.GetBytes(body, "generationConfig.imageConfig.imageSize").String())
	if err != nil {
		return nil, nil, err
	}
	outputFormat, err := adobeGeminiOutputFormat(gjson.GetBytes(body, "generationConfig.responseMimeType").String())
	if err != nil {
		return nil, nil, err
	}

	// size 同时喂给中转号请求体和非 banana 族的像素决策，所以必须带上比例；
	// 只给比例时按 Gemini 默认 1K 出尺寸（与 banana 省略 size 时的 1K 一致）。
	size := ""
	switch {
	case aspectRatio != "":
		resolution := imageSize
		if resolution == "" {
			resolution = string(adobe.Resolution1K)
		}
		pixels, ok := adobe.SizeFromRatio(adobe.OutputResolution(resolution), aspectRatio)
		if !ok {
			return nil, nil, fmt.Errorf("unsupported aspectRatio %q", aspectRatio)
		}
		size = pixels.String()
	case imageSize != "":
		size = adobe.SquareFromResolution(adobe.OutputResolution(imageSize)).String()
	}

	endpoint := openAIImagesGenerationsEndpoint
	if len(uploads) > 0 || len(imageURLs) > 0 {
		endpoint = openAIImagesEditsEndpoint
	}

	sizeTier := imageSize
	if sizeTier == "" {
		sizeTier = normalizeOpenAIImageSizeTier(size)
	}

	req := &OpenAIImagesRequest{
		Endpoint:       endpoint,
		ContentType:    "application/json",
		Model:          model,
		ExplicitModel:  true,
		Prompt:         prompt,
		N:              n,
		Size:           size,
		ExplicitSize:   size != "",
		SizeTier:       sizeTier,
		OutputFormat:   outputFormat,
		ResponseFormat: "b64_json",
		InputImageURLs: imageURLs,
		Uploads:        uploads,
	}
	call := NewAdobeImageCall(req, "")
	call.AspectRatio = aspectRatio
	call.ImageSize = imageSize
	return req, call, nil
}

func validateAdobeGeminiResponseModalities(modalities gjson.Result) error {
	if !modalities.Exists() {
		return nil
	}
	if !modalities.IsArray() {
		return fmt.Errorf("generationConfig.responseModalities must be an array")
	}
	for _, item := range modalities.Array() {
		if strings.EqualFold(strings.TrimSpace(item.String()), "IMAGE") {
			return nil
		}
	}
	return fmt.Errorf("Adobe generateContent requires IMAGE in responseModalities")
}

func collectGeminiPartTexts(node gjson.Result) []string {
	if !node.Exists() {
		return nil
	}
	var texts []string
	add := func(part gjson.Result) {
		if text := strings.TrimSpace(part.Get("text").String()); text != "" {
			texts = append(texts, text)
		}
	}
	if parts := node.Get("parts"); parts.IsArray() {
		for _, part := range parts.Array() {
			add(part)
		}
		return texts
	}
	if node.IsArray() {
		for _, item := range node.Array() {
			texts = append(texts, collectGeminiPartTexts(item)...)
		}
	}
	return texts
}

func parseAdobeGeminiImagePart(part gjson.Result) (*OpenAIImagesUpload, string, error) {
	inline := part.Get("inlineData")
	if !inline.Exists() {
		inline = part.Get("inline_data")
	}
	if inline.Exists() {
		if !inline.IsObject() {
			return nil, "", fmt.Errorf("inlineData must be an object")
		}
		mimeType := firstNonEmptyString(inline.Get("mimeType").String(), inline.Get("mime_type").String())
		data, err := decodeAdobeGeminiInlineImage(inline.Get("data").String())
		if err != nil {
			return nil, "", err
		}
		if mimeType == "" {
			mimeType = detectedImageContentType(data)
		}
		if mimeType == "" {
			mimeType = "image/png"
		}
		if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
			return nil, "", fmt.Errorf("inlineData mimeType must be an image, got %q", mimeType)
		}
		return &OpenAIImagesUpload{ContentType: mimeType, Data: data}, "", nil
	}

	file := part.Get("fileData")
	if !file.Exists() {
		file = part.Get("file_data")
	}
	if !file.Exists() {
		return nil, "", nil
	}
	if !file.IsObject() {
		return nil, "", fmt.Errorf("fileData must be an object")
	}
	uri := strings.TrimSpace(firstNonEmptyString(file.Get("fileUri").String(), file.Get("file_uri").String()))
	if uri == "" {
		return nil, "", fmt.Errorf("fileData.fileUri is required")
	}
	if strings.HasPrefix(strings.ToLower(uri), "https://") {
		return nil, uri, nil
	}
	return nil, "", fmt.Errorf("Adobe generateContent only accepts https fileData.fileUri, got %q", uri)
}

func decodeAdobeGeminiInlineImage(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("inlineData.data is required")
	}
	decoded, err := decodeAdobeGeminiBase64(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid inlineData.data: %w", err)
	}
	if len(decoded) == 0 {
		return nil, fmt.Errorf("inlineData.data is empty")
	}
	return decoded, nil
}

func decodeAdobeGeminiBase64(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil {
		return decoded, nil
	}
	return base64.RawStdEncoding.DecodeString(raw)
}

func normalizeAdobeGeminiImageSize(raw string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(raw))
	switch normalized {
	case "":
		return "", nil
	case "1K", "2K", "4K":
		return normalized, nil
	default:
		return "", fmt.Errorf("unsupported imageSize %q", strings.TrimSpace(raw))
	}
}

func adobeGeminiOutputFormat(mimeType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "":
		return "", nil
	case "image/png":
		return adobeOutputFormatPNG, nil
	case "image/jpeg", "image/jpg":
		return adobeOutputFormatJPEG, nil
	default:
		return "", fmt.Errorf("Adobe generateContent does not support responseMimeType %q", strings.TrimSpace(mimeType))
	}
}

func stripGeminiModelsPrefix(model string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(model), "models/"))
}

// AdobeGeminiModels 是 Adobe 分组 /v1beta/models 的静态列表。
func AdobeGeminiModels() []gemini.Model {
	ids := adobe.ImageModelIDs()
	out := make([]gemini.Model, 0, len(ids))
	for _, id := range ids {
		out = append(out, AdobeGeminiModel(id))
	}
	return out
}

// AdobeGeminiModel 返回单条 Gemini 形状的 Adobe 出图模型元数据。
func AdobeGeminiModel(model string) gemini.Model {
	id := stripGeminiModelsPrefix(model)
	display := id
	if label, ok := adobe.DisplayLabel(id); ok && strings.TrimSpace(label) != "" {
		display = label
	}
	name := id
	if name != "" && !strings.HasPrefix(name, "models/") {
		name = "models/" + name
	}
	return gemini.Model{
		Name:                       name,
		DisplayName:                display,
		SupportedGenerationMethods: append([]string(nil), adobeGeminiGenerationMethods...),
	}
}

// LookupAdobeGeminiModel 查找 Adobe 出图模型的 Gemini 元数据；未知模型返回 false。
func LookupAdobeGeminiModel(model string) (gemini.Model, bool) {
	id := stripGeminiModelsPrefix(model)
	if id == "" || !IsAdobeGeminiImageModel(id) {
		return gemini.Model{}, false
	}
	return AdobeGeminiModel(id), true
}

// ValidateAdobeGeminiModel 在渠道映射之后判定模型能否走 Adobe generateContent。
// 目录内的出图名直接放行；能被识别为其他平台的名字（gemini-* / claude-* 等）拒绝；
// 其余视为运维配置的别名，交给选号与 ResolveImage 决定。
func ValidateAdobeGeminiModel(model string) error {
	id := stripGeminiModelsPrefix(model)
	if id == "" {
		return fmt.Errorf("model is required")
	}
	if IsAdobeGeminiImageModel(id) {
		return nil
	}
	if platform, ok := DetectModelPlatform(id); ok && platform != PlatformAdobe {
		return fmt.Errorf("Adobe generateContent requires an image model, got %q", id)
	}
	return nil
}

// IsAdobeGeminiImageModel 判断 URL 模型是否为 Adobe 出图名（含 models/ 前缀与内部 firefly-*）。
func IsAdobeGeminiImageModel(model string) bool {
	id := stripGeminiModelsPrefix(model)
	return adobe.IsExternalImageModelID(id) || adobe.IsImageModelID(id)
}

// AdobeGeminiImage 是 Gemini 信封里的一张图：Data 非空时编成 inlineData，否则编成 fileData。
type AdobeGeminiImage struct {
	Data     []byte
	MimeType string
	FileURI  string
}

// AdobeGeminiImagesFromBytes 把原生出图的图片字节包成 AdobeGeminiImage。
func AdobeGeminiImagesFromBytes(images [][]byte) []AdobeGeminiImage {
	out := make([]AdobeGeminiImage, 0, len(images))
	for _, image := range images {
		out = append(out, AdobeGeminiImage{Data: image})
	}
	return out
}

// BuildGeminiGenerateContentResponse 把图片打成 Gemini generateContent 信封：
// 按 candidateCount 语义每张图一个 candidate。
func BuildGeminiGenerateContentResponse(model string, images []AdobeGeminiImage) ([]byte, error) {
	if len(images) == 0 {
		return nil, fmt.Errorf("adobe returned an empty image")
	}
	candidates := make([]map[string]any, 0, len(images))
	for index, image := range images {
		part, err := adobeGeminiImagePart(image)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, map[string]any{
			"content": map[string]any{
				"role":  "model",
				"parts": []map[string]any{part},
			},
			"finishReason": "STOP",
			"index":        index,
		})
	}
	payload := map[string]any{
		"candidates": candidates,
	}
	if version := stripGeminiModelsPrefix(model); version != "" {
		payload["modelVersion"] = version
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode gemini image response: %w", err)
	}
	return body, nil
}

func adobeGeminiImagePart(image AdobeGeminiImage) (map[string]any, error) {
	if len(image.Data) > 0 {
		mimeType := detectedImageContentType(image.Data)
		if mimeType == "" {
			mimeType = "image/png"
		}
		return map[string]any{
			"inlineData": map[string]any{
				"mimeType": mimeType,
				"data":     base64.StdEncoding.EncodeToString(image.Data),
			},
		}, nil
	}
	uri := strings.TrimSpace(image.FileURI)
	if uri == "" {
		return nil, fmt.Errorf("adobe returned an empty image")
	}
	fileData := map[string]any{"fileUri": uri}
	if mimeType := strings.TrimSpace(image.MimeType); mimeType != "" {
		fileData["mimeType"] = mimeType
	}
	return map[string]any{"fileData": fileData}, nil
}

// EncodeGeminiGenerateContentSSE 把完整 generateContent JSON 打成一帧 SSE。
func EncodeGeminiGenerateContentSSE(body []byte) []byte {
	var buf bytes.Buffer
	buf.Grow(len(body) + 8)
	buf.WriteString("data: ")
	buf.Write(body)
	buf.WriteString("\n\n")
	return buf.Bytes()
}

// MarshalAdobeGeminiRelayImagesBody 把解析后的 Gemini 生图请求编成中转号要的 OpenAI Images JSON。
func MarshalAdobeGeminiRelayImagesBody(req *OpenAIImagesRequest) ([]byte, error) {
	if req == nil {
		return nil, fmt.Errorf("images request is required")
	}
	payload := map[string]any{
		"model":           req.Model,
		"prompt":          req.Prompt,
		"n":               adobeImageRequestCount(req.N),
		"response_format": "b64_json",
	}
	if strings.TrimSpace(req.Size) != "" {
		payload["size"] = req.Size
	}
	if strings.TrimSpace(req.OutputFormat) != "" {
		payload["output_format"] = req.OutputFormat
	}
	if req.IsEdits() {
		images := make([]map[string]string, 0, len(req.Uploads)+len(req.InputImageURLs))
		for _, upload := range req.Uploads {
			if len(upload.Data) == 0 {
				continue
			}
			contentType := strings.TrimSpace(upload.ContentType)
			if contentType == "" {
				contentType = detectedImageContentType(upload.Data)
			}
			if contentType == "" {
				contentType = "image/png"
			}
			images = append(images, map[string]string{
				"image_url": fmt.Sprintf("data:%s;base64,%s", contentType, base64.StdEncoding.EncodeToString(upload.Data)),
			})
		}
		for _, rawURL := range req.InputImageURLs {
			if imageURL := strings.TrimSpace(rawURL); imageURL != "" {
				images = append(images, map[string]string{"image_url": imageURL})
			}
		}
		if len(images) == 0 {
			return nil, fmt.Errorf("images[].image_url is required")
		}
		payload["images"] = images
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode adobe gemini relay body: %w", err)
	}
	return body, nil
}

// OpenAIImagesJSONToGeminiImages 从中转号的 OpenAI Images JSON 抽出 Gemini 信封要的图片。
//
// b64_json / data URL 解成字节；只剩 http(s) url 的项降级为 fileData，不再二次下载：
// 缓冲模式下 backfillOpenAIImagesB64JSON 已经带账号代理与出站防护回填过一次，
// 走到这里说明网关拿不到，交给客户端自取。两者都没有的项丢弃；一张都没有时报错。
func OpenAIImagesJSONToGeminiImages(body []byte, outputFormat string) ([]AdobeGeminiImage, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("relay images response is empty")
	}
	data := gjson.GetBytes(body, "data")
	if !data.IsArray() || len(data.Array()) == 0 {
		return nil, fmt.Errorf("relay images response is missing data")
	}
	fileMimeType := adobeGeminiOutputMimeType(outputFormat)
	items := data.Array()
	images := make([]AdobeGeminiImage, 0, len(items))
	for _, item := range items {
		if image, ok := decodeOpenAIImagesJSONItem(item, fileMimeType); ok {
			images = append(images, image)
		}
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("relay images response has no usable image")
	}
	return images, nil
}

func decodeOpenAIImagesJSONItem(item gjson.Result, fileMimeType string) (AdobeGeminiImage, bool) {
	if raw := strings.TrimSpace(item.Get("b64_json").String()); raw != "" {
		if decoded, err := decodeAdobeGeminiBase64(trimAdobeGeminiDataURL(raw)); err == nil && len(decoded) > 0 {
			return AdobeGeminiImage{Data: decoded}, true
		}
	}
	rawURL := strings.TrimSpace(item.Get("url").String())
	lowerURL := strings.ToLower(rawURL)
	switch {
	case strings.HasPrefix(lowerURL, "data:"):
		if decoded, err := decodeAdobeGeminiBase64(trimAdobeGeminiDataURL(rawURL)); err == nil && len(decoded) > 0 {
			return AdobeGeminiImage{Data: decoded}, true
		}
	case strings.HasPrefix(lowerURL, "https://"), strings.HasPrefix(lowerURL, "http://"):
		return AdobeGeminiImage{FileURI: rawURL, MimeType: fileMimeType}, true
	}
	return AdobeGeminiImage{}, false
}

func adobeGeminiOutputMimeType(outputFormat string) string {
	switch strings.ToLower(strings.TrimSpace(outputFormat)) {
	case adobeOutputFormatPNG:
		return "image/png"
	case adobeOutputFormatJPEG, "jpg":
		return "image/jpeg"
	default:
		return ""
	}
}

func trimAdobeGeminiDataURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if idx := strings.Index(raw, ","); idx >= 0 && strings.HasPrefix(strings.ToLower(raw), "data:") {
		return strings.TrimSpace(raw[idx+1:])
	}
	return raw
}
