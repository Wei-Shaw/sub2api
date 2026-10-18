package repository

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"testing"
)

func TestAccountQualityCancelledLeaseCannotPersistOrResurrect(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT NOT cancel_requested.*FOR UPDATE").WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{"active"}).AddRow(false))
	mock.ExpectRollback()
	repo := &accountQualityRepo{db: db}
	err = repo.AddResult(context.Background(), &service.AccountQualityResult{RunID: 5})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expired or canceled lease must reject result: %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
