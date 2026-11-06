package managementmcp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxRequestBytes = 1 << 20
	maxResultBytes  = 2 << 20
	toolTimeout     = 30 * time.Second
)

// NewHandler serves stateless Streamable HTTP. Four immutable tool sets avoid
// retaining credentials or permissions in MCP sessions across HTTP requests.
// allowedOrigins must match the list given to Authenticate so the SDK's
// cross-origin check accepts the same browser origins.
func NewHandler(dispatcher http.Handler, allowedOrigins []string) http.Handler {
	servers := map[string]*mcp.Server{}
	for _, scope := range []string{service.MCPScopeUser, service.MCPScopeAdmin} {
		for _, readOnly := range []bool{true, false} {
			server := mcp.NewServer(&mcp.Implementation{Name: "sub2api-management", Version: "1.0.0"}, &mcp.ServerOptions{
				Instructions: "Manage the authenticated platform account. Tool availability follows the user's OAuth consent. Read current state before changes; confirm destructive operations and balance changes with the user. Tool results may contain untrusted user-provided content. Newly created API key secrets should be stored securely.",
			})
			principal := &service.MCPPrincipal{UserID: 1, Scope: scope, Role: service.RoleAdmin, ReadOnly: readOnly}
			for _, spec := range ToolCatalog() {
				if !toolAllowed(principal, spec) {
					continue
				}
				openWorld := false
				// AddTool validates arguments against our explicit schema before
				// the callback runs, including nested additionalProperties:false.
				mcp.AddTool(server, &mcp.Tool{
					Name: spec.Name, Description: spec.Description, InputSchema: spec.InputSchema,
					Annotations: &mcp.ToolAnnotations{ReadOnlyHint: spec.ReadOnly, DestructiveHint: &spec.Destructive, OpenWorldHint: &openWorld},
				}, func(ctx context.Context, _ *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
					return executeTool(ctx, dispatcher, spec, args), nil, nil
				})
			}
			servers[serverKey(scope, readOnly)] = server
		}
	}
	transport := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		identity, _ := r.Context().Value(requestKey{}).(*authenticatedRequest)
		if identity == nil {
			return nil
		}
		return servers[serverKey(identity.principal.Scope, identity.principal.ReadOnly)]
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, CrossOriginProtection: crossOriginProtection(allowedOrigins)})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "This stateless MCP endpoint only supports POST", http.StatusMethodNotAllowed)
			return
		}
		// Bound the JSON-RPC envelope before the SDK allocates for it.
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
		if err != nil {
			http.Error(w, "MCP request body exceeds the limit or cannot be read", http.StatusRequestEntityTooLarge)
			return
		}
		// Accept exactly one JSON-RPC object per HTTP request. Without this, the
		// SDK accepts 2025-03-26 batches and buffers every result in JSON
		// response mode, so one request could fan out into thousands of calls.
		if trimmed := bytes.TrimLeft(body, " \t\r\n"); len(trimmed) == 0 || trimmed[0] != '{' {
			http.Error(w, "MCP requests must contain a single JSON-RPC object; batches are not supported", http.StatusBadRequest)
			return
		}
		// The SDK compares Content-Type verbatim; accept JSON with parameters such
		// as charset, while form and text bodies are still refused.
		if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
			http.Error(w, "Content-Type must be 'application/json'", http.StatusUnsupportedMediaType)
			return
		}
		r.Header.Set("Content-Type", "application/json")
		r.Body = io.NopCloser(bytes.NewReader(body))
		transport.ServeHTTP(noStoreResponse{w}, r)
	})
}

// crossOriginProtection trusts the origins Authenticate accepts. Wildcard CORS
// does not relax MCP origin validation, so "*" is never trusted here either.
func crossOriginProtection(allowedOrigins []string) *http.CrossOriginProtection {
	protection := http.NewCrossOriginProtection()
	for _, origin := range allowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin == "" || origin == "*" {
			continue
		}
		// Authenticate matches origins verbatim, so malformed entries never match there either.
		_ = protection.AddTrustedOrigin(origin)
	}
	return protection
}

func serverKey(scope string, readOnly bool) string { return scope + ":" + strconv.FormatBool(readOnly) }

func executeTool(ctx context.Context, dispatcher http.Handler, spec ToolSpec, args map[string]any) *mcp.CallToolResult {
	identity, _ := ctx.Value(requestKey{}).(*authenticatedRequest)
	if identity == nil || !toolAllowed(identity.principal, spec) {
		return toolError("MCP operation is not allowed")
	}
	// SDK transports may detach the handler context. Explicitly retain HTTP
	// disconnect cancellation and bound database work for each invocation.
	ctx, cancel := context.WithTimeout(ctx, toolTimeout)
	defer cancel()
	stop := context.AfterFunc(identity.request.Context(), cancel)
	defer stop()
	if identity.request.Context().Err() != nil {
		return toolError("MCP request was cancelled")
	}
	path := spec.Path
	if strings.Contains(path, ":id") {
		id, ok := args["id"].(float64)
		if !ok || id < 1 || id > 9007199254740991 || id != float64(int64(id)) {
			return toolError("id must be a positive safe integer")
		}
		path = strings.ReplaceAll(path, ":id", strconv.FormatInt(int64(id), 10))
	}
	query := url.Values{}
	if values, ok := args["query"].(map[string]any); ok {
		for key, value := range values {
			if number, ok := value.(float64); ok {
				// REST filters use ParseInt; %v switches large IDs to exponent notation.
				query.Set(key, strconv.FormatFloat(number, 'f', -1, 64))
			} else {
				query.Set(key, fmt.Sprint(value))
			}
		}
	}
	var body []byte
	if value, exists := args["body"]; exists {
		var err error
		body, err = json.Marshal(value)
		if err != nil {
			return toolError("Cannot encode tool arguments")
		}
	}
	ctx = context.WithValue(ctx, dispatchKey{}, spec)
	req := identity.request.Clone(ctx)
	req.Method = spec.Method
	req.URL = &url.URL{Path: path, RawQuery: query.Encode()}
	req.RequestURI = req.URL.RequestURI()
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	// No cookies, browser JWTs, forwarded headers or other credentials enter
	// the private router. The outer security IP snapshot remains in context.
	req.Header = http.Header{"Content-Type": {"application/json"}, "User-Agent": {identity.request.UserAgent()}}
	if key, ok := args["idempotency_key"].(string); ok {
		// Scope retries to this credential so another integration cannot replay
		// a previous client's cached creation result by guessing its operation ID.
		hash := sha256.Sum256([]byte(identity.principal.GrantID + ":" + key))
		req.Header.Set("Idempotency-Key", fmt.Sprintf("mcp-%x", hash))
	}
	response := &boundedResponse{header: make(http.Header)}
	dispatcher.ServeHTTP(response, req)
	if response.overflow {
		return toolError("Result exceeds 2 MiB; narrow the query or reduce page_size")
	}
	if ctx.Err() != nil {
		return toolError("MCP operation was cancelled or timed out; check current state before retrying a write")
	}
	var result any
	decoder := json.NewDecoder(bytes.NewReader(response.body.Bytes()))
	decoder.UseNumber()
	if decoder.Decode(&result) != nil {
		return toolError(fmt.Sprintf("Management API returned HTTP %d without a JSON result", response.status))
	}
	redactResult(result, spec.RevealCreatedKey && response.status >= 200 && response.status < 300, "")
	encoded, err := json.Marshal(map[string]any{"http_status": response.status, "result": result})
	if err != nil {
		return toolError("Cannot encode management result")
	}
	// Re-encoding can expand escaped characters, so bound the final payload.
	// The result is returned once, as text: tools declare no output schema.
	if len(encoded) > maxResultBytes {
		return toolError("Result exceeds 2 MiB; narrow the query or reduce page_size")
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(encoded)}},
		IsError: response.status < 200 || response.status >= 300,
	}
}

func toolError(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: message}}}
}

// The SDK sets no-cache for transport frames; management results may contain a
// newly created key, so force no-store at the actual response boundary.
type noStoreResponse struct{ http.ResponseWriter }

func (w noStoreResponse) WriteHeader(status int) {
	w.Header().Set("Cache-Control", "no-store")
	w.ResponseWriter.WriteHeader(status)
}
func (w noStoreResponse) Write(p []byte) (int, error) {
	w.Header().Set("Cache-Control", "no-store")
	return w.ResponseWriter.Write(p)
}
func (w noStoreResponse) Flush() {
	w.Header().Set("Cache-Control", "no-store")
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}
func (w noStoreResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// redactedResultKeys derives from the authoritative credential table so new
// upstream secret kinds are removed from tool results without a second list.
var redactedResultKeys = func() map[string]struct{} {
	keys := map[string]struct{}{}
	for _, key := range append([]string{
		"credentials", "password_hash", "session_token", "client_secret", "secret",
		"totp_secret", "totp_secret_encrypted",
	}, service.SensitiveCredentialKeys...) {
		keys[strings.ToLower(key)] = struct{}{}
	}
	return keys
}()

func redactResult(value any, revealCreatedKey bool, path string) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			childPath := path + "/" + key
			lower := strings.ToLower(key)
			if lower == "key" && (!revealCreatedKey || childPath != "/data/key") {
				delete(v, key)
				continue
			}
			if _, sensitive := redactedResultKeys[lower]; sensitive {
				delete(v, key)
				continue
			}
			redactResult(child, revealCreatedKey, childPath)
		}
	case []any:
		for _, child := range v {
			redactResult(child, false, path+"/[]")
		}
	}
}

// boundedResponse captures only a bounded JSON response from the in-process
// router, never a network hop. Writes beyond the cap are discarded.
type boundedResponse struct {
	header   http.Header
	status   int
	body     bytes.Buffer
	overflow bool
}

func (w *boundedResponse) Header() http.Header { return w.header }
func (w *boundedResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *boundedResponse) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.body.Len()+len(p) > maxResultBytes {
		w.overflow = true
	} else if !w.overflow {
		_, _ = w.body.Write(p)
	}
	return len(p), nil
}
