package backend

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/config"
)

func TestSQLProxyResolverResolvesActiveProxy(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	updatedAt := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, protocol, host, port,")).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "protocol", "host", "port", "username", "password", "updated_at"}).
			AddRow(42, "http", "proxy.internal", 8080, "alice", "secret", updatedAt))

	resolved, err := NewSQLProxyResolver(db).Resolve(context.Background(), 42)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.ID != 42 || resolved.Protocol != "http" || resolved.Host != "proxy.internal" || resolved.Port != 8080 {
		t.Fatalf("resolved = %#v", resolved)
	}
	if resolved.Username != "alice" || resolved.Password != "secret" || !resolved.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("resolved credentials/version = %#v", resolved)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLProxyResolverRejectsMissingInactiveOrExpiredProxy(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, protocol, host, port,")).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "protocol", "host", "port", "username", "password", "updated_at"}))

	_, err = NewSQLProxyResolver(db).Resolve(context.Background(), 42)
	if !errors.Is(err, ErrProxyNotAvailable) {
		t.Fatalf("Resolve() error = %v, want ErrProxyNotAvailable", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUnavailableProxyResolverRejectsProxyLookup(t *testing.T) {
	resolver, err := NewProxyResolver(config.DatabaseConfig{})
	if err != nil {
		t.Fatalf("NewProxyResolver() error = %v", err)
	}
	_, err = resolver.Resolve(context.Background(), 42)
	if !errors.Is(err, ErrProxyStorageUnavailable) {
		t.Fatalf("Resolve() error = %v, want ErrProxyStorageUnavailable", err)
	}
}
