package service

import (
	"context"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type codexIdentityTemplateRepoStub struct {
	created          *CodexIdentityTemplate
	updated          *CodexIdentityTemplate
	expectedRevision int64
}

func (r *codexIdentityTemplateRepoStub) ListCodexIdentityTemplates(context.Context) ([]*CodexIdentityTemplate, error) {
	return nil, nil
}

func (r *codexIdentityTemplateRepoStub) GetCodexIdentityTemplate(context.Context, int64) (*CodexIdentityTemplate, error) {
	return nil, ErrCodexIdentityTemplateNotFound
}

func (r *codexIdentityTemplateRepoStub) CreateCodexIdentityTemplate(_ context.Context, template *CodexIdentityTemplate) (*CodexIdentityTemplate, error) {
	r.created = template
	return template, nil
}

func (r *codexIdentityTemplateRepoStub) UpdateCodexIdentityTemplate(_ context.Context, template *CodexIdentityTemplate, expectedRevision int64, _ bool) (*CodexIdentityTemplate, error) {
	r.updated = template
	r.expectedRevision = expectedRevision
	return template, nil
}

func (r *codexIdentityTemplateRepoStub) DeleteCodexIdentityTemplate(context.Context, int64) error {
	return nil
}

func TestCodexIdentityTemplateAllowsDesktopAndCLIForSameOS(t *testing.T) {
	repo := &codexIdentityTemplateRepoStub{}
	svc := NewCodexIdentityTemplateService(repo, nil)

	created, err := svc.Create(context.Background(), CodexIdentityTemplateCreateInput{
		Name: "dual surface",
		Profiles: []CodexIdentityTemplateProfile{
			{
				OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceDesktop,
				Architecture: CodexArchX8664, SlotCount: 1,
			},
			{
				OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceCLI,
				Architecture: CodexArchARM64, SlotCount: 2,
			},
		},
	})
	require.NoError(t, err)
	require.Same(t, repo.created, created)
	require.Len(t, created.Profiles, 2)
	require.Equal(t, CodexSurfaceCLI, created.Profiles[0].CanonicalSurface)
	require.Equal(t, CodexSurfaceDesktop, created.Profiles[1].CanonicalSurface)
	require.Equal(t, CodexProxyInherit, created.Profiles[0].ProxyMode)
	require.Equal(t, int64(1), created.Profiles[0].CatalogVersion)
	require.Equal(t, 3600, created.AffinityTTLSeconds)
	require.Equal(t, CodexSessionConversationIsolated, created.SessionPolicy.Mode)
}

func TestCodexIdentityTemplateRejectsDuplicateOSSurface(t *testing.T) {
	svc := NewCodexIdentityTemplateService(&codexIdentityTemplateRepoStub{}, nil)
	_, err := svc.Create(context.Background(), CodexIdentityTemplateCreateInput{
		Name: "duplicate",
		Profiles: []CodexIdentityTemplateProfile{
			{OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceCLI, Architecture: CodexArchX8664, SlotCount: 1},
			{OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceCLI, Architecture: CodexArchARM64, SlotCount: 1},
		},
	})
	require.Error(t, err)
	require.Equal(t, "INVALID_CODEX_IDENTITY_TEMPLATE", infraerrors.Reason(err))
}

func TestCodexIdentityTemplateUpdateRequiresAndForwardsExpectedRevision(t *testing.T) {
	repo := &codexIdentityTemplateRepoStub{}
	svc := NewCodexIdentityTemplateService(repo, nil)
	input := CodexIdentityTemplateUpdateInput{
		CodexIdentityTemplateCreateInput: CodexIdentityTemplateCreateInput{
			Name: "updated",
			Profiles: []CodexIdentityTemplateProfile{{
				OSClass: CodexOSMacOS, CanonicalSurface: CodexSurfaceDesktop,
				Architecture: CodexArchARM64, SlotCount: 1,
			}},
		},
	}

	_, err := svc.Update(context.Background(), 7, input)
	require.Error(t, err)
	require.Equal(t, "INVALID_CODEX_IDENTITY_TEMPLATE_REVISION", infraerrors.Reason(err))

	input.ExpectedRevision = 4
	updated, err := svc.Update(context.Background(), 7, input)
	require.NoError(t, err)
	require.Equal(t, int64(7), updated.ID)
	require.Equal(t, int64(4), repo.expectedRevision)
}

func TestCodexIdentityTemplateRejectsInvalidSlotProxyShape(t *testing.T) {
	proxyID := int64(9)
	svc := NewCodexIdentityTemplateService(&codexIdentityTemplateRepoStub{}, nil)
	_, err := svc.Create(context.Background(), CodexIdentityTemplateCreateInput{
		Name: "bad proxy",
		Profiles: []CodexIdentityTemplateProfile{{
			OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceCLI,
			Architecture: CodexArchX8664, SlotCount: 1,
			Slots: []CodexIdentityTemplateSlot{{Index: 0, ProxyMode: CodexProxyDirect, ProxyID: &proxyID}},
		}},
	})
	require.Error(t, err)
	require.Equal(t, "INVALID_CODEX_IDENTITY_TEMPLATE", infraerrors.Reason(err))
}

func TestCodexIdentityTemplateNormalizesPinnedSlotClientVersion(t *testing.T) {
	repo := &codexIdentityTemplateRepoStub{}
	svc := NewCodexIdentityTemplateService(repo, nil)
	created, err := svc.Create(context.Background(), CodexIdentityTemplateCreateInput{
		Name: "pinned client",
		Profiles: []CodexIdentityTemplateProfile{{
			OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceCLI,
			Architecture: CodexArchX8664, SlotCount: 1,
			Slots: []CodexIdentityTemplateSlot{{
				Index: 0, ClientVersionMode: CodexClientVersionPinned, ClientVersion: " 0.200.1 ",
			}},
		}},
	})
	require.NoError(t, err)
	require.Equal(t, CodexClientVersionPinned, created.Profiles[0].Slots[0].ClientVersionMode)
	require.Equal(t, "0.200.1", created.Profiles[0].Slots[0].ClientVersion)

	created.ID = 7
	created.Revision = 1
	policy, err := MaterializeCodexIdentityTemplate(created)
	require.NoError(t, err)
	require.Equal(t, CodexClientVersionPinned, policy.Profiles[0].Slots[0].ClientVersionMode)
	require.Equal(t, "0.200.1", policy.Profiles[0].Slots[0].ClientVersion)
}

func TestCodexIdentityTemplateRejectsInvalidPinnedSlotClientVersion(t *testing.T) {
	svc := NewCodexIdentityTemplateService(&codexIdentityTemplateRepoStub{}, nil)
	for name, slot := range map[string]CodexIdentityTemplateSlot{
		"below minimum": {Index: 0, ClientVersionMode: CodexClientVersionPinned, ClientVersion: "0.143.9"},
		"invalid":       {Index: 0, ClientVersionMode: CodexClientVersionPinned, ClientVersion: "latest"},
		"inherit value": {Index: 0, ClientVersionMode: CodexClientVersionInherit, ClientVersion: "0.200.1"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Create(context.Background(), CodexIdentityTemplateCreateInput{
				Name: "bad " + name,
				Profiles: []CodexIdentityTemplateProfile{{
					OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceCLI,
					Architecture: CodexArchX8664, SlotCount: 1,
					Slots: []CodexIdentityTemplateSlot{slot},
				}},
			})
			require.Error(t, err)
			require.Equal(t, "INVALID_CODEX_IDENTITY_TEMPLATE", infraerrors.Reason(err))
		})
	}
}

func TestMaterializeCodexIdentityTemplatePreservesAssignmentPolicy(t *testing.T) {
	proxyID := int64(9)
	policy, err := MaterializeCodexIdentityTemplate(&CodexIdentityTemplate{
		ID: 7, Revision: 3,
		SessionPolicy:      CodexSessionPolicySpec{Mode: CodexSessionAPIKeyShared},
		AffinityTTLSeconds: 7200, UnsupportedPolicy: CodexUnsupportedProfileReject,
		Profiles: []CodexIdentityTemplateProfile{
			{OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceDesktop, Architecture: CodexArchX8664, ProxyMode: CodexProxyInherit, SlotCount: 1, CatalogVersion: 1,
				Slots: []CodexIdentityTemplateSlot{{Index: 0, ClientVersionMode: CodexClientVersionPinned, ClientVersion: "0.200.1"}}},
			{OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceCLI, Architecture: CodexArchARM64, ProxyMode: CodexProxyExplicit, ProxyID: &proxyID, SlotCount: 1, CatalogVersion: 1},
		},
	})
	require.NoError(t, err)
	require.Equal(t, CodexIdentityPolicyOSProfileDevicePool, policy.Mode)
	require.Equal(t, CodexIdentityBindingAPIKeyOSSurface, policy.BindingScope)
	require.Equal(t, CodexSessionAPIKeyShared, policy.SessionPolicy.Mode)
	require.Equal(t, 7200, policy.AffinityTTLSeconds)
	require.Len(t, policy.Profiles, 2)
	require.Equal(t, CodexSurfaceCLI, policy.Profiles[0].CanonicalSurface)
	require.Equal(t, proxyID, *policy.Profiles[0].ProxyID)
	require.Equal(t, CodexClientVersionPinned, policy.Profiles[1].Slots[0].ClientVersionMode)
	require.Equal(t, "0.200.1", policy.Profiles[1].Slots[0].ClientVersion)
}

func TestCodexIdentityPolicyRuntimeSHA256IgnoresProjectionVersionsButIncludesSurface(t *testing.T) {
	base := CodexIdentityPolicySpec{
		Mode:               CodexIdentityPolicyOSProfileDevicePool,
		BindingScope:       CodexIdentityBindingAPIKeyOSSurface,
		SessionPolicy:      CodexSessionPolicySpec{Mode: CodexSessionConversationIsolated},
		AffinityTTLSeconds: 3600, UnsupportedPolicy: CodexUnsupportedProfileReject, Version: 2,
		Profiles: []CodexOSProfilePolicy{{
			OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceDesktop,
			Architecture: CodexArchX8664, ProxyMode: CodexProxyInherit,
			SlotCount: 1, Epoch: 3, CatalogVersion: 1,
		}},
	}
	first, err := CodexIdentityPolicyRuntimeSHA256(base)
	require.NoError(t, err)
	versionOnly := base
	versionOnly.Version = 9
	versionOnly.Profiles = append([]CodexOSProfilePolicy(nil), base.Profiles...)
	versionOnly.Profiles[0].Epoch = 11
	second, err := CodexIdentityPolicyRuntimeSHA256(versionOnly)
	require.NoError(t, err)
	require.Equal(t, first, second)
	surfaceChange := versionOnly
	surfaceChange.Profiles = append([]CodexOSProfilePolicy(nil), versionOnly.Profiles...)
	surfaceChange.Profiles[0].CanonicalSurface = CodexSurfaceCLI
	third, err := CodexIdentityPolicyRuntimeSHA256(surfaceChange)
	require.NoError(t, err)
	require.NotEqual(t, first, third)
}
