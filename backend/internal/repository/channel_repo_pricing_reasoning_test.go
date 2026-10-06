//go:build unit

package repository

import (
	"context"
	"database/sql/driver"
	"math"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

type capturedReasoningMultipliersJSON struct {
	value string
}

func (c *capturedReasoningMultipliersJSON) Match(value driver.Value) bool {
	data, ok := value.(string)
	if ok {
		c.value = data
	}
	return ok
}

func reasoningPricingRow(multipliers any) *sqlmock.Rows {
	return sqlmock.NewRows(channelModelPricingTimePricingColumns).AddRow(
		int64(11), int64(7), "openai", `["custom-model"]`, service.BillingModeToken,
		nil, nil, nil, nil, nil, nil, nil, multipliers, nil, nil, nil, nil,
		time.Time{}, time.Time{},
	)
}

func TestChannelReasoningEffortMultipliersRoundTrip(t *testing.T) {
	tests := []struct {
		name        string
		multipliers map[string]float64
		wantJSON    string
	}{
		{"custom levels", map[string]float64{"low": 0.75, "high": 1.5, "max": 4}, `{"low":0.75,"high":1.5,"max":4}`},
		{"nil clears configuration", nil, `{}`},
		{"empty clears configuration", map[string]float64{}, `{}`},
	}
	for _, tt := range tests {
		for _, operation := range []string{"create", "update", "replace"} {
			t.Run(tt.name+"/"+operation, func(t *testing.T) {
				repo, mock := newChannelModelPricingTimePricingRepo(t)
				ctx := context.Background()
				pricing := &service.ChannelModelPricing{
					ID: 11, ChannelID: 7, Platform: "openai", Models: []string{"custom-model"},
					ReasoningEffortMultipliers: tt.multipliers,
				}
				stored := &capturedReasoningMultipliersJSON{}
				if operation == "update" {
					mock.ExpectExec(`(?s)UPDATE channel_model_pricing.*reasoning_effort_multipliers = \$10.*WHERE id = \$16`).
						WithArgs([]byte(`["custom-model"]`), service.BillingModeToken,
							nil, nil, nil, nil, nil, nil, nil, stored, nil, nil, nil, nil, "openai", int64(11)).
						WillReturnResult(sqlmock.NewResult(0, 1))
					require.NoError(t, repo.UpdateModelPricing(ctx, pricing))
				} else {
					if operation == "replace" {
						mock.ExpectBegin()
						mock.ExpectExec(`DELETE FROM channel_model_pricing WHERE channel_id = \$1`).
							WithArgs(int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
					}
					mock.ExpectQuery(`INSERT INTO channel_model_pricing .*reasoning_effort_multipliers`).
						WithArgs(int64(7), "openai", []byte(`["custom-model"]`), service.BillingModeToken,
							nil, nil, nil, nil, nil, nil, nil, stored, nil, nil, nil, nil).
						WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(int64(11), time.Time{}, time.Time{}))
					if operation == "replace" {
						mock.ExpectCommit()
						require.NoError(t, repo.ReplaceModelPricing(ctx, 7, []service.ChannelModelPricing{*pricing}))
					} else {
						require.NoError(t, repo.CreateModelPricing(ctx, pricing))
					}
				}
				require.JSONEq(t, tt.wantJSON, stored.value)

				mock.ExpectQuery(`(?s)SELECT .*reasoning_effort_multipliers.*FROM channel_model_pricing.*channel_id = \$1`).
					WithArgs(int64(7)).WillReturnRows(reasoningPricingRow(stored.value))
				expectEmptyModelPricingIntervals(mock)
				loaded, err := repo.ListModelPricing(ctx, 7)
				require.NoError(t, err)
				require.Len(t, loaded, 1)
				if len(tt.multipliers) == 0 {
					require.Empty(t, loaded[0].ReasoningEffortMultipliers)
				} else {
					require.Equal(t, tt.multipliers, loaded[0].ReasoningEffortMultipliers)
				}
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}

func TestChannelReasoningEffortMultipliersBatchLoad(t *testing.T) {
	repo, mock := newChannelModelPricingTimePricingRepo(t)
	mock.ExpectQuery(`(?s)SELECT .*reasoning_effort_multipliers.*FROM channel_model_pricing.*channel_id = ANY`).
		WithArgs(pq.Array([]int64{7})).WillReturnRows(reasoningPricingRow(`{"medium":1.25,"high":2}`))
	expectEmptyModelPricingIntervals(mock)
	pricing, err := repo.batchLoadModelPricing(context.Background(), []int64{7})
	require.NoError(t, err)
	require.Len(t, pricing[7], 1)
	require.Equal(t, map[string]float64{"medium": 1.25, "high": 2}, pricing[7][0].ReasoningEffortMultipliers)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestChannelReasoningEffortMultipliersInvalidJSON(t *testing.T) {
	for _, data := range []string{`{"high":`, `{"high":"2"}`, `[]`} {
		t.Run(data, func(t *testing.T) {
			repo, mock := newChannelModelPricingTimePricingRepo(t)
			mock.ExpectQuery(`SELECT .*reasoning_effort_multipliers`).WithArgs(int64(7)).WillReturnRows(reasoningPricingRow(data))
			_, err := repo.ListModelPricing(context.Background(), 7)
			require.ErrorContains(t, err, "unmarshal reasoning effort multipliers")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
	for _, operation := range []string{"create", "update"} {
		t.Run(operation+" invalid number", func(t *testing.T) {
			repo, mock := newChannelModelPricingTimePricingRepo(t)
			pricing := &service.ChannelModelPricing{ReasoningEffortMultipliers: map[string]float64{"max": math.NaN()}}
			var err error
			if operation == "create" {
				err = repo.CreateModelPricing(context.Background(), pricing)
			} else {
				err = repo.UpdateModelPricing(context.Background(), pricing)
			}
			require.ErrorContains(t, err, "marshal reasoning effort multipliers")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAccountStatsReasoningEffortMultipliersRoundTrip(t *testing.T) {
	repo, mock := newChannelModelPricingTimePricingRepo(t)
	ctx := context.Background()
	pricing := &service.ChannelModelPricing{
		Platform: "anthropic", Models: []string{"custom-model"},
		ReasoningEffortMultipliers: map[string]float64{"medium": 0.5, "high": 2},
	}
	stored := &capturedReasoningMultipliersJSON{}
	mock.ExpectBegin()
	tx, err := repo.db.BeginTx(ctx, nil)
	require.NoError(t, err)
	mock.ExpectQuery(`INSERT INTO channel_account_stats_model_pricing .*reasoning_effort_multipliers`).
		WithArgs(int64(9), "anthropic", []byte(`["custom-model"]`), service.BillingModeToken,
			nil, nil, nil, nil, nil, stored, nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(int64(11), time.Time{}, time.Time{}))
	require.NoError(t, createAccountStatsModelPricingTx(ctx, tx, 9, pricing))
	mock.ExpectCommit()
	require.NoError(t, tx.Commit())
	require.JSONEq(t, `{"medium":0.5,"high":2}`, stored.value)

	columns := []string{
		"id", "rule_id", "platform", "models", "billing_mode", "input_price", "output_price",
		"cache_write_price", "cache_write_1h_price", "cache_read_price", "reasoning_effort_multipliers",
		"image_output_price", "per_request_price", "created_at", "updated_at",
	}
	mock.ExpectQuery(`(?s)SELECT .*reasoning_effort_multipliers.*FROM channel_account_stats_model_pricing`).
		WithArgs(pq.Array([]int64{9})).WillReturnRows(sqlmock.NewRows(columns).AddRow(
		int64(11), int64(9), "anthropic", `["custom-model"]`, service.BillingModeToken,
		nil, nil, nil, nil, nil, stored.value, nil, nil, time.Time{}, time.Time{},
	))
	mock.ExpectQuery(`(?s)SELECT .*FROM channel_account_stats_pricing_intervals`).
		WithArgs(pq.Array([]int64{11})).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	loaded, err := repo.batchLoadAccountStatsModelPricing(ctx, []int64{9})
	require.NoError(t, err)
	require.Len(t, loaded[9], 1)
	require.Equal(t, pricing.ReasoningEffortMultipliers, loaded[9][0].ReasoningEffortMultipliers)
	require.NoError(t, mock.ExpectationsWereMet())
}
