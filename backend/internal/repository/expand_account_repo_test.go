package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCreateExpandAccountReusesExistingProxy(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &expandAccountRepository{db: db}

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	input := &service.ExpandAccountCreateInput{
		Email:            "test@example.com",
		Platform:         "openai",
		SubscriptionType: "pro",
		Country:          "US",
		SessionKey:       "test-session-key-001",
		ProxyInfo: &service.ProxyInfo{
			Protocol: "socks5",
			Host:     "154.63.48.107",
			Port:     7778,
			Username: "a3p3p1Q0o5j8",
			Password: "m5N7v5T9s9h4",
		},
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"pg_advisory_xact_lock"}).AddRow(nil))
	mock.ExpectQuery("SELECT id\\s+FROM proxies").
		WithArgs("socks5", "154.63.48.107", 7778, "a3p3p1Q0o5j8", "m5N7v5T9s9h4").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(15)))
	mock.ExpectQuery("INSERT INTO expand_accounts").
		WithArgs(
			input.Email,
			input.Platform,
			input.SubscriptionType,
			input.Country,
			input.SessionKey,
			int64(15),
			sqlmock.AnyArg(),
			input.Used,
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "email", "platform", "subscription_type", "country", "session_key",
			"proxy_id", "proxy_info", "used", "account_id", "login_status", "device_id", "api_key", "created_at", "updated_at",
		}).AddRow(
			int64(9),
			input.Email,
			input.Platform,
			input.SubscriptionType,
			input.Country,
			input.SessionKey,
			int64(15),
			[]byte(`{"protocol":"socks5","host":"154.63.48.107","port":7778,"username":"a3p3p1Q0o5j8","password":"m5N7v5T9s9h4"}`),
			false,
			nil,
			int64(0),
			nil,
			nil,
			now,
			now,
		))
	mock.ExpectCommit()

	item, err := repo.CreateExpandAccount(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, item.ProxyID)
	require.Equal(t, int64(15), *item.ProxyID)
	require.NotNil(t, item.ProxyInfo)
	require.Equal(t, "socks5", item.ProxyInfo.Protocol)
	require.Equal(t, "154.63.48.107", item.ProxyInfo.Host)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateExpandAccountCreatesProxyWhenMissing(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &expandAccountRepository{db: db}

	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	input := &service.ExpandAccountCreateInput{
		Email:            "test@example.com",
		Platform:         "openai",
		SubscriptionType: "pro",
		Country:          "US",
		SessionKey:       "test-session-key-001",
		ProxyInfo: &service.ProxyInfo{
			Protocol: "socks5",
			Host:     "154.63.48.107",
			Port:     7778,
			Username: "a3p3p1Q0o5j8",
			Password: "m5N7v5T9s9h4",
		},
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"pg_advisory_xact_lock"}).AddRow(nil))
	mock.ExpectQuery("SELECT id\\s+FROM proxies").
		WithArgs("socks5", "154.63.48.107", 7778, "a3p3p1Q0o5j8", "m5N7v5T9s9h4").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("INSERT INTO proxies").
		WithArgs(
			"socks5://154.63.48.107:7778#a3p3p1Q0o5j8",
			"socks5",
			"154.63.48.107",
			7778,
			"a3p3p1Q0o5j8",
			"m5N7v5T9s9h4",
			service.StatusActive,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(31)))
	mock.ExpectQuery("INSERT INTO expand_accounts").
		WithArgs(
			input.Email,
			input.Platform,
			input.SubscriptionType,
			input.Country,
			input.SessionKey,
			int64(31),
			sqlmock.AnyArg(),
			input.Used,
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "email", "platform", "subscription_type", "country", "session_key",
			"proxy_id", "proxy_info", "used", "account_id", "login_status", "device_id", "api_key", "created_at", "updated_at",
		}).AddRow(
			int64(10),
			input.Email,
			input.Platform,
			input.SubscriptionType,
			input.Country,
			input.SessionKey,
			int64(31),
			[]byte(`{"protocol":"socks5","host":"154.63.48.107","port":7778,"username":"a3p3p1Q0o5j8","password":"m5N7v5T9s9h4"}`),
			false,
			nil,
			int64(0),
			nil,
			nil,
			now,
			now,
		))
	mock.ExpectCommit()

	item, err := repo.CreateExpandAccount(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, item.ProxyID)
	require.Equal(t, int64(31), *item.ProxyID)
	require.NoError(t, mock.ExpectationsWereMet())
}
