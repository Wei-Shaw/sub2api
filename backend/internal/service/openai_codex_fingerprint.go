package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// codexFingerprintIDsContextKey stores the account-owned installation snapshot
// between body projection and outbound header construction.
const codexFingerprintIDsContextKey = "codex_fingerprint_ids"

// stageCodexFingerprintIDs must overwrite the context even with nil. Otherwise
// failover from an enabled account to an off account can leak the prior identity.
func stageCodexFingerprintIDs(c *gin.Context, ids *codexFingerprintIDs) {
	if c != nil {
		c.Set(codexFingerprintIDsContextKey, ids)
	}
}

// applyStagedCodexFingerprintHeaders selects the official projection for the
// current request shape. The OAuth guard also blocks stale mixed-type failover state.
func applyStagedCodexFingerprintHeaders(c *gin.Context, account *Account, h http.Header) {
	if c == nil || account == nil || account.Type != AccountTypeOAuth {
		return
	}
	value, ok := c.Get(codexFingerprintIDsContextKey)
	if !ok {
		return
	}
	if ids, ok := value.(*codexFingerprintIDs); ok {
		if isOpenAIResponsesCompactPath(c) {
			applyCodexFingerprintCompactHeaders(h, ids)
		} else {
			applyCodexFingerprintHeaders(h, ids)
		}
	}
}

// codexFingerprintMode controls whether an OAuth account owns a stable Codex
// installation identity. Session, thread, turn, window, and cache identity stay
// client-owned so the proxy only emits states the official Codex client can produce.
type codexFingerprintMode string

const (
	// Off is the default and preserves the client's installation identity.
	codexFingerprintOff codexFingerprintMode = "off"
	// codexFingerprintDevice converges only installation_id. Multiple genuine
	// Codex sessions can validly originate from one persisted installation.
	codexFingerprintDevice codexFingerprintMode = "device"
	// These values are accepted only for backward compatibility and normalize
	// to device. Stateless rewriting cannot manufacture a valid Codex session graph.
	codexFingerprintSession codexFingerprintMode = "session"
	codexFingerprintFull    codexFingerprintMode = "full"
)

const codexFingerprintModeExtraKey = "codex_fingerprint_mode"

// codexFingerprintSeedExtraKey is a system-managed, account-scoped random seed.
// It is persisted so convergence identities survive restarts and restores without
// depending on the deployment-local accounts.id sequence.
const codexFingerprintSeedExtraKey = "codex_fingerprint_seed"

// normalizeCodexFingerprintMode keeps convergence opt-in. Missing or invalid
// values stay off; legacy session/full settings preserve their enabled state but
// narrow to device because the proxy no longer fabricates a session graph.
func normalizeCodexFingerprintMode(raw string) codexFingerprintMode {
	switch codexFingerprintMode(strings.TrimSpace(raw)) {
	case codexFingerprintDevice, codexFingerprintSession, codexFingerprintFull:
		return codexFingerprintDevice
	case codexFingerprintOff:
		return codexFingerprintOff
	default:
		return codexFingerprintOff
	}
}

func normalizeCodexFingerprintModeExtra(extra map[string]any) {
	if extra == nil {
		return
	}
	raw, ok := extra[codexFingerprintModeExtraKey]
	if !ok {
		return
	}
	mode := strings.TrimSpace(fmt.Sprint(raw))
	if mode == string(codexFingerprintSession) || mode == string(codexFingerprintFull) {
		extra[codexFingerprintModeExtraKey] = string(codexFingerprintDevice)
	}
}

// ShouldEnsureCodexFingerprintSeedForExtraUpdates reports whether a key-level
// extra update enables the account-owned installation identity.
func ShouldEnsureCodexFingerprintSeedForExtraUpdates(updates map[string]any) bool {
	if updates == nil {
		return false
	}
	return normalizeCodexFingerprintMode(fmt.Sprint(updates[codexFingerprintModeExtraKey])) == codexFingerprintDevice
}

func (a *Account) GetCodexFingerprintMode() codexFingerprintMode {
	if a == nil || !a.IsOpenAIOAuth() {
		return codexFingerprintOff
	}
	return normalizeCodexFingerprintMode(a.GetExtraString(codexFingerprintModeExtraKey))
}

func (a *Account) getCodexFingerprintSeed() string {
	if a == nil || !a.IsOpenAIOAuth() {
		return ""
	}
	seed := strings.TrimSpace(a.GetExtraString(codexFingerprintSeedExtraKey))
	if _, err := uuid.Parse(seed); err != nil {
		return ""
	}
	return seed
}

// initializeCodexFingerprintSeed owns seed creation for new account records.
// A duplicate must receive a fresh seed instead of inheriting its source's
// convergence identity, so replaceExisting is true on every create path.
func initializeCodexFingerprintSeed(account *Account, replaceExisting bool) {
	if account == nil || !account.IsOpenAIOAuth() {
		return
	}
	normalizeCodexFingerprintModeExtra(account.Extra)
	if !replaceExisting && account.getCodexFingerprintSeed() != "" {
		return
	}
	if account.Extra != nil {
		delete(account.Extra, codexFingerprintSeedExtraKey)
	}
	if account.GetCodexFingerprintMode() == codexFingerprintOff {
		return
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any)
	}
	account.Extra[codexFingerprintSeedExtraKey] = uuid.NewString()
}

// resolveConvergedInstallationID prefers an explicit device ID and otherwise
// uses the system-managed random account seed.
func resolveConvergedInstallationID(account *Account) string {
	if account == nil {
		return ""
	}
	if deviceID := account.GetOpenAIDeviceID(); deviceID != "" {
		return deviceID
	}
	return account.getCodexFingerprintSeed()
}

// codexFingerprintIDs is the immutable account-owned identity snapshot shared
// by every projection of one outbound attempt.
type codexFingerprintIDs struct {
	mode           codexFingerprintMode
	installationID string
}

// resolveCodexFingerprintIDs resolves only installation identity.
func resolveCodexFingerprintIDs(account *Account, mode codexFingerprintMode) *codexFingerprintIDs {
	mode = normalizeCodexFingerprintMode(string(mode))
	if mode != codexFingerprintDevice {
		return nil
	}
	ids := &codexFingerprintIDs{
		mode:           codexFingerprintDevice,
		installationID: resolveConvergedInstallationID(account),
	}
	if ids.installationID == "" {
		return nil
	}
	return ids
}

// extractClientSessionID resolves the genuine client-owned session identifier
// for turn-state and admission bookkeeping. Fingerprint convergence never rewrites it.
func extractClientSessionID(h http.Header) string {
	if v := strings.TrimSpace(h.Get("session-id")); v != "" {
		return v
	}
	return strings.TrimSpace(h.Get("session_id"))
}

// resolveCodexFingerprintIDsFromRequest keeps the request parameter in this
// boundary because callers obtain their identity snapshot per outbound attempt.
func resolveCodexFingerprintIDsFromRequest(account *Account, _ http.Header) *codexFingerprintIDs {
	if account == nil {
		return nil
	}
	mode := account.GetCodexFingerprintMode()
	return resolveCodexFingerprintIDs(account, mode)
}

// applyCodexFingerprintHeaders projects a regular HTTP or WS request after
// client header forwarding and before final Codex identity enforcement.
func applyCodexFingerprintHeaders(h http.Header, ids *codexFingerprintIDs) {
	if h == nil || ids == nil {
		return
	}
	// Regular HTTP and WS requests carry installation identity in
	// client_metadata, not as a direct header. Remove a stale client projection.
	h.Del("x-codex-installation-id")
	rewriteCodexTurnMetadataFields(h, map[string]any{
		"installation_id": ids.installationID,
	})
}

// applyCodexFingerprintCompactHeaders matches the official legacy compact
// request, which projects installation identity directly as a header.
func applyCodexFingerprintCompactHeaders(h http.Header, ids *codexFingerprintIDs) {
	if h == nil || ids == nil {
		return
	}
	h.Set("x-codex-installation-id", ids.installationID)
	rewriteCodexTurnMetadataFields(h, map[string]any{
		"installation_id": ids.installationID,
	})
}

// rewriteCodexTurnMetadataFields preserves every client-owned field not named
// in fields, including sandbox and thread_source.
func rewriteCodexTurnMetadataFields(h http.Header, fields map[string]any) {
	raw := strings.TrimSpace(h.Get("x-codex-turn-metadata"))
	if raw == "" {
		return
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return
	}
	for k, v := range fields {
		metadata[k] = v
	}
	rebuilt, err := json.Marshal(metadata)
	if err != nil {
		return
	}
	h.Set("x-codex-turn-metadata", string(rebuilt))
}

// applyCodexFingerprintClientMetadata projects installation identity into the
// regular request body without changing the client-owned session graph or cache.
func applyCodexFingerprintClientMetadata(reqBody map[string]any, ids *codexFingerprintIDs) bool {
	if reqBody == nil || ids == nil {
		return false
	}

	existing, _ := reqBody["client_metadata"].(map[string]any)
	if existing == nil {
		existing = make(map[string]any)
	}

	if !applyCodexFingerprintToClientMetadataMap(existing, ids) {
		return false
	}
	reqBody["client_metadata"] = existing
	return true
}

// applyCodexFingerprintToClientMetadataMap 是 client_metadata 改写的共享核心，
// map 版（非透传，body 已解码）与 raw 字节版（透传热路径）都经由它，保证两条
// 路径的收敛语义永不漂移。
func applyCodexFingerprintToClientMetadataMap(existing map[string]any, ids *codexFingerprintIDs) bool {
	if existing == nil || ids == nil {
		return false
	}

	modified := false

	if ids.installationID != "" {
		existing["x-codex-installation-id"] = ids.installationID
		modified = true
	}

	rewriteClientMetadataEmbeddedTurnMetadata(existing, map[string]any{
		"installation_id": ids.installationID,
	})
	return modified
}

// applyCodexFingerprintClientMetadataRaw 在原始 JSON 字节上改写 client_metadata，
// 供透传路径使用——透传是热路径，禁止对可能高达数十 MB 的 body 做全量
// Unmarshal（见 forwardOpenAIPassthrough 的轻量提取注释）。实现为：gjson 提取
// client_metadata 小对象单独解码，经共享核心改写后 sjson 一次性拼回，body
// 其余字节原样保留。语义与 applyCodexFingerprintClientMetadata 逐点一致
// （含"非对象值整体替换为收敛集合"的行为）。
func applyCodexFingerprintClientMetadataRaw(body []byte, ids *codexFingerprintIDs) ([]byte, bool, error) {
	if len(body) == 0 || ids == nil {
		return body, false, nil
	}
	// 非 JSON 对象的 body（数组/标量/畸形）没有 client_metadata 语义，
	// sjson 在这类根上写字段会改写整体结构，直接放行保持原样。
	if !gjson.ParseBytes(body).IsObject() {
		return body, false, nil
	}

	existing := map[string]any{}
	if cm := gjson.GetBytes(body, "client_metadata"); cm.IsObject() {
		if err := json.Unmarshal([]byte(cm.Raw), &existing); err != nil {
			return body, false, fmt.Errorf("decode client_metadata for fingerprint: %w", err)
		}
	}

	if !applyCodexFingerprintToClientMetadataMap(existing, ids) {
		return body, false, nil
	}

	raw, err := json.Marshal(existing)
	if err != nil {
		return body, false, fmt.Errorf("encode converged client_metadata: %w", err)
	}
	next, err := sjson.SetRawBytes(body, "client_metadata", raw)
	if err != nil {
		return body, false, fmt.Errorf("splice converged client_metadata: %w", err)
	}
	return next, true, nil
}

// rewriteClientMetadataEmbeddedTurnMetadata 改写 client_metadata 中内嵌的
// x-codex-turn-metadata JSON 字符串里的指定字段。
func rewriteClientMetadataEmbeddedTurnMetadata(clientMetadata map[string]any, fields map[string]any) {
	raw, ok := clientMetadata["x-codex-turn-metadata"].(string)
	if !ok || raw == "" {
		return
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return
	}
	for k, v := range fields {
		metadata[k] = v
	}
	if rebuilt, err := json.Marshal(metadata); err == nil {
		clientMetadata["x-codex-turn-metadata"] = string(rebuilt)
	}
}
