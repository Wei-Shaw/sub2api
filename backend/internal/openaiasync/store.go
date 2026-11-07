package openaiasync

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type Store struct {
	db        *sql.DB
	quota     int64
	tempQuota int64
	maxOutput int64
	maxActive int
}

func NewStore(db *sql.DB, quota, tempQuota, maxOutput int64, maxActive int) *Store {
	return &Store{db: db, quota: quota, tempQuota: tempQuota, maxOutput: maxOutput, maxActive: maxActive}
}

func (s *Store) storageLock(ctx context.Context, tx *sql.Tx, userID int64, size int64) error {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('openai_files:' || $1::text, 0))`, userID); err != nil {
		return err
	}
	var used, temporary int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(input_bytes + GREATEST(reserved_output_bytes,result_bytes)),0) FROM openai_batches WHERE user_id=$1`, userID).Scan(&temporary); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(octet_length(content)),0) FROM openai_files WHERE user_id=$1`, userID).Scan(&used); err != nil {
		return err
	}
	if size > s.quota-used-temporary {
		return ErrQuota
	}
	return nil
}

// reserveStorage serializes file uploads and batch admission across replicas.
// Completed results occupy the reserved output budget, not additional quota.
func (s *Store) reserveStorage(ctx context.Context, tx *sql.Tx, userID, size int64) error {
	if err := s.storageLock(ctx, tx, userID, size); err != nil {
		return err
	}
	var used int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(input_bytes + GREATEST(reserved_output_bytes,result_bytes)),0) FROM openai_batches WHERE user_id=$1`, userID).Scan(&used); err != nil {
		return err
	}
	if size > s.tempQuota-used {
		return ErrQuota
	}
	return nil
}

func insertFile(ctx context.Context, tx *sql.Tx, userID int64, filename, purpose string, body []byte, ttl time.Duration) (*File, error) {
	now := time.Now().Unix()
	f := &File{ID: NewID("file-local-"), Object: "file", Bytes: int64(len(body)), CreatedAt: now, ExpiresAt: now + int64(ttl/time.Second), Filename: filename, Purpose: purpose, Status: "processed"}
	_, err := tx.ExecContext(ctx, `INSERT INTO openai_files(id,user_id,purpose,filename,content,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, f.ID, userID, purpose, filename, body, f.CreatedAt, f.ExpiresAt)
	return f, err
}

func (s *Store) PutFile(ctx context.Context, userID int64, filename, purpose string, body []byte, ttl time.Duration) (*File, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = s.storageLock(ctx, tx, userID, int64(len(body))); err != nil {
		return nil, err
	}
	f, err := insertFile(ctx, tx, userID, filename, purpose, body, ttl)
	if err != nil {
		return nil, err
	}
	return f, tx.Commit()
}

func (s *Store) GetFile(ctx context.Context, userID int64, id string) (*File, []byte, error) {
	f := &File{Object: "file", Status: "processed"}
	var body []byte
	err := s.db.QueryRowContext(ctx, `SELECT id,purpose,filename,content,created_at,expires_at FROM openai_files WHERE id=$1 AND user_id=$2 AND expires_at>$3`, id, userID, time.Now().Unix()).Scan(&f.ID, &f.Purpose, &f.Filename, &body, &f.CreatedAt, &f.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	f.Bytes = int64(len(body))
	return f, body, err
}

// FileMetadata does not load the potentially large BYTEA content into memory.
func (s *Store) FileMetadata(ctx context.Context, userID int64, id string) (*File, error) {
	f := &File{Object: "file", Status: "processed"}
	err := s.db.QueryRowContext(ctx, `SELECT id,purpose,filename,octet_length(content),created_at,expires_at FROM openai_files WHERE id=$1 AND user_id=$2 AND expires_at>$3`, id, userID, time.Now().Unix()).Scan(&f.ID, &f.Purpose, &f.Filename, &f.Bytes, &f.CreatedAt, &f.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return f, err
}

func (s *Store) DeleteFile(ctx context.Context, userID int64, id string) error {
	r, err := s.db.ExecContext(ctx, `DELETE FROM openai_files WHERE id=$1 AND user_id=$2 AND expires_at>$3`, id, userID, time.Now().Unix())
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Cursor comparison includes the random id, giving stable pagination for uploads
// within the same second. Cursors are always resolved in the owner's namespace.
func (s *Store) ListFiles(ctx context.Context, userID int64, purpose, after string, asc bool, limit int) ([]File, error) {
	op, order := "<", "DESC"
	if asc {
		op, order = ">", "ASC"
	}
	args := []any{userID, time.Now().Unix(), purpose}
	query := `SELECT id,purpose,filename,octet_length(content),created_at,expires_at FROM openai_files WHERE user_id=$1 AND expires_at>$2 AND ($3='' OR purpose=$3)`
	if after != "" {
		var created int64
		if err := s.db.QueryRowContext(ctx, `SELECT created_at FROM openai_files WHERE id=$1 AND user_id=$2 AND expires_at>$3`, after, userID, time.Now().Unix()).Scan(&created); errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		} else if err != nil {
			return nil, err
		}
		args = append(args, created, after)
		query += fmt.Sprintf(` AND (created_at,id) %s ($4,$5)`, op)
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY created_at %s,id %s LIMIT $%d`, order, order, len(args))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	files := []File{}
	for rows.Next() {
		f := File{Object: "file", Status: "processed"}
		if err := rows.Scan(&f.ID, &f.Purpose, &f.Filename, &f.Bytes, &f.CreatedAt, &f.ExpiresAt); err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

func (s *Store) CreateBatch(ctx context.Context, j *Job, items []Item) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Serialize admission per user across replicas.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('openai_batches:' || $1::text,0))`, j.UserID); err != nil {
		return err
	}
	var active int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM openai_batches WHERE user_id=$1 AND status IN ('validating','in_progress','cancelling','finalizing')`, j.UserID).Scan(&active); err != nil {
		return err
	}
	if active >= s.maxActive {
		return ErrQuota
	}

	j.InputBytes = 0
	for _, item := range items {
		j.InputBytes += int64(len(item.Body) + len(item.CustomID))
	}
	j.OutputBudget = s.maxOutput
	if err = s.reserveStorage(ctx, tx, j.UserID, j.InputBytes+j.OutputBudget); err != nil {
		return err
	}
	payload, err := json.Marshal(j.Batch)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO openai_batches(id,user_id,api_key_id,group_id,remote_ip,status,payload,created_at,output_ttl_seconds,input_bytes,reserved_output_bytes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, j.ID, j.UserID, j.APIKeyID, j.GroupID, j.RemoteIP, j.Status, payload, j.CreatedAt, int64(j.OutputTTL/time.Second), j.InputBytes, j.OutputBudget)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO openai_batch_items(batch_id,ordinal,custom_id,body) VALUES($1,$2,$3,$4)`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	for i, item := range items {
		if _, err = stmt.ExecContext(ctx, j.ID, i, item.CustomID, []byte(item.Body)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) GetBatch(ctx context.Context, userID int64, id string) (*Batch, error) {
	var payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM openai_batches WHERE id=$1 AND user_id=$2`, id, userID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var b Batch
	err = json.Unmarshal(payload, &b)
	return &b, err
}

func (s *Store) ListBatches(ctx context.Context, userID int64, after string, limit int) ([]Batch, error) {
	query := `SELECT payload FROM openai_batches WHERE user_id=$1`
	args := []any{userID}
	if after != "" {
		var created int64
		err := s.db.QueryRowContext(ctx, `SELECT created_at FROM openai_batches WHERE id=$1 AND user_id=$2`, after, userID).Scan(&created)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		args = append(args, created, after)
		query += ` AND (created_at,id)<($2,$3)`
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY created_at DESC,id DESC LIMIT $%d`, len(args))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Batch{}
	for rows.Next() {
		var raw []byte
		var b Batch
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &b); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func lockJob(ctx context.Context, tx *sql.Tx, id string) (*Job, error) {
	var raw []byte
	var lease sql.NullString
	var ttl int64
	j := &Job{}
	err := tx.QueryRowContext(ctx, `SELECT payload,user_id,api_key_id,group_id,remote_ip,lease_token,output_ttl_seconds,input_bytes,reserved_output_bytes FROM openai_batches WHERE id=$1 FOR UPDATE`, id).Scan(&raw, &j.UserID, &j.APIKeyID, &j.GroupID, &j.RemoteIP, &lease, &ttl, &j.InputBytes, &j.OutputBudget)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(raw, &j.Batch)
	j.Lease = lease.String
	j.OutputTTL = time.Duration(ttl) * time.Second
	return j, err
}

func saveJob(ctx context.Context, tx *sql.Tx, j *Job) error {
	raw, err := json.Marshal(j.Batch)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE openai_batches SET status=$2,payload=$3 WHERE id=$1`, j.ID, j.Status, raw)
	return err
}

func (s *Store) Cancel(ctx context.Context, userID int64, id string) (*Batch, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	j, err := lockJob(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if j.UserID != userID {
		return nil, ErrNotFound
	}
	if !Terminal(j.Status) && j.Status != "cancelling" {
		now := time.Now().Unix()
		j.Status = "cancelling"
		j.CancellingAt = &now
		if err = saveJob(ctx, tx, j); err != nil {
			return nil, err
		}
	}
	return &j.Batch, tx.Commit()
}

// Claim recovers expired leases but never replays an item left running by a
// crashed process: its upstream outcome and billing may already be committed.
func (s *Store) Claim(ctx context.Context) (*Job, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var id string
	err = tx.QueryRowContext(ctx, `SELECT id FROM openai_batches WHERE status IN ('validating','in_progress','cancelling','finalizing') AND (lease_until IS NULL OR lease_until<NOW()) ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	j, err := lockJob(ctx, tx, id)
	if err != nil {
		return nil, err
	}

	if j.OutputBudget == 0 {
		var results int64
		if err = tx.QueryRowContext(ctx, `SELECT result_bytes FROM openai_batches WHERE id=$1`, id).Scan(&results); err != nil {
			return nil, err
		}
		budget := max(s.maxOutput, results)
		if err = s.reserveStorage(ctx, tx, j.UserID, budget-results); errors.Is(err, ErrQuota) {
			// Preserve old work and try again after space is available; never infer again.
			if _, err = tx.ExecContext(ctx, `UPDATE openai_batches SET lease_until=NOW()+INTERVAL '60 seconds',lease_token=$2 WHERE id=$1`, id, NewID("lease_")); err != nil {
				return nil, err
			}
			return nil, tx.Commit()
		} else if err != nil {
			return nil, err
		}
		j.OutputBudget = budget
		if _, err = tx.ExecContext(ctx, `UPDATE openai_batches SET reserved_output_bytes=$2 WHERE id=$1`, id, budget); err != nil {
			return nil, err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT ordinal,custom_id FROM openai_batch_items WHERE batch_id=$1 AND state='running'`, id)
	if err != nil {
		return nil, err
	}
	type abandoned struct {
		ordinal  int
		customID string
	}
	var abandonedItems []abandoned
	for rows.Next() {
		var a abandoned
		if err = rows.Scan(&a.ordinal, &a.customID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		abandonedItems = append(abandonedItems, a)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	for _, a := range abandonedItems {
		raw, _ := json.Marshal(Failure(a.customID, "batch_execution_interrupted", "Execution was interrupted; the upstream outcome is unknown. This request was not retried."))
		if _, err = tx.ExecContext(ctx, `UPDATE openai_batch_items SET state='completed',result=$3,succeeded=FALSE WHERE batch_id=$1 AND ordinal=$2`, id, a.ordinal, raw); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE openai_batches SET result_bytes=result_bytes+$2 WHERE id=$1`, id, len(raw)+1); err != nil {
			return nil, err
		}
		j.RequestCounts.Failed++
	}
	j.Lease = NewID("lease_")
	now := time.Now().Unix()
	if j.Status == "validating" {
		j.Status = "in_progress"
		j.InProgressAt = &now
	}
	if _, err = tx.ExecContext(ctx, `UPDATE openai_batches SET lease_token=$2,lease_until=NOW()+INTERVAL '60 seconds' WHERE id=$1`, id, j.Lease); err != nil {
		return nil, err
	}
	if err = saveJob(ctx, tx, j); err != nil {
		return nil, err
	}
	return j, tx.Commit()
}

func (s *Store) Renew(ctx context.Context, j *Job) error {
	r, err := s.db.ExecContext(ctx, `UPDATE openai_batches SET lease_until=NOW()+INTERVAL '60 seconds' WHERE id=$1 AND lease_token=$2 AND lease_until>NOW()`, j.ID, j.Lease)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrLeaseLost
	}
	return nil
}

func fencedJob(ctx context.Context, tx *sql.Tx, j *Job) (*Job, error) {
	current, err := lockJob(ctx, tx, j.ID)
	if err != nil {
		return nil, err
	}
	if current.Lease != j.Lease || Terminal(current.Status) {
		return nil, ErrLeaseLost
	}
	var live bool
	if err = tx.QueryRowContext(ctx, `SELECT lease_until>NOW() FROM openai_batches WHERE id=$1`, j.ID).Scan(&live); err != nil {
		return nil, err
	}
	if !live {
		return nil, ErrLeaseLost
	}
	return current, nil
}

func (s *Store) Next(ctx context.Context, j *Job) (*Item, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := fencedJob(ctx, tx, j)
	if err != nil {
		return nil, err
	}
	j.Batch = current.Batch
	if j.Status == "cancelling" || j.Status == "finalizing" || time.Now().Unix() >= j.ExpiresAt {
		return nil, tx.Commit()
	}
	i := &Item{}
	err = tx.QueryRowContext(ctx, `SELECT ordinal,custom_id,body FROM openai_batch_items WHERE batch_id=$1 AND state='pending' ORDER BY ordinal LIMIT 1`, j.ID).Scan(&i.Ordinal, &i.CustomID, &i.Body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, tx.Commit()
	}
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE openai_batch_items SET state='running' WHERE batch_id=$1 AND ordinal=$2`, j.ID, i.Ordinal)
	if err != nil {
		return nil, err
	}
	return i, tx.Commit()
}

func (s *Store) SaveResult(ctx context.Context, j *Job, i *Item, result Result, maxOutputBytes int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := fencedJob(ctx, tx, j)
	if err != nil {
		return err
	}
	if current.OutputBudget > 0 {
		maxOutputBytes = min(maxOutputBytes, current.OutputBudget)
	}
	var used int64
	if err = tx.QueryRowContext(ctx, `SELECT result_bytes FROM openai_batches WHERE id=$1`, j.ID).Scan(&used); err != nil {
		return err
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	// Reserve enough for a failure record even when a 512-byte custom_id
	// consists entirely of characters requiring six-byte JSON escapes.
	reserve := int64(current.RequestCounts.Total-current.RequestCounts.Completed-current.RequestCounts.Failed-1) * 4096
	if used+int64(len(raw))+1+reserve > maxOutputBytes {
		result = Failure(i.CustomID, "response_too_large", "Response exceeded the remaining local batch output budget")
		raw, err = json.Marshal(result)
		if err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE openai_batches SET result_bytes=result_bytes+$2 WHERE id=$1`, j.ID, len(raw)+1); err != nil {
		return err
	}
	success := result.Error == nil && result.Response != nil && result.Response.StatusCode >= 200 && result.Response.StatusCode < 300
	r, err := tx.ExecContext(ctx, `UPDATE openai_batch_items SET state='completed',result=$3,succeeded=$4 WHERE batch_id=$1 AND ordinal=$2 AND state='running'`, j.ID, i.Ordinal, raw, success)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrLeaseLost
	}
	if success {
		current.RequestCounts.Completed++
	} else {
		current.RequestCounts.Failed++
	}
	if err = saveJob(ctx, tx, current); err != nil {
		return err
	}
	j.Batch = current.Batch
	return tx.Commit()
}

// Finish creates both result files and commits their ids with the terminal
// state in the same transaction. Quota pressure retains all item results for
// a later export attempt; completed inference is never replayed or discarded.
func (s *Store) Finish(ctx context.Context, j *Job, maxOutputBytes int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := fencedJob(ctx, tx, j)
	if err != nil {
		return err
	}
	j.Batch = current.Batch
	if j.CancellingAt == nil && time.Now().Unix() >= j.ExpiresAt {
		r, updateErr := tx.ExecContext(ctx, `UPDATE openai_batch_items SET state='completed',succeeded=FALSE,result=json_build_object('id','batch_req_' || batch_id || '_' || ordinal::text,'custom_id',custom_id,'response',NULL,'error',json_build_object('code','batch_expired','message','The batch completion window expired before this request could be executed')) WHERE batch_id=$1 AND state='pending'`, j.ID)
		if updateErr != nil {
			return updateErr
		}
		n, updateErr := r.RowsAffected()
		if updateErr != nil {
			return updateErr
		}
		j.RequestCounts.Failed += int(n)
	}
	rows, err := tx.QueryContext(ctx, `SELECT result,succeeded FROM openai_batch_items WHERE batch_id=$1 AND state='completed' ORDER BY ordinal`, j.ID)
	if err != nil {
		return err
	}
	var good, bad bytes.Buffer
	var total int64
	tooLarge := false
	for rows.Next() {
		var raw []byte
		var success bool
		if err = rows.Scan(&raw, &success); err != nil {
			_ = rows.Close()
			return err
		}
		total += int64(len(raw) + 1)
		if total > maxOutputBytes {
			tooLarge = true
			break
		}
		buf := &bad
		if success {
			buf = &good
		}
		_, _ = buf.Write(raw)
		_ = buf.WriteByte('\n')
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	if j.FinalizingAt == nil {
		j.FinalizingAt = &now
	}
	if tooLarge {
		err = ErrQuota
	} else {
		err = s.storageLock(ctx, tx, j.UserID, total-j.InputBytes-j.OutputBudget)
	}
	if err != nil && !errors.Is(err, ErrQuota) {
		return err
	}
	if errors.Is(err, ErrQuota) {
		j.Status = "finalizing"
		j.Errors = map[string]any{"object": "list", "data": []ItemError{{Code: "storage_limit_exceeded", Message: "Results are preserved. Free storage or raise the output/storage limits; export will retry without executing requests again."}}}
		if err = saveJob(ctx, tx, j); err != nil {
			return err
		}
		// Keep the lease until expiry to bound retries to once per minute.
		if err = tx.Commit(); err != nil {
			return err
		}
		return ErrQuota
	} else {
		for _, out := range []struct {
			body []byte
			name string
			id   **string
		}{{good.Bytes(), "output.jsonl", &j.OutputFileID}, {bad.Bytes(), "errors.jsonl", &j.ErrorFileID}} {
			if len(out.body) > 0 {
				f, putErr := insertFile(ctx, tx, j.UserID, out.name, "batch_output", out.body, j.OutputTTL)
				if putErr != nil {
					return putErr
				}
				*out.id = &f.ID
			}
		}
		switch {
		case j.CancellingAt != nil:
			j.Status = "cancelled"
			j.CancelledAt = &now
		case now >= j.ExpiresAt:
			j.Status = "expired"
			j.ExpiredAt = &now
		default:
			j.Status = "completed"
			j.CompletedAt = &now
		}
	}
	j.Errors = nil
	if err = saveJob(ctx, tx, j); err != nil {
		return err
	}
	// Release temporary input/result copies after the output files are durable.
	if _, err = tx.ExecContext(ctx, `DELETE FROM openai_batch_items WHERE batch_id=$1`, j.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE openai_batches SET lease_token=NULL,lease_until=NULL,input_bytes=0,reserved_output_bytes=0,result_bytes=0 WHERE id=$1`, j.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Cleanup(ctx context.Context) error {
	// Bounded deletes keep maintenance from blocking large installations.
	_, err := s.db.ExecContext(ctx, `DELETE FROM openai_files WHERE id IN (SELECT id FROM openai_files WHERE expires_at<=$1 LIMIT 100)`, time.Now().Unix())
	return err
}
