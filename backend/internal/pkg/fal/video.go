package fal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ============================================================
// 视频/通用透传：为 seedance 等视频模型服务
//
// 与图片路径不同，视频门面对外呈现 fal 原生协议，请求 body 与结果 payload
// 都直接透传（不做二次结构化），仅在提交、状态、结果、取消四个动作上做
// 鉴权/计费/账号选择的包装。
// ============================================================

// SubmitRaw 以任意 payload 向 queue 协议提交异步任务。
// POST {queueBaseURL}/{model}
//
// 与 Submit（图片专用 *Request）并存：视频/通用透传路径直接使用客户端原始 body。
// 返回值同 SubmitResponse。
func (c *Client) SubmitRaw(ctx context.Context, model string, body any) (*SubmitResponse, error) {
	endpoint := fmt.Sprintf("%s/%s", c.queueBaseURL, strings.TrimLeft(model, "/"))
	var out SubmitResponse
	if err := c.doJSON(ctx, http.MethodPost, endpoint, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ResultRaw 拉取任意 fal 任务的 result 原始 JSON。
// GET {responseURL}
//
// 与 Result（图片专用 *Response）并存：视频路径需要保留 fal 上游的完整结构
// 直接透传给客户端（例如 { video: {url,...}, seed, ... }）。
func (c *Client) ResultRaw(ctx context.Context, responseURL string) (map[string]any, error) {
	if strings.TrimSpace(responseURL) == "" {
		return nil, errors.New("fal: response url is empty")
	}
	out := make(map[string]any)
	if err := c.doJSON(ctx, http.MethodGet, responseURL, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ExtractVideoURLs 尽力从 fal 视频 result payload 中抽取 video url 列表。
// 兼容常见结构：
//   - { "video":  { "url": "..." } }
//   - { "videos": [ { "url": "..." }, ... ] }
//   - { "output_video": { "url": "..." } }
//   - { "video_url": "..." }
//
// 未识别到时返回空切片（由调用方判定是否失败）。
func ExtractVideoURLs(payload map[string]any) []string {
	if payload == nil {
		return nil
	}
	out := make([]string, 0, 2)

	// 单个 video 对象
	if v, ok := payload["video"].(map[string]any); ok {
		if u := extractURLField(v); u != "" {
			out = append(out, u)
		}
	}
	// output_video（部分模型使用）
	if v, ok := payload["output_video"].(map[string]any); ok {
		if u := extractURLField(v); u != "" {
			out = append(out, u)
		}
	}
	// video_url 顶层字符串
	if s, ok := payload["video_url"].(string); ok && strings.TrimSpace(s) != "" {
		out = append(out, strings.TrimSpace(s))
	}
	// videos 数组
	if arr, ok := payload["videos"].([]any); ok {
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				if u := extractURLField(m); u != "" {
					out = append(out, u)
				}
			}
		}
	}
	return dedupURLs(out)
}

// extractURLField 从对象中读取 url 字段。
func extractURLField(m map[string]any) string {
	if m == nil {
		return ""
	}
	if s, ok := m["url"].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func dedupURLs(in []string) []string {
	if len(in) == 0 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// MarshalPayload 序列化一个 map payload；辅助单测/日志。
func MarshalPayload(p map[string]any) ([]byte, error) {
	return json.Marshal(p)
}
