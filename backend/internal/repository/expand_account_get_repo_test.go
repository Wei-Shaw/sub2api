package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestGetAndMarkExpandAccountByPlatform(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &expandAccountRepository{db: db}

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery("WITH picked AS").
		WithArgs("Anthropic").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "email", "platform", "subscription_type", "country", "session_key",
			"proxy_id", "proxy_info", "used", "created_at", "updated_at",
		}).AddRow(
			int64(3),
			"test1@example.com",
			"openai",
			"pro",
			"US",
			"test-session-key-001",
			int64(40),
			[]byte(`{"protocol":"socks5","host":"154.63.48.107","port":7778,"username":"a3p3p1Q0o5j8","password":"m5N7v5T9s9h4"}`),
			true,
			now,
			now,
		))

	item, err := repo.GetAndMarkExpandAccountByPlatform(context.Background(), "Anthropic")
	require.NoError(t, err)
	require.Equal(t, int64(3), item.ID)
	require.True(t, item.Used)
	require.NotNil(t, item.ProxyID)
	require.Equal(t, int64(40), *item.ProxyID)
	require.NotNil(t, item.ProxyInfo)
	require.Equal(t, "socks5", item.ProxyInfo.Protocol)
	require.NoError(t, mock.ExpectationsWereMet())
}
