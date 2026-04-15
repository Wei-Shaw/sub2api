package service

import "strings"

type OpenAIRoutingSnapshot struct {
	TargetGroup         string
	SelectedGroup       string
	ScheduleLayer       string
	Sticky              *openAIStickyEval
	SelectedAccountID   *int64
	SelectedAccountName *string
	RequestedModel      string
	EffectiveModel      string
	FailoverCount       int
	FailoverFinalReason string
}

type OpenAIRoutingSnapshotInput struct {
	TargetGroup    AccountTargetGroup
	SelectedGroup  string
	ScheduleLayer  string
	Sticky         *openAIStickyEval
	Account        *Account
	RequestedModel string
	EffectiveModel string
}

func NewOpenAIRoutingSnapshot(input OpenAIRoutingSnapshotInput) *OpenAIRoutingSnapshot {
	snapshot := &OpenAIRoutingSnapshot{
		TargetGroup:    string(normalizeTargetGroup(input.TargetGroup)),
		SelectedGroup:  strings.TrimSpace(input.SelectedGroup),
		ScheduleLayer:  strings.TrimSpace(input.ScheduleLayer),
		Sticky:         cloneOpenAIStickyEval(input.Sticky),
		RequestedModel: strings.TrimSpace(input.RequestedModel),
		EffectiveModel: strings.TrimSpace(input.EffectiveModel),
	}
	if input.Account != nil {
		accountID := input.Account.ID
		accountName := input.Account.Name
		snapshot.SelectedAccountID = &accountID
		snapshot.SelectedAccountName = &accountName
	}
	return snapshot
}

func (s *OpenAIRoutingSnapshot) RecordFailover(reason string) {
	if s == nil {
		return
	}
	s.FailoverCount++
	s.FailoverFinalReason = strings.TrimSpace(reason)
}
