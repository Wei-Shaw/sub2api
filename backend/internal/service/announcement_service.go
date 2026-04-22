package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type AnnouncementService struct {
	announcementRepo AnnouncementRepository
	readRepo         AnnouncementReadRepository
	userRepo         UserRepository
	userSubRepo      UserSubscriptionRepository
}

func NewAnnouncementService(
	announcementRepo AnnouncementRepository,
	readRepo AnnouncementReadRepository,
	userRepo UserRepository,
	userSubRepo UserSubscriptionRepository,
) *AnnouncementService {
	return &AnnouncementService{
		announcementRepo: announcementRepo,
		readRepo:         readRepo,
		userRepo:         userRepo,
		userSubRepo:      userSubRepo,
	}
}

type CreateAnnouncementInput struct {
	Title      string
	Content    string
	Status     string
	NotifyMode string
	Targeting  AnnouncementTargeting
	StartsAt   *time.Time
	EndsAt     *time.Time
	ActorID    *int64 // 管理员用户ID
}

type UpdateAnnouncementInput struct {
	Title      *string
	Content    *string
	Status     *string
	NotifyMode *string
	Targeting  *AnnouncementTargeting
	StartsAt   **time.Time
	EndsAt     **time.Time
	ActorID    *int64 // 管理员用户ID
}

type UserAnnouncement struct {
	Announcement Announcement
	ReadAt       *time.Time
}

type AnnouncementUserReadStatus struct {
	UserID   int64      `json:"user_id"`
	Email    string     `json:"email"`
	Username string     `json:"username"`
	Balance  float64    `json:"balance"`
	Eligible bool       `json:"eligible"`
	ReadAt   *time.Time `json:"read_at,omitempty"`
}

func (s *AnnouncementService) Create(ctx context.Context, input *CreateAnnouncementInput) (*Announcement, error) {
	if input == nil {
		return nil, ErrAnnouncementNilInput
	}

	title := strings.TrimSpace(input.Title)
	content := strings.TrimSpace(input.Content)
	if title == "" || len(title) > 200 {
		return nil, ErrAnnouncementInvalidTitle
	}
	if content == "" {
		return nil, ErrAnnouncementContentRequired
	}

	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = AnnouncementStatusDraft
	}
	if !isValidAnnouncementStatus(status) {
		return nil, ErrAnnouncementInvalidStatus
	}

	targeting, err := domain.AnnouncementTargeting(input.Targeting).NormalizeAndValidate()
	if err != nil {
		return nil, err
	}

	notifyMode := strings.TrimSpace(input.NotifyMode)
	if notifyMode == "" {
		notifyMode = AnnouncementNotifyModeSilent
	}
	if !isValidAnnouncementNotifyMode(notifyMode) {
		return nil, ErrAnnouncementInvalidNotifyMode
	}

	if input.StartsAt != nil && input.EndsAt != nil {
		if !input.StartsAt.Before(*input.EndsAt) {
			return nil, ErrAnnouncementInvalidSchedule
		}
	}

	a := &Announcement{
		Title:      title,
		Content:    content,
		Status:     status,
		NotifyMode: notifyMode,
		Targeting:  targeting,
		StartsAt:   input.StartsAt,
		EndsAt:     input.EndsAt,
	}
	if input.ActorID != nil && *input.ActorID > 0 {
		a.CreatedBy = input.ActorID
		a.UpdatedBy = input.ActorID
	}

	if err := s.announcementRepo.Create(ctx, a); err != nil {
		return nil, fmt.Errorf("create announcement: %w", err)
	}
	return a, nil
}

func (s *AnnouncementService) Update(ctx context.Context, id int64, input *UpdateAnnouncementInput) (*Announcement, error) {
	if input == nil {
		return nil, ErrAnnouncementNilInput
	}

	a, err := s.announcementRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if title == "" || len(title) > 200 {
			return nil, ErrAnnouncementInvalidTitle
		}
		a.Title = title
	}
	if input.Content != nil {
		content := strings.TrimSpace(*input.Content)
		if content == "" {
			return nil, ErrAnnouncementContentRequired
		}
		a.Content = content
	}
	if input.Status != nil {
		status := strings.TrimSpace(*input.Status)
		if !isValidAnnouncementStatus(status) {
			return nil, ErrAnnouncementInvalidStatus
		}
		a.Status = status
	}

	if input.NotifyMode != nil {
		notifyMode := strings.TrimSpace(*input.NotifyMode)
		if !isValidAnnouncementNotifyMode(notifyMode) {
			return nil, ErrAnnouncementInvalidNotifyMode
		}
		a.NotifyMode = notifyMode
	}

	if input.Targeting != nil {
		targeting, err := domain.AnnouncementTargeting(*input.Targeting).NormalizeAndValidate()
		if err != nil {
			return nil, err
		}
		a.Targeting = targeting
	}

	if input.StartsAt != nil {
		a.StartsAt = *input.StartsAt
	}
	if input.EndsAt != nil {
		a.EndsAt = *input.EndsAt
	}

	if a.StartsAt != nil && a.EndsAt != nil {
		if !a.StartsAt.Before(*a.EndsAt) {
			return nil, ErrAnnouncementInvalidSchedule
		}
	}

	if input.ActorID != nil && *input.ActorID > 0 {
		a.UpdatedBy = input.ActorID
	}

	if err := s.announcementRepo.Update(ctx, a); err != nil {
		return nil, fmt.Errorf("update announcement: %w", err)
	}
	return a, nil
}

func (s *AnnouncementService) Delete(ctx context.Context, id int64) error {
	if err := s.announcementRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete announcement: %w", err)
	}
	return nil
}

func (s *AnnouncementService) GetByID(ctx context.Context, id int64) (*Announcement, error) {
	return s.announcementRepo.GetByID(ctx, id)
}

func (s *AnnouncementService) List(ctx context.Context, params pagination.PaginationParams, filters AnnouncementListFilters) ([]Announcement, *pagination.PaginationResult, error) {
	return s.announcementRepo.List(ctx, params, filters)
}

func (s *AnnouncementService) ListForUser(ctx context.Context, userID int64, unreadOnly bool) ([]UserAnnouncement, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	activeSubs, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list active subscriptions: %w", err)
	}
	activeGroupIDs := make(map[int64]struct{}, len(activeSubs))
	for i := range activeSubs {
		activeGroupIDs[activeSubs[i].GroupID] = struct{}{}
	}

	now := time.Now()
	anns, err := s.announcementRepo.ListActive(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("list active announcements: %w", err)
	}

	visible := make([]Announcement, 0, len(anns))
	ids := make([]int64, 0, len(anns))
	for i := range anns {
		a := anns[i]
		if !a.IsActiveAt(now) {
			continue
		}
		if !a.Targeting.Matches(user.Balance, activeGroupIDs) {
			continue
		}
		visible = append(visible, a)
		ids = append(ids, a.ID)
	}

	if len(visible) == 0 {
		return []UserAnnouncement{}, nil
	}

	readMap, err := s.readRepo.GetReadMapByUser(ctx, userID, ids)
	if err != nil {
		return nil, fmt.Errorf("get read map: %w", err)
	}

	out := make([]UserAnnouncement, 0, len(visible))
	for i := range visible {
		a := visible[i]
		readAt, ok := readMap[a.ID]
		if unreadOnly && ok {
			continue
		}
		var ptr *time.Time
		if ok {
			t := readAt
			ptr = &t
		}
		out = append(out, UserAnnouncement{
			Announcement: a,
			ReadAt:       ptr,
		})
	}

	// 未读优先、同状态按创建时间倒序
	sort.Slice(out, func(i, j int) bool {
		ai, aj := out[i], out[j]
		if (ai.ReadAt == nil) != (aj.ReadAt == nil) {
			return ai.ReadAt == nil
		}
		return ai.Announcement.ID > aj.Announcement.ID
	})

	return out, nil
}

func (s *AnnouncementService) MarkRead(ctx context.Context, userID, announcementID int64) error {
	// 安全：仅允许标记当前用户“可见”的公告
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	a, err := s.announcementRepo.GetByID(ctx, announcementID)
	if err != nil {
		return err
	}

	now := time.Now()
	if !a.IsActiveAt(now) {
		return ErrAnnouncementNotFound
	}

	activeSubs, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("list active subscriptions: %w", err)
	}
	activeGroupIDs := make(map[int64]struct{}, len(activeSubs))
	for i := range activeSubs {
		activeGroupIDs[activeSubs[i].GroupID] = struct{}{}
	}

	if !a.Targeting.Matches(user.Balance, activeGroupIDs) {
		return ErrAnnouncementNotFound
	}

	if err := s.readRepo.MarkRead(ctx, announcementID, userID, now); err != nil {
		return fmt.Errorf("mark read: %w", err)
	}
	return nil
}

func (s *AnnouncementService) ListUserReadStatus(
	ctx context.Context,
	announcementID int64,
	params pagination.PaginationParams,
	search string,
) ([]AnnouncementUserReadStatus, *pagination.PaginationResult, error) {
	ann, err := s.announcementRepo.GetByID(ctx, announcementID)
	if err != nil {
		return nil, nil, err
	}

	filters := UserListFilters{
		Search: strings.TrimSpace(search),
	}

	users, page, err := s.userRepo.ListWithFilters(ctx, params, filters)
	if err != nil {
		return nil, nil, fmt.Errorf("list users: %w", err)
	}

	userIDs := make([]int64, 0, len(users))
	for i := range users {
		userIDs = append(userIDs, users[i].ID)
	}

	readMap, err := s.readRepo.GetReadMapByUsers(ctx, announcementID, userIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("get read map: %w", err)
	}

	out := make([]AnnouncementUserReadStatus, 0, len(users))
	for i := range users {
		u := users[i]
		subs, err := s.userSubRepo.ListActiveByUserID(ctx, u.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("list active subscriptions: %w", err)
		}
		activeGroupIDs := make(map[int64]struct{}, len(subs))
		for j := range subs {
			activeGroupIDs[subs[j].GroupID] = struct{}{}
		}

		readAt, ok := readMap[u.ID]
		var ptr *time.Time
		if ok {
			t := readAt
			ptr = &t
		}

		out = append(out, AnnouncementUserReadStatus{
			UserID:   u.ID,
			Email:    u.Email,
			Username: u.Username,
			Balance:  u.Balance,
			Eligible: domain.AnnouncementTargeting(ann.Targeting).Matches(u.Balance, activeGroupIDs),
			ReadAt:   ptr,
		})
	}

	return out, page, nil
}

func isValidAnnouncementStatus(status string) bool {
	switch status {
	case AnnouncementStatusDraft, AnnouncementStatusActive, AnnouncementStatusArchived:
		return true
	default:
		return false
	}
}

func isValidAnnouncementNotifyMode(mode string) bool {
	switch mode {
	case AnnouncementNotifyModeSilent, AnnouncementNotifyModePopup:
		return true
	default:
		return false
	}
}
