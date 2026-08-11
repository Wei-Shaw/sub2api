package service

// model_intro_doc_fetcher.go
//
// 背景：管理员在「模型介绍」编辑页要手填的东西非常多（标题、中英文介绍、
// 输入参数 schema、输出参数 schema、结果字段…）。而这些信息几乎都能在上游
// 平台的模型文档页（例如 fal.ai 的模型页）上找到。
//
// 于是提供一个"文档页抓取"能力：管理员填一个 URL，后端把该页面抓下来并抽成
// 纯文本返回给前端；前端再用管理员已选好的 API Key + chat 模型把纯文本解析成
// 表单 JSON，回填到编辑区供其修改后保存。
//
// 本文件只负责"抓 + 抽文本"这一步（AI 解析放在前端，直接复用网关自身的
// /v1/chat/completions，不额外增加后端对模型的依赖）。
//
// 安全性：
//   - 只允许 http / https；
//   - 复用 channel_monitor_ssrf.go 的 SSRF 防护（拒绝 loopback / 私网 /
//     link-local / 云元数据 host，并在 dial 层二次校验，防 DNS rebinding）；
//   - body 读取上限 3 MiB，抽出的文本上限 ModelIntroDocHardMaxChars 字符；
//   - 固定 25s 硬超时。

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	// modelIntroDocFetchTimeout 单页抓取硬超时。文档站偶有慢响应，给到 25s。
	modelIntroDocFetchTimeout = 25 * time.Second
	// modelIntroDocMaxBodyBytes HTTP body 读取上限，防止被超大页面打爆内存。
	modelIntroDocMaxBodyBytes = 3 * 1024 * 1024
	// ModelIntroDocDefaultMaxChars 抽取文本的默认字符上限（未指定 max_chars 时用）。
	// 40k 字符对绝大多数模型文档页足够，且能塞进主流 chat 模型的上下文。
	ModelIntroDocDefaultMaxChars = 40000
	// ModelIntroDocHardMaxChars 抽取文本的硬上限，调用方传再大也会被夹到这里。
	ModelIntroDocHardMaxChars = 120000
)

// 抓取阶段的错误分类：handler 用 errors.Is 决定返回 400 还是 502。
var (
	// ErrModelIntroDocInvalidURL URL 为空 / 无法解析 / scheme 非 http(s) / 缺 host。
	ErrModelIntroDocInvalidURL = errors.New("model intro doc: invalid url")
	// ErrModelIntroDocBlockedURL 目标指向内网/回环/云元数据，被 SSRF 策略拒绝。
	ErrModelIntroDocBlockedURL = errors.New("model intro doc: url blocked by security policy")
	// ErrModelIntroDocEmpty 页面抓到了但抽不出可用文本（例如纯 JS 渲染的空壳页）。
	ErrModelIntroDocEmpty = errors.New("model intro doc: no readable text extracted")
)

// modelIntroDocTitleRe 从原始 HTML 里粗提 <title>。
// 用正则而不是再解析一遍 DOM：title 结构极简单，没必要为它多花一次 html.Parse。
var modelIntroDocTitleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

// ModelIntroDocResult 一次抓取的结果。
type ModelIntroDocResult struct {
	// URL 实际抓取的 URL（已归一化，去掉 fragment）。
	URL string
	// Title 页面 <title>（可能为空）。
	Title string
	// Text 抽取出的纯文本（markdown 风格标题前缀，见 parseSupportDocHTML）。
	Text string
	// Length Text 的字符数（rune 计）。
	Length int
	// Truncated 是否因超过上限而被截断。
	Truncated bool
}

// ModelIntroDocFetcher 抓取公开文档页并抽成纯文本。无状态，可全局复用。
type ModelIntroDocFetcher struct {
	httpClient *http.Client
}

// NewModelIntroDocFetcher 构造 fetcher，内部使用带 SSRF 防护的 http.Client。
func NewModelIntroDocFetcher() *ModelIntroDocFetcher {
	return &ModelIntroDocFetcher{
		httpClient: newSSRFSafeHTTPClient(modelIntroDocFetchTimeout),
	}
}

// Fetch 抓取 rawURL 并返回抽取后的纯文本。
//
// maxChars <= 0 时用 ModelIntroDocDefaultMaxChars；超过 ModelIntroDocHardMaxChars
// 时夹到硬上限。截断按 rune 边界切，不会切碎多字节字符。
func (f *ModelIntroDocFetcher) Fetch(ctx context.Context, rawURL string, maxChars int) (*ModelIntroDocResult, error) {
	target, err := normalizeModelIntroDocURL(rawURL)
	if err != nil {
		return nil, err
	}

	// 提交时先做一次 host 校验：能在发请求前就明确拒绝内网目标，
	// 错误信息也比 dial 层的 AddrError 更友好。dial 层仍会再校验一次。
	blocked, lookupErr := isPrivateOrLoopbackHost(ctx, target.Hostname())
	if lookupErr != nil {
		return nil, fmt.Errorf("resolve %s: %w", target.Hostname(), lookupErr)
	}
	if blocked {
		return nil, ErrModelIntroDocBlockedURL
	}

	body, err := f.get(ctx, target.String())
	if err != nil {
		return nil, err
	}

	text, _, err := parseSupportDocHTML(body, target)
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, ErrModelIntroDocEmpty
	}

	limit := maxChars
	if limit <= 0 {
		limit = ModelIntroDocDefaultMaxChars
	}
	if limit > ModelIntroDocHardMaxChars {
		limit = ModelIntroDocHardMaxChars
	}
	runes := []rune(text)
	truncated := false
	if len(runes) > limit {
		runes = runes[:limit]
		truncated = true
		text = string(runes)
	}

	return &ModelIntroDocResult{
		URL:       target.String(),
		Title:     extractHTMLTitle(body),
		Text:      text,
		Length:    len(runes),
		Truncated: truncated,
	}, nil
}

// get 发起 GET 并读取 body（≤ modelIntroDocMaxBodyBytes）。非 2xx 视作错误。
func (f *ModelIntroDocFetcher) get(ctx context.Context, target string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", SupportDocFetchUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.8")
	req.Header.Set("Accept-Language", "en,zh-CN;q=0.8")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		// dial 层的 SSRF 拒绝也走到这里；归一到 Blocked 错误便于前端提示。
		if strings.Contains(err.Error(), "blocked by SSRF policy") {
			return nil, ErrModelIntroDocBlockedURL
		}
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream returned http %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, modelIntroDocMaxBodyBytes))
}

// normalizeModelIntroDocURL 校验并归一化用户输入的 URL：
//   - trim 空白；缺 scheme 时补 https://（管理员经常直接粘 "fal.ai/models/xxx"）
//   - 只允许 http / https
//   - 必须有 host
//   - 去掉 fragment（对服务端抓取毫无意义）
func normalizeModelIntroDocURL(rawURL string) (*url.URL, error) {
	s := strings.TrimSpace(rawURL)
	if s == "" {
		return nil, ErrModelIntroDocInvalidURL
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, ErrModelIntroDocInvalidURL
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, ErrModelIntroDocInvalidURL
	}
	u.Scheme = scheme
	if u.Host == "" || u.Hostname() == "" {
		return nil, ErrModelIntroDocInvalidURL
	}
	u.Fragment = ""
	return u, nil
}

// extractHTMLTitle 粗提 <title> 文本：去标签内实体转义、压缩空白、限长 300 字符。
func extractHTMLTitle(htmlBytes []byte) string {
	m := modelIntroDocTitleRe.FindSubmatch(htmlBytes)
	if len(m) < 2 {
		return ""
	}
	title := collapseWhitespace(html.UnescapeString(string(m[1])))
	title = strings.TrimSpace(strings.ReplaceAll(title, "\n", " "))
	if r := []rune(title); len(r) > 300 {
		title = string(r[:300])
	}
	return title
}
