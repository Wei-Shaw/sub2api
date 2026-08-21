package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Codex 会话切号净化（session taint rekey）。
//
// 一个 Codex 会话因额度耗尽从账号 A 切到账号 B 后，客户端 rollout 里仍带着
// A 铸造的 item id（msg_*/fc_* 等）与 A 见过的会话标识，每轮回放都会把
// "同一会话跨账号续写"的直接关联信号发给 B。本机制检测"该会话曾被多个
// 账号服务"，此后对它的所有出站请求做确定性 id 改写（rekey）：
//
//   - item id / call_id：旧值 → 前缀继承 + UUIDv5 派生新后缀（rekeyCodexTaintID）。
//     同账号同旧 id 永远得到同一新 id，input 字节跨轮稳定，prompt cache 前缀
//     命中不受影响；切号后账号变 → 全套 id 自动换新。官方客户端自己就大量
//     发送本地铸造的"账本外"id（每条 user 消息、每个工具输出），且其合成
//     output id 同样是确定性 UUIDv5 派生（codex-rs normalize.rs），该形态
//     对上游完全合法。
//   - 会话标识（session_id/conversation_id/prompt_cache_key/thread-id 等）：
//     混入账号 ID 派生（deriveCodexTaintUUID），同账号稳定、切号变值，
//     消除跨账号字符串交叉。官方 fork 即"新会话标识 + 老历史"的先例。
//
// 兼容性边界（codex-rs 源码验证）：服务端对 item id 无账本校验（无 404
// 处理路径）；call_id 按字符串配对、值任意，call/output 两侧同函数改写即
// 保持配对；encrypted_content 与 id 无绑定（compaction 密文挂客户端新铸
// cmp_ id 回放是官方固化行为），密文一律原样保留。

// codexTaintIDNamespace 是改写派生的固定 UUIDv5 命名空间。改写值必须跨轮
// 字节稳定，因此用确定性 UUIDv5 而非随机 v4/v7（与官方合成 output id 的
// 稳定性约束同构）。
var codexTaintIDNamespace = uuid.MustParse("b4b1d0e5-3c2a-4f6e-9d8a-7c1e5f2a9b34")

// rekeyCodexTaintID 把服务端铸造的旧 id 确定性改写为当前账号下的新 id。
// 前缀从旧 id 继承（msg_→msg_、fc_→fc_），后缀为 UUIDv5(accountID, oldID)。
// 无 "前缀_后缀" 形态的 legacy id 返回空串——官方客户端发送前同样剥掉它们
// （codex-rs client.rs prepare_response_items_for_request），调用方应删除。
func rekeyCodexTaintID(accountID int64, oldID string) string {
	idx := strings.Index(oldID, "_")
	if idx <= 0 || idx == len(oldID)-1 {
		return ""
	}
	prefix := oldID[:idx]
	name := fmt.Sprintf("sub2api:codex-taint:%d:%s", accountID, oldID)
	return prefix + "_" + uuid.NewSHA1(codexTaintIDNamespace, []byte(name)).String()
}

// deriveCodexTaintUUID 派生净化模式下的会话级标识（session_id/prompt_cache_key/
// thread-id 等）。domain 隔离不同用途，seed 为客户端原始会话标识。同账号同
// 会话稳定、切号变值。空 seed 返回空串。
func deriveCodexTaintUUID(domain string, apiKeyID, accountID int64, seed string) string {
	if seed == "" {
		return ""
	}
	name := fmt.Sprintf("sub2api:codex-taint-uuid:%s:%d:%d:%s", domain, apiKeyID, accountID, seed)
	return uuid.NewSHA1(codexTaintIDNamespace, []byte(name)).String()
}

// openAICodexSessionTaint 记录一个下游会话曾经由哪些账号服务过。
// everSwitched 一旦置位不复位：客户端 rollout 每轮重放切号前的 id，
// 净化必须持续到会话结束（TTL 到期）或 compaction 重建历史。
type openAICodexSessionTaint struct {
	firstAccountID int64
	everSwitched   bool
	expiresAt      time.Time
}

// noteOpenAICodexSessionAttempt 在每次 attempt 实际发出前记录（发出即记录：
// 即使该 attempt 的响应随后被 failover 丢弃，上游账本也已看到这次请求的
// 会话标识）。首次见到的账号记为 firstAccountID；不同账号出现即置
// everSwitched（单向）。TTL 与 WS 会话粘性一致，活跃会话持续续期。
func (s *OpenAIGatewayService) noteOpenAICodexSessionAttempt(c *gin.Context, account *Account) {
	if s == nil || account == nil || account.ID <= 0 {
		return
	}
	seed := openAICodexTurnStateSeed(c)
	if seed == "" {
		return
	}
	now := time.Now()
	ttl := s.openAIWSSessionStickyTTL()
	if ttl <= 0 {
		ttl = time.Hour
	}

	raw, loaded := s.openaiCodexSessionTaints.LoadOrStore(seed, openAICodexSessionTaint{
		firstAccountID: account.ID,
		expiresAt:      now.Add(ttl),
	})
	if loaded {
		if t, ok := raw.(openAICodexSessionTaint); ok {
			next := t
			next.expiresAt = now.Add(ttl)
			if account.ID != t.firstAccountID {
				next.everSwitched = true
			}
			if next != t {
				s.openaiCodexSessionTaints.Store(seed, next)
			}
		} else {
			// 类型异常的残留记录直接重建，防止永久污染。
			s.openaiCodexSessionTaints.Store(seed, openAICodexSessionTaint{
				firstAccountID: account.ID,
				expiresAt:      now.Add(ttl),
			})
		}
	}
	s.sweepOpenAICodexSessionTaints()
}

// isOpenAICodexSessionTainted 返回该会话是否曾被多个账号服务过（净化模式）。
func (s *OpenAIGatewayService) isOpenAICodexSessionTainted(c *gin.Context) bool {
	if s == nil {
		return false
	}
	seed := openAICodexTurnStateSeed(c)
	if seed == "" {
		return false
	}
	raw, ok := s.openaiCodexSessionTaints.Load(seed)
	if !ok {
		return false
	}
	t, ok := raw.(openAICodexSessionTaint)
	if !ok {
		s.openaiCodexSessionTaints.Delete(seed)
		return false
	}
	if !t.expiresAt.IsZero() && time.Now().After(t.expiresAt) {
		s.openaiCodexSessionTaints.Delete(seed)
		return false
	}
	return t.everSwitched
}

// sweepOpenAICodexSessionTaints 机会式清扫过期记录：每 256 次写入全量遍历
// 一轮，防止仅靠读侧惰性删除导致的慢泄漏（会话键无上界）。
func (s *OpenAIGatewayService) sweepOpenAICodexSessionTaints() {
	if s.openaiCodexSessionTaintWrites.Add(1)%256 != 0 {
		return
	}
	now := time.Now()
	s.openaiCodexSessionTaints.Range(func(key, value any) bool {
		t, ok := value.(openAICodexSessionTaint)
		if !ok || (!t.expiresAt.IsZero() && now.After(t.expiresAt)) {
			s.openaiCodexSessionTaints.Delete(key)
		}
		return true
	})
}

// openAICodexTaintSanitizeContextKey 是 gin context 标记键：值为本次 attempt
// 需要改写到的账号 ID（int64，0/缺失 = 不改写）。handler failover 循环在换号
// attempt 前设置，service 的 transform/header 层读取。
const openAICodexTaintSanitizeContextKey = "sub2api_openai_codex_taint_sanitize_account_id"

func setOpenAICodexTaintSanitize(c *gin.Context, accountID int64) {
	if c == nil {
		return
	}
	c.Set(openAICodexTaintSanitizeContextKey, accountID)
}

func openAICodexTaintSanitizeAccountID(c *gin.Context) int64 {
	if c == nil {
		return 0
	}
	v, ok := c.Get(openAICodexTaintSanitizeContextKey)
	if !ok {
		return 0
	}
	accountID, ok := v.(int64)
	if !ok {
		return 0
	}
	return accountID
}

// resolveOpenAICodexTaintInstallationID 返回净化模式下的 installation_id：
// 账号真实 device_id → 指纹种子派生（账号级稳定，与指纹收敛一致）→ taint
// 派生兜底（账号+API Key 确定，仍跨轮稳定）。
func resolveOpenAICodexTaintInstallationID(account *Account, apiKeyID, accountID int64) string {
	if account != nil {
		seed, _ := codexFingerprintSeed(account.Extra)
		if id := resolveConvergedInstallationID(account, seed); id != "" {
			return id
		}
	}
	return deriveCodexTaintUUID("install", apiKeyID, accountID, "installation")
}

// applyCodexTaintHeaders 对出站头应用切号净化：installation/window 头派生
// 改写、turn-metadata 内嵌标识改写。session_id/conversation_id 头由调用点
// 复用 transform 已派生的 prompt_cache_key（官方 session_id 头 == prompt_cache_key）。
// 指纹收敛档（device/session/full）在 buildUpstreamRequest 后续步骤覆盖同
// 字段，天然优先；off 档则保留此处派生值。x-codex-turn-state 已有专门的跨
// 账号剥离守卫（openai_codex_turn_state.go），此处不碰。
// 返回 error 仅当 turn-metadata 存在但无法解析——净化失败宁可拒绝请求，
// 不回退透传（跨账号矛盾信号比失败更糟）。
func applyCodexTaintHeaders(c *gin.Context, h http.Header, account *Account, apiKeyID int64) error {
	taintAccountID := openAICodexTaintSanitizeAccountID(c)
	if taintAccountID == 0 || h == nil {
		return nil
	}

	installationID := resolveOpenAICodexTaintInstallationID(account, apiKeyID, taintAccountID)
	if raw := strings.TrimSpace(h.Get("x-codex-installation-id")); raw != "" && raw != installationID {
		h.Set("x-codex-installation-id", installationID)
	}
	if raw := h.Get("x-codex-window-id"); raw != "" {
		if derived := deriveCodexTaintUUID("window", apiKeyID, taintAccountID, raw); derived != "" {
			h.Set("x-codex-window-id", derived)
		}
	}
	return applyCodexTaintTurnMetadata(h, apiKeyID, taintAccountID, installationID)
}

// applyCodexTaintTurnMetadata 改写 x-codex-turn-metadata 头内嵌标识。与指纹
// 收敛的宽松版（非法值重建）不同，这里解析失败必须报错：taint 净化失败
// 直接拒绝该请求。
func applyCodexTaintTurnMetadata(h http.Header, apiKeyID, accountID int64, installationID string) error {
	raw := strings.TrimSpace(h.Get("x-codex-turn-metadata"))
	if raw == "" {
		return nil
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil || metadata == nil {
		return fmt.Errorf("codex taint: unparsable x-codex-turn-metadata header")
	}
	if !rewriteCodexTaintMetadataMap(metadata, apiKeyID, accountID, installationID) {
		return nil
	}
	rebuilt, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("codex taint: re-encode x-codex-turn-metadata: %w", err)
	}
	h.Set("x-codex-turn-metadata", string(rebuilt))
	return nil
}

// rewriteCodexTaintMetadataMap 是 turn-metadata 字段改写的共享核心（头版与
// client_metadata 内嵌版共用）：session_id 与 thread_id 走同一派生（官方二者
// 同值，原值相等 → 派生后仍相等）；turn_id/window_id 独立派生；其余字段
// （git/sandbox 等）原样保留。返回是否有改动。
func rewriteCodexTaintMetadataMap(metadata map[string]any, apiKeyID, accountID int64, installationID string) bool {
	changed := false
	rederive := func(key, domain string) {
		if v, ok := metadata[key].(string); ok && v != "" {
			if derived := deriveCodexTaintUUID(domain, apiKeyID, accountID, v); derived != "" && derived != v {
				metadata[key] = derived
				changed = true
			}
		}
	}
	rederive("session_id", "pck")
	rederive("thread_id", "pck")
	rederive("turn_id", "turn")
	rederive("window_id", "window")
	if v, ok := metadata["installation_id"].(string); ok && v != "" && installationID != "" && v != installationID {
		metadata["installation_id"] = installationID
		changed = true
	}
	return changed
}

// applyCodexTaintClientMetadata 对请求体 client_metadata 应用切号净化，与
// 头侧（applyCodexTaintHeaders）同一套派生：顶层 session_id/thread_id/
// turn_id/x-codex-window-id/x-codex-installation-id 与内嵌
// x-codex-turn-metadata JSON 同步改写，避免头与 body 自相矛盾。
func applyCodexTaintClientMetadata(c *gin.Context, reqBody map[string]any, account *Account, apiKeyID int64) error {
	taintAccountID := openAICodexTaintSanitizeAccountID(c)
	if taintAccountID == 0 || reqBody == nil {
		return nil
	}
	existing, ok := reqBody["client_metadata"].(map[string]any)
	if !ok || existing == nil {
		return nil
	}

	changed := false
	installationID := resolveOpenAICodexTaintInstallationID(account, apiKeyID, taintAccountID)
	if v, ok := existing["x-codex-installation-id"].(string); ok && v != "" && installationID != "" && v != installationID {
		existing["x-codex-installation-id"] = installationID
		changed = true
	}
	rederive := func(key, domain string) {
		if v, ok := existing[key].(string); ok && v != "" {
			if derived := deriveCodexTaintUUID(domain, apiKeyID, taintAccountID, v); derived != "" && derived != v {
				existing[key] = derived
				changed = true
			}
		}
	}
	rederive("session_id", "pck")
	rederive("thread_id", "pck")
	rederive("turn_id", "turn")
	rederive("x-codex-window-id", "window")

	if raw, ok := existing["x-codex-turn-metadata"].(string); ok && strings.TrimSpace(raw) != "" {
		var metadata map[string]any
		if err := json.Unmarshal([]byte(raw), &metadata); err != nil || metadata == nil {
			return fmt.Errorf("codex taint: unparsable client_metadata.x-codex-turn-metadata")
		}
		if rewriteCodexTaintMetadataMap(metadata, apiKeyID, taintAccountID, installationID) {
			rebuilt, err := json.Marshal(metadata)
			if err != nil {
				return fmt.Errorf("codex taint: re-encode client_metadata.x-codex-turn-metadata: %w", err)
			}
			existing["x-codex-turn-metadata"] = string(rebuilt)
			changed = true
		}
	}

	if changed {
		reqBody["client_metadata"] = existing
	}
	return nil
}
