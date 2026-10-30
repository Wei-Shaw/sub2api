package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const AccountSchedulingPolicyKey = "account_scheduling_pool_policy"

var (
	ErrAccountSchedulingPoolNotFound    = infraerrors.NotFound("ACCOUNT_SCHEDULING_POOL_NOT_FOUND", "account scheduling pool not found")
	ErrAccountSchedulingPoolNotEmpty    = infraerrors.Conflict("ACCOUNT_SCHEDULING_POOL_NOT_EMPTY", "remove the pool members before deleting it")
	ErrAccountSchedulingExitInUse       = infraerrors.Conflict("ACCOUNT_SCHEDULING_EXIT_IN_USE", "wait for all active requests to drain before changing proxy exit groups")
	ErrAccountSchedulingUnavailable     = infraerrors.New(503, "ACCOUNT_SCHEDULING_UNAVAILABLE", "account scheduling capacity is unavailable")
	ErrAccountSchedulingLeaseLost       = errors.New("account scheduling lease lost")
	ErrAccountSchedulingLiveUnsupported = infraerrors.Conflict("ACCOUNT_SCHEDULING_LIVE_UNSUPPORTED", "Live calls for Free or unknown-tier OpenAI accounts are unavailable while account scheduling is enabled; use a known paid account or disable the policy after draining traffic")
)

const (
	AccountSchedulingActive       = "active"
	AccountSchedulingStandby      = "standby"
	AccountSchedulingDraining     = "draining"
	AccountSchedulingCooldown     = "cooldown"
	AccountSchedulingReauth       = "reauth"
	accountSchedulingLeaseTTL     = 90 * time.Second
	accountSchedulingLeaseRefresh = 20 * time.Second
)

type AccountSchedulingPolicy struct {
	Enabled           bool `json:"enabled"`
	MaxActiveAccounts int  `json:"max_active_accounts"`
	// Keys are "direct" or a decimal proxy ID. Unmapped routes deliberately
	// share one conservative scope; creating another proxy is not a new budget.
	ProxyExitGroups map[string]string `json:"proxy_exit_groups"`
}

type AccountSchedulingPool struct {
	ID        int64                     `json:"id"`
	Name      string                    `json:"name"`
	Enabled   bool                      `json:"enabled"`
	CreatedAt time.Time                 `json:"created_at"`
	UpdatedAt time.Time                 `json:"updated_at"`
	Members   []AccountSchedulingMember `json:"members"`
}

type AccountSchedulingMember struct {
	AccountID int64      `json:"account_id"`
	PoolID    *int64     `json:"pool_id"`
	Position  int64      `json:"position"`
	Name      string     `json:"name"`
	State     string     `json:"state"`
	Reason    string     `json:"reason"`
	ExitGroup string     `json:"exit_group"`
	QueuedAt  *time.Time `json:"queued_at"`
}

// AccountSchedulingCandidate never contains credentials. Identity is a hash of
// the upstream identity, not an account display name or a mutable access token.
type AccountSchedulingCandidate struct {
	Identity  string
	AccountID int64
	ExitGroup string
	State     string
	Reason    string
	Position  int64
}

type AccountSchedulingSlot struct {
	Identity  string
	AccountID int64
	ExitGroup string
	State     string
	Reason    string
	QueuedAt  time.Time
	UpdatedAt time.Time
}

type AccountSchedulingPoolRepository interface {
	List(context.Context) ([]AccountSchedulingPool, error)
	Create(context.Context, *AccountSchedulingPool) error
	Update(context.Context, *AccountSchedulingPool) error
	Delete(context.Context, int64) error
	SetMembers(context.Context, *int64, []int64) error
	ListMembers(context.Context) ([]AccountSchedulingMember, error)
	UpdatePolicy(context.Context, AccountSchedulingPolicy) error
	// Reconcile serializes capacity decisions across all processes. A nonempty
	// leaseID additionally acquires a request lease for requestedIdentity.
	Reconcile(context.Context, AccountSchedulingAdmission) (*AccountSchedulingDecision, error)
	RefreshLease(context.Context, string, time.Duration) (bool, error)
	ReleaseLease(context.Context, string) error
}

type AccountSchedulingPoolService struct {
	repo                AccountSchedulingPoolRepository
	accounts            AccountRepository
	proxies             ProxyRepository
	settings            SettingRepository
	eligibilityMu       sync.Mutex
	eligibilityUntil    time.Time
	eligibilityDecision *AccountSchedulingDecision
}

func NewAccountSchedulingPoolService(repo AccountSchedulingPoolRepository, accounts AccountRepository, proxies ProxyRepository, settings SettingRepository) *AccountSchedulingPoolService {
	return &AccountSchedulingPoolService{repo: repo, accounts: accounts, proxies: proxies, settings: settings}
}

func (s *AccountSchedulingPoolService) GetPolicy(ctx context.Context) (AccountSchedulingPolicy, error) {
	p := AccountSchedulingPolicy{MaxActiveAccounts: 3, ProxyExitGroups: map[string]string{}}
	if s == nil || s.settings == nil {
		return p, nil
	}
	raw, err := s.settings.GetValue(ctx, AccountSchedulingPolicyKey)
	if errors.Is(err, ErrSettingNotFound) || (err == nil && strings.TrimSpace(raw) == "") {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return p, fmt.Errorf("read account scheduling policy: %w", err)
	}
	if err := ValidateAccountSchedulingPolicy(p); err != nil {
		return p, err
	}
	return p, nil
}

func ValidateAccountSchedulingPolicy(p AccountSchedulingPolicy) error {
	if p.MaxActiveAccounts < 1 || p.MaxActiveAccounts > 100 {
		return infraerrors.BadRequest("ACCOUNT_SCHEDULING_LIMIT_INVALID", "active account limit must be between 1 and 100")
	}
	for key, group := range p.ProxyExitGroups {
		if key != "direct" {
			id, err := strconv.ParseInt(key, 10, 64)
			if err != nil || id <= 0 || strconv.FormatInt(id, 10) != key {
				return infraerrors.BadRequest("ACCOUNT_SCHEDULING_EXIT_INVALID", "exit keys must be direct or a proxy ID")
			}
		}
		if strings.TrimSpace(group) == "" || utf8.RuneCountInString(group) > 100 {
			return infraerrors.BadRequest("ACCOUNT_SCHEDULING_EXIT_INVALID", "exit group must contain 1 to 100 characters")
		}
	}
	return nil
}

func (s *AccountSchedulingPoolService) UpdatePolicy(ctx context.Context, p AccountSchedulingPolicy) error {
	if err := ValidateAccountSchedulingPolicy(p); err != nil {
		return err
	}
	if p.ProxyExitGroups == nil {
		p.ProxyExitGroups = map[string]string{}
	}
	for key, value := range p.ProxyExitGroups {
		p.ProxyExitGroups[key] = strings.TrimSpace(value)
	}
	return s.repo.UpdatePolicy(ctx, p)
}

func (s *AccountSchedulingPoolService) List(ctx context.Context) ([]AccountSchedulingPool, error) {
	pools, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	members, err := s.Members(ctx, nil)
	if err != nil {
		return nil, err
	}
	for i := range pools {
		pools[i].Members = []AccountSchedulingMember{}
		for _, m := range members {
			if m.PoolID != nil && *m.PoolID == pools[i].ID {
				pools[i].Members = append(pools[i].Members, m)
			}
		}
	}
	return pools, nil
}

func validateAccountSchedulingPoolName(name string) error {
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 100 {
		return infraerrors.BadRequest("ACCOUNT_SCHEDULING_POOL_NAME_INVALID", "pool name must contain 1 to 100 characters")
	}
	return nil
}
func (s *AccountSchedulingPoolService) Create(ctx context.Context, name string, enabled bool) (*AccountSchedulingPool, error) {
	name = strings.TrimSpace(name)
	if err := validateAccountSchedulingPoolName(name); err != nil {
		return nil, err
	}
	p := &AccountSchedulingPool{Name: name, Enabled: enabled, Members: []AccountSchedulingMember{}}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}
func (s *AccountSchedulingPoolService) Update(ctx context.Context, id int64, name string, enabled bool) (*AccountSchedulingPool, error) {
	name = strings.TrimSpace(name)
	if err := validateAccountSchedulingPoolName(name); err != nil {
		return nil, err
	}
	p := &AccountSchedulingPool{ID: id, Name: name, Enabled: enabled}
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}
func (s *AccountSchedulingPoolService) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}
func (s *AccountSchedulingPoolService) SetMembers(ctx context.Context, poolID *int64, ids []int64) error {
	if len(ids) == 0 || len(ids) > 1000 {
		return infraerrors.BadRequest("ACCOUNT_SCHEDULING_MEMBERS_INVALID", "select between 1 and 1000 accounts")
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return infraerrors.BadRequest("ACCOUNT_SCHEDULING_MEMBERS_INVALID", "account IDs must be positive and unique")
		}
		seen[id] = true
	}
	accounts, err := s.accounts.GetByIDs(ctx, ids)
	if err != nil {
		return err
	}
	if len(accounts) != len(ids) {
		return ErrAccountNotFound
	}
	for _, a := range accounts {
		if a.Platform != PlatformOpenAI || a.Type != AccountTypeOAuth || a.IsShadow() {
			return infraerrors.BadRequest("ACCOUNT_SCHEDULING_ACCOUNT_INVALID", "pools currently support OpenAI OAuth parent accounts")
		}
	}
	return s.repo.SetMembers(ctx, poolID, ids)
}

// Members(nil) returns all target accounts, including the ungrouped accounts
// which share the same exit budget. A nonnil ID filters that view to one pool.
func (s *AccountSchedulingPoolService) Members(ctx context.Context, poolID *int64) ([]AccountSchedulingMember, error) {
	decision, err := s.repo.Reconcile(ctx, AccountSchedulingAdmission{})
	if err != nil {
		return nil, err
	}
	result := []AccountSchedulingMember{}
	for _, member := range decision.Members {
		if poolID == nil || (member.PoolID != nil && *member.PoolID == *poolID) {
			result = append(result, member)
		}
	}
	return result, nil
}

// Admission snapshots are read inside the repository's serialized transaction.
// This prevents an old cache value from re-enabling an account after an edit.
type AccountSchedulingAdmission struct {
	AccountID      int64
	ActualProxyURL string
	LeaseID        string
	LeaseTTL       time.Duration
}
type AccountSchedulingDecision struct {
	Enabled  bool
	Allowed  map[int64]bool
	Members  []AccountSchedulingMember
	Acquired bool
}

func accountSchedulingIdentity(a *Account) string {
	identity := strings.TrimSpace(a.GetChatGPTAccountID())
	if identity == "" {
		identity = strings.ToLower(strings.TrimSpace(a.GetCredential("email")))
	}
	if identity == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("openai:" + identity))
	return hex.EncodeToString(sum[:])
}

func accountSchedulingTarget(a *Account) bool {
	if a == nil || a.Platform != PlatformOpenAI || a.Type != AccountTypeOAuth {
		return false
	}
	// Only recognized paid tiers are exempt. New, empty, and malformed imported
	// tier values are governed conservatively until tier discovery.
	switch strings.ToLower(strings.TrimSpace(a.GetCredential("plan_type"))) {
	case "plus", "pro", "promax", "prolite", "go", "team", "business", "self_serve_business_prolite", "self_serve_business_usage_based", "enterprise", "enterprise_cbp_usage_based", "enterprise_cbp_automation", "edu", "education":
		return false
	default:
		return true
	}
}

func accountSchedulingCandidateState(a *Account, now time.Time) (string, string) {
	if a.Status == StatusError {
		message := strings.ToLower(a.ErrorMessage)
		for _, code := range []string{"token_revoked", "token_invalidated", "invalid_grant", "refresh_token_reused"} {
			if strings.Contains(message, code) {
				return AccountSchedulingReauth, "authentication_failed"
			}
		}
		return AccountSchedulingReauth, "account_error"
	}
	if a.Status != StatusActive || !a.Schedulable {
		return AccountSchedulingStandby, "administrator_disabled"
	}
	if a.ExpiresAt != nil && !now.Before(*a.ExpiresAt) {
		return AccountSchedulingStandby, "account_expired"
	}
	for _, window := range []string{"5h", "7d"} {
		if used, ok := resolveOpenAIQuotaUtilization(a.Extra, window, now); ok && used >= 1 {
			return AccountSchedulingCooldown, "quota_exhausted"
		}
	}
	if !a.IsSchedulable() {
		return AccountSchedulingCooldown, "temporary_cooldown"
	}
	return AccountSchedulingStandby, "ready"
}

func schedulingExitGroup(p AccountSchedulingPolicy, key string) string {
	if value := strings.TrimSpace(p.ProxyExitGroups[key]); value != "" {
		return value
	}
	return "default"
}

// Proxy rows with the same connection URL are aliases for one configured exit.
// Giving aliases separate budgets would defeat the shared-exit limit.
func ValidateAccountSchedulingProxyGroups(p AccountSchedulingPolicy, proxies []Proxy) error {
	groups := map[string]string{}
	for _, proxy := range proxies {
		url := proxy.URL()
		group := schedulingExitGroup(p, strconv.FormatInt(proxy.ID, 10))
		if previous, exists := groups[url]; exists && previous != group {
			return infraerrors.Conflict("ACCOUNT_SCHEDULING_EXIT_AMBIGUOUS", "identical proxies must share one exit group")
		}
		groups[url] = group
	}
	return nil
}

// BuildAccountSchedulingCandidates classifies a transactionally read snapshot.
// Callers must not retain the snapshot after its admission transaction commits.
func BuildAccountSchedulingCandidates(p AccountSchedulingPolicy, accounts []Account, members []AccountSchedulingMember, pools []AccountSchedulingPool, proxies []Proxy, actualAccountID int64, actualProxyURL string, now time.Time) ([]AccountSchedulingCandidate, error) {
	if err := ValidateAccountSchedulingProxyGroups(p, proxies); err != nil {
		return nil, err
	}
	memberMap := map[int64]AccountSchedulingMember{}
	for _, m := range members {
		memberMap[m.AccountID] = m
	}
	enabled := map[int64]bool{}
	for _, pool := range pools {
		enabled[pool.ID] = pool.Enabled
	}
	byID := map[int64]*Account{}
	for i := range accounts {
		byID[accounts[i].ID] = &accounts[i]
	}
	actualScope := ""
	if actualAccountID > 0 {
		actualScope = schedulingExitGroup(p, "direct")
		if actualProxyURL != "" {
			scopes := map[string]bool{}
			for _, proxy := range proxies {
				if proxy.URL() == actualProxyURL {
					scopes[schedulingExitGroup(p, strconv.FormatInt(proxy.ID, 10))] = true
				}
			}
			if len(scopes) > 1 {
				return nil, infraerrors.Conflict("ACCOUNT_SCHEDULING_EXIT_AMBIGUOUS", "identical proxies must share one exit group")
			}
			actualScope = "default"
			for scope := range scopes {
				actualScope = scope
			}
		}
	}
	result := []AccountSchedulingCandidate{}
	for i := range accounts {
		a := &accounts[i]
		identityAccount := a
		if a.ParentAccountID != nil {
			identityAccount = byID[*a.ParentAccountID]
			if identityAccount == nil {
				// An orphaned shadow must remain governed even if its cached tier
				// says paid. Its upstream identity cannot be verified without its parent.
				result = append(result, AccountSchedulingCandidate{Identity: "missing:" + strconv.FormatInt(*a.ParentAccountID, 10), AccountID: a.ID, ExitGroup: "default", State: AccountSchedulingReauth, Reason: "identity_parent_missing", Position: a.ID})
				continue
			}
		}
		if !accountSchedulingTarget(identityAccount) {
			continue
		}
		state, reason := accountSchedulingCandidateState(identityAccount, now)
		if a.ID != identityAccount.ID && (a.Status != StatusActive || !a.Schedulable) {
			state, reason = AccountSchedulingStandby, "administrator_disabled"
		}
		identity := accountSchedulingIdentity(identityAccount)
		if identity == "" {
			state, reason = AccountSchedulingReauth, "identity_missing"
			identity = "missing:" + strconv.FormatInt(identityAccount.ID, 10)
		}
		key := "direct"
		if a.ProxyID != nil {
			key = strconv.FormatInt(*a.ProxyID, 10)
		}
		scope := schedulingExitGroup(p, key)
		if actualAccountID == a.ID {
			expectedURL := ""
			if a.ProxyID != nil {
				found := false
				for _, proxy := range proxies {
					if proxy.ID == *a.ProxyID {
						expectedURL = proxy.URL()
						found = true
						break
					}
				}
				if !found {
					return nil, ErrAccountSchedulingUnavailable
				}
			}
			if expectedURL != actualProxyURL {
				return nil, ErrAccountSchedulingUnavailable
			}
			scope = actualScope
		}
		m := memberMap[identityAccount.ID]
		if m.PoolID != nil && !enabled[*m.PoolID] {
			state, reason = AccountSchedulingStandby, "pool_disabled"
		}
		position := m.Position
		if position == 0 {
			position = a.ID
		}
		result = append(result, AccountSchedulingCandidate{Identity: identity, AccountID: a.ID, ExitGroup: scope, State: state, Reason: reason, Position: position})
	}
	return result, nil
}

func (s *AccountSchedulingPoolService) Eligible(ctx context.Context, account *Account) (bool, error) {
	if s == nil || account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		return true, nil
	}
	allowed, err := s.EligibleAccounts(ctx, []Account{*account})
	if err != nil {
		return false, err
	}
	return len(allowed) > 0, nil
}
func (s *AccountSchedulingPoolService) EligibleAccounts(ctx context.Context, accounts []Account) ([]Account, error) {
	if s == nil {
		return accounts, nil
	}
	p, err := s.GetPolicy(ctx)
	if err != nil {
		return nil, err
	}
	if !p.Enabled {
		return accounts, nil
	}
	s.eligibilityMu.Lock()
	decision := s.eligibilityDecision
	if decision == nil || !time.Now().Before(s.eligibilityUntil) {
		decision, err = s.repo.Reconcile(ctx, AccountSchedulingAdmission{})
		if err == nil {
			s.eligibilityDecision = decision
			s.eligibilityUntil = time.Now().Add(200 * time.Millisecond)
		}
	}
	s.eligibilityMu.Unlock()
	if err != nil {
		return nil, err
	}
	if !decision.Enabled {
		return accounts, nil
	}
	capability, _ := ctx.Value(accountSchedulingSelectionCapabilityKey{}).(OpenAIEndpointCapability)
	result := make([]Account, 0, len(accounts))
	for _, a := range accounts {
		// Exclude governed Live candidates before priority/TopK selection. A
		// local unsupported account must not consume a Live failover attempt
		// and hide a supported paid account farther down the candidate list.
		if allowed, governed := decision.Allowed[a.ID]; !governed || (allowed && capability != OpenAIEndpointCapabilityLive) {
			result = append(result, a)
		}
	}
	return result, nil
}
func (s *AccountSchedulingPoolService) Acquire(ctx context.Context, accountID int64, actualProxyURL string) (*AccountSchedulingLease, error) {
	if s == nil {
		return nil, nil
	}
	p, err := s.GetPolicy(ctx)
	if err != nil {
		return nil, err
	}
	if !p.Enabled {
		return nil, nil
	}
	id := generateRequestID()
	decision, err := s.repo.Reconcile(ctx, AccountSchedulingAdmission{AccountID: accountID, ActualProxyURL: actualProxyURL, LeaseID: id, LeaseTTL: accountSchedulingLeaseTTL})
	if err != nil {
		return nil, err
	}
	if !decision.Enabled {
		return nil, nil
	}
	if _, governed := decision.Allowed[accountID]; !governed {
		return nil, nil
	}
	if !decision.Acquired {
		return nil, ErrAccountSchedulingUnavailable
	}
	leaseCtx, cancel := context.WithCancelCause(ctx)
	lease := &AccountSchedulingLease{ctx: leaseCtx, cancel: cancel, repo: s.repo, id: id, stop: make(chan struct{}), done: make(chan struct{})}
	go lease.refresh()
	return lease, nil
}

// Live media outlives its HTTP handshake. Until Live has a persistent account
// lease tied to the complete call, governed identities must not allocate it.
func (s *AccountSchedulingPoolService) ValidateLiveAccount(ctx context.Context, accountID int64, actualProxyURL string) error {
	if s == nil {
		return nil
	}
	policy, err := s.GetPolicy(ctx)
	if err != nil || !policy.Enabled {
		return err
	}
	decision, err := s.repo.Reconcile(ctx, AccountSchedulingAdmission{AccountID: accountID, ActualProxyURL: actualProxyURL})
	if err != nil {
		return err
	}
	if decision.Enabled {
		if _, governed := decision.Allowed[accountID]; governed {
			return ErrAccountSchedulingLiveUnsupported
		}
	}
	return nil
}

type AccountSchedulingLease struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	repo   AccountSchedulingPoolRepository
	id     string
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once
}

func (l *AccountSchedulingLease) Context() context.Context {
	if l == nil {
		return context.Background()
	}
	return l.ctx
}
func (l *AccountSchedulingLease) Release() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		close(l.stop)
		<-l.done
		l.cancel(nil)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = l.repo.ReleaseLease(ctx, l.id)
	})
}
func (l *AccountSchedulingLease) refresh() {
	defer close(l.done)
	ticker := time.NewTicker(accountSchedulingLeaseRefresh)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-l.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			ok, err := l.repo.RefreshLease(ctx, l.id, accountSchedulingLeaseTTL)
			cancel()
			if err != nil || !ok {
				l.cancel(ErrAccountSchedulingLeaseLost)
				return
			}
		}
	}
}

type accountSchedulingInferenceKey struct{}

// WithAccountSchedulingInference marks only business requests. Refreshes and
// quota probes intentionally remain outside the business-account limit.
func WithAccountSchedulingInference(ctx context.Context) context.Context {
	return context.WithValue(ctx, accountSchedulingInferenceKey{}, true)
}
func IsAccountSchedulingInference(ctx context.Context) bool {
	v, _ := ctx.Value(accountSchedulingInferenceKey{}).(bool)
	return v
}

type accountSchedulingPoolFilterKey struct{}

func WithAccountSchedulingPoolFilter(ctx context.Context, poolID *int64) context.Context {
	return context.WithValue(ctx, accountSchedulingPoolFilterKey{}, poolID)
}
func AccountSchedulingPoolFilter(ctx context.Context) *int64 {
	v, _ := ctx.Value(accountSchedulingPoolFilterKey{}).(*int64)
	return v
}
