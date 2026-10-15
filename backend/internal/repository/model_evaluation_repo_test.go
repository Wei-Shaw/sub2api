package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestModelEvaluationRepositoryReservationDoesNotDuplicateOrSkipRounds(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		repo := &modelEvaluationRepository{db: db}
		initial := &service.ModelEvaluationResult{ID: "result", Status: "running", StartedAt: time.Now()}
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT rounds .* FOR UPDATE").WithArgs("report").WillReturnRows(sqlmock.NewRows([]string{"rounds"}).AddRow(5))
		rows := sqlmock.NewRows([]string{"round", "result"})
		if duplicate {
			payload, _ := json.Marshal(initial)
			rows.AddRow(2, payload)
		}
		mock.ExpectQuery("SELECT round, result").WithArgs("report", 2).WillReturnRows(rows)
		if duplicate {
			mock.ExpectCommit()
		} else {
			mock.ExpectQuery("SELECT EXISTS").WithArgs("report", 1).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
			mock.ExpectRollback()
		}
		row, created, err := repo.Reserve(context.Background(), "report", 2, initial)
		require.False(t, created)
		if duplicate {
			require.NoError(t, err)
			require.Equal(t, "result", row.Result.ID)
		} else {
			require.ErrorIs(t, err, service.ErrEvaluationRoundConflict)
		}
		require.NoError(t, mock.ExpectationsWereMet())
		_ = db.Close()
	}
}

func TestModelEvaluationRepositoryFinishDoesNotClaimMissingRecordWasSaved(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := &modelEvaluationRepository{db: db}
	mock.ExpectExec("UPDATE model_evaluation_rounds").WithArgs("report", 1, "correct", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	err = repo.Finish(context.Background(), "report", &service.ModelEvaluationRound{Round: 1, Result: &service.ModelEvaluationResult{Status: "correct"}})
	require.ErrorIs(t, err, service.ErrEvaluationRoundConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}
