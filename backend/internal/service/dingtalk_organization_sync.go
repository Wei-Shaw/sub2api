package service

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
	"github.com/google/uuid"
)

type DingTalkSyncJob struct {
	JobID       string     `json:"job_id"`
	Status      string     `json:"status"`
	StartedAt   *time.Time `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
	Error       string     `json:"error"`
	Departments int        `json:"departments"`
	Members     int        `json:"members"`
}

// The database lease deduplicates requests across server instances. An interrupted
// process leaves a visible expired job which can be retried, rather than running forever.
func (s *DingTalkOrganizationService) SyncStatus(ctx context.Context, app string) (*DingTalkSyncJob, error) {
	job := &DingTalkSyncJob{Status: "idle"}
	err := s.db.QueryRowContext(ctx, `SELECT job_id,
 CASE WHEN status='running' AND expires_at<=NOW() THEN 'failed' ELSE status END,
 started_at,finished_at,
 CASE WHEN status='running' AND expires_at<=NOW() THEN 'Sync interrupted or timed out; retry synchronization' ELSE error END,
 departments,members FROM dingtalk_sync_jobs WHERE app_id=$1`, app).Scan(&job.JobID, &job.Status, &job.StartedAt, &job.FinishedAt, &job.Error, &job.Departments, &job.Members)
	if errors.Is(err, sql.ErrNoRows) {
		return job, nil
	}
	return job, err
}

func (s *DingTalkOrganizationService) StartSync(ctx context.Context, app string, read func(context.Context) ([]DingTalkDepartment, []DingTalkDirectoryMember, error)) (*DingTalkSyncJob, error) {
	id := uuid.NewString()
	job := &DingTalkSyncJob{JobID: id, Status: "running"}
	err := s.db.QueryRowContext(ctx, `INSERT INTO dingtalk_sync_jobs(app_id,job_id,status,expires_at)
 VALUES($1,$2,'running',NOW()+INTERVAL '35 minutes')
 ON CONFLICT(app_id) DO UPDATE SET job_id=EXCLUDED.job_id,status='running',started_at=NOW(),finished_at=NULL,
 expires_at=EXCLUDED.expires_at,error='',departments=0,members=0
 WHERE dingtalk_sync_jobs.status<>'running' OR dingtalk_sync_jobs.expires_at<=NOW()
 RETURNING started_at`, app, id).Scan(&job.StartedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return s.SyncStatus(ctx, app)
	}
	if err != nil {
		return nil, err
	}
	go s.runSync(app, id, read)
	return job, nil
}

func (s *DingTalkOrganizationService) runSync(app, id string, read func(context.Context) ([]DingTalkDepartment, []DingTalkDirectoryMember, error)) {
	// Never inherit the HTTP request cancellation or its timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	status, message := "failed", "Sync failed; check application permissions and retry"
	departments, members := 0, 0
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Print("DingTalk background sync interrupted by panic")
		}
		finish, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_, err := s.db.ExecContext(finish, `UPDATE dingtalk_sync_jobs SET status=$3,error=$4,departments=$5,members=$6,finished_at=NOW() WHERE app_id=$1 AND job_id=$2`, app, id, status, message, departments, members)
		if err != nil {
			log.Printf("DingTalk sync status: %s", logredact.RedactText(err.Error()))
		}
	}()
	ds, ms, err := read(ctx)
	if err == nil {
		err = s.replaceDirectory(ctx, app, ds, ms, id)
	}
	if err != nil {
		log.Printf("DingTalk sync failed: %s", logredact.RedactText(err.Error()))
		if errors.Is(err, context.DeadlineExceeded) {
			message = "Sync timed out; retry synchronization"
		}
		return
	}
	status, message, departments, members = "succeeded", "", len(ds), len(ms)
}
