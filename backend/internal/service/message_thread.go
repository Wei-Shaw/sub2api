package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// Native Anthropic message threads (beta message-threads-2026-08-12).
//
// The upstream keeps the conversation; a continue request carries only the new
// messages and the id of the previous response. The gateway therefore has to
// send every continue to the exact upstream identity that created the thread,
// with the same final model and anthropic-beta set, and must never add
// system/tools to it. Nothing here stores conversation content or credentials:
// a binding is routing metadata keyed by tenant scope and response id.

const (
	messageThreadsBeta = "message-threads-2026-08-12"

	// MessageThreadTTL mirrors the upstream: a thread expires five minutes after
	// its last request, and every stored response restarts that window.
	MessageThreadTTL = 5 * time.Minute

	// MessageThreadAccountFlag is the account extra key an operator sets to
	// declare that a custom-origin or pool-mode upstream really implements
	// message threads. Without it such upstreams are never used for a thread:
	// a Messages-compatible proxy may drop the field and still answer 200.
	MessageThreadAccountFlag = "native_message_threads"

	messageThreadMaxTokens    = 100_000_000
	messageThreadMaxBetaBytes = 8192
	messageThreadBindTimeout  = 3 * time.Second
)

// MessageThreadError is a thread-specific client response. Written reports that
// an upstream response was already relayed verbatim.
type MessageThreadError struct {
	Status      int
	Type        string
	Code        string
	Message     string
	Unsupported []string
	Written     bool
	incapable   bool
	passedOver  bool
}

func (e *MessageThreadError) Error() string { return e.Message }

// Body is the Anthropic error envelope for a locally produced thread error.
func (e *MessageThreadError) Body() gin.H {
	detail := gin.H{"type": e.Type, "message": e.Message}
	if e.Code != "" {
		details := gin.H{"error_code": e.Code}
		if len(e.Unsupported) > 0 {
			details["unsupported"] = e.Unsupported
		}
		detail["details"] = details
	}
	return gin.H{"type": "error", "error": detail}
}

func threadNotFoundError() *MessageThreadError {
	return &MessageThreadError{Status: http.StatusNotFound, Type: "not_found_error", Code: "thread_not_found",
		Message: "thread: cannot continue this thread. Send the full conversation in a new create request."}
}

func threadStorageError() *MessageThreadError {
	return &MessageThreadError{Status: http.StatusServiceUnavailable, Type: "api_error",
		Message: "thread: message thread state is temporarily unavailable. Retry, or send the full conversation without thread."}
}

// NewMessageThreadNotFoundError is the answer when the gateway cannot continue
// a thread. It matches the upstream's own error, so clients replay.
func NewMessageThreadNotFoundError() *MessageThreadError { return threadNotFoundError() }

// NewMessageThreadUnavailableError is the answer when thread state cannot be
// read. It is deliberately not a miss.
func NewMessageThreadUnavailableError() *MessageThreadError { return threadStorageError() }

func threadInvalidError(message string) *MessageThreadError {
	return &MessageThreadError{Status: http.StatusBadRequest, Type: "invalid_request_error", Message: "thread: " + message}
}

func threadUnsupportedError(message string, unsupported ...string) *MessageThreadError {
	return &MessageThreadError{Status: http.StatusBadRequest, Type: "invalid_request_error", Code: "thread_unsupported_request",
		Message: "thread: " + message, Unsupported: unsupported}
}

func threadMismatchError(fields ...string) *MessageThreadError {
	return &MessageThreadError{Status: http.StatusBadRequest, Type: "invalid_request_error", Code: "thread_fingerprint_mismatch",
		Message: "thread: these fields differ from the request that created the thread: " + strings.Join(fields, ", ") +
			". Keep them the same or start a new thread."}
}

func threadInternalError(message string) *MessageThreadError {
	return &MessageThreadError{Status: http.StatusInternalServerError, Type: "api_error", Message: "thread: " + message}
}

// IsMessageThreadAccountIncapable reports that the selected account cannot
// serve threads, so a create request may try another account.
func IsMessageThreadAccountIncapable(err error) bool {
	var te *MessageThreadError
	return errors.As(err, &te) && te.incapable
}

// ErrMessageThreadNoCapableAccount is returned by account selection when the
// group has candidates but none of them can serve a thread.
var ErrMessageThreadNoCapableAccount = func() *MessageThreadError {
	e := threadUnsupportedError("no account in this group can serve message threads. Send the full conversation without thread.")
	e.incapable = true
	return e
}()

// MessageThreadStore persists bindings. Implementations must scope keys by the
// given tenant scope, expire entries after MessageThreadTTL and return
// (nil, nil) for a miss so callers can tell it from a storage failure.
type MessageThreadStore interface {
	ReadMessageThread(ctx context.Context, scope, messageID string) (*MessageThreadBinding, error)
	BindMessageThread(ctx context.Context, scope, messageID string, binding MessageThreadBinding) error
}

// MessageThreadBinding is the routing metadata kept for one response id.
type MessageThreadBinding struct {
	AccountID int64 `json:"a"`
	// Identity is a digest of the upstream identity the response came from. It
	// includes a digest of the credential that was actually sent, so the thread
	// is only continued with that same credential: a refreshed, rotated or
	// re-authorized token ends it. The account row alone cannot prove that a
	// new token belongs to the same upstream principal.
	Identity string `json:"i"`
	// Model and Beta are what was actually sent upstream on the create request,
	// and Wire is a digest of the other fixed fields as sent.
	Model string `json:"m"`
	Beta  string `json:"b"`
	Wire  string `json:"w"`
	// ClientModel, ClientBeta and ClientFixed are what the client sent, before
	// any gateway rewrite, to tell a client change of a fixed field from
	// gateway-side drift. ClientModel is the public model name.
	ClientModel string            `json:"cm"`
	ClientBeta  string            `json:"cb"`
	ClientFixed map[string]string `json:"cf"`
	// ContextTokens is the whole conversation after this response, as metered
	// by the upstream. It sizes the balance reservation of the next turn.
	ContextTokens int `json:"t"`
	// Session keeps the account's session limit on one key for the thread.
	Session string `json:"s"`
	// Mimic freezes which request path the create request took. ClaudeCode
	// records that the create request's system prompt passed strict Claude
	// Code validation; it is inherited unchanged and never set by a later turn.
	Mimic      bool `json:"mi"`
	ClaudeCode bool `json:"cc"`
	// HadTools, Breakpoints and ShortTTL describe what the upstream already
	// holds and a continue request cannot resend: whether tools were declared,
	// how many cache breakpoints system and tools carry, and whether any stored
	// breakpoint has a TTL shorter than one hour.
	HadTools    bool `json:"ht"`
	Breakpoints int  `json:"bp"`
	ShortTTL    bool `json:"st"`
}

// Valid rejects entries that could not have been written by this version: a
// missing value, or a digest that does not have the shape this version always
// writes. Every request of a thread carries the thread beta in the client's
// own header, so the digest of the client's beta set is never empty. Valid
// cannot tell a well-formed value that was replaced by another.
func (b *MessageThreadBinding) Valid() bool {
	if b == nil || b.AccountID <= 0 || b.Model == "" || b.ClientModel == "" || b.ClientFixed == nil ||
		!isThreadHexDigest(b.Identity, sha256.Size*2) || !isThreadHexDigest(b.Wire, threadWireDigestLen) ||
		!isThreadHexDigest(b.ClientBeta, sha256.Size*2) ||
		len(b.Beta) > messageThreadMaxBetaBytes || !containsBetaToken(b.Beta, messageThreadsBeta) ||
		b.ContextTokens <= 0 || b.ContextTokens > messageThreadMaxTokens || b.Breakpoints < 0 ||
		b.Breakpoints > maxCacheControlBlocks {
		return false
	}
	for field, digest := range b.ClientFixed {
		if !slices.Contains(messageThreadFixedFields, field) || !isThreadHexDigest(digest, threadFieldDigestLen) {
			return false
		}
	}
	return true
}

const (
	threadFieldDigestLen = 16
	threadWireDigestLen  = 32
)

func isThreadHexDigest(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func (b *MessageThreadBinding) sameRoute(o *MessageThreadBinding) bool {
	return b != nil && o != nil && b.AccountID == o.AccountID && b.Identity == o.Identity && b.Model == o.Model &&
		b.Wire == o.Wire && canonicalThreadBeta(b.Beta) == canonicalThreadBeta(o.Beta)
}

// MessageThread is the state of one thread request. It is owned by the request
// goroutine: the handler, Forward and the response writer run on it in turn.
type MessageThread struct {
	store       MessageThreadStore
	scope       string
	groupID     *int64
	parentID    string
	parent      *MessageThreadBinding
	clientModel string
	clientBeta  string
	clientFixed map[string]string
	schedModel  string
	session     string
	claudeCode  bool
	// admissionFault: a store an admission decision depended on failed.
	// limitDenied: an account was passed over for lack of room.
	admissionFault bool
	limitDenied    bool
	// stickyAccountID is the account this request's session is bound to.
	stickyAccountID int64
	attempt         messageThreadAttempt
}

// messageThreadAttempt is reset for every account attempt.
type messageThreadAttempt struct {
	// forwardModel is the model this attempt was handed, before the account's
	// own mapping; beta policy and the final route check are evaluated on it.
	forwardModel string
	// policy is the beta policy read once for this attempt. Every decision of
	// the attempt, on the client's header and on the outbound one, uses this
	// one snapshot; policyLoaded tells an unconfigured policy from none read.
	policy       *BetaPolicySettings
	policyLoaded bool
	sealed       bool
	rewritten    bool
	next         MessageThreadBinding
	// finished is set once the response of this attempt has been judged.
	finished bool
}

type messageThreadContextKey struct{}

// Context attaches the thread to a request context.
func (t *MessageThread) Context(ctx context.Context) context.Context {
	if t == nil {
		return ctx
	}
	return context.WithValue(ctx, messageThreadContextKey{}, t)
}

func threadFrom(ctx context.Context) *MessageThread {
	if ctx == nil {
		return nil
	}
	t, _ := ctx.Value(messageThreadContextKey{}).(*MessageThread)
	return t
}

// IsContinue reports a continue request with a known binding.
func (t *MessageThread) IsContinue() bool { return t != nil && t.parent != nil }

// PreviousTokens is the upstream-held context a continue request will be
// billed for in addition to its own body.
func (t *MessageThread) PreviousTokens() int {
	if !t.IsContinue() {
		return 0
	}
	return t.parent.ContextTokens
}

// BoundAccountID is the only account a continue request may use.
func (t *MessageThread) BoundAccountID() int64 {
	if !t.IsContinue() {
		return 0
	}
	return t.parent.AccountID
}

// InheritsClaudeCode reports that the thread was created by a request whose
// system prompt passed strict Claude Code validation.
func (t *MessageThread) InheritsClaudeCode() bool { return t.IsContinue() && t.parent.ClaudeCode }

// SessionHash returns the session key of the thread: the one computed for a
// create request, or the one stored with the thread for a continue request,
// whose short body would otherwise hash to a new session on every turn.
func (t *MessageThread) SessionHash(computed string) string {
	if t.IsContinue() {
		return t.parent.Session
	}
	return computed
}

// SetRequestContext records request facts the handler has established: the
// session key, the model the request is scheduled by, and whether its system
// prompt passed strict Claude Code validation (not merely a probe exception).
func (t *MessageThread) SetRequestContext(session, schedulingModel string, strictClaudeCode bool) {
	if t == nil {
		return
	}
	t.session, t.schedModel, t.claudeCode = session, schedulingModel, strictClaudeCode
}

func messageThreadScope(userID, keyID int64, groupID *int64) string {
	return fmt.Sprintf("%d:%d:%d", userID, keyID, derefGroupID(groupID))
}

func validMessageThreadID(id gjson.Result) bool {
	return id.Type == gjson.String && validThreadResponseID(id.Str)
}

func validThreadResponseID(id string) bool {
	return id != "" && len(id) <= 256 && strings.TrimSpace(id) == id
}

// anthropicBetaHeader accepts repeated header lines, including raw wire casing.
func anthropicBetaHeader(h http.Header) string {
	var values []string
	for key, lines := range h {
		if strings.EqualFold(key, "anthropic-beta") {
			values = append(values, lines...)
		}
	}
	return canonicalThreadBeta(strings.Join(values, ","))
}

func canonicalThreadBeta(beta string) string {
	parts := parseAnthropicBetaHeader(beta)
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	slices.Sort(parts)
	parts = slices.Compact(parts)
	return strings.Join(slices.DeleteFunc(parts, func(p string) bool { return p == "" }), ",")
}

func digestThreadValue(v string) string {
	if v == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

// A second "thread" key would let two JSON parsers route on different values.
func hasAmbiguousThreadField(body []byte, thread gjson.Result) bool {
	seen := 0
	gjson.ParseBytes(body).ForEach(func(key, _ gjson.Result) bool {
		if key.Str == "thread" {
			seen++
		}
		return seen < 2
	})
	if seen != 1 {
		return true
	}
	keys := map[string]bool{}
	duplicate := false
	thread.ForEach(func(key, _ gjson.Result) bool {
		duplicate = keys[key.Str]
		keys[key.Str] = true
		return !duplicate
	})
	return duplicate
}

// messageThreadFixedFields are the request fields the upstream stores with a
// thread and requires unchanged on every request, besides model and the beta
// set. An MCP server's authorization token and everything in container but its
// skills may change. fallbacks is refused on a thread, see PrepareMessageThread.
var messageThreadFixedFields = []string{
	"tool_choice", "thinking", "mcp_servers", "context_management", "output_config", "container.skills", "cache_control",
}

// threadFixedFieldDigests returns a short digest per fixed field present in
// body. Values are compared as JSON values, not as bytes: key order,
// whitespace and the spelling of a number do not matter, while two different
// numbers always differ, however large. An explicit null is a value of its
// own, distinct from an absent field. Only digests are kept, never the values.
func threadFixedFieldDigests(body []byte) map[string]string {
	out := map[string]string{}
	for _, name := range messageThreadFixedFields {
		value := gjson.GetBytes(body, name)
		if !value.Exists() {
			continue
		}
		out[name] = digestThreadValue(canonicalThreadJSON(name, value.Raw))[:threadFieldDigestLen]
	}
	return out
}

// canonicalThreadJSON renders a JSON value in one canonical form. Numbers are
// decoded without going through float64, so integers beyond 2^53 keep every
// digit. A value that cannot be decoded is compared by its bytes.
func canonicalThreadJSON(field, raw string) string {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil || decoder.More() {
		return raw
	}
	if field == "mcp_servers" {
		// An MCP server's authorization token may change between requests.
		if servers, ok := decoded.([]any); ok {
			for _, server := range servers {
				if fields, ok := server.(map[string]any); ok {
					delete(fields, "authorization_token")
				}
			}
		}
	}
	var out bytes.Buffer
	writeCanonicalThreadJSON(&out, decoded)
	return out.String()
}

func writeCanonicalThreadJSON(out *bytes.Buffer, value any) {
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		_ = out.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				_ = out.WriteByte(',')
			}
			encoded, _ := json.Marshal(key)
			_, _ = out.Write(encoded)
			_ = out.WriteByte(':')
			writeCanonicalThreadJSON(out, v[key])
		}
		_ = out.WriteByte('}')
	case []any:
		_ = out.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				_ = out.WriteByte(',')
			}
			writeCanonicalThreadJSON(out, item)
		}
		_ = out.WriteByte(']')
	case json.Number:
		_, _ = out.WriteString(canonicalThreadNumber(v.String()))
	default:
		encoded, _ := json.Marshal(v)
		_, _ = out.Write(encoded)
	}
}

// canonicalThreadNumber gives equal numbers one spelling (1024, 1024.0 and
// 1.024e3 are the same) without rounding. A literal too long or with too large
// an exponent to expand safely is kept as written.
func canonicalThreadNumber(literal string) string {
	if len(literal) > 64 {
		return literal
	}
	if i := strings.IndexAny(literal, "eE"); i >= 0 {
		exponent := strings.TrimLeft(literal[i+1:], "+-")
		if len(exponent) > 3 {
			return literal
		}
	}
	rat, ok := new(big.Rat).SetString(literal)
	if !ok {
		return literal
	}
	return rat.RatString()
}

func changedThreadFixedFields(stored, current map[string]string) []string {
	var changed []string
	for _, name := range messageThreadFixedFields {
		if stored[name] != current[name] {
			changed = append(changed, name)
		}
	}
	return changed
}

func threadWireDigest(body []byte) string {
	digests := threadFixedFieldDigests(body)
	var b strings.Builder
	for _, name := range messageThreadFixedFields {
		_, _ = b.WriteString(name + "=" + digests[name] + ";")
	}
	return digestThreadValue(b.String())[:threadWireDigestLen]
}

// PrepareMessageThread validates a thread request and resolves a continue
// request to its binding. It runs after authentication and before any queue,
// scheduler or upstream call: an unknown id is answered here.
//
// body is the client's request before any gateway rewrite and rawModel the
// model name as the client sent it. native is false when the request is about
// to be converted to another protocol, which would drop the thread and answer
// without the conversation.
func (s *GatewayService) PrepareMessageThread(ctx context.Context, body []byte, header http.Header, rawModel string, userID, keyID int64, groupID *int64, native bool) (*MessageThread, error) {
	field := gjson.GetBytes(body, "thread")
	if !field.Exists() {
		return nil, nil
	}
	if !field.IsObject() || hasAmbiguousThreadField(body, field) {
		return nil, threadInvalidError("must be a single object.")
	}
	kind, id := field.Get("type"), field.Get("previous_message_id")
	if kind.Type != gjson.String {
		return nil, threadInvalidError("type is required.")
	}
	switch kind.Str {
	case "create":
		if id.Exists() {
			return nil, threadInvalidError("previous_message_id is not allowed on a create request.")
		}
	case "continue":
		if !validMessageThreadID(id) {
			return nil, threadInvalidError("previous_message_id is required on a continue request.")
		}
		for _, name := range []string{"system", "tools"} {
			if gjson.GetBytes(body, name).Exists() {
				return nil, threadInvalidError(name + " is not allowed on a continue request. The thread keeps the values of its create request.")
			}
		}
	default:
		return nil, threadInvalidError("type must be create or continue.")
	}
	if !native {
		return nil, threadUnsupportedError("this route does not reach an Anthropic Messages endpoint that keeps message threads: the request is converted to another protocol or sent to a compatible third-party endpoint. Send the full conversation without thread.")
	}
	// The beta is the client's explicit opt-in on every request of a thread. An
	// account override that happens to carry it does not opt a client in.
	if !containsBetaToken(anthropicBetaHeader(header), messageThreadsBeta) {
		return nil, threadInvalidError("the " + messageThreadsBeta + " beta is required. Send it in the anthropic-beta header on every request of the thread.")
	}
	// A response written by a fallback model is billed per attempt and is never
	// stored by the upstream. This gateway settles one usage per request, so the
	// combination is refused before anything is sent.
	if gjson.GetBytes(body, "fallbacks").Exists() {
		return nil, threadUnsupportedError("server-side fallbacks are not supported together with a message thread on this gateway. Remove fallbacks or send the full conversation without thread.", "fallbacks")
	}
	if s == nil || userID <= 0 || keyID <= 0 {
		return nil, threadStorageError()
	}
	store, ok := s.cache.(MessageThreadStore)
	if !ok || store == nil {
		return nil, threadStorageError()
	}
	t := &MessageThread{
		store:   store,
		scope:   messageThreadScope(userID, keyID, groupID),
		groupID: groupID,
		// The public name: a composite route keeps what the client asked for.
		clientModel: compositeClientModel(ctx, rawModel),
		clientBeta:  digestThreadValue(canonicalThreadBeta(anthropicBetaHeader(header))),
		clientFixed: threadFixedFieldDigests(body),
	}
	if kind.Str == "create" {
		return t, nil
	}
	binding, err := store.ReadMessageThread(ctx, t.scope, id.Str)
	if err != nil {
		slog.Warn("message_thread.read_failed", "error", err)
		return nil, threadStorageError()
	}
	if binding == nil {
		return nil, threadNotFoundError()
	}
	// A binding that exists but cannot be used is a fault, not an expiry.
	if !binding.Valid() {
		slog.Error("message_thread.invalid_binding", "account_id", binding.AccountID)
		return nil, threadStorageError()
	}
	// The client changed a fixed field. The upstream would answer with the same
	// error if it saw the change, but a gateway rewrite can hide it; saying so
	// here keeps the client's choice from being silently overridden.
	var changed []string
	if binding.ClientModel != t.clientModel {
		changed = append(changed, "model")
	}
	if binding.ClientBeta != t.clientBeta {
		changed = append(changed, "anthropic-beta")
	}
	changed = append(changed, changedThreadFixedFields(binding.ClientFixed, t.clientFixed)...)
	if len(changed) > 0 {
		return nil, threadMismatchError(changed...)
	}
	t.parentID, t.parent = id.Str, binding
	return t, nil
}

func isOfficialAnthropicOrigin(baseURL string) bool {
	return strings.EqualFold(strings.TrimRight(strings.TrimSpace(baseURL), "/"), "https://api.anthropic.com")
}

// messageThreadAccountKind is the coarse check that the slim scheduler
// projection can answer. messageThreadAccountSupported needs the full account.
func messageThreadAccountKind(a *Account) bool {
	if a == nil || a.Platform != PlatformAnthropic {
		return false
	}
	return a.Type == AccountTypeAPIKey || a.Type == AccountTypeOAuth || a.Type == AccountTypeSetupToken
}

func messageThreadAccountSupported(a *Account) bool {
	if !messageThreadAccountKind(a) {
		return false
	}
	declared, _ := a.Extra[MessageThreadAccountFlag].(bool)
	if a.Type == AccountTypeAPIKey {
		if a.GetCredential("api_key") == "" {
			return false
		}
		return declared || (isOfficialAnthropicOrigin(getAnthropicGatewayBaseURL(a)) && !a.IsPoolMode())
	}
	return declared || !a.IsCustomBaseURLEnabled()
}

// messageThreadRowCredential is the credential an account row would send.
func messageThreadRowCredential(a *Account) string {
	if a == nil {
		return ""
	}
	if a.Type == AccountTypeAPIKey {
		return strings.TrimSpace(a.GetCredential("api_key"))
	}
	return strings.TrimSpace(a.GetCredential("access_token"))
}

// messageThreadIdentity digests what makes the upstream treat two requests as
// the same caller: the account row, its origin and the credential that is
// actually sent. Account metadata such as an account or organization id is
// deliberately not relied on: it can be absent or stale after the row is
// authorized again, and then the row would look unchanged while its token
// belongs to someone else. Only a digest of the credential is kept.
func messageThreadIdentity(a *Account, credential string) string {
	credential = strings.TrimSpace(credential)
	if a == nil || credential == "" {
		return ""
	}
	parts := []any{"message-thread-identity-v2", a.ID, a.Platform, a.Type, digestThreadValue(credential)}
	if a.Type == AccountTypeAPIKey {
		parts = append(parts, strings.TrimRight(getAnthropicGatewayBaseURL(a), "/"),
			a.GetHeaderOverrides(), a.IsAnthropicAPIKeyPassthroughEnabled())
	} else {
		parts = append(parts, a.IsCustomBaseURLEnabled(), strings.TrimRight(a.GetCustomBaseURL(), "/"))
	}
	raw, err := json.Marshal(parts)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// messageThreadWireModel is the model an account sends upstream for the model
// an attempt was handed. It mirrors the mapping Forward applies.
func messageThreadWireModel(a *Account, model string) string {
	if a == nil {
		return model
	}
	if a.Type == AccountTypeAPIKey {
		return a.GetMappedModel(model)
	}
	if a.IsAnthropicOAuthOrSetupToken() {
		return claude.NormalizeModelID(model)
	}
	return model
}

// filterMessageThreadCandidates narrows the scheduler's candidates for a
// thread request and leaves every other eligibility rule to the scheduler: a
// create request may use any account that can serve threads, a continue
// request only the bound one.
func (s *GatewayService) filterMessageThreadCandidates(ctx context.Context, accounts []Account) ([]Account, error) {
	t := threadFrom(ctx)
	if t == nil {
		return accounts, nil
	}
	kept := make([]Account, 0, len(accounts))
	for i := range accounts {
		if s.isMessageThreadCandidate(t, &accounts[i]) {
			kept = append(kept, accounts[i])
		}
	}
	if len(kept) == 0 && len(accounts) > 0 && !t.IsContinue() {
		return nil, ErrMessageThreadNoCapableAccount
	}
	return kept, nil
}

func (s *GatewayService) isMessageThreadCandidate(t *MessageThread, a *Account) bool {
	if t == nil {
		return true
	}
	if !messageThreadAccountKind(a) {
		return false
	}
	return !t.IsContinue() || a.ID == t.parent.AccountID
}

// checkMessageThreadCapability reports whether the account can serve threads
// at all and, for a continue request, whether it is the bound account.
func (s *GatewayService) checkMessageThreadCapability(ctx context.Context, t *MessageThread, account *Account, groupID *int64) error {
	if t == nil {
		return nil
	}
	if account == nil || !messageThreadAccountSupported(account) {
		if t.IsContinue() {
			return threadNotFoundError()
		}
		e := threadUnsupportedError("the selected upstream cannot serve message threads. Send the full conversation without thread.")
		e.incapable = true
		return e
	}
	if s.isBedrockCCCompatEnabled(ctx, account, groupID) {
		return threadUnsupportedError("this channel rewrites requests for a compatibility upstream, which cannot carry a message thread. Send the full conversation without thread.")
	}
	if t.IsContinue() && account.ID != t.parent.AccountID {
		return threadNotFoundError()
	}
	return nil
}

// CheckMessageThreadAccount is the check on the hydrated account the scheduler
// selected, before any queue wait and again after it: capability, for a
// continue request that the credential this account would send is the one the
// thread was created with, and the account's configured rate and cost limits,
// read for this account (see admitMessageThreadLimits). Every thread request
// passes here before it is forwarded. The final request is checked again when
// it is sealed.
func (s *GatewayService) CheckMessageThreadAccount(ctx context.Context, t *MessageThread, account *Account, groupID *int64) error {
	if err := s.checkMessageThreadCapability(ctx, t, account, groupID); err != nil {
		return err
	}
	if t.IsContinue() {
		credential, _, err := s.GetAccessToken(ctx, account)
		// A failure to get the credential is not a statement about the
		// thread; Forward reports it as usual.
		if err == nil && messageThreadIdentity(account, credential) != t.parent.Identity {
			return threadNotFoundError()
		}
	}
	return s.admitMessageThreadLimits(ctx, t, account)
}

// isThreadBoundAccount reports that the request continues a message thread
// that lives on this account. Such a request is not a new session for the
// account: like a sticky session it may use the zone a rate or cost limit
// reserves for sessions already there. The hard limit still applies.
func isThreadBoundAccount(ctx context.Context, account *Account) bool {
	t := threadFrom(ctx)
	return t.IsContinue() && account != nil && account.ID == t.parent.AccountID
}

// beginAttempt clears what a previous account attempt recorded.
func (t *MessageThread) beginAttempt(forwardModel string) {
	if t != nil {
		t.attempt = messageThreadAttempt{forwardModel: forwardModel}
	}
}

// markRewritten records that the gateway changed fixed fields of the request
// (a signature or thinking-budget rectification). The upstream would store a
// thread the client's own fields no longer match, so the response is delivered
// without a binding and the client replays.
func (t *MessageThread) markRewritten() {
	if t != nil {
		t.attempt.rewritten = true
	}
}

// resolveBeta decides the final anthropic-beta value of a thread request from
// the value the normal builder computed, which may come from the client or
// from an account header override. It is the one place that decides it: the
// result is used to sanitize the body and is the header that is sent.
//
// A create request drops what the current policy filters, whatever the source,
// and keeps the thread beta the client asked for even on a path that replaces
// the client's beta list. A continue request reuses the set sent on the create
// request, because the builder derives part of it from tools and system, which
// a continue request no longer has. A filtered token is never restored: the
// thread is then reported as not continuable.
func (t *MessageThread) resolveBeta(computed string, drop map[string]struct{}) (string, error) {
	if !t.IsContinue() {
		if _, dropped := drop[messageThreadsBeta]; dropped {
			return "", threadUnsupportedError("the message threads beta is not enabled for this route. Send the full conversation without thread.", "anthropic-beta")
		}
		beta := stripBetaTokensWithSet(computed, drop)
		if !containsBetaToken(beta, messageThreadsBeta) {
			beta = mergeAnthropicBeta(parseAnthropicBetaHeader(beta), messageThreadsBeta)
		}
		return beta, nil
	}
	for _, token := range parseAnthropicBetaHeader(t.parent.Beta) {
		if _, dropped := drop[token]; dropped {
			return "", threadNotFoundError()
		}
	}
	return t.parent.Beta, nil
}

// messageThreadMimicGuardedFields are native features whose request field
// needs a beta that the rewrite for subscription accounts does not carry: that
// path replaces the client's beta list with a fixed one. Sending the field
// without its beta is not a request this gateway can vouch for, so a thread
// request carrying one is refused on that path. Only the top-level field
// counts; an ordinary tool whose name mentions MCP is not an MCP server.
var messageThreadMimicGuardedFields = []struct{ field, betaPrefix string }{
	{"mcp_servers", "mcp-client-"},
	{"container.skills", "skills-"},
}

func threadMimicUnsupportedField(body []byte, beta string) string {
	tokens := parseAnthropicBetaHeader(beta)
	for _, guarded := range messageThreadMimicGuardedFields {
		value := gjson.GetBytes(body, guarded.field)
		if !value.IsArray() || len(value.Array()) == 0 {
			continue
		}
		if !slices.ContainsFunc(tokens, func(token string) bool { return strings.HasPrefix(strings.TrimSpace(token), guarded.betaPrefix) }) {
			return guarded.field
		}
	}
	return ""
}

// sealMessageThread runs on the final upstream request, after every rewrite,
// policy filter and header override. credential is what the request
// authenticates with. It verifies the request is still a valid thread request,
// re-checks a continue request against the authoritative account row and
// binding immediately before the transport, and records what a successful
// response will be bound to.
func (s *GatewayService) sealMessageThread(ctx context.Context, account *Account, req *http.Request, body []byte, credential string, mimic bool) error {
	t := threadFrom(ctx)
	if t == nil {
		return nil
	}
	t.attempt.sealed = false
	if !messageThreadAccountSupported(account) {
		if t.IsContinue() {
			return threadNotFoundError()
		}
		return threadUnsupportedError("the selected upstream cannot serve message threads. Send the full conversation without thread.")
	}
	identity := messageThreadIdentity(account, credential)
	model := gjson.GetBytes(body, "model").String()
	beta := anthropicBetaHeader(req.Header)
	if identity == "" || model == "" {
		return threadInternalError("the upstream request is incomplete.")
	}
	if len(beta) > messageThreadMaxBetaBytes {
		return threadUnsupportedError("the anthropic-beta list is too long for a message thread.", "anthropic-beta")
	}
	if !containsBetaToken(beta, messageThreadsBeta) {
		if t.IsContinue() {
			return threadNotFoundError()
		}
		return threadInvalidError("the " + messageThreadsBeta + " beta is required and is not part of the upstream request. Send it in the anthropic-beta header on every request of the thread.")
	}
	// The policy is evaluated on what is about to be sent, not only on what the
	// client asked for: a token the gateway or an account override added, or a
	// frozen set from before a rule was tightened, must not get past a rule.
	// It is the snapshot this attempt already read; without one nothing is sent.
	if !t.attempt.policyLoaded {
		return threadStorageError()
	}
	outbound := parseAnthropicBetaHeader(beta)
	if blocked := s.checkBetaPolicyBlockForTokens(ctx, outbound, account, t.attempt.forwardModel); blocked != nil {
		return blocked
	}
	filtered := s.evaluateBetaPolicy(ctx, beta, account, t.attempt.forwardModel).filterSet
	for _, token := range outbound {
		if _, dropped := filtered[strings.TrimSpace(token)]; dropped {
			if t.IsContinue() {
				return threadNotFoundError()
			}
			return threadInternalError("a filtered beta is part of the upstream request.")
		}
	}
	if mimic {
		if field := threadMimicUnsupportedField(body, beta); field != "" {
			return threadUnsupportedError(field+" is not supported together with a message thread for this client on this route. Send the full conversation without thread.", field)
		}
	}
	wantKind := "create"
	if t.IsContinue() {
		wantKind = "continue"
	}
	if gjson.GetBytes(body, "thread.type").Str != wantKind {
		return threadInternalError("the thread field was changed while preparing the request.")
	}
	wire := threadWireDigest(body)
	shortTTL, longTTL := threadCacheTTLs(body)
	if !t.IsContinue() {
		next := MessageThreadBinding{
			AccountID:   account.ID,
			Identity:    identity,
			Model:       model,
			Beta:        beta,
			Wire:        wire,
			ClientModel: t.clientModel,
			ClientBeta:  t.clientBeta,
			ClientFixed: t.clientFixed,
			Session:     t.session,
			Mimic:       mimic,
			ClaudeCode:  t.claudeCode,
			HadTools:    gjson.GetBytes(body, "tools.0").Exists(),
			Breakpoints: countThreadCreateBreakpoints(body),
			ShortTTL:    shortTTL,
		}
		t.attempt.next, t.attempt.sealed = next, true
		return nil
	}

	if gjson.GetBytes(body, "thread.previous_message_id").Str != t.parentID ||
		gjson.GetBytes(body, "system").Exists() || gjson.GetBytes(body, "tools").Exists() {
		return threadInternalError("a continue request must not carry system or tools.")
	}
	// The client's own fixed fields are unchanged (checked on entry), so a
	// difference here is the gateway's: the mapping, policy, override or rewrite
	// in force no longer yields what the thread was created with, or the
	// credential is not the one it was created with. The upstream would reject
	// or misattribute the request; a replay under the current settings works.
	if model != t.parent.Model || wire != t.parent.Wire || canonicalThreadBeta(beta) != canonicalThreadBeta(t.parent.Beta) ||
		account.ID != t.parent.AccountID || identity != t.parent.Identity {
		return threadNotFoundError()
	}
	// A one-hour breakpoint may not follow a shorter one, and the breakpoints
	// the thread already holds can no longer be raised. Checked here, on the
	// body as it will be sent, after every rewrite that can change a TTL. The
	// client replays, and the replay is repaired as a whole.
	if t.parent.ShortTTL && longTTL {
		return threadNotFoundError()
	}
	if err := s.verifyMessageThreadRoute(ctx, t, credential, identity); err != nil {
		return err
	}
	next := *t.parent
	next.ContextTokens = 0
	next.ShortTTL = t.parent.ShortTTL || shortTTL
	t.attempt.next, t.attempt.sealed = next, true
	return nil
}

// verifyMessageThreadRoute re-reads the binding and the account row. The
// scheduler works on a snapshot; this closes the window in which the account
// was deleted, moved, re-authorized, rate limited, or lost the model after it
// was selected.
func (s *GatewayService) verifyMessageThreadRoute(ctx context.Context, t *MessageThread, credential, identity string) error {
	current, err := t.store.ReadMessageThread(ctx, t.scope, t.parentID)
	if err != nil {
		slog.Warn("message_thread.read_failed", "error", err)
		return threadStorageError()
	}
	if current == nil {
		return threadNotFoundError()
	}
	if !current.Valid() {
		slog.Error("message_thread.invalid_binding", "account_id", current.AccountID)
		return threadStorageError()
	}
	if !current.sameRoute(t.parent) {
		return threadNotFoundError()
	}
	if s.accountRepo == nil {
		return threadStorageError()
	}
	latest, err := s.accountRepo.GetByID(ctx, t.parent.AccountID)
	if err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			return threadNotFoundError()
		}
		slog.Warn("message_thread.account_reload_failed", "account_id", t.parent.AccountID, "error", err)
		return threadStorageError()
	}
	// The credential in hand must be the row's current one, and the row must
	// still be allowed to serve this model and still map it to the model the
	// thread was created with.
	if latest == nil || latest.ID != t.parent.AccountID || !messageThreadAccountSupported(latest) ||
		messageThreadRowCredential(latest) != strings.TrimSpace(credential) ||
		messageThreadIdentity(latest, credential) != identity || !latest.IsSchedulable() ||
		!s.isAccountSchedulableForModelSelection(ctx, latest, t.schedModel) ||
		!s.isModelSupportedByAccountWithContext(ctx, latest, t.schedModel) ||
		messageThreadWireModel(latest, t.attempt.forwardModel) != t.parent.Model {
		return threadNotFoundError()
	}
	_, groupID, err := s.checkClaudeCodeRestriction(ctx, t.groupID)
	if err != nil {
		if errors.Is(err, ErrClaudeCodeOnly) {
			return threadNotFoundError()
		}
		slog.Warn("message_thread.group_resolve_failed", "error", err)
		return threadStorageError()
	}
	if !s.isAccountInThreadPool(latest, groupID) {
		return threadNotFoundError()
	}
	// The snapshot used during selection may predate a newly enabled or
	// tightened limit. Use the current row and the existing soft counters.
	return s.admitMessageThreadLimits(ctx, t, latest)
}

// isAccountInThreadPool applies the pool rule the scheduler used for the
// create request: in simple run mode accounts are not scoped by group.
func (s *GatewayService) isAccountInThreadPool(account *Account, groupID *int64) bool {
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return true
	}
	return s.isAccountInGroup(account, groupID)
}

// threadCacheTTLs reports whether body carries a cache breakpoint shorter than
// one hour and whether it carries a one-hour one.
func threadCacheTTLs(body []byte) (short, long bool) {
	_, messagePaths, toolPaths, systemPaths := collectCacheControlPaths(body)
	paths := append(append(append([]string{}, toolPaths...), systemPaths...), messagePaths...)
	if gjson.GetBytes(body, "cache_control").Exists() {
		paths = append(paths, "cache_control")
	}
	for _, path := range paths {
		if gjson.GetBytes(body, path+".ttl").String() == cacheTTLTarget1h {
			long = true
		} else {
			short = true
		}
	}
	return short, long
}

func threadUsageTokens(usage *ClaudeUsage) (int, bool) {
	if usage == nil {
		return 0, false
	}
	total := 0
	for _, n := range []int{usage.InputTokens, usage.CacheReadInputTokens, usage.CacheCreationInputTokens, usage.OutputTokens} {
		if n < 0 || n > messageThreadMaxTokens {
			return 0, false
		}
		total += n
	}
	return total, total > 0 && total <= messageThreadMaxTokens
}

// bind stores the binding of a completed response. It never fails the
// response: an unbound response is still a valid answer, and the client learns
// on its next continue request that it has to replay.
func (t *MessageThread) bind(ctx context.Context, response threadResponseFacts, usage *ClaudeUsage, clientGone bool) {
	if t == nil || t.attempt.finished {
		return
	}
	t.attempt.finished = true
	a := &t.attempt
	reason := ""
	switch {
	case !a.sealed:
		reason = "not_sealed"
	case a.rewritten:
		reason = "request_rewritten"
	case response.invalid != "":
		reason = "invalid_response:" + response.invalid
	case !validThreadResponseID(response.id):
		reason = "invalid_response"
	case clientGone || ctx.Err() != nil:
		reason = "client_gone"
	case response.stopReason == "":
		reason = "no_stop_reason"
	case response.stopReason == "refusal":
		reason = "refusal"
	case response.fallback:
		reason = "fallback_model"
	case response.stopReason == "max_tokens" && isThreadToolCallBlock(response.lastBlock):
		reason = "truncated_tool_call"
	}
	binding := a.next
	if reason == "" {
		tokens, ok := threadUsageTokens(usage)
		if !ok {
			reason = "no_usage"
		}
		binding.ContextTokens = tokens
	}
	if reason == "" && !binding.Valid() {
		reason = "invalid_binding"
	}
	if reason != "" {
		slog.Info("message_thread.not_bound", "account_id", a.next.AccountID, "reason", reason)
		return
	}
	bindCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), messageThreadBindTimeout)
	defer cancel()
	if err := t.store.BindMessageThread(bindCtx, t.scope, response.id, binding); err != nil {
		slog.Warn("message_thread.bind_failed", "account_id", binding.AccountID, "error", err)
	}
}

func isThreadToolCallBlock(blockType string) bool {
	switch blockType {
	case "tool_use", "server_tool_use", "mcp_tool_use":
		return true
	}
	return false
}
