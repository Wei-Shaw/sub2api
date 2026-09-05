package backend

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultAgnesImageBaseURL = "https://apihub.agnes-ai.com/v1"

type AgnesAdapter struct{}

type AgnesImageRequest struct {
	Model        string         `json:"model"`
	Prompt       string         `json:"prompt"`
	Size         string         `json:"size"`
	Ratio        string         `json:"ratio,omitempty"`
	ReturnBase64 bool           `json:"return_base64,omitempty"`
	ExtraBody    AgnesExtraBody `json:"extra_body,omitempty"`
}

type AgnesExtraBody struct {
	ResponseFormat string   `json:"response_format,omitempty"`
	Image          []string `json:"image,omitempty"`
}

type OpenAIImageRequest struct {
	Model             string `json:"model"`
	Prompt            string `json:"prompt"`
	Background        string `json:"background,omitempty"`
	Moderation        string `json:"moderation,omitempty"`
	N                 int    `json:"n,omitempty"`
	OutputCompression *int   `json:"output_compression,omitempty"`
	OutputFormat      string `json:"output_format,omitempty"`
	Quality           string `json:"quality,omitempty"`
	ResponseFormat    string `json:"response_format,omitempty"`
	Size              string `json:"size,omitempty"`
	Style             string `json:"style,omitempty"`
	User              string `json:"user,omitempty"`
}

type OpenAIImageResponse struct {
	Created int64             `json:"created"`
	Data    []OpenAIImageData `json:"data"`
}

type OpenAIImageData struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

type OpenAIImageEditRequest struct {
	Model          string
	Prompt         string
	N              int
	ResponseFormat string
	Size           string
	Images         []string
}

func NewAgnesAdapter() *AgnesAdapter {
	return &AgnesAdapter{}
}

func (*AgnesAdapter) Platform() string {
	return "agnes"
}

func (*AgnesAdapter) Descriptor() PlatformDescriptor {
	return PlatformDescriptor{Key: "agnes", DisplayName: "Agnes", Operation: "images/generations", Protocol: "agnes-image", DefaultBaseURL: defaultAgnesImageBaseURL}
}

func buildAgnesImageRequest(_ RouteConfig, modelValue, promptValue, sizeValue, responseFormat string, images []string) (AgnesImageRequest, error) {
	prompt := strings.TrimSpace(promptValue)
	if prompt == "" {
		return AgnesImageRequest{}, fmt.Errorf("prompt is required")
	}
	model := strings.TrimSpace(modelValue)
	if model == "" {
		return AgnesImageRequest{}, fmt.Errorf("model is required")
	}
	size, ratio, err := agnesSizeAndRatio(sizeValue)
	if err != nil {
		return AgnesImageRequest{}, err
	}
	outgoing := AgnesImageRequest{Model: model, Prompt: prompt, Size: size, Ratio: ratio}
	if len(images) > 0 {
		outgoing.ExtraBody.Image = images
	}
	switch strings.TrimSpace(responseFormat) {
	case "", "url":
		outgoing.ExtraBody.ResponseFormat = "url"
	case "b64_json":
		outgoing.ReturnBase64 = true
		outgoing.ExtraBody.ResponseFormat = "b64_json"
	default:
		return AgnesImageRequest{}, fmt.Errorf("unsupported response_format %q", responseFormat)
	}
	return outgoing, nil
}

func (*AgnesAdapter) ParseResponse(body []byte) (OpenAIImageResponse, error) {
	var response OpenAIImageResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return OpenAIImageResponse{}, fmt.Errorf("invalid Agnes image response: %w", err)
	}
	if len(response.Data) == 0 {
		return OpenAIImageResponse{}, fmt.Errorf("Agnes image response has no data")
	}
	return response, nil
}

func decodeAgnesResponseBody(headers http.Header, body []byte) ([]byte, error) {
	if !strings.EqualFold(strings.TrimSpace(headers.Get("Content-Encoding")), "gzip") {
		return body, nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("invalid gzip Agnes image response: %w", err)
	}
	defer reader.Close()
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read gzip Agnes image response: %w", err)
	}
	return decoded, nil
}

func decodeOpenAIImageEditRequest(r *http.Request) (OpenAIImageEditRequest, error) {
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		return OpenAIImageEditRequest{}, fmt.Errorf("invalid multipart image edit request: %w", err)
	}
	count := 1
	if raw := strings.TrimSpace(r.FormValue("n")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			return OpenAIImageEditRequest{}, fmt.Errorf("n must be at least 1")
		}
		count = parsed
	}
	files := r.MultipartForm.File["image"]
	if len(files) == 0 {
		return OpenAIImageEditRequest{}, fmt.Errorf("image is required")
	}
	images := make([]string, 0, len(files))
	for _, file := range files {
		content, err := file.Open()
		if err != nil {
			return OpenAIImageEditRequest{}, fmt.Errorf("read image: %w", err)
		}
		data, readErr := io.ReadAll(io.LimitReader(content, 10<<20))
		_ = content.Close()
		if readErr != nil {
			return OpenAIImageEditRequest{}, fmt.Errorf("read image: %w", readErr)
		}
		if len(data) == 0 {
			return OpenAIImageEditRequest{}, fmt.Errorf("image is empty")
		}
		mediaType := file.Header.Get("Content-Type")
		if mediaType == "" || mediaType == "application/octet-stream" {
			mediaType = mime.TypeByExtension(filepath.Ext(file.Filename))
		}
		if mediaType == "" {
			mediaType = "application/octet-stream"
		}
		images = append(images, "data:"+mediaType+";base64,"+base64.StdEncoding.EncodeToString(data))
	}
	return OpenAIImageEditRequest{Model: r.FormValue("model"), Prompt: r.FormValue("prompt"), N: count, ResponseFormat: r.FormValue("response_format"), Size: r.FormValue("size"), Images: images}, nil
}

func (adapter *AgnesAdapter) Handle(ctx context.Context, request PlatformRequest) (PlatformResponse, error) {
	endpoint := canonicalRelayPath(request.Endpoint)
	if endpoint != "images/generations" && endpoint != "images/edits" {
		return forwardPlatformRequest(ctx, request, request.Route, endpoint, request.Method, request.Body)
	}
	var incoming OpenAIImageRequest
	var images []string
	if endpoint == "images/edits" {
		httpRequest, err := http.NewRequest(http.MethodPost, "http://relay.local", strings.NewReader(string(request.Body)))
		if err != nil {
			return PlatformResponse{StatusCode: http.StatusBadRequest}, err
		}
		httpRequest.Header = request.Headers.Clone()
		edit, err := decodeOpenAIImageEditRequest(httpRequest)
		if err != nil {
			return PlatformResponse{StatusCode: http.StatusBadRequest}, err
		}
		incoming = OpenAIImageRequest{Model: edit.Model, Prompt: edit.Prompt, N: edit.N, ResponseFormat: edit.ResponseFormat, Size: edit.Size}
		images = edit.Images
	} else if err := json.Unmarshal(request.Body, &incoming); err != nil {
		return PlatformResponse{StatusCode: http.StatusBadRequest}, fmt.Errorf("invalid image request: %w", err)
	}
	if incoming.N == 0 {
		incoming.N = 1
	}
	payload, err := buildAgnesImageRequest(request.Route, incoming.Model, incoming.Prompt, incoming.Size, incoming.ResponseFormat, images)
	if err != nil {
		return PlatformResponse{StatusCode: http.StatusBadRequest}, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return PlatformResponse{StatusCode: http.StatusBadGateway}, err
	}
	forwardRequest := request
	forwardRequest.Headers = request.Headers.Clone()
	forwardRequest.Headers.Set("Content-Type", "application/json")
	result := OpenAIImageResponse{Data: make([]OpenAIImageData, 0, incoming.N)}
	for range incoming.N {
		response, err := forwardPlatformRequest(ctx, forwardRequest, request.Route, "images/generations", http.MethodPost, encoded)
		if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
			logAgnesUpstreamFailure(request.Route, response, err)
			return response, err
		}
		body, err := decodeAgnesResponseBody(response.Headers, response.Body)
		if err != nil {
			logAgnesResponseParseFailure(request.Route, response.Body, err)
			return PlatformResponse{StatusCode: http.StatusBadGateway}, err
		}
		parsed, err := adapter.ParseResponse(body)
		if err != nil {
			logAgnesResponseParseFailure(request.Route, response.Body, err)
			return PlatformResponse{StatusCode: http.StatusBadGateway}, err
		}
		if result.Created == 0 {
			result.Created = parsed.Created
		}
		result.Data = append(result.Data, parsed.Data...)
	}
	body, err := json.Marshal(result)
	if err != nil {
		return PlatformResponse{StatusCode: http.StatusBadGateway}, err
	}
	return PlatformResponse{StatusCode: http.StatusOK, Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: body}, nil
}

func logAgnesUpstreamFailure(config RouteConfig, response PlatformResponse, err error) {
	upstreamURL, resolveErr := ResolveRouteEndpointURL(config, "images/generations")
	if resolveErr != nil {
		upstreamURL = ""
	}
	preview := string(response.Body)
	if len(preview) > 1000 {
		preview = preview[:1000]
	}
	log.Printf("agnes_image_upstream_failure url=%q status=%d error=%q response=%q", upstreamURL, response.StatusCode, err, preview)
}

func logAgnesResponseParseFailure(config RouteConfig, body []byte, err error) {
	upstreamURL, resolveErr := ResolveRouteEndpointURL(config, "images/generations")
	if resolveErr != nil {
		upstreamURL = ""
	}
	preview := string(body)
	if len(preview) > 1000 {
		preview = preview[:1000]
	}
	log.Printf("agnes_image_response_parse_failure url=%q error=%q response=%q", upstreamURL, err, preview)
}

func agnesSizeAndRatio(value string) (string, string, error) {
	switch strings.TrimSpace(value) {
	case "", "1024x1024", "auto":
		return "1K", "1:1", nil
	case "1536x1024":
		return "1K", "3:2", nil
	case "1024x1536":
		return "1K", "2:3", nil
	default:
		return "", "", fmt.Errorf("unsupported size %q", value)
	}
}
