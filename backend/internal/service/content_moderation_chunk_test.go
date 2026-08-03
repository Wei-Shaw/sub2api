package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplitTextByTokens_ShortTextSingleChunk(t *testing.T) {
	chunks, err := splitTextByTokens("hello world", 3000)

	require.NoError(t, err)
	require.Len(t, chunks, 1)
	require.Equal(t, "hello world", chunks[0])
}

func TestSplitTextByTokens_SplitsAtChunkBoundary(t *testing.T) {
	// 每个 "word " 约 1 token，构造约 250 token 的文本，按 100 token 切
	text := strings.Repeat("word ", 250)

	chunks, err := splitTextByTokens(text, 100)

	require.NoError(t, err)
	require.GreaterOrEqual(t, len(chunks), 2)
	// 拼接后应无损（不丢内容）
	require.Equal(t, text, strings.Join(chunks, ""))
}

func TestSplitTextByTokens_DropsTinyTailChunk(t *testing.T) {
	// 100 token 一片、尾片阈值 100/8=12.5 → 尾片 5 token 应被丢弃
	text := strings.Repeat("word ", 100) + strings.Repeat("x ", 3)

	chunks, err := splitTextByTokens(text, 100)

	require.NoError(t, err)
	joined := strings.Join(chunks, "")
	require.NotEqual(t, text, joined, "过短的尾片应被丢弃")
	require.Less(t, len(joined), len(text))
}

func TestSplitTextByTokens_KeepsSubstantialTailChunk(t *testing.T) {
	// 尾片远大于 100/8 → 必须保留
	text := strings.Repeat("word ", 100) + strings.Repeat("tail ", 50)

	chunks, err := splitTextByTokens(text, 100)

	require.NoError(t, err)
	require.Equal(t, text, strings.Join(chunks, ""), "足够大的尾片必须保留")
}

func TestSplitTextByTokens_NeverDropsOnlyChunk(t *testing.T) {
	// 全文就只有一片且很短：不能因为"小于 1/8"就把唯一内容丢掉
	chunks, err := splitTextByTokens("tiny", 3000)

	require.NoError(t, err)
	require.Len(t, chunks, 1)
	require.Equal(t, "tiny", chunks[0])
}

func TestSplitTextByTokens_EmptyText(t *testing.T) {
	chunks, err := splitTextByTokens("", 3000)

	require.NoError(t, err)
	require.Empty(t, chunks)
}

func TestMergeMaxCategoryScores(t *testing.T) {
	merged := mergeMaxCategoryScores([]map[string]float64{
		{"violence": 0.1, "sexual": 0.9},
		{"violence": 0.8, "sexual": 0.2},
		{"violence": 0.3},
	})

	require.Equal(t, 0.8, merged["violence"])
	require.Equal(t, 0.9, merged["sexual"])
}

func TestContentModerationConfigChunkDefaults(t *testing.T) {
	cfg, err := parseContentModerationConfig("")

	require.NoError(t, err)
	require.Equal(t, 3000, cfg.ChunkTokens)
	require.Equal(t, 2, cfg.ChunkConcurrency)
	require.Equal(t, defaultContentModerationChunkMaxChunks, cfg.ChunkMaxChunks)
	require.False(t, cfg.ChunkModerationEnabled, "分块审核默认关闭，避免改变既有部署的成本与延迟")
}

func TestContentModerationConfigChunkClamped(t *testing.T) {
	cfg, err := parseContentModerationConfig(`{"chunk_tokens":999999,"chunk_concurrency":0}`)

	require.NoError(t, err)
	require.LessOrEqual(t, cfg.ChunkTokens, maxContentModerationChunkTokens,
		"分片上限必须被钳制在退化阈值以下")
	require.GreaterOrEqual(t, cfg.ChunkConcurrency, 1)
}

func TestContentModerationConfigChunkMaxChunksClamped(t *testing.T) {
	high, err := parseContentModerationConfig(`{"chunk_max_chunks":999999}`)
	require.NoError(t, err)
	require.Equal(t, maxContentModerationChunkMaxChunks, high.ChunkMaxChunks,
		"最大分片数必须被钳制，否则单次审核成本不可控")

	low, err := parseContentModerationConfig(`{"chunk_max_chunks":0}`)
	require.NoError(t, err)
	require.Equal(t, defaultContentModerationChunkMaxChunks, low.ChunkMaxChunks,
		"0 视为未配置，回落默认值")

	negative, err := parseContentModerationConfig(`{"chunk_max_chunks":-5}`)
	require.NoError(t, err)
	require.Equal(t, defaultContentModerationChunkMaxChunks, negative.ChunkMaxChunks)
}

func TestContentModerationConfigChunkModerationEnabledParsed(t *testing.T) {
	cfg, err := parseContentModerationConfig(`{"chunk_moderation_enabled":true}`)

	require.NoError(t, err)
	require.True(t, cfg.ChunkModerationEnabled)
}

func TestLimitChunks_TruncatesToMaxAndReportsDropped(t *testing.T) {
	chunks := []string{"a", "b", "c", "d", "e"}

	kept, dropped := limitChunks(chunks, 3)

	require.Equal(t, []string{"a", "b", "c"}, kept, "超出上限时取前 N 片")
	require.Equal(t, 2, dropped)
}

func TestLimitChunks_KeepsAllWhenUnderLimit(t *testing.T) {
	chunks := []string{"a", "b"}

	kept, dropped := limitChunks(chunks, 8)

	require.Equal(t, chunks, kept)
	require.Zero(t, dropped)
}

func TestLimitChunks_NonPositiveLimitFallsBackToDefault(t *testing.T) {
	chunks := make([]string, defaultContentModerationChunkMaxChunks+3)
	for i := range chunks {
		chunks[i] = "x"
	}

	kept, dropped := limitChunks(chunks, 0)

	require.Len(t, kept, defaultContentModerationChunkMaxChunks)
	require.Equal(t, 3, dropped)
}

func TestChunkSourceText_PrefersUntruncatedFullText(t *testing.T) {
	in := ContentModerationInput{Text: "truncated", FullText: "the whole thing"}

	require.Equal(t, "the whole thing", in.chunkSourceText())
}

func TestChunkSourceText_FallsBackToTextWhenNoFullText(t *testing.T) {
	in := ContentModerationInput{Text: "only this"}

	require.Equal(t, "only this", in.chunkSourceText())
}

func TestNormalizeKeepsFullTextForChunking(t *testing.T) {
	long := strings.Repeat("中", maxModerationInputRunes+500)
	in := ContentModerationInput{Text: long}

	in.Normalize()

	require.Len(t, []rune(in.Text), maxModerationInputRunes,
		"Text 仍按原有上限截断：Hash / 摘要 / 关键词匹配的行为不能变")
	require.Len(t, []rune(in.FullText), maxModerationInputRunes+500,
		"FullText 保留全文供分块审核使用")
}

// chunkModerationTestServer 记录每次送审的文本，用来断言"到底送了几片、各片是什么"。
type chunkModerationTestServer struct {
	mu      sync.Mutex
	inputs  []string
	scores  map[string]float64
	perCall map[string]map[string]float64
}

func (m *chunkModerationTestServer) record(text string) map[string]float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inputs = append(m.inputs, text)
	for marker, scores := range m.perCall {
		if strings.Contains(text, marker) {
			return scores
		}
	}
	return m.scores
}

// mockScoreByContent 让含指定标记的分片返回特定分数，用于验证"取各片最高分"。
func mockScoreByContent(m *chunkModerationTestServer, marker string, scores map[string]float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.perCall == nil {
		m.perCall = map[string]map[string]float64{}
	}
	m.perCall[marker] = scores
}

func (m *chunkModerationTestServer) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.inputs)
}

func newChunkModerationService(t *testing.T, mock *chunkModerationTestServer, mutate func(cfg *ContentModerationConfig)) (*ContentModerationService, *contentModerationTestRepo) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		text := gjsonModerationInputText(raw)
		scores := mock.record(text)
		_ = json.NewEncoder(w).Encode(moderationAPIResponse{
			Results: []moderationAPIResult{{CategoryScores: scores}},
		})
	}))
	t.Cleanup(server.Close)

	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.BaseURL = server.URL
	cfg.APIKeys = []string{"sk-test"}
	mutate(cfg)
	rawCfg, err := json.Marshal(cfg)
	require.NoError(t, err)

	repo := &contentModerationTestRepo{}
	svc := NewContentModerationService(
		&contentModerationTestSettingRepo{values: map[string]string{
			SettingKeyRiskControlEnabled:      "true",
			SettingKeyContentModerationConfig: string(rawCfg),
		}},
		repo, &contentModerationTestHashCache{}, nil, nil, nil, nil, nil,
	)
	return svc, repo
}

// gjsonModerationInputText 从 moderation 请求体里取出送审文本（input 为字符串或分段数组）。
func gjsonModerationInputText(raw []byte) string {
	var payload struct {
		Input json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	var asString string
	if err := json.Unmarshal(payload.Input, &asString); err == nil {
		return asString
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(payload.Input, &parts); err != nil {
		return ""
	}
	var sb strings.Builder
	for _, p := range parts {
		if p.Type == "text" {
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}

func chunkTestBody(text string) []byte {
	body, _ := json.Marshal(map[string]any{
		"messages": []map[string]string{{"role": "user", "content": text}},
	})
	return body
}

func TestCheck_ChunkDisabled_SendsSingleTruncatedCall(t *testing.T) {
	// 全文远超 maxModerationInputRunes：关闭分块时应维持既有行为——只发一次、且被截断。
	long := strings.Repeat("word ", 8000)
	mock := &chunkModerationTestServer{scores: map[string]float64{"violence": 0.1}}
	svc, _ := newChunkModerationService(t, mock, func(cfg *ContentModerationConfig) {
		cfg.ChunkModerationEnabled = false
	})

	_, err := svc.Check(context.Background(), ContentModerationCheckInput{
		Endpoint: "/v1/messages",
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     chunkTestBody(long),
	})

	require.NoError(t, err)
	require.Equal(t, 1, mock.callCount(), "关闭分块时必须只发一次调用")
	require.LessOrEqual(t, len([]rune(mock.inputs[0])), maxModerationInputRunes,
		"关闭分块时仍按原有上限截断")
}

func TestCheck_ChunkEnabled_SplitsFullTextAcrossCalls(t *testing.T) {
	long := strings.Repeat("word ", 8000)
	mock := &chunkModerationTestServer{scores: map[string]float64{"violence": 0.1}}
	svc, _ := newChunkModerationService(t, mock, func(cfg *ContentModerationConfig) {
		cfg.ChunkModerationEnabled = true
		cfg.ChunkTokens = 1000
		cfg.ChunkMaxChunks = 64
	})

	_, err := svc.Check(context.Background(), ContentModerationCheckInput{
		Endpoint: "/v1/messages",
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     chunkTestBody(long),
	})

	require.NoError(t, err)
	require.Greater(t, mock.callCount(), 1, "开启分块后应拆成多次调用")

	mock.mu.Lock()
	joined := strings.Join(mock.inputs, "")
	mock.mu.Unlock()
	require.Greater(t, len([]rune(joined)), maxModerationInputRunes,
		"分块必须覆盖到 12000 runes 截断之外的内容——这正是静默失效的根因")
}

func TestCheck_ChunkEnabled_HonoursMaxChunks(t *testing.T) {
	long := strings.Repeat("word ", 8000)
	mock := &chunkModerationTestServer{scores: map[string]float64{"violence": 0.1}}
	svc, _ := newChunkModerationService(t, mock, func(cfg *ContentModerationConfig) {
		cfg.ChunkModerationEnabled = true
		cfg.ChunkTokens = 500
		cfg.ChunkMaxChunks = 3
	})

	_, err := svc.Check(context.Background(), ContentModerationCheckInput{
		Endpoint: "/v1/messages",
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     chunkTestBody(long),
	})

	require.NoError(t, err)
	require.Equal(t, 3, mock.callCount(), "送审片数必须被 ChunkMaxChunks 限制住")
}

func TestCheck_ChunkEnabled_TakesMaxScoreAcrossChunks(t *testing.T) {
	// 只有靠后的分片含高分内容：不分块时它落在截断之外，分块后必须被取到。
	head := strings.Repeat("safe ", 200)
	tail := strings.Repeat("risky ", 200)
	mock := &chunkModerationTestServer{
		scores:  map[string]float64{"violence": 0.01},
		perCall: map[string]map[string]float64{},
	}
	svc, _ := newChunkModerationService(t, mock, func(cfg *ContentModerationConfig) {
		cfg.ChunkModerationEnabled = true
		cfg.ChunkTokens = 250
		cfg.ChunkMaxChunks = 64
		cfg.Thresholds = map[string]float64{"violence": 0.5}
	})

	// 让含 "risky" 的分片返回高分，其余低分。
	mockScoreByContent(mock, "risky", map[string]float64{"violence": 0.9})

	decision, err := svc.Check(context.Background(), ContentModerationCheckInput{
		Endpoint: "/v1/messages",
		Protocol: ContentModerationProtocolOpenAIChat,
		Body:     chunkTestBody(head + tail),
	})

	require.NoError(t, err)
	require.Equal(t, 0.9, decision.CategoryScores["violence"],
		"各分类应取所有分片的最高分")
	require.True(t, decision.Blocked, "任一分片超阈值即应拦截")
}
