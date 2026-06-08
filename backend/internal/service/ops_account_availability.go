package service

import (
	"context"
	"errors"
	"time"
)

// GetAccountAvailabilityStats returns current account availability stats.
//
// Query-level filtering is intentionally limited to platform/group to match the dashboard scope.
func (s *OpsService) GetAccountAvailabilityStats(ctx context.Context, platformFilter string, groupIDFilter *int64) (
	map[string]*PlatformAvailability,
	map[int64]*GroupAvailability,
	map[int64]*AccountAvailability,
	*time.Time,
	error,
) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, nil, nil, nil, err
	}

	accounts, err := s.listAllAccountsForOps(ctx, platformFilter)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	if groupIDFilter != nil && *groupIDFilter > 0 {
		filtered := make([]Account, 0, len(accounts))
		for _, acc := range accounts {
			for _, grp := range acc.Groups {
				if grp != nil && grp.ID == *groupIDFilter {
					filtered = append(filtered, acc)
					break
				}
			}
		}
		accounts = filtered
	}

	now := time.Now()
	collectedAt := now

	platform := make(map[string]*PlatformAvailability)
	group := make(map[int64]*GroupAvailability)
	account := make(map[int64]*AccountAvailability)

	for _, acc := range accounts {
		if acc.ID <= 0 {
			continue
		}

		isTempUnsched := false
		if acc.TempUnschedulableUntil != nil && now.Before(*acc.TempUnschedulableUntil) {
			isTempUnsched = true
		}

		isRateLimited := acc.RateLimitResetAt != nil && now.Before(*acc.RateLimitResetAt)
		isOverloaded := acc.OverloadUntil != nil && now.Before(*acc.OverloadUntil)
		hasError := acc.Status == StatusError

		// Normalize exclusive status flags so the UI doesn't show conflicting badges.
		if hasError {
			isRateLimited = false
			isOverloaded = false
		}

		isAvailable := acc.Status == StatusActive && acc.Schedulable && !isRateLimited && !isOverloaded && !isTempUnsched

		if acc.Platform != "" {
			if _, ok := platform[acc.Platform]; !ok {
				platform[acc.Platform] = &PlatformAvailability{
					Platform: acc.Platform,
				}
			}
			p := platform[acc.Platform]
			p.TotalAccounts++
			if isAvailable {
				p.AvailableCount++
			}
			if isRateLimited {
				p.RateLimitCount++
			}
			if hasError {
				p.ErrorCount++
			}
		}

		for _, grp := range acc.Groups {
			if grp == nil || grp.ID <= 0 {
				continue
			}
			if _, ok := group[grp.ID]; !ok {
				group[grp.ID] = &GroupAvailability{
					GroupID:   grp.ID,
					GroupName: grp.Name,
					Platform:  grp.Platform,
				}
			}
			g := group[grp.ID]
			g.TotalAccounts++
			if isAvailable {
				g.AvailableCount++
			}
			if isRateLimited {
				g.RateLimitCount++
			}
			if hasError {
				g.ErrorCount++
			}
		}

		displayGroupID := int64(0)
		displayGroupName := ""
		if len(acc.Groups) > 0 && acc.Groups[0] != nil {
			displayGroupID = acc.Groups[0].ID
			displayGroupName = acc.Groups[0].Name
		}

		item := &AccountAvailability{
			AccountID:   acc.ID,
			AccountName: acc.Name,
			Platform:    acc.Platform,
			GroupID:     displayGroupID,
			GroupName:   displayGroupName,
			Status:      acc.Status,

			IsAvailable:   isAvailable,
			IsRateLimited: isRateLimited,
			IsOverloaded:  isOverloaded,
			HasError:      hasError,

			ErrorMessage: acc.ErrorMessage,
		}

		if isRateLimited && acc.RateLimitResetAt != nil {
			item.RateLimitResetAt = acc.RateLimitResetAt
			remainingSec := int64(time.Until(*acc.RateLimitResetAt).Seconds())
			if remainingSec > 0 {
				item.RateLimitRemainingSec = &remainingSec
			}
		}
		if isOverloaded && acc.OverloadUntil != nil {
			item.OverloadUntil = acc.OverloadUntil
			remainingSec := int64(time.Until(*acc.OverloadUntil).Seconds())
			if remainingSec > 0 {
				item.OverloadRemainingSec = &remainingSec
			}
		}
		if isTempUnsched && acc.TempUnschedulableUntil != nil {
			item.TempUnschedulableUntil = acc.TempUnschedulableUntil
		}

		account[acc.ID] = item
	}

	return platform, group, account, &collectedAt, nil
}

type OpsAccountAvailability struct {
	Group       *GroupAvailability
	Accounts    map[int64]*AccountAvailability
	CollectedAt *time.Time
}

func (s *OpsService) GetAccountAvailability(ctx context.Context, platformFilter string, groupIDFilter *int64) (*OpsAccountAvailability, error) {
	if s == nil {
		return nil, errors.New("ops service is nil")
	}

	if s.getAccountAvailability != nil {
		return s.getAccountAvailability(ctx, platformFilter, groupIDFilter)
	}

	_, groupStats, accountStats, collectedAt, err := s.GetAccountAvailabilityStats(ctx, platformFilter, groupIDFilter)
	if err != nil {
		return nil, err
	}

	var group *GroupAvailability
	if groupIDFilter != nil && *groupIDFilter > 0 {
		group = groupStats[*groupIDFilter]
	}

	if accountStats == nil {
		accountStats = map[int64]*AccountAvailability{}
	}

	return &OpsAccountAvailability{
		Group:       group,
		Accounts:    accountStats,
		CollectedAt: collectedAt,
	}, nil
}
