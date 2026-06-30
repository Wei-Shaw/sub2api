// Package fal contains the data structures and bidirectional transformers
// between the OpenAI Images protocol and the fal.ai queue protocol for the
// gpt-image-2 family of models.
//
// fal API reference:
//   - text-to-image:  openai/gpt-image-2
//   - image-to-image: openai/gpt-image-2/edit
//
// Queue (async) protocol:
//
//	submit  POST https://queue.fal.run/{model}                 -> SubmitResponse
//	status  GET  https://queue.fal.run/{model}/requests/{id}/status -> StatusResponse
//	result  GET  https://queue.fal.run/{model}/requests/{id}   -> Response
//	cancel  PUT  https://queue.fal.run/{model}/requests/{id}/cancel
//	sync    POST https://fal.run/{model}                        -> Response (blocking)
//
// Auth header: Authorization: Key {FAL_KEY}
package fal

// Queue status values returned by the fal status endpoint.
const (
	StatusInQueue    = "IN_QUEUE"
	StatusInProgress = "IN_PROGRESS"
	StatusCompleted  = "COMPLETED"
)

// Named image_size enums supported by fal.
const (
	SizeSquareHD     = "square_hd"
	SizeSquare       = "square"
	SizePortrait43   = "portrait_4_3"
	SizePortrait169  = "portrait_16_9"
	SizeLandscape43  = "landscape_4_3"
	SizeLandscape169 = "landscape_16_9"
	SizeAuto         = "auto"
	DefaultImageSize = SizeLandscape43
)

// Quality values supported by fal.
const (
	QualityAuto   = "auto"
	QualityLow    = "low"
	QualityMedium = "medium"
	QualityHigh   = "high"
)

// ImageSizeDims is the explicit {width,height} form of image_size.
type ImageSizeDims struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Request is the fal request body for gpt-image-2 generations and edits.
//
// ImageSize is left as any because fal accepts either a string enum
// (e.g. "landscape_4_3"/"auto") or an explicit {width,height} object.
type Request struct {
	Prompt       string   `json:"prompt"`
	ImageURLs    []string `json:"image_urls,omitempty"` // required for /edit
	MaskURL      string   `json:"mask_url,omitempty"`
	ImageSize    any      `json:"image_size,omitempty"`
	Quality      string   `json:"quality,omitempty"`
	NumImages    int      `json:"num_images,omitempty"`
	OutputFormat string   `json:"output_format,omitempty"`
	SyncMode     bool     `json:"sync_mode,omitempty"`
}

// Image is a single image entry in a fal result.
type Image struct {
	URL         string `json:"url"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	FileName    string `json:"file_name,omitempty"`
	FileSize    int64  `json:"file_size,omitempty"`
}

// Response is the fal result body (also returned by the sync endpoint).
type Response struct {
	Images []Image `json:"images"`
}

// SubmitResponse is returned by the queue submit endpoint.
type SubmitResponse struct {
	RequestID   string `json:"request_id"`
	Status      string `json:"status"`
	StatusURL   string `json:"status_url"`
	ResponseURL string `json:"response_url"`
	CancelURL   string `json:"cancel_url"`
}

// StatusResponse is returned by the queue status endpoint.
type StatusResponse struct {
	Status        string `json:"status"`
	RequestID     string `json:"request_id,omitempty"`
	QueuePosition int    `json:"queue_position,omitempty"`
	ResponseURL   string `json:"response_url,omitempty"`
}

// IsTerminal reports whether the status represents a completed (terminal) state.
func (s StatusResponse) IsTerminal() bool {
	return s.Status == StatusCompleted
}

// ImageGenInput is a protocol-neutral description of an image request used to
// build a fal Request. It mirrors the meaningful fields of an OpenAI Images
// request without importing the service package (avoids an import cycle).
type ImageGenInput struct {
	Prompt       string
	Size         string // OpenAI-style: "1024x1024", "auto", or ""
	Quality      string // OpenAI-style: standard/hd or low/medium/high/auto
	N            int
	OutputFormat string
	ImageURLs    []string // edits: input images
	MaskURL      string
	IsEdit       bool
	SyncMode     bool
}

// OpenAIImageData is a single entry of an OpenAI Images response data array.
type OpenAIImageData struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// OpenAIImagesUsageTokenDetails 复刻 gpt-image usage 的 token 明细。
type OpenAIImagesUsageTokenDetails struct {
	ImageTokens int64 `json:"image_tokens"`
	TextTokens  int64 `json:"text_tokens"`
}

// OpenAIImagesUsage 复刻 gpt-image 的 usage 对象。
type OpenAIImagesUsage struct {
	InputTokens         int64                          `json:"input_tokens"`
	InputTokensDetails  *OpenAIImagesUsageTokenDetails `json:"input_tokens_details,omitempty"`
	OutputTokens        int64                          `json:"output_tokens"`
	OutputTokensDetails *OpenAIImagesUsageTokenDetails `json:"output_tokens_details,omitempty"`
	TotalTokens         int64                          `json:"total_tokens"`
}

// OpenAIImagesResponse is the OpenAI Images response envelope.
type OpenAIImagesResponse struct {
	Created      int64              `json:"created"`
	Data         []OpenAIImageData  `json:"data"`
	Background   string             `json:"background,omitempty"`
	OutputFormat string             `json:"output_format,omitempty"`
	Quality      string             `json:"quality,omitempty"`
	Size         string             `json:"size,omitempty"`
	Model        string             `json:"model,omitempty"`
	Usage        *OpenAIImagesUsage `json:"usage,omitempty"`
}

// ModelEntry is a single entry of the fal platform models list
// (GET https://api.fal.ai/v1/models). The model identifier is endpoint_id.
type ModelEntry struct {
	EndpointID string `json:"endpoint_id"`
}

// ModelsResponse is the fal platform models list response envelope.
// Pagination follows next_cursor / has_more.
type ModelsResponse struct {
	Models     []ModelEntry `json:"models"`
	NextCursor string       `json:"next_cursor"`
	HasMore    bool         `json:"has_more"`
}
