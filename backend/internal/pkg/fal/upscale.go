package fal

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// UpscaleRequest 是 SeedVR upscale（fal-ai/seedvr/upscale/image）的入参。
// image_url 接受 http(s) URL 或 base64 data URI；upscale_mode=factor 时按 upscale_factor 放大。
type UpscaleRequest struct {
	ImageURL      string `json:"image_url"`
	UpscaleMode   string `json:"upscale_mode,omitempty"`   // "factor"
	UpscaleFactor int    `json:"upscale_factor,omitempty"` // 2 / 4 ...
	OutputFormat  string `json:"output_format,omitempty"`  // "png" / "jpg"
}

// UpscaleImage 是 SeedVR upscale 的出图（单图，托管 URL）。
type UpscaleImage struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

// UpscaleResponse 是 SeedVR upscale 的结果体：{ "image": { url, content_type } }。
type UpscaleResponse struct {
	Image UpscaleImage `json:"image"`
}

// SubmitUpscale 以 queue 协议提交一个 upscale 任务。
//
// POST {queueBaseURL}/{model}
func (c *Client) SubmitUpscale(ctx context.Context, model string, body *UpscaleRequest) (*SubmitResponse, error) {
	endpoint := fmt.Sprintf("%s/%s", c.queueBaseURL, strings.TrimLeft(model, "/"))
	var out SubmitResponse
	if err := c.doJSON(ctx, http.MethodPost, endpoint, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpscaleResult 获取 upscale 任务的最终结果。
func (c *Client) UpscaleResult(ctx context.Context, responseURL string) (*UpscaleResponse, error) {
	if strings.TrimSpace(responseURL) == "" {
		return nil, fmt.Errorf("fal: response url is empty")
	}
	var out UpscaleResponse
	if err := c.doJSON(ctx, http.MethodGet, responseURL, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
