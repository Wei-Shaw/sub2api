package repository

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogAPIReferenceCostRoundTrip(t *testing.T) {
	for _, hasReference := range []bool{false, true} {
		t.Run(map[bool]string{false: "historical NULL", true: "known zero with pricing evidence"}[hasReference], func(t *testing.T) {
			db, mock := newSQLMock(t)
			repo := &usageLogRepository{sql: db}
			at := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
			log := &service.UsageLog{UserID: 1, APIKeyID: 2, AccountID: 3, RequestID: "reference-round-trip", Model: "gpt-5.6-terra", CreatedAt: at}
			if hasReference {
				zero := 0.0
				log.APIReferenceCost = &zero
				log.APIReferencePricing = &service.APIReferencePricingSnapshot{SchemaVersion: 1, Model: log.Model, Source: "model_catalog", PricingAt: at}
			}
			prepared := prepareUsageLogInsert(log)
			require.Len(t, prepared.args, len(usageLogInsertArgTypes))
			mock.ExpectQuery("INSERT INTO usage_logs").WithArgs(anySliceToDriverValues(prepared.args)...).
				WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(4), at))
			inserted, err := repo.Create(context.Background(), log)
			require.NoError(t, err)
			require.True(t, inserted)
			values := []driver.Value{int64(4)}
			for _, arg := range prepared.args {
				value, err := driver.DefaultParameterConverter.ConvertValue(arg)
				require.NoError(t, err)
				values = append(values, value)
			}
			mock.ExpectQuery("SELECT .* FROM usage_logs WHERE id").WithArgs(int64(4)).
				WillReturnRows(sqlmock.NewRows(strings.Split(usageLogSelectColumns, ", ")).AddRow(values...))
			got, err := repo.GetByID(context.Background(), 4)
			require.NoError(t, err)
			require.Equal(t, log.APIReferenceCost, got.APIReferenceCost)
			require.Equal(t, log.APIReferencePricing, got.APIReferencePricing)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
