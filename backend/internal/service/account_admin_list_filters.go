package service

import (
	"context"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const AccountUIDisplayGroupsExtraKey = "ui_display_groups"

type accountRepoWithAdminFilters interface {
	ListWithAdminFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, schedulable, search string, groupID int64, privacyMode, displayGroup, namePrefix, searchRegex string) ([]Account, *pagination.PaginationResult, error)
}

func listAccountsWithLegacyRepoAndExtraFilters(ctx context.Context, repo AccountRepository, params pagination.PaginationParams, platform, accountType, status, schedulable, search string, groupID int64, privacyMode, displayGroup, namePrefix, searchRegex string) ([]Account, int64, error) {
	fetchPageSize := params.PageSize
	if fetchPageSize < 200 {
		fetchPageSize = 200
	}
	if fetchPageSize <= 0 {
		fetchPageSize = 200
	}

	allAccounts := make([]Account, 0, fetchPageSize)
	for page := 1; ; page++ {
		items, result, err := repo.ListWithFilters(ctx, pagination.PaginationParams{
			Page:      page,
			PageSize:  fetchPageSize,
			SortBy:    params.SortBy,
			SortOrder: params.SortOrder,
		}, platform, accountType, status, search, groupID, privacyMode)
		if err != nil {
			return nil, 0, err
		}
		allAccounts = append(allAccounts, items...)
		if result == nil || int64(len(allAccounts)) >= result.Total || len(items) == 0 {
			break
		}
	}

	filtered, err := filterAccountsByAdminExtraFilters(allAccounts, schedulable, displayGroup, namePrefix, searchRegex)
	if err != nil {
		return nil, 0, err
	}

	total := int64(len(filtered))
	start := params.Offset()
	if start >= len(filtered) {
		return []Account{}, total, nil
	}
	end := start + params.Limit()
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

func filterAccountsByAdminExtraFilters(accounts []Account, schedulable, displayGroup, namePrefix, searchRegex string) ([]Account, error) {
	schedulable = strings.TrimSpace(schedulable)
	displayGroup = strings.TrimSpace(displayGroup)
	namePrefix = strings.TrimSpace(namePrefix)
	matcher, err := compileAccountSearchRegex(searchRegex)
	if err != nil {
		return nil, err
	}
	if schedulable == "" && displayGroup == "" && namePrefix == "" && matcher == nil {
		return accounts, nil
	}

	filtered := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		if !accountMatchesAdminExtraFilters(account, schedulable, displayGroup, namePrefix, matcher) {
			continue
		}
		filtered = append(filtered, account)
	}
	return filtered, nil
}

func accountMatchesAdminExtraFilters(account Account, schedulable, displayGroup, namePrefix string, matcher *regexp.Regexp) bool {
	if schedulable == "true" && !account.Schedulable {
		return false
	}
	if schedulable == "false" && account.Schedulable {
		return false
	}
	if displayGroup != "" && !accountHasUIDisplayGroup(account, displayGroup) {
		return false
	}
	if namePrefix != "" && !strings.HasPrefix(strings.ToLower(account.Name), strings.ToLower(namePrefix)) {
		return false
	}
	if matcher == nil {
		return true
	}

	if matcher.MatchString(account.Name) {
		return true
	}
	if account.Notes != nil && matcher.MatchString(strings.TrimSpace(*account.Notes)) {
		return true
	}
	for _, group := range getAccountUIDisplayGroups(account.Extra) {
		if matcher.MatchString(group) {
			return true
		}
	}
	return false
}

func compileAccountSearchRegex(searchRegex string) (*regexp.Regexp, error) {
	searchRegex = strings.TrimSpace(searchRegex)
	if searchRegex == "" {
		return nil, nil
	}
	return regexp.Compile("(?i:" + searchRegex + ")")
}

func accountHasUIDisplayGroup(account Account, displayGroup string) bool {
	displayGroup = strings.TrimSpace(displayGroup)
	if displayGroup == "" {
		return true
	}
	for _, candidate := range getAccountUIDisplayGroups(account.Extra) {
		if candidate == displayGroup {
			return true
		}
	}
	return false
}

func getAccountUIDisplayGroups(extra map[string]any) []string {
	if len(extra) == 0 {
		return nil
	}
	return normalizeUIDisplayGroupsValue(extra[AccountUIDisplayGroupsExtraKey])
}

func normalizeUIDisplayGroupsValue(raw any) []string {
	switch value := raw.(type) {
	case []string:
		return uniqueNonEmptyStrings(value)
	case []any:
		items := make([]string, 0, len(value))
		for _, entry := range value {
			if text, ok := entry.(string); ok {
				items = append(items, text)
			}
		}
		return uniqueNonEmptyStrings(items)
	case string:
		return uniqueNonEmptyStrings(splitDelimitedText(value))
	default:
		return nil
	}
}

func splitDelimitedText(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case '\n', '\r', '\t', ',', ';', '，', '；', '、':
			return true
		default:
			return false
		}
	})
	return uniqueNonEmptyStrings(parts)
}

func uniqueNonEmptyStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}
