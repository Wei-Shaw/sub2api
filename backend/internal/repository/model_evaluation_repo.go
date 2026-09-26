package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type modelEvaluationRepository struct{ db *sql.DB }

func NewModelEvaluationRepository(db *sql.DB) service.ModelEvaluationRepository {
	return &modelEvaluationRepository{db: db}
}

func (r *modelEvaluationRepository) Create(ctx context.Context, report *service.ModelEvaluationReport) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO model_evaluation_reports
		(id, target_type, target_id, target_name, model, effort, rounds, benchmark, prompt, created_by, visibility, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,0),'private',$11)`,
		report.ID, report.TargetType, report.TargetID, report.TargetName, report.Model, report.Effort,
		report.Rounds, report.Benchmark, report.Prompt, report.CreatedBy, report.CreatedAt)
	return err
}

const evaluationReportSelect = `SELECT r.id, r.target_type, r.target_id, r.target_name, r.model, r.effort,
	r.rounds, r.benchmark, COALESCE(r.created_by,0), r.visibility, r.created_at,
	(SELECT COUNT(*) FROM model_evaluation_rounds x WHERE x.report_id=r.id AND x.status <> 'running'),
	(SELECT COUNT(*) FROM model_evaluation_rounds x WHERE x.report_id=r.id AND x.status = 'correct'),
	(SELECT COUNT(*) FROM model_evaluation_rounds x WHERE x.report_id=r.id AND x.status IN ('correct','incorrect'))
	FROM model_evaluation_reports r `

func scanEvaluationReport(row scannable) (*service.ModelEvaluationReport, error) {
	report := &service.ModelEvaluationReport{}
	err := row.Scan(&report.ID, &report.TargetType, &report.TargetID, &report.TargetName,
		&report.Model, &report.Effort, &report.Rounds, &report.Benchmark, &report.CreatedBy,
		&report.Visibility, &report.CreatedAt, &report.Completed, &report.Correct, &report.Graded)
	return report, err
}

func (r *modelEvaluationRepository) List(ctx context.Context, targetType string, targetID int64, page, size int) ([]*service.ModelEvaluationReport, int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM model_evaluation_reports WHERE target_type=$1 AND target_id=$2`, targetType, targetID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.QueryContext(ctx, evaluationReportSelect+`WHERE r.target_type=$1 AND r.target_id=$2 ORDER BY r.created_at DESC, r.id DESC LIMIT $3 OFFSET $4`, targetType, targetID, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]*service.ModelEvaluationReport, 0)
	for rows.Next() {
		report, err := scanEvaluationReport(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, report)
	}
	return items, total, rows.Err()
}

func (r *modelEvaluationRepository) Get(ctx context.Context, id string) (*service.ModelEvaluationReport, error) {
	report, err := scanEvaluationReport(r.db.QueryRowContext(ctx, evaluationReportSelect+`WHERE r.id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrEvaluationReportNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := r.db.QueryRowContext(ctx, `SELECT prompt FROM model_evaluation_reports WHERE id=$1`, id).Scan(&report.Prompt); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT round, result FROM model_evaluation_rounds WHERE report_id=$1 ORDER BY round`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	report.Results = make([]*service.ModelEvaluationRound, 0)
	// Derive detail counters from these rows so a concurrent completion cannot
	// leave the report summary inconsistent with its displayed answers.
	report.Completed, report.Correct, report.Graded = 0, 0, 0
	for rows.Next() {
		row, err := scanEvaluationRound(rows)
		if err != nil {
			return nil, err
		}
		report.Results = append(report.Results, row)
		if row.Result.Status != "running" {
			report.Completed++
		}
		if row.Result.Status == "correct" {
			report.Correct++
		}
		if row.Result.Status == "correct" || row.Result.Status == "incorrect" {
			report.Graded++
		}
	}
	return report, rows.Err()
}

func scanEvaluationRound(row scannable) (*service.ModelEvaluationRound, error) {
	result := &service.ModelEvaluationRound{Saved: true}
	var payload []byte
	if err := row.Scan(&result.Round, &payload); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(payload, &result.Result); err != nil {
		return nil, err
	}
	if result.Result == nil {
		return nil, errors.New("missing evaluation result")
	}
	return result, nil
}

func (r *modelEvaluationRepository) Reserve(ctx context.Context, id string, number int, initial *service.ModelEvaluationResult) (*service.ModelEvaluationRound, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	var rounds int
	if err := tx.QueryRowContext(ctx, `SELECT rounds FROM model_evaluation_reports WHERE id=$1 FOR UPDATE`, id).Scan(&rounds); err != nil {
		return nil, false, err
	}
	if number < 1 || number > rounds {
		return nil, false, service.ErrEvaluationRoundConflict
	}
	existing, err := scanEvaluationRound(tx.QueryRowContext(ctx, `SELECT round, result FROM model_evaluation_rounds WHERE report_id=$1 AND round=$2`, id, number))
	if err == nil {
		return existing, false, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	if number > 1 {
		var ready bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM model_evaluation_rounds WHERE report_id=$1 AND round=$2 AND status <> 'running')`, id, number-1).Scan(&ready); err != nil {
			return nil, false, err
		}
		if !ready {
			return nil, false, service.ErrEvaluationRoundConflict
		}
	}
	payload, err := json.Marshal(initial)
	if err != nil {
		return nil, false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO model_evaluation_rounds (report_id, round, status, result, started_at) VALUES ($1,$2,'running',$3,$4)`, id, number, string(payload), initial.StartedAt)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return &service.ModelEvaluationRound{Round: number, Result: initial, Saved: true}, true, nil
}

func (r *modelEvaluationRepository) Finish(ctx context.Context, id string, row *service.ModelEvaluationRound) error {
	payload, err := json.Marshal(row.Result)
	if err != nil {
		return err
	}
	updated, err := r.db.ExecContext(ctx, `UPDATE model_evaluation_rounds SET status=$3, result=$4, finished_at=NOW() WHERE report_id=$1 AND round=$2 AND status='running'`, id, row.Round, row.Result.Status, string(payload))
	if err != nil {
		return err
	}
	count, err := updated.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return service.ErrEvaluationRoundConflict
	}
	return nil
}
