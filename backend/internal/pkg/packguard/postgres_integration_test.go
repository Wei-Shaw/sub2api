//go:build integration

package packguard

import (
	"context"
	"fmt"
	"os"
	"testing"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestMain(m *testing.M) {
	code := func() int {
		if os.Getenv("PACK_GUARD_TEST_DSN") != "" {
			return m.Run()
		}
		ctx := context.Background()
		pg, err := tcpostgres.Run(ctx, "postgres:18-alpine", tcpostgres.WithDatabase("packguard"), tcpostgres.WithUsername("packguard"), tcpostgres.WithPassword("test-only-not-a-provider-credential"), tcpostgres.BasicWaitStrategies())
		if err != nil {
			fmt.Fprintln(os.Stderr, "prepaid PostgreSQL fixture could not start:", err)
			return 1
		}
		defer func() { _ = pg.Terminate(ctx) }()
		dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			fmt.Fprintln(os.Stderr, "prepaid PostgreSQL fixture address unavailable")
			return 1
		}
		if err = os.Setenv("PACK_GUARD_TEST_DSN", dsn); err != nil {
			return 1
		}
		return m.Run()
	}()
	os.Exit(code)
}
