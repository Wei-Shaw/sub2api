//go:build integration

package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

// SupportTicketRepoSuite 覆盖 §3.5 的 Repository 用例：
//   - 创建 + GetByID 回放（含 chat_context）
//   - ListByUser：分页 + 不返回 chat_context
//   - ListAdmin：status / priority / category / q 过滤组合 + priority CASE-DESC 排序
//   - AppendReply / ListReplies：按 created_at ASC
//   - UpdateFields：status/priority/closed_at 部分更新
type SupportTicketRepoSuite struct {
	suite.Suite
	ctx    context.Context
	client *dbent.Client
	repo   *supportTicketRepository
}

func (s *SupportTicketRepoSuite) SetupTest() {
	tx := testEntTx(s.T())
	s.ctx = dbent.NewTxContext(context.Background(), tx)
	s.client = tx.Client()
	s.repo = NewSupportTicketRepository(s.client).(*supportTicketRepository)
}

func TestSupportTicketRepoSuite(t *testing.T) {
	suite.Run(t, new(SupportTicketRepoSuite))
}

// createUser 创建一个测试用户，返回 ID。直接用 ent 构建避免依赖 user_repo 的语义检查。
func (s *SupportTicketRepoSuite) createUser(email string) int64 {
	u, err := s.client.User.Create().
		SetEmail(email).
		SetPasswordHash("test-password-hash").
		Save(s.ctx)
	s.Require().NoError(err, "create user")
	return u.ID
}

// createTicket 是测试 helper：建一张工单并返回 service struct。
func (s *SupportTicketRepoSuite) createTicket(t *service.SupportTicket) *service.SupportTicket {
	if t.UserID == 0 {
		t.UserID = s.createUser("ticket-owner-" + time.Now().Format(time.RFC3339Nano) + "@example.com")
	}
	if t.Title == "" {
		t.Title = "test-title"
	}
	if t.Content == "" {
		t.Content = "test-content"
	}
	if t.Category == "" {
		t.Category = "其他"
	}
	s.Require().NoError(s.repo.Create(s.ctx, t))
	s.Require().NotZero(t.ID, "Create should set ID")
	return t
}

// --- Create / GetByID ---

func (s *SupportTicketRepoSuite) TestCreate_AndGetByID_RoundTripsChatContext() {
	chat := "user: hello\nassistant: hi"
	t := &service.SupportTicket{
		Title:       "需要帮助",
		Content:     "正文",
		Category:    "API",
		Priority:    service.SupportTicketPriorityHigh,
		ChatContext: &chat,
	}
	s.createTicket(t)

	got, err := s.repo.GetByID(s.ctx, t.ID)
	s.Require().NoError(err)
	s.Require().Equal(t.Title, got.Title)
	s.Require().Equal(service.SupportTicketStatusOpen, got.Status, "default status should be open")
	s.Require().Equal(service.SupportTicketPriorityHigh, got.Priority)
	s.Require().NotNil(got.ChatContext, "GetByID should return chat_context")
	s.Require().Equal(chat, *got.ChatContext)
	s.Require().NotZero(got.CreatedAt)
}

func (s *SupportTicketRepoSuite) TestGetByID_NotFound() {
	_, err := s.repo.GetByID(s.ctx, 99999999)
	s.Require().Error(err)
	s.Require().ErrorIs(err, service.ErrSupportTicketNotFound)
}

// --- ListByUser ---

func (s *SupportTicketRepoSuite) TestListByUser_FiltersByOwnerAndOmitsChatContext() {
	owner := s.createUser("owner@example.com")
	other := s.createUser("other@example.com")

	chat := "secret"
	s.createTicket(&service.SupportTicket{UserID: owner, Title: "owner-1", Category: "API", ChatContext: &chat})
	s.createTicket(&service.SupportTicket{UserID: owner, Title: "owner-2", Category: "Bug"})
	s.createTicket(&service.SupportTicket{UserID: other, Title: "other-1", Category: "API"})

	items, page, err := s.repo.ListByUser(s.ctx, owner, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err)
	s.Require().Equal(int64(2), page.Total, "should only count owner's tickets")
	s.Require().Len(items, 2)

	// 全部都属于 owner；chat_context 必须为 nil（即使 owner-1 数据库里非空）
	for _, it := range items {
		s.Require().Equal(owner, it.UserID)
		s.Require().Nil(it.ChatContext, "list view must drop chat_context")
	}
}

func (s *SupportTicketRepoSuite) TestListByUser_OrdersByCreatedAtDesc() {
	owner := s.createUser("order-owner@example.com")

	a := s.createTicket(&service.SupportTicket{UserID: owner, Title: "first"})
	time.Sleep(5 * time.Millisecond) // 强制 created_at 单调
	b := s.createTicket(&service.SupportTicket{UserID: owner, Title: "second"})

	items, _, err := s.repo.ListByUser(s.ctx, owner, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err)
	s.Require().Len(items, 2)
	s.Require().Equal(b.ID, items[0].ID, "newer ticket should be first")
	s.Require().Equal(a.ID, items[1].ID)
}

// --- ListAdmin filter combinations ---

func (s *SupportTicketRepoSuite) TestListAdmin_FilterStatusPriorityCategoryAndQ() {
	owner := s.createUser("admin-list@example.com")

	// 准备 4 张工单，覆盖 status/priority/category 多组合
	t1 := s.createTicket(&service.SupportTicket{
		UserID: owner, Title: "充值失败问题",
		Category: "充值", Priority: service.SupportTicketPriorityHigh,
	})
	t2 := s.createTicket(&service.SupportTicket{
		UserID: owner, Title: "API 504",
		Content: "调用接口偶发 504", Category: "API", Priority: service.SupportTicketPriorityNormal,
	})
	t3 := s.createTicket(&service.SupportTicket{
		UserID: owner, Title: "账号锁定",
		Category: "账号", Priority: service.SupportTicketPriorityLow,
	})
	t4 := s.createTicket(&service.SupportTicket{
		UserID: owner, Title: "Bug 报告",
		Category: "Bug", Priority: service.SupportTicketPriorityNormal,
	})
	// 把 t3 关闭，便于测 status 过滤
	closedAt := time.Now().UTC()
	closed := service.SupportTicketStatusClosed
	s.Require().NoError(s.repo.UpdateFields(s.ctx, t3.ID, service.SupportTicketPatch{
		Status:   &closed,
		ClosedAt: &closedAt,
	}))

	// 1. status 过滤：只返回 closed
	items, _, err := s.repo.ListAdmin(s.ctx, service.SupportTicketListFilters{
		Status: service.SupportTicketStatusClosed,
	}, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err)
	s.Require().Len(items, 1)
	s.Require().Equal(t3.ID, items[0].ID)

	// 2. priority + category 同时过滤：只匹配 t4（normal + Bug）
	items, _, err = s.repo.ListAdmin(s.ctx, service.SupportTicketListFilters{
		Priority: service.SupportTicketPriorityNormal,
		Category: "Bug",
	}, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err)
	s.Require().Len(items, 1)
	s.Require().Equal(t4.ID, items[0].ID)

	// 3. q 关键词命中 content（不是 title）：t2 的 content 含 504
	items, _, err = s.repo.ListAdmin(s.ctx, service.SupportTicketListFilters{
		Search: "504",
	}, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err)
	s.Require().Len(items, 1)
	s.Require().Equal(t2.ID, items[0].ID)

	// 4. q 关键词命中 title：t1 的 title 含 "充值"
	items, _, err = s.repo.ListAdmin(s.ctx, service.SupportTicketListFilters{
		Search: "充值",
	}, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err)
	s.Require().Len(items, 1)
	s.Require().Equal(t1.ID, items[0].ID)

	// 5. user_id 过滤：传入其他用户拿不到任何工单
	otherUser := s.createUser("admin-list-other@example.com")
	items, _, err = s.repo.ListAdmin(s.ctx, service.SupportTicketListFilters{
		UserID: &otherUser,
	}, pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err)
	s.Require().Empty(items)
}

func (s *SupportTicketRepoSuite) TestListAdmin_PriorityCaseOrderingHighFirst() {
	owner := s.createUser("admin-order@example.com")

	// 故意按 normal → high → low 顺序创建，确保排序结果不是按 created_at 决定。
	tNormal := s.createTicket(&service.SupportTicket{
		UserID: owner, Title: "p-normal", Priority: service.SupportTicketPriorityNormal,
	})
	time.Sleep(5 * time.Millisecond)
	tHigh := s.createTicket(&service.SupportTicket{
		UserID: owner, Title: "p-high", Priority: service.SupportTicketPriorityHigh,
	})
	time.Sleep(5 * time.Millisecond)
	tLow := s.createTicket(&service.SupportTicket{
		UserID: owner, Title: "p-low", Priority: service.SupportTicketPriorityLow,
	})

	items, _, err := s.repo.ListAdmin(s.ctx, service.SupportTicketListFilters{},
		pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(len(items), 3)

	// 头三条必须按 high → normal → low（CASE 表达式权重 3>2>1）
	got := []int64{items[0].ID, items[1].ID, items[2].ID}
	want := []int64{tHigh.ID, tNormal.ID, tLow.ID}
	s.Require().Equal(want, got, "expected priority CASE order high → normal → low")
}

func (s *SupportTicketRepoSuite) TestListAdmin_OmitsChatContext() {
	owner := s.createUser("admin-chat@example.com")
	chat := strings.Repeat("x", 1000)
	s.createTicket(&service.SupportTicket{
		UserID: owner, Title: "with-chat", ChatContext: &chat,
	})

	items, _, err := s.repo.ListAdmin(s.ctx, service.SupportTicketListFilters{},
		pagination.PaginationParams{Page: 1, PageSize: 10})
	s.Require().NoError(err)
	s.Require().GreaterOrEqual(len(items), 1)
	for _, it := range items {
		s.Require().Nil(it.ChatContext, "ListAdmin must drop chat_context")
	}
}

// --- AppendReply / ListReplies ---

func (s *SupportTicketRepoSuite) TestAppendReply_AndListRepliesAscending() {
	owner := s.createUser("reply-owner@example.com")
	t := s.createTicket(&service.SupportTicket{UserID: owner, Title: "with replies"})

	// 两条回复，强制时间间隔，验证 ASC 排序
	r1 := &service.SupportTicketReply{
		TicketID: t.ID, AuthorID: &owner, IsAdmin: false, Content: "first",
	}
	s.Require().NoError(s.repo.AppendReply(s.ctx, r1))
	s.Require().NotZero(r1.ID)
	s.Require().NotZero(r1.CreatedAt)

	time.Sleep(5 * time.Millisecond)
	adminID := s.createUser("admin@example.com")
	r2 := &service.SupportTicketReply{
		TicketID: t.ID, AuthorID: &adminID, IsAdmin: true, Content: "second",
	}
	s.Require().NoError(s.repo.AppendReply(s.ctx, r2))

	items, err := s.repo.ListReplies(s.ctx, t.ID)
	s.Require().NoError(err)
	s.Require().Len(items, 2)
	s.Require().Equal(r1.ID, items[0].ID, "earliest first")
	s.Require().Equal(r2.ID, items[1].ID)
	s.Require().False(items[0].IsAdmin)
	s.Require().True(items[1].IsAdmin)
}

func (s *SupportTicketRepoSuite) TestAppendReply_NilAuthorAllowedAfterUserDeletion() {
	owner := s.createUser("orphan-owner@example.com")
	t := s.createTicket(&service.SupportTicket{UserID: owner, Title: "orphan"})

	// AuthorID == nil 表示作者已被删除，FK ON DELETE SET NULL 也会导致这种情况；
	// repo 必须接受。
	r := &service.SupportTicketReply{
		TicketID: t.ID, AuthorID: nil, IsAdmin: true, Content: "from deleted admin",
	}
	s.Require().NoError(s.repo.AppendReply(s.ctx, r))

	items, err := s.repo.ListReplies(s.ctx, t.ID)
	s.Require().NoError(err)
	s.Require().Len(items, 1)
	s.Require().Nil(items[0].AuthorID)
	s.Require().True(items[0].IsAdmin, "is_admin snapshot survives author deletion")
}

// --- UpdateFields ---

func (s *SupportTicketRepoSuite) TestUpdateFields_PartialPatch() {
	owner := s.createUser("update@example.com")
	t := s.createTicket(&service.SupportTicket{
		UserID: owner, Title: "update", Priority: service.SupportTicketPriorityNormal,
	})

	high := service.SupportTicketPriorityHigh
	s.Require().NoError(s.repo.UpdateFields(s.ctx, t.ID, service.SupportTicketPatch{
		Priority: &high,
	}))

	got, err := s.repo.GetByID(s.ctx, t.ID)
	s.Require().NoError(err)
	s.Require().Equal(service.SupportTicketPriorityHigh, got.Priority)
	s.Require().Equal(service.SupportTicketStatusOpen, got.Status, "status untouched")
}

func (s *SupportTicketRepoSuite) TestUpdateFields_NotFound() {
	high := service.SupportTicketPriorityHigh
	err := s.repo.UpdateFields(s.ctx, 99999999, service.SupportTicketPatch{
		Priority: &high,
	})
	s.Require().Error(err)
	s.Require().ErrorIs(err, service.ErrSupportTicketNotFound)
}

func (s *SupportTicketRepoSuite) TestUpdateFields_EmptyPatchIsNoop() {
	owner := s.createUser("noop@example.com")
	t := s.createTicket(&service.SupportTicket{UserID: owner, Title: "noop"})

	// 先取一次基准 updated_at（避免 Create 返回值与 PG 存储值之间的微秒精度差异
	// 直接用作 expected）。
	baseline, err := s.repo.GetByID(s.ctx, t.ID)
	s.Require().NoError(err)

	// 等待几毫秒，确保如果发生了 UPDATE，updated_at 至少跳变 >1ms；
	// 然后调一次空 patch。
	time.Sleep(10 * time.Millisecond)
	s.Require().NoError(s.repo.UpdateFields(s.ctx, t.ID, service.SupportTicketPatch{}))

	got, err := s.repo.GetByID(s.ctx, t.ID)
	s.Require().NoError(err)
	// 没有触发 SQL UPDATE，updated_at 不应改变（允许 PG 存储微秒级抖动）。
	s.Require().WithinDuration(baseline.UpdatedAt, got.UpdatedAt, 100*time.Microsecond,
		"empty patch must not bump updated_at")
}
