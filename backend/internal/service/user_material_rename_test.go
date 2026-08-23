//go:build unit

package service

import (
	"context"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type renameMaterialRepo struct {
	UserMaterialRepository
	calls    int
	userID   int64
	publicID string
	fileName string
	result   *UserMaterial
}

func (r *renameMaterialRepo) UpdateFileNameByID(_ context.Context, userID, id int64, fileName string) (*UserMaterial, error) {
	r.calls++
	r.userID = userID
	r.fileName = fileName
	if r.result != nil {
		r.result.ID = id
	}
	return r.result, nil
}

func (r *renameMaterialRepo) UpdateFileNameByPublicID(_ context.Context, userID int64, publicID, fileName string) (*UserMaterial, error) {
	r.calls++
	r.userID = userID
	r.publicID = publicID
	r.fileName = fileName
	return r.result, nil
}

func TestRenameMaterialByPublicIDValidatesAndScopesUpdate(t *testing.T) {
	publicID := "550e8400-e29b-41d4-a716-446655440000"
	repo := &renameMaterialRepo{result: &UserMaterial{PublicID: publicID, UserID: 7, FileName: "renamed.png"}}
	service := NewUserMaterialService(repo, nil, nil, nil)

	material, err := service.RenameByPublicID(context.Background(), 7, "  "+publicID+"  ", "  renamed.png  ")
	require.NoError(t, err)
	require.Equal(t, "renamed.png", material.FileName)
	require.Equal(t, int64(7), repo.userID)
	require.Equal(t, publicID, repo.publicID)
	require.Equal(t, "renamed.png", repo.fileName)
}

func TestRenameMaterialByPublicIDRejectsInvalidInput(t *testing.T) {
	publicID := "550e8400-e29b-41d4-a716-446655440000"
	tests := []struct {
		name     string
		userID   int64
		publicID string
		fileName string
		reason   string
	}{
		{name: "invalid user", userID: 0, publicID: publicID, fileName: "name.png", reason: "INVALID_USER"},
		{name: "invalid id", userID: 7, publicID: "not-a-uuid", fileName: "name.png", reason: "INVALID_ID"},
		{name: "empty name", userID: 7, publicID: publicID, fileName: "  ", reason: "INVALID_FILE_NAME"},
		{name: "name too long", userID: 7, publicID: publicID, fileName: strings.Repeat("名", MaterialFileNameMaxRunes+1), reason: "INVALID_FILE_NAME"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &renameMaterialRepo{}
			_, err := NewUserMaterialService(repo, nil, nil, nil).RenameByPublicID(
				context.Background(), test.userID, test.publicID, test.fileName,
			)
			require.Error(t, err)
			require.Equal(t, test.reason, infraerrors.Reason(err))
			require.Zero(t, repo.calls)
		})
	}
}

func TestRenameMaterialByPublicIDReturnsNotFoundForForeignOrMissingMaterial(t *testing.T) {
	publicID := "550e8400-e29b-41d4-a716-446655440000"
	_, err := NewUserMaterialService(&renameMaterialRepo{}, nil, nil, nil).
		RenameByPublicID(context.Background(), 7, publicID, "name.png")
	require.Error(t, err)
	require.Equal(t, "MATERIAL_NOT_FOUND", infraerrors.Reason(err))
}

func TestRenameMaterialByIDValidatesAndScopesUpdate(t *testing.T) {
	repo := &renameMaterialRepo{result: &UserMaterial{UserID: 7, FileName: "renamed.png"}}
	material, err := NewUserMaterialService(repo, nil, nil, nil).
		Rename(context.Background(), 7, 42, "  renamed.png  ")
	require.NoError(t, err)
	require.Equal(t, int64(42), material.ID)
	require.Equal(t, "renamed.png", material.FileName)
	require.Equal(t, int64(7), repo.userID)
	require.Equal(t, "renamed.png", repo.fileName)
}

func TestRenameMaterialByIDRejectsInvalidInputAndMissingMaterial(t *testing.T) {
	tests := []struct {
		name     string
		userID   int64
		id       int64
		fileName string
		reason   string
	}{
		{name: "invalid user", userID: 0, id: 42, fileName: "name.png", reason: "INVALID_USER"},
		{name: "invalid id", userID: 7, id: 0, fileName: "name.png", reason: "INVALID_ID"},
		{name: "empty name", userID: 7, id: 42, fileName: "  ", reason: "INVALID_FILE_NAME"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &renameMaterialRepo{}
			_, err := NewUserMaterialService(repo, nil, nil, nil).
				Rename(context.Background(), test.userID, test.id, test.fileName)
			require.Error(t, err)
			require.Equal(t, test.reason, infraerrors.Reason(err))
			require.Zero(t, repo.calls)
		})
	}

	_, err := NewUserMaterialService(&renameMaterialRepo{}, nil, nil, nil).
		Rename(context.Background(), 7, 42, "name.png")
	require.Error(t, err)
	require.Equal(t, "MATERIAL_NOT_FOUND", infraerrors.Reason(err))
}
