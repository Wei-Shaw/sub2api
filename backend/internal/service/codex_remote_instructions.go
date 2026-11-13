package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

const (
	// codexRemoteInstructionsTTL prompt 低频变化，6 小时足够新鲜。
	codexRemoteInstructionsTTL = 6 * time.Hour
	// codexRemoteInstructionsBackoff 拉取失败退避窗口：窗口内跳过远端段。
	codexRemoteInstructionsBackoff = 60 * time.Second
	// codexRemoteInstructionsTimeout 单次拉取超时（响应几十 KB，15s 足够）。
	codexRemoteInstructionsTimeout = 15 * time.Second
	// codexRemoteInstructionsBodyLimit models.json 约几十 KB，给 2MB 余量。
	codexRemoteInstructionsBodyLimit = 2 << 20
)

// codexRemoteModel / codexRemoteModelMessages 镜像 openai/codex
// codex-rs/models-manager/models.json 的最小契约：slug + base prompt。
type codexRemoteModel struct {
	Slug          string                   `json:"slug"`
	ModelMessages codexRemoteModelMessages `json:"model_messages"`
}

type codexRemoteModelMessages struct {
	InstructionsTemplate string `json:"instructions_template"`
}

// codexRemoteInstructionsSource 运行时同步 openai/codex 仓库的 models.json，
// 为 embed prompt 未覆盖的未知模型提供最新 base prompt（零发版依赖）。
// embed 链有明确匹配的模型完全走 embed，既有语义与测试不变式保持不变。
type codexRemoteInstructionsSource struct {
	mu        sync.Mutex
	table     map[string]string // slug → instructions_template
	fetchedAt time.Time
	lastFail  time.Time
	client    *http.Client
	// refreshing 标记后台刷新在途（锁内读写），避免每请求重复 spawn 刷新协程。
	refreshing bool
	// syncFetch 测试钩子：为 true 时在调用方同步拉取（可确定性断言）；
	// 生产为 false——后台刷新，请求路径零阻塞。
	syncFetch bool
}

// codexRemoteModelsURL codex-rs models.json 地址（包级变量，测试 stub 先例：
// chatgptCodexModelsURL）。指向 openai/codex main 分支。
var codexRemoteModelsURL = "https://raw.githubusercontent.com/openai/codex/main/codex-rs/models-manager/models.json"

// codexRemoteInstructions 单例经 sync.Once 惰性初始化（两个消费点均为包级
// 函数，无宿主 service 可挂载）；lookup 为包级变量，测试可整体替换。
var (
	codexRemoteInstructionsOnce sync.Once
	codexRemoteInstructionsInst *codexRemoteInstructionsSource
)

func defaultRemoteCodexInstructionsLookup(model string) (string, bool) {
	codexRemoteInstructionsOnce.Do(func() {
		codexRemoteInstructionsInst = newCodexRemoteInstructionsSource(nil)
	})
	return codexRemoteInstructionsInst.instructionsFor(model)
}

var remoteCodexInstructionsLookup = defaultRemoteCodexInstructionsLookup

// remoteCodexInstructionsFor 包级入口：slug 匹配远端 prompt。
func remoteCodexInstructionsFor(model string) (string, bool) {
	return remoteCodexInstructionsLookup(model)
}

// newCodexRemoteInstructionsSource 创建 source。client 为 nil 时用默认 client。
func newCodexRemoteInstructionsSource(client *http.Client) *codexRemoteInstructionsSource {
	return &codexRemoteInstructionsSource{client: client}
}

// instructionsFor 返回 model 对应的远端 prompt：
// slug 精确 → canonical 别名拼写 → normalizeKnownOpenAICodexModel 归一化。
func (s *codexRemoteInstructionsSource) instructionsFor(model string) (string, bool) {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return "", false
	}
	table, ok := s.ensureTable()
	if !ok {
		return "", false
	}
	slug, ok := matchCodexRemoteSlug(table, model)
	if !ok {
		return "", false
	}
	template := table[slug]
	if strings.TrimSpace(template) == "" {
		return "", false
	}
	return template, true
}

// matchCodexRemoteSlug 在远端 slug 表中定位 model：精确 → canonical 别名拼写
// → normalizeKnownOpenAICodexModel 归一化基名。
func matchCodexRemoteSlug(table map[string]string, model string) (string, bool) {
	if _, ok := table[model]; ok {
		return model, true
	}
	canonical := openai.CanonicalizeOpenAIModelAliasSpelling(model)
	if canonical != "" {
		if _, ok := table[canonical]; ok {
			return canonical, true
		}
	}
	if normalized := normalizeKnownOpenAICodexModel(model); normalized != "" {
		if _, ok := table[normalized]; ok {
			return normalized, true
		}
	}
	return "", false
}

// ensureTable 返回 slug→template 表。新鲜（TTL 内）直接返回；过期时**立即**
// 返回现有内容（可为空）并触发后台刷新——请求路径不阻塞在网络拉取上。
// 失败进入退避窗口（codexRemoteInstructionsBackoff），窗口内跳过刷新。
func (s *codexRemoteInstructionsSource) ensureTable() (map[string]string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.table) > 0 && time.Since(s.fetchedAt) < codexRemoteInstructionsTTL {
		return s.table, true
	}
	if !s.lastFail.IsZero() && time.Since(s.lastFail) < codexRemoteInstructionsBackoff {
		return s.table, len(s.table) > 0
	}
	if s.syncFetch {
		s.fetchLocked()
		return s.table, len(s.table) > 0
	}
	// 生产路径：立即返回现有内容，后台刷新（refreshing 标记去重并发触发）。
	if !s.refreshing {
		s.refreshing = true
		go s.refresh()
	}
	return s.table, len(s.table) > 0
}

// refresh 后台刷新入口（独立协程执行）。
func (s *codexRemoteInstructionsSource) refresh() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fetchLocked()
}

// fetchLocked 执行拉取并更新状态（须持锁）。成功且非空才更新表与 fetchedAt；
// 失败或解析出空表（schema 漂移）均记 lastFail 进入退避窗口，避免每请求
// 重新拉取。
func (s *codexRemoteInstructionsSource) fetchLocked() {
	s.refreshing = false
	table, err := s.fetch()
	if err != nil {
		s.lastFail = time.Now()
		slog.Warn("codex remote instructions unavailable, skipping remote source", "error", err)
		return
	}
	if len(table) == 0 {
		s.lastFail = time.Now()
		slog.Warn("codex remote instructions parsed empty (schema drift?)", "url", codexRemoteModelsURL)
		return
	}
	s.table = table
	s.fetchedAt = time.Now()
}

// fetch 拉取并解析 codex-rs models.json 为 slug→template 表。
func (s *codexRemoteInstructionsSource) fetch() (map[string]string, error) {
	client := s.client
	if client == nil {
		client = http.DefaultClient
	}
	client = &http.Client{Timeout: codexRemoteInstructionsTimeout, Transport: client.Transport}
	req, err := http.NewRequest(http.MethodGet, codexRemoteModelsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch codex remote instructions: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("codex remote instructions returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, codexRemoteInstructionsBodyLimit+1))
	if err != nil {
		return nil, fmt.Errorf("read codex remote instructions: %w", err)
	}
	if int64(len(body)) > codexRemoteInstructionsBodyLimit {
		return nil, fmt.Errorf("codex remote instructions exceeds %d bytes", codexRemoteInstructionsBodyLimit)
	}
	var manifest struct {
		Models []codexRemoteModel `json:"models"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("parse codex remote instructions: %w", err)
	}
	table := make(map[string]string, len(manifest.Models))
	for _, m := range manifest.Models {
		slug := strings.TrimSpace(m.Slug)
		template := strings.TrimSpace(m.ModelMessages.InstructionsTemplate)
		if slug == "" || template == "" {
			continue
		}
		table[slug] = template
	}
	return table, nil
}
