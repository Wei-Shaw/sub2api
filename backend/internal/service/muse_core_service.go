package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/muse"
	"github.com/shopspring/decimal"
)

type MuseCoreService struct {
	accounts AccountRepository
	runtime  *MuseRuntimeService
	store    MuseProviderStore
	gateway  *OpenAIGatewayService
	keys     *APIKeyService
	provider muse.Provider
	mu       sync.RWMutex
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func NewMuseCoreService(accounts AccountRepository, runtimeStore muse.RuntimeStore, store MuseProviderStore, gateway *OpenAIGatewayService) *MuseCoreService {
	s := &MuseCoreService{accounts: accounts, runtime: NewMuseRuntimeService(runtimeStore), store: store, gateway: gateway, provider: muse.DisabledProvider{}}
	if gateway.httpUpstream != nil {
		s.provider = muse.NewNativeProvider(func(req *http.Request, session muse.Session) (*http.Response, error) {
			req = req.WithContext(WithHTTPUpstreamRedirectsDisabled(req.Context()))
			return gateway.httpUpstream.Do(req, session.ProxyURL, session.AccountID, 1)
		})
	}
	gateway.muse = s
	return s
}

func (s *MuseCoreService) SetProvider(p muse.Provider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p == nil {
		p = muse.DisabledProvider{}
	}
	s.provider = p
}
func (s *MuseCoreService) Provider() muse.Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.provider
}
func (s *MuseCoreService) Qualified() bool { return s != nil && s.Provider().Qualified() }

func (s *MuseCoreService) session(a *Account) (muse.Session, error) {
	if a == nil || !a.IsMuse() || !a.IsActive() {
		return muse.Session{}, muse.ErrOwner
	}
	if err := ValidateMuseAccount(a.Platform, a.Type, a.Credentials, a.Extra); err != nil {
		return muse.Session{}, err
	}
	proxy := ""
	if a.ProxyID != nil {
		if a.Proxy == nil {
			return muse.Session{}, muse.ErrGeneration
		}
		proxy = a.Proxy.URL()
	}
	document, ok := a.Credentials["muse_session"].(map[string]any)
	if !ok {
		return muse.Session{}, muse.ErrInvalid
	}
	return muse.Session{AccountID: a.ID, OwnerUserID: MuseOwnerUserID(a.Extra), AccountUpdatedAt: a.UpdatedAt, Document: document, ProxyURL: proxy}, nil
}

func (s *MuseCoreService) Verify(ctx context.Context, id int64) (*muse.Observation, error) {
	if !s.Qualified() {
		return nil, muse.ErrTransportUnqualified
	}
	a, err := s.accounts.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	session, err := s.session(a)
	if err != nil {
		return nil, err
	}
	observed, err := s.Provider().Verify(ctx, session)
	if err != nil {
		return nil, err
	}
	if observed == nil || !observed.InferenceAllowed || len(observed.Capabilities.Models) == 0 || (observed.SessionExpiresAt != nil && !time.Now().Before(*observed.SessionExpiresAt)) {
		return nil, muse.ErrCapability
	}
	if a.Proxy != nil {
		at := a.Proxy.UpdatedAt
		observed.ProxyUpdatedAt = &at
	}
	observed.Identity.AccountID = a.ID
	observed.Identity.AccountUpdatedAt = a.UpdatedAt
	observed.Identity.OwnerUserID = session.OwnerUserID
	if err = observed.Identity.Validate(); err != nil {
		return nil, err
	}
	if _, err = s.runtime.BindVerifiedWorkspace(ctx, observed.Identity); err != nil {
		return nil, err
	}
	if err = s.store.SaveProfile(ctx, a, observed); err != nil {
		return nil, err
	}
	if err = s.store.EnableVerified(ctx, a); err != nil {
		return nil, err
	}
	return observed, nil
}

func (s *MuseCoreService) RenewSession(ctx context.Context, id int64) error {
	if !s.Qualified() {
		_, err := s.CheckSession(ctx, id)
		return err
	}
	a, err := s.accounts.GetByID(ctx, id)
	if err != nil {
		return err
	}
	session, err := s.session(a)
	if err != nil {
		return err
	}
	provider := s.Provider()
	if err = s.store.RenewSession(ctx, a, func(probe context.Context) (map[string]any, error) { return provider.Renew(probe, session) }); err != nil {
		return err
	}
	_, err = s.Verify(ctx, id)
	return err
}

func (s *MuseCoreService) Status(ctx context.Context, id int64) (map[string]any, error) {
	a, err := s.accounts.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !a.IsMuse() {
		return nil, muse.ErrOwner
	}
	cookieAuthSupported := false
	if session, e := s.session(a); e == nil {
		_, e = muse.ParseCookieSession(session.Document)
		cookieAuthSupported = e == nil && s.gateway.httpUpstream != nil
	}
	result := map[string]any{"cookie_auth_supported": cookieAuthSupported, "qualified_transport": s.Qualified(), "owner_user_id": MuseOwnerUserID(a.Extra), "verified": false, "state": "verification_required", "models": []string{}}
	result["pending_turns"], err = s.store.PendingTurns(ctx, id)
	if err != nil {
		return nil, err
	}
	if !s.Qualified() {
		result["state"] = "transport_unqualified"
		return result, nil
	}
	profile, err := s.verifiedProfile(ctx, a)
	if errors.Is(err, muse.ErrTransportUnqualified) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	if !profile.InferenceAllowed {
		return result, nil
	}
	if profile.SessionExpiresAt != nil && !time.Now().Before(*profile.SessionExpiresAt) {
		result["state"] = "session_expired"
		return result, nil
	}
	result["session_expires_at"] = profile.SessionExpiresAt
	result["pending_turns"], err = s.store.PendingTurns(ctx, id)
	if err != nil {
		return nil, err
	}
	result["verified"] = true
	result["state"] = "ready"
	result["capabilities"] = profile.Capabilities
	result["models"] = profile.Capabilities.Models
	result["usage"] = profile.Usage
	return result, nil
}

func (s *MuseCoreService) Models(ctx context.Context, groupID *int64, actor muse.Actor) ([]string, error) {
	if !s.Qualified() {
		return []string{}, nil
	}
	var accounts []Account
	var err error
	if groupID != nil {
		accounts, err = s.accounts.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, PlatformMuse)
	} else {
		accounts, err = s.accounts.ListSchedulableUngroupedByPlatform(ctx, PlatformMuse)
	}
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	models := []string{}
	for i := range accounts {
		a := &accounts[i]
		if MuseOwnerUserID(a.Extra) != actor.UserID {
			continue
		}
		p, err := s.verifiedProfile(ctx, a)
		if err != nil {
			continue
		}
		if !p.InferenceAllowed || p.Identity.OwnerUserID != actor.UserID {
			continue
		}
		if p.SessionExpiresAt != nil && !time.Now().Before(*p.SessionExpiresAt) {
			continue
		}
		for _, m := range p.Capabilities.Models {
			if !seen[m] {
				models = append(models, m)
				seen[m] = true
			}
		}
	}
	sort.Strings(models)
	return models, nil
}

func (s *MuseCoreService) Execute(ctx context.Context, key *APIKey, account *Account, input *apicompat.ResponsesRequest, subscription *UserSubscription, emit func(muse.Event) error) (*muse.Result, *muse.Turn, error) {
	if !s.Qualified() {
		return nil, nil, muse.ErrTransportUnqualified
	}
	if key == nil || key.User == nil || key.Group == nil {
		return nil, nil, muse.ErrOwner
	}
	if input == nil {
		return nil, nil, muse.ErrInvalid
	}
	if account == nil || key.UserID != key.User.ID || key.ID <= 0 {
		return nil, nil, muse.ErrOwner
	}
	provider := s.Provider()
	a, err := s.accounts.GetByID(ctx, account.ID)
	if err != nil {
		return nil, nil, err
	}
	if MuseOwnerUserID(a.Extra) != key.UserID {
		return nil, nil, muse.ErrOwner
	}
	profile, err := s.verifiedProfile(ctx, a)
	if err != nil {
		return nil, nil, err
	}
	if !profile.InferenceAllowed {
		return nil, nil, muse.ErrTransportUnqualified
	}
	if profile.SessionExpiresAt != nil && !time.Now().Before(*profile.SessionExpiresAt) {
		return nil, nil, muse.ErrTransportUnqualified
	}
	canonical := *input
	canonical.Model = a.GetMappedModel(input.Model)
	if err = profile.Capabilities.Validate(&canonical); err != nil {
		return nil, nil, err
	}
	if native, ok := provider.(muse.RequestValidator); ok {
		if err = native.ValidateRequest(&canonical); err != nil {
			return nil, nil, err
		}
	}
	session, err := s.session(a)
	if err != nil {
		return nil, nil, err
	}
	w, err := s.runtime.BindVerifiedWorkspace(ctx, profile.Identity)
	if err != nil {
		return nil, nil, err
	}
	actor := muse.Actor{UserID: key.UserID, APIKeyID: key.ID}
	parent := ""
	if input.PreviousResponseID != "" {
		id := museTurnFromResponseID(input.PreviousResponseID)
		old, err := s.runtime.GetTurn(ctx, id, actor)
		if err != nil {
			return nil, nil, err
		}
		if old.WorkspaceID != w.ID || old.Generation != w.Generation || old.State != muse.Completed {
			return nil, nil, muse.ErrGeneration
		}
		parent = old.ProviderTurnID
	}
	pricing, cost, err := s.price(ctx, key, input.Model)
	if err != nil {
		return nil, nil, err
	}
	turn, lease, err := s.runtime.Reserve(ctx, muse.Reservation{WorkspaceID: w.ID, Generation: w.Generation, Actor: actor, AccountID: a.ID, AccountUpdatedAt: a.UpdatedAt, ProxyUpdatedAt: profile.ProxyUpdatedAt, LeaseOwner: "gateway", LeaseDuration: time.Minute, Pricing: pricing})
	if err != nil {
		return nil, nil, err
	}
	ratio, _ := decimal.NewFromString(pricing.Multiplier)
	rate, _ := ratio.Float64()
	log := &UsageLog{UserID: key.UserID, APIKeyID: key.ID, AccountID: a.ID, RequestID: turn.ID, Model: input.Model, GroupID: key.GroupID, InputTokens: 0, OutputTokens: 0, TotalCost: cost.TotalCost, ActualCost: cost.ActualCost, RateMultiplier: rate, BillingType: BillingTypeBalance, Stream: input.Stream}
	log.RequestedModel = input.Model
	log.UpstreamModel = &canonical.Model
	mode := "per_request"
	log.BillingMode = &mode
	isSub := key.Group.IsSubscriptionType() && pricing.Mode != "test_free"
	if isSub && (subscription == nil || subscription.UserID != key.UserID || subscription.GroupID != key.Group.ID) {
		_ = s.runtime.Advance(ctx, lease, muse.Reserved, muse.Rejected, "")
		return nil, turn, muse.ErrInvalid
	}
	if isSub {
		log.SubscriptionID = &subscription.ID
		log.BillingType = BillingTypeSubscription
	}
	cmd := buildUsageBillingCommand(turn.ID, log, &postUsageBillingParams{Cost: cost, User: key.User, APIKey: key, Account: a, Subscription: subscription, IsSubscriptionBill: isSub, Platform: PlatformMuse, AccountRateMultiplier: 1})
	if cost.ActualCost > 0 && key.Quota > 0 {
		cmd.APIKeyQuotaCost = cost.ActualCost
	}
	if cost.ActualCost > 0 && key.HasRateLimits() {
		cmd.APIKeyRateLimitCost = cost.ActualCost
	}
	cmd.RequestFingerprint = ""
	cmd.Normalize()
	if err = s.store.FreezeCharge(ctx, turn.ID, cmd, log); err != nil {
		_ = s.runtime.Advance(ctx, lease, muse.Reserved, muse.Rejected, "")
		return nil, turn, err
	}
	providerRequest := muse.Request{OperationID: turn.ID, ProviderParentID: parent, Workspace: w, Session: session, Input: &canonical}
	probeReference := ""
	if native, ok := provider.(muse.SubmissionProvider); ok {
		probeReference, err = native.SubmissionID(providerRequest)
		if err != nil {
			_ = s.runtime.Advance(ctx, lease, muse.Reserved, muse.Rejected, "")
			_, _ = s.settle(ctx, turn.ID)
			return nil, turn, err
		}
	}
	if err = s.runtime.BeginSubmission(ctx, lease, probeReference); err != nil {
		_ = s.runtime.Advance(ctx, lease, muse.Reserved, muse.Rejected, "")
		_, _ = s.settle(ctx, turn.ID)
		return nil, turn, err
	}

	state := muse.Submitting
	providerID := ""
	lastSeq := int64(0)
	seen := map[int64][32]byte{}
	var eventMu sync.Mutex
	var terminalEvent *muse.Event
	causalFailure := false
	callbacksClosed := false
	operation, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	stopLease := make(chan struct{})
	defer close(stopLease)
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if e := s.runtime.Renew(operation, lease, time.Minute); e != nil {
					cancel()
					return
				}
			case <-ctx.Done():
				cancel()
				return
			case <-operation.Done():
				return
			case <-stopLease:
				return
			}
		}
	}()
	result, executeErr := provider.Execute(operation, providerRequest, func(event muse.Event) (eventErr error) {
		eventMu.Lock()
		defer eventMu.Unlock()
		defer func() {
			if errors.Is(eventErr, muse.ErrGeneration) || errors.Is(eventErr, muse.ErrInvalid) {
				causalFailure = true
			}
		}()
		if callbacksClosed {
			return muse.ErrTransition
		}
		if event.OperationID != turn.ID || event.ProviderTurnID == "" || (probeReference != "" && event.ProviderTurnID != probeReference) || (providerID != "" && event.ProviderTurnID != providerID) {
			return muse.ErrGeneration
		}
		encoded, e := json.Marshal(event)
		if e != nil {
			return e
		}
		hash := sha256.Sum256(encoded)
		if event.Sequence <= lastSeq {
			if old, ok := seen[event.Sequence]; !ok || old != hash {
				return muse.ErrGeneration
			}
			return nil
		}
		if event.Sequence != lastSeq+1 || event.Sequence > 100000 || terminalEvent != nil {
			return muse.ErrGeneration
		}
		lastSeq = event.Sequence
		seen[event.Sequence] = hash
		providerID = event.ProviderTurnID
		if state == muse.Submitting {
			if e := s.runtime.Advance(operation, lease, state, muse.Accepted, providerID); e != nil {
				return e
			}
			state = muse.Accepted
		}
		if event.Data.Type == "response.output_text.delta" && state == muse.Accepted {
			if e := s.runtime.Advance(operation, lease, state, muse.Running, providerID); e != nil {
				return e
			}
			state = muse.Running
		}
		if event.Data.Type == "response.completed" || event.Data.Type == "response.failed" || event.Data.Type == "response.incomplete" {
			if event.Data.Response == nil {
				return muse.ErrInvalid
			}
			if event.Data.Type == "response.completed" && event.Data.Response.Status != "completed" {
				return muse.ErrInvalid
			}
			terminalEvent = &event
			return nil
		}
		if emit != nil {
			return emit(event)
		}
		return nil
	})
	// Provider implementations must finish callbacks before returning. Keep the
	// mutex held while finalizing to prevent accidental concurrent mutation.
	eventMu.Lock()
	defer eventMu.Unlock()
	callbacksClosed = true
	cleanup, done := context.WithTimeout(context.Background(), 15*time.Second)
	defer done()
	terminal := muse.State("")
	if !causalFailure && result != nil && result.ProviderTurnID != "" && result.Response != nil && (probeReference == "" || probeReference == result.ProviderTurnID) && (providerID == "" || providerID == result.ProviderTurnID) {
		switch result.Response.Status {
		case "completed":
			terminal = muse.Completed
		case "failed", "incomplete":
			terminal = muse.Failed
		}
	}
	if terminal != "" {
		providerID = result.ProviderTurnID
		if state == muse.Submitting {
			if err = s.runtime.Advance(cleanup, lease, state, muse.Accepted, providerID); err != nil {
				return nil, turn, err
			}
			state = muse.Accepted
		}
		if err = s.store.RecordResult(cleanup, turn.ID, result); err != nil {
			_ = s.runtime.Advance(cleanup, lease, state, muse.Ambiguous, providerID)
			return nil, turn, err
		}
		if err = s.runtime.Advance(cleanup, lease, state, terminal, providerID); err != nil {
			return nil, turn, err
		}
		turn.State = terminal
		turn.ProviderTurnID = providerID
		if _, e := s.settle(cleanup, turn.ID); e != nil {
			// Remote completion remains authoritative. A bounded worker retries only
			// local settlement; reporting this as an upstream failure risks replay.
			slog.Error("Muse settlement pending", "turn_id", turn.ID)
		}
		if terminal == muse.Failed {
			return result, turn, muse.ErrRemoteFailed
		}
		if input.Stream && emit != nil {
			if terminalEvent == nil {
				terminalEvent = &muse.Event{OperationID: turn.ID, Sequence: lastSeq + 1, ProviderTurnID: providerID, Data: apicompat.ResponsesStreamEvent{Type: "response.completed", Response: result.Response}}
			}
			terminalEvent.Data.Response = result.Response
			if e := emit(*terminalEvent); e != nil {
				return result, turn, e
			}
		}
		return result, turn, nil
	}
	if errors.Is(executeErr, muse.ErrRejected) && state == muse.Submitting && providerID == "" {
		if e := s.runtime.Advance(cleanup, lease, state, muse.Rejected, ""); e == nil {
			turn.State = muse.Rejected
			_, _ = s.settle(cleanup, turn.ID)
		}
		return nil, turn, executeErr
	}
	if executeErr == nil {
		executeErr = muse.ErrInvalid
	}
	if ctx.Err() != nil && providerID != "" {
		confirmed, _ := provider.Cancel(cleanup, session, providerID)
		if confirmed {
			if err = s.runtime.Advance(cleanup, lease, state, muse.CancelPending, providerID); err == nil {
				state = muse.CancelPending
				if err = s.runtime.Advance(cleanup, lease, state, muse.Cancelled, providerID); err == nil {
					turn.State = muse.Cancelled
					_, _ = s.settle(cleanup, turn.ID)
					return nil, turn, ctx.Err()
				}
			}
		}
	}
	if err = s.runtime.Advance(cleanup, lease, state, muse.Ambiguous, providerID); err == nil {
		turn.State = muse.Ambiguous
		turn.ProviderTurnID = providerID
		if turn.ProviderTurnID == "" {
			turn.ProviderTurnID = probeReference
		}
	}
	return nil, turn, executeErr
}

func (s *MuseCoreService) price(ctx context.Context, key *APIKey, model string) (muse.Pricing, *CostBreakdown, error) {
	if s.gateway.cfg != nil && s.gateway.cfg.RunMode == config.RunModeSimple {
		return muse.Pricing{Mode: "test_free", UnitPrice: "0", Multiplier: "1"}, &CostBreakdown{}, nil
	}
	if s.gateway.resolver == nil {
		return muse.Pricing{}, nil, muse.ErrInvalid
	}
	r := s.gateway.resolver.Resolve(ctx, PricingInput{Model: model, GroupID: key.GroupID, Group: key.Group})
	if r == nil || r.Mode != BillingModePerRequest || r.DefaultPerRequestPrice <= 0 || len(r.RequestTiers) > 0 {
		return muse.Pricing{}, nil, fmt.Errorf("explicit flat Muse per-request pricing required")
	}
	mult := s.gateway.ResolveUserGroupRateMultiplier(ctx, key.UserID, key.Group.ID, key.Group.RateMultiplier)
	if math.IsNaN(r.DefaultPerRequestPrice) || math.IsInf(r.DefaultPerRequestPrice, 0) || math.IsNaN(mult) || math.IsInf(mult, 0) {
		return muse.Pricing{}, nil, muse.ErrInvalid
	}
	unit := decimal.NewFromFloat(r.DefaultPerRequestPrice).Round(8)
	ratio := decimal.NewFromFloat(mult).Round(8)
	p := muse.Pricing{Mode: "flat_request", UnitPrice: unit.String(), Multiplier: ratio.String()}
	if err := p.Validate(); err != nil {
		return p, nil, err
	}
	actualDecimal := unit.Mul(ratio).Round(8)
	// Usage-log money columns are NUMERIC(20,10), narrower than the ledger.
	if unit.GreaterThanOrEqual(decimal.New(1, 10)) || actualDecimal.GreaterThanOrEqual(decimal.New(1, 10)) {
		return muse.Pricing{}, nil, muse.ErrInvalid
	}
	actual, _ := actualDecimal.Float64()
	total, _ := unit.Float64()
	return p, &CostBreakdown{TotalCost: total, ActualCost: actual}, nil
}

func museResponseID(id string) string { return "resp_muse_" + strings.TrimPrefix(id, "muse_turn_") }
func museTurnFromResponseID(id string) string {
	if !strings.HasPrefix(id, "resp_muse_") {
		return ""
	}
	return "muse_turn_" + strings.TrimPrefix(id, "resp_muse_")
}

func (s *MuseCoreService) Start() {
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				budget, budgetCancel := context.WithTimeout(ctx, 45*time.Second)
				s.Recover(budget)
				s.RenewDue(budget)
				budgetCancel()
			case <-ctx.Done():
				return
			}
		}
	}()
}
func (s *MuseCoreService) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
}

func (s *MuseCoreService) Recover(ctx context.Context) {
	// Local settlement is safe even when remote transport is disabled.
	pending, e := s.store.PendingSettlement(ctx, 25)
	if e == nil {
		for _, id := range pending {
			if ctx.Err() != nil {
				return
			}
			probe, cancel := context.WithTimeout(ctx, 10*time.Second)
			_, _ = s.settle(probe, id)
			cancel()
		}
	}
	ids, err := s.store.PendingRecovery(ctx, 25)
	if err != nil {
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		turn, lease, err := s.runtime.ClaimRecovery(ctx, id, "recovery", time.Minute)
		if err != nil {
			continue
		}
		if turn.State == muse.Reserved {
			_ = s.runtime.Advance(ctx, lease, muse.Reserved, muse.Rejected, "")
			_, _ = s.settle(ctx, turn.ID)
			continue
		}
		if !s.Qualified() {
			_ = s.runtime.Advance(ctx, lease, muse.Ambiguous, muse.OwnerReview, "")
			continue
		}
		a, err := s.accounts.GetByID(ctx, turn.AccountID)
		if err != nil {
			_ = s.runtime.Advance(ctx, lease, muse.Ambiguous, muse.OwnerReview, "")
			continue
		}
		session, err := s.session(a)
		profile, profileErr := s.verifiedProfile(ctx, a)
		if err != nil || profileErr != nil || profile == nil || turn.ProviderTurnID == "" {
			_ = s.runtime.Advance(ctx, lease, muse.Ambiguous, muse.OwnerReview, "")
			continue
		}
		probe, cancel := context.WithTimeout(ctx, 15*time.Second)
		result, err := s.Provider().Reconcile(probe, session, turn.ProviderTurnID)
		cancel()
		if err != nil || result == nil || result.Response == nil {
			_ = s.runtime.Advance(ctx, lease, muse.Ambiguous, muse.OwnerReview, "")
			continue
		}
		if result.ProviderTurnID != turn.ProviderTurnID {
			_ = s.runtime.Advance(ctx, lease, muse.Ambiguous, muse.OwnerReview, "")
			continue
		}
		outcome := muse.State("")
		switch result.Response.Status {
		case "completed":
			outcome = muse.Completed
		case "failed", "incomplete":
			outcome = muse.Failed
		}
		if outcome != "" {
			if s.store.RecordResult(ctx, turn.ID, result) == nil && s.runtime.Advance(ctx, lease, muse.Ambiguous, outcome, turn.ProviderTurnID) == nil {
				_, _ = s.settle(ctx, turn.ID)
			}
		} else {
			_ = s.runtime.Advance(ctx, lease, muse.Ambiguous, muse.OwnerReview, "")
		}
	}
}

func (s *MuseCoreService) AdminTurn(ctx context.Context, id string) (*muse.Turn, error) {
	return s.store.AdminTurn(ctx, id)
}
func (s *MuseCoreService) Resolve(ctx context.Context, id string, state muse.State) error {
	t, err := s.store.AdminTurn(ctx, id)
	if err != nil {
		return err
	}
	if err = s.store.ResolveOwnerReview(ctx, id, t.Actor, state); err != nil {
		return err
	}
	_, err = s.settle(ctx, id)
	return err
}

func (s *MuseCoreService) settle(ctx context.Context, id string) (*MuseSettlement, error) {
	settlement, err := s.store.Settle(ctx, id)
	if err != nil {
		_ = s.store.DeferSettlement(ctx, id)
		return settlement, err
	}
	if settlement == nil {
		return nil, nil
	}
	cmd := settlement.Command
	if cache := s.gateway.billingCacheService; cache != nil {
		_ = cache.InvalidateUserBalance(ctx, cmd.UserID)
		if settlement.Log.GroupID != nil {
			_ = cache.InvalidateSubscription(ctx, cmd.UserID, *settlement.Log.GroupID)
		}
		_ = cache.InvalidateAPIKeyRateLimit(ctx, cmd.APIKeyID)
	}
	if s.keys != nil {
		key, e := s.keys.GetByID(ctx, cmd.APIKeyID)
		if e == nil && key != nil {
			s.keys.InvalidateAuthCacheByKey(ctx, key.Key)
		}
	}
	return settlement, nil
}

func (s *MuseCoreService) RenewDue(ctx context.Context) {
	if !s.Qualified() {
		return
	}
	ids, err := s.store.DueRenewal(ctx, 5)
	if err != nil {
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		a, e := s.accounts.GetByID(ctx, id)
		if e != nil {
			continue
		}
		p, e := s.verifiedProfile(ctx, a)
		if e != nil || p == nil || p.SessionExpiresAt == nil || p.SessionExpiresAt.After(time.Now().Add(5*time.Minute)) {
			continue
		}
		if e = s.RenewSession(ctx, id); e != nil {
			_ = s.store.DeferRenewal(ctx, id)
		}
	}
}

func (s *MuseCoreService) RetrySettlement(ctx context.Context, id string) error {
	_, err := s.store.AdminTurn(ctx, id)
	if err != nil {
		return err
	}
	_, err = s.settle(ctx, id)
	return err
}

// The observation and credentials must describe the same loaded snapshot. The
// runtime persists this version and checks it again before external submission.
func (s *MuseCoreService) verifiedProfile(ctx context.Context, a *Account) (*muse.Observation, error) {
	p, err := s.store.Profile(ctx, a)
	if err != nil {
		return nil, err
	}
	if p == nil || p.Identity.AccountID != a.ID || p.Identity.OwnerUserID != MuseOwnerUserID(a.Extra) || !p.Identity.AccountUpdatedAt.Equal(a.UpdatedAt) {
		return nil, muse.ErrGeneration
	}
	if a.ProxyID == nil {
		if p.ProxyUpdatedAt != nil {
			return nil, muse.ErrGeneration
		}
	} else if a.Proxy == nil || p.ProxyUpdatedAt == nil || !p.ProxyUpdatedAt.Equal(a.Proxy.UpdatedAt) {
		return nil, muse.ErrGeneration
	}
	return p, nil
}

// CheckSession ports the reference projects' app-cookie bootstrap/renewal path.
// Authentication success does not qualify inference or fabricate a model catalog,
// subscription allowance, subject identifier, or canonical workspace identity.
func (s *MuseCoreService) CheckSession(ctx context.Context, id int64) (*muse.SessionCheck, error) {
	a, err := s.accounts.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	session, err := s.session(a)
	if err != nil {
		return nil, err
	}
	if _, err = muse.ParseCookieSession(session.Document); err != nil {
		return nil, err
	}
	if s.gateway.httpUpstream == nil {
		return nil, muse.ErrTransportUnqualified
	}
	client := &muse.SessionClient{Do: func(req *http.Request, session muse.Session) (*http.Response, error) {
		req = req.WithContext(WithHTTPUpstreamRedirectsDisabled(req.Context()))
		return s.gateway.httpUpstream.Do(req, session.ProxyURL, session.AccountID, 1)
	}}
	var refreshed *muse.SessionRefresh
	err = s.store.RenewSession(ctx, a, func(probe context.Context) (map[string]any, error) {
		var e error
		refreshed, e = client.Refresh(probe, session)
		if e != nil {
			return nil, e
		}
		return refreshed.Document, nil
	})
	if err != nil {
		return nil, err
	}
	return &refreshed.Check, nil
}
