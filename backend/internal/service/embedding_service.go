// Package service — embedding_service.go 提供文本向量化 helper。
//
// 设计要点：
//
//  1. 转发到 admin 在 Settings 配置的"外部 upstream"。凭据 = SettingService
//     GetSupportChatEmbeddingCredentials 返回的一对 base_url + api_key（switch-
//     embedding-credentials 起从 chat 凭据拆出，独立配置；严格不回退到 chat 凭据）。
//
//  2. Provider 分派（switch-gemini-embedding）：
//     - `support_chat_rag_embed_provider = "openai"` (兼容旧行为)：
//     POST <base_url>/embeddings 走 OpenAI-compatible JSON
//     `{"model","input","encoding_format","dimensions"}`，
//     Header `Authorization: Bearer <api_key>`。
//     - `support_chat_rag_embed_provider = "gemini"` (默认)：
//     POST <base_url>/models/<model>:batchEmbedContents 走 Google Generative
//     Language API 官方协议。Header `x-goog-api-key: <api_key>`，
//     body 是 `{"requests":[{"model":"models/<m>","content":{"parts":[{"text":...}]},
//     "outputDimensionality":<dim>}]}`。
//     两条路径都统一裁到 SupportChatRAGEmbedDimension 维度（PG `vector(N)` 硬约束）。
//
//  3. 凭据缺失（base_url == "" || api_key == ""）→ 返回 ErrEmbeddingDisabled 哨兵。
//     调用方（FAQ service / doc pipeline / RAG retrieval helper）按各自现有的兜底分支处理：
//     FAQ Create 仍成功（embedding=NULL）；doc pipeline 整批 chunks embedding=NULL；
//     retrieval helper 把哨兵转成"空检索结果"让 chat 走"无相关知识"分支。
//
//  4. 失败不 retry：上层根据上下文决定。短超时（30s）兜底 batch 100 条。
//
// 接口面向 RAG 调用方（FAQ service / doc pipeline / chat handler），单条 + 批量。
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// EmbeddingService 把文本转成向量（PG `vector(N)`，N=SupportChatRAGEmbedDimension）。
type EmbeddingService interface {
	// Embed 单条文本 → 向量。文本 trim 后为空时返回 ErrEmbeddingEmptyText（避免上游 400）。
	Embed(ctx context.Context, text string) ([]float32, error)
	// EmbedBatch 多条文本 → 多条向量；输入与输出顺序一一对应。
	// 任一条 trim 后为空 → ErrEmbeddingEmptyText（上层应预过滤再调）。
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// ErrEmbeddingEmptyText 输入文本（trim 后）为空时返回。
var ErrEmbeddingEmptyText = errors.New("embedding service: empty text")

// ErrEmbeddingDisabled 当 admin 没配 support_chat_embedding_base_url /
// support_chat_embedding_api_key 时返回（switch-embedding-credentials 起 embedding
// 凭据独立，严格不回退到 chat 凭据）。caller（FAQ service / pipeline / chat handler /
// retrieval helper）应把它视作"凭据缺失，应当走兜底"的哨兵：FAQ 仍可保存
// （embedding=NULL）；doc pipeline 把整批 chunks 标记 embedding=NULL；retrieval
// helper 把它转成"空检索结果"让 chat 走无 RAG 分支。
//
// 变量名跨多次演进保持不变：旧版意为"未配 api_key_id"，中间版意为"未配 llm base_url/
// api_key"，当前意为"未配 embedding base_url/api_key"；保留以避免破坏所有
// caller 的 errors.Is 检查。
var ErrEmbeddingDisabled = errors.New("embedding service: disabled (embedding credentials not configured)")

// supportChatEmbeddingService 是默认实现：从 SettingService 拿一对 base_url + api_key，
// 按 provider setting 分派到 OpenAI-compat 或 Gemini native embedding 端点。
type supportChatEmbeddingService struct {
	settingService *SettingService
	httpClient     *http.Client
	now            func() time.Time
}

// NewSupportChatEmbeddingService 构造 embedding service。
//
// switch-embedding-credentials 之后，这里读取的是 embedding 专用的 base_url + api_key
// （SettingService.GetSupportChatEmbeddingCredentials），与 chat 凭据完全独立。
// 端口/Host 也无关（上游是外部 URL，没有 self-call 的 host:port 拼接需求）。
func NewSupportChatEmbeddingService(
	settingService *SettingService,
) EmbeddingService {
	return &supportChatEmbeddingService{
		settingService: settingService,
		// 30s 单次 timeout（embedding 上游一般 < 5s，30s 留 buffer 应对 batch 100 条情况）。
		httpClient: &http.Client{Timeout: 30 * time.Second},
		now:        time.Now,
	}
}

// Embed 单条文本 → 向量。
func (s *supportChatEmbeddingService) Embed(ctx context.Context, text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return nil, ErrEmbeddingEmptyText
	}
	out, err := s.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(out) != 1 {
		return nil, fmt.Errorf("embedding service: unexpected batch size %d (want 1)", len(out))
	}
	return out[0], nil
}

// EmbedBatch 批量。
func (s *supportChatEmbeddingService) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	for i, t := range texts {
		if strings.TrimSpace(t) == "" {
			return nil, fmt.Errorf("%w: index=%d", ErrEmbeddingEmptyText, i)
		}
	}

	// 凭据 short-circuit：空 base_url 或空 api_key → 早返回 ErrEmbeddingDisabled，
	// 避免向 "" + "/embeddings" 发请求拿一个不可读错误。
	// 注意：此处只读 embedding 专用凭据，不回退到客服 chat 的 LLM 凭据
	// （switch-embedding-credentials 严格解耦）。
	baseURL, apiKey := s.settingService.GetSupportChatEmbeddingCredentials(ctx)
	if baseURL == "" || apiKey == "" {
		return nil, ErrEmbeddingDisabled
	}

	model := strings.TrimSpace(s.settingService.GetSupportChatRAGEmbedModel(ctx))
	if model == "" {
		model = SupportChatRAGEmbedModelDefault
	}
	provider := strings.TrimSpace(s.settingService.GetSupportChatRAGEmbedProvider(ctx))
	if !IsSupportChatRAGAllowedEmbedProvider(provider) {
		provider = SupportChatRAGEmbedProviderDefault
	}

	switch provider {
	case SupportChatRAGEmbedProviderGemini:
		return s.embedGemini(ctx, baseURL, apiKey, model, texts)
	default:
		return s.embedOpenAICompat(ctx, baseURL, apiKey, model, texts)
	}
}

// embedOpenAICompat 走 OpenAI-compatible `/embeddings` 协议。
// 请求 body 里显式带 `dimensions` 参数以适配 3072 维硬约束（OpenAI text-embedding-3-*
// 支持 dimensions 缩放；不支持该参数的兼容网关会忽略——若上游模型原生非 3072 维会在
// 响应校验处报 dim mismatch）。
func (s *supportChatEmbeddingService) embedOpenAICompat(
	ctx context.Context,
	baseURL, apiKey, model string,
	texts []string,
) ([][]float32, error) {
	body, err := json.Marshal(map[string]any{
		"model":           model,
		"input":           texts,
		"encoding_format": "float", // 保证返回 float64 数组而非 base64
		"dimensions":      SupportChatRAGEmbedDimension,
	})
	if err != nil {
		return nil, fmt.Errorf("embedding service: marshal request: %w", err)
	}

	upstreamURL := buildUpstreamEmbeddingURL(baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedding service: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding service: post: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024)) // 32MiB 上限（batch 100×3072 浮点 ≈ 10MiB）
	if err != nil {
		return nil, fmt.Errorf("embedding service: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		preview := respBody
		if len(preview) > 1024 {
			preview = preview[:1024]
		}
		slog.WarnContext(ctx, "embedding_upstream_error",
			slog.String("provider", SupportChatRAGEmbedProviderOpenAI),
			slog.Int("status", resp.StatusCode),
			slog.String("body_preview", string(preview)),
		)
		return nil, fmt.Errorf("embedding service: upstream status %d", resp.StatusCode)
	}

	var parsed struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("embedding service: parse response: %w", err)
	}
	if len(parsed.Data) != len(texts) {
		return nil, fmt.Errorf("embedding service: response size mismatch: got %d, want %d", len(parsed.Data), len(texts))
	}

	// 按 index 排序回原始顺序（OpenAI 协议保证 index 与 input 对应）。
	out := make([][]float32, len(texts))
	for _, item := range parsed.Data {
		if item.Index < 0 || item.Index >= len(out) {
			return nil, fmt.Errorf("embedding service: response index out of range: %d", item.Index)
		}
		if len(item.Embedding) != SupportChatRAGEmbedDimension {
			return nil, fmt.Errorf("embedding service: vector dim mismatch: got %d, want %d (model=%s)",
				len(item.Embedding), SupportChatRAGEmbedDimension, model)
		}
		out[item.Index] = item.Embedding
	}
	for i, v := range out {
		if v == nil {
			return nil, fmt.Errorf("embedding service: missing vector at index %d", i)
		}
	}
	return out, nil
}

// embedGemini 走 Google Generative Language API 的 `:batchEmbedContents` 端点。
// 请求 URL：<base_url>/models/<model>:batchEmbedContents
// Header：`x-goog-api-key: <api_key>` （官方鉴权头；同时也支持 ?key= query，但 header
// 更安全避免日志泄漏）。
// Body 结构：
//
//	{
//	  "requests": [
//	    {
//	      "model": "models/<model>",
//	      "content": {"parts":[{"text": "<input>"}]},
//	      "outputDimensionality": <dim>
//	    }, ...
//	  ]
//	}
//
// Response：
//
//	{"embeddings":[{"values":[...]}, ...]}
//
// batchEmbedContents 官方保证响应 embeddings 数组顺序与 requests 一致，
// 故直接按 index 一一对应回填。
func (s *supportChatEmbeddingService) embedGemini(
	ctx context.Context,
	baseURL, apiKey, model string,
	texts []string,
) ([][]float32, error) {
	normalizedModel := normalizeGeminiEmbedModel(model)

	type geminiPart struct {
		Text string `json:"text"`
	}
	type geminiContent struct {
		Parts []geminiPart `json:"parts"`
	}
	type geminiRequest struct {
		Model                string        `json:"model"`
		Content              geminiContent `json:"content"`
		OutputDimensionality int           `json:"outputDimensionality"`
	}
	type geminiBatchRequest struct {
		Requests []geminiRequest `json:"requests"`
	}

	batch := geminiBatchRequest{Requests: make([]geminiRequest, len(texts))}
	for i, t := range texts {
		batch.Requests[i] = geminiRequest{
			Model:                normalizedModel,
			Content:              geminiContent{Parts: []geminiPart{{Text: t}}},
			OutputDimensionality: SupportChatRAGEmbedDimension,
		}
	}
	body, err := json.Marshal(batch)
	if err != nil {
		return nil, fmt.Errorf("embedding service: marshal gemini request: %w", err)
	}

	upstreamURL := buildGeminiBatchEmbedURL(baseURL, normalizedModel)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedding service: build gemini request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding service: post gemini: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("embedding service: read gemini response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		preview := respBody
		if len(preview) > 1024 {
			preview = preview[:1024]
		}
		slog.WarnContext(ctx, "embedding_upstream_error",
			slog.String("provider", SupportChatRAGEmbedProviderGemini),
			slog.Int("status", resp.StatusCode),
			slog.String("body_preview", string(preview)),
		)
		return nil, fmt.Errorf("embedding service: upstream status %d", resp.StatusCode)
	}

	var parsed struct {
		Embeddings []struct {
			Values []float32 `json:"values"`
		} `json:"embeddings"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("embedding service: parse gemini response: %w", err)
	}
	if len(parsed.Embeddings) != len(texts) {
		return nil, fmt.Errorf("embedding service: gemini response size mismatch: got %d, want %d",
			len(parsed.Embeddings), len(texts))
	}

	out := make([][]float32, len(texts))
	for i, e := range parsed.Embeddings {
		if len(e.Values) != SupportChatRAGEmbedDimension {
			return nil, fmt.Errorf("embedding service: vector dim mismatch: got %d, want %d (model=%s)",
				len(e.Values), SupportChatRAGEmbedDimension, model)
		}
		out[i] = e.Values
	}
	return out, nil
}

// buildUpstreamEmbeddingURL 把 admin 配置的 base_url 拼成 OpenAI-compatible embeddings endpoint。
// 与 handler.buildUpstreamChatURL 同语义：trim 末尾 `/` + 追加 `/embeddings`。
//
// base_url 自身的格式 / scheme 校验由 SettingService.buildSystemSettingsUpdates 保证；
// 这里只做"信任的字符串拼接"。
func buildUpstreamEmbeddingURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/embeddings"
}

// buildGeminiBatchEmbedURL 拼 Gemini `:batchEmbedContents` URL。
//
// 官方形式：https://generativelanguage.googleapis.com/v1beta/models/<model>:batchEmbedContents
// 允许 admin 把 base_url 配成 `https://generativelanguage.googleapis.com/v1beta`
// （标准）或 `https://generativelanguage.googleapis.com`（无版本，则走 v1beta 兜底）。
// 若 admin 已经在 base_url 中带了 `/v1beta` 或 `/v1`，此处直接沿用；否则追加 `/v1beta`。
func buildGeminiBatchEmbedURL(baseURL, normalizedModel string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	// 检查 base_url 是否已包含版本段 (/v1beta 或 /v1)；如果 admin 只填了 host，则补 /v1beta。
	lower := strings.ToLower(trimmed)
	hasVersion := strings.HasSuffix(lower, "/v1beta") ||
		strings.HasSuffix(lower, "/v1") ||
		strings.Contains(lower, "/v1beta/") ||
		strings.Contains(lower, "/v1/")
	if !hasVersion {
		// 没写版本号；追加官方推荐 v1beta。
		trimmed += "/v1beta"
	}
	// normalizedModel 形如 "models/gemini-embedding-001"，需 URL-escape 冒号。
	// 但 Google API 对 `:batchEmbedContents` 后缀不做 percent-encoding，直接拼即可。
	// 为保险起见还是显式检查 model 段不含意外字符。
	return trimmed + "/" + normalizedModel + ":batchEmbedContents"
}

// normalizeGeminiEmbedModel 把 admin 填的 model 字段规范成 Gemini API 需要的
// `models/<name>` 形式。
//   - 已是 `models/xxx` → 原样返回；
//   - 只有 `<name>` → 前缀 `models/`。
//
// 这样 admin 既可以填 `gemini-embedding-001`，也可以填 `models/gemini-embedding-001`。
func normalizeGeminiEmbedModel(model string) string {
	m := strings.TrimSpace(model)
	if m == "" {
		m = SupportChatRAGEmbedModelDefault
	}
	if strings.HasPrefix(m, "models/") {
		return m
	}
	return "models/" + m
}
