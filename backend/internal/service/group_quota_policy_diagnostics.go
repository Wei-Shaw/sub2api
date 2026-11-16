package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"golang.org/x/sync/singleflight"
)

type GroupQuotaModelOption struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	AccountIDs []int64 `json:"account_ids"`
}

type GroupQuotaAccountModel struct {
	ID            string `json:"id"`
	UpstreamModel string `json:"upstream_model"`
}

type GroupQuotaModelAccount struct {
	ID              int64                    `json:"id"`
	Name            string                   `json:"name"`
	Status          string                   `json:"status"`
	Schedulable     bool                     `json:"schedulable"`
	MonitorEligible bool                     `json:"monitor_eligible"`
	CatalogStatus   string                   `json:"catalog_status"`
	ErrorCode       string                   `json:"error_code,omitempty"`
	Models          []GroupQuotaAccountModel `json:"models"`
}

type GroupQuotaModels struct {
	GroupID   int64                    `json:"group_id"`
	Scope     string                   `json:"scope"`
	CheckedAt int64                    `json:"checked_at"`
	Accounts  []GroupQuotaModelAccount `json:"accounts"`
	Models    []GroupQuotaModelOption  `json:"models"`
	Warnings  []string                 `json:"warnings"`
}

type GroupQuotaMappingRequest struct {
	Source     string  `json:"source"`
	Target     string  `json:"target"`
	AccountIDs []int64 `json:"account_ids"`
	Force      bool    `json:"force,omitempty"`
}

type GroupQuotaMappingAccountResult struct {
	AccountID     int64  `json:"account_id"`
	AccountName   string `json:"account_name"`
	UpstreamModel string `json:"upstream_model"`
	Supported     bool   `json:"supported"`
	Status        string `json:"status"`
	ErrorCode     string `json:"error_code,omitempty"`
	Message       string `json:"message,omitempty"`
	LatencyMS     int64  `json:"latency_ms"`
	Cached        bool   `json:"cached"`
	CheckedAt     int64  `json:"checked_at"`
	ExpiresAt     int64  `json:"expires_at"`
}

type GroupQuotaMappingValidation struct {
	GroupID     int64                            `json:"group_id"`
	Source      string                           `json:"source"`
	Target      string                           `json:"target"`
	Scope       string                           `json:"scope"`
	Protocol    string                           `json:"protocol"`
	PointInTime bool                             `json:"point_in_time"`
	Status      string                           `json:"status"`
	CheckedAt   int64                            `json:"checked_at"`
	ExpiresAt   int64                            `json:"expires_at"`
	Cached      bool                             `json:"cached"`
	Accounts    []GroupQuotaMappingAccountResult `json:"accounts"`
	Warnings    []string                         `json:"warnings"`
}

type quotaDiagnosticCacheEntry struct {
	result  GroupQuotaMappingAccountResult
	expires time.Time
}

type groupQuotaPolicyDiagnostics struct {
	discover func(context.Context, *Account) ([]openai.Model, error)
	probe    func(context.Context, *Account, string) GroupQuotaMappingAccountResult
	gateway  *OpenAIGatewayService
	mu       sync.Mutex
	cache    map[string]quotaDiagnosticCacheEntry
	flights  singleflight.Group
	slots    chan struct{}
}

func newGroupQuotaPolicyDiagnostics() *groupQuotaPolicyDiagnostics {
	return &groupQuotaPolicyDiagnostics{cache: make(map[string]quotaDiagnosticCacheEntry), slots: make(chan struct{}, 2)}
}

// SetModelDiagnostics is only called while wiring the service at startup.
func (s *GroupQuotaPolicyService) SetModelDiagnostics(tester *AccountTestService) {
	if tester == nil {
		return
	}
	d := newGroupQuotaPolicyDiagnostics()
	d.discover, d.probe, d.gateway = quotaDiagnosticModelCandidates, tester.quotaTargetProbe, tester.openaiGatewayService
	s.diagnostics = d
}

// Model-list refreshes can quarantine OAuth accounts on 401 or recover Agent
// Identity tasks. An editor list therefore reads existing metadata/config only.
// Connectivity is established solely by the separate bounded target probe.
func quotaDiagnosticModelCandidates(_ context.Context, account *Account) ([]openai.Model, error) {
	models := make([]openai.Model, 0)
	seen := map[string]bool{}
	add := func(id string) {
		if validQuotaDiagnosticModel(id) && !seen[id] && account.IsModelSupported(id) {
			seen[id] = true
			models = append(models, openai.Model{ID: id})
		}
	}
	for id := range account.GetModelMapping() {
		add(id)
	}
	if snapshot := account.GetUpstreamModelMetadataSnapshot(); snapshot != nil {
		for id := range snapshot.Models {
			add(id)
		}
	} else {
		for _, model := range openai.DefaultModels {
			add(model.ID)
		}
	}
	return models, nil
}

func validQuotaDiagnosticModel(model string) bool {
	return model != "" && len(model) <= 200 && !strings.Contains(model, "*") && strings.IndexFunc(model, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) < 0
}

func (s *GroupQuotaPolicyService) diagnosticAccounts(ctx context.Context, groupID int64) (*Group, []Account, error) {
	group, err := s.groups.GetByIDLite(ctx, groupID)
	if err != nil {
		return nil, nil, err
	}
	if group == nil {
		return nil, nil, ErrGroupNotFound
	}
	if group.Platform != PlatformOpenAI {
		return nil, nil, infraerrors.BadRequest("GROUP_QUOTA_POLICY_PLATFORM", "quota policies currently support OpenAI groups only")
	}
	if s.diagnostics == nil || s.diagnostics.probe == nil || s.diagnostics.discover == nil {
		return nil, nil, infraerrors.New(503, "QUOTA_DIAGNOSTICS_UNAVAILABLE", "Quota model diagnostics are unavailable")
	}
	all, err := s.accounts.ListByGroup(ctx, groupID)
	if err != nil {
		return nil, nil, err
	}
	accounts := make([]Account, 0, len(all))
	for _, account := range all {
		if account.Platform == PlatformOpenAI {
			accounts = append(accounts, account)
		}
	}
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].ID < accounts[j].ID })
	return group, accounts, nil
}

func (s *GroupQuotaPolicyService) Models(ctx context.Context, groupID int64) (*GroupQuotaModels, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	_, accounts, err := s.diagnosticAccounts(ctx, groupID)
	if err != nil {
		return nil, err
	}
	result := &GroupQuotaModels{GroupID: groupID, Scope: "group", CheckedAt: time.Now().Unix(), Accounts: make([]GroupQuotaModelAccount, len(accounts)), Models: []GroupQuotaModelOption{}, Warnings: []string{
		"Catalog entries are candidates, not proof of connectivity. Rules execute across the active group account pool, independently of monitored accounts.",
		"Source names match after channel mapping; target names are resolved through each execution account's model mapping.",
		"Candidates come from existing account metadata, configured aliases and local defaults; this list does not refresh or authenticate upstream accounts.",
	}}
	var wg sync.WaitGroup
	for i := range accounts {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			a := &accounts[i]
			row := GroupQuotaModelAccount{ID: a.ID, Name: a.Name, Status: a.Status, Schedulable: a.IsSchedulable(), MonitorEligible: groupQuotaEligible(a), CatalogStatus: "unavailable", Models: []GroupQuotaAccountModel{}}
			defer func() { result.Accounts[i] = row }()
			seen := make(map[string]bool)
			add := func(id string) {
				if validQuotaDiagnosticModel(id) && !seen[id] && len(row.Models) < 500 {
					seen[id] = true
					row.Models = append(row.Models, GroupQuotaAccountModel{ID: id, UpstreamModel: ResolveOpenAIAccountUpstreamModelForRequest(a, id, false)})
				}
			}
			select {
			case s.diagnostics.slots <- struct{}{}:
			case <-ctx.Done():
				row.ErrorCode = "CATALOG_TIMEOUT"
				return
			}
			probeCtx, stop := context.WithTimeout(ctx, 8*time.Second)
			models, discoverErr := s.diagnostics.discover(probeCtx, a)
			stop()
			<-s.diagnostics.slots
			if discoverErr == nil {
				row.CatalogStatus = "configured"
				for _, model := range models {
					add(model.ID)
				}
			} else {
				row.ErrorCode, row.CatalogStatus = "CATALOG_UNAVAILABLE", "configured"
				for id := range a.GetModelMapping() {
					add(id)
				}
			}
			sort.Slice(row.Models, func(i, j int) bool { return row.Models[i].ID < row.Models[j].ID })
		}()
	}
	wg.Wait()
	byID := map[string]*GroupQuotaModelOption{}
	for _, account := range result.Accounts {
		for _, model := range account.Models {
			if byID[model.ID] == nil {
				byID[model.ID] = &GroupQuotaModelOption{ID: model.ID, Label: model.ID, AccountIDs: []int64{}}
			}
			byID[model.ID].AccountIDs = append(byID[model.ID].AccountIDs, account.ID)
		}
	}
	// Channel outputs are valid source names even when the source upstream is
	// no longer available (which may be exactly why a quota mapping is needed).
	if gateway := s.diagnostics.gateway; gateway != nil && gateway.channelService != nil {
		if channel, err := gateway.channelService.GetChannelForGroup(ctx, groupID); err == nil && channel != nil {
			for _, id := range channel.ModelMapping[PlatformOpenAI] {
				if validQuotaDiagnosticModel(id) && byID[id] == nil {
					byID[id] = &GroupQuotaModelOption{ID: id, Label: id, AccountIDs: []int64{}}
				}
			}
		}
	}
	for _, model := range byID {
		result.Models = append(result.Models, *model)
	}
	sort.Slice(result.Models, func(i, j int) bool { return result.Models[i].ID < result.Models[j].ID })
	return result, nil
}

// Fingerprints are hashes only, never logged or returned. Exclude usage-only
// timestamps such as LastUsedAt, but include every credential/extra/proxy change
// and the complete group membership. Conservative invalidation is intentional.
func quotaDiagnosticAccountFingerprint(account *Account) any {
	return struct {
		ID                                                                 int64
		Platform, Type, Status, QuotaDimension                             string
		Credentials, Extra                                                 map[string]any
		Proxy                                                              *Proxy
		ParentID                                                           *int64
		Schedulable                                                        bool
		ExpiresAt, RateLimitResetAt, OverloadUntil, TempUnschedulableUntil *time.Time
		AutoPause                                                          bool
	}{account.ID, account.Platform, account.Type, account.Status, account.QuotaDimension, account.Credentials, account.Extra, account.Proxy, account.ParentAccountID, account.Schedulable, account.ExpiresAt, account.RateLimitResetAt, account.OverloadUntil, account.TempUnschedulableUntil, account.AutoPauseOnExpired}
}

func (s *GroupQuotaPolicyService) diagnosticScopeKey(ctx context.Context, group *Group, accounts []Account) (string, error) {
	parts := []any{group}
	for i := range accounts {
		parts = append(parts, quotaDiagnosticAccountFingerprint(&accounts[i]))
		if accounts[i].IsCredentialShadow() {
			parent, err := resolveCredentialAccount(ctx, s.accounts, &accounts[i])
			if err != nil {
				return "", infraerrors.New(503, "QUOTA_DIAGNOSTIC_CREDENTIAL_UNAVAILABLE", "Unable to resolve a group account's credentials")
			}
			parts = append(parts, quotaDiagnosticAccountFingerprint(parent))
		}
	}
	if gateway := s.diagnostics.gateway; gateway != nil && gateway.channelService != nil {
		channel, err := gateway.channelService.GetChannelForGroup(ctx, group.ID)
		if err != nil {
			return "", err
		}
		parts = append(parts, channel)
	}
	encoded, err := json.Marshal(parts)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func (s *GroupQuotaPolicyService) ValidateMapping(ctx context.Context, groupID int64, request GroupQuotaMappingRequest) (*GroupQuotaMappingValidation, error) {
	ctx, cancel := context.WithTimeout(ctx, 28*time.Second)
	defer cancel()
	if !validQuotaDiagnosticModel(request.Source) || !validQuotaDiagnosticModel(request.Target) || request.Source == request.Target {
		return nil, infraerrors.BadRequest("GROUP_QUOTA_MAPPING_INVALID", "source and target must be distinct exact model names")
	}
	if len(request.AccountIDs) < 1 || len(request.AccountIDs) > 50 {
		return nil, infraerrors.BadRequest("GROUP_QUOTA_MAPPING_INVALID", "select between 1 and 50 monitored accounts")
	}
	group, accounts, err := s.diagnosticAccounts(ctx, groupID)
	if err != nil {
		return nil, err
	}
	eligible := map[int64]bool{}
	for i := range accounts {
		if groupQuotaEligible(&accounts[i]) {
			eligible[accounts[i].ID] = true
		}
	}
	for _, id := range request.AccountIDs {
		if !eligible[id] {
			return nil, infraerrors.BadRequest("GROUP_QUOTA_MAPPING_ACCOUNT_INVALID", "monitored accounts must be unique eligible members of this group")
		}
		delete(eligible, id)
	}
	scopeKey, err := s.diagnosticScopeKey(ctx, group, accounts)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	result := &GroupQuotaMappingValidation{GroupID: groupID, Source: request.Source, Target: request.Target, Scope: "group", Protocol: "responses_text", PointInTime: true, Status: "untested", CheckedAt: now, ExpiresAt: now + 60, Accounts: make([]GroupQuotaMappingAccountResult, len(accounts)), Warnings: []string{
		"This is a point-in-time Responses text test, not a guarantee of future scheduling, WebSocket, compact, or quota-trigger behavior.",
		"Monitored account selection does not restrict the group execution pool. Source names are checked statically and are not sent upstream.",
		"Original-client-name pricing restrictions and concurrent admission limits still apply to real requests.",
	}}
	var wg sync.WaitGroup
	for i := range accounts {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			a := &accounts[i]
			upstream := ResolveOpenAIAccountUpstreamModelForRequest(a, request.Target, false)
			row := GroupQuotaMappingAccountResult{AccountID: a.ID, AccountName: a.Name, UpstreamModel: upstream, Status: "skipped", CheckedAt: now, ExpiresAt: now + 15}
			defer func() { result.Accounts[i] = row }()
			if !a.IsModelSupported(request.Target) {
				row.Status, row.ErrorCode, row.Message = "unsupported", "MODEL_NOT_SUPPORTED", "Target is not supported by this account's model mapping"
				return
			}
			row.Supported = true
			if !quotaDiagnosticTextModel(upstream) {
				row.Status, row.ErrorCode, row.Message = "unsupported", "NON_TEXT_TARGET", "Automatic tests do not generate images, audio or video"
				return
			}
			if reason := openAICompatibleAccountEligibilityFailureReasonBeforeProfit(ctx, a, PlatformOpenAI, request.Target, false, OpenAIEndpointCapabilityResponses); reason != "" {
				row.ErrorCode, row.Message = "NOT_CURRENTLY_ELIGIBLE", "Account is currently unavailable for this Responses target ("+reason+")"
				return
			}
			if gateway := s.diagnostics.gateway; gateway != nil {
				if gateway.needsUpstreamChannelRestrictionCheck(ctx, &groupID) && gateway.isUpstreamModelRestrictedByChannel(ctx, groupID, a, request.Target, false) {
					row.Status, row.ErrorCode, row.Message = "unsupported", "CHANNEL_MODEL_RESTRICTED", "Target upstream model is restricted by the group's channel"
					return
				}
			}
			key := fmt.Sprintf("%s:%d:%s", scopeKey, a.ID, request.Target)
			probe := s.cachedQuotaTargetProbe(ctx, key, a, upstream, request.Force)
			probe.AccountID, probe.AccountName, probe.UpstreamModel = a.ID, a.Name, upstream
			row = probe
		}()
	}
	wg.Wait()
	passed, failed, allCached := 0, 0, len(result.Accounts) > 0
	for _, row := range result.Accounts {
		if row.Status == "passed" {
			passed++
		}
		if row.Status == "failed" || row.Status == "unsupported" {
			failed++
		}
		if !row.Cached {
			allCached = false
		}
		if row.ExpiresAt < result.ExpiresAt {
			result.ExpiresAt = row.ExpiresAt
		}
	}
	result.Cached = allCached
	switch {
	case passed > 0 && passed == len(result.Accounts):
		result.Status = "passed"
	case passed > 0:
		result.Status = "partial"
	case failed > 0:
		result.Status = "failed"
	}
	return result, nil
}

func (s *GroupQuotaPolicyService) cachedQuotaTargetProbe(ctx context.Context, key string, account *Account, upstream string, force bool) GroupQuotaMappingAccountResult {
	d := s.diagnostics
	readCache := func() (GroupQuotaMappingAccountResult, bool) {
		d.mu.Lock()
		defer d.mu.Unlock()
		cached, ok := d.cache[key]
		if ok && time.Now().Before(cached.expires) {
			result := cached.result
			result.Cached = true
			return result, true
		}
		return GroupQuotaMappingAccountResult{}, false
	}
	if !force {
		if result, ok := readCache(); ok {
			return result
		}
	}
	// Keep an already-sent probe alive when the editor cancels an old source
	// selection, so another row can join it instead of charging another request.
	flight := d.flights.DoChan(key, func() (any, error) {
		if !force {
			if result, ok := readCache(); ok {
				return result, nil
			}
		}
		probeCtx, cancel := context.WithTimeout(s.ctx, 28*time.Second)
		defer cancel()
		select {
		case d.slots <- struct{}{}:
		case <-probeCtx.Done():
			return GroupQuotaMappingAccountResult{Status: "skipped", Supported: true, ErrorCode: "PROBE_QUEUE_TIMEOUT", Message: "Target test queue timed out", CheckedAt: time.Now().Unix(), ExpiresAt: time.Now().Unix() + 15}, nil
		}
		defer func() { <-d.slots }()
		probeCtx, stop := context.WithTimeout(probeCtx, 10*time.Second)
		defer stop()
		result := d.probe(probeCtx, account, upstream)
		result.CheckedAt = time.Now().Unix()
		ttl := 15 * time.Second
		if result.Status == "passed" {
			ttl = 60 * time.Second
		}
		expires := time.Now().Add(ttl)
		result.ExpiresAt = expires.Unix()
		d.mu.Lock()
		if len(d.cache) >= 512 {
			for k, entry := range d.cache {
				if time.Now().After(entry.expires) {
					delete(d.cache, k)
				}
			}
			if len(d.cache) >= 512 {
				for k := range d.cache {
					delete(d.cache, k)
					break
				}
			}
		}
		d.cache[key] = quotaDiagnosticCacheEntry{result: result, expires: expires}
		d.mu.Unlock()
		return result, nil
	})
	select {
	case <-ctx.Done():
		return GroupQuotaMappingAccountResult{Status: "skipped", Supported: true, ErrorCode: "PROBE_TIMEOUT", Message: "Target test did not finish within the request deadline", CheckedAt: time.Now().Unix(), ExpiresAt: time.Now().Unix() + 15}
	case outcome := <-flight:
		if outcome.Err != nil {
			return GroupQuotaMappingAccountResult{Status: "failed", Supported: true, ErrorCode: "PROBE_FAILED", Message: "Target diagnostic failed", CheckedAt: time.Now().Unix(), ExpiresAt: time.Now().Unix() + 15}
		}
		return outcome.Val.(GroupQuotaMappingAccountResult)
	}
}
