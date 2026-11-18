//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestChannelModelPricingUltrafastMultiplierRoundTrip(t *testing.T) {
	fast, flex, ultrafast := 2.5, 0.5, 4.0
	pricing := &service.ChannelModelPricing{
		ID: 11, ChannelID: 7, Platform: "openai", Models: []string{"gpt-6.2-astra"},
		FastMultiplier: &fast, FlexMultiplier: &flex, UltrafastMultiplier: &ultrafast,
	}

	t.Run("create binds ultrafast after flex", func(t *testing.T) {
		repo, mock := newChannelModelPricingTimePricingRepo(t)
		mock.ExpectQuery(`(?s)INSERT INTO channel_model_pricing .*fast_multiplier, flex_multiplier, ultrafast_multiplier, reasoning_effort_multipliers`).
			WithArgs(int64(7), "openai", []byte(`["gpt-6.2-astra"]`), service.BillingModeToken,
				nil, nil, nil, nil, nil, nil, &fast, &flex, &ultrafast, "{}", nil, nil, nil, nil).
			WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(int64(11), time.Time{}, time.Time{}))
		require.NoError(t, repo.CreateModelPricing(context.Background(), pricing))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("update binds ultrafast after flex", func(t *testing.T) {
		repo, mock := newChannelModelPricingTimePricingRepo(t)
		mock.ExpectExec(`(?s)UPDATE channel_model_pricing.*flex_multiplier = \$10, ultrafast_multiplier = \$11, reasoning_effort_multipliers = \$12.*WHERE id = \$18`).
			WithArgs([]byte(`["gpt-6.2-astra"]`), service.BillingModeToken,
				nil, nil, nil, nil, nil, nil, &fast, &flex, &ultrafast, "{}", nil, nil, nil, nil, "openai", int64(11)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		require.NoError(t, repo.UpdateModelPricing(context.Background(), pricing))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("list scans ultrafast and keeps NULL as nil", func(t *testing.T) {
		for _, stored := range []any{4.0, nil} {
			repo, mock := newChannelModelPricingTimePricingRepo(t)
			mock.ExpectQuery(`(?s)SELECT .*flex_multiplier, ultrafast_multiplier, reasoning_effort_multipliers.*FROM channel_model_pricing.*channel_id = \$1`).
				WithArgs(int64(7)).
				WillReturnRows(sqlmock.NewRows(channelModelPricingTimePricingColumns).AddRow(
					int64(11), int64(7), "openai", `["gpt-6.2-astra"]`, service.BillingModeToken,
					nil, nil, nil, nil, nil, nil, 2.5, nil, stored, "{}", nil, nil, nil, nil,
					time.Time{}, time.Time{},
				))
			expectEmptyModelPricingIntervals(mock)

			loaded, err := repo.ListModelPricing(context.Background(), 7)
			require.NoError(t, err)
			require.Len(t, loaded, 1)
			require.Equal(t, 2.5, *loaded[0].FastMultiplier)
			if stored == nil {
				require.Nil(t, loaded[0].UltrafastMultiplier)
			} else {
				require.Equal(t, 4.0, *loaded[0].UltrafastMultiplier)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		}
	})
}
