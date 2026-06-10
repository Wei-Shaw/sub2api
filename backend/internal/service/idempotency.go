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
	"unicode/utf8"

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
REDACTED

type IdempotencyRepository interface {
	CreateProcessing(ctx context.Context, record *IdempotencyRecord) (bool, error)
	GetByScopeAndKeyHash(ctx context.Context, scope, keyHash string) (*IdempotencyRecord, error)
	TryReclaim(ctx context.Context, id int64, fromStatus string, now, newLockedUntil, newExpiresAt time.Time) (bool, error)
	ExtendProcessingLock(ctx context.Context, id int64, requestFingerprint string, newLockedUntil, newExpiresAt time.Time) (bool, error)
	MarkSucceeded(ctx context.Context, id int64, responseStatus int, responseBody string, expiresAt time.Time) error
	MarkFailedRetryable(ctx context.Context, id int64, errorReason string, lockedUntil, expiresAt time.Time) error
	DeleteExpired(ctx context.Context, now time.Time, limit int) (int64, error)
REDACTED

type IdempotencyConfig struct {
	DefaultTTL           time.Duration
	SystemOperationTTL   time.Duration
	ProcessingTimeout    time.Duration
	FailedRetryBackoff   time.Duration
	MaxStoredResponseLen int
	ObserveOnly          bool
REDACTED

func DefaultIdempotencyConfig() IdempotencyConfig {
	return IdempotencyConfig{
		DefaultTTL:           24 * time.Hour,
		SystemOperationTTL:   1 * time.Hour,
		ProcessingTimeout:    30 * time.Second,
		FailedRetryBackoff:   5 * time.Second,
		MaxStoredResponseLen: 64 * 1024,
		ObserveOnly:          true, // 默认先观察再强制，避免老客户端立刻中断
REDACTED
REDACTED

type IdempotencyExecuteOptions struct {
	Scope          string
	ActorScope     string
	Method         string
	Route          string
	IdempotencyKey string
	Payload        any
	TTL            time.Duration
	RequireKey     bool
REDACTED

type IdempotencyExecuteResult struct {
	Data     any
	Replayed bool
REDACTED

type IdempotencyCoordinator struct {
	repo IdempotencyRepository
	cfg  IdempotencyConfig
REDACTED

var (
	defaultIdempotencyMu  sync.RWMutex
	defaultIdempotencySvc *IdempotencyCoordinator
)

func SetDefaultIdempotencyCoordinator(svc *IdempotencyCoordinator) {
	defaultIdempotencyMu.Lock()
	defaultIdempotencySvc = svc
	defaultIdempotencyMu.Unlock()
REDACTED

func DefaultIdempotencyCoordinator() *IdempotencyCoordinator {
	defaultIdempotencyMu.RLock()
	defer defaultIdempotencyMu.RUnlock()
	return defaultIdempotencySvc
REDACTED

func DefaultWriteIdempotencyTTL() time.Duration {
	defaultTTL := DefaultIdempotencyConfig().DefaultTTL
	if coordinator := DefaultIdempotencyCoordinator(); coordinator != nil && coordinator.cfg.DefaultTTL > 0 {
		return coordinator.cfg.DefaultTTL
REDACTED
	return defaultTTL
REDACTED

func DefaultSystemOperationIdempotencyTTL() time.Duration {
	defaultTTL := DefaultIdempotencyConfig().SystemOperationTTL
	if coordinator := DefaultIdempotencyCoordinator(); coordinator != nil && coordinator.cfg.SystemOperationTTL > 0 {
		return coordinator.cfg.SystemOperationTTL
REDACTED
	return defaultTTL
REDACTED

func NewIdempotencyCoordinator(repo IdempotencyRepository, cfg IdempotencyConfig) *IdempotencyCoordinator {
	return &IdempotencyCoordinator{
		repo: repo,
		cfg:  cfg,
REDACTED
REDACTED

func NormalizeIdempotencyKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", nil
REDACTED
	if len(key) > 128 {
		return "", ErrIdempotencyKeyInvalid
REDACTED
	for _, r := range key {
		if r < 33 || r > 126 {
			return "", ErrIdempotencyKeyInvalid
	REDACTED
REDACTED
	return key, nil
REDACTED

func HashIdempotencyKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
REDACTED

func BuildIdempotencyFingerprint(method, route, actorScope string, payload any) (string, error) {
	if method == "" {
		method = "POST"
REDACTED
	if route == "" {
		route = "/"
REDACTED
	if actorScope == "" {
		actorScope = "anonymous"
REDACTED

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", ErrIdempotencyInvalidPayload.WithCause(err)
REDACTED
	sum := sha256.Sum256([]byte(
		strings.ToUpper(method) + "\n" + route + "\n" + actorScope + "\n" + string(raw),
	))
	return hex.EncodeToString(sum[:]), nil
REDACTED

func RetryAfterSecondsFromError(err error) int {
	appErr := new(infraerrors.ApplicationError)
	if !errors.As(err, &appErr) || appErr == nil || appErr.Metadata == nil {
		return 0
REDACTED
	v := strings.TrimSpace(appErr.Metadata["retry_after"])
	if v == "" {
		return 0
REDACTED
	seconds, convErr := strconv.Atoi(v)
	if convErr != nil || seconds <= 0 {
		return 0
REDACTED
	return seconds
REDACTED

func (c *IdempotencyCoordinator) Execute(
	ctx context.Context,
	opts IdempotencyExecuteOptions,
	execute func(context.Context) (any, error),
) (*IdempotencyExecuteResult, error) {
	if execute == nil {
		return nil, infraerrors.InternalServer("IDEMPOTENCY_EXECUTOR_NIL", "idempotency executor is nil")
REDACTED

	key, err := NormalizeIdempotencyKey(opts.IdempotencyKey)
	if err != nil {
		return nil, err
REDACTED
	if key == "" {
		if opts.RequireKey && !c.cfg.ObserveOnly {
			return nil, ErrIdempotencyKeyRequired
	REDACTED
		data, execErr := execute(ctx)
		if execErr != nil {
			return nil, execErr
	REDACTED
		return &IdempotencyExecuteResult{Data: dataREDACTED, nil
REDACTED
	if c.repo == nil {
		RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "repo_nil")
		return nil, ErrIdempotencyStoreUnavail
REDACTED

	if opts.Scope == "" {
		return nil, infraerrors.BadRequest("IDEMPOTENCY_SCOPE_REQUIRED", "idempotency scope is required")
REDACTED

	fingerprint, err := BuildIdempotencyFingerprint(opts.Method, opts.Route, opts.ActorScope, opts.Payload)
	if err != nil {
		return nil, err
REDACTED

	ttl := opts.TTL
	if ttl <= 0 {
		ttl = c.cfg.DefaultTTL
REDACTED
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
REDACTED

	owner, err := c.repo.CreateProcessing(ctx, record)
	if err != nil {
		RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "create_processing_error")
		logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "unknown->store_unavailable", false, map[string]string{
			"operation": "create_processing",
	REDACTED)
		return nil, ErrIdempotencyStoreUnavail.WithCause(err)
REDACTED
	if owner {
		recordIdempotencyClaim(opts.Route, opts.Scope, map[string]string{"mode": "new_claim"REDACTED)
		logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "none->processing", false, map[string]string{
			"claim_mode": "new",
	REDACTED)
REDACTED
	if !owner {
		existing, getErr := c.repo.GetByScopeAndKeyHash(ctx, opts.Scope, keyHash)
		if getErr != nil {
			RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "get_existing_error")
			logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "unknown->store_unavailable", false, map[string]string{
				"operation": "get_existing",
		REDACTED)
			return nil, ErrIdempotencyStoreUnavail.WithCause(getErr)
	REDACTED
		if existing == nil {
			RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "missing_existing")
			logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "unknown->store_unavailable", false, map[string]string{
				"operation": "missing_existing",
		REDACTED)
			return nil, ErrIdempotencyStoreUnavail
	REDACTED
		if existing.RequestFingerprint != fingerprint {
			recordIdempotencyConflict(opts.Route, opts.Scope, map[string]string{"reason": "fingerprint_mismatch"REDACTED)
			logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "existing->fingerprint_mismatch", false, nil)
			return nil, ErrIdempotencyKeyConflict
	REDACTED
		reclaimedByExpired := false
		if !existing.ExpiresAt.After(now) {
			taken, reclaimErr := c.repo.TryReclaim(ctx, existing.ID, existing.Status, now, lockedUntil, expiresAt)
			if reclaimErr != nil {
				RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "try_reclaim_expired_error")
				logIdempotencyAudit(opts.Route, opts.Scope, keyHash, existing.Status+"->store_unavailable", false, map[string]string{
					"operation": "try_reclaim_expired",
			REDACTED)
				return nil, ErrIdempotencyStoreUnavail.WithCause(reclaimErr)
		REDACTED
			if taken {
				reclaimedByExpired = true
				recordIdempotencyClaim(opts.Route, opts.Scope, map[string]string{"mode": "expired_reclaim"REDACTED)
				logIdempotencyAudit(opts.Route, opts.Scope, keyHash, existing.Status+"->processing", false, map[string]string{
					"claim_mode": "expired_reclaim",
			REDACTED)
				record.ID = existing.ID
		REDACTED else {
				latest, latestErr := c.repo.GetByScopeAndKeyHash(ctx, opts.Scope, keyHash)
				if latestErr != nil {
					RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "get_existing_after_expired_reclaim_error")
					logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "unknown->store_unavailable", false, map[string]string{
						"operation": "get_existing_after_expired_reclaim",
				REDACTED)
					return nil, ErrIdempotencyStoreUnavail.WithCause(latestErr)
			REDACTED
				if latest == nil {
					RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "missing_existing_after_expired_reclaim")
					logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "unknown->store_unavailable", false, map[string]string{
						"operation": "missing_existing_after_expired_reclaim",
				REDACTED)
					return nil, ErrIdempotencyStoreUnavail
			REDACTED
				if latest.RequestFingerprint != fingerprint {
					recordIdempotencyConflict(opts.Route, opts.Scope, map[string]string{"reason": "fingerprint_mismatch"REDACTED)
					logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "existing->fingerprint_mismatch", false, nil)
					return nil, ErrIdempotencyKeyConflict
			REDACTED
				existing = latest
		REDACTED
	REDACTED

		if !reclaimedByExpired {
			switch existing.Status {
			case IdempotencyStatusSucceeded:
				data, parseErr := c.decodeStoredResponse(existing.ResponseBody)
				if parseErr != nil {
					RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "decode_stored_response_error")
					logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "succeeded->store_unavailable", false, map[string]string{
						"operation": "decode_stored_response",
				REDACTED)
					return nil, ErrIdempotencyStoreUnavail.WithCause(parseErr)
			REDACTED
				recordIdempotencyReplay(opts.Route, opts.Scope, nil)
				logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "succeeded->replayed", true, nil)
				return &IdempotencyExecuteResult{Data: data, Replayed: trueREDACTED, nil
			case IdempotencyStatusProcessing:
				recordIdempotencyConflict(opts.Route, opts.Scope, map[string]string{"reason": "in_progress"REDACTED)
				logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "processing->conflict", false, nil)
				return nil, c.conflictWithRetryAfter(ErrIdempotencyInProgress, existing.LockedUntil, now)
			case IdempotencyStatusFailedRetryable:
				if existing.LockedUntil != nil && existing.LockedUntil.After(now) {
					recordIdempotencyConflict(opts.Route, opts.Scope, map[string]string{"reason": "retry_backoff"REDACTED)
					recordIdempotencyRetryBackoff(opts.Route, opts.Scope, nil)
					logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "failed_retryable->retry_backoff_conflict", false, nil)
					return nil, c.conflictWithRetryAfter(ErrIdempotencyRetryBackoff, existing.LockedUntil, now)
			REDACTED
				taken, reclaimErr := c.repo.TryReclaim(ctx, existing.ID, IdempotencyStatusFailedRetryable, now, lockedUntil, expiresAt)
				if reclaimErr != nil {
					RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "try_reclaim_error")
					logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "failed_retryable->store_unavailable", false, map[string]string{
						"operation": "try_reclaim",
				REDACTED)
					return nil, ErrIdempotencyStoreUnavail.WithCause(reclaimErr)
			REDACTED
				if !taken {
					recordIdempotencyConflict(opts.Route, opts.Scope, map[string]string{"reason": "reclaim_race"REDACTED)
					logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "failed_retryable->conflict", false, map[string]string{
						"conflict": "reclaim_race",
				REDACTED)
					return nil, c.conflictWithRetryAfter(ErrIdempotencyInProgress, existing.LockedUntil, now)
			REDACTED
				recordIdempotencyClaim(opts.Route, opts.Scope, map[string]string{"mode": "reclaim"REDACTED)
				logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "failed_retryable->processing", false, map[string]string{
					"claim_mode": "reclaim",
			REDACTED)
				record.ID = existing.ID
			default:
				recordIdempotencyConflict(opts.Route, opts.Scope, map[string]string{"reason": "unexpected_status"REDACTED)
				logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "existing->conflict", false, map[string]string{
					"status": existing.Status,
			REDACTED)
				return nil, ErrIdempotencyKeyConflict
		REDACTED
	REDACTED
REDACTED

	if record.ID == 0 {
		RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "record_id_missing")
		logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "processing->store_unavailable", false, map[string]string{
			"operation": "record_id_missing",
	REDACTED)
		return nil, ErrIdempotencyStoreUnavail
REDACTED

	execStart := time.Now()
	defer func() {
		recordIdempotencyProcessingDuration(opts.Route, opts.Scope, time.Since(execStart), nil)
REDACTED()

	data, execErr := execute(ctx)
	if execErr != nil {
		backoffUntil := time.Now().Add(c.cfg.FailedRetryBackoff)
		reason := infraerrors.Reason(execErr)
		if reason == "" {
			reason = "EXECUTION_FAILED"
	REDACTED
		recordIdempotencyRetryBackoff(opts.Route, opts.Scope, nil)
		logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "processing->failed_retryable", false, map[string]string{
			"reason": reason,
	REDACTED)
		if markErr := c.repo.MarkFailedRetryable(ctx, record.ID, reason, backoffUntil, expiresAt); markErr != nil {
			RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "mark_failed_retryable_error")
			logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "processing->store_unavailable", false, map[string]string{
				"operation": "mark_failed_retryable",
		REDACTED)
	REDACTED
		return nil, execErr
REDACTED

	storedBody, marshalErr := c.marshalStoredResponse(data)
	if marshalErr != nil {
		RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "marshal_response_error")
		logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "processing->store_unavailable", false, map[string]string{
			"operation": "marshal_response",
	REDACTED)
		return nil, ErrIdempotencyStoreUnavail.WithCause(marshalErr)
REDACTED
	if markErr := c.repo.MarkSucceeded(ctx, record.ID, 200, storedBody, expiresAt); markErr != nil {
		RecordIdempotencyStoreUnavailable(opts.Route, opts.Scope, "mark_succeeded_error")
		logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "processing->store_unavailable", false, map[string]string{
			"operation": "mark_succeeded",
	REDACTED)
		return nil, ErrIdempotencyStoreUnavail.WithCause(markErr)
REDACTED
	logIdempotencyAudit(opts.Route, opts.Scope, keyHash, "processing->succeeded", false, nil)

	return &IdempotencyExecuteResult{Data: dataREDACTED, nil
REDACTED

func (c *IdempotencyCoordinator) conflictWithRetryAfter(base *infraerrors.ApplicationError, lockedUntil *time.Time, now time.Time) error {
	if lockedUntil == nil {
		return base
REDACTED
	sec := int(lockedUntil.Sub(now).Seconds())
	if sec <= 0 {
		sec = 1
REDACTED
	return base.WithMetadata(map[string]string{"retry_after": strconv.Itoa(sec)REDACTED)
REDACTED

func (c *IdempotencyCoordinator) marshalStoredResponse(data any) (string, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
REDACTED
	redacted := logredact.RedactText(string(raw))
	if c.cfg.MaxStoredResponseLen > 0 && len(redacted) > c.cfg.MaxStoredResponseLen {
		redacted = truncateUTF8(redacted, c.cfg.MaxStoredResponseLen) + "...(truncated)"
REDACTED
	return redacted, nil
REDACTED

func truncateUTF8(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
REDACTED
	for maxBytes > 0 && !utf8.ValidString(s[:maxBytes]) {
		maxBytes--
REDACTED
	return s[:maxBytes]
REDACTED

func (c *IdempotencyCoordinator) decodeStoredResponse(stored *string) (any, error) {
	if stored == nil || strings.TrimSpace(*stored) == "" {
		return map[string]any{REDACTED, nil
REDACTED
	var out any
	if err := json.Unmarshal([]byte(*stored), &out); err != nil {
		return nil, fmt.Errorf("decode stored response: %w", err)
REDACTED
	return out, nil
REDACTED
