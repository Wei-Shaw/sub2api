package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/upstream"
)

// 本文件把「一次请求的失败链」归一化落库（migrations/251 的 upstream_attempts）。
//
// 为什么在这里做：ops_error_logs 已经把每一跳攒进 entry.UpstreamErrors（JSONB），
// 而 OpsService.RecordErrorBatch 是所有错误日志唯一落库点。在这里双写，
// 90 个 appendOpsUpstreamError 调用点一行都不用改。
//
// 保留 JSONB 的理由是既有排障页面还在读它；新增表的理由是 JSONB 无法按账号建索引、
// 无法 join account_status_events、也无法独立设置保留期。

// OpsUpstreamAttemptRecord 是 upstream_attempts 的一行。
type OpsUpstreamAttemptRecord struct {
	RequestID      string
	AttemptNo      int
	AccountID      *int64
	RetryFrom      *int64
	Platform       string
	ModelReq       string
	ModelUpstream  *string
	UpstreamURL    *string
	UpstreamStatus *int
	FailureKind    string
	ErrorCode      *string
	ErrorMsg       *string
	LatencyMs      *int
	AccountState   *string
	CoolingUntil   *time.Time
	CredentialFP   *string
	ProxyID        *int64
	CreatedAt      time.Time
}

// OpsAccountStatusEventRecord 是 account_status_events 的一行。
type OpsAccountStatusEventRecord struct {
	AccountID    int64
	FromState    *string
	ToState      string
	Reason       *string
	ErrorCode    *string
	CoolingUntil *time.Time
	Source       string
	RequestID    *string
	OperatorID   *int64
	CreatedAt    time.Time
}

// opsTraceRegistry 复用与运行时同一套适配器做失败分类，保证「落库的分类」和
// 「调度器据此摘号/冷却的分类」是同一个判断，而不是两套。
var opsTraceRegistry = func() *upstream.Registry {
	r, err := upstream.NewRegistry(
		upstream.NewClaudeAdapter(),
		upstream.NewOpenAIAdapter(),
		upstream.NewAntigravityAdapter(),
	)
	if err != nil {
		// 三个固定的内置适配器不会冲突；失败只可能是编程错误，降级为通用分类。
		return nil
	}
	return r
}()

// upstreamPlatformForTrace 把持久化的 platform 字符串映射到适配器平台。
// 注意两套命名并不一致：库里是 anthropic/openai/gemini/antigravity，
// 适配器侧是 claude/openai/antigravity。
func upstreamPlatformForTrace(platform string) (upstream.Platform, bool) {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case PlatformAnthropic, "claude":
		return upstream.PlatformClaude, true
	case PlatformOpenAI:
		return upstream.PlatformOpenAI, true
	case PlatformAntigravity:
		return upstream.PlatformAntigravity, true
	default:
		return "", false
	}
}

// classifyTraceFailure 给一次历史尝试打上 FailureKind。平台已知时走适配器
// （这样 OpenAI 的 429+insufficient_quota 会被记成 auth 而不是 rate_limit），
// 否则退回通用状态码映射。
func classifyTraceFailure(platform string, status int, body []byte) upstream.FailureKind {
	if status <= 0 {
		// 连接层错误：没有状态码，只能算瞬时故障。
		return upstream.FailureTransient
	}
	if p, ok := upstreamPlatformForTrace(platform); ok && opsTraceRegistry != nil {
		if adapter, found := opsTraceRegistry.Get(p); found {
			return adapter.Classify(time.Now(), status, nil, body).Kind
		}
	}
	return upstream.ClassifyStatus(status, nil, body).Kind
}

var (
	// 从已脱敏的响应片段里提取上游错误码；不解析整段 JSON，因为片段可能被截断。
	//
	// 顺序很重要：Anthropic 的信封是 {"type":"error","error":{"type":"..."}}，
	// 若先匹配裸 "type"，拿到的会是外层 envelope 的 "error"，等于没提取到信息。
	traceCodePatterns = []*regexp.Regexp{
		regexp.MustCompile(`"code"\s*:\s*"([A-Za-z0-9_.\-]{1,64})"`),
		regexp.MustCompile(`"error"\s*:\s*\{[^{}]{0,200}?"type"\s*:\s*"([A-Za-z0-9_.\-]{1,64})"`),
		regexp.MustCompile(`"type"\s*:\s*"([A-Za-z0-9_.\-]{1,64})"`),
		regexp.MustCompile(`"status"\s*:\s*"([A-Za-z0-9_.\-]{1,64})"`),
	}
)

// extractTraceErrorCode 尝试从响应片段里取错误码，取不到返回 nil。
func extractTraceErrorCode(body string) *string {
	if body == "" {
		return nil
	}
	for _, re := range traceCodePatterns {
		if m := re.FindStringSubmatch(body); len(m) == 2 {
			code := m[1]
			return &code
		}
	}
	return nil
}

// truncateTraceMsg 截断到列宽，并抹掉换行，避免日志内容撑爆单行。
func truncateTraceMsg(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " "))
	const maxLen = 512
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}

// credentialFingerprint 计算账号当前凭据的指纹（sha256 前 8 字节）。
//
// 目的不是保密，而是回答「同一个 account_id 在不同时间报错，是账号变了还是
// 凭据轮换导致的」。8 字节足以区分，又不可逆。
func credentialFingerprint(account *Account) *string {
	if account == nil {
		return nil
	}
	for _, key := range []string{"access_token", "api_key", "refresh_token"} {
		if v := strings.TrimSpace(account.GetCredential(key)); v != "" {
			sum := sha256.Sum256([]byte(v))
			fp := hex.EncodeToString(sum[:8])
			return &fp
		}
	}
	return nil
}

// traceEventsFromEntry 取出这次请求的逐跳错误事件。
//
// 注意顺序：prepareErrorLogInput 在序列化后会把 entry.UpstreamErrors 置空
// （service/ops_service.go 的 sanitizeOpsUpstreamErrors），所以一旦走完 prepare，
// 切片就是 nil，只能从 UpstreamErrorsJSON 回读。两条路径都要覆盖，否则线上只有
// JSONB 有数据、归一化表永远是空的——这正是端到端验证抓出来的问题。
func traceEventsFromEntry(entry *OpsInsertErrorLogInput) []*OpsUpstreamErrorEvent {
	if entry == nil {
		return nil
	}
	if len(entry.UpstreamErrors) > 0 {
		return entry.UpstreamErrors
	}
	if entry.UpstreamErrorsJSON == nil {
		return nil
	}
	raw := strings.TrimSpace(*entry.UpstreamErrorsJSON)
	if raw == "" {
		return nil
	}
	var events []*OpsUpstreamErrorEvent
	if err := json.Unmarshal([]byte(raw), &events); err != nil {
		return nil
	}
	return events
}

// buildUpstreamAttemptRecords 把一批错误日志 entry 展开成 attempt 行。
//
// 纯函数：不碰数据库、不读时钟，fingerprint 通过回调注入，便于单测覆盖
// 「failover 链 → retry_from」这类容易写错的语义。
func buildUpstreamAttemptRecords(
	entries []*OpsInsertErrorLogInput,
	now time.Time,
	fingerprint func(accountID int64) *string,
) []*OpsUpstreamAttemptRecord {
	records := make([]*OpsUpstreamAttemptRecord, 0, len(entries))

	for _, entry := range entries {
		if entry == nil || entry.RequestID == "" {
			continue
		}
		events := traceEventsFromEntry(entry)
		if len(events) == 0 {
			continue
		}

		var modelUpstreamPtr *string
		if v := strings.TrimSpace(entry.UpstreamModel); v != "" {
			modelUpstreamPtr = &v
		} else if v := strings.TrimSpace(entry.Model); v != "" {
			modelUpstreamPtr = &v
		}

		var prevAccountID int64
		attemptNo := 0
		for _, ev := range events {
			if ev == nil {
				continue
			}
			attemptNo++

			platform := strings.TrimSpace(ev.Platform)
			if platform == "" {
				platform = entry.Platform
			}

			msg := strings.TrimSpace(ev.Message)
			if msg == "" {
				msg = strings.TrimSpace(ev.Detail)
			}
			var bodyForClassify []byte
			if ev.UpstreamResponseBody != "" {
				bodyForClassify = []byte(ev.UpstreamResponseBody)
			} else if msg != "" {
				bodyForClassify = []byte(msg)
			}

			rec := &OpsUpstreamAttemptRecord{
				RequestID:     entry.RequestID,
				AttemptNo:     attemptNo,
				Platform:      platform,
				ModelReq:      strings.TrimSpace(entry.RequestedModel),
				ModelUpstream: modelUpstreamPtr,
				CreatedAt:     now,
				FailureKind:   classifyTraceFailure(platform, ev.UpstreamStatusCode, bodyForClassify).String(),
			}
			if rec.ModelReq == "" {
				rec.ModelReq = strings.TrimSpace(entry.Model)
			}
			if ev.AccountID > 0 {
				accountID := ev.AccountID
				rec.AccountID = &accountID
				if fingerprint != nil {
					rec.CredentialFP = fingerprint(accountID)
				}
			}
			// retry_from：链上上一跳的账号，这就是「换了哪个号」的直接证据。
			if prevAccountID > 0 {
				from := prevAccountID
				rec.RetryFrom = &from
			}
			if ev.AccountID > 0 {
				prevAccountID = ev.AccountID
			}
			if ev.ProxyID != nil && *ev.ProxyID > 0 {
				proxyID := *ev.ProxyID
				rec.ProxyID = &proxyID
			}
			if status := ev.UpstreamStatusCode; status > 0 {
				v := status
				rec.UpstreamStatus = &v
			}
			if url := strings.TrimSpace(ev.UpstreamURL); url != "" {
				rec.UpstreamURL = &url
			}
			if msg != "" {
				truncated := truncateTraceMsg(msg)
				rec.ErrorMsg = &truncated
			}
			// 错误码优先取自响应体：不少链路只填 Detail（原始错误 JSON），
			// UpstreamResponseBody 为空；只看后者会退化成把 Kind（如 failover）
			// 当成 error_code，读起来像错误码但其实不是。
			codeBody := ev.UpstreamResponseBody
			if strings.TrimSpace(codeBody) == "" {
				codeBody = ev.Detail
			}
			if code := extractTraceErrorCode(codeBody); code != nil {
				rec.ErrorCode = code
			} else if kind := strings.TrimSpace(ev.Kind); kind != "" {
				// Failover 链里的 kind 本身也是有效线索（http_error / retry_exhausted 等）
				c := kind
				if len(c) > 64 {
					c = c[:64]
				}
				rec.ErrorCode = &c
			}
			records = append(records, rec)
		}
	}
	return records
}

// recordUpstreamAttempts 落库本次批次的 attempt 链。任何错误只记日志，
// 绝不影响请求或错误日志本身。
func (s *OpsService) recordUpstreamAttempts(ctx context.Context, entries []*OpsInsertErrorLogInput) {
	if s == nil || s.opsRepo == nil || len(entries) == 0 {
		return
	}

	// 同一批次里同一账号只查一次，避免把批量刷盘变成 N 次账号查询。
	fpCache := make(map[int64]*string, 4)
	records := buildUpstreamAttemptRecords(entries, time.Now(), func(accountID int64) *string {
		return s.lookupCredentialFingerprint(ctx, accountID, fpCache)
	})
	if len(records) == 0 {
		return
	}
	if _, err := s.opsRepo.InsertUpstreamAttempts(ctx, records); err != nil {
		log.Printf("[Ops] upstream attempt insert failed (%d rows): %v", len(records), err)
	}
}

// lookupCredentialFingerprint 查账号并算指纹；失败返回 nil（不允许打断主流程）。
func (s *OpsService) lookupCredentialFingerprint(ctx context.Context, accountID int64, cache map[int64]*string) *string {
	if s == nil || s.accountRepo == nil || accountID <= 0 {
		return nil
	}
	if fp, ok := cache[accountID]; ok {
		return fp
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		cache[accountID] = nil
		return nil
	}
	fp := credentialFingerprint(account)
	cache[accountID] = fp
	return fp
}

// RecordAccountStatusEvents 落账号状态变更事件。由 accountStatusEventSink 调用
// （见 account_status_event_sink.go）。
func (s *OpsService) RecordAccountStatusEvents(ctx context.Context, events []*OpsAccountStatusEventRecord) error {
	if s == nil || s.opsRepo == nil || len(events) == 0 {
		return nil
	}
	if _, err := s.opsRepo.InsertAccountStatusEvents(ctx, events); err != nil {
		log.Printf("[Ops] account status event insert failed (%d rows): %v", len(events), err)
		return err
	}
	return nil
}
