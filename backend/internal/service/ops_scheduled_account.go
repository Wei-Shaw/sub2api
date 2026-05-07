package service

import "strings"

func ResolveOpsScheduledAccount(accountID *int64, accountName string, upstreamErrorsRaw string) (*int64, string) {
	var resolvedID *int64
	if accountID != nil && *accountID > 0 {
		v := *accountID
		resolvedID = &v
	}

	resolvedName := strings.TrimSpace(accountName)
	if resolvedID != nil && resolvedName != "" {
		return resolvedID, resolvedName
	}

	events, err := ParseOpsUpstreamErrors(upstreamErrorsRaw)
	if err != nil {
		return resolvedID, resolvedName
	}

	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev == nil {
			continue
		}
		if resolvedID == nil && ev.AccountID > 0 {
			v := ev.AccountID
			resolvedID = &v
		}
		if resolvedName == "" {
			resolvedName = strings.TrimSpace(ev.AccountName)
		}
		if resolvedID != nil && resolvedName != "" {
			break
		}
	}

	return resolvedID, resolvedName
}
