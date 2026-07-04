package service

import (
	"context"
	"time"
)

// GroupCapacitySummary holds aggregated capacity for a single group.
type GroupCapacitySummary struct {
	GroupID         int64 `json:"group_id"`
	ConcurrencyUsed int   `json:"concurrency_used"`
	ConcurrencyMax  int   `json:"concurrency_max"`
	SessionsUsed    int   `json:"sessions_used"`
	SessionsMax     int   `json:"sessions_max"`
	RPMUsed         int   `json:"rpm_used"`
	RPMMax          int   `json:"rpm_max"`
REDACTED

// GroupAccountCapacityRow is the lightweight account projection needed for
// capacity summary aggregation.
type GroupAccountCapacityRow struct {
	GroupID             int64
	AccountID           int64
	Concurrency         int
	Extra               map[string]any
	SessionWindowStart  *time.Time
	SessionWindowEnd    *time.Time
	SessionWindowStatus string
REDACTED

type groupCapacityActiveGroupIDLister interface {
	ListActiveIDs(ctx context.Context) ([]int64, error)
REDACTED

type groupCapacityAccountLister interface {
	ListSchedulableCapacityByGroupIDs(ctx context.Context, groupIDs []int64) ([]GroupAccountCapacityRow, error)
REDACTED

// GroupCapacityService aggregates per-group capacity from runtime data.
type GroupCapacityService struct {
	accountRepo        AccountRepository
	groupRepo          GroupRepository
	concurrencyService *ConcurrencyService
	sessionLimitCache  SessionLimitCache
	rpmCache           RPMCache
REDACTED

// NewGroupCapacityService creates a new GroupCapacityService.
func NewGroupCapacityService(
	accountRepo AccountRepository,
	groupRepo GroupRepository,
	concurrencyService *ConcurrencyService,
	sessionLimitCache SessionLimitCache,
	rpmCache RPMCache,
) *GroupCapacityService {
	return &GroupCapacityService{
		accountRepo:        accountRepo,
		groupRepo:          groupRepo,
		concurrencyService: concurrencyService,
		sessionLimitCache:  sessionLimitCache,
		rpmCache:           rpmCache,
REDACTED
REDACTED

// GetAllGroupCapacity returns capacity summary for all active groups.
func (s *GroupCapacityService) GetAllGroupCapacity(ctx context.Context) ([]GroupCapacitySummary, error) {
	groupIDs, err := s.listActiveGroupIDs(ctx)
	if err != nil {
		return nil, err
REDACTED

	if lister, ok := s.accountRepo.(groupCapacityAccountLister); ok {
		return s.getGroupCapacitiesBatch(ctx, groupIDs, lister)
REDACTED

	return s.getGroupCapacitiesSequential(ctx, groupIDs), nil
REDACTED

func (s *GroupCapacityService) listActiveGroupIDs(ctx context.Context) ([]int64, error) {
	if lister, ok := s.groupRepo.(groupCapacityActiveGroupIDLister); ok {
		return lister.ListActiveIDs(ctx)
REDACTED

	groups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, err
REDACTED
	groupIDs := make([]int64, 0, len(groups))
	for i := range groups {
		groupIDs = append(groupIDs, groups[i].ID)
REDACTED
	return groupIDs, nil
REDACTED

func (s *GroupCapacityService) getGroupCapacitiesSequential(ctx context.Context, groupIDs []int64) []GroupCapacitySummary {
	results := make([]GroupCapacitySummary, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		cap, err := s.getGroupCapacity(ctx, groupID)
		if err != nil {
			// Skip groups with errors, return partial results
			continue
	REDACTED
		cap.GroupID = groupID
		results = append(results, cap)
REDACTED
	return results
REDACTED

type groupCapacityAccountRef struct {
	groupID   int64
	accountID int64
REDACTED

func (s *GroupCapacityService) getGroupCapacitiesBatch(ctx context.Context, groupIDs []int64, lister groupCapacityAccountLister) ([]GroupCapacitySummary, error) {
	results := make([]GroupCapacitySummary, len(groupIDs))
	groupIndex := make(map[int64]int, len(groupIDs))
	for i, groupID := range groupIDs {
		results[i].GroupID = groupID
		groupIndex[groupID] = i
REDACTED
	if len(groupIDs) == 0 {
		return results, nil
REDACTED

	rows, err := lister.ListSchedulableCapacityByGroupIDs(ctx, groupIDs)
	if err != nil {
		return nil, err
REDACTED
	if len(rows) == 0 {
		return results, nil
REDACTED

	refs := make([]groupCapacityAccountRef, 0, len(rows))
	seenGroupAccount := make(map[groupCapacityAccountRef]struct{REDACTED, len(rows))
	accountIDSet := make(map[int64]struct{REDACTED, len(rows))
	accountIDs := make([]int64, 0, len(rows))
	sessionTimeouts := make(map[int64]time.Duration)

	for _, row := range rows {
		idx, ok := groupIndex[row.GroupID]
		if !ok || row.AccountID <= 0 {
			continue
	REDACTED

		ref := groupCapacityAccountRef{groupID: row.GroupID, accountID: row.AccountIDREDACTED
		if _, ok := seenGroupAccount[ref]; ok {
			continue
	REDACTED
		seenGroupAccount[ref] = struct{REDACTED{REDACTED
		refs = append(refs, ref)

		if _, ok := accountIDSet[row.AccountID]; !ok {
			accountIDSet[row.AccountID] = struct{REDACTED{REDACTED
			accountIDs = append(accountIDs, row.AccountID)
	REDACTED

		acc := Account{
			ID:                  row.AccountID,
			Concurrency:         row.Concurrency,
			Extra:               row.Extra,
			SessionWindowStart:  row.SessionWindowStart,
			SessionWindowEnd:    row.SessionWindowEnd,
			SessionWindowStatus: row.SessionWindowStatus,
	REDACTED

		results[idx].ConcurrencyMax += acc.Concurrency

		if maxSessions := acc.GetMaxSessions(); maxSessions > 0 {
			results[idx].SessionsMax += maxSessions
			timeout := time.Duration(acc.GetSessionIdleTimeoutMinutes()) * time.Minute
			if timeout <= 0 {
				timeout = 5 * time.Minute
		REDACTED
			sessionTimeouts[acc.ID] = timeout
	REDACTED

		if rpm := acc.GetBaseRPM(); rpm > 0 {
			results[idx].RPMMax += rpm
	REDACTED
REDACTED

	if len(accountIDs) == 0 {
		return results, nil
REDACTED

	concurrencyMap := map[int64]int{REDACTED
	if s.concurrencyService != nil {
		concurrencyMap, _ = s.concurrencyService.GetAccountConcurrencyBatch(ctx, accountIDs)
REDACTED

	sessionAccountIDs := accountIDsForGroupsWithLimit(refs, groupIndex, results, func(summary GroupCapacitySummary) bool {
		return summary.SessionsMax > 0
REDACTED)
	var sessionsMap map[int64]int
	if len(sessionAccountIDs) > 0 && s.sessionLimitCache != nil {
		sessionsMap, _ = s.sessionLimitCache.GetActiveSessionCountBatch(ctx, sessionAccountIDs, sessionTimeouts)
REDACTED

	rpmAccountIDs := accountIDsForGroupsWithLimit(refs, groupIndex, results, func(summary GroupCapacitySummary) bool {
		return summary.RPMMax > 0
REDACTED)
	var rpmMap map[int64]int
	if len(rpmAccountIDs) > 0 && s.rpmCache != nil {
		rpmMap, _ = s.rpmCache.GetRPMBatch(ctx, rpmAccountIDs)
REDACTED

	for _, ref := range refs {
		idx := groupIndex[ref.groupID]
		results[idx].ConcurrencyUsed += concurrencyMap[ref.accountID]
		if sessionsMap != nil && results[idx].SessionsMax > 0 {
			results[idx].SessionsUsed += sessionsMap[ref.accountID]
	REDACTED
		if rpmMap != nil && results[idx].RPMMax > 0 {
			results[idx].RPMUsed += rpmMap[ref.accountID]
	REDACTED
REDACTED
	return results, nil
REDACTED

func accountIDsForGroupsWithLimit(refs []groupCapacityAccountRef, groupIndex map[int64]int, summaries []GroupCapacitySummary, include func(GroupCapacitySummary) bool) []int64 {
	seen := make(map[int64]struct{REDACTED)
	accountIDs := make([]int64, 0)
	for _, ref := range refs {
		idx, ok := groupIndex[ref.groupID]
		if !ok || !include(summaries[idx]) {
			continue
	REDACTED
		if _, ok := seen[ref.accountID]; ok {
			continue
	REDACTED
		seen[ref.accountID] = struct{REDACTED{REDACTED
		accountIDs = append(accountIDs, ref.accountID)
REDACTED
	return accountIDs
REDACTED

func (s *GroupCapacityService) getGroupCapacity(ctx context.Context, groupID int64) (GroupCapacitySummary, error) {
	accounts, err := s.accountRepo.ListSchedulableByGroupID(ctx, groupID)
	if err != nil {
		return GroupCapacitySummary{REDACTED, err
REDACTED
	if len(accounts) == 0 {
		return GroupCapacitySummary{REDACTED, nil
REDACTED

	// Collect account IDs and config values
	accountIDs := make([]int64, 0, len(accounts))
	sessionTimeouts := make(map[int64]time.Duration)
	var concurrencyMax, sessionsMax, rpmMax int

	for i := range accounts {
		acc := &accounts[i]
		accountIDs = append(accountIDs, acc.ID)
		concurrencyMax += acc.Concurrency

		if ms := acc.GetMaxSessions(); ms > 0 {
			sessionsMax += ms
			timeout := time.Duration(acc.GetSessionIdleTimeoutMinutes()) * time.Minute
			if timeout <= 0 {
				timeout = 5 * time.Minute
		REDACTED
			sessionTimeouts[acc.ID] = timeout
	REDACTED

		if rpm := acc.GetBaseRPM(); rpm > 0 {
			rpmMax += rpm
	REDACTED
REDACTED

	// Batch query runtime data from Redis
	concurrencyMap, _ := s.concurrencyService.GetAccountConcurrencyBatch(ctx, accountIDs)

	var sessionsMap map[int64]int
	if sessionsMax > 0 && s.sessionLimitCache != nil {
		sessionsMap, _ = s.sessionLimitCache.GetActiveSessionCountBatch(ctx, accountIDs, sessionTimeouts)
REDACTED

	var rpmMap map[int64]int
	if rpmMax > 0 && s.rpmCache != nil {
		rpmMap, _ = s.rpmCache.GetRPMBatch(ctx, accountIDs)
REDACTED

	// Aggregate
	var concurrencyUsed, sessionsUsed, rpmUsed int
	for _, id := range accountIDs {
		concurrencyUsed += concurrencyMap[id]
		if sessionsMap != nil {
			sessionsUsed += sessionsMap[id]
	REDACTED
		if rpmMap != nil {
			rpmUsed += rpmMap[id]
	REDACTED
REDACTED

	return GroupCapacitySummary{
		ConcurrencyUsed: concurrencyUsed,
		ConcurrencyMax:  concurrencyMax,
		SessionsUsed:    sessionsUsed,
		SessionsMax:     sessionsMax,
		RPMUsed:         rpmUsed,
		RPMMax:          rpmMax,
REDACTED, nil
REDACTED
