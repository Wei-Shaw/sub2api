package service

import "github.com/Wei-Shaw/sub2api/internal/domain"

// ChannelMonitorRequestTemplate sub-domain types live in domain; re-exported
// here so existing service call sites compile unchanged.
type (
	ChannelMonitorRequestTemplate             = domain.ChannelMonitorRequestTemplate
	ChannelMonitorRequestTemplateListParams   = domain.ChannelMonitorRequestTemplateListParams
	ChannelMonitorRequestTemplateCreateParams = domain.ChannelMonitorRequestTemplateCreateParams
	ChannelMonitorRequestTemplateUpdateParams = domain.ChannelMonitorRequestTemplateUpdateParams
	AssociatedMonitorBrief                    = domain.AssociatedMonitorBrief
)

// Template errors re-exported from domain.
var (
	ErrChannelMonitorTemplateNotFound          = domain.ErrChannelMonitorTemplateNotFound
	ErrChannelMonitorTemplateInvalidProvider   = domain.ErrChannelMonitorTemplateInvalidProvider
	ErrChannelMonitorTemplateInvalidAPIMode    = domain.ErrChannelMonitorTemplateInvalidAPIMode
	ErrChannelMonitorTemplateMissingName       = domain.ErrChannelMonitorTemplateMissingName
	ErrChannelMonitorTemplateInvalidBodyMode   = domain.ErrChannelMonitorTemplateInvalidBodyMode
	ErrChannelMonitorTemplateBodyRequired      = domain.ErrChannelMonitorTemplateBodyRequired
	ErrChannelMonitorTemplateHeaderForbidden   = domain.ErrChannelMonitorTemplateHeaderForbidden
	ErrChannelMonitorTemplateHeaderInvalidName = domain.ErrChannelMonitorTemplateHeaderInvalidName
	ErrChannelMonitorTemplateProviderMismatch  = domain.ErrChannelMonitorTemplateProviderMismatch
	ErrChannelMonitorTemplateAPIModeMismatch   = domain.ErrChannelMonitorTemplateAPIModeMismatch
	ErrChannelMonitorTemplateApplyEmpty        = domain.ErrChannelMonitorTemplateApplyEmpty
)
