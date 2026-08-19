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
	Prompt      string `json:"prompt"`
	Quality     string `json:"quality"`
	AspectRatio string `json:"aspect_ratio"`
	Size        string `json:"size"`
	Resolution  string `json:"resolution"`
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
	URL       string `json:"url"`
	MediaType string `json:"media_type,omitempty"`
	MIMEType  string `json:"mime_type,omitempty"`
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
		return "leonardo task failed"
	}
	return fmt.Sprintf("leonardo task failed: %v", t.Error)
}

func BuildSubmitRequest(model string, input fal.ImageGenInput, estimatedCreditCost float64) *SubmitRequest {
	if strings.TrimSpace(model) == "" {
		model = "gpt-image-2"
	}
	if estimatedCreditCost <= 0 {
		estimatedCreditCost = DefaultEstimatedCreditCost
	}
	resolution := normalizeResolution(input.Size)
	return &SubmitRequest{
		Provider: "leonardo",
		TaskType: "IMAGE_GENERATION",
		Model:    strings.TrimSpace(model),
		Mode:     "text-to-image",
		Input: GenerateInput{
			Prompt:      strings.TrimSpace(input.Prompt),
			Quality:     normalizeQuality(input.Quality),
			AspectRatio: aspectRatio(resolution),
			Size:        sizeClass(resolution),
			Resolution:  resolution,
		},
		EstimatedCreditCost: estimatedCreditCost,
	}
}

func normalizeQuality(value string) string {
	switch strings.ToUpper(strings.TrimSpace(fal.MapQualityToFal(value))) {
	case "MEDIUM", "HIGH", "AUTO":
		return strings.ToUpper(strings.TrimSpace(fal.MapQualityToFal(value)))
	default:
		return "LOW"
	}
}

func normalizeResolution(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	parts := strings.SplitN(value, "x", 2)
	if len(parts) != 2 {
		return "1024x1024"
	}
	w, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return "1024x1024"
	}
	return fmt.Sprintf("%dx%d", w, h)
}

func aspectRatio(resolution string) string {
	parts := strings.SplitN(resolution, "x", 2)
	w, _ := strconv.Atoi(parts[0])
	h, _ := strconv.Atoi(parts[1])
	divisor := gcd(w, h)
	if divisor <= 0 {
		return "1:1"
	}
	return fmt.Sprintf("%d:%d", w/divisor, h/divisor)
}

func sizeClass(resolution string) string {
	parts := strings.SplitN(resolution, "x", 2)
	w, _ := strconv.Atoi(parts[0])
	h, _ := strconv.Atoi(parts[1])
	maxDimension := max(w, h)
	switch {
	case maxDimension > 2048:
		return "LARGE"
	case maxDimension > 1024:
		return "MEDIUM"
	default:
		return "SMALL"
	}
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
