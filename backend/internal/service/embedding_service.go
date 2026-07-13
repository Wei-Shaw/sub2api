// Package service — embedding_service.go 提供文本向量化 helper。
//
// 设计要点：
//
//  1. 转发到 admin 在 Settings 配置的"外部 OpenAI-compatible upstream"。
//     URL = `support_chat_llm_base_url + "/embeddings"`，凭据 = `support_chat_llm_api_key`。
//     embedding 与 chat 共用同一对凭据（design D2，change-support-chat-external-llm 引入）。
//     这套方案替代了旧的"自调 127.0.0.1 + admin api_keys 表"路径——admin 不再需要维护
//     一条专门的 internal API key 来"代客服浮窗调本机网关"。
//
//  2. 凭据缺失（base_url == "" || api_key == ""）→ 返回 ErrEmbeddingDisabled 哨兵。
//     调用方（FAQ service / doc pipeline / RAG retrieval helper）按各自现有的兜底分支处理：
//     FAQ Create 仍成功（embedding=NULL）；doc pipeline 整批 chunks embedding=NULL；
//     retrieval helper 把哨兵转成"空检索结果"让 chat 走"无相关知识"分支。
//
//  3. 失败不 retry：上层根据上下文决定。短超时（30s）兜底 batch 100 条。
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

// EmbeddingService 把文本转成向量（PG `vector(N)`，N=1536 写死匹配 text-embedding-3-small）。
type EmbeddingService interface {
	// Embed 单条文本 → 向量。文本 trim 后为空时返回 ErrEmbeddingEmptyText（避免上游 400）。
	Embed(ctx context.Context, text string) ([]float32, error)
	// EmbedBatch 多条文本 → 多条向量；输入与输出顺序一一对应。
	// 任一条 trim 后为空 → ErrEmbeddingEmptyText（上层应预过滤再调）。
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// ErrEmbeddingEmptyText 输入文本（trim 后）为空时返回。
var ErrEmbeddingEmptyText = errors.New("embedding service: empty text")

// ErrEmbeddingDisabled 当 admin 没配 support_chat_llm_base_url / support_chat_llm_api_key
// 时返回。caller（FAQ service / pipeline / chat handler / retrieval helper）应该把这视作
// "凭据缺失，应当走兜底"的哨兵：FAQ 仍可保存（embedding=NULL）；doc pipeline 把整批 chunks
// 标记 embedding=NULL；retrieval helper 把它转成"空检索结果"让 chat 走无 RAG 分支。
//
// 由 change-support-chat-external-llm 重新定义语义：旧版意为"未配 api_key_id"，新版意为
// "未配 base_url 或 api_key"；变量名保留以避免破坏所有 caller 的 errors.Is 检查。
var ErrEmbeddingDisabled = errors.New("embedding service: disabled (LLM credentials not configured)")

// supportChatEmbeddingService 是默认实现：从 SettingService 拿一对 base_url + api_key，
// 直接 POST 到 `<base_url>/embeddings`。无任何对内部 APIKeyService 的依赖。
type supportChatEmbeddingService struct {
	settingService *SettingService
	httpClient     *http.Client
	now            func() time.Time
}

// NewSupportChatEmbeddingService 构造 embedding service。
//
// change-support-chat-external-llm 之后，这里不再依赖 *APIKeyService / *config.Config —
// 凭据完全由 SettingService.GetSupportChatLLMCredentials 提供，端口/Host 也无关
// （上游是外部 URL，没有 self-call 的 host:port 拼接需求）。
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
	baseURL, apiKey := s.settingService.GetSupportChatLLMCredentials(ctx)
	if baseURL == "" || apiKey == "" {
		return nil, ErrEmbeddingDisabled
	}

	model := strings.TrimSpace(s.settingService.GetSupportChatRAGEmbedModel(ctx))
	if model == "" {
		model = SupportChatRAGEmbedModelDefault
	}

	body, err := json.Marshal(map[string]any{
		"model":           model,
		"input":           texts,
		"encoding_format": "float", // 保证返回 float64 数组而非 base64
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

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024)) // 32MiB 上限（batch 100×1536 浮点 ≈ 5MiB）
	if err != nil {
		return nil, fmt.Errorf("embedding service: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 上游错误：截前 1KiB 进日志，避免日志爆炸
		preview := respBody
		if len(preview) > 1024 {
			preview = preview[:1024]
		}
		slog.WarnContext(ctx, "embedding_upstream_error",
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

// buildUpstreamEmbeddingURL 把 admin 配置的 base_url 拼成 OpenAI-compatible embeddings endpoint。
// 与 handler.buildUpstreamChatURL 同语义：trim 末尾 `/` + 追加 `/embeddings`。
//
// base_url 自身的格式 / scheme 校验由 SettingService.buildSystemSettingsUpdates 保证；
// 这里只做"信任的字符串拼接"。
func buildUpstreamEmbeddingURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/embeddings"
}
