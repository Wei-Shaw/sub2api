package service

import (
	"context"
	"errors"
	"strings"
	"time"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

func (s *ModerationService) buildLog(input ContentModerationCheckInput, cfg *ContentModerationConfig, action string, flagged bool, highestCategory string, highestScore float64, scores map[string]float64, text string, latency *int, queueDelay *int, errText string) *ContentModerationLog {
	var userID *int64
	if input.UserID > 0 {
		userID = &input.UserID
	}
	var apiKeyID *int64
	if input.APIKeyID > 0 {
		apiKeyID = &input.APIKeyID
	}
	return &ContentModerationLog{
		RequestID:         input.RequestID,
		UserID:            userID,
		UserEmail:         input.UserEmail,
		APIKeyID:          apiKeyID,
		APIKeyName:        input.APIKeyName,
		GroupID:           cloneInt64Ptr(input.GroupID),
		GroupName:         input.GroupName,
		Endpoint:          input.Endpoint,
		Provider:          input.Provider,
		Model:             input.Model,
		Mode:              cfg.Mode,
		Action:            action,
		Flagged:           flagged,
		HighestCategory:   highestCategory,
		HighestScore:      highestScore,
		CategoryScores:    cloneFloatMap(scores),
		ThresholdSnapshot: cloneFloatMap(cfg.Thresholds),
		InputExcerpt:      trimRunes(redactContentModerationSecrets(text), maxModerationExcerptRunes),
		UpstreamLatencyMS: latency,
		QueueDelayMS:      queueDelay,
		Error:             errText,
	}
}

func (s *ModerationService) persistContentModerationLog(ctx context.Context, cfg *ContentModerationConfig, log *ContentModerationLog, hashText string, recordHash bool, applySideEffects bool) {
	if s == nil || log == nil {
		return
	}
	if recordHash && s.hashCache != nil {
		if err := s.hashCache.RecordFlaggedInputHash(ctx, hashText); err != nil {
			s.logger.Warn("content_moderation.record_hash_failed", "user_id", contentModerationEmailUserID(log), "endpoint", log.Endpoint, "error", err)
		}
	}
	autoBanJustApplied := false
	if applySideEffects {
		autoBanJustApplied = s.applyFlaggedAccountSideEffects(ctx, cfg, log)
		s.sendFlaggedNotificationSideEffects(ctx, cfg, log, autoBanJustApplied)
	}
	if s.repo != nil {
		if err := s.repo.CreateLog(ctx, log); err != nil {
			s.logger.Warn("content_moderation.create_log_failed", "user_id", contentModerationEmailUserID(log), "endpoint", log.Endpoint, "action", log.Action, "error", err)
			return
		}
	}
}

func (s *ModerationService) applyFlaggedAccountSideEffects(ctx context.Context, cfg *ContentModerationConfig, log *ContentModerationLog) bool {
	if s == nil || cfg == nil || log == nil || !log.Flagged || log.UserID == nil || *log.UserID <= 0 {
		return false
	}
	count := 1
	if s.repo != nil && cfg.ViolationWindowHours > 0 {
		since := time.Now().Add(-time.Duration(cfg.ViolationWindowHours) * time.Hour)
		if n, err := s.repo.CountFlaggedByUserSince(ctx, *log.UserID, since); err == nil {
			count = n + 1
		}
	}
	log.ViolationCount = count
	autoBanJustApplied := false
	if cfg.AutoBanEnabled && cfg.BanThreshold > 0 && count >= cfg.BanThreshold && s.host != nil {
		if err := s.host.SetUserDisabled(ctx, *log.UserID, true, "content_moderation_auto_ban"); err != nil {
			if !errors.Is(err, pluginsdk.ErrHostUserDisableUnavailable) {
				s.logger.Warn("content_moderation.ban_update_user_failed", "user_id", *log.UserID, "error", err)
			}
			return false
		}
		autoBanJustApplied = true
		log.AutoBanned = true
	}
	return autoBanJustApplied
}

// sendFlaggedNotificationSideEffects logs that a notification would be sent.
// The core's EmailService is not available to the plugin; until a host email
// extension exists the email body is built and logged for observability but
// not dispatched.
func (s *ModerationService) sendFlaggedNotificationSideEffects(_ context.Context, cfg *ContentModerationConfig, log *ContentModerationLog, autoBanJustApplied bool) {
	if s == nil || cfg == nil || log == nil || !log.Flagged {
		return
	}
	if strings.TrimSpace(log.UserEmail) == "" {
		return
	}
	if cfg.EmailOnHit {
		s.logEmailIntent("violation", log, cfg)
	}
	if autoBanJustApplied {
		s.logEmailIntent("account_disabled", log, cfg)
	}
}

func (s *ModerationService) logEmailIntent(kind string, log *ContentModerationLog, cfg *ContentModerationConfig) {
	var body string
	switch kind {
	case "account_disabled":
		body = buildContentModerationAccountDisabledEmailBody("Sub2API", log, cfg)
	default:
		body = buildContentModerationViolationEmailBody("Sub2API", log, cfg)
	}
	s.logger.Info("content_moderation.email_skipped",
		"kind", kind,
		"user_id", contentModerationEmailUserID(log),
		"recipient", log.UserEmail,
		"body_len", len(body))
}

func contentModerationEmailUserID(log *ContentModerationLog) int64 {
	if log == nil || log.UserID == nil {
		return 0
	}
	return *log.UserID
}
