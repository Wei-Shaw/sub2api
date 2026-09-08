package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrCodexIdentityTemplateNotFound = infraerrors.NotFound(
		"CODEX_IDENTITY_TEMPLATE_NOT_FOUND",
		"Codex identity template not found",
	)
	ErrCodexIdentityTemplateNameExists = infraerrors.Conflict(
		"CODEX_IDENTITY_TEMPLATE_NAME_EXISTS",
		"a Codex identity template with this name already exists",
	)
	ErrCodexIdentityTemplateRevisionConflict = infraerrors.Conflict(
		"CODEX_IDENTITY_TEMPLATE_REVISION_CONFLICT",
		"the Codex identity template was changed by another request",
	)
	ErrCodexIdentityTemplateInUse = infraerrors.Conflict(
		"CODEX_IDENTITY_TEMPLATE_IN_USE",
		"the Codex identity template is assigned to one or more accounts",
	)
	ErrCodexIdentityTemplateUpdateConfirmationRequired = infraerrors.Conflict(
		"CODEX_IDENTITY_TEMPLATE_UPDATE_CONFIRMATION_REQUIRED",
		"confirm the runtime update for accounts assigned to this template",
	)
)

// CodexIdentityTemplate is the reusable control-plane policy. Per-account
// policy/profile/slot rows are runtime projections and are intentionally not
// represented by this write contract.
type CodexIdentityTemplate struct {
	ID                   int64                          `json:"id"`
	Name                 string                         `json:"name"`
	Description          string                         `json:"description"`
	Revision             int64                          `json:"revision"`
	SessionPolicy        CodexSessionPolicySpec         `json:"session_policy"`
	AffinityTTLSeconds   int                            `json:"affinity_ttl_seconds"`
	UnsupportedPolicy    CodexUnsupportedProfilePolicy  `json:"unsupported_policy"`
	Profiles             []CodexIdentityTemplateProfile `json:"profiles"`
	AssignedAccountCount int64                          `json:"assigned_account_count"`
	CreatedAt            time.Time                      `json:"created_at"`
	UpdatedAt            time.Time                      `json:"updated_at"`
}

type CodexIdentityTemplateProfile struct {
	ID               int64                       `json:"id,omitempty"`
	OSClass          CodexOSClass                `json:"os_class"`
	CanonicalSurface CodexClientSurface          `json:"canonical_surface"`
	Architecture     CodexArchitecture           `json:"architecture,omitempty"`
	ProxyMode        CodexProxyMode              `json:"proxy_mode"`
	ProxyID          *int64                      `json:"proxy_id,omitempty"`
	SlotCount        int                         `json:"slot_count"`
	CatalogVersion   int64                       `json:"catalog_version"`
	Slots            []CodexIdentityTemplateSlot `json:"slots"`
}

type CodexIdentityTemplateSlot struct {
	ID                int64                  `json:"id,omitempty"`
	Index             int                    `json:"index"`
	ProxyMode         CodexProxyMode         `json:"proxy_mode"`
	ProxyID           *int64                 `json:"proxy_id,omitempty"`
	ClientVersionMode CodexClientVersionMode `json:"client_version_mode"`
	ClientVersion     string                 `json:"client_version,omitempty"`
}

// CodexIdentityTemplateRuntimeEqual compares only runtime-affecting template
// fields. Sparse slot rows and explicit inherit defaults are equivalent.
// Presentation metadata (name, description, IDs, revision and timestamps) is
// intentionally excluded so editing it cannot rotate live profiles.
func CodexIdentityTemplateRuntimeEqual(left, right *CodexIdentityTemplate) bool {
	if left == nil || right == nil ||
		left.SessionPolicy != right.SessionPolicy ||
		left.AffinityTTLSeconds != right.AffinityTTLSeconds ||
		left.UnsupportedPolicy != right.UnsupportedPolicy ||
		len(left.Profiles) != len(right.Profiles) {
		return false
	}
	for index := range left.Profiles {
		leftProfile := codexIdentityTemplateProfileRuntimeMaterial(left.Profiles[index])
		rightProfile := codexIdentityTemplateProfileRuntimeMaterial(right.Profiles[index])
		if !reflect.DeepEqual(leftProfile, rightProfile) {
			return false
		}
	}
	return true
}

func codexIdentityTemplateProfileRuntimeMaterial(profile CodexIdentityTemplateProfile) CodexIdentityTemplateProfile {
	profile.ID = 0
	if profile.SlotCount <= 0 {
		profile.Slots = append([]CodexIdentityTemplateSlot(nil), profile.Slots...)
		return profile
	}
	canonical := make([]CodexIdentityTemplateSlot, profile.SlotCount)
	for index := range canonical {
		canonical[index] = CodexIdentityTemplateSlot{
			Index:             index,
			ProxyMode:         CodexProxyInherit,
			ClientVersionMode: CodexClientVersionInherit,
		}
	}
	for _, slot := range profile.Slots {
		if slot.Index < 0 || slot.Index >= len(canonical) {
			continue
		}
		slot.ID = 0
		if slot.ProxyMode == "" {
			if slot.ProxyID != nil {
				slot.ProxyMode = CodexProxyExplicit
			} else {
				slot.ProxyMode = CodexProxyInherit
			}
		}
		if slot.ClientVersionMode == "" {
			slot.ClientVersionMode = CodexClientVersionInherit
		}
		if slot.ClientVersionMode == CodexClientVersionInherit {
			slot.ClientVersion = ""
		}
		canonical[slot.Index] = slot
	}
	profile.Slots = canonical
	return profile
}

type CodexIdentityTemplateCreateInput struct {
	Name               string
	Description        string
	SessionPolicy      CodexSessionPolicySpec
	AffinityTTLSeconds int
	UnsupportedPolicy  CodexUnsupportedProfilePolicy
	Profiles           []CodexIdentityTemplateProfile
}

type CodexIdentityTemplateUpdateInput struct {
	CodexIdentityTemplateCreateInput
	ExpectedRevision        int64
	ConfirmAssignedAccounts bool
}

type CodexIdentityAssignment struct {
	Enabled               bool   `json:"enabled"`
	TemplateID            int64  `json:"template_id,omitempty"`
	ExpectedRevision      *int64 `json:"expected_revision,omitempty"`
	ExpectedTemplateName  string `json:"expected_template_name,omitempty"`
	ExpectedRuntimeSHA256 string `json:"expected_runtime_sha256,omitempty"`
}

type CodexIdentityTemplateReader interface {
	GetCodexIdentityTemplate(ctx context.Context, id int64) (*CodexIdentityTemplate, error)
	GetCodexIdentityTemplateByName(ctx context.Context, name string) (*CodexIdentityTemplate, error)
}

func MaterializeCodexIdentityTemplate(template *CodexIdentityTemplate) (CodexIdentityPolicySpec, error) {
	if template == nil || template.ID <= 0 || template.Revision <= 0 {
		return CodexIdentityPolicySpec{}, ErrCodexIdentityTemplateNotFound
	}
	policy := CodexIdentityPolicySpec{
		Mode:               CodexIdentityPolicyOSProfileDevicePool,
		BindingScope:       CodexIdentityBindingAPIKeyOSSurface,
		SessionPolicy:      template.SessionPolicy,
		AffinityTTLSeconds: template.AffinityTTLSeconds,
		UnsupportedPolicy:  template.UnsupportedPolicy,
		Profiles:           make([]CodexOSProfilePolicy, len(template.Profiles)),
	}
	for i, profile := range template.Profiles {
		policy.Profiles[i] = CodexOSProfilePolicy{
			OSClass: profile.OSClass, CanonicalSurface: profile.CanonicalSurface,
			Architecture: profile.Architecture, ProxyMode: profile.ProxyMode,
			ProxyID: profile.ProxyID, SlotCount: profile.SlotCount,
			CatalogVersion: profile.CatalogVersion,
			Slots:          make([]CodexDeviceSlotPolicy, len(profile.Slots)),
		}
		for j, slot := range profile.Slots {
			policy.Profiles[i].Slots[j] = CodexDeviceSlotPolicy{
				Index: slot.Index, ProxyMode: slot.ProxyMode, ProxyID: slot.ProxyID,
				ClientVersionMode: slot.ClientVersionMode, ClientVersion: slot.ClientVersion,
			}
		}
	}
	return policy.NormalizeAndValidate(PlatformOpenAI, AccountTypeOAuth)
}

func CodexIdentityPolicyRuntimeSHA256(policy CodexIdentityPolicySpec) (string, error) {
	normalized, err := policy.NormalizeAndValidate(PlatformOpenAI, AccountTypeOAuth)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(codexPolicyMaterial(normalized))
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

// CodexIdentityTemplateRepository is deliberately narrow so account
// assignment/materialization can be added separately without granting the
// settings API account mutation capabilities.
type CodexIdentityTemplateRepository interface {
	ListCodexIdentityTemplates(ctx context.Context) ([]*CodexIdentityTemplate, error)
	GetCodexIdentityTemplate(ctx context.Context, id int64) (*CodexIdentityTemplate, error)
	CreateCodexIdentityTemplate(ctx context.Context, template *CodexIdentityTemplate) (*CodexIdentityTemplate, error)
	UpdateCodexIdentityTemplate(ctx context.Context, template *CodexIdentityTemplate, expectedRevision int64, confirmAssignedAccounts bool) (*CodexIdentityTemplate, error)
	DeleteCodexIdentityTemplate(ctx context.Context, id int64) error
}

type CodexIdentityTemplateService struct {
	repo      CodexIdentityTemplateRepository
	proxyRepo ProxyRepository
}

func NewCodexIdentityTemplateService(repo CodexIdentityTemplateRepository, proxyRepo ProxyRepository) *CodexIdentityTemplateService {
	return &CodexIdentityTemplateService{repo: repo, proxyRepo: proxyRepo}
}

func (s *CodexIdentityTemplateService) List(ctx context.Context) ([]*CodexIdentityTemplate, error) {
	return s.repo.ListCodexIdentityTemplates(ctx)
}

func (s *CodexIdentityTemplateService) Get(ctx context.Context, id int64) (*CodexIdentityTemplate, error) {
	if id <= 0 {
		return nil, infraerrors.BadRequest("INVALID_CODEX_IDENTITY_TEMPLATE_ID", "invalid Codex identity template id")
	}
	return s.repo.GetCodexIdentityTemplate(ctx, id)
}

func (s *CodexIdentityTemplateService) Create(ctx context.Context, input CodexIdentityTemplateCreateInput) (*CodexIdentityTemplate, error) {
	template, err := normalizeCodexIdentityTemplateInput(input)
	if err != nil {
		return nil, err
	}
	if err := s.validateTemplateProxies(ctx, template); err != nil {
		return nil, err
	}
	return s.repo.CreateCodexIdentityTemplate(ctx, template)
}

func (s *CodexIdentityTemplateService) Update(ctx context.Context, id int64, input CodexIdentityTemplateUpdateInput) (*CodexIdentityTemplate, error) {
	if id <= 0 {
		return nil, infraerrors.BadRequest("INVALID_CODEX_IDENTITY_TEMPLATE_ID", "invalid Codex identity template id")
	}
	if input.ExpectedRevision <= 0 {
		return nil, infraerrors.BadRequest("INVALID_CODEX_IDENTITY_TEMPLATE_REVISION", "expected_revision must be positive")
	}
	template, err := normalizeCodexIdentityTemplateInput(input.CodexIdentityTemplateCreateInput)
	if err != nil {
		return nil, err
	}
	template.ID = id
	if err := s.validateTemplateProxies(ctx, template); err != nil {
		return nil, err
	}
	return s.repo.UpdateCodexIdentityTemplate(ctx, template, input.ExpectedRevision, input.ConfirmAssignedAccounts)
}

func (s *CodexIdentityTemplateService) validateTemplateProxies(ctx context.Context, template *CodexIdentityTemplate) error {
	if template == nil {
		return nil
	}
	proxyIDs := make([]int64, 0)
	seen := make(map[int64]struct{})
	for _, profile := range template.Profiles {
		if profile.ProxyMode == CodexProxyExplicit && profile.ProxyID != nil {
			seen[*profile.ProxyID] = struct{}{}
		}
		for _, slot := range profile.Slots {
			if slot.ProxyMode == CodexProxyExplicit && slot.ProxyID != nil {
				seen[*slot.ProxyID] = struct{}{}
			}
		}
	}
	for id := range seen {
		proxyIDs = append(proxyIDs, id)
	}
	if len(proxyIDs) == 0 {
		return nil
	}
	if s.proxyRepo == nil {
		return infraerrors.BadRequest("INVALID_CODEX_IDENTITY_TEMPLATE_PROXY", "proxy repository is unavailable")
	}
	proxies, err := s.proxyRepo.ListByIDs(ctx, proxyIDs)
	if err != nil {
		return err
	}
	byID := make(map[int64]Proxy, len(proxies))
	for _, proxy := range proxies {
		byID[proxy.ID] = proxy
	}
	now := time.Now()
	for _, id := range proxyIDs {
		proxy, ok := byID[id]
		if !ok || !proxy.IsActive() || proxy.IsExpired(now) {
			return infraerrors.BadRequest("INVALID_CODEX_IDENTITY_TEMPLATE_PROXY", fmt.Sprintf("proxy %d is unavailable", id))
		}
	}
	return nil
}

func (s *CodexIdentityTemplateService) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return infraerrors.BadRequest("INVALID_CODEX_IDENTITY_TEMPLATE_ID", "invalid Codex identity template id")
	}
	return s.repo.DeleteCodexIdentityTemplate(ctx, id)
}

func normalizeCodexIdentityTemplateInput(input CodexIdentityTemplateCreateInput) (*CodexIdentityTemplate, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return nil, invalidCodexIdentityTemplate("name is required and must contain at most 100 characters")
	}
	description := strings.TrimSpace(input.Description)
	if utf8.RuneCountInString(description) > 500 {
		return nil, invalidCodexIdentityTemplate("description must contain at most 500 characters")
	}
	if input.SessionPolicy.Mode == "" {
		input.SessionPolicy.Mode = CodexSessionConversationIsolated
	}
	if err := validateCodexSessionPolicy(input.SessionPolicy); err != nil {
		return nil, invalidCodexIdentityTemplate("invalid session_policy: %v", err)
	}
	if input.AffinityTTLSeconds == 0 {
		input.AffinityTTLSeconds = 3600
	}
	if input.AffinityTTLSeconds < 60 || input.AffinityTTLSeconds > 86400 {
		return nil, invalidCodexIdentityTemplate("affinity_ttl_seconds must be between 60 and 86400")
	}
	if input.UnsupportedPolicy == "" {
		input.UnsupportedPolicy = CodexUnsupportedProfileReject
	}
	if input.UnsupportedPolicy != CodexUnsupportedProfileReject {
		return nil, invalidCodexIdentityTemplate("unsupported_policy must be reject")
	}
	if len(input.Profiles) == 0 {
		return nil, invalidCodexIdentityTemplate("at least one profile is required")
	}

	profiles := make([]CodexIdentityTemplateProfile, len(input.Profiles))
	seenProfiles := make(map[string]struct{}, len(input.Profiles))
	for i := range input.Profiles {
		profile, err := normalizeCodexIdentityTemplateProfile(input.Profiles[i])
		if err != nil {
			return nil, invalidCodexIdentityTemplate("profile %d: %v", i, err)
		}
		key := string(profile.OSClass) + "\x00" + string(profile.CanonicalSurface)
		if _, exists := seenProfiles[key]; exists {
			return nil, invalidCodexIdentityTemplate("duplicate profile for %s/%s", profile.OSClass, profile.CanonicalSurface)
		}
		seenProfiles[key] = struct{}{}
		profiles[i] = profile
	}
	sort.Slice(profiles, func(i, j int) bool {
		if profiles[i].OSClass != profiles[j].OSClass {
			return profiles[i].OSClass < profiles[j].OSClass
		}
		return profiles[i].CanonicalSurface < profiles[j].CanonicalSurface
	})

	return &CodexIdentityTemplate{
		Name:               name,
		Description:        description,
		SessionPolicy:      input.SessionPolicy,
		AffinityTTLSeconds: input.AffinityTTLSeconds,
		UnsupportedPolicy:  input.UnsupportedPolicy,
		Profiles:           profiles,
	}, nil
}

func normalizeCodexIdentityTemplateProfile(profile CodexIdentityTemplateProfile) (CodexIdentityTemplateProfile, error) {
	profile.ID = 0
	profile.OSClass = CodexOSClass(strings.TrimSpace(string(profile.OSClass)))
	profile.CanonicalSurface = CodexClientSurface(strings.TrimSpace(string(profile.CanonicalSurface)))
	profile.Architecture = CodexArchitecture(strings.TrimSpace(string(profile.Architecture)))
	if profile.CatalogVersion == 0 {
		profile.CatalogVersion = 1
	}
	if profile.CatalogVersion != 1 {
		return CodexIdentityTemplateProfile{}, fmt.Errorf("catalog_version must be 1")
	}
	if profile.SlotCount < 1 || profile.SlotCount > 3 {
		return CodexIdentityTemplateProfile{}, fmt.Errorf("slot_count must be between 1 and 3")
	}
	if err := normalizeCodexIdentityTemplateProxy(&profile.ProxyMode, profile.ProxyID); err != nil {
		return CodexIdentityTemplateProfile{}, fmt.Errorf("profile proxy: %w", err)
	}
	switch profile.OSClass {
	case CodexOSWindows, CodexOSMacOS, CodexOSLinux:
		if profile.CanonicalSurface != CodexSurfaceDesktop && profile.CanonicalSurface != CodexSurfaceCLI {
			return CodexIdentityTemplateProfile{}, fmt.Errorf("%s must use desktop or cli", profile.OSClass)
		}
		if profile.Architecture != CodexArchX8664 && profile.Architecture != CodexArchARM64 {
			return CodexIdentityTemplateProfile{}, fmt.Errorf("%s requires x86_64 or arm64 architecture", profile.OSClass)
		}
	case CodexOSGeneric:
		if profile.CanonicalSurface != CodexSurfaceSDK && profile.CanonicalSurface != CodexSurfaceThirdParty {
			return CodexIdentityTemplateProfile{}, fmt.Errorf("generic must use sdk or third_party")
		}
		if profile.Architecture != "" {
			return CodexIdentityTemplateProfile{}, fmt.Errorf("generic cannot declare an architecture")
		}
	default:
		return CodexIdentityTemplateProfile{}, fmt.Errorf("unsupported OS class %q", profile.OSClass)
	}

	slots := make([]CodexIdentityTemplateSlot, len(profile.Slots))
	seenSlots := make(map[int]struct{}, len(profile.Slots))
	for i := range profile.Slots {
		slot := profile.Slots[i]
		slot.ID = 0
		if slot.Index < 0 || slot.Index >= profile.SlotCount {
			return CodexIdentityTemplateProfile{}, fmt.Errorf("slot index %d is outside slot_count", slot.Index)
		}
		if _, exists := seenSlots[slot.Index]; exists {
			return CodexIdentityTemplateProfile{}, fmt.Errorf("duplicate slot index %d", slot.Index)
		}
		seenSlots[slot.Index] = struct{}{}
		if err := normalizeCodexIdentityTemplateProxy(&slot.ProxyMode, slot.ProxyID); err != nil {
			return CodexIdentityTemplateProfile{}, fmt.Errorf("slot %d proxy: %w", slot.Index, err)
		}
		if err := normalizeCodexClientVersion(&slot.ClientVersionMode, &slot.ClientVersion, fmt.Sprintf("slot %d", slot.Index)); err != nil {
			return CodexIdentityTemplateProfile{}, err
		}
		slots[i] = slot
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].Index < slots[j].Index })
	profile.Slots = slots
	return profile, nil
}

func normalizeCodexIdentityTemplateProxy(mode *CodexProxyMode, proxyID *int64) error {
	if *mode == "" {
		if proxyID == nil {
			*mode = CodexProxyInherit
		} else {
			*mode = CodexProxyExplicit
		}
	}
	switch *mode {
	case CodexProxyInherit, CodexProxyDirect:
		if proxyID != nil {
			return fmt.Errorf("proxy_id is only valid when proxy_mode=proxy")
		}
	case CodexProxyExplicit:
		if proxyID == nil || *proxyID <= 0 {
			return fmt.Errorf("proxy_mode=proxy requires a positive proxy_id")
		}
	default:
		return fmt.Errorf("unsupported proxy_mode %q", *mode)
	}
	return nil
}

func invalidCodexIdentityTemplate(format string, args ...any) error {
	return infraerrors.BadRequest("INVALID_CODEX_IDENTITY_TEMPLATE", fmt.Sprintf(format, args...))
}
