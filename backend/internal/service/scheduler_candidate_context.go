package service

import "context"

type schedulerCandidateExclusionsKey struct{}
type schedulerCandidatePredicateKey struct{}
type schedulerCandidateBatchPredicateKey struct{}
type schedulerCandidatePriorityIDsKey struct{}

func withSchedulerCandidateExclusions(ctx context.Context, excludedIDs map[int64]struct{}) context.Context {
	if ctx == nil || len(excludedIDs) == 0 {
		return ctx
	}
	copyIDs := make(map[int64]struct{}, len(excludedIDs))
	for id := range excludedIDs {
		if id > 0 {
			copyIDs[id] = struct{}{}
		}
	}
	if len(copyIDs) == 0 {
		return ctx
	}
	return context.WithValue(ctx, schedulerCandidateExclusionsKey{}, copyIDs)
}

func schedulerCandidateExcluded(ctx context.Context, accountID int64) bool {
	if ctx == nil || accountID <= 0 {
		return false
	}
	excluded, ok := ctx.Value(schedulerCandidateExclusionsKey{}).(map[int64]struct{})
	if !ok {
		return false
	}
	_, exists := excluded[accountID]
	return exists
}

func withSchedulerCandidatePredicate(ctx context.Context, predicate func(*Account) bool) context.Context {
	if ctx == nil || predicate == nil {
		return ctx
	}
	if previous, ok := ctx.Value(schedulerCandidatePredicateKey{}).(func(*Account) bool); ok && previous != nil {
		current := predicate
		predicate = func(account *Account) bool {
			return previous(account) && current(account)
		}
	}
	return context.WithValue(ctx, schedulerCandidatePredicateKey{}, predicate)
}

func schedulerCandidateMatchesRequest(ctx context.Context, account *Account) bool {
	if ctx == nil || account == nil {
		return account != nil
	}
	predicate, ok := ctx.Value(schedulerCandidatePredicateKey{}).(func(*Account) bool)
	return !ok || predicate == nil || predicate(account)
}

type schedulerCandidateBatchPredicate func([]*Account) []bool

func withSchedulerCandidateBatchPredicate(ctx context.Context, predicate schedulerCandidateBatchPredicate) context.Context {
	if ctx == nil || predicate == nil {
		return ctx
	}
	if previous, ok := ctx.Value(schedulerCandidateBatchPredicateKey{}).(schedulerCandidateBatchPredicate); ok && previous != nil {
		current := predicate
		predicate = func(accounts []*Account) []bool {
			left := previous(accounts)
			right := current(accounts)
			result := make([]bool, len(accounts))
			if len(left) != len(accounts) || len(right) != len(accounts) {
				return result
			}
			for i := range accounts {
				result[i] = left[i] && right[i]
			}
			return result
		}
	}
	return context.WithValue(ctx, schedulerCandidateBatchPredicateKey{}, predicate)
}

func schedulerCandidateBatchMatches(ctx context.Context, accounts []*Account) []bool {
	result := make([]bool, len(accounts))
	if ctx == nil {
		for i := range result {
			result[i] = true
		}
		return result
	}
	predicate, ok := ctx.Value(schedulerCandidateBatchPredicateKey{}).(schedulerCandidateBatchPredicate)
	if !ok || predicate == nil {
		for i := range result {
			result[i] = true
		}
		return result
	}
	matches := predicate(accounts)
	if len(matches) != len(accounts) {
		return result
	}
	return matches
}

func withSchedulerCandidatePriorityIDs(ctx context.Context, ids []int64) context.Context {
	if ctx == nil || len(ids) == 0 {
		return ctx
	}
	unique := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return ctx
	}
	return context.WithValue(ctx, schedulerCandidatePriorityIDsKey{}, unique)
}

func schedulerCandidatePriorityIDs(ctx context.Context) []int64 {
	if ctx == nil {
		return nil
	}
	ids, _ := ctx.Value(schedulerCandidatePriorityIDsKey{}).([]int64)
	return ids
}
