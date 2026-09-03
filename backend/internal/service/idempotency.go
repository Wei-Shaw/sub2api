package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

const (
	IdempotencyStatusProcessing      = "processing"
	IdempotencyStatusSucceeded       = "succeeded"
	IdempotencyStatusFailedRetryable = "failed_retryable"
)

var (
	ErrIdempotencyKeyRequired    = infraerrors.BadRequest("IDEMPOTENCY_KEY_REQUIRED", "idempotency key is required")
	ErrIdempotencyKeyInvalid     = infraerrors.BadRequest("IDEMPOTENCY_KEY_INVALID", "idempotency key is invalid")
	ErrIdempotencyKeyConflict    = infraerrors.Conflict("IDEMPOTENCY_KEY_CONFLICT", "idempotency key reused with different payload")
	ErrIdempotencyInProgress     = infraerrors.Conflict("IDEMPOTENCY_IN_PROGRESS", "idempotent request is still processing")
	ErrIdempotencyRetryBackoff   = infraerrors.Conflict("IDEMPOTENCY_RETRY_BACKOFF", "idempotent request is in retry backoff window")
	ErrIdempotencyStoreUnavail   = infraerrors.ServiceUnavailable("IDEMPOTENCY_STORE_UNAVAILABLE", "idempotency store unavailable")
	ErrIdempotencyInvalidPayload = infraerrors.BadRequest("IDEMPOTENCY_PAYLOAD_INVALID", "failed to normalize request payload")
	// ErrIdempotencyResponseNotReplayable 表示首次请求已成功执行，但响应体超过
	// MaxStoredResponseLen 未被缓存，因此无法原样重放。这是 409 而不是 503：
	// 服务端一切正常，重试也不会被再次执行（副作用不会重复），客户端应改用查询接口确认结果。
	ErrIdempotencyResponseNotReplayable = infraerrors.Conflict(
		"IDEMPOTENCY_RESPONSE_NOT_REPLAYABLE",
		"the original request already succeeded but its response was too large to cache and cannot be replayed",
	)
)

const (
	// idempotencyOversizedResponseMarkerKey/Value 构成「响应过大、未缓存」的占位记录。
	//
	// 历史实现把 "...(truncated)" 直接拼在已序列化的 JSON 后面，写进 response_body。
	// 那串文本不是合法 JSON，重放时 json.Unmarshal 必然失败，于是一次成功的大响应
	// 会让后续同 key 重试拿到 503 —— 与幂等层的目的完全相反。
	// 现在超限时改写为一个**合法**的标记对象：写入的永远是可解析 JSON，
	// 读路径识别到标记后返回 ErrIdempotencyResponseNotReplayable，
	// 既不会把不同的结果冒充成原始响应，也不会重复执行业务逻辑。
	//
	// 标记体是纯 ASCII，天然是合法 UTF-8，不会触发 Postgres 的编码校验；
	// 它通常比 MaxStoredResponseLen 更短，即便配置得极小也只有约 100 字节，
	// response_body 是 TEXT，不存在长度上限问题。
	idempotencyOversizedResponseMarkerKey   = "__sub2api_idempotency_response_omitted__"
	idempotencyOversizedResponseMarkerValue = "response_too_large"

	// legacyIdempotencyTruncationSuffix 是升级前写入的坏数据留下的尾巴。
	// 这些记录最长会在库里再活一个 TTL（默认 24h），按同样的「不可重放」处理，
	// 免得升级窗口内的重试仍旧收到 503。
	legacyIdempotencyTruncationSuffix = "...(truncated)"
)

type IdempotencyRecord struct {
	ID                 int64
	Scope              string
	IdempotencyKeyHash string
	RequestFingerprint string
	Status             string
	ResponseStatus     *int
	ResponseBody       *string
	ErrorReason        *string
	LockedUntil        *time.Time
	ExpiresAt          time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type IdempotencyRepository interface {
	CreateProcessing(ctx context.Context, record *IdempotencyRecord) (bool, error)
	GetByScopeAndKeyHash(ctx context.Context, scope, keyHash string) (*IdempotencyRecord, error)
	TryReclaim(ctx context.Context, id int64, fromStatus string, now, newLockedUntil, newExpiresAt time.Time) (bool, error)
	ExtendProcessingLock(ctx context.Context, id int64, requestFingerprint string, newLockedUntil, newExpiresAt time.Time) (bool, error)
	MarkSucceeded(ctx context.Context, id int64, responseStatus int, responseBody string, expiresAt time.Time) error
	MarkFailedRetryable(ctx context.Context, id int64, errorReason string, lockedUntil, expiresAt time.Time) error
	DeleteExpired(ctx context.Context, now time.Time, limit int) (int64, error)
}

type IdempotencyConfig struct {
	DefaultTTL           time.Duration
	SystemOperationTTL   time.Duration
	ProcessingTimeout    time.Duration
	FailedRetryBackoff   time.Duration
	MaxStoredResponseLen int
	ObserveOnly          bool
}

func DefaultIdempotencyConfig() IdempotencyConfig {
	return IdempotencyConfig{
		DefaultTTL:           24 * time.Hour,
		SystemOperationTTL:   1 * time.Hour,
		ProcessingTimeout:    30 * time.Second,
		FailedRetryBackoff:   5 * time.Second,
		MaxStoredResponseLen: 64 * 1024,
		ObserveOnly:          true, // 默认先观察再强制，避免老客户端立刻中断
	}
}

type IdempotencyExecuteOptions struct {
	Scope          string
	ActorScope     string
	Method         string
	Route          string
	IdempotencyKey string
	Payload        any
	TTL            time.Duration
	RequireKey     bool
}

type IdempotencyExecuteResult struct {
	Data     any
	Replayed bool
}

type IdempotencyCoordinator struct {
	repo IdempotencyRepository
	cfg  IdempotencyConfig
}

var (
	defaultIdempotencyMu  sync.RWMutex
	defaultIdempotencySvc *IdempotencyCoordinator
)

func SetDefaultIdempotencyCoordinator(svc *IdempotencyCoordinator) {
	defaultIdempotencyMu.Lock()
	defaultIdempotencySvc = svc
	defaultIdempotencyMu.Unlock()
}

func DefaultIdempotencyCoordinator() *IdempotencyCoordinator {
	defaultIdempotencyMu.RLock()
	defer defaultIdempotencyMu.RUnlock()
	return defaultIdempotencySvc
}

func DefaultWriteIdempotencyTTL() time.Duration {
	defaultTTL := DefaultIdempotencyConfig().DefaultTTL
	if coordinator := DefaultIdempotencyCoordinator(); coordinator != nil && coordinator.cfg.DefaultTTL > 0 {
		return coordinator.cfg.DefaultTTL
	}
	return defaultTTL
}

func DefaultSystemOperationIdempotencyTTL() time.Duration {
	defaultTTL := DefaultIdempotencyConfig().SystemOperationTTL
	if coordinator := DefaultIdempotencyCoordinator(); coordinator != nil && coordinator.cfg.SystemOperationTTL > 0 {
		return coordinator.cfg.SystemOperationTTL
	}
	return defaultTTL
}

func NewIdempotencyCoordinator(repo IdempotencyRepository, cfg IdempotencyConfig) *IdempotencyCoordinator {
	return &IdempotencyCoordinator{
		repo: repo,
		cfg:  cfg,
	}
}

func NormalizeIdempotencyKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", nil
	}
	if len(key) > 128 {
		return "", ErrIdempotencyKeyInvalid
	}
	for _, r := range key {
		if r < 33 || r > 126 {
			return "", ErrIdempotencyKeyInvalid
		}
	}
	return key, nil
}

func HashIdempotencyKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func BuildIdempotencyFingerprint(method, route, actorScope string, payload any) (string, error) {
	if method == "" {
		method = "POST"
	}
	if route == "" {
		route = "/"
	}
	if actorScope == "" {
		actorScope = "anonymous"
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", ErrIdempotencyInvalidPayload.WithCause(err)
	}
	sum := sha256.Sum256([]byte(
		strings.ToUpper(method) + "\n" + route + "\n" + actorScope + "\n" + string(raw),
	))
	return hex.EncodeToString(sum[:]), nil
}

func RetryAfterSecondsFromError(err error) int {
	appErr := new(infraerrors.ApplicationError)
	if !errors.As(err, &appErr) || appErr == nil || appErr.Metadata == nil {
		return 0
	}
	v := strings.TrimSpace(appErr.Metadata["retry_after"])
	if v == "" {
		return 0
	}
	seconds, convErr := strconv.Atoi(v)
	if convErr != nil || seconds <= 0 {
		return 0
	}
	return seconds
}

func (c *IdempotencyCoordinator) Execute(
	ctx context.Context,
	opts IdempotencyExecuteOptions,
	execute func(context.Context) (any, error),
) (*IdempotencyExecuteResult, error) {
	if execute == nil {
		return nil, infraerrors.InternalServer("IDEMPOTENCY_EXECUTOR_NIL", "idempotency executor is nil")
	}

	key, err := NormalizeIdempotencyKey(opts.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if key == "" {
		if opts.RequireKey && !c.cfg.ObserveOnly {
			return nil, ErrIdempotencyKeyRequired
		}
		data, execErr := execute(ctx)
		if execErr != nil {
			return nil, execErr
		}
		return &IdempotencyExecuteResult{Data: data}, nil
	}
	if c.repo == nil {
		RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "repo_nil")
		return nil, ErrIdempotencyStoreUnavail
	}

	if opts.Scope == "" {
		return nil, infraerrors.BadRequest("IDEMPOTENCY_SCOPE_REQUIRED", "idempotency scope is required")
	}

	fingerprint, err := BuildIdempotencyFingerprint(opts.Method, opts.Route, opts.ActorScope, opts.Payload)
	if err != nil {
		return nil, err
	}

	ttl := opts.TTL
	if ttl <= 0 {
		ttl = c.cfg.DefaultTTL
	}
	now := time.Now()
	expiresAt := now.Add(ttl)
	lockedUntil := now.Add(c.cfg.ProcessingTimeout)
	keyHash := HashIdempotencyKey(key)

	record := &IdempotencyRecord{
		Scope:              opts.Scope,
		IdempotencyKeyHash: keyHash,
		RequestFingerprint: fingerprint,
		Status:             IdempotencyStatusProcessing,
		LockedUntil:        &lockedUntil,
		ExpiresAt:          expiresAt,
	}

	owner, err := c.repo.CreateProcessing(ctx, record)
	if err != nil {
		RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "create_processing_error")
		logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "unknown->store_unavailable", false, map[string]string{
			"operation": "create_processing",
		})
		return nil, ErrIdempotencyStoreUnavail.WithCause(err)
	}
	if owner {
		recordIdempotencyClaim(opts.Route, opts.Scope, map[string]string{"mode": "new_claim"})
		logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "none->processing", false, map[string]string{
			"claim_mode": "new",
		})
	}
	if !owner {
		existing, getErr := c.repo.GetByScopeAndKeyHash(ctx, opts.Scope, keyHash)
		if getErr != nil {
			RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "get_existing_error")
			logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "unknown->store_unavailable", false, map[string]string{
				"operation": "get_existing",
			})
			return nil, ErrIdempotencyStoreUnavail.WithCause(getErr)
		}
		if existing == nil {
			RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "missing_existing")
			logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "unknown->store_unavailable", false, map[string]string{
				"operation": "missing_existing",
			})
			return nil, ErrIdempotencyStoreUnavail
		}
		if existing.RequestFingerprint != fingerprint {
			recordIdempotencyConflict(opts.Route, opts.Scope, map[string]string{"reason": "fingerprint_mismatch"})
			logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "existing->fingerprint_mismatch", false, nil)
			return nil, ErrIdempotencyKeyConflict
		}
		reclaimedByExpired := false
		if !existing.ExpiresAt.After(now) {
			taken, reclaimErr := c.repo.TryReclaim(ctx, existing.ID, existing.Status, now, lockedUntil, expiresAt)
			if reclaimErr != nil {
				RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "try_reclaim_expired_error")
				logIdempotencyAudit(opts.Route, opts.Scope, keyHash, existing.Status+"->store_unavailable", false, map[string]string{
					"operation": "try_reclaim_expired",
				})
				return nil, ErrIdempotencyStoreUnavail.WithCause(reclaimErr)
			}
			if taken {
				reclaimedByExpired = true
				recordIdempotencyClaim(opts.Route, opts.Scope, map[string]string{"mode": "expired_reclaim"})
				logIdempotencyAudit(opts.Route, opts.Scope, keyHash, existing.Status+"->processing", false, map[string]string{
					"claim_mode": "expired_reclaim",
				})
				record.ID = existing.ID
			} else {
				latest, latestErr := c.repo.GetByScopeAndKeyHash(ctx, opts.Scope, keyHash)
				if latestErr != nil {
					RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "get_existing_after_expired_reclaim_error")
					logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "unknown->store_unavailable", false, map[string]string{
						"operation": "get_existing_after_expired_reclaim",
					})
					return nil, ErrIdempotencyStoreUnavail.WithCause(latestErr)
				}
				if latest == nil {
					RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "missing_existing_after_expired_reclaim")
					logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "unknown->store_unavailable", false, map[string]string{
						"operation": "missing_existing_after_expired_reclaim",
					})
					return nil, ErrIdempotencyStoreUnavail
				}
				if latest.RequestFingerprint != fingerprint {
					recordIdempotencyConflict(opts.Route, opts.Scope, map[string]string{"reason": "fingerprint_mismatch"})
					logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "existing->fingerprint_mismatch", false, nil)
					return nil, ErrIdempotencyKeyConflict
				}
				existing = latest
			}
		}

		if !reclaimedByExpired {
			switch existing.Status {
			case IdempotencyStatusSucceeded:
				if isOversizedIdempotencyResponse(existing.ResponseBody) {
					recordIdempotencyConflict(opts.Route, opts.Scope, map[string]string{"reason": "response_not_replayable"})
					logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "succeeded->response_not_replayable", false, map[string]string{
						"reason": "response_too_large",
					})
					return nil, ErrIdempotencyResponseNotReplayable
				}
				data, parseErr := c.decodeStoredResponse(existing.ResponseBody)
				if parseErr != nil {
					RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "decode_stored_response_error")
					logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "succeeded->store_unavailable", false, map[string]string{
						"operation": "decode_stored_response",
					})
					return nil, ErrIdempotencyStoreUnavail.WithCause(parseErr)
				}
				recordIdempotencyReplay(opts.Route, opts.Scope, nil)
				logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "succeeded->replayed", true, nil)
				return &IdempotencyExecuteResult{Data: data, Replayed: true}, nil
			case IdempotencyStatusProcessing:
				recordIdempotencyConflict(opts.Route, opts.Scope, map[string]string{"reason": "in_progress"})
				logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "processing->conflict", false, nil)
				return nil, c.conflictWithRetryAfter(ErrIdempotencyInProgress, existing.LockedUntil, now)
			case IdempotencyStatusFailedRetryable:
				if existing.LockedUntil != nil && existing.LockedUntil.After(now) {
					recordIdempotencyConflict(opts.Route, opts.Scope, map[string]string{"reason": "retry_backoff"})
					recordIdempotencyRetryBackoff(opts.Route, opts.Scope, nil)
					logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "failed_retryable->retry_backoff_conflict", false, nil)
					return nil, c.conflictWithRetryAfter(ErrIdempotencyRetryBackoff, existing.LockedUntil, now)
				}
				taken, reclaimErr := c.repo.TryReclaim(ctx, existing.ID, IdempotencyStatusFailedRetryable, now, lockedUntil, expiresAt)
				if reclaimErr != nil {
					RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "try_reclaim_error")
					logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "failed_retryable->store_unavailable", false, map[string]string{
						"operation": "try_reclaim",
					})
					return nil, ErrIdempotencyStoreUnavail.WithCause(reclaimErr)
				}
				if !taken {
					recordIdempotencyConflict(opts.Route, opts.Scope, map[string]string{"reason": "reclaim_race"})
					logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "failed_retryable->conflict", false, map[string]string{
						"conflict": "reclaim_race",
					})
					return nil, c.conflictWithRetryAfter(ErrIdempotencyInProgress, existing.LockedUntil, now)
				}
				recordIdempotencyClaim(opts.Route, opts.Scope, map[string]string{"mode": "reclaim"})
				logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "failed_retryable->processing", false, map[string]string{
					"claim_mode": "reclaim",
				})
				record.ID = existing.ID
			default:
				recordIdempotencyConflict(opts.Route, opts.Scope, map[string]string{"reason": "unexpected_status"})
				logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "existing->conflict", false, map[string]string{
					"status": existing.Status,
				})
				return nil, ErrIdempotencyKeyConflict
			}
		}
	}

	if record.ID == 0 {
		RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "record_id_missing")
		logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "processing->store_unavailable", false, map[string]string{
			"operation": "record_id_missing",
		})
		return nil, ErrIdempotencyStoreUnavail
	}

	execStart := time.Now()
	defer func() {
		recordIdempotencyProcessingDuration(opts.Route, opts.Scope, time.Since(execStart), nil)
	}()

	data, execErr := execute(ctx)
	if execErr != nil {
		backoffUntil := time.Now().Add(c.cfg.FailedRetryBackoff)
		reason := infraerrors.Reason(execErr)
		if reason == "" {
			reason = "EXECUTION_FAILED"
		}
		recordIdempotencyRetryBackoff(opts.Route, opts.Scope, nil)
		logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "processing->failed_retryable", false, map[string]string{
			"reason": reason,
		})
		if markErr := c.repo.MarkFailedRetryable(ctx, record.ID, reason, backoffUntil, expiresAt); markErr != nil {
			RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "mark_failed_retryable_error")
			logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "processing->store_unavailable", false, map[string]string{
				"operation": "mark_failed_retryable",
			})
		}
		return nil, execErr
	}

	storedBody, marshalErr := c.marshalStoredResponse(data)
	if marshalErr != nil {
		RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "marshal_response_error")
		logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "processing->store_unavailable", false, map[string]string{
			"operation": "marshal_response",
		})
		return nil, ErrIdempotencyStoreUnavail.WithCause(marshalErr)
	}
	if isOversizedIdempotencyResponse(&storedBody) {
		logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "processing->succeeded_response_omitted", false, map[string]string{
			"reason": "response_too_large",
		})
	}
	if markErr := c.repo.MarkSucceeded(ctx, record.ID, 200, storedBody, expiresAt); markErr != nil {
		RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "mark_succeeded_error")
		logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "processing->store_unavailable", false, map[string]string{
			"operation": "mark_succeeded",
		})
		return nil, ErrIdempotencyStoreUnavail.WithCause(markErr)
	}
	logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "processing->succeeded", false, nil)

	return &IdempotencyExecuteResult{Data: data}, nil
}

func (c *IdempotencyCoordinator) conflictWithRetryAfter(base *infraerrors.ApplicationError, lockedUntil *time.Time, now time.Time) error {
	if lockedUntil == nil {
		return base
	}
	sec := int(lockedUntil.Sub(now).Seconds())
	if sec <= 0 {
		sec = 1
	}
	return base.WithMetadata(map[string]string{"retry_after": strconv.Itoa(sec)})
}

func (c *IdempotencyCoordinator) marshalStoredResponse(data any) (string, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	redacted := logredact.RedactText(string(raw))
	if c.cfg.MaxStoredResponseLen > 0 && len(redacted) > c.cfg.MaxStoredResponseLen {
		// 超限时不再截断已序列化的 JSON（截断结果不可解析），而是存一个合法的标记对象。
		return marshalOversizedIdempotencyResponseMarker(len(redacted), c.cfg.MaxStoredResponseLen)
	}
	return redacted, nil
}

// marshalOversizedIdempotencyResponseMarker 生成「响应过大、未缓存」的占位 JSON。
func marshalOversizedIdempotencyResponseMarker(responseLen, maxStoredResponseLen int) (string, error) {
	marker, err := json.Marshal(map[string]any{
		idempotencyOversizedResponseMarkerKey: idempotencyOversizedResponseMarkerValue,
		"response_bytes":                      responseLen,
		"max_stored_response_len":             maxStoredResponseLen,
	})
	if err != nil {
		return "", err
	}
	return string(marker), nil
}

// isOversizedIdempotencyResponse 判断 response_body 是否是「响应过大」占位记录。
// 只有顶层是 JSON 对象、且标记键取到约定值时才成立，避免误判业务响应。
func isOversizedIdempotencyResponse(stored *string) bool {
	if stored == nil {
		return false
	}
	trimmed := strings.TrimSpace(*stored)
	if strings.HasSuffix(trimmed, legacyIdempotencyTruncationSuffix) {
		return true
	}
	if !strings.Contains(trimmed, idempotencyOversizedResponseMarkerKey) {
		return false
	}
	var probe map[string]any
	if err := json.Unmarshal([]byte(trimmed), &probe); err != nil {
		return false
	}
	marker, _ := probe[idempotencyOversizedResponseMarkerKey].(string)
	return marker == idempotencyOversizedResponseMarkerValue
}

func (c *IdempotencyCoordinator) decodeStoredResponse(stored *string) (any, error) {
	if stored == nil || strings.TrimSpace(*stored) == "" {
		return map[string]any{}, nil
	}
	var out any
	if err := json.Unmarshal([]byte(*stored), &out); err != nil {
		return nil, fmt.Errorf("decode stored response: %w", err)
	}
	return out, nil
}
