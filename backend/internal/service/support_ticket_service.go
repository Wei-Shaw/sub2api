// Package service — support_ticket_service.go
//
// SupportTicketService 是工单系统的业务规则与状态机协调层。它在 Repository（纯
// CRUD）之上叠加：
//
//   - 功能开关：用户路径任何方法在 settings.support_ticket_enabled = false 时返回
//     ErrSupportFeatureDisabled（handler 翻成 404，避免泄露功能存在）。admin 路径
//     不卡 enabled，便于运营人员预先编辑配置或处理已有工单（spec 7.2）。
//   - 校验：title / content / chat_context 长度，category 在当前 settings 配置内，
//     priority ∈ {low,normal,high}（缺省走 settings 默认值）。
//   - 状态机：
//   - open / in_progress 可任意写入回复；任何路径触达 closed 工单返回 409。
//   - AppendAdminReply 在事务里追加 reply 后，若 status=open 自动跃迁为
//     in_progress（保持 admin 介入语义）。
//   - CloseUserTicket / Admin patch closed：set status=closed + closed_at=now()，
//     重复 close 返回 409（幂等收敛由 handler 决定如何展示，但 service 层显式
//     拒绝以防静默掩盖客户端 bug）。
//   - PatchAdmin 拒绝 closed → 非 closed 转移。
//   - 安全边界：用户路径方法强制 ticket.user_id == 调用者 userID，否则统一返回
//     ErrSupportTicketNotFound（与 ticket 不存在不区分），防止探测他人工单。
package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// SupportTicketSettingsReader 是 SupportTicketService 依赖的只读 settings 契约。
// *SettingService 隐式实现该接口；单测可注入轻量桩。
type SupportTicketSettingsReader interface {
	GetSupportTicketRuntime(ctx context.Context) SupportTicketRuntime
}

// SupportTicketService 编排工单系统的业务规则与状态机。
type SupportTicketService struct {
	repo      SupportTicketRepository
	settings  SupportTicketSettingsReader
	entClient *dbent.Client
	cos       *COSImageTransferService
	now       func() time.Time
}

// NewSupportTicketService 构造工单服务实例。
//
// entClient 用于在 AppendAdminReply 时开启外层事务，让"追加 reply + 状态跃迁"在
// 同一事务里原子提交。entClient 允许为 nil（fallback：两步非事务执行，可接受
// 极少数边界场景下的不一致——例如 AppendReply 成功后 UpdateFields 失败，下次
// admin 操作仍可恢复语义）。生产路由应始终注入非 nil。
//
// cos 用于承担工单附件上传。允许为 nil（测试桩），此时 UploadAttachment 直接返回
// ErrSupportTicketAttachmentsUnavailable；提交阶段的 URL 白名单校验也会退化为
// 仅按格式校验（不做前缀比对），避免测试环境构造 URL 过于繁琐。
func NewSupportTicketService(
	repo SupportTicketRepository,
	settings SupportTicketSettingsReader,
	entClient *dbent.Client,
	cos *COSImageTransferService,
) *SupportTicketService {
	return &SupportTicketService{
		repo:      repo,
		settings:  settings,
		entClient: entClient,
		cos:       cos,
		now:       time.Now,
	}
}

// CreateTicketInput 是用户提交新工单的参数。
type CreateTicketInput struct {
	UserID      int64
	Title       string
	Content     string
	Category    string
	ChatContext *string
	// Priority 可空：空值表示采用 settings.default_priority。非空时必须 ∈ {low,normal,high}。
	Priority string
	// Images 可空：主帖附带的图片列表；每个元素必须由本 service 的 UploadAttachment 生成
	// 并落到已配置的 COS bucket 上，Repository 层只做透明持久化。
	Images []domain.SupportTicketImage
}

// AppendReplyInput 是追加回复（用户/admin 共用）的参数。
//
// Content 走 validateReplyContent 强校验；Images 与主帖同规则（<=5 张、URL 前缀白名单）。
type AppendReplyInput struct {
	Content string
	Images  []domain.SupportTicketImage
}

// CreateTicket：用户新建工单。
//
// 校验顺序刻意从"开关"→"必填字段"→"长度"→"枚举"，与前端表单提示就近匹配，
// 让用户先看到的错误是"内容必填"而非"分类无效"。
func (s *SupportTicketService) CreateTicket(ctx context.Context, in CreateTicketInput) (*SupportTicket, error) {
	rt := s.settings.GetSupportTicketRuntime(ctx)
	if !rt.Enabled {
		return nil, ErrSupportFeatureDisabled
	}

	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, ErrSupportTicketTitleRequired
	}
	if utf8.RuneCountInString(title) > SupportTicketTitleMaxLen {
		return nil, ErrSupportTicketTitleTooLong
	}

	content := strings.TrimSpace(in.Content)
	if content == "" {
		return nil, ErrSupportTicketContentRequired
	}
	if utf8.RuneCountInString(content) > SupportTicketContentMaxLen {
		return nil, ErrSupportTicketContentTooLong
	}

	category := strings.TrimSpace(in.Category)
	if !categoryAllowed(rt.Categories, category) {
		return nil, ErrSupportTicketCategoryInvalid
	}

	priority := strings.TrimSpace(in.Priority)
	if priority == "" {
		priority = rt.DefaultPriority
	}
	if !IsValidSupportTicketPriority(priority) {
		return nil, ErrSupportTicketPriorityInvalid
	}

	chatCtx, err := normalizeChatContext(in.ChatContext)
	if err != nil {
		return nil, err
	}

	images, err := s.validateImages(ctx, in.Images)
	if err != nil {
		return nil, err
	}

	t := &SupportTicket{
		UserID:      in.UserID,
		Title:       title,
		Content:     content,
		Category:    category,
		Status:      SupportTicketStatusOpen,
		Priority:    priority,
		ChatContext: chatCtx,
		Images:      images,
	}
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, fmt.Errorf("create support ticket: %w", err)
	}
	return t, nil
}

// GetUserTicket：返回某工单详情（含完整 chat_context 与回复时间线）。
// 强制 owner 校验：非 owner / 不存在均返回 ErrSupportTicketNotFound。
func (s *SupportTicketService) GetUserTicket(ctx context.Context, userID, ticketID int64) (*SupportTicketWithReplies, error) {
	if !s.settings.GetSupportTicketRuntime(ctx).Enabled {
		return nil, ErrSupportFeatureDisabled
	}
	t, err := s.repo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if t.UserID != userID {
		return nil, ErrSupportTicketNotFound
	}
	replies, err := s.repo.ListReplies(ctx, ticketID)
	if err != nil {
		return nil, fmt.Errorf("list replies: %w", err)
	}
	return &SupportTicketWithReplies{Ticket: *t, Replies: replies}, nil
}

// ListUserTickets：用户视角分页列表（不含 chat_context）。
func (s *SupportTicketService) ListUserTickets(
	ctx context.Context,
	userID int64,
	params pagination.PaginationParams,
) ([]SupportTicket, *pagination.PaginationResult, error) {
	if !s.settings.GetSupportTicketRuntime(ctx).Enabled {
		return nil, nil, ErrSupportFeatureDisabled
	}
	return s.repo.ListByUser(ctx, userID, params)
}

// AppendUserReply：用户追加回复。
//
// 不做 status 跃迁：状态机里 in_progress → open 不存在意义；spec D2 也只规定 admin
// 回复触发 open → in_progress。
func (s *SupportTicketService) AppendUserReply(ctx context.Context, userID, ticketID int64, in AppendReplyInput) (*SupportTicketReply, error) {
	if !s.settings.GetSupportTicketRuntime(ctx).Enabled {
		return nil, ErrSupportFeatureDisabled
	}
	body, err := validateReplyContent(in.Content)
	if err != nil {
		return nil, err
	}
	images, err := s.validateImages(ctx, in.Images)
	if err != nil {
		return nil, err
	}

	t, err := s.repo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if t.UserID != userID {
		return nil, ErrSupportTicketNotFound
	}
	if t.Status == SupportTicketStatusClosed {
		return nil, ErrSupportTicketClosed
	}

	uid := userID
	reply := &SupportTicketReply{
		TicketID: ticketID,
		AuthorID: &uid,
		IsAdmin:  false,
		Content:  body,
		Images:   images,
	}
	if err := s.repo.AppendReply(ctx, reply); err != nil {
		return nil, fmt.Errorf("append user reply: %w", err)
	}
	return reply, nil
}

// CloseUserTicket：用户主动关闭工单。
// 已 closed 状态再次关闭返回 409（避免静默掩盖客户端 bug）。
func (s *SupportTicketService) CloseUserTicket(ctx context.Context, userID, ticketID int64) error {
	if !s.settings.GetSupportTicketRuntime(ctx).Enabled {
		return ErrSupportFeatureDisabled
	}
	t, err := s.repo.GetByID(ctx, ticketID)
	if err != nil {
		return err
	}
	if t.UserID != userID {
		return ErrSupportTicketNotFound
	}
	if t.Status == SupportTicketStatusClosed {
		return ErrSupportTicketClosed
	}

	closedAt := s.now().UTC()
	closed := SupportTicketStatusClosed
	if err := s.repo.UpdateFields(ctx, ticketID, SupportTicketPatch{
		Status:   &closed,
		ClosedAt: &closedAt,
	}); err != nil {
		return fmt.Errorf("close support ticket: %w", err)
	}
	return nil
}

// ----------------------------------------------------------------------------
// admin 路径
// ----------------------------------------------------------------------------

// ListAdminTickets：admin 视角分页列表。不卡 feature_enabled。
func (s *SupportTicketService) ListAdminTickets(
	ctx context.Context,
	filters SupportTicketListFilters,
	params pagination.PaginationParams,
) ([]SupportTicket, *pagination.PaginationResult, error) {
	return s.repo.ListAdmin(ctx, filters, params)
}

// GetAdminTicket：admin 拿任意工单详情。
func (s *SupportTicketService) GetAdminTicket(ctx context.Context, ticketID int64) (*SupportTicketWithReplies, error) {
	t, err := s.repo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	replies, err := s.repo.ListReplies(ctx, ticketID)
	if err != nil {
		return nil, fmt.Errorf("list replies: %w", err)
	}
	return &SupportTicketWithReplies{Ticket: *t, Replies: replies}, nil
}

// AppendAdminReply：admin 追加回复。
//
// 事务边界：
//   - 在外层事务中 (1) INSERT reply (2) 若当前 status = open，UPDATE 为 in_progress。
//   - tx 失败任意一步整体回滚，避免出现"reply 写了但 status 没动"的中间态。
//   - 若 entClient 为 nil（默认不会出现，仅测试场景），退化为非事务两步执行。
func (s *SupportTicketService) AppendAdminReply(ctx context.Context, adminID, ticketID int64, in AppendReplyInput) (*SupportTicketReply, error) {
	body, err := validateReplyContent(in.Content)
	if err != nil {
		return nil, err
	}
	images, err := s.validateImages(ctx, in.Images)
	if err != nil {
		return nil, err
	}

	t, err := s.repo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if t.Status == SupportTicketStatusClosed {
		return nil, ErrSupportTicketClosed
	}

	aid := adminID
	reply := &SupportTicketReply{
		TicketID: ticketID,
		AuthorID: &aid,
		IsAdmin:  true,
		Content:  body,
		Images:   images,
	}

	exec := func(execCtx context.Context) error {
		if err := s.repo.AppendReply(execCtx, reply); err != nil {
			return fmt.Errorf("append admin reply: %w", err)
		}
		// open → in_progress 跃迁；其他状态（in_progress）不改。
		if t.Status == SupportTicketStatusOpen {
			next := SupportTicketStatusInProgress
			if err := s.repo.UpdateFields(execCtx, ticketID, SupportTicketPatch{Status: &next}); err != nil {
				return fmt.Errorf("transition status open->in_progress: %w", err)
			}
		}
		return nil
	}

	if s.entClient == nil {
		// 非事务路径（测试桩 / 未注入 entClient 的边界场景）。
		if err := exec(ctx); err != nil {
			return nil, err
		}
		return reply, nil
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin support ticket tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := exec(txCtx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit support ticket tx: %w", err)
	}
	committed = true
	return reply, nil
}

// AdminTicketPatch 是 admin 编辑工单可选字段集合。
//
// 与 SupportTicketPatch 不同的是：admin 视角不允许直接写 ClosedAt，由 service 层
// 在 status 跃迁时同步维护；status 也只允许 in_progress / closed（open → in_progress
// 由系统自动触发，不通过该 endpoint 显式设置）。
type AdminTicketPatch struct {
	Status   *string
	Priority *string
	Category *string
}

// PatchAdmin：admin 编辑工单状态 / 优先级 / 分类。
//
// 业务规则：
//   - 至少一个字段非 nil，否则 400 ErrSupportTicketNoFieldsToUpdate。
//   - status 必须 ∈ {open, in_progress, closed}；
//     当前 status = closed 且 patch.Status != closed → 409 ErrSupportTicketInvalidStatusTransition
//     （不允许重新打开已关闭工单）。
//   - 当 patch.Status = closed 时同步设置 ClosedAt = now()；当 status 从 closed 转回时
//     不存在该路径（已被前面的 transition 拒绝）。
//   - priority 必须 ∈ {low, normal, high}。
//   - category 必须在当前 settings.support_ticket_categories 内。
//
// 返回更新后的完整 ticket（重新 GetByID，避免内存对象与 DB 漂移）。
func (s *SupportTicketService) PatchAdmin(ctx context.Context, ticketID int64, patch AdminTicketPatch) (*SupportTicket, error) {
	if patch.Status == nil && patch.Priority == nil && patch.Category == nil {
		return nil, ErrSupportTicketNoFieldsToUpdate
	}

	current, err := s.repo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}

	repoPatch := SupportTicketPatch{}

	if patch.Status != nil {
		next := strings.TrimSpace(*patch.Status)
		if !IsValidSupportTicketStatus(next) {
			return nil, ErrSupportTicketStatusInvalid
		}
		// 状态机校验：closed 是终态，不允许回到 open / in_progress。
		if current.Status == SupportTicketStatusClosed && next != SupportTicketStatusClosed {
			return nil, ErrSupportTicketInvalidStatusTransition
		}
		// 重复 close：与 CloseUserTicket 行为一致返回 409。
		if current.Status == SupportTicketStatusClosed && next == SupportTicketStatusClosed {
			return nil, ErrSupportTicketClosed
		}

		repoPatch.Status = &next
		if next == SupportTicketStatusClosed {
			closedAt := s.now().UTC()
			repoPatch.ClosedAt = &closedAt
		}
	}

	if patch.Priority != nil {
		nextP := strings.TrimSpace(*patch.Priority)
		if !IsValidSupportTicketPriority(nextP) {
			return nil, ErrSupportTicketPriorityInvalid
		}
		repoPatch.Priority = &nextP
	}

	if patch.Category != nil {
		nextC := strings.TrimSpace(*patch.Category)
		// admin 改分类同样必须在当前 settings 配置内（spec 4.6）。
		rt := s.settings.GetSupportTicketRuntime(ctx)
		if !categoryAllowed(rt.Categories, nextC) {
			return nil, ErrSupportTicketCategoryInvalid
		}
		repoPatch.Category = &nextC
	}

	if err := s.repo.UpdateFields(ctx, ticketID, repoPatch); err != nil {
		return nil, fmt.Errorf("patch support ticket: %w", err)
	}

	updated, err := s.repo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// ListCategories：用户新建工单页拉取分类下拉与默认优先级。
//
// 当 feature_enabled = false 时返回 ErrSupportFeatureDisabled——前端守卫已经把
// 入口隐藏了，但仍要在 service 层加 hard guard 防止接口被直接调用。
func (s *SupportTicketService) ListCategories(ctx context.Context) (categories []string, defaultPriority string, err error) {
	rt := s.settings.GetSupportTicketRuntime(ctx)
	if !rt.Enabled {
		return nil, "", ErrSupportFeatureDisabled
	}
	out := make([]string, len(rt.Categories))
	copy(out, rt.Categories)
	return out, rt.DefaultPriority, nil
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

// categoryAllowed 判断 cat 是否在当前 categories 列表内。空 cat 直接判否。
func categoryAllowed(categories []string, cat string) bool {
	if cat == "" {
		return false
	}
	for _, c := range categories {
		if c == cat {
			return true
		}
	}
	return false
}

// validateReplyContent 对回复正文做 trim + 必填 + 长度上限校验。
func validateReplyContent(raw string) (string, error) {
	body := strings.TrimSpace(raw)
	if body == "" {
		return "", ErrSupportTicketReplyContentRequired
	}
	if utf8.RuneCountInString(body) > SupportTicketReplyContentMaxLen {
		return "", ErrSupportTicketReplyContentTooLong
	}
	return body, nil
}

// normalizeChatContext 处理 chat_context 入参：
//   - nil / 空白 / "" 一律视为未提供，返回 nil（不写入 DB）。
//   - 非空且超出 SupportTicketChatContextMaxLen 返回 400。
//   - 否则返回去 trim 后的指针。
func normalizeChatContext(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	cc := strings.TrimSpace(*raw)
	if cc == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(cc) > SupportTicketChatContextMaxLen {
		return nil, ErrSupportTicketChatContextTooLong
	}
	return &cc, nil
}

// ----------------------------------------------------------------------------
// 附件上传 & 图片校验
// ----------------------------------------------------------------------------

// SupportTicketAttachmentUpload 是 UploadAttachment 的入参：由 handler 从 multipart 中读到内存后传入。
//
// FileName 仅用于日志与扩展名辅助推断（COS key 会重命名，不落原文件名到对象存储路径）。
type SupportTicketAttachmentUpload struct {
	FileName        string
	ContentTypeHint string
	Data            []byte
}

// UploadAttachment 完成"校验 + 写 COS + 返回结构化附件记录"的全部工作。
//
// 校验顺序：
//  1. 功能开关（feature_enabled=false → 404）。
//  2. COS 已启用且配置齐全（否则 400 ErrSupportTicketAttachmentsUnavailable，避免上传后无法访问）。
//  3. 单张体积 ≤ 5 MB（尽早失败，防止大文件上传占用带宽与 COS 配额）。
//  4. magic-bytes 嗅探必须是 image/png 或 image/jpeg（不信任 Content-Type，防止伪装扩展名绕过）。
//  5. 通过 COSImageTransferService.UploadImageBytes 上传，返回对外 URL。
//
// 返回的 SupportTicketImage.Key 是 COS 对象 key（含前缀）、URL 是可展示地址。
// 上传成功但用户没有把 URL 塞进 create/append —— 会产生"孤儿对象"，运维可通过
// COS bucket 生命周期规则清理（例如 24h 未引用），本 service 层不做主动清理，
// 避免与用户"上传后先编辑再发送"的操作节奏冲突。
func (s *SupportTicketService) UploadAttachment(ctx context.Context, in SupportTicketAttachmentUpload) (*domain.SupportTicketImage, error) {
	if !s.settings.GetSupportTicketRuntime(ctx).Enabled {
		return nil, ErrSupportFeatureDisabled
	}
	if s.cos == nil || !s.cos.IsEnabled(ctx) {
		return nil, ErrSupportTicketAttachmentsUnavailable
	}
	if int64(len(in.Data)) == 0 || int64(len(in.Data)) > SupportTicketImageMaxBytes {
		return nil, ErrSupportTicketImageTooLarge
	}
	// magic bytes 嗅探：http.DetectContentType 只读前 512 字节，能可靠识别 PNG/JPEG。
	sniff := normalizeImageContentType(http.DetectContentType(in.Data[:min(len(in.Data), 512)]))
	if !isAllowedSupportTicketImageMIME(sniff) {
		return nil, ErrSupportTicketImageUnsupportedType
	}
	// 显式提示 srcHint 走原文件名，便于 COS key 保留正确扩展名。
	url, err := s.cos.UploadImageBytes(ctx, in.Data, sniff, in.FileName)
	if err != nil {
		return nil, fmt.Errorf("upload support ticket attachment: %w", err)
	}
	if strings.TrimSpace(url) == "" {
		// UploadImageBytes 未启用时返回 ("", nil)，但我们前面已检查 IsEnabled，
		// 若仍走到这里说明 config 在中途被禁用，按 "unavailable" 返回。
		return nil, ErrSupportTicketAttachmentsUnavailable
	}
	key := extractCOSObjectKey(url)
	return &domain.SupportTicketImage{
		Key:  key,
		URL:  url,
		Size: int64(len(in.Data)),
		MIME: sniff,
	}, nil
}

// validateImages 对 CreateTicket / AppendReply 传入的 images 做完整性 + 白名单校验。
//
// 规则：
//   - nil / 长度 0 → 返回 nil（不写入任何附件，向 DB 传 nil 让 ent 走默认 '[]'）。
//   - 长度 > SupportTicketImagesMaxCount → ErrSupportTicketImagesTooMany。
//   - 每条元素：url/key/mime 非空，mime 必须在白名单，url 必须指向已配置的 COS bucket
//     的公开域名（防伪造外链 / SSRF）。size 允许为 0（老客户端），但若 > 上限直接拒绝。
//   - 输出 slice 是一份新分配的副本，与入参解耦。
func (s *SupportTicketService) validateImages(ctx context.Context, in []domain.SupportTicketImage) ([]domain.SupportTicketImage, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if len(in) > SupportTicketImagesMaxCount {
		return nil, ErrSupportTicketImagesTooMany
	}
	allowedPrefixes := s.allowedImageURLPrefixes(ctx)

	out := make([]domain.SupportTicketImage, 0, len(in))
	for _, img := range in {
		url := strings.TrimSpace(img.URL)
		key := strings.TrimSpace(img.Key)
		mime := normalizeImageContentType(strings.TrimSpace(img.MIME))
		if url == "" || key == "" || mime == "" {
			return nil, ErrSupportTicketImageInvalid
		}
		if !isAllowedSupportTicketImageMIME(mime) {
			return nil, ErrSupportTicketImageUnsupportedType
		}
		if img.Size < 0 || img.Size > SupportTicketImageMaxBytes {
			return nil, ErrSupportTicketImageTooLarge
		}
		// URL 前缀白名单：只允许上传接口返回过的 base URL。测试环境 cos 未挂载时跳过。
		if len(allowedPrefixes) > 0 && !hasAnyPrefix(url, allowedPrefixes) {
			return nil, ErrSupportTicketImageInvalid
		}
		out = append(out, domain.SupportTicketImage{
			Key:  key,
			URL:  url,
			Size: img.Size,
			MIME: mime,
		})
	}
	return out, nil
}

// allowedImageURLPrefixes 返回当前 COS 配置下可能出现的 URL 前缀集合。
// cos 未挂载或未启用 → 返回空 slice，validateImages 将放行任意 URL（仅测试路径会用到）。
func (s *SupportTicketService) allowedImageURLPrefixes(ctx context.Context) []string {
	if s.cos == nil {
		return nil
	}
	cfg, err := s.cos.GetConfig(ctx)
	if err != nil || cfg == nil || !cfg.IsConfigured() {
		return nil
	}
	prefixes := make([]string, 0, 4)
	if base := strings.TrimRight(strings.TrimSpace(cfg.PublicBaseURL), "/"); base != "" {
		prefixes = append(prefixes, base+"/")
	}
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if endpoint != "" {
		// path-style（force_path_style=true 或客户端始终用 https://host/bucket/key）
		prefixes = append(prefixes, endpoint+"/"+cfg.Bucket+"/")
		// virtual-hosted style：https://{bucket}.{host}/{key}
		if scheme, host, ok := splitScheme(endpoint); ok {
			prefixes = append(prefixes, scheme+"://"+cfg.Bucket+"."+host+"/")
		}
	}
	return prefixes
}

// isAllowedSupportTicketImageMIME 判断规范化后的 mime 是否在允许列表。
func isAllowedSupportTicketImageMIME(mime string) bool {
	for _, m := range SupportTicketAllowedImageMIMEs {
		if m == mime {
			return true
		}
	}
	return false
}

// hasAnyPrefix 判断 s 是否以任一 prefix 起始（区分大小写；URL 主机/路径都按大小写敏感处理）。
func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if p != "" && strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// extractCOSObjectKey 从 URL 尾部推断 object key（用于回显到前端 & 便于后续审计）。
//
// 由于 COS 的 URL 形式有 path-style（endpoint/bucket/key）和 virtual-hosted-style
// （bucket.host/key）两种，且都在同一次配置下二选一，这里采取一个鲁棒但**近似**的做法：
// 找 "://" 之后的第一个 "/"，其后剩余部分即视为 key。
// 若 URL 前缀是 cfg.PublicBaseURL，会保留全部相对路径作为 key。
// 无法解析时回退返回整段 URL —— 便于运维人肉查询，不影响业务读写。
func extractCOSObjectKey(url string) string {
	if idx := strings.Index(url, "://"); idx >= 0 {
		rest := url[idx+3:]
		if slash := strings.IndexByte(rest, '/'); slash >= 0 {
			return rest[slash+1:]
		}
	}
	return url
}

// consumeAttachmentBody 是 handler 侧读取 multipart body 时用的助手：
// 用 io.ReadAll(io.LimitReader(...)) 一次性读入内存，禁止流式绕过大小限制。
// 放在 service 包里让 handler 无需自行处理 io 细节，同时保持 handler-no-repository 约束。
func consumeAttachmentBody(r io.Reader, limit int64) ([]byte, error) {
	if r == nil {
		return nil, nil
	}
	// +1 是为了检测"刚好超过 limit"的情况：ReadAll 若读到 limit+1 字节就说明超限。
	buf := &bytes.Buffer{}
	if _, err := io.CopyN(buf, r, limit+1); err != nil && err != io.EOF {
		return nil, err
	}
	if int64(buf.Len()) > limit {
		return nil, ErrSupportTicketImageTooLarge
	}
	return buf.Bytes(), nil
}

// ReadAttachmentBody 是 consumeAttachmentBody 的对外 wrapper（package 内部工具变外部可见），
// 供 handler 层复用体积上限逻辑。
func ReadAttachmentBody(r io.Reader) ([]byte, error) {
	return consumeAttachmentBody(r, SupportTicketImageMaxBytes)
}
