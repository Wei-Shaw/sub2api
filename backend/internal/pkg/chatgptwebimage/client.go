// Package chatgptwebimage implements the opt-in ChatGPT web image transport.
// It does not own account credentials, refresh tokens, scheduling, or cooldowns.
package chatgptwebimage

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/imroc/req/v3"
	_ "golang.org/x/image/webp"
	"golang.org/x/net/publicsuffix"
)

const (
	defaultBaseURL       = "https://chatgpt.com"
	userAgent            = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	maxJSONBytes   int64 = 8 << 20
	maxStreamBytes int64 = 32 << 20
	maxPixels      int64 = 25_000_000
)

// Options configure transport limits. HTTPClient and BaseURL are injection points
// for tests; production should use ProxyURL and the default ChatGPT origin.
// HTTPClient is copied, and each Generate call creates an isolated cookie jar.
type Options struct {
	ProxyURL      string
	HTTPClient    *http.Client
	BaseURL       string
	Timeout       time.Duration
	PollInterval  time.Duration
	MaxPolls      int
	MaxImageBytes int64
}

type GenerateRequest struct {
	AccessToken string
	Prompt      string
	// AllowRegularChat explicitly accepts creation of a normal, retained web
	// conversation. This provider cannot honor a temporary/no-history session.
	AllowRegularChat bool
}

type Image struct {
	Bytes  []byte
	Format string
	Width  int
	Height int
}

type Result struct {
	Images    []Image
	WebDriver string
	// ObservedImageModel stays empty unless an explicit image-model field is
	// returned by upstream. A chat driver or a caller alias is not an image model.
	ObservedImageModel string
	ConversationID     string
}

// Error deliberately omits upstream bodies, signed URLs, and credentials.
// Accepted means generation may have started: the caller must not blindly retry
// or fail over to another account. A missing ConversationID does not prove that
// upstream rejected the request (for example, a network timeout during POST).
type Error struct {
	Kind           string
	Stage          string
	StatusCode     int
	Accepted       bool
	ConversationID string
	RetryAfter     time.Duration
	cause          error
}

func (e *Error) Error() string {
	d := e.Diagnostic()
	return fmt.Sprintf("chatgpt web image %s: %s (HTTP %d, accepted=%t)", d.Stage, d.Kind, d.StatusCode, d.Accepted)
}

func (e *Error) Unwrap() error { return e.cause }

type Client struct {
	opts Options
	base *url.URL
	http *http.Client
}

func New(options Options) (*Client, error) {
	if options.BaseURL == "" {
		options.BaseURL = defaultBaseURL
	}
	base, err := url.Parse(strings.TrimRight(options.BaseURL, "/"))
	if err != nil || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Path != "" && base.Path != "/") {
		return nil, errors.New("chatgpt web image: invalid base origin")
	}
	if base.Scheme != "https" && (options.HTTPClient == nil || base.Scheme != "http") {
		return nil, errors.New("chatgpt web image: HTTPS is required")
	}
	if options.Timeout == 0 {
		options.Timeout = 4 * time.Minute
	}
	if options.PollInterval == 0 {
		options.PollInterval = 3 * time.Second
	}
	if options.MaxPolls == 0 {
		options.MaxPolls = 40
	}
	if options.MaxImageBytes == 0 {
		options.MaxImageBytes = 16 << 20
	}
	if options.Timeout < 0 || options.Timeout > 10*time.Minute || options.PollInterval < 0 || options.MaxPolls < 0 || options.MaxPolls > 100 || options.MaxImageBytes < 1 || options.MaxImageBytes > 64<<20 {
		return nil, errors.New("chatgpt web image: invalid request limits")
	}
	var hc http.Client
	if options.HTTPClient != nil {
		hc = *options.HTTPClient
	} else {
		rc := req.C().ImpersonateChrome()
		if options.ProxyURL != "" {
			proxy, parseErr := url.Parse(options.ProxyURL)
			if parseErr != nil || proxy.Host == "" || (proxy.Scheme != "http" && proxy.Scheme != "https" && proxy.Scheme != "socks5" && proxy.Scheme != "socks5h") {
				return nil, errors.New("chatgpt web image: invalid proxy URL")
			}
			rc.SetProxy(http.ProxyURL(proxy))
		}
		hc = *rc.GetClient()
	}
	// Inspect every redirect ourselves; automatic redirects must never forward
	// an account Authorization header, Sentinel token, or cookie to a CDN.
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	hc.Timeout = options.Timeout
	hc.Jar = nil
	base.Path = ""
	return &Client{opts: options, base: base, http: &hc}, nil
}

// CloseIdleConnections releases idle transport connections when a caller creates
// a short-lived provider. It does not interrupt an active generation.
func (c *Client) CloseIdleConnections() { c.http.CloseIdleConnections() }

type session struct {
	client   *Client
	http     *http.Client
	public   *http.Client
	headers  http.Header
	result   Result
	accepted bool
}

func (c *Client) Generate(ctx context.Context, input GenerateRequest) (Result, error) {
	if !input.AllowRegularChat || strings.TrimSpace(input.AccessToken) == "" || strings.TrimSpace(input.Prompt) == "" || len(input.Prompt) > 64<<10 || strings.ContainsAny(input.AccessToken, "\r\n") {
		return Result{}, &Error{Kind: "invalid_request", Stage: "validate"}
	}
	ctx, cancel := context.WithTimeout(ctx, c.opts.Timeout)
	defer cancel()
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return Result{}, err
	}
	hc := *c.http
	hc.Jar = jar
	public := *c.http
	public.Jar = nil
	s := &session{client: c, http: &hc, public: &public, result: Result{WebDriver: "auto"}, headers: make(http.Header)}
	s.headers.Set("Authorization", "Bearer "+input.AccessToken)
	s.headers.Set("User-Agent", userAgent)
	s.headers.Set("Origin", c.base.String())
	s.headers.Set("Referer", c.base.String()+"/")
	s.headers.Set("Accept", "application/json")
	s.headers.Set("Accept-Language", "en-US,en;q=0.9")
	s.headers.Set("Oai-Language", "en-US")
	s.headers.Set("Oai-Device-Id", uuid.NewString())
	if err = s.authenticate(ctx); err != nil {
		return s.result, err
	}
	if err = s.generate(ctx, input.Prompt); err != nil {
		return s.result, err
	}
	return s.result, nil
}

func (s *session) failure(stage, kind string, status int, cause error) *Error {
	if errors.Is(cause, context.DeadlineExceeded) || errors.Is(cause, context.Canceled) {
		kind = "timeout_or_canceled"
	}
	return &Error{Kind: kind, Stage: stage, StatusCode: status, Accepted: s.accepted, ConversationID: s.result.ConversationID, cause: cause}
}

func (s *session) request(ctx context.Context, method, path string, payload any, extra http.Header) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(encoded)
	}
	r, err := http.NewRequestWithContext(ctx, method, s.client.base.String()+path, body)
	if err != nil {
		return nil, err
	}
	r.Header = s.headers.Clone()
	if payload != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	for k, vv := range extra {
		r.Header[k] = vv
	}
	return s.http.Do(r)
}

func (s *session) status(stage string, r *http.Response) *Error {
	kind := "upstream_http"
	if stage == "conversation" && r.StatusCode == http.StatusNotFound && isWebImageModelNotFound(r.Body) {
		kind = "model_not_found"
	}
	if r.StatusCode == 401 || r.StatusCode == 403 {
		kind = "authentication_or_challenge"
	}
	if r.StatusCode == 429 {
		kind = "rate_limited"
	}
	e := s.failure(stage, kind, r.StatusCode, nil)
	if seconds, err := time.ParseDuration(r.Header.Get("Retry-After") + "s"); err == nil && seconds > 0 {
		e.RetryAfter = seconds
	}
	if at, err := http.ParseTime(r.Header.Get("Retry-After")); err == nil {
		e.RetryAfter = max(0, time.Until(at))
	}
	return e
}

// Recognize only the fixed model error in known JSON error fields. Reading a
// bounded body here must never retain or expose arbitrary upstream messages.
func isWebImageModelNotFound(body io.Reader) bool {
	if body == nil {
		return false
	}
	data, err := readLimited(body, 64<<10)
	if err != nil {
		return false
	}
	var payload struct {
		Detail  json.RawMessage `json:"detail"`
		Message string          `json:"message"`
		Code    string          `json:"code"`
		Error   json.RawMessage `json:"error"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return false
	}
	if payload.Message == "Model not found" || payload.Code == "model_not_found" {
		return true
	}
	var detail string
	if json.Unmarshal(payload.Detail, &detail) == nil && detail == "Model not found" {
		return true
	}
	for _, raw := range []json.RawMessage{payload.Detail, payload.Error} {
		var fields struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		}
		if json.Unmarshal(raw, &fields) == nil && (fields.Message == "Model not found" || fields.Code == "model_not_found") {
			return true
		}
	}
	return false
}

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("response exceeds size limit")
	}
	return b, nil
}

func (s *session) jsonRequest(ctx context.Context, stage, method, path string, input any, output any) error {
	r, err := s.request(ctx, method, path, input, nil)
	if err != nil {
		return s.failure(stage, "transport", 0, err)
	}
	defer func() { _ = r.Body.Close() }()
	if r.StatusCode != http.StatusOK {
		return s.status(stage, r)
	}
	b, err := readLimited(r.Body, maxJSONBytes)
	if err != nil {
		return s.failure(stage, "invalid_response", r.StatusCode, err)
	}
	if err = json.Unmarshal(b, output); err != nil {
		return s.failure(stage, "invalid_response", r.StatusCode, err)
	}
	return nil
}

func (s *session) generate(ctx context.Context, prompt string) error {
	parentID := uuid.NewString()
	message := map[string]any{
		"id": uuid.NewString(), "author": map[string]any{"role": "user"},
		"content": map[string]any{"content_type": "text", "parts": []string{prompt}},
	}
	prepare := map[string]any{
		"action": "next", "fork_from_shared_post": false, "parent_message_id": parentID,
		"model": "auto", "client_prepare_state": "success", "timezone_offset_min": 0, "timezone": "UTC",
		"conversation_mode": map[string]any{"kind": "primary_assistant"}, "system_hints": []string{"picture_v2"},
		"partial_query": message, "supports_buffering": true, "supported_encodings": []string{"v1"},
		"client_contextual_info": map[string]any{"app_name": "chatgpt.com"},
	}
	var prepared struct {
		Conduit string `json:"conduit_token"`
	}
	if err := s.jsonRequest(ctx, "prepare", http.MethodPost, "/backend-api/f/conversation/prepare", prepare, &prepared); err != nil {
		return err
	}
	if prepared.Conduit == "" {
		return s.failure("prepare", "missing_conduit", 200, nil)
	}
	body := map[string]any{
		"action": "next", "messages": []any{message}, "parent_message_id": parentID, "model": "auto",
		"conversation_mode": map[string]any{"kind": "primary_assistant"}, "history_and_training_disabled": false,
		"force_use_sse": true, "system_hints": []string{"picture_v2"}, "supported_encodings": []string{"v1"},
		"client_prepare_state": "sent", "supports_buffering": true, "enable_message_followups": true,
		"force_parallel_switch": "auto", "timezone": "UTC", "timezone_offset_min": 0,
		"client_contextual_info": map[string]any{"app_name": "chatgpt.com"}, "paragen_cot_summary_display_override": "allow",
		"conversation_origin": nil, "force_paragen": false, "force_paragen_model_slug": "", "force_rate_limit": false,
		"paragen_stream_type_override": nil, "reset_rate_limits": false, "suggestions": []string{},
		"variant_purpose": "comparison_implicit", "websocket_request_id": uuid.NewString(),
	}
	headers := make(http.Header)
	headers.Set("Accept", "text/event-stream")
	headers.Set("X-Conduit-Token", prepared.Conduit)
	headers.Set("X-Oai-Turn-Trace-Id", uuid.NewString())
	// An interrupted POST has an unknown outcome. Mark it conservatively before
	// transport dispatch and never re-submit generation in this provider.
	s.accepted = true
	r, err := s.request(ctx, http.MethodPost, "/backend-api/f/conversation", body, headers)
	if err != nil {
		return s.failure("conversation", "transport", 0, err)
	}
	if r.StatusCode != http.StatusOK {
		defer func() { _ = r.Body.Close() }()
		// A definite client rejection did not accept a generation job.
		if r.StatusCode >= 400 && r.StatusCode < 500 {
			s.accepted = false
		}
		return s.status("conversation", r)
	}
	assets := make(map[string]asset)
	streamErr := s.readEvents(r.Body, assets)
	_ = r.Body.Close()
	if ctx.Err() != nil {
		return s.failure("conversation", "timeout_or_canceled", 200, ctx.Err())
	}
	// Even a truncated stream may have returned a conversation ID. Poll that
	// existing conversation instead of ever repeating its generation request.
	if len(assets) == 0 && s.result.ConversationID != "" {
		for attempt := 0; attempt < s.client.opts.MaxPolls; attempt++ {
			if attempt > 0 {
				timer := time.NewTimer(s.client.opts.PollInterval)
				select {
				case <-ctx.Done():
					timer.Stop()
					return s.failure("poll", "timeout_or_canceled", 0, ctx.Err())
				case <-timer.C:
				}
			}
			var payload any
			if err := s.jsonRequest(ctx, "poll", http.MethodGet, "/backend-api/conversation/"+url.PathEscape(s.result.ConversationID), nil, &payload); err != nil {
				return err
			}
			s.observe(payload, assets)
			if len(assets) > 0 {
				break
			}
		}
	}
	if len(assets) == 0 {
		if streamErr != nil {
			return s.failure("conversation", "invalid_stream", 200, streamErr)
		}
		return s.failure("result", "no_image", 200, nil)
	}
	return s.downloadImages(ctx, assets)
}

type asset struct{ kind, id string }

var (
	assetPattern = regexp.MustCompile(`(file-service|sediment)://([A-Za-z0-9_-]+)`)
	filePattern  = regexp.MustCompile(`\bfile_00000000[a-f0-9]{24}\b`)
)

func (s *session) observe(value any, assets map[string]asset) {
	switch item := value.(type) {
	case map[string]any:
		// Poll responses contain the submitted user message too. A file-like
		// string in that prompt is not evidence of a newly generated image.
		if author, ok := item["author"].(map[string]any); ok && author["role"] == "user" {
			return
		}
		for key, v := range item {
			if str, ok := v.(string); ok {
				if key == "conversation_id" && validID(str) {
					s.result.ConversationID = str
				}
				if key == "image_model" || key == "image_gen_model" || key == "image_generation_model" {
					if len(str) < 128 && !strings.ContainsAny(str, "\r\n") {
						s.result.ObservedImageModel = str
					}
				}
			}
			s.observe(v, assets)
		}
	case []any:
		for _, v := range item {
			s.observe(v, assets)
		}
	case string:
		for _, match := range assetPattern.FindAllStringSubmatch(item, 16) {
			if len(assets) < 16 {
				assets[match[1]+":"+match[2]] = asset{match[1], match[2]}
			}
		}
		for _, id := range filePattern.FindAllString(item, 16) {
			if len(assets) < 16 {
				assets["file-service:"+id] = asset{"file-service", id}
			}
		}
	}
}

func validID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' {
			continue
		}
		return false
	}
	return true
}

func (s *session) readEvents(body io.Reader, assets map[string]asset) error {
	limited := &io.LimitedReader{R: body, N: maxStreamBytes + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 32<<10), int(maxJSONBytes))
	var event []byte
	consume := func() bool {
		if len(event) == 0 {
			return false
		}
		data := bytes.TrimSpace(event)
		if bytes.Equal(data, []byte("[DONE]")) {
			event = nil
			return true
		}
		var payload any
		if json.Unmarshal(data, &payload) == nil {
			s.observe(payload, assets)
		}
		event = nil
		return false
	}
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			if consume() {
				return nil
			}
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(line[5:])
		// Some upstream frames are data-only lines without a blank delimiter.
		// Complete JSON can be consumed immediately; incomplete multiline JSON
		// stays buffered until the SSE event boundary.
		if len(event) > 0 {
			event = append(event, '\n')
		}
		event = append(event, data...)
		if len(event) > int(maxJSONBytes) {
			return errors.New("SSE event exceeds size limit")
		}
		if json.Valid(event) || bytes.Equal(event, []byte("[DONE]")) {
			if consume() {
				return nil
			}
		}
	}
	consume()
	if limited.N <= 0 {
		return errors.New("SSE stream exceeds size limit")
	}
	return scanner.Err()
}

func (s *session) downloadImages(ctx context.Context, assets map[string]asset) error {
	keys := make([]string, 0, len(assets))
	for key := range assets {
		keys = append(keys, key)
	}
	sort.Strings(keys) // Prefer file-service references, then sediment fallbacks.
	seen := make(map[[32]byte]bool)
	var lastErr error
	for _, key := range keys {
		item := assets[key]
		path := "/backend-api/files/" + url.PathEscape(item.id) + "/download"
		if item.kind == "sediment" {
			if s.result.ConversationID == "" {
				continue
			}
			path = "/backend-api/conversation/" + url.PathEscape(s.result.ConversationID) + "/attachment/" + url.PathEscape(item.id) + "/download"
		}
		var link struct {
			DownloadURL string `json:"download_url"`
			URL         string `json:"url"`
		}
		if err := s.jsonRequest(ctx, "download_lookup", http.MethodGet, path, nil, &link); err != nil {
			lastErr = err
			continue
		}
		if link.DownloadURL == "" {
			link.DownloadURL = link.URL
		}
		binary, err := s.download(ctx, link.DownloadURL)
		if err != nil {
			lastErr = err
			continue
		}
		digest := sha256.Sum256(binary)
		if seen[digest] {
			continue
		}
		config, format, err := image.DecodeConfig(bytes.NewReader(binary))
		if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > maxPixels {
			lastErr = s.failure("download", "invalid_image", 200, nil)
			continue
		}
		if format != "png" && format != "jpeg" && format != "webp" {
			lastErr = s.failure("download", "unsupported_image", 200, nil)
			continue
		}
		// Decode the full image: a header or HTML response is not a success.
		if _, _, err = image.Decode(bytes.NewReader(binary)); err != nil {
			lastErr = s.failure("download", "invalid_image", 200, nil)
			continue
		}
		seen[digest] = true
		s.result.Images = append(s.result.Images, Image{Bytes: binary, Format: format, Width: config.Width, Height: config.Height})
	}
	if len(s.result.Images) == 0 {
		if lastErr != nil {
			return lastErr
		}
		return s.failure("result", "no_image", 200, nil)
	}
	// The initial contract is n=1. Multiple references usually describe the same
	// file; hash deduplication above still validates the actual returned bytes.
	s.result.Images = s.result.Images[:1]
	return nil
}

func sameOrigin(a, b *url.URL) bool { return a.Scheme == b.Scheme && strings.EqualFold(a.Host, b.Host) }

func (s *session) allowedAssetURL(u *url.URL) bool {
	if u.User != nil || u.Fragment != "" {
		return false
	}
	if sameOrigin(u, s.client.base) {
		return strings.HasPrefix(u.Path, "/")
	}
	if u.Scheme != "https" || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return strings.HasSuffix(host, ".oaiusercontent.com") || strings.HasSuffix(host, ".blob.core.windows.net")
}

func (s *session) download(ctx context.Context, location string) ([]byte, error) {
	u, err := url.Parse(location)
	if err != nil || !s.allowedAssetURL(u) {
		return nil, s.failure("download", "untrusted_download_origin", 0, nil)
	}
	for redirects := 0; redirects <= 3; redirects++ {
		r, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, s.failure("download", "invalid_download_url", 0, nil)
		}
		hc := s.public
		r.Header.Set("User-Agent", userAgent)
		if sameOrigin(u, s.client.base) {
			r.Header = s.headers.Clone()
			hc = s.http
		}
		response, err := hc.Do(r)
		if err != nil {
			return nil, s.failure("download", "transport", 0, err)
		}
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			next, locationErr := response.Location()
			_ = response.Body.Close()
			if locationErr != nil || !s.allowedAssetURL(next) {
				return nil, s.failure("download", "untrusted_download_origin", response.StatusCode, nil)
			}
			u = next
			continue
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			return nil, s.status("download", response)
		}
		binary, err := readLimited(response.Body, s.client.opts.MaxImageBytes)
		_ = response.Body.Close()
		if err != nil {
			return nil, s.failure("download", "invalid_image", response.StatusCode, err)
		}
		return binary, nil
	}
	return nil, s.failure("download", "too_many_redirects", 0, nil)
}
