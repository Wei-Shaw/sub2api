//go:build unit

package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type movingMaterialRepo struct {
	UserMaterialRepository
	inserted  *UserMaterial
	insertErr error
}

func (r *movingMaterialRepo) UsageByUser(context.Context, int64) (int64, int64, error) {
	return 0, 0, nil
}

func (r *movingMaterialRepo) Insert(_ context.Context, material *UserMaterial) (int64, error) {
	clone := *material
	r.inserted = &clone
	if r.insertErr != nil {
		return 0, r.insertErr
	}
	material.ID = 19
	return material.ID, nil
}

func newMovingMaterialService(cos *COSImageTransferService, repo *movingMaterialRepo) *UserMaterialService {
	cfg := &config.Config{}
	cfg.JWT.Secret = "developer-file-test-secret"
	users := &materialPathUserRepo{user: &User{ID: 7, AccountID: "1000000000000001", Status: StatusActive}}
	return NewUserMaterialService(repo, cos, users, cfg)
}

func TestAddMaterialByURLMovesOwnedDeveloperUpload(t *testing.T) {
	files, cos, store := newDeveloperFileServiceTest(t)
	temporary, err := files.Upload(t.Context(), 7, "reference.png", "image/png", 1, strings.NewReader("x"))
	require.NoError(t, err)
	sourceKey := store.uploadKey
	repo := &movingMaterialRepo{}

	material, err := newMovingMaterialService(cos, repo).AddMaterialByURL(t.Context(), 7, temporary.URL)
	require.NoError(t, err)
	require.NotEmpty(t, material.CosKey)
	require.Contains(t, material.CosKey, "/materials/")
	require.Equal(t, "https://cdn.example.com/storage/"+material.CosKey, material.CosURL)
	require.Equal(t, [][2]string{{sourceKey, material.CosKey}}, store.copies)
	require.Equal(t, []string{sourceKey}, store.deleted)
	require.Equal(t, material.CosKey, repo.inserted.CosKey)
	require.Equal(t, material.CosURL, repo.inserted.CosURL)
}

func TestAddMaterialByURLRejectsForeignDeveloperUpload(t *testing.T) {
	_, cos, store := newDeveloperFileServiceTest(t)
	repo := &movingMaterialRepo{}
	foreignURL := "https://cdn.example.com/storage/assets/file_uploads/u_foreign/2026/01/a.png"

	_, err := newMovingMaterialService(cos, repo).AddMaterialByURL(t.Context(), 7, foreignURL)
	require.ErrorIs(t, err, ErrDeveloperFileForbidden)
	require.Nil(t, repo.inserted)
	require.Empty(t, store.copies)
	require.Empty(t, store.deleted)
}

func TestAddMaterialByURLKeepsNonTemporaryCOSURL(t *testing.T) {
	_, cos, store := newDeveloperFileServiceTest(t)
	repo := &movingMaterialRepo{}
	rawURL := "https://cdn.example.com/storage/assets/existing/reference.png"

	material, err := newMovingMaterialService(cos, repo).AddMaterialByURL(t.Context(), 7, rawURL)
	require.NoError(t, err)
	require.Equal(t, rawURL, material.CosURL)
	require.Empty(t, material.CosKey)
	require.Empty(t, store.copies)
	require.Empty(t, store.deleted)
}

func TestAddMaterialByURLRollsBackMoveWhenInsertFails(t *testing.T) {
	files, cos, store := newDeveloperFileServiceTest(t)
	temporary, err := files.Upload(t.Context(), 7, "reference.png", "image/png", 1, strings.NewReader("x"))
	require.NoError(t, err)
	sourceKey := store.uploadKey
	wantErr := errors.New("insert failed")
	repo := &movingMaterialRepo{insertErr: wantErr}

	_, err = newMovingMaterialService(cos, repo).AddMaterialByURL(t.Context(), 7, temporary.URL)
	require.ErrorIs(t, err, wantErr)
	require.Len(t, store.copies, 2)
	destinationKey := store.copies[0][1]
	require.Equal(t, [2]string{sourceKey, destinationKey}, store.copies[0])
	require.Equal(t, [2]string{destinationKey, sourceKey}, store.copies[1])
	require.Equal(t, []string{sourceKey, destinationKey}, store.deleted)
}
