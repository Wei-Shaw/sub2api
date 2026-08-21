package leonardo

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/fal"
)

const (
	StatusCompleted = "COMPLETED"
	StatusFailed    = "FAILED"

	DefaultBaseURL             = "http://127.0.0.1:28080"
	DefaultEstimatedCreditCost = 8
)

type GenerateInput struct {
	Prompt             string   `json:"prompt"`
	Quality            string   `json:"quality"`
	Width              int      `json:"width"`
	Height             int      `json:"height"`
	ReferenceImageURLs []string `json:"reference_image_urls,omitempty"`
}

type SubmitRequest struct {
	Provider            string        `json:"provider"`
	TaskType            string        `json:"task_type"`
	Model               string        `json:"model"`
	Mode                string        `json:"mode"`
	Input               GenerateInput `json:"input"`
	EstimatedCreditCost float64       `json:"estimated_credit_cost"`
}

type Media struct {
	ID        string `json:"id,omitempty"`
	URL       string `json:"url"`
	Type      string `json:"type,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	MIMEType  string `json:"mime_type,omitempty"`
	FileName  string `json:"file_name,omitempty"`
	FileSize  int64  `json:"file_size,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
}

type Output struct {
	Media []Media `json:"media"`
}

type Task struct {
	TaskUUID string `json:"task_uuid"`
	Status   string `json:"status"`
	Output   Output `json:"output"`
	Error    any    `json:"error,omitempty"`
}

func (t *Task) IsCompleted() bool {
	return t != nil && strings.EqualFold(strings.TrimSpace(t.Status), StatusCompleted)
}

func (t *Task) IsFailed() bool {
	return t != nil && strings.EqualFold(strings.TrimSpace(t.Status), StatusFailed)
}

func (t *Task) FailureMessage() string {
	if t == nil || t.Error == nil {
		return "task failed"
	}
	return fmt.Sprintf("task failed: %v", t.Error)
}

func BuildSubmitRequest(model string, input fal.ImageGenInput, estimatedCreditCost float64) *SubmitRequest {
	if strings.TrimSpace(model) == "" {
		model = "gpt-image-2"
	}
	if estimatedCreditCost <= 0 {
		estimatedCreditCost = DefaultEstimatedCreditCost
	}
	width, height := imageDimensions(input.Size)
	mode := "text-to-image"
	var referenceImageURLs []string
	if input.IsEdit {
		mode = "image-to-image"
		referenceImageURLs = trimReferenceImageURLs(input.ImageURLs)
	}
	return &SubmitRequest{
		Provider: "leonardo",
		TaskType: "IMAGE_GENERATION",
		Model:    strings.TrimSpace(model),
		Mode:     mode,
		Input: GenerateInput{
			Prompt:             strings.TrimSpace(input.Prompt),
			Quality:            normalizeQuality(input.Quality),
			Width:              width,
			Height:             height,
			ReferenceImageURLs: referenceImageURLs,
		},
		EstimatedCreditCost: estimatedCreditCost,
	}
}

func trimReferenceImageURLs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func normalizeQuality(value string) string {
	switch strings.ToUpper(strings.TrimSpace(fal.MapQualityToFal(value))) {
	case "MEDIUM", "HIGH":
		return strings.ToUpper(strings.TrimSpace(fal.MapQualityToFal(value)))
	case "AUTO":
		// Leonardo does not expose an AUTO quality; use its medium preset.
		return "MEDIUM"
	default:
		return "LOW"
	}
}

func imageDimensions(value string) (int, int) {
	value = strings.ToLower(strings.TrimSpace(value))
	parts := strings.SplitN(value, "x", 2)
	if len(parts) != 2 {
		return 1024, 1024
	}
	w, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return 1024, 1024
	}
	return w, h
}
