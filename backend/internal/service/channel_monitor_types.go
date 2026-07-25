package service

import "github.com/Wei-Shaw/sub2api/internal/domain"

// ChannelMonitor aggregate + view/history DTOs live in domain; re-exported
// here so existing service call sites compile unchanged. Mirror of the channel
// BC pattern.
type (
	ChannelMonitor             = domain.ChannelMonitor
	ChannelMonitorListParams   = domain.ChannelMonitorListParams
	ChannelMonitorCreateParams = domain.ChannelMonitorCreateParams
	ChannelMonitorUpdateParams = domain.ChannelMonitorUpdateParams
	CheckResult                = domain.CheckResult
	UserMonitorView            = domain.UserMonitorView
	UserMonitorTimelinePoint   = domain.UserMonitorTimelinePoint
	ExtraModelStatus           = domain.ExtraModelStatus
	UserMonitorDetail          = domain.UserMonitorDetail
	ModelDetail                = domain.ModelDetail
	ChannelMonitorHistoryRow   = domain.ChannelMonitorHistoryRow
	ChannelMonitorHistoryEntry = domain.ChannelMonitorHistoryEntry
	ChannelMonitorLatest       = domain.ChannelMonitorLatest
	ChannelMonitorAvailability = domain.ChannelMonitorAvailability
	MonitorStatusSummary       = domain.MonitorStatusSummary
)

// Body-override / api-mode / provider / duplicate-operation-id constants
// re-exported from domain.
const (
	MonitorBodyOverrideModeOff                    = domain.MonitorBodyOverrideModeOff
	MonitorBodyOverrideModeMerge                  = domain.MonitorBodyOverrideModeMerge
	MonitorBodyOverrideModeReplace                = domain.MonitorBodyOverrideModeReplace
	MonitorAPIModeChatCompletions                 = domain.MonitorAPIModeChatCompletions
	MonitorAPIModeResponses                       = domain.MonitorAPIModeResponses
	MonitorProviderOpenAI                         = domain.MonitorProviderOpenAI
	MonitorProviderAnthropic                      = domain.MonitorProviderAnthropic
	MonitorProviderGemini                         = domain.MonitorProviderGemini
	MonitorProviderGrok                           = domain.MonitorProviderGrok
	ChannelMonitorDuplicateOperationIDMetadataKey = domain.ChannelMonitorDuplicateOperationIDMetadataKey
)
