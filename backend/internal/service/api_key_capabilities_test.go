package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/stretchr/testify/require"
)

type capabilityAPIKeyReaderStub struct {
	key *APIKey
	err error
}

func (s capabilityAPIKeyReaderStub) GetByID(context.Context, int64) (*APIKey, error) {
	return s.key, s.err
}

type capabilityGroupReaderStub struct {
	group *Group
	err   error
}

func (s capabilityGroupReaderStub) GetByID(context.Context, int64) (*Group, error) {
	return s.group, s.err
}

type capabilityAccountReaderStub struct {
	accounts       []Account
	platforms      []string
	groupID        *int64
	includeGrouped bool
	err            error
}

func (s *capabilityAccountReaderStub) ListModelAvailabilityCandidates(_ context.Context, groupID *int64, platforms []string, includeGrouped bool) ([]Account, error) {
	s.groupID = groupID
	s.platforms = append([]string(nil), platforms...)
	s.includeGrouped = includeGrouped
	return s.accounts, s.err
}

type capabilityAsyncImageStateStub struct {
	enabled  bool
	pollable bool
}

func (s capabilityAsyncImageStateStub) Enabled() bool  { return s.enabled }
func (s capabilityAsyncImageStateStub) Pollable() bool { return s.pollable }

func TestResolveCapabilitiesReturnsStructuredResponseForMissingKey(t *testing.T) {
	svc := &APIKeyCapabilitiesService{
		apiKeyRepo: capabilityAPIKeyReaderStub{},
	}

	caps, err := svc.ResolveCapabilities(context.Background(), 1, 999)

	require.NoError(t, err)
	require.NotNil(t, caps)
	require.Equal(t, "api_key_capabilities", caps.Object)
	require.Equal(t, int64(999), caps.KeyID)
	require.False(t, caps.Image.Available)
	require.Equal(t, CapabilityReasonNoGroup, caps.Image.Sync.Reason)
}

func TestResolveImageCapabilityRequiresGroupGateAndSchedulableMappedCandidate(t *testing.T) {
	groupID := int64(12)
	accountRepo := &capabilityAccountReaderStub{accounts: []Account{{
		Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"model_mapping": map[string]any{"gpt-image-2": "gpt-image-2"}},
	}}}
	svc := &APIKeyCapabilitiesService{
		apiKeyRepo:      capabilityAPIKeyReaderStub{key: &APIKey{ID: 9, UserID: 3, GroupID: &groupID}},
		groupRepo:       capabilityGroupReaderStub{group: &Group{ID: groupID, Platform: PlatformOpenAI, AllowImageGeneration: true}},
		accountRepo:     accountRepo,
		asyncImageState: capabilityAsyncImageStateStub{enabled: true, pollable: true},
	}

	caps, err := svc.ResolveCapabilities(context.Background(), 3, 9)

	require.NoError(t, err)
	require.True(t, caps.Image.GateOpen)
	require.True(t, caps.Image.AccountAvailable)
	require.True(t, caps.Image.Sync.Available)
	require.Equal(t, "POST", caps.Image.Sync.Method)
	require.Equal(t, "/v1/images/generations", caps.Image.Sync.Path)
	require.True(t, caps.Image.Async.Generation.Available)
	require.Equal(t, "POST", caps.Image.Async.Generation.Method)
	require.Equal(t, "/v1/images/generations/async", caps.Image.Async.Generation.Path)
	require.True(t, caps.Image.Async.Edit.Available)
	require.Equal(t, "POST", caps.Image.Async.Edit.Method)
	require.Equal(t, "/v1/images/edits/async", caps.Image.Async.Edit.Path)
	require.True(t, caps.Image.Async.Poll.Available)
	require.Equal(t, "GET", caps.Image.Async.Poll.Method)
	require.Equal(t, "/v1/images/tasks/{task_id}", caps.Image.Async.Poll.Path)
	require.Equal(t, &groupID, accountRepo.groupID)
	require.Equal(t, []string{PlatformOpenAI}, accountRepo.platforms)
}

func TestResolveImageCapabilityRejectsMissingGateAndUnsupportedPlatform(t *testing.T) {
	for _, tc := range []struct {
		name     string
		platform string
		gate     bool
		want     CapabilityReason
	}{
		{name: "group gate closed", platform: PlatformOpenAI, gate: false, want: CapabilityReasonGroupDisabled},
		{name: "platform unsupported", platform: PlatformComposite, gate: true, want: CapabilityReasonPlatformUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			groupID := int64(12)
			accountRepo := &capabilityAccountReaderStub{}
			svc := &APIKeyCapabilitiesService{
				accountRepo: accountRepo,
			}

			caps := svc.resolveImageCapability(context.Background(), &APIKey{ID: 9, UserID: 3, GroupID: &groupID}, &Group{
				ID: groupID, Platform: tc.platform, AllowImageGeneration: tc.gate,
			})

			require.False(t, caps.Sync.Available)
			require.Equal(t, tc.want, caps.Sync.Reason)
			require.Nil(t, accountRepo.groupID, "unsupported routes must not query unrelated account lanes")
		})
	}
}

func TestResolveImageCapabilityFailsClosedOnCandidateLookupError(t *testing.T) {
	groupID := int64(12)
	accountRepo := &capabilityAccountReaderStub{err: errors.New("repository unavailable")}
	svc := &APIKeyCapabilitiesService{
		apiKeyRepo:  capabilityAPIKeyReaderStub{key: &APIKey{ID: 9, UserID: 3, GroupID: &groupID}},
		groupRepo:   capabilityGroupReaderStub{group: &Group{ID: groupID, Platform: PlatformOpenAI, AllowImageGeneration: true}},
		accountRepo: accountRepo,
	}

	caps, err := svc.ResolveCapabilities(context.Background(), 3, 9)

	require.NoError(t, err)
	require.False(t, caps.Image.Available)
	require.Equal(t, CapabilityReasonAccountCapabilityUnverified, caps.Image.Sync.Reason)
}

func TestResolveImageCapabilityAsyncSubmitAndPollAreIndependent(t *testing.T) {
	groupID := int64(12)
	accountRepo := &capabilityAccountReaderStub{accounts: []Account{{
		Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
		Extra:       map[string]any{GrokMediaEligibleExtraKey: true},
		Credentials: map[string]any{"openai_capabilities": []string{"chat_completions", "grok_media_generation"}, "model_mapping": map[string]any{"grok-imagine-image": "grok-imagine-image"}},
	}}}
	svc := &APIKeyCapabilitiesService{
		apiKeyRepo:      capabilityAPIKeyReaderStub{key: &APIKey{ID: 9, UserID: 3, GroupID: &groupID}},
		groupRepo:       capabilityGroupReaderStub{group: &Group{ID: groupID, Platform: PlatformGrok, AllowImageGeneration: true}},
		accountRepo:     accountRepo,
		asyncImageState: capabilityAsyncImageStateStub{enabled: false, pollable: true},
	}

	caps, err := svc.ResolveCapabilities(context.Background(), 3, 9)

	require.NoError(t, err)
	require.True(t, caps.Image.Sync.Available)
	require.False(t, caps.Image.Async.Generation.Available)
	require.Equal(t, CapabilityReasonFeatureDisabled, caps.Image.Async.Generation.Reason)
	require.False(t, caps.Image.Async.Edit.Available)
	require.Equal(t, CapabilityReasonFeatureDisabled, caps.Image.Async.Edit.Reason)
	require.True(t, caps.Image.Async.Poll.Available)
	require.Equal(t, "/v1/images/tasks/{task_id}", caps.Image.Async.Poll.Path)
	require.Equal(t, []string{PlatformGrok}, accountRepo.platforms)
}

func TestResolveImageCapabilityAsyncRoutesHaveIndependentContracts(t *testing.T) {
	groupID := int64(49)
	account := Account{
		Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"model_mapping": map[string]any{"gpt-image-2": "gpt-image-2"}},
	}
	svc := &APIKeyCapabilitiesService{
		accountRepo:     &capabilityAccountReaderStub{accounts: []Account{account}},
		asyncImageState: capabilityAsyncImageStateStub{enabled: true, pollable: true},
	}
	cap := svc.resolveImageCapability(context.Background(), &APIKey{GroupID: &groupID}, &Group{
		ID: groupID, Platform: PlatformOpenAI, AllowImageGeneration: true,
	})
	encoded, err := json.Marshal(cap)
	require.NoError(t, err)

	var response map[string]any
	require.NoError(t, json.Unmarshal(encoded, &response))
	async, ok := response["async"].(map[string]any)
	require.True(t, ok)
	for name, path := range map[string]string{
		"generation": "/v1/images/generations/async",
		"edit":       "/v1/images/edits/async",
		"poll":       "/v1/images/tasks/{task_id}",
	} {
		route, ok := async[name].(map[string]any)
		require.True(t, ok, "async.%s must be a route object", name)
		require.Equal(t, path, route["path"])
		require.Equal(t, true, route["available"])
	}
}

func TestAPIKeyCapabilitiesSerializationContainsOnlyPublicContract(t *testing.T) {
	payload := APIKeyCapabilities{
		Object:     "api_key_capabilities",
		KeyID:      42,
		Group:      &APIKeyCapabilityGroup{ID: 7, Platform: PlatformOpenAI},
		Image:      APIKeyImageCapability{Sync: APIKeyCapabilityRoute{Available: true, Method: "POST", Path: "/v1/images/generations"}},
		BatchImage: APIKeyBatchImageCapability{Models: APIKeyCapabilityRoute{Available: true, Method: "GET", Path: "/v1/images/batches/models"}},
		Video:      APIKeyVideoCapability{Generation: APIKeyCapabilityRoute{Path: "/v1/videos/generations"}},
		Seedance:   APIKeySeedanceCapability{Status: APIKeyCapabilityRoute{Path: "/contents/generations/tasks/{task_id}"}},
		GeminiNative: APIKeyGeminiNativeCapability{ImageGeneration: APIKeyGeminiNativeImageCapability{
			GenerateContent: APIKeyCapabilityRoute{Path: "/v1beta/models/{model}:generateContent"},
		}},
	}

	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	serialized := string(encoded)
	for _, forbidden := range []string{
		"credential", "api_key", "token", "password", "secret", "base_url", "account_id", "upstream",
	} {
		require.NotContains(t, serialized, `"`+forbidden+`"`)
	}
}

type capabilityBatchImageReaderStub struct {
	response *BatchImagePublicModelsResponse
	err      error
	owner    BatchImageOwner
}

func (s *capabilityBatchImageReaderStub) ListModels(_ context.Context, owner BatchImageOwner) (*BatchImagePublicModelsResponse, error) {
	s.owner = owner
	return s.response, s.err
}

func TestBatchImageCapabilityFlagFalse(t *testing.T) {
	groupID := int64(12)
	svc := &APIKeyCapabilitiesService{accountRepo: &capabilityAccountReaderStub{}}

	caps := svc.resolveBatchImageCapability(context.Background(), &APIKey{ID: 9, UserID: 3, GroupID: &groupID}, &Group{
		ID: groupID, Platform: PlatformGemini, AllowBatchImageGeneration: false,
	})

	require.False(t, caps.Available)
	require.Equal(t, CapabilityReasonGroupDisabled, caps.Reason)
}

func TestBatchImageCapabilityRejectsNonGeminiPlatform(t *testing.T) {
	groupID := int64(12)
	accountRepo := &capabilityAccountReaderStub{}
	svc := &APIKeyCapabilitiesService{accountRepo: accountRepo}

	caps := svc.resolveBatchImageCapability(context.Background(), &APIKey{ID: 9, UserID: 3, GroupID: &groupID}, &Group{
		ID: groupID, Platform: PlatformOpenAI, AllowBatchImageGeneration: true,
	})

	require.False(t, caps.Available)
	require.Equal(t, CapabilityReasonPlatformUnsupported, caps.Reason)
	require.Nil(t, accountRepo.groupID)
}

func TestBatchImageCapabilityRequiresGeminiCandidate(t *testing.T) {
	groupID := int64(12)
	accountRepo := &capabilityAccountReaderStub{}
	reader := &capabilityBatchImageReaderStub{}
	svc := &APIKeyCapabilitiesService{accountRepo: accountRepo, batchImageReader: reader}

	caps := svc.resolveBatchImageCapability(context.Background(), &APIKey{ID: 9, UserID: 3, GroupID: &groupID}, &Group{
		ID: groupID, Platform: PlatformGemini, AllowBatchImageGeneration: true,
	})

	require.False(t, caps.Available)
	require.Equal(t, CapabilityReasonNoAccount, caps.Reason)
	require.Nil(t, reader.owner.GroupID)
}

func TestBatchImageCapabilityUsesExistingModelsDiscovery(t *testing.T) {
	groupID := int64(12)
	key := &APIKey{ID: 9, UserID: 3, GroupID: &groupID}
	accountRepo := &capabilityAccountReaderStub{accounts: []Account{{
		Platform: PlatformGemini, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"model_mapping": map[string]any{"gemini-2.5-flash-image": "gemini-2.5-flash-image"}},
	}}}
	reader := &capabilityBatchImageReaderStub{response: &BatchImagePublicModelsResponse{
		Object: "list",
		Data:   []BatchImagePublicModel{{ID: "gemini-2.5-flash-image", Provider: BatchImageProviderGeminiAPI}},
	}}
	svc := &APIKeyCapabilitiesService{accountRepo: accountRepo, batchImageReader: reader}

	caps := svc.resolveBatchImageCapability(context.Background(), key, &Group{
		ID: groupID, Platform: PlatformGemini, AllowBatchImageGeneration: true,
	})

	require.True(t, caps.Available)
	require.Equal(t, "GET", caps.Models.Method)
	require.Equal(t, "/v1/images/batches/models", caps.Models.Path)
	require.Equal(t, int64(9), reader.owner.APIKeyID)
	require.Equal(t, int64(3), reader.owner.UserID)
	require.Equal(t, &groupID, reader.owner.GroupID)
}

func TestBatchImageCapabilityRejectsUnbackedModelAndAllowlist(t *testing.T) {
	groupID := int64(13)
	key := &APIKey{ID: 9, UserID: 3, GroupID: &groupID}
	accountRepo := &capabilityAccountReaderStub{accounts: []Account{{
		Platform: PlatformGemini, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"model_mapping": map[string]any{"other-image-model": "other-image-model"}},
	}}}
	reader := &capabilityBatchImageReaderStub{response: &BatchImagePublicModelsResponse{
		Object: "list",
		Data:   []BatchImagePublicModel{{ID: "gemini-2.5-flash-image", Provider: BatchImageProviderGeminiAPI}},
	}}
	svc := &APIKeyCapabilitiesService{accountRepo: accountRepo, batchImageReader: reader}

	caps := svc.resolveBatchImageCapability(context.Background(), key, &Group{
		ID: groupID, Platform: PlatformGemini, AllowBatchImageGeneration: true,
		ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"other-image-model"}},
	})

	require.False(t, caps.Available)
	require.Equal(t, CapabilityReasonModelNotAvailable, caps.Reason)
}

func TestBatchImageCapabilityFailsClosedOnModelsDiscoveryError(t *testing.T) {
	groupID := int64(12)
	accountRepo := &capabilityAccountReaderStub{accounts: []Account{{
		Platform: PlatformGemini, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
	}}}
	svc := &APIKeyCapabilitiesService{
		accountRepo:      accountRepo,
		batchImageReader: &capabilityBatchImageReaderStub{err: errors.New("provider registry unavailable")},
	}

	caps := svc.resolveBatchImageCapability(context.Background(), &APIKey{ID: 9, UserID: 3, GroupID: &groupID}, &Group{
		ID: groupID, Platform: PlatformGemini, AllowBatchImageGeneration: true,
	})

	require.False(t, caps.Available)
	require.Equal(t, CapabilityReasonAccountCapabilityUnverified, caps.Reason)
}

func TestBatchImageCapabilityRejectsEmptyModelsDiscovery(t *testing.T) {
	groupID := int64(12)
	accountRepo := &capabilityAccountReaderStub{accounts: []Account{{
		Platform: PlatformGemini, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
	}}}
	svc := &APIKeyCapabilitiesService{
		accountRepo: accountRepo,
		batchImageReader: &capabilityBatchImageReaderStub{response: &BatchImagePublicModelsResponse{
			Object: "list",
			Data:   []BatchImagePublicModel{{ID: "", Provider: BatchImageProviderGeminiAPI}},
		}},
	}

	caps := svc.resolveBatchImageCapability(context.Background(), &APIKey{ID: 9, UserID: 3, GroupID: &groupID}, &Group{
		ID: groupID, Platform: PlatformGemini, AllowBatchImageGeneration: true,
	})

	require.False(t, caps.Available)
	require.Equal(t, CapabilityReasonModelNotAvailable, caps.Reason)
}

func TestBatchImageCapabilityMapsDisabledFeatureFromModelsDiscovery(t *testing.T) {
	groupID := int64(12)
	accountRepo := &capabilityAccountReaderStub{accounts: []Account{{
		Platform: PlatformGemini, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
	}}}
	svc := &APIKeyCapabilitiesService{
		accountRepo:      accountRepo,
		batchImageReader: &capabilityBatchImageReaderStub{err: ErrBatchImageDisabled},
	}

	caps := svc.resolveBatchImageCapability(context.Background(), &APIKey{ID: 9, UserID: 3, GroupID: &groupID}, &Group{
		ID: groupID, Platform: PlatformGemini, AllowBatchImageGeneration: true,
	})

	require.False(t, caps.Available)
	require.Equal(t, CapabilityReasonFeatureDisabled, caps.Reason)
}

func newGeminiNativeImageCapabilityFixture(accounts []Account, err error) (*APIKeyCapabilitiesService, *APIKey, *Group, *capabilityAccountReaderStub) {
	groupID := int64(42)
	key := &APIKey{ID: 9, UserID: 3, GroupID: &groupID}
	group := &Group{
		ID:                   groupID,
		Platform:             PlatformGemini,
		AllowImageGeneration: true,
		ModelAllowlist: GroupModelAllowlist{
			Enabled: true,
			Models:  []string{"gemini-2.5-flash-image"},
		},
	}
	accountRepo := &capabilityAccountReaderStub{accounts: accounts, err: err}
	svc := &APIKeyCapabilitiesService{
		apiKeyRepo:  capabilityAPIKeyReaderStub{key: key},
		groupRepo:   capabilityGroupReaderStub{group: group},
		accountRepo: accountRepo,
	}
	return svc, key, group, accountRepo
}

func TestGeminiNativeImageCapabilityRequiresRuntimeRouteAndProviderEvidence(t *testing.T) {
	account := Account{
		Platform:    PlatformGemini,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"access_token":  "[REDACTED]",
			"model_mapping": map[string]any{"gemini-2.5-flash-image": "gemini-2.5-flash-image"},
		},
	}
	svc, key, group, accountRepo := newGeminiNativeImageCapabilityFixture([]Account{account}, nil)

	caps := svc.resolveGeminiNativeImageCapability(context.Background(), key, group)

	require.True(t, caps.ImageGeneration.Available)
	require.True(t, caps.ImageGeneration.GenerateContent.Available)
	require.Equal(t, "POST", caps.ImageGeneration.GenerateContent.Method)
	require.Equal(t, "/v1beta/models/{model}:generateContent", caps.ImageGeneration.GenerateContent.Path)
	require.True(t, caps.ImageGeneration.StreamGenerateContent.Available)
	require.Equal(t, "POST", caps.ImageGeneration.StreamGenerateContent.Method)
	require.Equal(t, "/v1beta/models/{model}:streamGenerateContent?alt=sse", caps.ImageGeneration.StreamGenerateContent.Path)
	require.Equal(t, []string{PlatformGemini, PlatformAntigravity}, accountRepo.platforms)
	require.False(t, caps.ImageGeneration.Reason == CapabilityReasonAccountCapabilityUnverified,
		"a mapped, schedulable account must still pass native provider/route evidence")
}

func TestGeminiNativeImageCapabilityRejectsMappingMismatchAndUnschedulableAccount(t *testing.T) {
	tests := []struct {
		name    string
		account Account
		want    CapabilityReason
	}{
		{
			name: "mapping mismatch",
			account: Account{
				Platform:    PlatformGemini,
				Type:        AccountTypeOAuth,
				Status:      StatusActive,
				Schedulable: true,
				Credentials: map[string]any{"model_mapping": map[string]any{"gemini-2.5-flash": "gemini-2.5-flash"}},
			},
			want: CapabilityReasonModelNotAvailable,
		},
		{
			name: "unschedulable",
			account: Account{
				Platform:    PlatformGemini,
				Type:        AccountTypeOAuth,
				Status:      StatusActive,
				Schedulable: false,
				Credentials: map[string]any{"model_mapping": map[string]any{"gemini-2.5-flash-image": "gemini-2.5-flash-image"}},
			},
			want: CapabilityReasonNoAccount,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc, key, group, _ := newGeminiNativeImageCapabilityFixture([]Account{test.account}, nil)

			caps := svc.resolveGeminiNativeImageCapability(context.Background(), key, group)

			require.False(t, caps.ImageGeneration.Available)
			require.Equal(t, test.want, caps.ImageGeneration.Reason)
			require.False(t, caps.ImageGeneration.GenerateContent.Available)
			require.False(t, caps.ImageGeneration.StreamGenerateContent.Available)
		})
	}
}

func TestGeminiNativeImageCapabilityFailsClosedOnCandidateLookupError(t *testing.T) {
	svc, key, group, accountRepo := newGeminiNativeImageCapabilityFixture(nil, errors.New("candidate lookup failed"))

	caps := svc.resolveGeminiNativeImageCapability(context.Background(), key, group)

	require.False(t, caps.ImageGeneration.Available)
	require.Equal(t, CapabilityReasonAccountCapabilityUnverified, caps.ImageGeneration.Reason)
	require.False(t, caps.ImageGeneration.GenerateContent.Available)
	require.False(t, caps.ImageGeneration.StreamGenerateContent.Available)
	require.Equal(t, []string{PlatformGemini, PlatformAntigravity}, accountRepo.platforms)
}

func TestGeminiNativeProviderRouteEvidenceUsesAccountType(t *testing.T) {
	tests := []struct {
		name    string
		account Account
		want    bool
	}{
		{
			name: "gemini api key",
			account: Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"api_key": "[REDACTED]",
			}},
			want: true,
		},
		{
			name: "gemini oauth refreshable",
			account: Account{Platform: PlatformGemini, Type: AccountTypeOAuth, Credentials: map[string]any{
				"refresh_token": "[REDACTED]",
			}},
			want: true,
		},
		{
			name: "vertex service account",
			account: Account{Platform: PlatformGemini, Type: AccountTypeServiceAccount, Credentials: map[string]any{
				"service_account_json": `{"client_email":"[REDACTED]","private_key":"[REDACTED]","project_id":"fixture-project"}`,
			}},
			want: true,
		},
		{
			name:    "missing api key",
			account: Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{}},
			want:    false,
		},
		{
			name: "antigravity requires custom base url",
			account: Account{Platform: PlatformAntigravity, Type: AccountTypeAPIKey, Credentials: map[string]any{
				"api_key":  "test-api-key",
				"base_url": "https://antigravity.example.invalid",
			}},
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, geminiNativeProviderRouteSupported(&test.account, "gemini-2.5-flash-image"))
		})
	}
}

func TestGeminiNativeImageCapabilitySupportsEmptyPassthroughMapping(t *testing.T) {
	account := Account{
		Platform:    PlatformGemini,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{"api_key": "[REDACTED]"},
	}
	svc, key, group, _ := newGeminiNativeImageCapabilityFixture([]Account{account}, nil)
	group.ModelAllowlist = GroupModelAllowlist{}
	require.Equal(t, geminiNativeDefaultImageModel, account.GetMappedModel(geminiNativeDefaultImageModel))
	require.True(t, isImageGenerationModel(geminiNativeDefaultImageModel))
	require.True(t, group.ModelAllowlist.Allows(geminiNativeDefaultImageModel))
	require.True(t, account.IsModelSupported(geminiNativeDefaultImageModel))
	require.True(t, account.IsSchedulableForModelWithContext(context.Background(), geminiNativeDefaultImageModel))
	require.True(t, geminiNativeProviderRouteSupported(&account, geminiNativeDefaultImageModel))

	caps := svc.resolveGeminiNativeImageCapability(context.Background(), key, group)

	require.True(t, caps.ImageGeneration.Available)
}

func TestResolveVideoCapabilityUsesSchedulableGrokMediaCandidate(t *testing.T) {
	groupID := int64(44)
	accountRepo := &capabilityAccountReaderStub{accounts: []Account{{
		Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
		Extra: map[string]any{GrokMediaEligibleExtraKey: true},
		Credentials: map[string]any{
			"model_mapping": map[string]any{"grok-imagine-video": "grok-imagine-video"},
		},
	}}}
	svc := &APIKeyCapabilitiesService{accountRepo: accountRepo}

	cap := svc.resolveVideoCapability(context.Background(), &APIKey{ID: 9, UserID: 3, GroupID: &groupID}, &Group{
		ID: groupID, Platform: PlatformGrok, AllowImageGeneration: true,
	})

	require.True(t, cap.Generation.Available)
	require.Equal(t, "POST", cap.Generation.Method)
	require.Equal(t, "/v1/videos/generations", cap.Generation.Path)
	require.True(t, cap.Status.Available)
	require.Equal(t, "GET", cap.Status.Method)
	require.Equal(t, "/v1/videos/generations/{request_id}", cap.Status.Path)
	require.True(t, cap.Content.Available)
	require.Equal(t, "GET", cap.Content.Method)
	require.Equal(t, "/v1/videos/generations/{request_id}/content", cap.Content.Path)
	require.True(t, cap.Edit.Available)
	require.True(t, cap.Extension.Available)
	require.Equal(t, []string{PlatformGrok}, accountRepo.platforms)
}

func TestResolveVideoCapabilityFailsClosedWithoutSchedulableCandidate(t *testing.T) {
	groupID := int64(44)
	svc := &APIKeyCapabilitiesService{
		accountRepo: &capabilityAccountReaderStub{accounts: []Account{{
			Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: false,
			Credentials: map[string]any{"openai_capabilities": []string{"grok_media_generation"}},
		}}},
	}

	cap := svc.resolveVideoCapability(context.Background(), &APIKey{ID: 9, UserID: 3, GroupID: &groupID}, &Group{
		ID: groupID, Platform: PlatformGrok, AllowImageGeneration: true,
	})

	require.False(t, cap.Generation.Available)
	require.Equal(t, CapabilityReasonNoAccount, cap.Generation.Reason)
}

func TestResolveSeedanceCapabilityUsesMappedOpenAISeedanceCandidate(t *testing.T) {
	groupID := int64(45)
	accountRepo := &capabilityAccountReaderStub{accounts: []Account{{
		Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{
			"api_key":             "[REDACTED]",
			"base_url":            "https://example.invalid",
			"openai_capabilities": []string{"seedance"},
			"model_mapping":       map[string]any{"doubao-seedance-1-0-pro": "doubao-seedance-1-0-pro"},
		},
	}}}
	svc := &APIKeyCapabilitiesService{accountRepo: accountRepo}

	cap := svc.resolveSeedanceCapability(context.Background(), &APIKey{ID: 9, UserID: 3, GroupID: &groupID}, &Group{
		ID: groupID, Platform: PlatformOpenAI, AllowImageGeneration: true,
	})

	require.True(t, cap.Available)
	require.Empty(t, cap.Reason)
	require.Equal(t, "POST", cap.Create.Method)
	require.Equal(t, "GET", cap.Status.Method)
	require.Equal(t, "DELETE", cap.Delete.Method)
	require.Equal(t, []string{PlatformOpenAI}, accountRepo.platforms)
}

func TestResolveSeedanceCapabilityRequiresAPIKeyCustomEndpoint(t *testing.T) {
	groupID := int64(451)
	group := &Group{ID: groupID, Platform: PlatformOpenAI, AllowImageGeneration: true}
	key := &APIKey{GroupID: &groupID}
	for _, account := range []Account{
		{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
			Credentials: map[string]any{"base_url": "https://example.invalid", "model_mapping": map[string]any{"doubao-seedance-1-0-pro": "doubao-seedance-1-0-pro"}}},
		{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
			Credentials: map[string]any{"model_mapping": map[string]any{"doubao-seedance-1-0-pro": "doubao-seedance-1-0-pro"}}},
	} {
		svc := &APIKeyCapabilitiesService{accountRepo: &capabilityAccountReaderStub{accounts: []Account{account}}}
		cap := svc.resolveSeedanceCapability(context.Background(), key, group)
		require.False(t, cap.Available)
	}
}

func TestResolveVideoCapabilityRejectsTextMappedModelAndUnobservedOAuth(t *testing.T) {
	groupID := int64(44)
	group := &Group{ID: groupID, Platform: PlatformGrok, AllowImageGeneration: true}
	key := &APIKey{GroupID: &groupID}
	for _, account := range []Account{
		{Platform: PlatformGrok, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
			Credentials: map[string]any{"model_mapping": map[string]any{"grok-imagine-video": "grok-4.3"}}},
		{Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
			Credentials: map[string]any{"model_mapping": map[string]any{"grok-imagine-video": "grok-imagine-video"}}},
	} {
		svc := &APIKeyCapabilitiesService{accountRepo: &capabilityAccountReaderStub{accounts: []Account{account}}}
		cap := svc.resolveVideoCapability(context.Background(), key, group)
		require.False(t, cap.Generation.Available)
		require.False(t, cap.Edit.Available)
	}
}

func TestResolveVideoCapabilityRespectsAllowlist(t *testing.T) {
	groupID := int64(44)
	svc := &APIKeyCapabilitiesService{accountRepo: &capabilityAccountReaderStub{accounts: []Account{{
		Platform: PlatformGrok, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"model_mapping": map[string]any{"grok-imagine-video": "grok-imagine-video"}},
	}}}}
	cap := svc.resolveVideoCapability(context.Background(), &APIKey{GroupID: &groupID}, &Group{
		ID: groupID, Platform: PlatformGrok, AllowImageGeneration: true,
		ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"grok-4.3"}},
	})
	require.False(t, cap.Generation.Available)
}

func TestResolveSeedanceCapabilityRespectsAllowlistAndMappedTarget(t *testing.T) {
	groupID := int64(45)
	key := &APIKey{GroupID: &groupID}
	account := Account{
		Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"api_key": "[REDACTED]", "base_url": "https://example.invalid",
			"openai_capabilities": []string{"seedance"}, "model_mapping": map[string]any{"video-alias": "doubao-seedance-1-0-pro"}},
	}
	svc := &APIKeyCapabilitiesService{accountRepo: &capabilityAccountReaderStub{accounts: []Account{account}}}
	cap := svc.resolveSeedanceCapability(context.Background(), key, &Group{ID: groupID, Platform: PlatformOpenAI, AllowImageGeneration: true,
		ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"video-alias"}}})
	require.True(t, cap.Available)

	cap = svc.resolveSeedanceCapability(context.Background(), key, &Group{ID: groupID, Platform: PlatformOpenAI, AllowImageGeneration: true,
		ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"other-model"}}})
	require.False(t, cap.Available)

	account.Credentials["model_mapping"] = map[string]any{"doubao-seedance-1-0-pro": "gpt-4.1"}
	svc.accountRepo = &capabilityAccountReaderStub{accounts: []Account{account}}
	cap = svc.resolveSeedanceCapability(context.Background(), key, &Group{ID: groupID, Platform: PlatformOpenAI, AllowImageGeneration: true})
	require.False(t, cap.Available)
}

func TestResolveImageCapabilityRejectsNonImageMappingAndDisallowedModel(t *testing.T) {
	groupID := int64(46)
	key := &APIKey{GroupID: &groupID}
	group := &Group{
		ID: groupID, Platform: PlatformOpenAI, AllowImageGeneration: true,
		ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"gpt-image-2"}},
	}
	account := Account{
		Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{"model_mapping": map[string]any{"gpt-image-2": "gpt-5.6-sol"}},
	}
	svc := &APIKeyCapabilitiesService{accountRepo: &capabilityAccountReaderStub{accounts: []Account{account}}}

	cap := svc.resolveImageCapability(context.Background(), key, group)

	require.False(t, cap.Available)
	require.Equal(t, CapabilityReasonNoAccount, cap.Sync.Reason)

	account.Credentials["model_mapping"] = map[string]any{"gpt-image-2": "gpt-image-2"}
	group.ModelAllowlist.Models = []string{"other-model"}
	svc.accountRepo = &capabilityAccountReaderStub{accounts: []Account{account}}
	cap = svc.resolveImageCapability(context.Background(), key, group)

	require.False(t, cap.Available)
	require.Equal(t, CapabilityReasonNoAccount, cap.Sync.Reason)
}

func TestResolveImageCapabilityRejectsUnverifiedGrokBilling(t *testing.T) {
	groupID := int64(47)
	group := &Group{ID: groupID, Platform: PlatformGrok, AllowImageGeneration: true}
	key := &APIKey{GroupID: &groupID}

	for _, account := range []Account{
		{
			Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
			Credentials: map[string]any{"model_mapping": map[string]any{"grok-imagine-image": "grok-imagine-image"}},
		},
		{
			Platform: PlatformGrok, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
			Extra:       map[string]any{grokBillingExtraKey: &xai.BillingSummary{StatusCode: http.StatusOK, Partial: true}},
			Credentials: map[string]any{"model_mapping": map[string]any{"grok-imagine-image": "grok-imagine-image"}},
		},
	} {
		svc := &APIKeyCapabilitiesService{accountRepo: &capabilityAccountReaderStub{accounts: []Account{account}}}
		cap := svc.resolveImageCapability(context.Background(), key, group)
		require.False(t, cap.Available)
		require.Equal(t, CapabilityReasonNoAccount, cap.Sync.Reason)
	}
}

func TestResolveVideoCapabilitySupportsGrokImagineVideo15(t *testing.T) {
	groupID := int64(48)
	account := Account{
		Platform: PlatformGrok, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Extra:       map[string]any{GrokMediaEligibleExtraKey: true},
		Credentials: map[string]any{"model_mapping": map[string]any{"grok-imagine-video-1.5": "grok-imagine-video-1.5"}},
	}
	group := &Group{
		ID: groupID, Platform: PlatformGrok, AllowImageGeneration: true,
		ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"grok-imagine-video-1.5"}},
	}
	svc := &APIKeyCapabilitiesService{accountRepo: &capabilityAccountReaderStub{accounts: []Account{account}}}

	cap := svc.resolveVideoCapability(context.Background(), &APIKey{GroupID: &groupID}, group)

	require.True(t, cap.Generation.Available)
	require.True(t, cap.Status.Available)
}

func TestResolveVideoCapabilityCompositeIsGenerationOnly(t *testing.T) {
	groupID := int64(49)
	account := Account{
		Platform: PlatformGrok, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true,
		Extra:       map[string]any{GrokMediaEligibleExtraKey: true},
		Credentials: map[string]any{"model_mapping": map[string]any{"grok-imagine-video": "grok-imagine-video"}},
	}
	svc := &APIKeyCapabilitiesService{accountRepo: &capabilityAccountReaderStub{accounts: []Account{account}}}

	cap := svc.resolveVideoCapability(context.Background(), &APIKey{GroupID: &groupID}, &Group{
		ID: groupID, Platform: PlatformComposite, AllowImageGeneration: true,
		ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"grok-imagine-video"}},
	})

	require.True(t, cap.Generation.Available)
	require.True(t, cap.Status.Available)
	require.True(t, cap.Content.Available)
	require.False(t, cap.Edit.Available)
	require.Equal(t, CapabilityReasonRouteUnsupported, cap.Edit.Reason)
	require.False(t, cap.Extension.Available)
	require.Equal(t, CapabilityReasonRouteUnsupported, cap.Extension.Reason)
}
