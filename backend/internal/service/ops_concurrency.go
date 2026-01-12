package service

import (
	"context"
	"log"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	opsAccountsPageSize          = 100
	opsConcurrencyBatchChunkSize = 200
)

func (s *OpsService) listAllAccountsForOps(ctx context.Context, platformFilter string) ([]Account, error) {
	if s == nil || s.accountRepo == nil {
		return []Account{REDACTED, nil
REDACTED

	out := make([]Account, 0, 128)
	page := 1
	for {
		accounts, pageInfo, err := s.accountRepo.ListWithFilters(ctx, pagination.PaginationParams{
			Page:     page,
			PageSize: opsAccountsPageSize,
	REDACTED, platformFilter, "", "", "")
		if err != nil {
			return nil, err
	REDACTED
		if len(accounts) == 0 {
			break
	REDACTED

		out = append(out, accounts...)
		if pageInfo != nil && int64(len(out)) >= pageInfo.Total {
			break
	REDACTED
		if len(accounts) < opsAccountsPageSize {
			break
	REDACTED

		page++
		if page > 10_000 {
			log.Printf("[Ops] listAllAccountsForOps: aborting after too many pages (platform=%q)", platformFilter)
			break
	REDACTED
REDACTED

	return out, nil
REDACTED

func (s *OpsService) getAccountsLoadMapBestEffort(ctx context.Context, accounts []Account) map[int64]*AccountLoadInfo {
	if s == nil || s.concurrencyService == nil {
		return map[int64]*AccountLoadInfo{REDACTED
REDACTED
	if len(accounts) == 0 {
		return map[int64]*AccountLoadInfo{REDACTED
REDACTED

	// De-duplicate IDs (and keep the max concurrency to avoid under-reporting).
	unique := make(map[int64]int, len(accounts))
	for _, acc := range accounts {
		if acc.ID <= 0 {
			continue
	REDACTED
		if prev, ok := unique[acc.ID]; !ok || acc.Concurrency > prev {
			unique[acc.ID] = acc.Concurrency
	REDACTED
REDACTED

	batch := make([]AccountWithConcurrency, 0, len(unique))
	for id, maxConc := range unique {
		batch = append(batch, AccountWithConcurrency{
			ID:             id,
			MaxConcurrency: maxConc,
	REDACTED)
REDACTED

	out := make(map[int64]*AccountLoadInfo, len(batch))
	for i := 0; i < len(batch); i += opsConcurrencyBatchChunkSize {
		end := i + opsConcurrencyBatchChunkSize
		if end > len(batch) {
			end = len(batch)
	REDACTED
		part, err := s.concurrencyService.GetAccountsLoadBatch(ctx, batch[i:end])
		if err != nil {
			// Best-effort: return zeros rather than failing the ops UI.
			log.Printf("[Ops] GetAccountsLoadBatch failed: %v", err)
			continue
	REDACTED
		for k, v := range part {
			out[k] = v
	REDACTED
REDACTED

	return out
REDACTED

// GetConcurrencyStats returns real-time concurrency usage aggregated by platform/group/account.
//
// Optional filters:
// - platformFilter: only include accounts in that platform (best-effort reduces DB load)
// - groupIDFilter: only include accounts that belong to that group
func (s *OpsService) GetConcurrencyStats(
	ctx context.Context,
	platformFilter string,
	groupIDFilter *int64,
) (map[string]*PlatformConcurrencyInfo, map[int64]*GroupConcurrencyInfo, map[int64]*AccountConcurrencyInfo, *time.Time, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, nil, nil, nil, err
REDACTED

	accounts, err := s.listAllAccountsForOps(ctx, platformFilter)
	if err != nil {
		return nil, nil, nil, nil, err
REDACTED

	collectedAt := time.Now()
	loadMap := s.getAccountsLoadMapBestEffort(ctx, accounts)

	platform := make(map[string]*PlatformConcurrencyInfo)
	group := make(map[int64]*GroupConcurrencyInfo)
	account := make(map[int64]*AccountConcurrencyInfo)

	for _, acc := range accounts {
		if acc.ID <= 0 {
			continue
	REDACTED

		var matchedGroup *Group
		if groupIDFilter != nil && *groupIDFilter > 0 {
			for _, grp := range acc.Groups {
				if grp == nil || grp.ID <= 0 {
					continue
			REDACTED
				if grp.ID == *groupIDFilter {
					matchedGroup = grp
					break
			REDACTED
		REDACTED
			// Group filter provided: skip accounts not in that group.
			if matchedGroup == nil {
				continue
		REDACTED
	REDACTED

		load := loadMap[acc.ID]
		currentInUse := int64(0)
		waiting := int64(0)
		if load != nil {
			currentInUse = int64(load.CurrentConcurrency)
			waiting = int64(load.WaitingCount)
	REDACTED

		// Account-level view picks one display group (the first group).
		displayGroupID := int64(0)
		displayGroupName := ""
		if matchedGroup != nil {
			displayGroupID = matchedGroup.ID
			displayGroupName = matchedGroup.Name
	REDACTED else if len(acc.Groups) > 0 && acc.Groups[0] != nil {
			displayGroupID = acc.Groups[0].ID
			displayGroupName = acc.Groups[0].Name
	REDACTED

		if _, ok := account[acc.ID]; !ok {
			info := &AccountConcurrencyInfo{
				AccountID:      acc.ID,
				AccountName:    acc.Name,
				Platform:       acc.Platform,
				GroupID:        displayGroupID,
				GroupName:      displayGroupName,
				CurrentInUse:   currentInUse,
				MaxCapacity:    int64(acc.Concurrency),
				WaitingInQueue: waiting,
		REDACTED
			if info.MaxCapacity > 0 {
				info.LoadPercentage = float64(info.CurrentInUse) / float64(info.MaxCapacity) * 100
		REDACTED
			account[acc.ID] = info
	REDACTED

		// Platform aggregation.
		if acc.Platform != "" {
			if _, ok := platform[acc.Platform]; !ok {
				platform[acc.Platform] = &PlatformConcurrencyInfo{
					Platform: acc.Platform,
			REDACTED
		REDACTED
			p := platform[acc.Platform]
			p.MaxCapacity += int64(acc.Concurrency)
			p.CurrentInUse += currentInUse
			p.WaitingInQueue += waiting
	REDACTED

		// Group aggregation (one account may contribute to multiple groups).
		if matchedGroup != nil {
			grp := matchedGroup
			if _, ok := group[grp.ID]; !ok {
				group[grp.ID] = &GroupConcurrencyInfo{
					GroupID:   grp.ID,
					GroupName: grp.Name,
					Platform:  grp.Platform,
			REDACTED
		REDACTED
			g := group[grp.ID]
			if g.GroupName == "" && grp.Name != "" {
				g.GroupName = grp.Name
		REDACTED
			if g.Platform != "" && grp.Platform != "" && g.Platform != grp.Platform {
				// Groups are expected to be platform-scoped. If mismatch is observed, avoid misleading labels.
				g.Platform = ""
		REDACTED
			g.MaxCapacity += int64(acc.Concurrency)
			g.CurrentInUse += currentInUse
			g.WaitingInQueue += waiting
	REDACTED else {
			for _, grp := range acc.Groups {
				if grp == nil || grp.ID <= 0 {
					continue
			REDACTED
				if _, ok := group[grp.ID]; !ok {
					group[grp.ID] = &GroupConcurrencyInfo{
						GroupID:   grp.ID,
						GroupName: grp.Name,
						Platform:  grp.Platform,
				REDACTED
			REDACTED
				g := group[grp.ID]
				if g.GroupName == "" && grp.Name != "" {
					g.GroupName = grp.Name
			REDACTED
				if g.Platform != "" && grp.Platform != "" && g.Platform != grp.Platform {
					// Groups are expected to be platform-scoped. If mismatch is observed, avoid misleading labels.
					g.Platform = ""
			REDACTED
				g.MaxCapacity += int64(acc.Concurrency)
				g.CurrentInUse += currentInUse
				g.WaitingInQueue += waiting
		REDACTED
	REDACTED
REDACTED

	for _, info := range platform {
		if info.MaxCapacity > 0 {
			info.LoadPercentage = float64(info.CurrentInUse) / float64(info.MaxCapacity) * 100
	REDACTED
REDACTED
	for _, info := range group {
		if info.MaxCapacity > 0 {
			info.LoadPercentage = float64(info.CurrentInUse) / float64(info.MaxCapacity) * 100
	REDACTED
REDACTED

	return platform, group, account, &collectedAt, nil
REDACTED
