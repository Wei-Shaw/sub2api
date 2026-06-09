package domain

import (
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Email broadcast body formats.
const (
	EmailBroadcastBodyFormatHTML = "html"
	EmailBroadcastBodyFormatText = "text"
)

// Email broadcast recipient modes.
const (
	EmailBroadcastRecipientsModeAll      = "all"
	EmailBroadcastRecipientsModeSelected = "selected"
)

// Email broadcast lifecycle statuses.
const (
	EmailBroadcastStatusPending   = "pending"
	EmailBroadcastStatusSending   = "sending"
	EmailBroadcastStatusCompleted = "completed"
	EmailBroadcastStatusFailed    = "failed"
)

// EmailBroadcastSubjectMaxLen 邮件主题最大字符数 (与 schema 约束保持一致)。
const EmailBroadcastSubjectMaxLen = 200

// EmailBroadcastBodyMaxLen 邮件正文最大字符数 (上限保护，防止 DoS)。
const EmailBroadcastBodyMaxLen = 65536

// EmailBroadcastMaxSelectedRecipients selected 模式下单次最大收件人数量。
const EmailBroadcastMaxSelectedRecipients = 5000

var (
	ErrEmailBroadcastNotFound          = infraerrors.NotFound("EMAIL_BROADCAST_NOT_FOUND", "email broadcast not found")
	ErrEmailBroadcastSubjectRequired   = infraerrors.BadRequest("EMAIL_BROADCAST_SUBJECT_REQUIRED", "email broadcast subject is required")
	ErrEmailBroadcastBodyRequired      = infraerrors.BadRequest("EMAIL_BROADCAST_BODY_REQUIRED", "email broadcast body is required")
	ErrEmailBroadcastSubjectTooLong    = infraerrors.BadRequest("EMAIL_BROADCAST_SUBJECT_TOO_LONG", "email broadcast subject exceeds maximum length")
	ErrEmailBroadcastBodyTooLong       = infraerrors.BadRequest("EMAIL_BROADCAST_BODY_TOO_LONG", "email broadcast body exceeds maximum length")
	ErrEmailBroadcastInvalidBodyFormat = infraerrors.BadRequest("EMAIL_BROADCAST_INVALID_BODY_FORMAT", "email broadcast body format must be html or text")
	ErrEmailBroadcastInvalidMode       = infraerrors.BadRequest("EMAIL_BROADCAST_INVALID_MODE", "email broadcast recipients mode must be all or selected")
	ErrEmailBroadcastNoRecipients      = infraerrors.BadRequest("EMAIL_BROADCAST_NO_RECIPIENTS", "email broadcast requires at least one recipient")
	ErrEmailBroadcastTooManyRecipients = infraerrors.BadRequest("EMAIL_BROADCAST_TOO_MANY_RECIPIENTS", "email broadcast recipient count exceeds maximum")
	ErrEmailBroadcastEmailNotConfigured = infraerrors.BadRequest("EMAIL_BROADCAST_EMAIL_NOT_CONFIGURED", "SMTP/email service is not configured")
	ErrEmailBroadcastDeleteInFlight     = infraerrors.Conflict("EMAIL_BROADCAST_DELETE_IN_FLIGHT", "cannot delete a broadcast that is still pending or sending")
)
