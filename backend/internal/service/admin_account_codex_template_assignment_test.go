package service

import (
	"context"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type codexAssignmentAccountRepoStub struct {
	AccountRepository
	template *CodexIdentityTemplate
	err      error
}

type codexAssignmentNameFallbackRepoStub struct {
	AccountRepository
	byID, byName *CodexIdentityTemplate
}

func (r *codexAssignmentNameFallbackRepoStub) GetCodexIdentityTemplate(context.Context, int64) (*CodexIdentityTemplate, error) {
	return r.byID, nil
}

func (r *codexAssignmentNameFallbackRepoStub) GetCodexIdentityTemplateByName(context.Context, string) (*CodexIdentityTemplate, error) {
	return r.byName, nil
}

func (r *codexAssignmentAccountRepoStub) GetCodexIdentityTemplate(context.Context, int64) (*CodexIdentityTemplate, error) {
	return r.template, r.err
}

func (r *codexAssignmentAccountRepoStub) GetCodexIdentityTemplateByName(context.Context, string) (*CodexIdentityTemplate, error) {
	return r.template, r.err
}

func TestMaterializeCodexIdentityAssignment(t *testing.T) {
	template := &CodexIdentityTemplate{
		ID: 17, Revision: 4,
		SessionPolicy:      CodexSessionPolicySpec{Mode: CodexSessionConversationIsolated},
		AffinityTTLSeconds: 3600,
		UnsupportedPolicy:  CodexUnsupportedProfileReject,
		Profiles: []CodexIdentityTemplateProfile{{
			OSClass: CodexOSWindows, CanonicalSurface: CodexSurfaceDesktop,
			Architecture: CodexArchX8664, ProxyMode: CodexProxyInherit,
			SlotCount: 1, CatalogVersion: 1,
		}},
	}
	svc := &adminServiceImpl{accountRepo: &codexAssignmentAccountRepoStub{template: template}}

	policy, templateID, revision, err := svc.materializeCodexIdentityAssignment(t.Context(), &CodexIdentityAssignment{
		Enabled: true, TemplateID: 17,
	})
	require.NoError(t, err)
	require.Equal(t, CodexIdentityPolicyOSProfileDevicePool, policy.Mode)
	require.Equal(t, CodexIdentityBindingAPIKeyOSSurface, policy.BindingScope)
	require.Equal(t, int64(17), *templateID)
	require.Equal(t, int64(4), *revision)

	policy, templateID, revision, err = svc.materializeCodexIdentityAssignment(t.Context(), &CodexIdentityAssignment{Enabled: false})
	require.NoError(t, err)
	require.Equal(t, CodexIdentityPolicyOff, policy.Mode)
	require.Nil(t, templateID)
	require.Nil(t, revision)
}

func TestMaterializeCodexIdentityAssignmentRejectsMissingTemplateID(t *testing.T) {
	svc := &adminServiceImpl{accountRepo: &codexAssignmentAccountRepoStub{}}
	_, _, _, err := svc.materializeCodexIdentityAssignment(t.Context(), &CodexIdentityAssignment{Enabled: true})
	require.Error(t, err)
	require.Equal(t, "INVALID_CODEX_IDENTITY_ASSIGNMENT", infraerrors.Reason(err))
}

func TestMaterializeCodexIdentityAssignmentRejectsExportedStaleRevision(t *testing.T) {
	template := &CodexIdentityTemplate{
		ID: 17, Revision: 5,
		SessionPolicy:      CodexSessionPolicySpec{Mode: CodexSessionConversationIsolated},
		AffinityTTLSeconds: 3600, UnsupportedPolicy: CodexUnsupportedProfileReject,
		Profiles: []CodexIdentityTemplateProfile{{
			OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceCLI,
			Architecture: CodexArchX8664, ProxyMode: CodexProxyInherit,
			SlotCount: 1, CatalogVersion: 1,
		}},
	}
	svc := &adminServiceImpl{accountRepo: &codexAssignmentAccountRepoStub{template: template}}
	expectedRevision := int64(4)
	_, _, _, err := svc.materializeCodexIdentityAssignment(t.Context(), &CodexIdentityAssignment{
		Enabled: true, TemplateID: 17, ExpectedRevision: &expectedRevision,
	})
	require.Error(t, err)
	require.Equal(t, "CODEX_IDENTITY_TEMPLATE_REVISION_CONFLICT", infraerrors.Reason(err))
}

func TestMaterializeCodexIdentityAssignmentRemapsExportByNameAndRuntimeHash(t *testing.T) {
	profile := CodexIdentityTemplateProfile{
		OSClass: CodexOSLinux, CanonicalSurface: CodexSurfaceCLI,
		Architecture: CodexArchX8664, ProxyMode: CodexProxyInherit,
		SlotCount: 1, CatalogVersion: 1,
	}
	correct := &CodexIdentityTemplate{
		ID: 29, Name: "Linux CLI", Revision: 8,
		SessionPolicy:      CodexSessionPolicySpec{Mode: CodexSessionConversationIsolated},
		AffinityTTLSeconds: 3600, UnsupportedPolicy: CodexUnsupportedProfileReject,
		Profiles: []CodexIdentityTemplateProfile{profile},
	}
	policy, err := MaterializeCodexIdentityTemplate(correct)
	require.NoError(t, err)
	digest, err := CodexIdentityPolicyRuntimeSHA256(policy)
	require.NoError(t, err)
	wrong := *correct
	wrong.ID = 12
	wrong.Name = "Different template"
	wrong.Profiles = append([]CodexIdentityTemplateProfile(nil), correct.Profiles...)
	wrong.Profiles[0].CanonicalSurface = CodexSurfaceDesktop
	svc := &adminServiceImpl{accountRepo: &codexAssignmentNameFallbackRepoStub{byID: &wrong, byName: correct}}
	sourceRevision := int64(1)
	_, templateID, revision, err := svc.materializeCodexIdentityAssignment(t.Context(), &CodexIdentityAssignment{
		Enabled: true, TemplateID: 12, ExpectedRevision: &sourceRevision,
		ExpectedTemplateName: "Linux CLI", ExpectedRuntimeSHA256: digest,
	})
	require.NoError(t, err)
	require.Equal(t, int64(29), *templateID)
	require.Equal(t, int64(8), *revision)
}
