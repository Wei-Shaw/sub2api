//go:build integration

package service

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/lib/pq"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestPostgresLatePaymentWaitsForExpiryAndRecoversOrder(t *testing.T) {
	ctx := context.Background()
	if err := exec.CommandContext(ctx, "docker", "info").Run(); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("docker is required in CI: %v", err)
		}
		t.Skip("docker is not available")
	}
	if _, configured := os.LookupEnv("DOCKER_HOST"); !configured {
		output, inspectErr := exec.CommandContext(ctx, "docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output()
		require.NoError(t, inspectErr)
		host := strings.TrimSpace(string(output))
		require.NotEmpty(t, host)
		t.Setenv("DOCKER_HOST", host)
		if host != "unix:///var/run/docker.sock" {
			t.Setenv("TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE", "/var/run/docker.sock")
		}
	}

	container, err := tcpostgres.Run(
		ctx,
		"postgres:18.1-alpine3.23",
		tcpostgres.WithDatabase("sub2api_payment_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(context.Background())) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable", "TimeZone=UTC")
	require.NoError(t, err)
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.Eventually(t, func() bool { return db.PingContext(ctx) == nil }, 20*time.Second, 100*time.Millisecond)

	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	require.NoError(t, client.Schema.Create(ctx))

	user, err := client.User.Create().
		SetEmail("late-payment-race@example.com").
		SetPasswordHash("hash").
		SetUsername("late-payment-race").
		Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(2).
		SetPayAmount(2).
		SetRechargeCode("LATE-PAYMENT-RACE").
		SetOutTradeNo("sub2_late_payment_race").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(-time.Minute)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	expiryTx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = expiryTx.Rollback() })
	result, err := expiryTx.ExecContext(ctx, `
		UPDATE payment_orders
		SET status = 'EXPIRED', updated_at = NOW()
		WHERE id = $1 AND status = 'PENDING'
	`, order.ID)
	require.NoError(t, err)
	rows, err := result.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	type transitionResult struct {
		updated int
		err     error
	}
	started := make(chan struct{})
	finished := make(chan transitionResult, 1)
	go func() {
		close(started)
		updated, transitionErr := (&PaymentService{entClient: client}).transitionOrderToPaid(
			context.Background(),
			order,
			"provider-trade-late",
			2,
			payment.TypeAlipay,
			time.Now().UTC(),
			nil,
		)
		finished <- transitionResult{updated: updated, err: transitionErr}
	}()
	<-started
	select {
	case result := <-finished:
		t.Fatalf("payment transition returned before the conflicting expiry transaction committed: %+v", result)
	case <-time.After(250 * time.Millisecond):
	}

	require.NoError(t, expiryTx.Commit())
	select {
	case transition := <-finished:
		require.NoError(t, transition.err)
		require.Equal(t, 1, transition.updated)
	case <-time.After(5 * time.Second):
		t.Fatal("payment transition remained blocked after expiry committed")
	}

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPaid, reloaded.Status)
	require.Equal(t, "provider-trade-late", reloaded.PaymentTradeNo)
	require.NotNil(t, reloaded.PaidAt)
}
