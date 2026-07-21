//go:build integration

// Package routes — inbox_repository_integration_test.go
//
// 通用信箱 Repository 的 Postgres 集成测试（tasks §2.15 / §13.2）。复用本包已有的
// PG testcontainer 脚手架（setupRoutesPG，已应用全部迁移，含 184_add_general_inbox.sql）。
//
// 放在 routes 包而非 inbox 包的原因：inbox 的测试若 import repository 会形成导入环
// （repository → service → inbox）。routes 包已依赖 inbox 且持有 PG 脚手架，天然合适。
//
// 覆盖：单播 dedup 幂等 / seq 主键冲突、广播 dedup、ack 单调 + 懒初始化、
// catchup 分页 + 未 ack 派生、30 天保留删除、管理端审计分页。
package routes

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/inbox"
)

func inboxJSON(s string) json.RawMessage { return json.RawMessage(s) }

// inboxCreateUser 用 ent 建用户（direct_messages / user_inbox_state 有 users 外键）。
func inboxCreateUser(t *testing.T, ent *dbent.Client, email string) int64 {
	t.Helper()
	u, err := ent.User.Create().
		SetEmail(email).
		SetPasswordHash("test-hash").
		SetRole("user").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	return u.ID
}

func newInboxRepo(t *testing.T) (inbox.Repository, *sql.DB, *dbent.Client) {
	db, ent := setupRoutesPG(t)
	return inbox.NewRepository(db), db, ent
}

func TestInboxRepo_DirectDedupAndSeqConflict(t *testing.T) {
	repo, _, ent := newInboxRepo(t)
	ctx := context.Background()
	uid := inboxCreateUser(t, ent, "inbox-dedup@example.com")

	in := inbox.PublishDirectInput{RecipientID: uid, Namespace: "support_ticket", DedupKey: "k1", Payload: inboxJSON(`{"a":1}`)}

	created, err := repo.InsertDirectMessage(ctx, 1000, in)
	if err != nil || !created {
		t.Fatalf("首次插入应成功: created=%v err=%v", created, err)
	}

	// 同 (user,ns,dedup) 幂等命中：不同 seq，ON CONFLICT DO NOTHING → created=false。
	created, err = repo.InsertDirectMessage(ctx, 1001, in)
	if err != nil || created {
		t.Fatalf("dedup 命中应 created=false err=nil: created=%v err=%v", created, err)
	}

	// 相同 seq、不同 dedup → 主键冲突 → ErrSeqConflict。
	in2 := in
	in2.DedupKey = "k2"
	if _, err = repo.InsertDirectMessage(ctx, 1000, in2); err != inbox.ErrSeqConflict {
		t.Fatalf("seq 主键冲突应返回 ErrSeqConflict, got %v", err)
	}
}

func TestInboxRepo_BroadcastDedup(t *testing.T) {
	repo, _, _ := newInboxRepo(t)
	ctx := context.Background()

	in := inbox.PublishBroadcastInput{Namespace: "announcement", DedupKey: "bc1", Targeting: inboxJSON(`{"op":"all_users"}`), Payload: inboxJSON(`{"x":1}`)}
	created, err := repo.InsertBroadcast(ctx, 2000, in)
	if err != nil || !created {
		t.Fatalf("首次广播插入应成功: created=%v err=%v", created, err)
	}
	created, err = repo.InsertBroadcast(ctx, 2001, in)
	if err != nil || created {
		t.Fatalf("广播 dedup 命中应 created=false: created=%v err=%v", created, err)
	}
}

func TestInboxRepo_AckMonotonicAndLazyInit(t *testing.T) {
	repo, _, ent := newInboxRepo(t)
	ctx := context.Background()
	uid := inboxCreateUser(t, ent, "inbox-ack@example.com")

	if _, found, err := repo.GetInboxState(ctx, uid); err != nil || found {
		t.Fatalf("初始应无水位: found=%v err=%v", found, err)
	}
	if err := repo.InitInboxState(ctx, uid, 500); err != nil {
		t.Fatalf("init state: %v", err)
	}
	acked, found, err := repo.GetInboxState(ctx, uid)
	if err != nil || !found || acked != 500 {
		t.Fatalf("懒初始化后应为 500: acked=%d found=%v err=%v", acked, found, err)
	}

	// Ack 单调：低值不回退，高值抬升。
	if err := repo.Ack(ctx, uid, 300); err != nil {
		t.Fatalf("ack 300: %v", err)
	}
	if acked, _, _ = repo.GetInboxState(ctx, uid); acked != 500 {
		t.Fatalf("ack(300) 不应回退, got %d", acked)
	}
	if err := repo.Ack(ctx, uid, 800); err != nil {
		t.Fatalf("ack 800: %v", err)
	}
	if acked, _, _ = repo.GetInboxState(ctx, uid); acked != 800 {
		t.Fatalf("ack(800) 应抬升到 800, got %d", acked)
	}
}

func TestInboxRepo_ListDirectSincePagination(t *testing.T) {
	repo, _, ent := newInboxRepo(t)
	ctx := context.Background()
	uid := inboxCreateUser(t, ent, "inbox-page@example.com")

	base := int64(3000)
	for i := int64(0); i < 5; i++ {
		in := inbox.PublishDirectInput{RecipientID: uid, Namespace: "n", DedupKey: "p" + strconv.FormatInt(i, 10), Payload: inboxJSON(`{}`)}
		if _, err := repo.InsertDirectMessage(ctx, base+i, in); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	// since=base（严格大于），limit=3 → base+1,+2,+3。
	msgs, err := repo.ListDirectSince(ctx, uid, base, 3)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(msgs) != 3 || msgs[0].Seq != base+1 || msgs[2].Seq != base+3 {
		t.Fatalf("分页/排序异常: %+v", msgs)
	}

	// 未 ack 派生：水位=base+2 → (base+2,max] 为 base+3,+4。
	seqs, err := repo.UnackedDirectSeqs(ctx, uid, base+2, base+100, 10)
	if err != nil {
		t.Fatalf("unacked: %v", err)
	}
	if len(seqs) != 2 || seqs[0] != base+3 || seqs[1] != base+4 {
		t.Fatalf("未 ack seq 异常: %v", seqs)
	}
}

func TestInboxRepo_RetentionDelete(t *testing.T) {
	repo, db, _ := newInboxRepo(t)
	ctx := context.Background()

	in := inbox.PublishBroadcastInput{Namespace: "retn", DedupKey: "old1", Targeting: inboxJSON(`{"op":"all_users"}`), Payload: inboxJSON(`{}`)}
	if _, err := repo.InsertBroadcast(ctx, 4000, in); err != nil {
		t.Fatalf("insert: %v", err)
	}
	// 把 created_at 改到 40 天前，模拟过期。
	if _, err := db.ExecContext(ctx, `UPDATE broadcasts SET created_at = now() - interval '40 days' WHERE seq = 4000`); err != nil {
		t.Fatalf("age broadcast: %v", err)
	}

	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	n, err := repo.DeleteExpiredBroadcasts(ctx, cutoff, 1000)
	if err != nil {
		t.Fatalf("delete expired: %v", err)
	}
	if n < 1 {
		t.Fatalf("应至少删除 1 条过期广播, got %d", n)
	}

	items, _, err := repo.ListBroadcastsPaged(ctx, "retn", 100, 0)
	if err != nil {
		t.Fatalf("list paged: %v", err)
	}
	for _, b := range items {
		if b.Seq == 4000 {
			t.Fatal("过期广播 4000 应已被删除")
		}
	}
}

func TestInboxRepo_AuditPaged(t *testing.T) {
	repo, _, ent := newInboxRepo(t)
	ctx := context.Background()
	uid := inboxCreateUser(t, ent, "inbox-audit@example.com")

	_, _ = repo.InsertBroadcast(ctx, 5000, inbox.PublishBroadcastInput{Namespace: "audit_ns", DedupKey: "a1", Targeting: inboxJSON(`{"op":"all_users"}`), Payload: inboxJSON(`{}`)})
	_, _ = repo.InsertBroadcast(ctx, 5001, inbox.PublishBroadcastInput{Namespace: "audit_ns", DedupKey: "a2", Targeting: inboxJSON(`{"op":"all_users"}`), Payload: inboxJSON(`{}`)})
	_, _ = repo.InsertDirectMessage(ctx, 5100, inbox.PublishDirectInput{RecipientID: uid, Namespace: "audit_ns", DedupKey: "d1", Payload: inboxJSON(`{}`)})

	bcs, total, err := repo.ListBroadcastsPaged(ctx, "audit_ns", 10, 0)
	if err != nil || total < 2 || len(bcs) < 2 {
		t.Fatalf("广播审计分页异常: total=%d len=%d err=%v", total, len(bcs), err)
	}
	if bcs[0].Seq < bcs[len(bcs)-1].Seq {
		t.Fatalf("广播审计应按 seq 倒序, got %+v", bcs)
	}

	dms, dtotal, err := repo.ListDirectMessagesPaged(ctx, "audit_ns", uid, 10, 0)
	if err != nil || dtotal < 1 || len(dms) < 1 {
		t.Fatalf("单播审计分页异常: total=%d len=%d err=%v", dtotal, len(dms), err)
	}
	if dms[0].UserID != uid {
		t.Fatalf("单播审计 user 过滤异常: %+v", dms[0])
	}
}
