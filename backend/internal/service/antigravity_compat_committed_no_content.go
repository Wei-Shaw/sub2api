package service

import (
	"context"
	"errors"
	"log/slog"
)

// antigravityCompatCommittedNoContentError reports a compat stream that
// committed HTTP 200 (pre-content keepalive) and then ended or timed out
// without any content. The response can no longer fail over, so the client
// retries; the handler releases the session's sticky account so that retry is
// scheduled afresh instead of returning to the account that just failed.
type antigravityCompatCommittedNoContentError struct {
	msg string
}

func newAntigravityCompatCommittedNoContentError(msg string) error {
	return &antigravityCompatCommittedNoContentError{msg: msg}
}

func (e *antigravityCompatCommittedNoContentError) Error() string { return e.msg }

// IsAntigravityCompatCommittedNoContent reports whether err is a compat stream
// that committed its response and then produced no content.
func IsAntigravityCompatCommittedNoContent(err error) bool {
	var target *antigravityCompatCommittedNoContentError
	return errors.As(err, &target)
}

// ReleaseStickySessionAfterCommittedNoContent removes the session's sticky
// binding when err is a committed no-content compat stream and the session is
// still bound to accountID. Only this session is affected: the account is not
// cooled or excluded for anyone else. Returns true when a binding was removed.
func (s *GatewayService) ReleaseStickySessionAfterCommittedNoContent(ctx context.Context, groupID *int64, sessionHash string, accountID int64, err error) bool {
	if sessionHash == "" || accountID <= 0 || s.cache == nil || !IsAntigravityCompatCommittedNoContent(err) {
		return false
	}
	gid := derefGroupID(groupID)
	bound, getErr := s.cache.GetSessionAccountID(ctx, gid, sessionHash)
	if getErr != nil || bound != accountID {
		return false
	}
	if delErr := s.cache.DeleteSessionAccountID(ctx, gid, sessionHash); delErr != nil {
		slog.Warn("antigravity_compat_sticky_release_failed", "group_id", gid, "account_id", accountID, "error", delErr)
		return false
	}
	slog.Info("antigravity_compat_sticky_released", "group_id", gid, "account_id", accountID, "reason", err.Error())
	return true
}
