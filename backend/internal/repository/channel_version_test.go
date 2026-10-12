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

func TestChannelVersionConcurrentUpdateRollsBackBeforeReplacingPricing(t *testing.T) {
	repo, mock := newChannelModelPricingTimePricingRepo(t)
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE channels.*WHERE id = \$10 AND updated_at = \$11`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	err := repo.Update(context.Background(), &service.Channel{ID: 3, UpdatedAt: time.Now(), ModelPricing: []service.ChannelModelPricing{{Models: []string{"deepseek-v4-pro"}}}})
	require.ErrorIs(t, err, service.ErrChannelModified)
	require.NoError(t, mock.ExpectationsWereMet())
}
