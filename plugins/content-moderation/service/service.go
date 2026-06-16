// Package service holds the migrated content-moderation logic: policy
// evaluation, OpenAI-moderation API calls, keyword matching, the flagged-hash
// cache, async worker pool, and auto-ban escalation. It was migrated from
// backend/internal/service/content_moderation*.go. Core repositories and the
// EmailService are replaced by SDK-provided resources (DB, Redis, Settings,
// Host); email sending is logged rather than dispatched until a host email
// extension exists.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
	apperrors "github.com/Wei-Shaw/sub2api/plugins/content-moderation/internal/errors"
	"github.com/Wei-Shaw/sub2api/plugins/content-moderation/internal/pagination"
)

// ModerationService is the migrated content-moderation engine. It replaces the
// core's ContentModerationService; instead of injected repositories it holds
// SDK-provided resources and a Repository/HashCache built on top of them.
type ModerationService struct {
	settings  pluginsdk.SettingsClient
	host      pluginsdk.HostPaymentClient
	logger    *slog.Logger
	repo      Repository
	hashCache HashCache

	httpClient               *http.Client
	asyncQueue               chan contentModerationTask
	apiKeyCursor             atomic.Uint64
	asyncActive              atomic.Int64
	asyncEnqueued            atomic.Int64
	asyncDropped             atomic.Int64
	asyncProcessed           atomic.Int64
	asyncErrors              atomic.Int64
	preBlockActive           atomic.Int64
	preBlockChecked          atomic.Int64
	preBlockAllowed          atomic.Int64
	preBlockBlocked          atomic.Int64
	preBlockErrors           atomic.Int64
	preBlockLatencyTotalMS   atomic.Int64
	lastCleanupUnix          atomic.Int64
	lastCleanupDeletedHit    atomic.Int64
	lastCleanupDeletedNonHit atomic.Int64
	keyHealthMu              sync.Mutex
	keyHealth                map[string]*contentModerationKeyHealth
}

type contentModerationTask struct {
	input            ContentModerationCheckInput
	content          ContentModerationInput
	inputHash        string
	log              *ContentModerationLog
	config           *ContentModerationConfig
	recordHash       bool
	applySideEffects bool
	enqueuedAt       time.Time
}

// NewModerationService builds the engine from the SDK context plus the
// SQL repository and Redis hash cache. It launches the async worker pool and
// the retention-cleanup loop.
func NewModerationService(ctx pluginsdk.PluginContext, repo Repository, hashCache HashCache) *ModerationService {
	svc := &ModerationService{
		settings:   ctx.Settings(),
		host:       ctx.HostPayment(),
		logger:     ctx.Logger(),
		repo:       repo,
		hashCache:  hashCache,
		httpClient: &http.Client{},
		asyncQueue: make(chan contentModerationTask, maxContentModerationQueueSize),
		keyHealth:  make(map[string]*contentModerationKeyHealth),
	}
	for i := 0; i < maxContentModerationWorkerCount; i++ {
		go svc.worker(i)
	}
	go svc.cleanupWorker()
	return svc
}

func (s *ModerationService) GetConfig(ctx context.Context) (*ContentModerationConfigView, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	return s.configView(cfg), nil
}

func (s *ModerationService) Check(ctx context.Context, input ContentModerationCheckInput) (*ContentModerationDecision, error) {
	allow := &ContentModerationDecision{Allowed: true, Action: ContentModerationActionAllow}
	if s == nil {
		return allow, nil
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		s.logger.Warn("content_moderation.skip_config_load_failed",
			"user_id", input.UserID, "endpoint", input.Endpoint, "protocol", input.Protocol, "error", err)
		return allow, nil
	}
	if !cfg.Enabled {
		return allow, nil
	}
	if cfg.Mode == ContentModerationModeOff {
		return allow, nil
	}
	if !cfg.includesGroup(input.GroupID) {
		return allow, nil
	}
	if !cfg.includesModel(input.Model) {
		return allow, nil
	}
	content := ExtractContentModerationInput(input.Protocol, input.Body)
	if content.IsEmpty() {
		return allow, nil
	}
	content.Normalize()
	hashText := content.Hash()
	if decision, handled := s.checkPreBlockGuards(ctx, input, cfg, content, hashText); handled {
		return decision, nil
	}
	if !cfg.shouldSample(hashText) {
		if cfg.Mode == ContentModerationModePreBlock {
			s.recordPreBlockSyncMetric(0, ContentModerationActionAllow)
		}
		return allow, nil
	}
	if len(cfg.apiKeys()) == 0 {
		if cfg.Mode == ContentModerationModePreBlock {
			s.recordPreBlockSyncMetric(0, ContentModerationActionError)
		}
		s.logger.Warn("content_moderation.skip_no_audit_api_keys",
			"user_id", input.UserID, "endpoint", input.Endpoint, "protocol", input.Protocol)
		return allow, nil
	}
	if cfg.Mode == ContentModerationModeObserve {
		s.enqueueAsync(input, cfg, content, hashText)
		return allow, nil
	}
	return s.checkSync(ctx, input, cfg, content, hashText, nil, true), nil
}

// checkPreBlockGuards applies the synchronous keyword and hash short-circuits
// that run before sampling and the OpenAI API call. Returns (decision, true)
// when the request is decided here.
func (s *ModerationService) checkPreBlockGuards(ctx context.Context, input ContentModerationCheckInput, cfg *ContentModerationConfig, content ContentModerationInput, hashText string) (*ContentModerationDecision, bool) {
	allow := &ContentModerationDecision{Allowed: true, Action: ContentModerationActionAllow}
	if cfg.Mode == ContentModerationModePreBlock {
		if cfg.KeywordBlockingMode != ContentModerationKeywordModeAPIOnly && len(cfg.BlockedKeywords) > 0 {
			if keyword, hit := matchBlockedKeyword(content.Text, cfg.BlockedKeywords); hit {
				s.recordPreBlockSyncMetric(0, ContentModerationActionKeywordBlock)
				s.logger.Info("content_moderation.keyword_block", "user_id", input.UserID, "endpoint", input.Endpoint, "keyword", keyword)
				scores := map[string]float64{contentModerationKeywordCategory: 1.0}
				log := s.buildLog(input, cfg, ContentModerationActionKeywordBlock, true, contentModerationKeywordCategory, 1.0, scores, content.ExcerptText(), nil, nil, "")
				s.enqueueRecord(input, cfg, log, hashText, false, true)
				return &ContentModerationDecision{
					Allowed: false, Blocked: true, Flagged: true,
					Message: cfg.BlockMessage, StatusCode: cfg.BlockStatus,
					HighestCategory: contentModerationKeywordCategory, HighestScore: 1.0,
					CategoryScores: scores, Action: ContentModerationActionKeywordBlock,
				}, true
			}
		}
		if cfg.KeywordBlockingMode == ContentModerationKeywordModeKeywordOnly {
			s.recordPreBlockSyncMetric(0, ContentModerationActionAllow)
			return allow, true
		}
	}
	if cfg.PreHashCheckEnabled && s.hashCache != nil {
		matched, err := s.hashCache.HasFlaggedInputHash(ctx, hashText)
		if err != nil {
			s.logger.Warn("content_moderation.hash_check_failed", "user_id", input.UserID, "endpoint", input.Endpoint, "error", err)
		}
		if matched {
			if cfg.Mode == ContentModerationModePreBlock {
				s.recordPreBlockSyncMetric(0, ContentModerationActionHashBlock)
			}
			s.logger.Info("content_moderation.hash_block", "user_id", input.UserID, "endpoint", input.Endpoint, "input_hash", hashText)
			message := cfg.BlockMessage
			if message != "" {
				message = fmt.Sprintf("%s（hash: %s）", message, hashText)
			}
			scores := map[string]float64{"hash": 1.0}
			log := s.buildLog(input, cfg, ContentModerationActionHashBlock, true, "hash", 1.0, scores, content.ExcerptText(), nil, nil, "")
			s.enqueueRecord(input, cfg, log, hashText, false, false)
			return &ContentModerationDecision{
				Allowed: false, Blocked: true, Flagged: true,
				Message: message, StatusCode: cfg.BlockStatus,
				InputHash: hashText, Action: ContentModerationActionHashBlock,
			}, true
		}
	}
	return nil, false
}

func (s *ModerationService) checkSync(ctx context.Context, input ContentModerationCheckInput, cfg *ContentModerationConfig, content ContentModerationInput, hashText string, queueDelay *int, allowBlock bool) *ContentModerationDecision {
	allow := &ContentModerationDecision{Allowed: true, Action: ContentModerationActionAllow}
	trackPreBlock := queueDelay == nil && allowBlock && cfg != nil && cfg.Mode == ContentModerationModePreBlock
	if trackPreBlock {
		s.preBlockActive.Add(1)
		defer s.preBlockActive.Add(-1)
	}
	start := time.Now()
	result, err := s.callModeration(ctx, cfg, content.ModerationInput(), trackPreBlock)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		if trackPreBlock {
			s.recordPreBlockSyncMetric(latency, ContentModerationActionError)
		}
		s.logger.Warn("content_moderation.audit_api_failed",
			"user_id", input.UserID, "endpoint", input.Endpoint, "mode", cfg.Mode, "latency_ms", latency, "error", err)
		if queueDelay != nil {
			s.asyncErrors.Add(1)
		}
		if cfg.RecordNonHits {
			log := s.buildLog(input, cfg, ContentModerationActionError, false, "", 0, nil, content.ExcerptText(), &latency, queueDelay, err.Error())
			_ = s.repo.CreateLog(ctx, log)
		}
		return allow
	}

	flagged, highestCategory, highestScore := evaluateModerationScores(result.CategoryScores, cfg.Thresholds)
	action := ContentModerationActionAllow
	blocked := false
	if allowBlock && flagged && cfg.Mode == ContentModerationModePreBlock {
		action = ContentModerationActionBlock
		blocked = true
	}
	if trackPreBlock {
		s.recordPreBlockSyncMetric(latency, action)
	}
	s.logger.Info("content_moderation.audit_result",
		"user_id", input.UserID, "endpoint", input.Endpoint, "mode", cfg.Mode,
		"flagged", flagged, "blocked", blocked, "action", action,
		"highest_category", highestCategory, "highest_score", highestScore, "latency_ms", latency)
	if flagged || cfg.RecordNonHits {
		log := s.buildLog(input, cfg, action, flagged, highestCategory, highestScore, result.CategoryScores, content.ExcerptText(), &latency, queueDelay, "")
		if queueDelay == nil && cfg.Mode == ContentModerationModePreBlock {
			s.enqueueRecord(input, cfg, log, hashText, flagged, flagged)
		} else {
			s.persistContentModerationLog(ctx, cfg, log, hashText, flagged, flagged)
		}
	}
	if blocked {
		return &ContentModerationDecision{
			Allowed: false, Blocked: true, Flagged: true,
			Message: cfg.BlockMessage, StatusCode: cfg.BlockStatus,
			HighestCategory: highestCategory, HighestScore: highestScore,
			CategoryScores: result.CategoryScores, Action: action,
		}
	}
	return &ContentModerationDecision{
		Allowed: true, Flagged: flagged,
		HighestCategory: highestCategory, HighestScore: highestScore,
		CategoryScores: result.CategoryScores, Action: action,
	}
}

func (s *ModerationService) recordPreBlockSyncMetric(latencyMS int, action string) {
	if s == nil {
		return
	}
	s.preBlockChecked.Add(1)
	if latencyMS < 0 {
		latencyMS = 0
	}
	s.preBlockLatencyTotalMS.Add(int64(latencyMS))
	switch action {
	case ContentModerationActionBlock, ContentModerationActionHashBlock, ContentModerationActionKeywordBlock:
		s.preBlockBlocked.Add(1)
	case ContentModerationActionError:
		s.preBlockErrors.Add(1)
	default:
		s.preBlockAllowed.Add(1)
	}
}

func (s *ModerationService) ListLogs(ctx context.Context, filter ContentModerationLogFilter) ([]ContentModerationLog, *pagination.PaginationResult, error) {
	if filter.Pagination.Page <= 0 {
		filter.Pagination.Page = 1
	}
	if filter.Pagination.PageSize <= 0 {
		filter.Pagination.PageSize = 20
	}
	if filter.Pagination.PageSize > 100 {
		filter.Pagination.PageSize = 100
	}
	if filter.Pagination.SortOrder == "" {
		filter.Pagination.SortOrder = "desc"
	}
	return s.repo.ListLogs(ctx, filter)
}

func (s *ModerationService) UnbanUser(ctx context.Context, userID int64) (*ContentModerationUnbanUserResult, error) {
	if s == nil || s.host == nil {
		return nil, apperrors.InternalServer("CONTENT_MODERATION_HOST_UNAVAILABLE", "用户服务不可用")
	}
	if userID <= 0 {
		return nil, apperrors.BadRequest("INVALID_USER_ID", "用户 ID 无效")
	}
	if err := s.host.SetUserDisabled(ctx, userID, false, "content_moderation_unban"); err != nil {
		if errors.Is(err, pluginsdk.ErrHostUserDisableUnavailable) {
			return nil, apperrors.InternalServer("CONTENT_MODERATION_HOST_UNAVAILABLE", "用户服务不可用")
		}
		return nil, fmt.Errorf("content moderation unban user: %w", err)
	}
	return &ContentModerationUnbanUserResult{UserID: userID, Status: "active"}, nil
}

func (s *ModerationService) DeleteFlaggedInputHash(ctx context.Context, inputHash string) (*ContentModerationDeleteHashResult, error) {
	inputHash = normalizeContentModerationHash(inputHash)
	if inputHash == "" {
		return nil, apperrors.BadRequest("INVALID_CONTENT_MODERATION_HASH", "风险输入哈希无效")
	}
	if s == nil || s.hashCache == nil {
		return nil, apperrors.InternalServer("CONTENT_MODERATION_HASH_CACHE_UNAVAILABLE", "内容审计哈希缓存不可用")
	}
	deleted, err := s.hashCache.DeleteFlaggedInputHash(ctx, inputHash)
	if err != nil {
		return nil, fmt.Errorf("delete content moderation flagged hash: %w", err)
	}
	return &ContentModerationDeleteHashResult{InputHash: inputHash, Deleted: deleted}, nil
}

func (s *ModerationService) ClearFlaggedInputHashes(ctx context.Context) (*ContentModerationClearHashesResult, error) {
	if s == nil || s.hashCache == nil {
		return nil, apperrors.InternalServer("CONTENT_MODERATION_HASH_CACHE_UNAVAILABLE", "内容审计哈希缓存不可用")
	}
	deleted, err := s.hashCache.ClearFlaggedInputHashes(ctx)
	if err != nil {
		return nil, fmt.Errorf("clear content moderation flagged hashes: %w", err)
	}
	return &ContentModerationClearHashesResult{Deleted: deleted}, nil
}
