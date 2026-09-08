package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestVerifyCodexIdentityTemplateAssignmentRejectsStaleRevision(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	templateID, appliedRevision := int64(8), int64(3)
	account := &service.Account{
		CodexIdentityTemplateID:              &templateID,
		CodexIdentityTemplateAppliedRevision: &appliedRevision,
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT revision FROM codex_identity_templates WHERE id=$1 FOR SHARE")).
		WithArgs(templateID).
		WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(int64(4)))

	err = verifyCodexIdentityTemplateAssignment(context.Background(), db, account)
	require.Error(t, err)
	require.Equal(t, "CODEX_IDENTITY_TEMPLATE_REVISION_CONFLICT", infraerrors.Reason(err))
	require.Equal(t, "4", infraerrors.FromError(err).Metadata["current_revision"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyCodexIdentityTemplateAssignmentLocksMatchingRevision(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	templateID, appliedRevision := int64(8), int64(4)
	account := &service.Account{
		CodexIdentityTemplateID:              &templateID,
		CodexIdentityTemplateAppliedRevision: &appliedRevision,
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT revision FROM codex_identity_templates WHERE id=$1 FOR SHARE")).
		WithArgs(templateID).
		WillReturnRows(sqlmock.NewRows([]string{"revision"}).AddRow(appliedRevision))

	require.NoError(t, verifyCodexIdentityTemplateAssignment(context.Background(), db, account))
	require.NoError(t, mock.ExpectationsWereMet())
}
