package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// GroupsStatusHandler serves the anonymous, aggregate-only group status page.
// It deliberately exposes no account identifiers or exclusive-group metadata.
type GroupsStatusHandler struct {
	groupService *service.GroupService
}

func NewGroupsStatusHandler(groupService *service.GroupService) *GroupsStatusHandler {
	return &GroupsStatusHandler{groupService: groupService}
}

type groupsStatusGroup struct {
	ID                      int64   `json:"id"`
	Name                    string  `json:"name"`
	Description             string  `json:"description"`
	Platform                string  `json:"platform"`
	SubscriptionType        string  `json:"subscription_type"`
	RateMultiplier          float64 `json:"rate_multiplier"`
	PeakRateEnabled         bool    `json:"peak_rate_enabled"`
	PeakStart               string  `json:"peak_start"`
	PeakEnd                 string  `json:"peak_end"`
	PeakRateMultiplier      float64 `json:"peak_rate_multiplier"`
	AccountCount            int64   `json:"account_count"`
	AvailableAccountCount   int64   `json:"available_account_count"`
	RateLimitedAccountCount int64   `json:"rate_limited_account_count"`
	Status                  string  `json:"status"`
	Availability            string  `json:"availability"`
	Available               bool    `json:"available"`
}

type groupsStatusSummary struct {
	GroupCount              int   `json:"group_count"`
	AvailableGroupCount     int   `json:"available_group_count"`
	AccountCount            int64 `json:"account_count"`
	AvailableAccountCount   int64 `json:"available_account_count"`
	RateLimitedAccountCount int64 `json:"rate_limited_account_count"`
}

type groupsStatusResponse struct {
	Groups  []groupsStatusGroup `json:"groups"`
	Summary groupsStatusSummary `json:"summary"`
}

const (
	groupAvailabilityAvailable   = "available"
	groupAvailabilityDegraded    = "degraded"
	groupAvailabilityRateLimited = "rate_limited"
	groupAvailabilityUnavailable = "unavailable"
)

// Get returns all groups that are public to every user. Exclusive groups are
// filtered again here as a defensive boundary even though the service already
// asks the repository for is_exclusive=false.
// GET /api/v1/groups-status
func (h *GroupsStatusHandler) Get(c *gin.Context) {
	groups, err := h.groupService.ListPublic(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]groupsStatusGroup, 0, len(groups))
	summary := groupsStatusSummary{}
	for i := range groups {
		if groups[i].IsExclusive {
			continue
		}
		item := toGroupsStatusGroup(&groups[i])
		out = append(out, item)
		summary.GroupCount++
		if item.Available {
			summary.AvailableGroupCount++
		}
		summary.AccountCount += item.AccountCount
		summary.AvailableAccountCount += item.AvailableAccountCount
		summary.RateLimitedAccountCount += item.RateLimitedAccountCount
	}

	response.Success(c, groupsStatusResponse{Groups: out, Summary: summary})
}

func toGroupsStatusGroup(group *service.Group) groupsStatusGroup {
	availability := resolveGroupAvailability(group)
	return groupsStatusGroup{
		ID:                      group.ID,
		Name:                    group.Name,
		Description:             group.Description,
		Platform:                group.Platform,
		SubscriptionType:        group.SubscriptionType,
		RateMultiplier:          group.RateMultiplier,
		PeakRateEnabled:         group.PeakRateEnabled,
		PeakStart:               group.PeakStart,
		PeakEnd:                 group.PeakEnd,
		PeakRateMultiplier:      group.PeakRateMultiplier,
		AccountCount:            group.AccountCount,
		AvailableAccountCount:   group.ActiveAccountCount,
		RateLimitedAccountCount: group.RateLimitedAccountCount,
		Status:                  group.Status,
		Availability:            availability,
		Available: availability == groupAvailabilityAvailable ||
			availability == groupAvailabilityDegraded,
	}
}

func resolveGroupAvailability(group *service.Group) string {
	if group == nil || group.Status != service.StatusActive {
		return groupAvailabilityUnavailable
	}
	if group.ActiveAccountCount > 0 {
		if group.RateLimitedAccountCount > 0 {
			return groupAvailabilityDegraded
		}
		return groupAvailabilityAvailable
	}
	if group.RateLimitedAccountCount > 0 {
		return groupAvailabilityRateLimited
	}
	return groupAvailabilityUnavailable
}
