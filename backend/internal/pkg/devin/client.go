// Adapted from Oh My Pi (MIT), commit 4049c62fbb0a7f5068954d01f11a73112c942a11,
// packages/ai/src/providers/devin.ts and packages/catalog/src/{discovery,wire}/devin.ts.
// See the repository root THIRD_PARTY_NOTICES.md for license and attribution.

// Package devin translates Chat Completions to Devin's subscription Connect API.
// All HTTP traffic must use the caller's injected, security-controlled transport.
package devin

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
)

const DefaultBaseURL = "https://server.codeium.com"
const ChatPath = "/exa.api_server_pb.ApiServerService/GetChatMessage"
const authPath = "/exa.auth_pb.AuthService/GetUserJwt"
const assignPath = "/exa.api_server_pb.ApiServerService/AssignModel"
const modelsPath = "/exa.api_server_pb.ApiServerService/GetCliModelConfigs"

type Model struct {
	ID                    string `json:"id"`
	DisplayName           string `json:"display_name"`
	UID                   string `json:"-"`
	MaxTokens             int    `json:"max_tokens,omitempty"`
	Router                bool   `json:"-"`
	SupportsImages        bool   `json:"-"`
	SupportsParallelTools bool   `json:"-"`
	// nil means the server did not declare tool capabilities (notably routers).
	supportsTools *bool
}

func isFusionModelID(id string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	return id == "fusion" || strings.HasPrefix(id, "fusion-")
}

func DefaultModels() []Model {
	return []Model{{ID: "swe-1-6", DisplayName: "SWE-1.6", UID: "swe-1-6", MaxTokens: 64000}, {ID: "swe-1-6-fast", DisplayName: "SWE-1.6 Fast", UID: "swe-1-6-fast", MaxTokens: 64000}}
}

type Client struct {
	Do func(*http.Request) (*http.Response, error)
	// ValidateBaseURL must enforce the gateway's SSRF/account policy. Custom auth
	// endpoints are rejected when it is absent. It returns the approved base URL.
	ValidateBaseURL func(string) (string, error)
}

type Error struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *Error) Error() string {
	return fmt.Sprintf("Devin %s (%d): %s", e.Code, e.StatusCode, e.Message)
}
func inputError(message string) *Error { return &Error{400, "invalid_argument", message} }
func protocolError() *Error {
	return &Error{502, "invalid_response", "Invalid or incomplete Devin response"}
}
func credentialsError() *Error {
	return &Error{401, "unauthenticated", "Devin credentials are missing or expired"}
}
func connectError(code string) *Error {
	status := 502
	switch code {
	case "unauthenticated":
		return credentialsError()
	case "permission_denied":
		status = 403
	case "resource_exhausted":
		status = 429
	case "unavailable":
		status = 503
	case "invalid_argument", "out_of_range":
		status = 400
	case "deadline_exceeded":
		status = 504
	case "not_found":
		status = 404
	case "unimplemented":
		status = 501
	case "canceled":
		status = 499
	case "internal", "unknown", "data_loss", "failed_precondition", "aborted":
	default:
		code = "upstream_error"
	}
	return &Error{status, code, "Devin rejected the request"}
}
func checkToken(token string) error {
	t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(token), "devin-session-token$"))
	if t == "" {
		return credentialsError()
	}
	return nil
}
func (c *Client) request(ctx context.Context, base, path string, body []byte, stream bool) (*http.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c == nil || c.Do == nil {
		return nil, &Error{500, "transport_unconfigured", "Devin transport is not configured"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(body))
	if err != nil {
		return nil, protocolError()
	}
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("Accept-Encoding", "identity")
	if stream {
		req.Header.Set("Content-Type", "application/connect+proto")
		req.Header.Set("Connect-Content-Encoding", "gzip")
		req.Header.Set("Connect-Accept-Encoding", "gzip")
		req.Header.Set("User-Agent", "connect-go/1.18.1 (go1.26.3)")
	} else {
		req.Header.Set("Content-Type", "application/proto")
		req.Header.Set("Accept", "*/*")
	}
	resp, err := c.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return nil, fmt.Errorf("Devin transport: %w", err)
	}
	if resp == nil {
		return nil, protocolError()
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.Body != nil {
			resp.Body.Close()
		}
		status := resp.StatusCode
		if status < 400 {
			status = 502
		}
		return nil, &Error{status, "upstream_http_error", "Devin HTTP request failed"}
	}
	if resp.Body == nil {
		return nil, protocolError()
	}
	return resp, nil
}
func (c *Client) unary(ctx context.Context, base, path string, body []byte) (protoMessage, error) {
	resp, err := c.request(ctx, base, path, body, false)
	if err != nil {
		return nil, err
	}
	var closeOnce sync.Once
	closeBody := func() { closeOnce.Do(func() { resp.Body.Close() }) }
	defer closeBody()
	stop := context.AfterFunc(ctx, closeBody)
	defer stop()
	b, err := boundedRead(resp.Body)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return decodeUnary(b)
}
func (c *Client) auth(ctx context.Context, token string) (jwt, base string, err error) {
	m, err := c.unary(ctx, DefaultBaseURL, authPath, pb(nil, 1, metadata(token, "", false)))
	if err != nil {
		return "", "", err
	}
	if err = m.check([]int32{1, 2}, nil); err != nil {
		return "", "", err
	}
	jwt = m.text(1)
	if strings.TrimSpace(jwt) == "" {
		return "", "", credentialsError()
	}
	base = DefaultBaseURL
	if custom := strings.TrimSpace(m.text(2)); custom != "" {
		if c.ValidateBaseURL == nil {
			return "", "", &Error{502, "unsafe_endpoint", "Devin custom endpoint requires validation"}
		}
		base, err = c.ValidateBaseURL(custom)
		if err != nil {
			return "", "", &Error{502, "unsafe_endpoint", "Devin custom endpoint was rejected"}
		}
		u, e := url.Parse(base)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return "", "", &Error{502, "unsafe_endpoint", "Devin custom endpoint was rejected"}
		}
		base = strings.TrimRight(base, "/")
	}
	return jwt, base, nil
}

// Models returns usable native variants keyed by their actual wire UID. Effort
// variants remain separate entries; unsupported effort settings are never guessed.
func (c *Client) Models(ctx context.Context, token string) ([]Model, error) {
	if err := checkToken(token); err != nil {
		return nil, err
	}
	m, err := c.unary(ctx, DefaultBaseURL, modelsPath, pb(nil, 1, metadata(token, "", true)))
	if err != nil {
		return nil, err
	}
	if err = m.check([]int32{1}, nil); err != nil {
		return nil, err
	}
	models := make([]Model, 0, len(m[1]))
	seen := map[string]bool{}
	for _, f := range m[1] {
		cfg, e := parseProto(f.b)
		if e != nil {
			return nil, e
		}
		if e = cfg.check([]int32{1, 22, 23}, []int32{4, 5}); e != nil {
			return nil, e
		}
		info, e := parseProto(cfg.data(23))
		if e != nil {
			return nil, e
		}
		if e = info.check([]int32{6}, []int32{2, 13, 22, 25}); e != nil {
			return nil, e
		}
		uid := strings.TrimSpace(cfg.text(22))
		if cfg.num(4) != 0 || info.num(22) == 4 || info.num(22) == 6 || uid == "" || seen[uid] {
			continue
		}
		// Fusion is native lead/sidekick orchestration, not an AssignModel
		// router. Do not advertise pairings this adapter cannot execute, or
		// silently replace a requested pairing with its frontier lead alone.
		if isFusionModelID(uid) {
			continue
		}
		feat, e := parseProto(info.data(6))
		if e != nil {
			return nil, e
		}
		if e = feat.check(nil, []int32{11, 12, 21}); e != nil {
			return nil, e
		}
		model := Model{ID: uid, UID: uid, DisplayName: strings.TrimSpace(cfg.text(1)), MaxTokens: 64000,
			Router: info.num(22) == 3 || info.num(25) != 0, SupportsImages: cfg.num(5) != 0, SupportsParallelTools: feat.num(21) != 0}
		if len(info[6]) > 0 {
			model.SupportsImages = feat.num(11) != 0
			v := feat.num(12) != 0
			model.supportsTools = &v
		}
		if uid == "swe-1-6" || uid == "swe-1-6-fast" {
			model.SupportsImages = false
		}
		if max := info.num(13); max > 0 && max <= 1<<31-1 {
			model.MaxTokens = int(max)
		}
		if model.DisplayName == "" {
			model.DisplayName = uid
		}
		models = append(models, model)
		seen[uid] = true
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

// ownedBody makes cancellation close even a transport body blocked in Read.
// No producer goroutine is used: the CC stream is translated on demand.
type ownedBody struct {
	io.ReadCloser
	ctx    context.Context
	cancel context.CancelFunc
	stop   func() bool
	once   sync.Once
}

func (b *ownedBody) closeUpstream() error {
	var err error
	b.once.Do(func() { err = b.ReadCloser.Close() })
	return err
}
func (b *ownedBody) Close() error {
	if b.stop != nil {
		b.stop()
	}
	b.cancel()
	return b.closeUpstream()
}
func (b *ownedBody) Read(p []byte) (int, error) {
	if err := b.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := b.ReadCloser.Read(p)
	if e := b.ctx.Err(); e != nil {
		return n, e
	}
	return n, err
}
