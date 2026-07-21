package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountRepositoryStoresAndReadsUpstreamSiteLogoCache(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := newAccountRepositoryWithSQL(nil, db, nil)
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	logo := &service.UpstreamSiteLogo{ContentType: "image/png", Data: []byte("\x89PNG\r\n\x1a\ncustom")}

	mock.ExpectExec("INSERT INTO upstream_site_logos").
		WithArgs(key, logo.ContentType, logo.Data).
		WillReturnResult(sqlmock.NewResult(1, 1))
	require.NoError(t, repo.PutUpstreamSiteLogoCache(context.Background(), key, logo))

	mock.ExpectQuery("SELECT content_type, content").
		WithArgs(key).
		WillReturnRows(sqlmock.NewRows([]string{"content_type", "content"}).AddRow(logo.ContentType, logo.Data))
	stored, found, err := repo.GetUpstreamSiteLogoCache(context.Background(), key)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, logo, stored)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountRepositoryReadsNegativeUpstreamSiteLogoCache(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := newAccountRepositoryWithSQL(nil, db, nil)
	key := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

	mock.ExpectQuery("SELECT content_type, content").
		WithArgs(key).
		WillReturnRows(sqlmock.NewRows([]string{"content_type", "content"}).AddRow(nil, nil))
	stored, found, err := repo.GetUpstreamSiteLogoCache(context.Background(), key)
	require.NoError(t, err)
	require.True(t, found)
	require.Nil(t, stored)
	require.NoError(t, mock.ExpectationsWereMet())
}
