package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const codexAccountIdentityNamespaceVersion = "v1"

const codexAccountIdentitySourceContextKey = "openai_codex_account_identity_source"

// prepareCodexAccountIdentitySource resolves credential shadows once per selected
// attempt. The handler reuses gin.Context across failover attempts, so every entry
// point overwrites the staged source before projecting outbound identity.
func (s *OpenAIGatewayService) prepareCodexAccountIdentitySource(ctx context.Context, c *gin.Context, account *Account) (*Account, error) {
	source := account
	if account != nil && account.IsShadow() {
		resolved, err := resolveCredentialAccount(ctx, s.accountRepo, account)
		if err != nil {
			return nil, err
	REDACTED
		source = resolved
REDACTED
	if c != nil {
		c.Set(codexAccountIdentitySourceContextKey, source)
REDACTED
	return source, nil
REDACTED

func codexAccountIdentitySource(c *gin.Context, fallback *Account) *Account {
	if c != nil {
		if staged, ok := c.Get(codexAccountIdentitySourceContextKey); ok {
			if source, ok := staged.(*Account); ok && source != nil {
				return source
		REDACTED
	REDACTED
REDACTED
	return fallback
REDACTED

// codexAccountIdentityNamespace returns a stable, credential-scoped namespace.
// Multiple local rows that use the same ChatGPT account intentionally share the
// same namespace. Setup tokens use an irreversible bearer fingerprint because
// they have no refresh lifecycle or imported account metadata. Refreshable OAuth
// otherwise falls back only to a persistent fingerprint seed: local row IDs are
// deployment-relative and must never become upstream identity.
func codexAccountIdentityNamespace(account *Account) string {
	if account == nil || !account.IsOpenAIOAuthLike() {
		return ""
REDACTED
	if upstreamAccountID := strings.TrimSpace(account.GetChatGPTAccountID()); upstreamAccountID != "" {
		if upstreamUserID := strings.TrimSpace(account.GetCredential("chatgpt_user_id")); upstreamUserID != "" {
			return "chatgpt:" + upstreamAccountID + ":user:" + upstreamUserID
	REDACTED
		return "chatgpt:" + upstreamAccountID
REDACTED
	if seed, ok := codexFingerprintSeed(account.Extra); ok {
		return "seed:" + seed
REDACTED
	if account.Type == AccountTypeSetupToken {
		if token := strings.TrimSpace(account.GetOpenAIAccessToken()); token != "" {
			sum := sha256.Sum256([]byte("openai-setup-token:" + token))
			return fmt.Sprintf("setup-token:%x", sum[:16])
	REDACTED
REDACTED
	return ""
REDACTED

// isolateOpenAIUpstreamSessionID preserves the existing API-key isolation while
// adding the selected OAuth credential namespace. A scheduler failover therefore
// cannot send the same session/conversation identity through two upstream accounts.
func isolateOpenAIUpstreamSessionID(apiKeyID int64, account *Account, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
REDACTED
	namespace := codexAccountIdentityNamespace(account)
	if namespace == "" {
		return isolateOpenAISessionID(apiKeyID, raw)
REDACTED
	sum := sha256.Sum256([]byte(fmt.Sprintf("u%d:a%s:%s", apiKeyID, namespace, raw)))
	return fmt.Sprintf("%x", sum[:8])
REDACTED

func scopeCodexAccountIdentityValue(account *Account, apiKeyID int64, kind, raw string) string {
	raw = strings.TrimSpace(raw)
	namespace := codexAccountIdentityNamespace(account)
	if raw == "" || namespace == "" {
		return raw
REDACTED
	return deriveStableUUIDv4(fmt.Sprintf(
		"sub2api:codex-account-identity:%s:user:%d:account:%s:kind:%s:value:%s",
		codexAccountIdentityNamespaceVersion,
		apiKeyID,
		namespace,
		kind,
		raw,
	))
REDACTED

var codexAccountIdentityFields = []struct {
	name string
	kind string
REDACTED{
	{name: "installation_id", kind: "installation"REDACTED,
	{name: "x-codex-installation-id", kind: "installation"REDACTED,
	{name: "session_id", kind: "session"REDACTED,
	{name: "session-id", kind: "session"REDACTED,
	{name: "thread_id", kind: "thread"REDACTED,
	{name: "thread-id", kind: "thread"REDACTED,
	{name: "turn_id", kind: "turn"REDACTED,
	{name: "turn-id", kind: "turn"REDACTED,
	{name: "window_id", kind: "window"REDACTED,
	{name: "x-codex-window-id", kind: "window"REDACTED,
	{name: "x-client-request-id", kind: "request"REDACTED,
REDACTED

func applyCodexAccountIdentityFields(values map[string]any, account *Account, apiKeyID int64) bool {
	if values == nil || codexAccountIdentityNamespace(account) == "" {
		return false
REDACTED
	changed := false
	for _, field := range codexAccountIdentityFields {
		raw, ok := values[field.name].(string)
		if !ok || strings.TrimSpace(raw) == "" {
			continue
	REDACTED
		next := scopeCodexAccountIdentityValue(account, apiKeyID, field.kind, raw)
		if next != raw {
			values[field.name] = next
			changed = true
	REDACTED
REDACTED
	return changed
REDACTED

func applyCodexAccountIdentityEmbeddedMetadata(values map[string]any, account *Account, apiKeyID int64) bool {
	raw, ok := values[openAIWSTurnMetadataHeader].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return false
REDACTED
	metadata := map[string]any{REDACTED
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil || metadata == nil {
		return false
REDACTED
	if !applyCodexAccountIdentityFields(metadata, account, apiKeyID) {
		return false
REDACTED
	rebuilt, err := json.Marshal(metadata)
	if err != nil {
		return false
REDACTED
	values[openAIWSTurnMetadataHeader] = string(rebuilt)
	return true
REDACTED

func applyCodexAccountIdentityClientMetadataMap(requestBody map[string]any, account *Account, apiKeyID int64) bool {
	if requestBody == nil || codexAccountIdentityNamespace(account) == "" {
		return false
REDACTED
	changed := false
	clientMetadata, _ := requestBody["client_metadata"].(map[string]any)
	originalBodySessionID := ""
	if clientMetadata != nil {
		originalBodySessionID, _ = clientMetadata["session_id"].(string)
		if applyCodexAccountIdentityFields(clientMetadata, account, apiKeyID) {
			changed = true
	REDACTED
		if applyCodexAccountIdentityEmbeddedMetadata(clientMetadata, account, apiKeyID) {
			changed = true
	REDACTED
REDACTED
	if raw, ok := requestBody["prompt_cache_key"].(string); ok && strings.TrimSpace(raw) != "" {
		kind := "prompt-cache"
		if strings.TrimSpace(originalBodySessionID) != "" && raw == originalBodySessionID {
			kind = "session"
	REDACTED
		next := scopeCodexAccountIdentityValue(account, apiKeyID, kind, raw)
		if next != raw {
			requestBody["prompt_cache_key"] = next
			changed = true
	REDACTED
REDACTED
	return changed
REDACTED

// applyCodexAccountIdentityClientMetadataRaw scopes only the small identity
// subobjects with gjson/sjson. The passthrough hot path never unmarshals the
// potentially multi-megabyte request body.
func applyCodexAccountIdentityClientMetadataRaw(body []byte, account *Account, apiKeyID int64) ([]byte, bool, error) {
	if len(body) == 0 || codexAccountIdentityNamespace(account) == "" {
		return body, false, nil
REDACTED
	root := gjson.ParseBytes(body)
	if !root.IsObject() {
		return body, false, nil
REDACTED

	next := body
	changed := false
	originalBodySessionID := ""
	if cm := gjson.GetBytes(body, "client_metadata"); cm.IsObject() {
		clientMetadata := map[string]any{REDACTED
		if err := json.Unmarshal([]byte(cm.Raw), &clientMetadata); err != nil {
			return body, false, fmt.Errorf("decode client_metadata for account identity: %w", err)
	REDACTED
		originalBodySessionID, _ = clientMetadata["session_id"].(string)
		metadataChanged := applyCodexAccountIdentityFields(clientMetadata, account, apiKeyID)
		if applyCodexAccountIdentityEmbeddedMetadata(clientMetadata, account, apiKeyID) {
			metadataChanged = true
	REDACTED
		if metadataChanged {
			raw, err := json.Marshal(clientMetadata)
			if err != nil {
				return body, false, fmt.Errorf("encode account-scoped client_metadata: %w", err)
		REDACTED
			var setErr error
			next, setErr = sjson.SetRawBytes(next, "client_metadata", raw)
			if setErr != nil {
				return body, false, fmt.Errorf("splice account-scoped client_metadata: %w", setErr)
		REDACTED
			changed = true
	REDACTED
REDACTED
	if promptCacheKey := gjson.GetBytes(body, "prompt_cache_key"); promptCacheKey.Type == gjson.String && strings.TrimSpace(promptCacheKey.String()) != "" {
		raw := promptCacheKey.String()
		kind := "prompt-cache"
		if strings.TrimSpace(originalBodySessionID) != "" && raw == originalBodySessionID {
			kind = "session"
	REDACTED
		scoped := scopeCodexAccountIdentityValue(account, apiKeyID, kind, raw)
		if scoped != raw {
			rewritten, err := sjson.SetBytes(next, "prompt_cache_key", scoped)
			if err != nil {
				return body, false, fmt.Errorf("splice account-scoped prompt_cache_key: %w", err)
		REDACTED
			next = rewritten
			changed = true
	REDACTED
REDACTED
	return next, changed, nil
REDACTED

func applyCodexAccountIdentityHeaders(headers http.Header, account *Account, apiKeyID int64) {
	if headers == nil || codexAccountIdentityNamespace(account) == "" {
		return
REDACTED
	for _, field := range codexAccountIdentityFields {
		// Underscore session/conversation headers are rebuilt separately from the
		// prompt cache key by each request builder.
		if field.name == "session_id" {
			continue
	REDACTED
		raw := strings.TrimSpace(headers.Get(field.name))
		if raw != "" {
			headers.Set(field.name, scopeCodexAccountIdentityValue(account, apiKeyID, field.kind, raw))
	REDACTED
REDACTED
	if raw := strings.TrimSpace(headers.Get(openAIWSTurnMetadataHeader)); raw != "" {
		metadata := map[string]any{REDACTED
		if err := json.Unmarshal([]byte(raw), &metadata); err == nil && metadata != nil && applyCodexAccountIdentityFields(metadata, account, apiKeyID) {
			if rebuilt, err := json.Marshal(metadata); err == nil {
				headers.Set(openAIWSTurnMetadataHeader, string(rebuilt))
		REDACTED
	REDACTED
REDACTED
REDACTED
