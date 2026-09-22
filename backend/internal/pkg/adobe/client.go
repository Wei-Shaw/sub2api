package adobe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// 默认的超时与轮询间隔。视频比图像慢一个量级，故超时也放宽一个量级。
const (
	DefaultImageTimeout  = 180 * time.Second
	DefaultVideoTimeout  = 600 * time.Second
	DefaultPollInterval  = 3 * time.Second
	defaultSubmitTimeout = 60 * time.Second
	// submitAttempts 是 generate-async 提交在同一账号上的尝试次数，只用于 408/429 降载。
	// Adobe 用 408 + timeout_error / "system under load" 做降载，第一次立刻失败很常见。
	submitAttempts = 3
	// maxConsecutivePollFailures 是轮询连续遇到临时故障（网络错误、408/429/5xx）的容忍次数。
	// 任务已提交、credits 已在消耗，一次抖动就放弃会让 handler 换号重新生成；成功一次即清零。
	maxConsecutivePollFailures = 3
	// videoDownloadTimeout 是视频产物下载的单次超时；图片沿用 defaultSubmitTimeout。
	videoDownloadTimeout = 5 * time.Minute
)

// submitRetryWait 是两次提交之间的基础等待；按次翻倍（1.5s / 3s）。
// 单测把它置 0，避免给套件加秒级延迟。
var submitRetryWait = 1500 * time.Millisecond

// ClientConfig 是构造 Client 的参数。
type ClientConfig struct {
	// Identity 的零值字段会用 DefaultIdentity 补齐。
	Identity Identity
	// Transport 用于 Adobe API 调用（提交/轮询/上传/鉴权）；为空时按 Identity 与
	// ProxyURL 构造带 TLS 伪装的实现。
	Transport Transport
	// DownloadTransport 用于下载 presigned 产物；为空时用标准库实现。
	DownloadTransport Transport
	// ProxyURL 是账号级代理，仅在 Transport / DownloadTransport 未显式给出时生效。
	ProxyURL string
}

// Client 是一个 Adobe Firefly 直连客户端。
//
// 它不持有账号 token：token 由调用方按请求传入，因为账号池会在同一个 Client 配置下
// 轮换多个账号。
type Client struct {
	identity          Identity
	transport         Transport
	downloadTransport Transport
}

// NewClient 构造 Firefly 客户端。
func NewClient(cfg ClientConfig) *Client {
	identity := cfg.Identity.withDefaults()
	transport := cfg.Transport
	if transport == nil {
		transport = NewImpersonateTransport(identity, cfg.ProxyURL)
	}
	download := cfg.DownloadTransport
	if download == nil {
		download = NewPlainTransport(cfg.ProxyURL)
	}
	return &Client{identity: identity, transport: transport, downloadTransport: download}
}

// Identity 返回客户端使用的身份参数。
func (c *Client) Identity() Identity { return c.identity }

// headerBuilder 按写入顺序累积请求头。顺序本身是浏览器指纹的一部分，
// 不能交给 map 的随机迭代。
type headerBuilder struct {
	order  []string
	values map[string]string
}

func newHeaderBuilder() *headerBuilder {
	return &headerBuilder{values: make(map[string]string)}
}

func (b *headerBuilder) set(name, value string) *headerBuilder {
	lower := strings.ToLower(name)
	if _, exists := b.values[lower]; !exists {
		b.order = append(b.order, lower)
	}
	b.values[lower] = value
	return b
}

func (b *headerBuilder) build() (map[string]string, []string) {
	return b.values, b.order
}

// browserHeaders 是模拟浏览器发起跨站 fetch 时的固定头组。
func (c *Client) browserHeaders() *headerBuilder {
	return newHeaderBuilder().
		set("user-agent", c.identity.UserAgent).
		set("origin", c.identity.Origin).
		set("referer", c.identity.Referer).
		set("accept-language", "en-US,en;q=0.9").
		set("sec-ch-ua", c.identity.SecChUA).
		set("sec-ch-ua-mobile", "?0").
		set("sec-ch-ua-platform", c.identity.SecChUAPlatform).
		set("sec-fetch-site", "cross-site").
		set("sec-fetch-mode", "cors").
		set("sec-fetch-dest", "empty")
}

func (c *Client) submitHeaders(token, prompt, arpSessionID string) (map[string]string, []string) {
	// 顺序对齐 Firefly 前端 generate-async 抓包：自定义头（authorization /
	// content-type / x-api-key / x-arp / x-nonce）在 origin 与 sec-fetch 之前。
	// sec-ch-ua* 仍带上，因为 TLS 伪装是 Chrome，不能按 Camoufox Firefox 省略。
	arp := strings.TrimSpace(arpSessionID)
	if arp == "" {
		arp = BuildARPSessionID()
	}
	headers := newHeaderBuilder().
		set("user-agent", c.identity.UserAgent).
		set("accept", "*/*").
		set("accept-language", "en-US,en;q=0.9").
		set("referer", c.identity.Referer).
		set("authorization", "Bearer "+token).
		set("content-type", "application/json").
		set("x-api-key", c.identity.FireflyAPIKey).
		set("x-arp-session-id", arp)
	if nonce := BuildSubmitNonce(token, prompt); nonce != "" {
		headers.set("x-nonce", nonce)
	}
	headers.
		set("origin", c.identity.Origin).
		set("sec-ch-ua", c.identity.SecChUA).
		set("sec-ch-ua-mobile", "?0").
		set("sec-ch-ua-platform", c.identity.SecChUAPlatform).
		set("sec-fetch-dest", "empty").
		set("sec-fetch-mode", "cors").
		set("sec-fetch-site", "cross-site")
	return headers.build()
}

func (c *Client) pollHeaders(token string) (map[string]string, []string) {
	// bks-epo 轮询抓包不带 x-api-key / content-type。
	return newHeaderBuilder().
		set("user-agent", c.identity.UserAgent).
		set("accept", "*/*").
		set("accept-language", "en-US,en;q=0.9").
		set("referer", c.identity.Referer).
		set("authorization", "Bearer "+token).
		set("origin", c.identity.Origin).
		set("sec-ch-ua", c.identity.SecChUA).
		set("sec-ch-ua-mobile", "?0").
		set("sec-ch-ua-platform", c.identity.SecChUAPlatform).
		set("sec-fetch-dest", "empty").
		set("sec-fetch-mode", "cors").
		set("sec-fetch-site", "cross-site").
		build()
}

func (c *Client) uploadHeaders(token, mimeType string) (map[string]string, []string) {
	return c.browserHeaders().
		set("authorization", "Bearer "+token).
		set("x-api-key", c.identity.FireflyAPIKey).
		set("content-type", mimeType).
		set("accept", "application/json").
		build()
}

// UploadImage 上传一张参考图，返回 Adobe 侧的 image id（供 payload 的 referenceBlobs 引用）。
func (c *Client) UploadImage(ctx context.Context, token string, image []byte, mimeType string) (string, error) {
	if len(image) == 0 {
		return "", NewRequestError("image is empty")
	}
	if strings.TrimSpace(mimeType) == "" {
		mimeType = "image/jpeg"
	}

	headers, order := c.uploadHeaders(token, mimeType)
	resp, err := c.transport.Do(ctx, &Request{
		Method:      http.MethodPost,
		URL:         ImageUploadURL,
		Headers:     headers,
		HeaderOrder: order,
		Body:        image,
		Timeout:     defaultSubmitTimeout,
	})
	if err != nil {
		return "", err
	}
	if err := c.errorForStatus(resp, "upload image"); err != nil {
		return "", err
	}

	var payload struct {
		Images []struct {
			ID string `json:"id"`
		} `json:"images"`
	}
	if err := json.Unmarshal(resp.Body, &payload); err != nil || len(payload.Images) == 0 ||
		strings.TrimSpace(payload.Images[0].ID) == "" {
		return "", NewRequestError("upload image succeeded but no image id returned")
	}
	return payload.Images[0].ID, nil
}

// GenerateImageInput 是一次出图请求。
type GenerateImageInput struct {
	Token   string
	Options ImagePayloadOptions
	// ARPSessionID 是账号里保存的 Sherlock x-arp-session-id；空则回落到 BuildARPSessionID stub。
	ARPSessionID string
	// Timeout 是整个「提交 + 轮询」的上限；为空取 DefaultImageTimeout。
	Timeout time.Duration
	// PollInterval 为空取 DefaultPollInterval。
	PollInterval time.Duration
}

// GenerateResult 是一次生成的产物。
type GenerateResult struct {
	// Bytes 是下载好的图片/视频字节。
	Bytes []byte
	// Raw 是最后一次轮询返回的原始 JSON，供上层记录用量等信息。
	Raw map[string]any
}

// GenerateImage 走「提交 → 轮询 → 下载」完成一次出图。
//
// 提交阶段依次尝试 BuildImagePayloadCandidates 返回的候选，命中 200 即停；
// 遇到 401/403 立即中断——那是凭据问题，换 payload 形状无用。
// 408/429/451/5xx 是上游过载或故障，不是 schema 问题：不再换候选（重试策略见 postSubmit）。
func (c *Client) GenerateImage(ctx context.Context, input GenerateImageInput) (*GenerateResult, error) {
	candidates, err := BuildImagePayloadCandidates(input.Options)
	if err != nil {
		return nil, err
	}

	headers, order := c.submitHeaders(input.Token, input.Options.Prompt, input.ARPSessionID)
	var submitResp *Response
	for _, payload := range candidates {
		body, err := marshalPayloadJSON(payload)
		if err != nil {
			return nil, NewRequestError(fmt.Sprintf("marshal image payload: %v", err))
		}
		submitResp, err = c.postSubmit(ctx, ImageSubmitURL, headers, order, body)
		if err != nil {
			return nil, err
		}
		if submitResp.StatusCode == http.StatusOK {
			break
		}
		if isAuthStatus(submitResp.StatusCode) || IsRetryableStatus(submitResp.StatusCode) {
			break
		}
	}
	if submitResp == nil {
		return nil, NewRequestError("submit failed: no response")
	}
	// 错误信息与内容拒绝判断只看最后这次响应自己的 body：状态码与 body 必须来自同一次提交。
	if err := c.errorForSubmit(submitResp, "submit"); err != nil {
		return nil, err
	}

	pollURL, err := pollURLFromSubmit(submitResp, "submit")
	if err != nil {
		return nil, err
	}
	return c.poll(ctx, pollParams{
		token:        input.Token,
		pollURL:      NormalizePollURL(pollURL),
		label:        "image",
		outputKey:    "image",
		timeout:      orDuration(input.Timeout, DefaultImageTimeout),
		pollInterval: orDuration(input.PollInterval, DefaultPollInterval),
		maxDownload:  MaxImageDownloadBytes,
		downloadWait: defaultSubmitTimeout,
	})
}

// GenerateVideoInput 是一次视频生成请求。
type GenerateVideoInput struct {
	Token        string
	Options      VideoPayloadOptions
	// ARPSessionID 是账号里保存的 Sherlock x-arp-session-id；空则回落到 BuildARPSessionID stub。
	ARPSessionID string
	Timeout      time.Duration
	PollInterval time.Duration
}

// GenerateVideo 走「提交 → 轮询 → 下载」完成一次视频生成。
//
// 与图像不同，视频只发单个 payload：各引擎的形状由 Engine 确定，不存在候选回退。
func (c *Client) GenerateVideo(ctx context.Context, input GenerateVideoInput) (*GenerateResult, error) {
	body, err := marshalPayloadJSON(BuildVideoPayload(input.Options))
	if err != nil {
		return nil, NewRequestError(fmt.Sprintf("marshal video payload: %v", err))
	}

	headers, order := c.submitHeaders(input.Token, input.Options.Prompt, input.ARPSessionID)
	submitResp, err := c.postSubmit(ctx, VideoSubmitURL, headers, order, body)
	if err != nil {
		return nil, err
	}
	if err := c.errorForSubmit(submitResp, "video submit"); err != nil {
		return nil, err
	}

	rawPollURL, err := pollURLFromSubmit(submitResp, "video submit")
	if err != nil {
		return nil, err
	}
	return c.poll(ctx, pollParams{
		token:        input.Token,
		pollURL:      NormalizePollURL(rawPollURL),
		label:        "video",
		outputKey:    "video",
		timeout:      orDuration(input.Timeout, DefaultVideoTimeout),
		pollInterval: orDuration(input.PollInterval, DefaultPollInterval),
		maxDownload:  MaxVideoDownloadBytes,
		downloadWait: videoDownloadTimeout,
	})
}

type pollParams struct {
	token   string
	pollURL string
	// label 用于错误信息（image / video）。
	label string
	// outputKey 是 outputs[0] 下承载产物的键（image / video）。
	outputKey    string
	timeout      time.Duration
	pollInterval time.Duration
	// maxDownload / downloadWait 是产物下载的大小上限与单次超时。
	maxDownload  int64
	downloadWait time.Duration
}

func (c *Client) poll(ctx context.Context, params pollParams) (*GenerateResult, error) {
	// 轮询链接来自上游响应，且请求会带账号 token：只允许发往 adobe.io。
	if err := validateAPIURL(params.pollURL); err != nil {
		return nil, err
	}
	headers, order := c.pollHeaders(params.token)
	deadline := timeNow().Add(params.timeout)
	consecutiveFailures := 0

	for {
		resp, err := c.transport.Do(ctx, &Request{
			Method:      http.MethodGet,
			URL:         params.pollURL,
			Headers:     headers,
			HeaderOrder: order,
			Timeout:     defaultSubmitTimeout,
		})
		if err != nil {
			if !isTransientPollError(ctx, err) {
				return nil, err
			}
			consecutiveFailures++
			if consecutiveFailures > maxConsecutivePollFailures {
				return nil, err
			}
			if waitErr := waitNextPoll(ctx, deadline, params); waitErr != nil {
				return nil, waitErr
			}
			continue
		}
		switch {
		case resp.StatusCode == http.StatusOK, resp.StatusCode == http.StatusCreated:
		case resp.StatusCode == http.StatusAccepted:
			// 202：任务仍在排队/运行，没有可解析的结果。
			consecutiveFailures = 0
			if err := waitNextPoll(ctx, deadline, params); err != nil {
				return nil, err
			}
			continue
		case IsRetryableStatus(resp.StatusCode) && !IsContentRejectedBody(string(resp.Body)):
			consecutiveFailures++
			if consecutiveFailures > maxConsecutivePollFailures {
				return nil, c.errorForStatus(resp, params.label+" poll")
			}
			if err := waitNextPoll(ctx, deadline, params); err != nil {
				return nil, err
			}
			continue
		default:
			return nil, c.errorForStatus(resp, params.label+" poll")
		}
		consecutiveFailures = 0

		var latest map[string]any
		if err := json.Unmarshal(resp.Body, &latest); err != nil {
			latest = map[string]any{}
		}

		if mediaURL, found, err := presignedURL(latest, params.outputKey); err != nil {
			return nil, err
		} else if found {
			bytes, err := c.download(ctx, mediaURL, params.maxDownload, params.downloadWait)
			if err != nil {
				return nil, err
			}
			return &GenerateResult{Bytes: bytes, Raw: latest}, nil
		}

		if isTerminalJobStatus(jobStatus(latest, resp)) {
			body := truncate(string(resp.Body), maxErrorBodyBytes)
			message := fmt.Sprintf("%s job failed: %s", params.label, body)
			if IsContentRejectedBody(body) {
				return nil, NewContentRejectedError(message, resp.StatusCode,
					"Image content was rejected by the upstream safety filter")
			}
			jobErr := NewRequestError(message)
			jobErr.UserMessage = fmt.Sprintf("Adobe %s generation failed", params.label)
			return nil, jobErr
		}

		if err := waitNextPoll(ctx, deadline, params); err != nil {
			return nil, err
		}
	}
}

// isTransientPollError 判断轮询请求的传输错误是否值得在同一任务上继续轮询。
func isTransientPollError(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	var temporary *UpstreamTemporaryError
	return errors.As(err, &temporary)
}

// waitNextPoll 在两次轮询之间检查截止时间并等待 pollInterval；ctx 取消立即返回。
func waitNextPoll(ctx context.Context, deadline time.Time, params pollParams) error {
	if timeNow().After(deadline) {
		return NewRequestError(fmt.Sprintf("%s generation timed out", params.label))
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(params.pollInterval):
		return nil
	}
}

func (c *Client) download(ctx context.Context, mediaURL string, maxBytes int64, timeout time.Duration) ([]byte, error) {
	if err := validateDownloadURL(mediaURL); err != nil {
		return nil, err
	}
	resp, err := c.downloadTransport.Do(ctx, &Request{
		Method:       http.MethodGet,
		URL:          mediaURL,
		Headers:      map[string]string{"accept": "*/*"},
		Timeout:      orDuration(timeout, defaultSubmitTimeout),
		MaxBodyBytes: maxBytes,
	})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &RequestError{
			Message:    fmt.Sprintf("media download failed: HTTP %d", resp.StatusCode),
			StatusCode: resp.StatusCode,
			ErrorType:  ErrorTypeStatus,
		}
	}
	return resp.Body, nil
}

// postSubmit 发送 generate-async 提交，只对 Adobe 降载状态码（408/429）在同一账号上退避重试。
//
// 5xx/451 不在同一账号重试：请求可能已被上游受理，中间层才回了错误，同号重发会叠加
// 重复的付费任务；这类错误交给 handler 换号，最坏提交次数从 3×换号数降到换号数。
func (c *Client) postSubmit(
	ctx context.Context, rawURL string, headers map[string]string, order []string, body []byte,
) (*Response, error) {
	var last *Response
	for attempt := 1; attempt <= submitAttempts; attempt++ {
		resp, err := c.transport.Do(ctx, &Request{
			Method:      http.MethodPost,
			URL:         rawURL,
			Headers:     headers,
			HeaderOrder: order,
			Body:        body,
			Timeout:     defaultSubmitTimeout,
		})
		if err != nil {
			return nil, err
		}
		last = resp
		if !isSubmitLoadSheddingStatus(resp.StatusCode) || IsContentRejectedBody(string(resp.Body)) {
			return resp, nil
		}
		if attempt == submitAttempts {
			return resp, nil
		}
		if err := waitSubmitRetry(ctx, attempt); err != nil {
			return nil, err
		}
	}
	return last, nil
}

// isSubmitLoadSheddingStatus 报告提交响应是否为 Adobe 的降载拒绝（请求未被受理），可同号重试。
func isSubmitLoadSheddingStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests
}

func waitSubmitRetry(ctx context.Context, attempt int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	wait := submitRetryWait
	if wait <= 0 {
		return nil
	}
	if attempt > 1 {
		wait *= time.Duration(1 << (attempt - 1))
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// errorForSubmit 处理提交响应，把 401/403 细分成配额耗尽、权益不足与鉴权失效。
func (c *Client) errorForSubmit(resp *Response, label string) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if isAuthStatus(resp.StatusCode) {
		return authOrQuotaError(resp)
	}
	body := resp.BodyPreview()
	message := fmt.Sprintf("%s failed: %d %s", label, resp.StatusCode, body)
	return classifyAdobeHTTPError(resp.StatusCode, body, message)
}

func (c *Client) errorForStatus(resp *Response, label string) error {
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		return nil
	}
	if isAuthStatus(resp.StatusCode) {
		return authOrQuotaError(resp)
	}
	body := resp.BodyPreview()
	message := fmt.Sprintf("%s failed: %d %s", label, resp.StatusCode, body)
	return classifyAdobeHTTPError(resp.StatusCode, body, message)
}

// classifyAdobeHTTPError 把非鉴权 HTTP 失败分成：内容安全拒绝、可重试临时故障、终态 4xx。
func classifyAdobeHTTPError(status int, body, message string) error {
	if IsContentRejectedBody(body) {
		return NewContentRejectedError(message, status, "Image content was rejected by the upstream safety filter")
	}
	if IsRetryableStatus(status) {
		return NewUpstreamTemporaryError(message, status, ErrorTypeStatus)
	}
	// 上游 4xx 体可能含内部字段：原文只留在 Message（日志），对外给固定文案。
	requestErr := NewRequestError(message)
	requestErr.UserMessage = fmt.Sprintf("Adobe rejected the request (HTTP %d)", status)
	return requestErr
}

// authOrQuotaError 区分「配额耗尽」「权益不足」「token 失效」：三者共用 401/403，
// 但处置完全不同——冷却账号、换更高套餐号、刷新凭据，不能混为一谈。
func authOrQuotaError(resp *Response) error {
	accessError := resp.Header("x-access-error")
	if accessError == "taste_exhausted" {
		return NewQuotaExhaustedError("Adobe quota exhausted for this account", resp.StatusCode)
	}
	preview := resp.BodyPreview()
	if IsNotEntitledCode(accessError) || IsNotEntitledBody(preview) {
		message := "Adobe account is not entitled to the requested model or quality"
		if preview != "" {
			message = fmt.Sprintf("%s: %s", message, preview)
		}
		return NewNotEntitledError(message, resp.StatusCode, "")
	}
	message := "Token invalid or expired"
	if preview != "" {
		message += ": " + preview
	}
	authErr := NewAuthError(message, resp.StatusCode)
	authErr.UserMessage = "Adobe account authentication failed"
	return authErr
}

func isAuthStatus(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden
}

func pollURLFromSubmit(resp *Response, label string) (string, error) {
	var submitData map[string]any
	if err := json.Unmarshal(resp.Body, &submitData); err != nil {
		submitData = map[string]any{}
	}
	pollURL := ExtractResultLink(resp.Headers, submitData)
	if pollURL == "" {
		return "", NewRequestError(label + " succeeded but no poll url returned")
	}
	return pollURL, nil
}

// presignedURL 从轮询结果里取产物直链。outputs 非空但没有直链是上游异常，直接报错。
func presignedURL(latest map[string]any, outputKey string) (string, bool, error) {
	outputs, ok := latest["outputs"].([]any)
	if !ok || len(outputs) == 0 {
		return "", false, nil
	}
	first, ok := outputs[0].(map[string]any)
	if !ok {
		return "", false, NewRequestError("job finished with malformed outputs")
	}
	media, ok := first[outputKey].(map[string]any)
	if !ok {
		return "", false, NewRequestError(fmt.Sprintf("job finished without %s url", outputKey))
	}
	link, ok := media["presignedUrl"].(string)
	if !ok || strings.TrimSpace(link) == "" {
		return "", false, NewRequestError(fmt.Sprintf("job finished without %s url", outputKey))
	}
	return link, true, nil
}

// jobStatus 取任务状态，body 里没有时回落到响应头。
func jobStatus(latest map[string]any, resp *Response) string {
	if status, ok := latest["status"].(string); ok && strings.TrimSpace(status) != "" {
		return strings.ToUpper(status)
	}
	return strings.ToUpper(resp.Header("x-task-status"))
}

func isTerminalJobStatus(status string) bool {
	switch status {
	case "FAILED", "CANCELLED", "ERROR":
		return true
	default:
		return false
	}
}

// ExtractResultLink 取轮询地址：优先响应头 x-override-status-link，再取 body.links.result
// （后者可能是字符串，也可能是 {href} 对象）。
func ExtractResultLink(headers map[string]string, submitData map[string]any) string {
	if headers != nil {
		if link := strings.TrimSpace(headers["x-override-status-link"]); link != "" {
			return link
		}
	}
	links, ok := submitData["links"].(map[string]any)
	if !ok {
		return ""
	}
	switch result := links["result"].(type) {
	case string:
		return strings.TrimSpace(result)
	case map[string]any:
		href, _ := result["href"].(string)
		return strings.TrimSpace(href)
	default:
		return ""
	}
}

// shardPattern 匹配 firefly-epo 主机名里的四位分片号。
var shardPattern = regexp.MustCompile(`^\d{4}$`)

// NormalizePollURL 把 firefly-epo 分片链接转换成 Adobe 实际的任务查询地址。
//
// 提交 body.links.result 是 https://firefly-epo{shard}….adobe.io/jobs/result/{id}；
// 浏览器实际轮询 x-override-status-link：
// https://bks-epo{shard前4位}.adobe.io/v2/jobs/result/{id}?host={原主机}/
// 图像与视频同一套改写。不是该形态的链接原样返回。
func NormalizePollURL(rawURL string) string {
	if rawURL == "" {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	host := parsed.Host
	if host == "" || !strings.HasPrefix(host, "firefly-epo") || !isAdobeIOHost(parsed.Hostname()) {
		return rawURL
	}
	pathParts := strings.FieldsFunc(parsed.Path, func(r rune) bool { return r == '/' })
	if len(pathParts) == 0 {
		return rawURL
	}

	jobID := pathParts[len(pathParts)-1]
	hostSuffix, _, _ := strings.Cut(strings.TrimPrefix(host, "firefly-epo"), ".")
	shard := strings.TrimSpace(truncate(hostSuffix, 4))
	if jobID == "" || !shardPattern.MatchString(shard) {
		return rawURL
	}
	return fmt.Sprintf("https://bks-epo%s.adobe.io/v2/jobs/result/%s?host=%s/", shard, jobID, host)
}

// NormalizeVideoPollURL 是 NormalizePollURL 的别名，保留给既有调用方。
func NormalizeVideoPollURL(rawURL string) string { return NormalizePollURL(rawURL) }

func truncate(s string, limit int) string {
	if len(s) > limit {
		return s[:limit]
	}
	return s
}

func orDuration(value, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}
