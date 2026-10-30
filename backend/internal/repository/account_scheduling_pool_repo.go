package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"maps"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/account"
	"github.com/Wei-Shaw/sub2api/ent/accountschedulingpool"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

const accountSchedulingLockID int64 = 849731826

type accountSchedulingPoolRepository struct {
	client *dbent.Client
	db     *sql.DB
}

func NewAccountSchedulingPoolRepository(client *dbent.Client, db *sql.DB) service.AccountSchedulingPoolRepository {
	return &accountSchedulingPoolRepository{client: client, db: db}
}
func poolFromEntity(p *dbent.AccountSchedulingPool) service.AccountSchedulingPool {
	return service.AccountSchedulingPool{ID: p.ID, Name: p.Name, Enabled: p.Enabled, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, Members: []service.AccountSchedulingMember{}}
}
func (r *accountSchedulingPoolRepository) List(ctx context.Context) ([]service.AccountSchedulingPool, error) {
	rows, err := clientFromContext(ctx, r.client).AccountSchedulingPool.Query().Order(dbent.Asc(accountschedulingpool.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]service.AccountSchedulingPool, 0, len(rows))
	for _, p := range rows {
		result = append(result, poolFromEntity(p))
	}
	return result, nil
}
func (r *accountSchedulingPoolRepository) Create(ctx context.Context, p *service.AccountSchedulingPool) error {
	row, err := r.client.AccountSchedulingPool.Create().SetName(p.Name).SetEnabled(p.Enabled).Save(ctx)
	if err != nil {
		return err
	}
	*p = poolFromEntity(row)
	return nil
}
func (r *accountSchedulingPoolRepository) Update(ctx context.Context, p *service.AccountSchedulingPool) error {
	row, err := r.client.AccountSchedulingPool.UpdateOneID(p.ID).SetName(p.Name).SetEnabled(p.Enabled).Save(ctx)
	if dbent.IsNotFound(err) {
		return service.ErrAccountSchedulingPoolNotFound
	}
	if err != nil {
		return err
	}
	*p = poolFromEntity(row)
	return nil
}
func (r *accountSchedulingPoolRepository) Delete(ctx context.Context, id int64) error {
	// Deleting a pool must not silently enable its formerly disabled members.
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", accountSchedulingLockID); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, "SELECT m.account_id FROM account_scheduling_members m JOIN accounts a ON a.id=m.account_id AND a.deleted_at IS NULL WHERE m.pool_id=$1", id)
	if err != nil {
		return err
	}
	ids := []int64{}
	for rows.Next() {
		var aid int64
		if err = rows.Scan(&aid); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, aid)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	if len(ids) > 0 {
		return service.ErrAccountSchedulingPoolNotEmpty
	}
	if err = tx.AccountSchedulingPool.DeleteOneID(id).Exec(ctx); dbent.IsNotFound(err) {
		return service.ErrAccountSchedulingPoolNotFound
	} else if err != nil {
		return err
	}
	return tx.Commit()
}
func (r *accountSchedulingPoolRepository) SetMembers(ctx context.Context, poolID *int64, ids []int64) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", accountSchedulingLockID); err != nil {
		return err
	}
	if poolID != nil {
		exists, err := tx.AccountSchedulingPool.Query().Where(accountschedulingpool.IDEQ(*poolID)).Exist(ctx)
		if err != nil {
			return err
		}
		if !exists {
			return service.ErrAccountSchedulingPoolNotFound
		}
	}
	accounts, err := tx.Account.Query().Where(account.IDIn(ids...)).ForShare().All(ctx)
	if err != nil {
		return err
	}
	if len(accounts) != len(ids) {
		return service.ErrAccountNotFound
	}
	for _, row := range accounts {
		if row.Platform != service.PlatformOpenAI || row.Type != service.AccountTypeOAuth || row.ParentAccountID != nil {
			return fmt.Errorf("scheduling pools support OpenAI OAuth parent accounts")
		}
	}
	for _, id := range ids {
		if poolID == nil {
			_, err = tx.ExecContext(ctx, "DELETE FROM account_scheduling_members WHERE account_id=$1", id)
		} else {
			_, err = tx.ExecContext(ctx, `INSERT INTO account_scheduling_members(account_id,pool_id,position) VALUES($1,$2,(SELECT COALESCE(MAX(position),0)+1 FROM account_scheduling_members)) ON CONFLICT(account_id) DO UPDATE SET pool_id=EXCLUDED.pool_id,position=EXCLUDED.position,updated_at=NOW()`, id, *poolID)
		}
		if err != nil {
			return err
		}
		if err = enqueueSchedulerOutbox(ctx, tx.Client(), service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
			return err
		}
	}
	// Explicitly moving/reordering members queues them together at the tail.
	// Their requested position then decides the order, even after quota recovery
	// gave them different queued_at values. Active identities are not preempted.
	if _, err = tx.ExecContext(ctx, "UPDATE account_scheduling_slots SET queued_at=statement_timestamp() WHERE account_id=ANY($1::bigint[])", pq.Array(ids)); err != nil {
		return err
	}
	return tx.Commit()
}
func schedulingMembers(ctx context.Context, exec sqlExecutor) ([]service.AccountSchedulingMember, error) {
	rows, err := exec.QueryContext(ctx, `SELECT m.account_id,m.pool_id,m.position,a.name FROM account_scheduling_members m JOIN accounts a ON a.id=m.account_id AND a.deleted_at IS NULL ORDER BY m.position,m.account_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []service.AccountSchedulingMember{}
	for rows.Next() {
		var m service.AccountSchedulingMember
		if err = rows.Scan(&m.AccountID, &m.PoolID, &m.Position, &m.Name); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}
func (r *accountSchedulingPoolRepository) ListMembers(ctx context.Context) ([]service.AccountSchedulingMember, error) {
	return schedulingMembers(ctx, clientFromContext(ctx, r.client))
}

func readSchedulingPolicy(ctx context.Context, exec sqlExecutor) (service.AccountSchedulingPolicy, error) {
	p := service.AccountSchedulingPolicy{MaxActiveAccounts: 3, ProxyExitGroups: map[string]string{}}
	rows, err := exec.QueryContext(ctx, "SELECT value FROM settings WHERE key=$1 FOR SHARE", service.AccountSchedulingPolicyKey)
	if err != nil {
		return p, err
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return p, err
		}
		if err = json.Unmarshal([]byte(raw), &p); err != nil {
			return p, err
		}
	}
	if err := service.ValidateAccountSchedulingPolicy(p); err != nil {
		return p, err
	}
	return p, rows.Err()
}

func (r *accountSchedulingPoolRepository) UpdatePolicy(ctx context.Context, policy service.AccountSchedulingPolicy) error {
	if err := service.ValidateAccountSchedulingPolicy(policy); err != nil {
		return err
	}
	raw, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", accountSchedulingLockID); err != nil {
		return err
	}
	previous, err := readSchedulingPolicy(ctx, tx.Client())
	if err != nil {
		return err
	}
	if !maps.Equal(previous.ProxyExitGroups, policy.ProxyExitGroups) {
		// A scope rename must not leave live identities charged to an old scope
		// while replacement identities gain a fresh budget for the same exit.
		rows, err := tx.QueryContext(ctx, "SELECT EXISTS(SELECT 1 FROM account_scheduling_leases WHERE expires_at>clock_timestamp())")
		if err != nil {
			return err
		}
		var live bool
		if !rows.Next() {
			_ = rows.Close()
			return fmt.Errorf("read active account scheduling leases")
		}
		err = rows.Scan(&live)
		_ = rows.Close()
		if err != nil {
			return err
		}
		if live {
			return service.ErrAccountSchedulingExitInUse
		}
	}
	proxyRows, err := tx.Proxy.Query().ForShare().All(ctx)
	if err != nil {
		return err
	}
	proxies := make([]service.Proxy, 0, len(proxyRows))
	for _, row := range proxyRows {
		proxies = append(proxies, *proxyEntityToService(row))
	}
	if err = service.ValidateAccountSchedulingProxyGroups(policy, proxies); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES($1,$2,clock_timestamp()) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=EXCLUDED.updated_at`, service.AccountSchedulingPolicyKey, string(raw)); err != nil {
		return err
	}
	return tx.Commit()
}

func readSchedulingAccounts(ctx context.Context, exec sqlExecutor) ([]service.Account, error) {
	// Admission needs identity and availability, never access/refresh tokens or
	// the many billing/model configuration fields in a complete account snapshot.
	rows, err := exec.QueryContext(ctx, `SELECT id,name,proxy_id,parent_account_id,status,COALESCE(error_message,''),schedulable,
		auto_pause_on_expired,expires_at,rate_limit_reset_at,overload_until,temp_unschedulable_until,
		jsonb_build_object('chatgpt_account_id',credentials->'chatgpt_account_id','email',credentials->'email','plan_type',credentials->'plan_type'),extra
		FROM accounts WHERE platform=$1 AND type=$2 AND deleted_at IS NULL ORDER BY id FOR SHARE`, service.PlatformOpenAI, service.AccountTypeOAuth)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := []service.Account{}
	for rows.Next() {
		a := service.Account{Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
		var credentials, extra []byte
		if err = rows.Scan(&a.ID, &a.Name, &a.ProxyID, &a.ParentAccountID, &a.Status, &a.ErrorMessage, &a.Schedulable,
			&a.AutoPauseOnExpired, &a.ExpiresAt, &a.RateLimitResetAt, &a.OverloadUntil, &a.TempUnschedulableUntil, &credentials, &extra); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(credentials, &a.Credentials); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(extra, &a.Extra); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

func (r *accountSchedulingPoolRepository) Reconcile(ctx context.Context, request service.AccountSchedulingAdmission) (*service.AccountSchedulingDecision, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", accountSchedulingLockID); err != nil {
		return nil, err
	}
	policy, err := readSchedulingPolicy(ctx, tx.Client())
	if err != nil {
		return nil, err
	}
	decision := &service.AccountSchedulingDecision{Enabled: policy.Enabled, Allowed: map[int64]bool{}, Members: []service.AccountSchedulingMember{}}
	// Read account state after obtaining the lock. Share-locks serialize admin
	// disable/delete/credential changes with the admission decision.
	accounts, err := readSchedulingAccounts(ctx, tx.Client())
	if err != nil {
		return nil, err
	}
	if request.AccountID > 0 {
		// A deleted account must never be mistaken for an unmanaged account.
		exists, lookupErr := tx.Account.Query().Where(account.IDEQ(request.AccountID)).Exist(ctx)
		if lookupErr != nil || !exists {
			return nil, service.ErrAccountSchedulingUnavailable
		}
	}
	names := map[int64]string{}
	for _, a := range accounts {
		names[a.ID] = a.Name
	}
	members, err := schedulingMembers(ctx, tx.Client())
	if err != nil {
		return nil, err
	}
	byID := map[int64]service.AccountSchedulingMember{}
	for _, m := range members {
		byID[m.AccountID] = m
	}
	poolEntities, err := tx.AccountSchedulingPool.Query().ForShare().All(ctx)
	if err != nil {
		return nil, err
	}
	pools := []service.AccountSchedulingPool{}
	for _, p := range poolEntities {
		pools = append(pools, poolFromEntity(p))
	}
	proxyEntities, err := tx.Proxy.Query().ForShare().All(ctx)
	if err != nil {
		return nil, err
	}
	proxies := []service.Proxy{}
	for _, p := range proxyEntities {
		proxies = append(proxies, *proxyEntityToService(p))
	}
	// Use the database clock after waiting for the lock. App hosts may have
	// different clocks, and transaction-start NOW() can predate a long lock wait.
	clockRows, err := tx.QueryContext(ctx, "SELECT clock_timestamp()")
	if err != nil {
		return nil, err
	}
	var now time.Time
	if !clockRows.Next() {
		_ = clockRows.Close()
		return nil, fmt.Errorf("read scheduling database clock")
	}
	err = clockRows.Scan(&now)
	_ = clockRows.Close()
	if err != nil {
		return nil, err
	}
	candidates, err := service.BuildAccountSchedulingCandidates(policy, accounts, members, pools, proxies, request.AccountID, request.ActualProxyURL, now)
	if err != nil {
		return nil, err
	}
	governed := map[int64]bool{}
	for _, c := range candidates {
		governed[c.AccountID] = true
	}
	// Membership is a management view even for a paid account which does not
	// consume the Free account admission budget.
	for _, member := range members {
		if !governed[member.AccountID] {
			member.State = "unmanaged"
			member.Reason = "not_free_tier"
			decision.Members = append(decision.Members, member)
		}
	}
	if !policy.Enabled {
		for _, c := range candidates {
			m := byID[c.AccountID]
			m.AccountID = c.AccountID
			m.Name = names[c.AccountID]
			m.State = "unmanaged"
			m.Reason = "policy_disabled"
			m.ExitGroup = c.ExitGroup
			decision.Members = append(decision.Members, m)
		}
		return decision, tx.Commit()
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM account_scheduling_leases WHERE expires_at<=$1", now); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT identity,account_id,exit_group,state,reason,queued_at,updated_at FROM account_scheduling_slots ORDER BY id FOR UPDATE")
	if err != nil {
		return nil, err
	}
	previous := []service.AccountSchedulingSlot{}
	for rows.Next() {
		var slot service.AccountSchedulingSlot
		if err = rows.Scan(&slot.Identity, &slot.AccountID, &slot.ExitGroup, &slot.State, &slot.Reason, &slot.QueuedAt, &slot.UpdatedAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		previous = append(previous, slot)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()
	rows, err = tx.QueryContext(ctx, "SELECT identity,COUNT(*) FROM account_scheduling_leases GROUP BY identity")
	if err != nil {
		return nil, err
	}
	live := map[string]int{}
	for rows.Next() {
		var identity string
		var count int
		if err = rows.Scan(&identity, &count); err != nil {
			_ = rows.Close()
			return nil, err
		}
		live[identity] = count
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()
	slots, allowed := service.ReconcileAccountSchedulingSlots(candidates, previous, live, policy.MaxActiveAccounts, now)
	decision.Allowed = allowed
	identities := []string{}
	slotMap := map[string]service.AccountSchedulingSlot{}
	previousByIdentity := map[string]service.AccountSchedulingSlot{}
	for _, slot := range previous {
		previousByIdentity[slot.Identity] = slot
	}
	for _, slot := range slots {
		identities = append(identities, slot.Identity)
		slotMap[slot.Identity] = slot
		if old, found := previousByIdentity[slot.Identity]; found && old == slot {
			continue
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO account_scheduling_slots(identity,account_id,exit_group,state,reason,queued_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(identity) DO UPDATE SET account_id=EXCLUDED.account_id,exit_group=EXCLUDED.exit_group,state=EXCLUDED.state,reason=EXCLUDED.reason,queued_at=EXCLUDED.queued_at,updated_at=EXCLUDED.updated_at`, slot.Identity, slot.AccountID, slot.ExitGroup, slot.State, slot.Reason, slot.QueuedAt, slot.UpdatedAt)
		if err != nil {
			return nil, err
		}
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM account_scheduling_slots WHERE NOT(identity=ANY($1::text[]))", pq.Array(identities)); err != nil {
		return nil, err
	}
	for _, c := range candidates {
		m := byID[c.AccountID]
		m.AccountID = c.AccountID
		m.Name = names[c.AccountID]
		slot := slotMap[c.Identity]
		m.State = slot.State
		m.Reason = slot.Reason
		m.ExitGroup = slot.ExitGroup
		q := slot.QueuedAt
		m.QueuedAt = &q
		decision.Members = append(decision.Members, m)
		if c.AccountID == request.AccountID && request.LeaseID != "" && allowed[c.AccountID] {
			_, err = tx.ExecContext(ctx, `INSERT INTO account_scheduling_leases(lease_id,identity,expires_at) VALUES($1,$2,clock_timestamp()+$3*INTERVAL '1 second')`, request.LeaseID, c.Identity, request.LeaseTTL.Seconds())
			if err != nil {
				return nil, err
			}
			decision.Acquired = true
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return decision, nil
}

func (r *accountSchedulingPoolRepository) RefreshLease(ctx context.Context, id string, ttl time.Duration) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	// Serialize expiry extension with policy relabeling. Otherwise a renewal
	// that began just before expiry could commit after the relabel's live check.
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", accountSchedulingLockID); err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE account_scheduling_leases SET expires_at=clock_timestamp()+$2*INTERVAL '1 second',updated_at=clock_timestamp() WHERE lease_id=$1 AND expires_at>clock_timestamp()`, id, ttl.Seconds())
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return count == 1, tx.Commit()
}
func (r *accountSchedulingPoolRepository) ReleaseLease(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM account_scheduling_leases WHERE lease_id=$1", id)
	return err
}

// createSchedulingPoolMember runs in the account creation transaction so a new
// imported account cannot be observed before its pool membership exists.
func createSchedulingPoolMember(ctx context.Context, client *dbent.Client, a *service.Account) error {
	if a.SchedulingPoolID == nil {
		return nil
	}
	if a.Platform != service.PlatformOpenAI || a.Type != service.AccountTypeOAuth || a.IsShadow() {
		return fmt.Errorf("scheduling pools support OpenAI OAuth parent accounts")
	}
	if _, err := client.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", accountSchedulingLockID); err != nil {
		return err
	}
	exists, err := client.AccountSchedulingPool.Query().Where(accountschedulingpool.IDEQ(*a.SchedulingPoolID)).Exist(ctx)
	if err != nil {
		return err
	}
	if !exists {
		return service.ErrAccountSchedulingPoolNotFound
	}
	_, err = client.ExecContext(ctx, `INSERT INTO account_scheduling_members(account_id,pool_id,position) VALUES($1,$2,(SELECT COALESCE(MAX(position),0)+1 FROM account_scheduling_members))`, a.ID, *a.SchedulingPoolID)
	return err
}
