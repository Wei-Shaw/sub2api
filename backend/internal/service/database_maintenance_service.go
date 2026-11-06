package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
)

const databaseMaintenanceTimeout = 30 * time.Minute

// Only historical records with a well-defined retention policy can be deleted.
// Accounts, payments, balances and other business state are never cleanup targets.
var databaseCleanupColumns = map[string]string{
	"usage_logs":                    "created_at",
	"ops_error_logs":                "created_at",
	"ops_ingress_reject_aggregates": "bucket_start",
	"ops_alert_events":              "created_at",
	"ops_system_logs":               "created_at",
	"ops_system_metrics":            "created_at",
	"ops_metrics_hourly":            "bucket_start",
	"ops_metrics_daily":             "bucket_date",
}

func DatabaseCleanupColumn(table string) (string, bool) {
	col, ok := databaseCleanupColumns[table]
	return col, ok
}

type DatabaseTableStats struct {
	Schema           string     `json:"schema"`
	Name             string     `json:"name"`
	DataBytes        int64      `json:"data_bytes"`
	IndexBytes       int64      `json:"index_bytes"`
	TotalBytes       int64      `json:"total_bytes"`
	LiveRows         int64      `json:"live_rows"`
	DeadRows         int64      `json:"dead_rows"`
	LastVacuum       *time.Time `json:"last_vacuum"`
	CleanupSupported bool       `json:"cleanup_supported"`
}

type DatabaseStats struct {
	Name      string               `json:"name"`
	SizeBytes int64                `json:"size_bytes"`
	Tables    []DatabaseTableStats `json:"tables"`
}

type DatabaseMaintenanceRequest struct {
	Operation     string   `json:"operation"`
	Tables        []string `json:"tables"`
	RetentionDays int      `json:"retention_days"`
	Confirm       bool     `json:"confirm"`
}

type DatabaseMaintenanceJob struct {
	ID              string     `json:"id"`
	Operation       string     `json:"operation"`
	Tables          []string   `json:"tables"`
	Status          string     `json:"status"`
	CreatedBy       int64      `json:"created_by"`
	StartedAt       time.Time  `json:"started_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	FinishedAt      *time.Time `json:"finished_at"`
	Cutoff          *time.Time `json:"cutoff"`
	CurrentTable    string     `json:"current_table"`
	CompletedTables int        `json:"completed_tables"`
	DeletedRows     int64      `json:"deleted_rows"`
	Error           string     `json:"error"`
}

type DatabaseMaintenanceRepository interface {
	Stats(context.Context) (*DatabaseStats, error)
	LatestJob(context.Context) (*DatabaseMaintenanceJob, error)
	Acquire(context.Context) (DatabaseMaintenanceSession, error)
}

// A session owns a PostgreSQL advisory lock until Close. VACUUM runs outside
// transactions; cleanup batches commit separately so progress survives failures.
type DatabaseMaintenanceSession interface {
	SaveJob(context.Context, *DatabaseMaintenanceJob) error
	CleanupBatch(context.Context, string, time.Time) (int64, error)
	RecomputeUsage(context.Context, time.Time) error
	Vacuum(context.Context, string, bool) error
	Close()
}

type DatabaseMaintenanceService struct{ repo DatabaseMaintenanceRepository }

func NewDatabaseMaintenanceService(repo DatabaseMaintenanceRepository) *DatabaseMaintenanceService {
	return &DatabaseMaintenanceService{repo: repo}
}

func (s *DatabaseMaintenanceService) Stats(ctx context.Context) (*DatabaseStats, error) {
	return s.repo.Stats(ctx)
}

func (s *DatabaseMaintenanceService) LatestJob(ctx context.Context) (*DatabaseMaintenanceJob, error) {
	job, err := s.repo.LatestJob(ctx)
	if err == nil && job != nil && job.Status == "running" && time.Since(job.StartedAt) > databaseMaintenanceTimeout+time.Minute {
		// A process restart may have interrupted a job before it could save its result.
		job.Status = "failed"
		job.Error = "maintenance interrupted or timed out; completed batches remain committed"
	}
	return job, err
}

func (s *DatabaseMaintenanceService) Start(ctx context.Context, req DatabaseMaintenanceRequest, userID int64) (*DatabaseMaintenanceJob, error) {
	if req.Operation != "cleanup" && req.Operation != "vacuum" && req.Operation != "vacuum_full" {
		return nil, infraerrors.BadRequest("INVALID_DATABASE_OPERATION", "unsupported maintenance operation")
	}
	if userID <= 0 || !req.Confirm {
		return nil, infraerrors.BadRequest("DATABASE_CONFIRMATION_REQUIRED", "an administrator must explicitly confirm maintenance")
	}
	if req.Operation == "cleanup" && (req.RetentionDays < 1 || req.RetentionDays > 36500) {
		return nil, infraerrors.BadRequest("INVALID_RETENTION_DAYS", "retention days must be between 1 and 36500")
	}
	if len(req.Tables) == 0 || len(req.Tables) > 500 {
		return nil, infraerrors.BadRequest("INVALID_DATABASE_TABLES", "select between 1 and 500 tables")
	}
	stats, err := s.repo.Stats(ctx)
	if err != nil {
		return nil, err
	}
	available := make(map[string]bool, len(stats.Tables))
	for _, table := range stats.Tables {
		available[table.Name] = true
	}
	seen := make(map[string]bool)
	tables := make([]string, 0, len(req.Tables))
	for _, table := range req.Tables {
		if !available[table] {
			return nil, infraerrors.BadRequest("INVALID_DATABASE_TABLE", "unknown database table")
		}
		if _, ok := DatabaseCleanupColumn(table); req.Operation == "cleanup" && !ok {
			return nil, infraerrors.BadRequest("DATABASE_CLEANUP_UNSUPPORTED", "cleanup is only supported for historical log tables")
		}
		if !seen[table] {
			tables = append(tables, table)
			seen[table] = true
		}
	}
	session, err := s.repo.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	job := &DatabaseMaintenanceJob{ID: uuid.NewString(), Operation: req.Operation, Tables: tables, Status: "running", CreatedBy: userID, StartedAt: now, UpdatedAt: now}
	if req.Operation == "cleanup" {
		cutoff := now.AddDate(0, 0, -req.RetentionDays)
		job.Cutoff = &cutoff
	}
	if err := session.SaveJob(ctx, job); err != nil {
		session.Close()
		return nil, err
	}
	// Return a separate copy: the worker mutates its job while the handler encodes the response.
	result := *job
	go s.run(session, job)
	return &result, nil
}

func (s *DatabaseMaintenanceService) run(session DatabaseMaintenanceSession, job *DatabaseMaintenanceJob) {
	defer session.Close()
	ctx, cancel := context.WithTimeout(context.Background(), databaseMaintenanceTimeout)
	defer cancel()
	err := s.execute(ctx, session, job)
	now := time.Now().UTC()
	job.UpdatedAt, job.FinishedAt = now, &now
	job.Status = "succeeded"
	if err != nil {
		job.Status = "failed"
		job.Error = err.Error()
	}
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer saveCancel()
	if saveErr := session.SaveJob(saveCtx, job); saveErr != nil {
		slog.Error("database maintenance result could not be saved", "job_id", job.ID, "error", saveErr)
	}
	slog.Info("database maintenance finished", "job_id", job.ID, "operation", job.Operation, "created_by", job.CreatedBy, "status", job.Status, "deleted_rows", job.DeletedRows, "error", job.Error)
}

func (s *DatabaseMaintenanceService) execute(ctx context.Context, session DatabaseMaintenanceSession, job *DatabaseMaintenanceJob) error {
	for _, table := range job.Tables {
		job.CurrentTable = table
		job.UpdatedAt = time.Now().UTC()
		if err := session.SaveJob(ctx, job); err != nil {
			return err
		}
		if job.Operation == "cleanup" {
			if err := s.cleanupTable(ctx, session, job, table); err != nil {
				return err
			}
		} else if err := session.Vacuum(ctx, table, job.Operation == "vacuum_full"); err != nil {
			return fmt.Errorf("%s: %w", table, err)
		}
		job.CompletedTables++
	}
	job.CurrentTable = ""
	return nil
}

func (s *DatabaseMaintenanceService) cleanupTable(ctx context.Context, session DatabaseMaintenanceSession, job *DatabaseMaintenanceJob, table string) (err error) {
	defer func() {
		if table == "usage_logs" {
			// Repair dashboard buckets even after a partially completed cleanup. This
			// uses the same session and runs before the maintenance lock is released.
			repairCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if repairErr := session.RecomputeUsage(repairCtx, *job.Cutoff); repairErr != nil {
				err = errors.Join(err, fmt.Errorf("recompute usage statistics: %w", repairErr))
			}
		}
	}()
	for {
		n, batchErr := session.CleanupBatch(ctx, table, *job.Cutoff)
		job.DeletedRows += n
		if batchErr != nil {
			return fmt.Errorf("%s: %w", table, batchErr)
		}
		job.UpdatedAt = time.Now().UTC()
		if err := session.SaveJob(ctx, job); err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
	}
}
