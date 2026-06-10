// Package service — support_ticket_service_test.go
//
// 单测覆盖 SupportTicketService 的业务规则、状态机与权限边界。
//
// 设计：注入轻量内存桩 repo + settings reader，不依赖数据库；事务路径下走
// "entClient = nil 退化为非事务两步" 分支（覆盖核心语义即可，事务原子性由
// support_ticket_repo_integration_test.go 在真实 PG 上验证）。
package service

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"

	"github.com/stretchr/testify/require"
)

// ----------------------------------------------------------------------------
// stubs
// ----------------------------------------------------------------------------

// supportTicketSettingsStub 实现 SupportTicketSettingsReader。
type supportTicketSettingsStub struct {
	enabled         bool
	categories      []string
	defaultPriority string
}

func (s *supportTicketSettingsStub) GetSupportTicketRuntime(context.Context) SupportTicketRuntime {
	return SupportTicketRuntime{
		Enabled:         s.enabled,
		Categories:      s.categories,
		DefaultPriority: s.defaultPriority,
	}
}

func newDefaultSettingsStub(enabled bool) *supportTicketSettingsStub {
	return &supportTicketSettingsStub{
		enabled:         enabled,
		categories:      []string{"充值", "账号", "API", "Bug", "其他"},
		defaultPriority: SupportTicketPriorityNormal,
	}
}

// supportTicketRepoStub 是单测专用的内存 repo。
//
// 行为：
//   - Create / AppendReply 自增 ID 并填充时间戳。
//   - List* 返回所有命中条目（按插入顺序），列表视图强制 ChatContext = nil
//     与真实 repo 行为一致。
//   - GetByID 返回深拷贝（避免单测错误地依赖共享指针）。
type supportTicketRepoStub struct {
	tickets    []*SupportTicket
	replies    []*SupportTicketReply
	idCounter  atomic.Int64
	rid        atomic.Int64
	updateLog  []SupportTicketPatch
	overrideTx bool
}

func newSupportTicketRepoStub() *supportTicketRepoStub {
	return &supportTicketRepoStub{}
}

func (s *supportTicketRepoStub) Create(_ context.Context, t *SupportTicket) error {
	t.ID = s.idCounter.Add(1)
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = now
	}
	if t.Status == "" {
		t.Status = SupportTicketStatusOpen
	}
	if t.Priority == "" {
		t.Priority = SupportTicketPriorityNormal
	}
	cp := *t
	s.tickets = append(s.tickets, &cp)
	return nil
}

func (s *supportTicketRepoStub) GetByID(_ context.Context, id int64) (*SupportTicket, error) {
	for _, t := range s.tickets {
		if t.ID == id {
			cp := *t
			return &cp, nil
		}
	}
	return nil, ErrSupportTicketNotFound
}

func (s *supportTicketRepoStub) ListByUser(
	_ context.Context,
	userID int64,
	params pagination.PaginationParams,
) ([]SupportTicket, *pagination.PaginationResult, error) {
	out := make([]SupportTicket, 0)
	for _, t := range s.tickets {
		if t.UserID == userID {
			cp := *t
			cp.ChatContext = nil // list 视图行为
			out = append(out, cp)
		}
	}
	return out, &pagination.PaginationResult{Total: int64(len(out)), Page: params.Page, PageSize: params.Limit()}, nil
}

func (s *supportTicketRepoStub) ListAdmin(
	_ context.Context,
	filters SupportTicketListFilters,
	params pagination.PaginationParams,
) ([]SupportTicket, *pagination.PaginationResult, error) {
	out := make([]SupportTicket, 0)
	for _, t := range s.tickets {
		if filters.UserID != nil && t.UserID != *filters.UserID {
			continue
		}
		if filters.Status != "" && t.Status != filters.Status {
			continue
		}
		if filters.Priority != "" && t.Priority != filters.Priority {
			continue
		}
		if filters.Category != "" && t.Category != filters.Category {
			continue
		}
		if kw := strings.TrimSpace(filters.Search); kw != "" {
			lc := strings.ToLower(kw)
			if !strings.Contains(strings.ToLower(t.Title), lc) && !strings.Contains(strings.ToLower(t.Content), lc) {
				continue
			}
		}
		cp := *t
		cp.ChatContext = nil
		out = append(out, cp)
	}
	return out, &pagination.PaginationResult{Total: int64(len(out)), Page: params.Page, PageSize: params.Limit()}, nil
}

func (s *supportTicketRepoStub) UpdateFields(_ context.Context, id int64, patch SupportTicketPatch) error {
	s.updateLog = append(s.updateLog, patch)
	for _, t := range s.tickets {
		if t.ID == id {
			if patch.Status != nil {
				t.Status = *patch.Status
			}
			if patch.Priority != nil {
				t.Priority = *patch.Priority
			}
			if patch.Category != nil {
				t.Category = *patch.Category
			}
			if patch.ClosedAt != nil {
				cp := *patch.ClosedAt
				t.ClosedAt = &cp
			}
			t.UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return ErrSupportTicketNotFound
}

func (s *supportTicketRepoStub) AppendReply(_ context.Context, reply *SupportTicketReply) error {
	reply.ID = s.rid.Add(1)
	reply.CreatedAt = time.Now().UTC()
	cp := *reply
	s.replies = append(s.replies, &cp)
	return nil
}

func (s *supportTicketRepoStub) ListReplies(_ context.Context, ticketID int64) ([]SupportTicketReply, error) {
	out := make([]SupportTicketReply, 0)
	for _, r := range s.replies {
		if r.TicketID == ticketID {
			out = append(out, *r)
		}
	}
	return out, nil
}

// helper：构造 service + repo + settings stub 三件套。entClient = nil → 退化为非事务两步执行。
func newSupportTicketServiceForTest(t *testing.T, enabled bool) (*SupportTicketService, *supportTicketRepoStub, *supportTicketSettingsStub) {
	t.Helper()
	repo := newSupportTicketRepoStub()
	settings := newDefaultSettingsStub(enabled)
	svc := NewSupportTicketService(repo, settings, nil)
	// 固定 now 便于断言 closed_at。
	fixed := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixed }
	return svc, repo, settings
}

// ----------------------------------------------------------------------------
// CreateTicket
// ----------------------------------------------------------------------------

func TestSupportTicketService_CreateTicket_HappyPath(t *testing.T) {
	svc, repo, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()

	cc := "  some chat context  "
	tk, err := svc.CreateTicket(ctx, CreateTicketInput{
		UserID:      42,
		Title:       "  登录失败  ",
		Content:     "  详细描述  ",
		Category:    "账号",
		ChatContext: &cc,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), tk.ID)
	require.Equal(t, "登录失败", tk.Title)
	require.Equal(t, "详细描述", tk.Content)
	require.Equal(t, "账号", tk.Category)
	require.Equal(t, SupportTicketStatusOpen, tk.Status)
	require.Equal(t, SupportTicketPriorityNormal, tk.Priority) // 默认值
	require.NotNil(t, tk.ChatContext)
	require.Equal(t, "some chat context", *tk.ChatContext)
	require.Len(t, repo.tickets, 1)
}

func TestSupportTicketService_CreateTicket_FeatureDisabled(t *testing.T) {
	svc, _, _ := newSupportTicketServiceForTest(t, false)

	_, err := svc.CreateTicket(context.Background(), CreateTicketInput{
		UserID:   1,
		Title:    "foo",
		Content:  "bar",
		Category: "账号",
	})
	require.ErrorIs(t, err, ErrSupportFeatureDisabled)
	require.True(t, infraerrors.IsNotFound(err))
}

func TestSupportTicketService_CreateTicket_ValidationErrors(t *testing.T) {
	svc, _, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()

	cases := []struct {
		name   string
		input  CreateTicketInput
		wantIs error
	}{
		{"empty title", CreateTicketInput{UserID: 1, Title: "  ", Content: "x", Category: "账号"}, ErrSupportTicketTitleRequired},
		{"title too long", CreateTicketInput{UserID: 1, Title: strings.Repeat("a", SupportTicketTitleMaxLen+1), Content: "x", Category: "账号"}, ErrSupportTicketTitleTooLong},
		{"empty content", CreateTicketInput{UserID: 1, Title: "t", Content: "  ", Category: "账号"}, ErrSupportTicketContentRequired},
		{"content too long", CreateTicketInput{UserID: 1, Title: "t", Content: strings.Repeat("a", SupportTicketContentMaxLen+1), Category: "账号"}, ErrSupportTicketContentTooLong},
		{"invalid category", CreateTicketInput{UserID: 1, Title: "t", Content: "c", Category: "未知分类"}, ErrSupportTicketCategoryInvalid},
		{"empty category", CreateTicketInput{UserID: 1, Title: "t", Content: "c", Category: ""}, ErrSupportTicketCategoryInvalid},
		{"invalid priority", CreateTicketInput{UserID: 1, Title: "t", Content: "c", Category: "账号", Priority: "urgent"}, ErrSupportTicketPriorityInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateTicket(ctx, tc.input)
			require.ErrorIs(t, err, tc.wantIs)
		})
	}
}

func TestSupportTicketService_CreateTicket_ChatContextTooLong(t *testing.T) {
	svc, _, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()

	cc := strings.Repeat("a", SupportTicketChatContextMaxLen+1)
	_, err := svc.CreateTicket(ctx, CreateTicketInput{
		UserID: 1, Title: "t", Content: "c", Category: "账号", ChatContext: &cc,
	})
	require.ErrorIs(t, err, ErrSupportTicketChatContextTooLong)
}

func TestSupportTicketService_CreateTicket_BlankChatContextStoredAsNil(t *testing.T) {
	svc, repo, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()

	blank := "   "
	tk, err := svc.CreateTicket(ctx, CreateTicketInput{
		UserID: 1, Title: "t", Content: "c", Category: "账号", ChatContext: &blank,
	})
	require.NoError(t, err)
	require.Nil(t, tk.ChatContext)
	require.Nil(t, repo.tickets[0].ChatContext)
}

// ----------------------------------------------------------------------------
// GetUserTicket / ListUserTickets
// ----------------------------------------------------------------------------

func TestSupportTicketService_GetUserTicket_OwnerCheck(t *testing.T) {
	svc, repo, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()

	owner := int64(7)
	tk := &SupportTicket{UserID: owner, Title: "x", Content: "y", Category: "账号", Status: SupportTicketStatusOpen, Priority: SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(ctx, tk))

	// owner 取得
	got, err := svc.GetUserTicket(ctx, owner, tk.ID)
	require.NoError(t, err)
	require.Equal(t, tk.ID, got.Ticket.ID)

	// 非 owner → 视为不存在
	_, err = svc.GetUserTicket(ctx, owner+1, tk.ID)
	require.ErrorIs(t, err, ErrSupportTicketNotFound)

	// 工单不存在 → 同样 NotFound
	_, err = svc.GetUserTicket(ctx, owner, 99999)
	require.ErrorIs(t, err, ErrSupportTicketNotFound)
}

func TestSupportTicketService_ListUserTickets_DisabledFeature(t *testing.T) {
	svc, _, _ := newSupportTicketServiceForTest(t, false)
	_, _, err := svc.ListUserTickets(context.Background(), 1, pagination.DefaultPagination())
	require.ErrorIs(t, err, ErrSupportFeatureDisabled)
}

// ----------------------------------------------------------------------------
// AppendUserReply
// ----------------------------------------------------------------------------

func TestSupportTicketService_AppendUserReply_HappyPath(t *testing.T) {
	svc, repo, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()

	owner := int64(5)
	tk := &SupportTicket{UserID: owner, Title: "x", Content: "y", Category: "账号", Status: SupportTicketStatusOpen, Priority: SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(ctx, tk))

	reply, err := svc.AppendUserReply(ctx, owner, tk.ID, "  我也遇到了  ")
	require.NoError(t, err)
	require.Equal(t, tk.ID, reply.TicketID)
	require.False(t, reply.IsAdmin)
	require.NotNil(t, reply.AuthorID)
	require.Equal(t, owner, *reply.AuthorID)
	require.Equal(t, "我也遇到了", reply.Content)
	require.Len(t, repo.replies, 1)

	// AppendUserReply 不应触发 status 跃迁
	got, err := repo.GetByID(ctx, tk.ID)
	require.NoError(t, err)
	require.Equal(t, SupportTicketStatusOpen, got.Status)
}

func TestSupportTicketService_AppendUserReply_NotOwner(t *testing.T) {
	svc, repo, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()
	tk := &SupportTicket{UserID: 1, Title: "x", Content: "y", Category: "账号", Status: SupportTicketStatusOpen, Priority: SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(ctx, tk))

	_, err := svc.AppendUserReply(ctx, 2, tk.ID, "hi")
	require.ErrorIs(t, err, ErrSupportTicketNotFound)
}

func TestSupportTicketService_AppendUserReply_Closed(t *testing.T) {
	svc, repo, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()
	tk := &SupportTicket{UserID: 1, Title: "x", Content: "y", Category: "账号", Status: SupportTicketStatusClosed, Priority: SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(ctx, tk))

	_, err := svc.AppendUserReply(ctx, 1, tk.ID, "hi")
	require.ErrorIs(t, err, ErrSupportTicketClosed)
	require.True(t, infraerrors.IsConflict(err))
}

func TestSupportTicketService_AppendUserReply_EmptyContent(t *testing.T) {
	svc, repo, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()
	tk := &SupportTicket{UserID: 1, Title: "x", Content: "y", Category: "账号", Status: SupportTicketStatusOpen, Priority: SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(ctx, tk))

	_, err := svc.AppendUserReply(ctx, 1, tk.ID, "   ")
	require.ErrorIs(t, err, ErrSupportTicketReplyContentRequired)
}

// ----------------------------------------------------------------------------
// CloseUserTicket
// ----------------------------------------------------------------------------

func TestSupportTicketService_CloseUserTicket_SetsClosedAtAndStatus(t *testing.T) {
	svc, repo, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()
	tk := &SupportTicket{UserID: 1, Title: "x", Content: "y", Category: "账号", Status: SupportTicketStatusOpen, Priority: SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(ctx, tk))

	require.NoError(t, svc.CloseUserTicket(ctx, 1, tk.ID))

	got, _ := repo.GetByID(ctx, tk.ID)
	require.Equal(t, SupportTicketStatusClosed, got.Status)
	require.NotNil(t, got.ClosedAt)
}

func TestSupportTicketService_CloseUserTicket_AlreadyClosedReturnsConflict(t *testing.T) {
	svc, repo, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()
	tk := &SupportTicket{UserID: 1, Title: "x", Content: "y", Category: "账号", Status: SupportTicketStatusClosed, Priority: SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(ctx, tk))

	err := svc.CloseUserTicket(ctx, 1, tk.ID)
	require.ErrorIs(t, err, ErrSupportTicketClosed)
}

func TestSupportTicketService_CloseUserTicket_NotOwner(t *testing.T) {
	svc, repo, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()
	tk := &SupportTicket{UserID: 1, Title: "x", Content: "y", Category: "账号", Status: SupportTicketStatusOpen, Priority: SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(ctx, tk))

	err := svc.CloseUserTicket(ctx, 2, tk.ID)
	require.ErrorIs(t, err, ErrSupportTicketNotFound)
}

// ----------------------------------------------------------------------------
// AppendAdminReply（含 open → in_progress 跃迁）
// ----------------------------------------------------------------------------

func TestSupportTicketService_AppendAdminReply_TransitionsOpenToInProgress(t *testing.T) {
	svc, repo, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()
	tk := &SupportTicket{UserID: 7, Title: "x", Content: "y", Category: "账号", Status: SupportTicketStatusOpen, Priority: SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(ctx, tk))

	reply, err := svc.AppendAdminReply(ctx, 99, tk.ID, "正在处理")
	require.NoError(t, err)
	require.True(t, reply.IsAdmin)

	got, _ := repo.GetByID(ctx, tk.ID)
	require.Equal(t, SupportTicketStatusInProgress, got.Status)
	require.Len(t, repo.replies, 1)
}

func TestSupportTicketService_AppendAdminReply_DoesNotTransitionInProgress(t *testing.T) {
	svc, repo, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()
	tk := &SupportTicket{UserID: 7, Title: "x", Content: "y", Category: "账号", Status: SupportTicketStatusInProgress, Priority: SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(ctx, tk))

	_, err := svc.AppendAdminReply(ctx, 99, tk.ID, "继续处理")
	require.NoError(t, err)
	got, _ := repo.GetByID(ctx, tk.ID)
	require.Equal(t, SupportTicketStatusInProgress, got.Status)

	// 应当只 INSERT reply，没有 UpdateFields 调用。
	require.Empty(t, repo.updateLog)
}

func TestSupportTicketService_AppendAdminReply_RejectsClosed(t *testing.T) {
	svc, repo, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()
	tk := &SupportTicket{UserID: 1, Title: "x", Content: "y", Category: "账号", Status: SupportTicketStatusClosed, Priority: SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(ctx, tk))

	_, err := svc.AppendAdminReply(ctx, 99, tk.ID, "hi")
	require.ErrorIs(t, err, ErrSupportTicketClosed)
	require.Empty(t, repo.replies)
}

func TestSupportTicketService_AppendAdminReply_FeatureDisabledStillWorks(t *testing.T) {
	// admin 路径不受 enabled 影响（spec 5.2）。
	svc, repo, _ := newSupportTicketServiceForTest(t, false)
	ctx := context.Background()
	tk := &SupportTicket{UserID: 7, Title: "x", Content: "y", Category: "账号", Status: SupportTicketStatusOpen, Priority: SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(ctx, tk))

	_, err := svc.AppendAdminReply(ctx, 99, tk.ID, "hi")
	require.NoError(t, err)
	got, _ := repo.GetByID(ctx, tk.ID)
	require.Equal(t, SupportTicketStatusInProgress, got.Status)
}

// ----------------------------------------------------------------------------
// PatchAdmin
// ----------------------------------------------------------------------------

func TestSupportTicketService_PatchAdmin_NoFieldsBadRequest(t *testing.T) {
	svc, repo, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()
	tk := &SupportTicket{UserID: 1, Title: "x", Content: "y", Category: "账号", Status: SupportTicketStatusOpen, Priority: SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(ctx, tk))

	_, err := svc.PatchAdmin(ctx, tk.ID, AdminTicketPatch{})
	require.ErrorIs(t, err, ErrSupportTicketNoFieldsToUpdate)
}

func TestSupportTicketService_PatchAdmin_UpdatesPriorityAndCategory(t *testing.T) {
	svc, repo, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()
	tk := &SupportTicket{UserID: 1, Title: "x", Content: "y", Category: "账号", Status: SupportTicketStatusOpen, Priority: SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(ctx, tk))

	high := SupportTicketPriorityHigh
	cat := "API"
	updated, err := svc.PatchAdmin(ctx, tk.ID, AdminTicketPatch{Priority: &high, Category: &cat})
	require.NoError(t, err)
	require.Equal(t, SupportTicketPriorityHigh, updated.Priority)
	require.Equal(t, "API", updated.Category)
}

func TestSupportTicketService_PatchAdmin_RejectsClosedReopen(t *testing.T) {
	svc, repo, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()
	tk := &SupportTicket{UserID: 1, Title: "x", Content: "y", Category: "账号", Status: SupportTicketStatusClosed, Priority: SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(ctx, tk))

	open := SupportTicketStatusOpen
	_, err := svc.PatchAdmin(ctx, tk.ID, AdminTicketPatch{Status: &open})
	require.ErrorIs(t, err, ErrSupportTicketInvalidStatusTransition)
	require.True(t, infraerrors.IsConflict(err))
}

func TestSupportTicketService_PatchAdmin_RejectsRepeatedClose(t *testing.T) {
	svc, repo, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()
	tk := &SupportTicket{UserID: 1, Title: "x", Content: "y", Category: "账号", Status: SupportTicketStatusClosed, Priority: SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(ctx, tk))

	closed := SupportTicketStatusClosed
	_, err := svc.PatchAdmin(ctx, tk.ID, AdminTicketPatch{Status: &closed})
	require.ErrorIs(t, err, ErrSupportTicketClosed)
}

func TestSupportTicketService_PatchAdmin_CloseSetsClosedAt(t *testing.T) {
	svc, repo, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()
	tk := &SupportTicket{UserID: 1, Title: "x", Content: "y", Category: "账号", Status: SupportTicketStatusInProgress, Priority: SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(ctx, tk))

	closed := SupportTicketStatusClosed
	updated, err := svc.PatchAdmin(ctx, tk.ID, AdminTicketPatch{Status: &closed})
	require.NoError(t, err)
	require.Equal(t, SupportTicketStatusClosed, updated.Status)
	require.NotNil(t, updated.ClosedAt)
	require.Equal(t, time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC), updated.ClosedAt.UTC())
}

func TestSupportTicketService_PatchAdmin_InvalidCategory(t *testing.T) {
	svc, repo, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()
	tk := &SupportTicket{UserID: 1, Title: "x", Content: "y", Category: "账号", Status: SupportTicketStatusOpen, Priority: SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(ctx, tk))

	cat := "未知分类"
	_, err := svc.PatchAdmin(ctx, tk.ID, AdminTicketPatch{Category: &cat})
	require.ErrorIs(t, err, ErrSupportTicketCategoryInvalid)
}

func TestSupportTicketService_PatchAdmin_InvalidPriority(t *testing.T) {
	svc, repo, _ := newSupportTicketServiceForTest(t, true)
	ctx := context.Background()
	tk := &SupportTicket{UserID: 1, Title: "x", Content: "y", Category: "账号", Status: SupportTicketStatusOpen, Priority: SupportTicketPriorityNormal}
	require.NoError(t, repo.Create(ctx, tk))

	bad := "urgent"
	_, err := svc.PatchAdmin(ctx, tk.ID, AdminTicketPatch{Priority: &bad})
	require.ErrorIs(t, err, ErrSupportTicketPriorityInvalid)
}

func TestSupportTicketService_PatchAdmin_NotFound(t *testing.T) {
	svc, _, _ := newSupportTicketServiceForTest(t, true)
	high := SupportTicketPriorityHigh
	_, err := svc.PatchAdmin(context.Background(), 999, AdminTicketPatch{Priority: &high})
	require.ErrorIs(t, err, ErrSupportTicketNotFound)
}

// ----------------------------------------------------------------------------
// ListCategories
// ----------------------------------------------------------------------------

func TestSupportTicketService_ListCategories_HappyPath(t *testing.T) {
	svc, _, _ := newSupportTicketServiceForTest(t, true)
	cats, def, err := svc.ListCategories(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"充值", "账号", "API", "Bug", "其他"}, cats)
	require.Equal(t, SupportTicketPriorityNormal, def)
}

func TestSupportTicketService_ListCategories_DisabledReturnsErr(t *testing.T) {
	svc, _, _ := newSupportTicketServiceForTest(t, false)
	_, _, err := svc.ListCategories(context.Background())
	require.ErrorIs(t, err, ErrSupportFeatureDisabled)
}

// ----------------------------------------------------------------------------
// 错误类型断言（确保 sentinels 仍然可用 errors.Is）
// ----------------------------------------------------------------------------

func TestSupportTicketErrors_AreSentinelsForErrorsIs(t *testing.T) {
	wrapped := errors.New("upstream")
	_ = wrapped // 占位

	// wrap-then-Is 测试：确保业务代码 wrapped fmt.Errorf("...: %w", err) 之后仍可
	// 用 errors.Is 精确匹配，不会被 %w 折叠成普通 error。
	wrappedErr := infraerrors.NotFound("X", "y")
	require.True(t, infraerrors.IsNotFound(wrappedErr))

	// 业务错误类型自检
	require.True(t, infraerrors.IsNotFound(ErrSupportFeatureDisabled))
	require.True(t, infraerrors.IsNotFound(ErrSupportTicketNotFound))
	require.True(t, infraerrors.IsConflict(ErrSupportTicketClosed))
	require.True(t, infraerrors.IsConflict(ErrSupportTicketInvalidStatusTransition))
	require.True(t, infraerrors.IsBadRequest(ErrSupportTicketTitleRequired))
	require.True(t, infraerrors.IsBadRequest(ErrSupportTicketCategoryInvalid))
	require.True(t, infraerrors.IsBadRequest(ErrSupportTicketChatContextTooLong))
}
