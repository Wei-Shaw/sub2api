// Package service — support_doc_pipeline.go
//
// add-support-knowledge-rag 文档抓取管线（design.md D3）。
//
// 工作流：
//
//  1. 取 PG advisory lock（防止并发跑重复任务）；
//  2. 校验 doc_url 非空；
//  3. 从 root URL 开始 BFS：fetch → parse HTML → 抽 main 文本 → 切 chunks；
//  4. 同域名的 <a href> 链接收集到 todo 队列（depth 控制）；
//  5. 单次最多抓 SupportDocIndexHardPageCap 页（硬上限 50）；
//  6. 计算 chunks 的 sha256(content_hash) → 通过 UPSERT (ON CONFLICT DO NOTHING) 去重；
//  7. 每页处理完后 DeleteOrphans（清理"上次抓到、本次没出现"的旧 chunks）；
//  8. 批量 embed（100/批）：失败的 chunk 仍写入但 embedding=NULL；
//  9. 把状态序列化进 setting `support_chat_rag_doc_index_status`。
//
// 依赖：EmbeddingService（embed batch）+ SupportDocChunkRepository（持久化）+
//       *SettingService（读 RAG runtime + 写 status setting）+ *sql.DB（advisory lock）。
//
// 异步执行模型：admin handler 调 RunAsync(ctx) 后立即返回，本服务起 goroutine 实际跑；
// goroutine 内 advisory lock 拿不到 → 直接退出（前一次还没跑完）；
// 同步路径（cron 调用）走 Run(ctx)，错误冒泡。
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"database/sql"

	"golang.org/x/net/html"
)

// SupportDocPipeline 是 doc 抓取/索引管线。
type SupportDocPipeline struct {
	settings    *SettingService
	embedding   EmbeddingService
	chunkRepo   SupportDocChunkRepository
	sqlDB       *sql.DB
	httpClient  *http.Client
	now         func() time.Time
	pageCap     int
	chunkMinLen int

	mu      sync.Mutex
	running bool // 进程内并发守卫；advisory lock 是跨进程守卫，两者并存。
}

// NewSupportDocPipeline 构造 pipeline。
func NewSupportDocPipeline(
	settings *SettingService,
	embedding EmbeddingService,
	chunkRepo SupportDocChunkRepository,
	sqlDB *sql.DB,
) *SupportDocPipeline {
	return &SupportDocPipeline{
		settings:    settings,
		embedding:   embedding,
		chunkRepo:   chunkRepo,
		sqlDB:       sqlDB,
		httpClient:  &http.Client{Timeout: SupportDocFetchTimeout},
		now:         time.Now,
		pageCap:     SupportDocIndexHardPageCap,
		chunkMinLen: SupportDocChunkMinChars,
	}
}

// Run 同步执行一次完整抓取 + 索引。返回最终 status；任何 error 都已写入 status.Errors。
//
// 错误优先级（影响最终 state 字段）：
//   - advisory lock 失败 → 返回 ErrSupportDocIndexAlreadyRunning，state 不写。
//   - doc_url 空 → state="failed"，errors 含 SUPPORT_DOC_URL_EMPTY。
//   - root fetch 失败 → state="failed"，errors 含 fetch_failed。
//   - 单页失败 → 不影响整体 state；errors 累加。
func (p *SupportDocPipeline) Run(ctx context.Context) (SupportDocIndexStatus, error) {
	startedAt := p.now()
	status := SupportDocIndexStatus{
		State:     SupportDocIndexStateRunning,
		StartedAt: startedAt,
		Errors:    make([]SupportDocIndexError, 0, 4),
	}

	// 1. 进程内单实例守卫。
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return status, ErrSupportDocIndexAlreadyRunning
	}
	p.running = true
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.running = false
		p.mu.Unlock()
	}()

	// 2. PG advisory lock（跨进程守卫）。
	conn, err := p.sqlDB.Conn(ctx)
	if err != nil {
		return p.failStatus(ctx, status, fmt.Errorf("acquire conn: %w", err))
	}
	defer func() { _ = conn.Close() }()

	var locked bool
	if err := conn.QueryRowContext(ctx,
		`SELECT pg_try_advisory_lock($1)`,
		SupportDocIndexAdvisoryLockKey,
	).Scan(&locked); err != nil {
		return p.failStatus(ctx, status, fmt.Errorf("advisory lock: %w", err))
	}
	if !locked {
		return status, ErrSupportDocIndexAlreadyRunning
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(),
			`SELECT pg_advisory_unlock($1)`, SupportDocIndexAdvisoryLockKey)
	}()

	// 3. 读 RAG runtime（doc_url / depth / chunk_size / chunk_overlap）。
	rt := p.settings.GetSupportChatRAGRuntime(ctx)
	root := strings.TrimSpace(rt.DocURL)
	if root == "" {
		status.Errors = append(status.Errors, SupportDocIndexError{
			URL: "", Message: "support_doc_url empty"})
		status.State = SupportDocIndexStateFailed
		status.LastRunAt = p.now()
		status.DurationSeconds = int(p.now().Sub(startedAt).Seconds())
		p.persistStatus(ctx, status)
		return status, ErrSupportDocURLEmpty
	}

	rootURL, err := url.Parse(root)
	if err != nil || (rootURL.Scheme != "http" && rootURL.Scheme != "https") {
		status.Errors = append(status.Errors, SupportDocIndexError{
			URL: root, Message: "invalid url scheme"})
		status.State = SupportDocIndexStateFailed
		status.LastRunAt = p.now()
		status.DurationSeconds = int(p.now().Sub(startedAt).Seconds())
		p.persistStatus(ctx, status)
		return status, fmt.Errorf("invalid doc url: %s", root)
	}

	// 4. BFS 抓取 + 切片 + embed + upsert。
	visited := map[string]struct{}{root: {}}
	queue := []string{root}

	for len(queue) > 0 {
		if len(visited) > p.pageCap && status.PagesCapHit {
			break
		}

		currentURL := queue[0]
		queue = queue[1:]
		status.PagesVisited++

		// 单页处理超时由 httpClient.Timeout 控制；context 也透传以便 caller 取消。
		htmlBytes, fetchErr := p.fetchURL(ctx, currentURL)
		if fetchErr != nil {
			status.Errors = append(status.Errors, SupportDocIndexError{
				URL: currentURL, Message: "fetch_failed: " + fetchErr.Error()})
			continue
		}

		text, links, parseErr := parseSupportDocHTML(htmlBytes, rootURL)
		if parseErr != nil {
			status.Errors = append(status.Errors, SupportDocIndexError{
				URL: currentURL, Message: "parse_failed: " + parseErr.Error()})
			continue
		}

		// 切片 + 去噪：filter empty / too-short。
		chunks := chunkSupportDocText(text, rt.ChunkSize, rt.ChunkOverlap, p.chunkMinLen)
		if len(chunks) == 0 {
			// 空页面也尝试清掉旧 chunks（"页面内容被删空"语义）。
			removed, _ := p.chunkRepo.DeleteOrphans(ctx, currentURL, nil)
			status.ChunksRemoved += removed
		} else {
			added, failed, removed := p.indexPageChunks(ctx, currentURL, chunks)
			status.ChunksAdded += added
			status.ChunksFailedEmbed += failed
			status.ChunksRemoved += removed
		}

		// 收集同域链接（depth 控制）。
		if rt.DocDepth >= 1 {
			for _, link := range links {
				if _, ok := visited[link]; ok {
					continue
				}
				if len(visited) >= p.pageCap {
					if !status.PagesCapHit {
						status.PagesCapHit = true
						status.Errors = append(status.Errors, SupportDocIndexError{
							URL: link,
							Message: fmt.Sprintf("page cap %d reached, skipping further links",
								p.pageCap),
						})
					}
					break
				}
				visited[link] = struct{}{}
				queue = append(queue, link)
			}
		}
	}

	// 5. 写 chunks_total + 状态。
	if total, cerr := p.chunkRepo.CountAll(ctx); cerr == nil {
		status.ChunksTotal = total
	}
	status.State = SupportDocIndexStateCompleted
	status.LastRunAt = p.now()
	status.DurationSeconds = int(p.now().Sub(startedAt).Seconds())
	p.persistStatus(ctx, status)
	return status, nil
}

// RunAsync 起一个 goroutine 跑 Run；admin "立即重建" 按钮专用。
//
// 调用立即返回；不等待 pipeline 完成。advisory lock 由 goroutine 内部 Run 处理。
// 错误（包括 already-running）只 log，不冒泡（admin 通过 status 接口轮询查看）。
func (p *SupportDocPipeline) RunAsync() {
	go func() {
		// 用独立的 background context；admin handler 的请求 ctx 在响应返回时就 done 了。
		bgctx := context.Background()
		_, err := p.Run(bgctx)
		if err != nil {
			slog.WarnContext(bgctx, "support_doc_pipeline: async run finished with error",
				slog.Any("err", err))
		}
	}()
}

// failStatus 把 err 写进 status.Errors，state=failed，持久化后返回。
func (p *SupportDocPipeline) failStatus(ctx context.Context, status SupportDocIndexStatus, err error) (SupportDocIndexStatus, error) {
	status.Errors = append(status.Errors, SupportDocIndexError{
		URL: "", Message: err.Error()})
	status.State = SupportDocIndexStateFailed
	status.LastRunAt = p.now()
	status.DurationSeconds = int(p.now().Sub(status.StartedAt).Seconds())
	p.persistStatus(ctx, status)
	return status, err
}

// persistStatus 把 status JSON 编码后写入 setting key（覆盖写）。
//
// 失败只 log，不冒泡 —— pipeline 已经做完工作，admin 看不到状态最多是体验问题。
func (p *SupportDocPipeline) persistStatus(ctx context.Context, status SupportDocIndexStatus) {
	if p.settings == nil {
		return
	}
	if err := p.settings.SetSupportDocIndexStatus(ctx, status); err != nil {
		slog.WarnContext(ctx, "support_doc_pipeline: persist status failed",
			slog.Any("err", err))
	}
}

// indexPageChunks 处理一页的 chunks：批量 embed + upsert + 删除孤儿。
// 返回 (added, failedEmbed, removed)。
func (p *SupportDocPipeline) indexPageChunks(
	ctx context.Context,
	pageURL string,
	chunks []string,
) (added, failedEmbed, removed int) {
	hashes := make([]string, 0, len(chunks))
	hashToText := make(map[string]string, len(chunks))
	for _, c := range chunks {
		h := sha256Hex(c)
		if _, dup := hashToText[h]; dup {
			continue // 同一页面内 hash 撞了，去重
		}
		hashToText[h] = c
		hashes = append(hashes, h)
	}

	// 批量 embed（分批 100）。
	texts := make([]string, len(hashes))
	for i, h := range hashes {
		texts[i] = hashToText[h]
	}

	vecs := make([][]float32, len(texts))
	// 凭据缺失 short-circuit：直接读 SettingService 探一下 base_url + api_key，
	// 任一为空就跳过整页 chunks 的 HTTP 调用，全部走 embedding=NULL 路径。
	// 避免每个 batch 都白白发请求。由 change-support-chat-external-llm 引入。
	credBaseURL, credAPIKey := p.settings.GetSupportChatLLMCredentials(ctx)
	if credBaseURL == "" || credAPIKey == "" {
		slog.WarnContext(ctx, "support_doc_pipeline: embedding credentials missing, persisting all chunks with NULL embeddings",
			slog.String("page", pageURL),
			slog.Int("chunks", len(texts)))
		failedEmbed = len(texts)
		// vecs 全 nil → 后续 UpsertChunk 写 embedding=NULL。
	} else {
		for i := 0; i < len(texts); i += SupportDocEmbedBatchSize {
			end := i + SupportDocEmbedBatchSize
			if end > len(texts) {
				end = len(texts)
			}
			batch := texts[i:end]
			batchVecs, err := p.embedding.EmbedBatch(ctx, batch)
			if err != nil {
				// 整批失败：所有 chunk 走 nil 向量路径（仍持久化，embedding=NULL）。
				slog.WarnContext(ctx, "support_doc_pipeline: embed batch failed, persisting with NULL embeddings",
					slog.String("page", pageURL),
					slog.Int("batch_size", end-i),
					slog.Any("err", err))
				for j := i; j < end; j++ {
					failedEmbed++
					vecs[j] = nil
				}
				continue
			}
			for j := 0; j < len(batchVecs); j++ {
				vecs[i+j] = batchVecs[j]
			}
		}
	}

	// Upsert 全部 chunks。
	for i, h := range hashes {
		inserted, err := p.chunkRepo.UpsertChunk(ctx, pageURL, h, hashToText[h], vecs[i])
		if err != nil {
			slog.WarnContext(ctx, "support_doc_pipeline: upsert chunk failed",
				slog.String("page", pageURL),
				slog.String("hash", h),
				slog.Any("err", err))
			continue
		}
		if inserted {
			added++
		}
	}

	// 删除该 URL 下"本次没出现"的旧 chunks。
	rmCount, err := p.chunkRepo.DeleteOrphans(ctx, pageURL, hashes)
	if err != nil {
		slog.WarnContext(ctx, "support_doc_pipeline: delete orphans failed",
			slog.String("page", pageURL),
			slog.Any("err", err))
	} else {
		removed = rmCount
	}
	return added, failedEmbed, removed
}

// fetchURL HTTP GET 一个页面，返回 body 字节（≤ 5 MiB）。
//
// User-Agent 固定为 SupportDocFetchUserAgent；非 200 状态码视作错误。
func (p *SupportDocPipeline) fetchURL(ctx context.Context, target string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", SupportDocFetchUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	const maxBytes = 5 * 1024 * 1024
	return io.ReadAll(io.LimitReader(resp.Body, maxBytes))
}

// sha256Hex 计算 sha256 并十六进制编码（64 字符；与 schema CHAR(64) 对齐）。
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// parseSupportDocHTML 解析 HTML：
//   - 抽取 main / article / body 中的可见文本（去除 script/style/nav/footer）；
//   - 在 h1/h2/h3 标题前插入 markdown 风格 `# / ## / ###` + 换行（chunk 切片时识别）；
//   - 收集 <a href> 中同域名链接（带 query/fragment 去除，path 归一）。
//
// 返回 (text, sameDomainLinks, error)。
func parseSupportDocHTML(htmlBytes []byte, base *url.URL) (string, []string, error) {
	root, err := html.Parse(strings.NewReader(string(htmlBytes)))
	if err != nil {
		return "", nil, err
	}

	var (
		buf      strings.Builder
		linksSet = make(map[string]struct{}, 16)
		walk     func(*html.Node)
	)

	skipTag := map[string]bool{
		"script": true, "style": true, "noscript": true,
		"nav": true, "footer": true, "header": true,
		"aside": true, "form": true,
	}

	headingPrefix := map[string]string{
		"h1": "\n\n# ",
		"h2": "\n\n## ",
		"h3": "\n\n### ",
		"h4": "\n\n#### ",
	}

	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			tag := strings.ToLower(n.Data)
			if skipTag[tag] {
				return
			}
			if prefix, ok := headingPrefix[tag]; ok {
				buf.WriteString(prefix)
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				buf.WriteString("\n\n")
				return
			}
			if tag == "br" {
				buf.WriteByte('\n')
			}
			if tag == "p" || tag == "li" || tag == "div" {
				buf.WriteByte('\n')
			}
			if tag == "a" {
				for _, attr := range n.Attr {
					if strings.EqualFold(attr.Key, "href") {
						if u, err := base.Parse(strings.TrimSpace(attr.Val)); err == nil {
							if sameHost(u, base) && (u.Scheme == "http" || u.Scheme == "https") {
								u.Fragment = ""
								// 标准化：去掉 trailing slash 一次性策略
								norm := u.String()
								linksSet[norm] = struct{}{}
							}
						}
						break
					}
				}
			}
		} else if n.Type == html.TextNode {
			buf.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)

	// 收敛连续空白。
	text := collapseWhitespace(buf.String())

	links := make([]string, 0, len(linksSet))
	for u := range linksSet {
		links = append(links, u)
	}
	return text, links, nil
}

func sameHost(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Host, b.Host)
}

// collapseWhitespace 把连续空白（含换行）压缩到 ≤ 2 个换行；移除行首尾空格。
func collapseWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevNL := 0
	prevSpace := false
	for _, r := range s {
		switch r {
		case '\n':
			if prevNL < 2 {
				b.WriteByte('\n')
				prevNL++
			}
			prevSpace = false
		case ' ', '\t', '\r':
			if !prevSpace && prevNL == 0 {
				b.WriteByte(' ')
				prevSpace = true
			}
		default:
			b.WriteRune(r)
			prevNL = 0
			prevSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}

// chunkSupportDocText 按 markdown headings 优先切片；段过长按 chunkSize 硬切，overlap 衔接。
//
// 算法（与 design D3 一致）：
//  1. 按双换行 `\n\n` 切成 paragraph 单元。
//  2. 同一 heading 之下（直到下一 heading）的多个 paragraphs 合并成一个 section。
//  3. section 长度 > chunkSize 时按 chunkSize 硬切，overlap 取上一片末尾 chunkOverlap 字符。
//  4. 长度 < minChars 的 chunk 丢弃（design：太短没语义）。
//
// 全部以 rune 计数，避免在多字节字符中间切断。
func chunkSupportDocText(text string, chunkSize, chunkOverlap, minChars int) []string {
	if chunkSize <= 0 {
		chunkSize = SupportChatRAGChunkSizeDefault
	}
	if chunkOverlap < 0 || chunkOverlap >= chunkSize {
		chunkOverlap = SupportChatRAGChunkOverlapDefault
	}
	if minChars <= 0 {
		minChars = SupportDocChunkMinChars
	}

	paragraphs := strings.Split(text, "\n\n")
	var sections []string
	var currentSection strings.Builder
	flushSection := func() {
		s := strings.TrimSpace(currentSection.String())
		if s != "" {
			sections = append(sections, s)
		}
		currentSection.Reset()
	}

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// 新 heading 分节。
		if strings.HasPrefix(p, "# ") || strings.HasPrefix(p, "## ") || strings.HasPrefix(p, "### ") {
			flushSection()
		}
		if currentSection.Len() > 0 {
			currentSection.WriteString("\n\n")
		}
		currentSection.WriteString(p)
	}
	flushSection()

	// section -> chunks（硬切 + overlap）。
	out := make([]string, 0, len(sections))
	for _, sec := range sections {
		runes := []rune(sec)
		if len(runes) <= chunkSize {
			if len(runes) >= minChars {
				out = append(out, sec)
			}
			continue
		}
		i := 0
		for i < len(runes) {
			end := i + chunkSize
			if end > len(runes) {
				end = len(runes)
			}
			piece := strings.TrimSpace(string(runes[i:end]))
			if len([]rune(piece)) >= minChars {
				out = append(out, piece)
			}
			if end == len(runes) {
				break
			}
			i = end - chunkOverlap
			if i < 0 {
				i = 0
			}
		}
	}
	return out
}
