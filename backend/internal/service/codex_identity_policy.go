package service

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type AccountProvisioningState string

const (
	AccountProvisioningPending AccountProvisioningState = "pending"
	AccountProvisioningActive  AccountProvisioningState = "active"
)

type CodexIdentityPolicyMode string

const (
	CodexIdentityPolicyOff                 CodexIdentityPolicyMode = "off"
	CodexIdentityPolicyOSProfileDevicePool CodexIdentityPolicyMode = "os_profile_device_pool"
)

type CodexOSClass string

const (
	CodexOSWindows CodexOSClass = "windows"
	CodexOSMacOS   CodexOSClass = "macos"
	CodexOSLinux   CodexOSClass = "linux"
	CodexOSGeneric CodexOSClass = "generic"
)

type CodexClientSurface string

const (
	CodexSurfaceDesktop    CodexClientSurface = "desktop"
	CodexSurfaceCLI        CodexClientSurface = "cli"
	CodexSurfaceSDK        CodexClientSurface = "sdk"
	CodexSurfaceThirdParty CodexClientSurface = "third_party"
)

type CodexArchitecture string

const (
	CodexArchX8664 CodexArchitecture = "x86_64"
	CodexArchARM64 CodexArchitecture = "arm64"
)

type CodexSessionPolicyMode string

const (
	CodexSessionConversationIsolated CodexSessionPolicyMode = "conversation_isolated"
	CodexSessionAPIKeyShared         CodexSessionPolicyMode = "api_key_shared"
	CodexSessionPool                 CodexSessionPolicyMode = "session_pool"
	CodexSessionDeviceShared         CodexSessionPolicyMode = "device_shared"
)

type CodexIdentityBindingScope string

const CodexIdentityBindingAPIKeyOS CodexIdentityBindingScope = "api_key_os"

type CodexUnsupportedProfilePolicy string

const CodexUnsupportedProfileReject CodexUnsupportedProfilePolicy = "reject"

const (
	defaultCodexAffinityTTLSeconds = 3600
	minCodexAffinityTTLSeconds     = 60
	maxCodexAffinityTTLSeconds     = 86400
	maxCodexDeviceSlotsPerProfile  = 3
)

// CodexSessionPolicySpec controls only application identity. HTTP/WS state
// remains isolated by downstream API key independently of this setting.
type CodexSessionPolicySpec struct {
	Mode                          CodexSessionPolicyMode `json:"mode"`
	SessionsPerDevice             int                    `json:"sessions_per_device,omitempty"`
	MaxActiveConversationsPerSlot int                    `json:"max_active_conversations_per_slot,omitempty"`
	DisableCrossKeyContinuation   bool                   `json:"disable_cross_key_continuation,omitempty"`
}

// CodexDeviceSlotPolicy contains administrator-controlled slot placement only.
// The installation ID is derived from the account seed and is never accepted
// through this contract.
type CodexDeviceSlotPolicy struct {
	Index   int    `json:"index"`
	ProxyID *int64 `json:"proxy_id,omitempty"`
}

// CodexOSProfilePolicy defines one canonical client surface for an OS class.
// Profiles are closed enums; arbitrary user agents and versions are not part of
// the account-management trust boundary.
type CodexOSProfilePolicy struct {
	OSClass          CodexOSClass            `json:"os_class"`
	CanonicalSurface CodexClientSurface      `json:"canonical_surface"`
	Architecture     CodexArchitecture       `json:"architecture,omitempty"`
	SlotCount        int                     `json:"slot_count"`
	ProxyID          *int64                  `json:"proxy_id,omitempty"`
	Epoch            int64                   `json:"epoch"`
	CatalogVersion   int64                   `json:"catalog_version,omitempty"`
	Slots            []CodexDeviceSlotPolicy `json:"slots,omitempty"`
}

// CodexIdentityPolicySpec is persisted as one immutable scheduling snapshot.
// An off policy has no profiles and preserves the existing account behavior.
type CodexIdentityPolicySpec struct {
	Mode               CodexIdentityPolicyMode       `json:"mode"`
	BindingScope       CodexIdentityBindingScope     `json:"binding_scope"`
	SessionPolicy      CodexSessionPolicySpec        `json:"session_policy"`
	AffinityTTLSeconds int                           `json:"affinity_ttl_seconds"`
	UnsupportedPolicy  CodexUnsupportedProfilePolicy `json:"unsupported_policy"`
	Version            int64                         `json:"version"`
	Profiles           []CodexOSProfilePolicy        `json:"profiles,omitempty"`
}

// AccountProvisioningSpec is the single create/import boundary. State is
// service-owned and intentionally excluded from JSON so callers cannot bypass
// validation by declaring an account active.
type AccountProvisioningSpec struct {
	Account           *Account                 `json:"-"`
	GroupIDs          []int64                  `json:"group_ids,omitempty"`
	Identity          *CodexIdentityPolicySpec `json:"identity,omitempty"`
	FinalStatus       string                   `json:"-"`
	Schedulable       bool                     `json:"-"`
	ProvisioningState AccountProvisioningState `json:"-"`
}

// AccountProvisioningRepository commits one complete account configuration.
// It is intentionally narrower than AccountRepository so gateway/test readers
// do not gain an admin-only write capability.
type AccountProvisioningRepository interface {
	ProvisionAccount(ctx context.Context, spec *AccountProvisioningSpec) error
	UpdateProvisionedAccount(
		ctx context.Context,
		spec *AccountProvisioningSpec,
		probeEnabled *bool,
		rateSyncEnabled *bool,
		rateMultiplier *float64,
	) error
}

type AtomicAccountProvisioningRepository interface {
	AccountProvisioningRepository
	SupportsAtomicAccountProvisioning() bool
}

func HasAtomicAccountProvisioning(repository any) bool {
	atomic, ok := repository.(AtomicAccountProvisioningRepository)
	return ok && atomic.SupportsAtomicAccountProvisioning()
}

func DefaultAccountProvisioningSpec() AccountProvisioningSpec {
	policy := DefaultCodexIdentityPolicySpec()
	return AccountProvisioningSpec{
		Identity:          &policy,
		FinalStatus:       StatusActive,
		Schedulable:       true,
		ProvisioningState: AccountProvisioningActive,
	}
}

func DefaultCodexIdentityPolicySpec() CodexIdentityPolicySpec {
	return CodexIdentityPolicySpec{
		Mode:               CodexIdentityPolicyOff,
		BindingScope:       CodexIdentityBindingAPIKeyOS,
		SessionPolicy:      CodexSessionPolicySpec{Mode: CodexSessionConversationIsolated},
		AffinityTTLSeconds: defaultCodexAffinityTTLSeconds,
		UnsupportedPolicy:  CodexUnsupportedProfileReject,
		Version:            1,
	}
}

func (s AccountProvisioningSpec) NormalizeAndValidate() (AccountProvisioningSpec, error) {
	normalized := s
	if normalized.Account == nil {
		return AccountProvisioningSpec{}, infraerrors.BadRequest("ACCOUNT_PROVISIONING_INVALID", "account is required")
	}
	if normalized.ProvisioningState == "" {
		normalized.ProvisioningState = AccountProvisioningActive
	}
	if normalized.ProvisioningState != AccountProvisioningPending && normalized.ProvisioningState != AccountProvisioningActive {
		return AccountProvisioningSpec{}, invalidCodexIdentityPolicy("unsupported provisioning state %q", normalized.ProvisioningState)
	}

	if normalized.FinalStatus == "" {
		normalized.FinalStatus = StatusActive
	}
	if normalized.FinalStatus != StatusActive && normalized.FinalStatus != StatusDisabled && normalized.FinalStatus != StatusError {
		return AccountProvisioningSpec{}, infraerrors.BadRequest("ACCOUNT_PROVISIONING_INVALID", "unsupported final account status")
	}
	policy := DefaultCodexIdentityPolicySpec()
	if normalized.Identity != nil {
		policy = *normalized.Identity
	}
	policy, err := policy.NormalizeAndValidate(normalized.Account.Platform, normalized.Account.Type)
	if err != nil {
		return AccountProvisioningSpec{}, err
	}
	normalized.Identity = &policy
	if policy.Mode == CodexIdentityPolicyOSProfileDevicePool {
		if normalized.ProvisioningState == AccountProvisioningActive &&
			!hasUsableCodexProvisioningCredential(normalized.Account) {
			return AccountProvisioningSpec{}, infraerrors.BadRequest(
				"ACCOUNT_PROVISIONING_INVALID",
				"active Codex OS profile accounts require OAuth bearer credentials or a valid Agent Identity runtime and private key",
			)
		}
		legacyMode, _ := normalized.Account.Extra[codexFingerprintModeExtraKey].(string)
		legacyMode = strings.TrimSpace(legacyMode)
		if legacyMode != "" && legacyMode != string(codexFingerprintOff) {
			return AccountProvisioningSpec{}, infraerrors.BadRequest(
				"ACCOUNT_PROVISIONING_INVALID",
				"os_profile_device_pool cannot be enabled while legacy codex_fingerprint_mode is active",
			)
		}
	}
	return normalized, nil
}

func hasUsableCodexProvisioningCredential(account *Account) bool {
	if account == nil {
		return false
	}
	credentials := account.Credentials
	for _, key := range []string{"access_token", "refresh_token"} {
		if value, _ := credentials[key].(string); strings.TrimSpace(value) != "" {
			return true
		}
	}
	if !account.IsOpenAIAgentIdentity() {
		return false
	}
	if strings.TrimSpace(account.GetCredential("agent_runtime_id")) == "" {
		return false
	}
	return ValidateOpenAIAgentIdentityPrivateKey(account.GetCredential("agent_private_key")) == nil
}

func (s CodexIdentityPolicySpec) NormalizeAndValidate(platform, accountType string) (CodexIdentityPolicySpec, error) {
	normalized := s
	normalized.Profiles = append([]CodexOSProfilePolicy(nil), s.Profiles...)
	for i := range normalized.Profiles {
		normalized.Profiles[i].Slots = append([]CodexDeviceSlotPolicy(nil), s.Profiles[i].Slots...)
	}
	if normalized.Mode == "" {
		normalized.Mode = CodexIdentityPolicyOff
	}
	if normalized.SessionPolicy.Mode == "" {
		normalized.SessionPolicy.Mode = CodexSessionConversationIsolated
	}
	if normalized.BindingScope == "" {
		normalized.BindingScope = CodexIdentityBindingAPIKeyOS
	}
	if normalized.UnsupportedPolicy == "" {
		normalized.UnsupportedPolicy = CodexUnsupportedProfileReject
	}
	if normalized.Version == 0 {
		normalized.Version = 1
	}
	if normalized.AffinityTTLSeconds == 0 {
		normalized.AffinityTTLSeconds = defaultCodexAffinityTTLSeconds
	}

	switch normalized.Mode {
	case CodexIdentityPolicyOff:
		if len(normalized.Profiles) != 0 {
			return CodexIdentityPolicySpec{}, invalidCodexIdentityPolicy("off mode cannot define OS profiles")
		}
	case CodexIdentityPolicyOSProfileDevicePool:
		if platform != PlatformOpenAI || accountType != AccountTypeOAuth {
			return CodexIdentityPolicySpec{}, invalidCodexIdentityPolicy("OS profile device pool requires an OpenAI OAuth account")
		}
		if len(normalized.Profiles) == 0 {
			return CodexIdentityPolicySpec{}, invalidCodexIdentityPolicy("OS profile device pool requires at least one profile")
		}
	default:
		return CodexIdentityPolicySpec{}, invalidCodexIdentityPolicy("unsupported identity policy mode %q", normalized.Mode)
	}

	if normalized.AffinityTTLSeconds < minCodexAffinityTTLSeconds || normalized.AffinityTTLSeconds > maxCodexAffinityTTLSeconds {
		return CodexIdentityPolicySpec{}, invalidCodexIdentityPolicy(
			"affinity_ttl_seconds must be between %d and %d",
			minCodexAffinityTTLSeconds,
			maxCodexAffinityTTLSeconds,
		)
	}
	if normalized.BindingScope != CodexIdentityBindingAPIKeyOS {
		return CodexIdentityPolicySpec{}, invalidCodexIdentityPolicy("unsupported binding_scope %q", normalized.BindingScope)
	}
	if normalized.UnsupportedPolicy != CodexUnsupportedProfileReject {
		return CodexIdentityPolicySpec{}, invalidCodexIdentityPolicy("unsupported unsupported_policy %q", normalized.UnsupportedPolicy)
	}
	if normalized.Version < 1 {
		return CodexIdentityPolicySpec{}, invalidCodexIdentityPolicy("version must be positive")
	}
	if err := validateCodexSessionPolicy(normalized.SessionPolicy); err != nil {
		return CodexIdentityPolicySpec{}, err
	}

	seenProfiles := make(map[CodexOSClass]struct{}, len(normalized.Profiles))
	for i := range normalized.Profiles {
		profile, err := normalizeCodexOSProfile(normalized.Profiles[i])
		if err != nil {
			return CodexIdentityPolicySpec{}, fmt.Errorf("profile %d: %w", i, err)
		}
		if _, exists := seenProfiles[profile.OSClass]; exists {
			return CodexIdentityPolicySpec{}, invalidCodexIdentityPolicy("duplicate OS profile %q", profile.OSClass)
		}
		seenProfiles[profile.OSClass] = struct{}{}
		normalized.Profiles[i] = profile
	}
	sort.Slice(normalized.Profiles, func(i, j int) bool {
		return normalized.Profiles[i].OSClass < normalized.Profiles[j].OSClass
	})
	return normalized, nil
}

func validateCodexSessionPolicy(policy CodexSessionPolicySpec) error {
	switch policy.Mode {
	case CodexSessionConversationIsolated, CodexSessionAPIKeyShared:
		if policy.SessionsPerDevice != 0 {
			return invalidCodexIdentityPolicy("sessions_per_device is only valid for session_pool")
		}
		if policy.MaxActiveConversationsPerSlot != 0 || policy.DisableCrossKeyContinuation {
			return invalidCodexIdentityPolicy("device_shared restrictions are only valid for device_shared")
		}
	case CodexSessionPool:
		if policy.SessionsPerDevice < 1 || policy.SessionsPerDevice > 3 {
			return invalidCodexIdentityPolicy("session_pool requires sessions_per_device between 1 and 3")
		}
		if policy.MaxActiveConversationsPerSlot != 0 || policy.DisableCrossKeyContinuation {
			return invalidCodexIdentityPolicy("device_shared restrictions are only valid for device_shared")
		}
	case CodexSessionDeviceShared:
		if policy.SessionsPerDevice != 0 {
			return invalidCodexIdentityPolicy("sessions_per_device is only valid for session_pool")
		}
		if policy.MaxActiveConversationsPerSlot != 1 || !policy.DisableCrossKeyContinuation {
			return invalidCodexIdentityPolicy(
				"device_shared requires max_active_conversations_per_slot=1 and disable_cross_key_continuation=true",
			)
		}
	default:
		return invalidCodexIdentityPolicy("unsupported session policy mode %q", policy.Mode)
	}
	return nil
}

func normalizeCodexOSProfile(profile CodexOSProfilePolicy) (CodexOSProfilePolicy, error) {
	profile.OSClass = CodexOSClass(strings.TrimSpace(string(profile.OSClass)))
	profile.CanonicalSurface = CodexClientSurface(strings.TrimSpace(string(profile.CanonicalSurface)))
	profile.Architecture = CodexArchitecture(strings.TrimSpace(string(profile.Architecture)))
	if profile.Epoch == 0 {
		profile.Epoch = 1
	}
	if profile.Epoch < 1 {
		return CodexOSProfilePolicy{}, invalidCodexIdentityPolicy("epoch must be positive")
	}
	if profile.CatalogVersion == 0 {
		profile.CatalogVersion = 1
	}
	if profile.CatalogVersion != 1 {
		return CodexOSProfilePolicy{}, invalidCodexIdentityPolicy("unsupported catalog_version %d", profile.CatalogVersion)
	}
	if profile.SlotCount < 1 || profile.SlotCount > maxCodexDeviceSlotsPerProfile {
		return CodexOSProfilePolicy{}, invalidCodexIdentityPolicy(
			"slot_count must be between 1 and %d",
			maxCodexDeviceSlotsPerProfile,
		)
	}

	switch profile.OSClass {
	case CodexOSWindows, CodexOSMacOS, CodexOSLinux:
		if profile.CanonicalSurface != CodexSurfaceDesktop && profile.CanonicalSurface != CodexSurfaceCLI {
			return CodexOSProfilePolicy{}, invalidCodexIdentityPolicy("%s profile must use desktop or cli", profile.OSClass)
		}
		if profile.Architecture != CodexArchX8664 && profile.Architecture != CodexArchARM64 {
			return CodexOSProfilePolicy{}, invalidCodexIdentityPolicy("%s profile requires x86_64 or arm64 architecture", profile.OSClass)
		}
	case CodexOSGeneric:
		if profile.CanonicalSurface != CodexSurfaceSDK && profile.CanonicalSurface != CodexSurfaceThirdParty {
			return CodexOSProfilePolicy{}, invalidCodexIdentityPolicy("generic profile must use sdk or third_party")
		}
		if profile.Architecture != "" {
			return CodexOSProfilePolicy{}, invalidCodexIdentityPolicy("generic profile cannot declare an architecture")
		}
	default:
		return CodexOSProfilePolicy{}, invalidCodexIdentityPolicy("unsupported OS class %q", profile.OSClass)
	}

	seenSlots := make(map[int]struct{}, len(profile.Slots))
	for _, slot := range profile.Slots {
		if slot.Index < 0 || slot.Index >= profile.SlotCount {
			return CodexOSProfilePolicy{}, invalidCodexIdentityPolicy("slot index %d is outside configured slot_count", slot.Index)
		}
		if _, exists := seenSlots[slot.Index]; exists {
			return CodexOSProfilePolicy{}, invalidCodexIdentityPolicy("duplicate slot index %d", slot.Index)
		}
		seenSlots[slot.Index] = struct{}{}
		if slot.ProxyID != nil && *slot.ProxyID <= 0 {
			return CodexOSProfilePolicy{}, invalidCodexIdentityPolicy("slot proxy_id must be positive")
		}
	}
	if profile.ProxyID != nil && *profile.ProxyID <= 0 {
		return CodexOSProfilePolicy{}, invalidCodexIdentityPolicy("profile proxy_id must be positive")
	}
	sort.Slice(profile.Slots, func(i, j int) bool { return profile.Slots[i].Index < profile.Slots[j].Index })
	return profile, nil
}

func PrepareCodexIdentityPolicyForCreate(
	requested CodexIdentityPolicySpec,
	platform string,
	accountType string,
) (CodexIdentityPolicySpec, error) {
	normalized, err := requested.NormalizeAndValidate(platform, accountType)
	if err != nil {
		return CodexIdentityPolicySpec{}, err
	}
	normalized.Version = 1
	for i := range normalized.Profiles {
		normalized.Profiles[i].Epoch = 1
	}
	return normalized, nil
}

func PrepareCodexIdentityPolicyForUpdate(
	existing CodexIdentityPolicySpec,
	requested CodexIdentityPolicySpec,
	platform string,
	accountType string,
) (CodexIdentityPolicySpec, bool, error) {
	return PrepareCodexIdentityPolicyForAccountTransition(
		existing, platform, accountType,
		requested, platform, accountType,
	)
}

func PrepareCodexIdentityPolicyForAccountTransition(
	existing CodexIdentityPolicySpec,
	existingPlatform string,
	existingAccountType string,
	requested CodexIdentityPolicySpec,
	newPlatform string,
	newAccountType string,
) (CodexIdentityPolicySpec, bool, error) {
	current, err := existing.NormalizeAndValidate(existingPlatform, existingAccountType)
	if err != nil {
		return CodexIdentityPolicySpec{}, false, err
	}
	next, err := requested.NormalizeAndValidate(newPlatform, newAccountType)
	if err != nil {
		return CodexIdentityPolicySpec{}, false, err
	}
	changed := !reflect.DeepEqual(codexPolicyMaterial(current), codexPolicyMaterial(next))
	if !changed {
		return current, false, nil
	}
	if current.Version < 1 {
		current.Version = 1
	}
	next.Version = current.Version + 1
	currentProfiles := make(map[CodexOSClass]CodexOSProfilePolicy, len(current.Profiles))
	for _, profile := range current.Profiles {
		currentProfiles[profile.OSClass] = profile
	}
	for i := range next.Profiles {
		previous, exists := currentProfiles[next.Profiles[i].OSClass]
		if !exists {
			next.Profiles[i].Epoch = 1
			continue
		}
		if reflect.DeepEqual(codexProfileMaterial(previous), codexProfileMaterial(next.Profiles[i])) {
			next.Profiles[i].Epoch = previous.Epoch
			continue
		}
		if previous.Epoch < 1 {
			previous.Epoch = 1
		}
		next.Profiles[i].Epoch = previous.Epoch + 1
	}
	return next, true, nil
}

func codexPolicyMaterial(policy CodexIdentityPolicySpec) CodexIdentityPolicySpec {
	material := policy
	material.Version = 0
	material.Profiles = append([]CodexOSProfilePolicy(nil), policy.Profiles...)
	for i := range material.Profiles {
		material.Profiles[i].Slots = append([]CodexDeviceSlotPolicy(nil), policy.Profiles[i].Slots...)
		material.Profiles[i].Epoch = 0
	}
	return material
}

func codexProfileMaterial(profile CodexOSProfilePolicy) CodexOSProfilePolicy {
	profile.Epoch = 0
	return profile
}

func (s CodexIdentityPolicySpec) ReferencedProxyIDs() []int64 {
	seen := make(map[int64]struct{})
	for _, profile := range s.Profiles {
		if profile.ProxyID != nil && *profile.ProxyID > 0 {
			seen[*profile.ProxyID] = struct{}{}
		}
		for _, slot := range profile.Slots {
			if slot.ProxyID != nil && *slot.ProxyID > 0 {
				seen[*slot.ProxyID] = struct{}{}
			}
		}
	}
	ids := make([]int64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func EncodeCodexIdentityPolicy(policy CodexIdentityPolicySpec) (map[string]any, error) {
	payload, err := json.Marshal(policy)
	if err != nil {
		return nil, err
	}
	encoded := make(map[string]any)
	if err := json.Unmarshal(payload, &encoded); err != nil {
		return nil, err
	}
	return encoded, nil
}

func DecodeCodexIdentityPolicy(raw map[string]any, platform, accountType string) (CodexIdentityPolicySpec, error) {
	if len(raw) == 0 {
		return DefaultCodexIdentityPolicySpec(), nil
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return CodexIdentityPolicySpec{}, err
	}
	var policy CodexIdentityPolicySpec
	if err := json.Unmarshal(payload, &policy); err != nil {
		return CodexIdentityPolicySpec{}, err
	}
	return policy.NormalizeAndValidate(platform, accountType)
}

func invalidCodexIdentityPolicy(format string, args ...any) error {
	return infraerrors.BadRequest("CODEX_IDENTITY_POLICY_INVALID", fmt.Sprintf(format, args...))
}

func (s *adminServiceImpl) validateAccountProvisioningProxies(
	ctx context.Context,
	accountProxyID *int64,
	policy CodexIdentityPolicySpec,
) error {
	if policy.Mode != CodexIdentityPolicyOSProfileDevicePool {
		return nil
	}
	proxyIDs := policy.ReferencedProxyIDs()
	if accountProxyID != nil {
		if *accountProxyID <= 0 {
			return infraerrors.BadRequest("ACCOUNT_PROVISIONING_INVALID", "account proxy_id must be positive")
		}
		proxyIDs = append(proxyIDs, *accountProxyID)
	}
	if len(proxyIDs) == 0 {
		return nil
	}
	if s == nil || s.proxyRepo == nil {
		return infraerrors.BadRequest("ACCOUNT_PROVISIONING_INVALID", "proxy repository is unavailable")
	}
	seen := make(map[int64]struct{}, len(proxyIDs))
	uniqueIDs := make([]int64, 0, len(proxyIDs))
	for _, id := range proxyIDs {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	proxies, err := s.proxyRepo.ListByIDs(ctx, uniqueIDs)
	if err != nil {
		return err
	}
	byID := make(map[int64]Proxy, len(proxies))
	for _, proxy := range proxies {
		byID[proxy.ID] = proxy
	}
	now := time.Now()
	for _, id := range uniqueIDs {
		proxy, ok := byID[id]
		if !ok {
			return infraerrors.BadRequest("ACCOUNT_PROVISIONING_INVALID", fmt.Sprintf("proxy %d does not exist", id))
		}
		if !proxy.IsActive() || proxy.IsExpired(now) {
			return infraerrors.BadRequest("ACCOUNT_PROVISIONING_INVALID", fmt.Sprintf("proxy %d is not active", id))
		}
	}
	return nil
}
