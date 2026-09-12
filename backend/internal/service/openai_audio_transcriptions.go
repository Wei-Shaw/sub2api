package service

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/audioduration"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	// OpenAIAudioTranscriptionMaxBodySize bounds the whole multipart upload:
	// OpenAI's 25 MB file limit plus form fields and boundaries.
	OpenAIAudioTranscriptionMaxBodySize  = 32 << 20
	openAIAudioTranscriptionMaxFileSize  = 25 << 20
	openAIAudioTranscriptionMaxFieldSize = 64 << 10
	// A served transcription never bills zero, even for sub-second clips.
	openAIAudioTranscriptionMinBilledSeconds = 1
	openAIAudioTranscriptionBillingMode      = "stt"
	openAIAudioTranscriptionRequestIDPrefix  = "openai_audio:"
)

// OpenAIAudioTranscriptionRequest is a parsed POST /v1/audio/transcriptions
// multipart request. Body keeps the original upload so API key upstreams
// receive it verbatim.
type OpenAIAudioTranscriptionRequest struct {
	Model           string
	FileName        string
	FileContentType string
	Audio           []byte
	Prompt          string
	Language        string
	ResponseFormat  string
	Body            []byte
	ContentType     string
	DurationSeconds float64
	DurationExact   bool
}

// OpenAIAudioTranscriptionRequestError is a client-side validation failure
// with the HTTP status it should be reported as.
type OpenAIAudioTranscriptionRequestError struct {
	StatusCode int
	Message    string
}

func (e *OpenAIAudioTranscriptionRequestError) Error() string {
	return e.Message
}

func openAIAudioTranscriptionBadRequest(message string) error {
	return &OpenAIAudioTranscriptionRequestError{StatusCode: http.StatusBadRequest, Message: message}
}

// ParseOpenAIAudioTranscriptionRequest validates an OpenAI-style multipart
// transcription upload. Unknown form fields are ignored so clients that send
// vendor extensions (hotwords, temperature, timestamp granularities) still work.
func ParseOpenAIAudioTranscriptionRequest(contentType string, body []byte) (*OpenAIAudioTranscriptionRequest, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return nil, openAIAudioTranscriptionBadRequest("multipart/form-data is required")
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return nil, openAIAudioTranscriptionBadRequest("multipart boundary is required")
	}

	parsed := &OpenAIAudioTranscriptionRequest{Body: body, ContentType: contentType}
	fileSeen := false
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, openAIAudioTranscriptionBadRequest("invalid multipart body")
		}
		name := strings.TrimSpace(part.FormName())
		if name == "file" {
			// Read one byte past the cap so an oversized file is rejected
			// instead of silently truncated.
			data, err := io.ReadAll(io.LimitReader(part, openAIAudioTranscriptionMaxFileSize+1))
			_ = part.Close()
			if err != nil {
				return nil, openAIAudioTranscriptionBadRequest("failed to read file")
			}
			if len(data) > openAIAudioTranscriptionMaxFileSize {
				return nil, &OpenAIAudioTranscriptionRequestError{
					StatusCode: http.StatusRequestEntityTooLarge,
					Message:    "file exceeds the 25 MB limit",
				}
			}
			fileSeen = true
			parsed.Audio = data
			parsed.FileName = strings.TrimSpace(part.FileName())
			parsed.FileContentType = strings.TrimSpace(part.Header.Get("Content-Type"))
			continue
		}
		if part.FileName() != "" {
			_ = part.Close()
			continue
		}
		data, err := io.ReadAll(io.LimitReader(part, openAIAudioTranscriptionMaxFieldSize))
		_ = part.Close()
		if err != nil {
			return nil, openAIAudioTranscriptionBadRequest(fmt.Sprintf("failed to read field %s", name))
		}
		value := strings.TrimSpace(string(data))
		switch name {
		case "model":
			parsed.Model = value
		case "prompt":
			parsed.Prompt = value
		case "language":
			parsed.Language = value
		case "response_format":
			parsed.ResponseFormat = strings.ToLower(value)
		case "stream":
			if enabled, err := strconv.ParseBool(value); err == nil && enabled {
				return nil, openAIAudioTranscriptionBadRequest("stream is not supported for audio transcriptions")
			}
		}
	}

	if !fileSeen {
		return nil, openAIAudioTranscriptionBadRequest("file is required")
	}
	if len(parsed.Audio) == 0 {
		return nil, openAIAudioTranscriptionBadRequest("file is empty")
	}
	if parsed.Model == "" {
		return nil, openAIAudioTranscriptionBadRequest("model is required")
	}
	switch parsed.ResponseFormat {
	case "", "json", "verbose_json", "text":
	default:
		return nil, openAIAudioTranscriptionBadRequest(fmt.Sprintf("response_format %q is not supported", parsed.ResponseFormat))
	}
	parsed.DurationSeconds, parsed.DurationExact = audioduration.Measure(parsed.Audio)
	return parsed, nil
}

// ModerationBody is the JSON handed to the security audit: the audio itself
// has no transcript before the upstream call, so only the prompt is audited.
func (r *OpenAIAudioTranscriptionRequest) ModerationBody() []byte {
	payload, _ := marshalOpenAIUpstreamJSON(map[string]string{
		"model":  r.Model,
		"prompt": r.Prompt,
	})
	return payload
}

// BilledSeconds rounds the measured duration up to whole seconds.
func (r *OpenAIAudioTranscriptionRequest) BilledSeconds() int {
	return openAIAudioTranscriptionBilledSeconds(r.DurationSeconds)
}

func openAIAudioTranscriptionBilledSeconds(seconds float64) int {
	billed := int(math.Ceil(seconds))
	if billed < openAIAudioTranscriptionMinBilledSeconds {
		return openAIAudioTranscriptionMinBilledSeconds
	}
	return billed
}

func openAIAudioTranscriptionUsage(seconds int) *AudioUsage {
	return &AudioUsage{
		Mode:            openAIAudioTranscriptionBillingMode,
		DurationOrUnits: float64(seconds) / 3600,
	}
}

// openAIAudioTranscriptionResponse renders a ChatGPT transcript in the shape
// the client asked for. verbose_json carries no segments because the upstream
// returns none; clients fall back to text in that case.
func openAIAudioTranscriptionResponse(text string, parsed *OpenAIAudioTranscriptionRequest, seconds int) ([]byte, string) {
	usage := map[string]any{"type": "duration", "seconds": seconds}
	switch parsed.ResponseFormat {
	case "text":
		return []byte(text), "text/plain; charset=utf-8"
	case "verbose_json":
		payload := map[string]any{
			"task":     "transcribe",
			"duration": seconds,
			"text":     text,
			"usage":    usage,
		}
		if parsed.Language != "" {
			payload["language"] = parsed.Language
		}
		body, _ := marshalOpenAIUpstreamJSON(payload)
		return body, "application/json"
	default:
		body, _ := marshalOpenAIUpstreamJSON(map[string]any{"text": text, "usage": usage})
		return body, "application/json"
	}
}

// openAIAudioTranscriptionUpstreamSeconds reads a whisper-style
// usage.type == "duration" report, which is more accurate than the local
// measurement for compressed uploads.
func openAIAudioTranscriptionUpstreamSeconds(body []byte) (float64, bool) {
	usage := gjson.GetBytes(body, "usage")
	if !usage.IsObject() || usage.Get("type").String() != "duration" {
		return 0, false
	}
	seconds := usage.Get("seconds").Float()
	if seconds <= 0 {
		return 0, false
	}
	return seconds, true
}

// stableOpenAIAudioTranscriptionRequestID is the durable usage_logs / dedup key
// for one transcription call; a reused client request id must not collapse
// two uploads into one charge.
func stableOpenAIAudioTranscriptionRequestID(upstreamRequestID string) string {
	upstreamRequestID = strings.TrimSpace(upstreamRequestID)
	if upstreamRequestID == "" {
		upstreamRequestID = generateRequestID()
	}
	return openAIAudioTranscriptionRequestIDPrefix + upstreamRequestID
}

func writeOpenAIAudioTranscriptionError(c *gin.Context, statusCode int, errType, message string) {
	if c == nil || c.Writer.Written() {
		return
	}
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

func writeOpenAIAudioTranscriptionUpstreamResponse(c *gin.Context, resp *http.Response, body []byte, filter *responseheaders.CompiledHeaderFilter) {
	if c == nil || resp == nil || c.Writer.Written() {
		return
	}
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, filter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, body)
}
