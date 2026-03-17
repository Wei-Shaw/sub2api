package service

import "context"

// gatewayAffinityFlow encapsulates affinity-specific scheduling steps so the
// main account selection flow can stay focused on generic scheduling.
type gatewayAffinityFlow struct {
	svc              *GatewayService
	ctx              context.Context
	groupID          *int64
	sessionHash      string
	requestedModel   string
	affinityClientID string
	affinityUserID   int64
	platform         string
	useMixed         bool
	accountByID      map[int64]*Account
	isExcluded       func(int64) bool
}

type affinityWaitCandidate struct {
	account *Account
}

func newGatewayAffinityFlow(
	svc *GatewayService,
	ctx context.Context,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	affinityClientID string,
	affinityUserID int64,
	platform string,
	useMixed bool,
	accountByID map[int64]*Account,
	isExcluded func(int64) bool,
) *gatewayAffinityFlow {
	return &gatewayAffinityFlow{
		svc:              svc,
		ctx:              ctx,
		groupID:          groupID,
		sessionHash:      sessionHash,
		requestedModel:   requestedModel,
		affinityClientID: affinityClientID,
		affinityUserID:   affinityUserID,
		platform:         platform,
		useMixed:         useMixed,
		accountByID:      accountByID,
		isExcluded:       isExcluded,
	}
}

// shouldFilterAccountWithoutClientID excludes affinity-enabled Anthropic accounts
// when metadata.user_id does not provide a usable client_id.
func shouldFilterAccountWithoutClientID(account *Account, affinityClientID string) bool {
	if account == nil || affinityClientID != "" {
		return false
	}
	if account.Platform != PlatformAnthropic {
		return false
	}
	return account.IsAffinityEnabled()
}

func filterAccountsWithoutClientID(accounts []Account, affinityClientID string) []Account {
	if affinityClientID != "" {
		return accounts
	}
	filtered := make([]Account, 0, len(accounts))
	for _, acc := range accounts {
		if shouldFilterAccountWithoutClientID(&acc, affinityClientID) {
			continue
		}
		filtered = append(filtered, acc)
	}
	return filtered
}

func (f *gatewayAffinityFlow) preprocessPinnedUsers(accounts []Account) {
	if f.affinityUserID <= 0 || f.affinityClientID == "" || f.svc.cache == nil {
		return
	}
	for i := range accounts {
		if accounts[i].IsPinnedUser(f.affinityUserID) && accounts[i].IsAffinityEnabled() {
			_ = f.svc.cache.UpdateAffinity(
				f.ctx,
				derefGroupID(f.groupID),
				f.affinityUserID,
				f.affinityClientID,
				accounts[i].ID,
				ClientAffinityTTL,
			)
		}
	}
}

// trySelectAffinityAccount runs Layer 1.4 and returns:
// - result != nil: affinity path selected an account or wait plan
// - affinityHit == true: an effective affinity-enabled record was considered and should suppress sticky fallback
func (f *gatewayAffinityFlow) trySelectAffinityAccount() (*AccountSelectionResult, bool, error) {
	if f.affinityClientID == "" || f.affinityUserID <= 0 || f.svc.cache == nil {
		return nil, false, nil
	}

	gid := derefGroupID(f.groupID)
	affinityAccountIDs, err := f.svc.cache.GetAffinityAccounts(f.ctx, gid, f.affinityUserID, f.affinityClientID, ClientAffinityTTL)
	if err != nil || len(affinityAccountIDs) == 0 {
		return nil, false, nil
	}

	noSwitchBlocked := false
	anyAllowSwitch := false
	effectiveAffinityHit := false
	waitCandidates := make(map[int64]*affinityWaitCandidate)

	for _, affinityAccID := range affinityAccountIDs {
		account, ok := f.accountByID[affinityAccID]

		if f.isExcluded != nil && f.isExcluded(affinityAccID) {
			checkAcc := account
			if !ok && f.svc.accountRepo != nil {
				if acc, repoErr := f.svc.accountRepo.GetByID(f.ctx, affinityAccID); repoErr == nil && acc != nil {
					checkAcc = acc
				}
			}
			if checkAcc != nil && checkAcc.IsAffinityEnabled() {
				effectiveAffinityHit = true
				if !checkAcc.IsAffinityAllowSwitch() {
					noSwitchBlocked = true
				} else {
					anyAllowSwitch = true
				}
			}
			continue
		}

		if !ok || !f.svc.isAccountSchedulableForSelection(account) {
			checkAcc := account
			if !ok && f.svc.accountRepo != nil {
				if acc, repoErr := f.svc.accountRepo.GetByID(f.ctx, affinityAccID); repoErr == nil && acc != nil {
					checkAcc = acc
				}
			}
			if checkAcc != nil && checkAcc.IsAffinityEnabled() {
				effectiveAffinityHit = true
				if !checkAcc.IsAffinityAllowSwitch() {
					noSwitchBlocked = true
				} else {
					anyAllowSwitch = true
				}
			}
			continue
		}

		if !account.IsAffinityEnabled() {
			continue
		}
		effectiveAffinityHit = true
		if account.IsAffinityAllowSwitch() {
			anyAllowSwitch = true
		}

		if !f.svc.isAccountAllowedForPlatform(account, f.platform, f.useMixed) ||
			(f.requestedModel != "" && !f.svc.isModelSupportedByAccountWithContext(f.ctx, account, f.requestedModel)) ||
			!f.svc.isAccountSchedulableForModelSelection(f.ctx, account, f.requestedModel) ||
			!f.svc.isAccountSchedulableForQuota(account) ||
			!f.svc.isAccountSchedulableForWindowCost(f.ctx, account, false) ||
			!f.svc.isAccountSchedulableForRPM(f.ctx, account, false) {
			if !account.IsAffinityAllowSwitch() {
				noSwitchBlocked = true
			}
			continue
		}

		userCount, clientCount, perUserCount, multiErr := f.svc.cache.GetAffinityMultiCount(
			f.ctx, gid, affinityAccID, f.affinityUserID, ClientAffinityTTL,
		)
		if multiErr == nil {
			zone := account.GetMultiDimAffinityZone(userCount, clientCount, perUserCount)
			if zone == AffinityZoneRed {
				if !account.IsAffinityAllowSwitch() {
					noSwitchBlocked = true
				}
				continue
			}
		}

		result, acquireErr := f.svc.tryAcquireAccountSlot(f.ctx, affinityAccID, account.Concurrency)
		if acquireErr == nil && result.Acquired {
			if !f.svc.checkAndRegisterSession(f.ctx, account, f.sessionHash) {
				result.ReleaseFunc()
				continue
			}
			_ = f.svc.cache.UpdateAffinity(f.ctx, gid, f.affinityUserID, f.affinityClientID, affinityAccID, ClientAffinityTTL)
			if f.sessionHash != "" {
				_ = f.svc.cache.SetSessionAccountID(f.ctx, gid, f.sessionHash, affinityAccID, stickySessionTTL)
			}
			return &AccountSelectionResult{
				Account:     account,
				Acquired:    true,
				ReleaseFunc: result.ReleaseFunc,
			}, true, nil
		}
		if acquireErr == nil && !result.Acquired && !account.IsAffinityAllowSwitch() {
			noSwitchBlocked = true
			waitCandidates[affinityAccID] = &affinityWaitCandidate{account: account}
		}
	}

	if noSwitchBlocked && !anyAllowSwitch && f.svc.concurrencyService != nil {
		for _, waitAccID := range affinityAccountIDs {
			candidate, ok := waitCandidates[waitAccID]
			if !ok || candidate == nil || candidate.account == nil {
				continue
			}
			acc := candidate.account
			waitingCount, _ := f.svc.concurrencyService.GetAccountWaitingCount(f.ctx, waitAccID)
			if waitingCount >= f.svc.schedulingConfig().StickySessionMaxWaiting {
				continue
			}
			if !f.svc.checkAndRegisterSession(f.ctx, acc, f.sessionHash) {
				continue
			}
			cfg := f.svc.schedulingConfig()
			return &AccountSelectionResult{
				Account: acc,
				WaitPlan: &AccountWaitPlan{
					AccountID:      waitAccID,
					MaxConcurrency: acc.Concurrency,
					Timeout:        cfg.StickySessionWaitTimeout,
					MaxWaiting:     cfg.StickySessionMaxWaiting,
				},
			}, true, nil
		}
		return nil, effectiveAffinityHit, ErrAffinityNoSwitch
	}

	return nil, effectiveAffinityHit, nil
}
