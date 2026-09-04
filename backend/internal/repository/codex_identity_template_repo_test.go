package repository

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestCodexIdentityTemplateUpdateRejectsStaleRevision(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &codexIdentityTemplateRepository{db: db}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT revision FROM codex_identity_templates WHERE id=$1 FOR UPDATE")).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(int64(4)))
	mock.ExpectRollback()

	_, err = repo.UpdateCodexIdentityTemplate(context.Background(), &service.CodexIdentityTemplate{ID: 9}, 3, false)
	require.Error(t, err)
	require.Equal(t, "CODEX_IDENTITY_TEMPLATE_REVISION_CONFLICT", infraerrors.Reason(err))
	require.Equal(t, "4", infraerrors.FromError(err).Metadata["current_revision"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCodexIdentityTemplateDeleteRejectsAssignedTemplate(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &codexIdentityTemplateRepository{db: db}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM accounts WHERE codex_identity_template_id=$1")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))
	mock.ExpectRollback()

	err = repo.DeleteCodexIdentityTemplate(context.Background(), 7)
	require.Error(t, err)
	require.Equal(t, "CODEX_IDENTITY_TEMPLATE_IN_USE", infraerrors.Reason(err))
	require.Equal(t, "2", infraerrors.FromError(err).Metadata["assigned_account_count"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCodexIdentityTemplateRuntimeUpdateRequiresAssignedAccountConfirmation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &codexIdentityTemplateRepository{db: db}
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT revision FROM codex_identity_templates WHERE id=$1 FOR UPDATE")).
		WithArgs(int64(12)).
		WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(int64(5)))
	mock.ExpectQuery("SELECT templates.id, templates.name, templates.description, templates.revision").
		WithArgs(int64(12)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "description", "revision", "session_policy", "affinity_ttl_seconds",
			"unsupported_policy", "created_at", "updated_at", "assigned_account_count",
		}).AddRow(
			int64(12), "shared", "", int64(5), []byte(`{"mode":"conversation_isolated"}`),
			3600, "reject", now, now, int64(2),
		))
	mock.ExpectQuery("FROM codex_identity_template_profiles").
		WithArgs(int64(12)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "os_class", "canonical_surface", "architecture", "proxy_mode", "proxy_id", "slot_count", "catalog_version",
		}))
	mock.ExpectRollback()

	_, err = repo.UpdateCodexIdentityTemplate(context.Background(), &service.CodexIdentityTemplate{
		ID: 12, Name: "shared", SessionPolicy: service.CodexSessionPolicySpec{Mode: service.CodexSessionConversationIsolated},
		AffinityTTLSeconds: 7200, UnsupportedPolicy: service.CodexUnsupportedProfileReject,
	}, 5, false)
	require.Error(t, err)
	require.Equal(t, "CODEX_IDENTITY_TEMPLATE_UPDATE_CONFIRMATION_REQUIRED", infraerrors.Reason(err))
	require.Equal(t, "2", infraerrors.FromError(err).Metadata["assigned_account_count"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPropagateCodexIdentityTemplateAdvancesAppliedRevisionWithoutRotatingUnchangedPolicy(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)

	template := &service.CodexIdentityTemplate{
		ID: 4, Revision: 7,
		SessionPolicy:      service.CodexSessionPolicySpec{Mode: service.CodexSessionConversationIsolated},
		AffinityTTLSeconds: 3600,
		UnsupportedPolicy:  service.CodexUnsupportedProfileReject,
		Profiles: []service.CodexIdentityTemplateProfile{{
			OSClass: service.CodexOSWindows, CanonicalSurface: service.CodexSurfaceDesktop,
			Architecture: service.CodexArchX8664, ProxyMode: service.CodexProxyInherit,
			SlotCount: 1, CatalogVersion: 1,
		}},
	}
	policy, err := service.MaterializeCodexIdentityTemplate(template)
	require.NoError(t, err)
	policy.Version = 3
	policy.Profiles[0].Epoch = 2
	encoded, err := service.EncodeCodexIdentityPolicy(policy)
	require.NoError(t, err)
	policyJSON, err := json.Marshal(encoded)
	require.NoError(t, err)

	mock.ExpectQuery("SELECT id, platform, type, codex_identity_policy").
		WithArgs(int64(4)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "platform", "type", "codex_identity_policy"}).
			AddRow(int64(19), service.PlatformOpenAI, service.AccountTypeOAuth, policyJSON))
	mock.ExpectExec("UPDATE accounts").
		WithArgs(sqlmock.AnyArg(), int64(7), int64(19), int64(4)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	accountIDs, err := propagateCodexIdentityTemplate(ctx, tx, template)
	require.NoError(t, err)
	require.Equal(t, []int64{19}, accountIDs)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTranslateCodexIdentityTemplateWriteErrorMapsNameConflictAndAssignmentFK(t *testing.T) {
	err := translateCodexIdentityTemplateWriteError(&pq.Error{
		Code:       "23505",
		Constraint: "idx_codex_identity_templates_name_ci",
	})
	require.Equal(t, "CODEX_IDENTITY_TEMPLATE_NAME_EXISTS", infraerrors.Reason(err))

	err = translateCodexIdentityTemplateWriteError(&pq.Error{
		Code:       "23503",
		Constraint: "accounts_codex_identity_template_fk",
	})
	require.Equal(t, "CODEX_IDENTITY_TEMPLATE_IN_USE", infraerrors.Reason(err))
}

func TestCodexIdentityTemplateRuntimeEqualityIgnoresPresentationMetadata(t *testing.T) {
	left := &service.CodexIdentityTemplate{
		ID: 1, Name: "old", Description: "old", Revision: 3,
		SessionPolicy:      service.CodexSessionPolicySpec{Mode: service.CodexSessionConversationIsolated},
		AffinityTTLSeconds: 3600, UnsupportedPolicy: service.CodexUnsupportedProfileReject,
		Profiles: []service.CodexIdentityTemplateProfile{{
			ID: 7, OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceCLI,
			Architecture: service.CodexArchX8664, ProxyMode: service.CodexProxyInherit,
			SlotCount: 1, CatalogVersion: 1,
		}},
	}
	right := *left
	right.Name = "renamed"
	right.Description = "new description"
	right.Profiles = append([]service.CodexIdentityTemplateProfile(nil), left.Profiles...)
	right.Profiles[0].ID = 99
	require.True(t, codexIdentityTemplateRuntimeEqual(left, &right))
	right.Profiles[0].SlotCount = 2
	require.False(t, codexIdentityTemplateRuntimeEqual(left, &right))
}

func TestCodexIdentityTemplateRuntimeEqualityTreatsSparseSlotsAsInheritedDefaults(t *testing.T) {
	left := &service.CodexIdentityTemplate{
		ID: 1, Revision: 2,
		SessionPolicy:      service.CodexSessionPolicySpec{Mode: service.CodexSessionConversationIsolated},
		AffinityTTLSeconds: 3600, UnsupportedPolicy: service.CodexUnsupportedProfileReject,
		Profiles: []service.CodexIdentityTemplateProfile{{
			OSClass: service.CodexOSWindows, CanonicalSurface: service.CodexSurfaceDesktop,
			Architecture: service.CodexArchX8664, ProxyMode: service.CodexProxyInherit,
			SlotCount: 2, CatalogVersion: 1,
		}},
	}
	right := *left
	right.Profiles = append([]service.CodexIdentityTemplateProfile(nil), left.Profiles...)
	right.Profiles[0].Slots = []service.CodexIdentityTemplateSlot{
		{Index: 0, ProxyMode: service.CodexProxyInherit, ClientVersionMode: service.CodexClientVersionInherit},
		{Index: 1, ProxyMode: service.CodexProxyInherit, ClientVersionMode: service.CodexClientVersionInherit},
	}
	require.True(t, codexIdentityTemplateRuntimeEqual(left, &right))
}

func TestCodexIdentityTemplateTransactionalProxyValidationRejectsMissingOrSoftDeletedProxy(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	proxyID := int64(41)
	template := &service.CodexIdentityTemplate{Profiles: []service.CodexIdentityTemplateProfile{{
		OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceCLI,
		Architecture: service.CodexArchX8664, ProxyMode: service.CodexProxyExplicit,
		ProxyID: &proxyID, SlotCount: 1, CatalogVersion: 1,
	}}}
	mock.ExpectQuery("SELECT id, status, expires_at").
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "expires_at"}))
	mock.ExpectRollback()
	err = validateCodexIdentityTemplateProxyReferences(context.Background(), tx, template)
	require.Error(t, err)
	require.Equal(t, "INVALID_CODEX_IDENTITY_TEMPLATE_PROXY", infraerrors.Reason(err))
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadCodexIdentityTemplateSlotsIncludesClientVersion(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery("SELECT id, slot_index, proxy_mode, proxy_id, client_version_mode, client_version").
		WithArgs(int64(7), int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "slot_index", "proxy_mode", "proxy_id", "client_version_mode", "client_version",
		}).AddRow(int64(19), 0, "inherit", nil, "pinned", "0.200.1"))

	profile := &service.CodexIdentityTemplateProfile{ID: 11}
	err = loadCodexIdentityTemplateSlots(context.Background(), db, 7, profile)
	require.NoError(t, err)
	require.Len(t, profile.Slots, 1)
	require.Equal(t, service.CodexClientVersionPinned, profile.Slots[0].ClientVersionMode)
	require.Equal(t, "0.200.1", profile.Slots[0].ClientVersion)
	require.NoError(t, mock.ExpectationsWereMet())
}
