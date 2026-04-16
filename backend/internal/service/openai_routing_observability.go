package service

import (
	"strings"

	"github.com/gin-gonic/gin"
)

const openAIRoutingAffinityBindingKey = "openai_routing_affinity_binding"

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
	selectedGroup := normalizeOpenAISelectedGroup(input.SelectedGroup)
	if selectedGroup == "" {
		switch normalizeTargetGroup(input.TargetGroup) {
		case TargetGroupActive:
			selectedGroup = string(TargetGroupActive)
		case TargetGroupExhausted:
			selectedGroup = string(TargetGroupExhausted)
		}
	}

	snapshot := &OpenAIRoutingSnapshot{
		TargetGroup:    string(normalizeTargetGroup(input.TargetGroup)),
		SelectedGroup:  selectedGroup,
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

func buildOpenAIRoutingAffinityBinding(snapshot *OpenAIRoutingSnapshot) *openAIAffinityBinding {
	if snapshot == nil || snapshot.SelectedAccountID == nil {
		return nil
	}
	return newOpenAIAffinityBinding(*snapshot.SelectedAccountID, snapshot.SelectedGroup)
}

func BindOpenAIRoutingAffinityBinding(c *gin.Context, snapshot *OpenAIRoutingSnapshot) {
	if c == nil {
		return
	}
	c.Set(openAIRoutingAffinityBindingKey, buildOpenAIRoutingAffinityBinding(snapshot))
}

func GetOpenAIRoutingAffinityBinding(c *gin.Context) *openAIAffinityBinding {
	if c == nil {
		return nil
	}
	value, ok := c.Get(openAIRoutingAffinityBindingKey)
	if !ok {
		return nil
	}
	binding, _ := value.(*openAIAffinityBinding)
	return cloneOpenAIAffinityBinding(binding)
}
