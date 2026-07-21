package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	UpstreamIdentityExtraKey        = "upstream_identity"
	UpstreamIdentityDetectorVersion = 2

	UpstreamIdentityStatusIdentified = "identified"
	UpstreamIdentityStatusFailed     = "failed"

	UpstreamIdentityProviderSub2API = "sub2api"
	UpstreamIdentityProviderNewAPI  = "new_api"

	UpstreamIdentityVariantLegacy = "legacy"
	UpstreamIdentityVariantModern = "modern"

	upstreamIdentityCycleInterval = time.Minute
	upstreamIdentityBatchSize     = 10
	upstreamIdentityAccountDelay  = 2 * time.Second
	upstreamIdentityRetryDelay    = 2 * time.Second
	upstreamIdentityMaxAttempts   = 3
	upstreamIdentityTimeout       = 40 * time.Second
	upstreamIdentityLeaderLockKey = "upstream:identity:detect:leader"
	// Ten minutes covers the bounded worst case for one ten-account batch:
	// per-account timeout/retries plus the spacing between accounts.
	upstreamIdentityLeaderLockTTL = 10 * time.Minute
)

type UpstreamIdentitySnapshot struct {
	DetectorVersion int       `json:"detector_version"`
	Status          string    `json:"status"`
	Provider        string    `json:"provider,omitempty"`
	Variant         string    `json:"variant,omitempty"`
	Version         string    `json:"version,omitempty"`
	SiteLogoKey     string    `json:"site_logo_key,omitempty"`
	DetectedAt      time.Time `json:"detected_at"`
}

type upstreamIdentityPendingAccountLister interface {
	ListPendingUpstreamIdentityAccounts(context.Context, int, int) ([]Account, error)
}

type upstreamIdentitySnapshotWriter interface {
	UpdateUpstreamIdentitySnapshot(context.Context, *Account, *UpstreamIdentitySnapshot) error
}

type upstreamIdentityProbeResult struct {
	provider      string
	variant       string
	version       string
	logoCandidate string
	retryable     bool
}

func decodeUpstreamIdentitySnapshot(extra map[string]any) *UpstreamIdentitySnapshot {
	if extra == nil {
		return nil
	}
	raw, ok := extra[UpstreamIdentityExtraKey]
	if !ok {
		return nil
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var snapshot UpstreamIdentitySnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil || snapshot.DetectorVersion <= 0 || snapshot.DetectedAt.IsZero() {
		return nil
	}
	switch snapshot.Status {
	case UpstreamIdentityStatusFailed:
		return &snapshot
	case UpstreamIdentityStatusIdentified:
		if snapshot.SiteLogoKey != "" && !validUpstreamSiteLogoKey(snapshot.SiteLogoKey) {
			snapshot.SiteLogoKey = ""
		}
		switch snapshot.Provider {
		case UpstreamIdentityProviderSub2API:
			if snapshot.Variant != "" {
				return nil
			}
		case UpstreamIdentityProviderNewAPI:
			if snapshot.Variant != UpstreamIdentityVariantLegacy && snapshot.Variant != UpstreamIdentityVariantModern {
				return nil
			}
		default:
			return nil
		}
		return &snapshot
	default:
		return nil
	}
}

func decodeDisplayableUpstreamIdentity(extra map[string]any) *UpstreamIdentitySnapshot {
	snapshot := decodeUpstreamIdentitySnapshot(extra)
	if snapshot == nil || snapshot.Status != UpstreamIdentityStatusIdentified {
		return nil
	}
	return snapshot
}

// identityFromStoredBillingProbe reuses an already validated Sub2API billing
// snapshot. The snapshot is still written through the account identity CAS, so
// a concurrent credential/proxy/TLS change cannot turn this into a stale mark.
func identityFromStoredBillingProbe(account *Account) *upstreamIdentityProbeResult {
	snapshot := decodeUpstreamBillingProbeSnapshot(account.Extra)
	if snapshot == nil || snapshot.Status != UpstreamBillingProbeStatusOK || snapshot.Data == nil {
		return nil
	}
	data, err := json.Marshal(snapshot.Data)
	if err != nil {
		return nil
	}
	if _, err := parseUpstreamBillingProbeResponse(data); err != nil {
		return nil
	}
	return &upstreamIdentityProbeResult{provider: UpstreamIdentityProviderSub2API}
}

func (s *UpstreamBillingProbeService) canRunUpstreamIdentityDetection() bool {
	if s == nil || s.accountRepo == nil || s.accountTestService == nil || s.accountTestService.httpUpstream == nil {
		return false
	}
	_, canList := s.accountRepo.(upstreamIdentityPendingAccountLister)
	_, canWrite := s.accountRepo.(upstreamIdentitySnapshotWriter)
	return canList && canWrite
}

func (s *UpstreamBillingProbeService) runUpstreamIdentityLoop() {
	defer s.wg.Done()
	if err := s.RunPendingUpstreamIdentityDetection(s.parentCtx); err != nil {
		logger.LegacyPrintf("service.upstream_identity", "initial_run_failed: err=%v", err)
	}
	ticker := time.NewTicker(upstreamIdentityCycleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.parentCtx.Done():
			return
		case <-ticker.C:
			if err := s.RunPendingUpstreamIdentityDetection(s.parentCtx); err != nil {
				logger.LegacyPrintf("service.upstream_identity", "run_failed: err=%v", err)
			}
		}
	}
}

func (s *UpstreamBillingProbeService) RunPendingUpstreamIdentityDetection(ctx context.Context) error {
	if !s.canRunUpstreamIdentityDetection() {
		return nil
	}
	s.identityCycleMu.Lock()
	defer s.identityCycleMu.Unlock()

	release, acquired := tryAcquireSingletonLeaderLock(
		ctx, s.lockCache, s.db, upstreamIdentityLeaderLockKey, s.instanceID, upstreamIdentityLeaderLockTTL,
	)
	if !acquired {
		return nil
	}
	defer release()

	lister, ok := s.accountRepo.(upstreamIdentityPendingAccountLister)
	if !ok {
		return ErrUpstreamBillingProbeUnavailable
	}
	accounts, err := lister.ListPendingUpstreamIdentityAccounts(
		ctx, UpstreamIdentityDetectorVersion, upstreamIdentityBatchSize,
	)
	if err != nil {
		return fmt.Errorf("list pending upstream identities: %w", err)
	}
	for i := range accounts {
		if i > 0 {
			timer := time.NewTimer(upstreamIdentityAccountDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		opCtx, cancel := context.WithTimeout(ctx, upstreamIdentityTimeout)
		err := s.detectAndPersistUpstreamIdentity(opCtx, accounts[i].ID)
		cancel()
		if err != nil && !errors.Is(err, ErrUpstreamQuotaIdentityChanged) {
			logger.LegacyPrintf("service.upstream_identity", "detect_failed: account_id=%d err=%v", accounts[i].ID, err)
		}
	}
	return nil
}

func (s *UpstreamBillingProbeService) detectAndPersistUpstreamIdentity(ctx context.Context, accountID int64) error {
	select {
	case s.probeSlots <- struct{}{}:
		defer func() { <-s.probeSlots }()
	case <-ctx.Done():
		return upstreamQuotaContextError(ctx)
	}

	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return err
	}
	if !isUpstreamBillingProbeAccount(account) {
		return nil
	}
	now := s.currentTime().UTC()
	result := identityFromStoredBillingProbe(account)
	var client *upstreamQuotaQueryClient
	if result == nil {
		client, err = s.newUpstreamQuotaQueryClient(account)
	}
	if result == nil && err == nil {
		for attempt := 0; attempt < upstreamIdentityMaxAttempts; attempt++ {
			attemptCtx, cancel := context.WithTimeout(ctx, upstreamBillingProbeRequestTimeout)
			result, err = client.detectIdentity(attemptCtx)
			cancel()
			if err == nil || result == nil || !result.retryable || attempt == upstreamIdentityMaxAttempts-1 {
				break
			}
			timer := time.NewTimer(upstreamIdentityRetryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				err = upstreamQuotaContextError(ctx)
				attempt = upstreamIdentityMaxAttempts
			case <-timer.C:
			}
		}
	}

	snapshot := &UpstreamIdentitySnapshot{
		DetectorVersion: UpstreamIdentityDetectorVersion,
		Status:          UpstreamIdentityStatusFailed,
		DetectedAt:      now,
	}
	if err == nil && result != nil {
		snapshot.Status = UpstreamIdentityStatusIdentified
		snapshot.Provider = result.provider
		snapshot.Variant = result.variant
		snapshot.Version = result.version
		if client == nil && s.canCacheUpstreamSiteLogos() {
			client, _ = s.newUpstreamQuotaQueryClient(account)
		}
		if client != nil {
			snapshot.SiteLogoKey = s.resolveUpstreamSiteLogo(ctx, client, result)
		}
	}

	current, loadErr := s.accountRepo.GetByID(ctx, accountID)
	if loadErr != nil {
		return loadErr
	}
	if !sameUpstreamProtocolIdentity(account, current) {
		return ErrUpstreamQuotaIdentityChanged
	}
	writer, ok := s.accountRepo.(upstreamIdentitySnapshotWriter)
	if !ok {
		return ErrUpstreamBillingProbeUnavailable
	}
	if writeErr := writer.UpdateUpstreamIdentitySnapshot(ctx, account, snapshot); writeErr != nil {
		return writeErr
	}
	return nil
}

func (c *upstreamQuotaQueryClient) detectIdentity(ctx context.Context) (*upstreamIdentityProbeResult, error) {
	response, err := c.get(ctx, buildOpenAIEndpointURL(c.baseURL, "/v1/usage"), true)
	if err != nil {
		return &upstreamIdentityProbeResult{retryable: isRetryableUpstreamIdentityError(err)}, err
	}
	if response.status != http.StatusNotFound && response.status != http.StatusMethodNotAllowed {
		if retryable, statusErr := upstreamIdentityHTTPError(response.status, false); statusErr != nil {
			return &upstreamIdentityProbeResult{retryable: retryable}, statusErr
		}
		if _, parseErr := parseSub2APIUsage(response.body); parseErr != nil {
			return &upstreamIdentityProbeResult{}, ErrUpstreamQuotaInvalidResponse
		}
		return &upstreamIdentityProbeResult{provider: UpstreamIdentityProviderSub2API}, nil
	}

	subscription, err := c.get(ctx, buildOpenAIEndpointURL(c.baseURL, "/v1/dashboard/billing/subscription"), true)
	if err != nil {
		return &upstreamIdentityProbeResult{retryable: isRetryableUpstreamIdentityError(err)}, err
	}
	if retryable, statusErr := upstreamIdentityHTTPError(subscription.status, true); statusErr != nil {
		return &upstreamIdentityProbeResult{retryable: retryable}, statusErr
	}
	if _, parseErr := parseNewAPISubscription(subscription.body); parseErr != nil {
		return &upstreamIdentityProbeResult{}, ErrUpstreamQuotaInvalidResponse
	}

	statusURL, err := upstreamQuotaStatusURL(c.baseURL)
	if err != nil {
		return &upstreamIdentityProbeResult{}, ErrUpstreamQuotaInvalidResponse
	}
	statusCtx, cancel := context.WithTimeout(ctx, upstreamQuotaStatusTimeout)
	defer cancel()
	statusResponse, err := c.get(statusCtx, statusURL, false)
	if err != nil {
		return &upstreamIdentityProbeResult{retryable: isRetryableUpstreamIdentityError(err)}, err
	}
	if retryable, statusErr := upstreamIdentityHTTPError(statusResponse.status, false); statusErr != nil {
		return &upstreamIdentityProbeResult{retryable: retryable}, statusErr
	}
	metadata, err := parseNewAPIStatusMetadata(statusResponse.body)
	if err != nil {
		return &upstreamIdentityProbeResult{}, ErrUpstreamQuotaInvalidResponse
	}
	variant := ""
	if metadata.QuotaDisplayType != nil {
		variant = UpstreamIdentityVariantModern
	} else if metadata.DisplayInCurrency != nil {
		variant = UpstreamIdentityVariantLegacy
	} else {
		return &upstreamIdentityProbeResult{}, ErrUpstreamQuotaInvalidResponse
	}
	version := strings.TrimSpace(metadata.Version)
	if len(version) > 64 {
		version = version[:64]
	}
	return &upstreamIdentityProbeResult{
		provider:      UpstreamIdentityProviderNewAPI,
		variant:       variant,
		version:       version,
		logoCandidate: metadata.Logo,
	}, nil
}

func upstreamIdentityHTTPError(status int, unsupported bool) (bool, error) {
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		return false, nil
	}
	if status == http.StatusTooManyRequests || status >= http.StatusInternalServerError {
		return true, upstreamQuotaHTTPError(status, unsupported)
	}
	return false, upstreamQuotaHTTPError(status, unsupported)
}

func isRetryableUpstreamIdentityError(err error) bool {
	return errors.Is(err, ErrUpstreamQuotaTimeout) ||
		errors.Is(err, ErrUpstreamQuotaRateLimited) ||
		errors.Is(err, ErrUpstreamQuotaRequestFailed)
}
