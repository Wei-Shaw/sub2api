//go:build integration

package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (s *UserRepoSuite) TestGetByIDUsesTransactionContext() {
	user := s.mustCreateUser(&service.User{
		Email:   "transaction-visible-user@example.com",
		Balance: 1,
	})

	tx, err := s.client.Tx(s.ctx)
	if err != nil {
		s.T().Fatalf("start transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	ctx, cancel := context.WithTimeout(s.ctx, 2*time.Second)
	defer cancel()
	txCtx := dbent.NewTxContext(ctx, tx)
	if _, err := tx.Client().User.UpdateOneID(user.ID).SetBalance(9).Save(txCtx); err != nil {
		s.T().Fatalf("update balance in transaction: %v", err)
	}

	got, err := s.repo.GetByID(txCtx, user.ID)
	if err != nil {
		s.T().Fatalf("get user through transaction context: %v", err)
	}
	if got.Balance != 9 {
		s.T().Fatalf("balance = %v, want 9", got.Balance)
	}
}
