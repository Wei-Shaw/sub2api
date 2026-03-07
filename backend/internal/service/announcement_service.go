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
REDACTED

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
REDACTED
REDACTED

type CreateAnnouncementInput struct {
	Title      string
	Content    string
	Status     string
	NotifyMode string
	Targeting  AnnouncementTargeting
	StartsAt   *time.Time
	EndsAt     *time.Time
	ActorID    *int64 // 管理员用户ID
REDACTED

type UpdateAnnouncementInput struct {
	Title      *string
	Content    *string
	Status     *string
	NotifyMode *string
	Targeting  *AnnouncementTargeting
	StartsAt   **time.Time
	EndsAt     **time.Time
	ActorID    *int64 // 管理员用户ID
REDACTED

type UserAnnouncement struct {
	Announcement Announcement
	ReadAt       *time.Time
REDACTED

type AnnouncementUserReadStatus struct {
	UserID   int64      `json:"user_id"`
	Email    string     `json:"email"`
	Username string     `json:"username"`
	Balance  float64    `json:"balance"`
	Eligible bool       `json:"eligible"`
	ReadAt   *time.Time `json:"read_at,omitempty"`
REDACTED

func (s *AnnouncementService) Create(ctx context.Context, input *CreateAnnouncementInput) (*Announcement, error) {
	if input == nil {
		return nil, fmt.Errorf("create announcement: nil input")
REDACTED

	title := strings.TrimSpace(input.Title)
	content := strings.TrimSpace(input.Content)
	if title == "" || len(title) > 200 {
		return nil, fmt.Errorf("create announcement: invalid title")
REDACTED
	if content == "" {
		return nil, fmt.Errorf("create announcement: content is required")
REDACTED

	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = AnnouncementStatusDraft
REDACTED
	if !isValidAnnouncementStatus(status) {
		return nil, fmt.Errorf("create announcement: invalid status")
REDACTED

	targeting, err := domain.AnnouncementTargeting(input.Targeting).NormalizeAndValidate()
	if err != nil {
		return nil, err
REDACTED

	notifyMode := strings.TrimSpace(input.NotifyMode)
	if notifyMode == "" {
		notifyMode = AnnouncementNotifyModeSilent
REDACTED
	if !isValidAnnouncementNotifyMode(notifyMode) {
		return nil, fmt.Errorf("create announcement: invalid notify_mode")
REDACTED

	if input.StartsAt != nil && input.EndsAt != nil {
		if !input.StartsAt.Before(*input.EndsAt) {
			return nil, fmt.Errorf("create announcement: starts_at must be before ends_at")
	REDACTED
REDACTED

	a := &Announcement{
		Title:      title,
		Content:    content,
		Status:     status,
		NotifyMode: notifyMode,
		Targeting:  targeting,
		StartsAt:   input.StartsAt,
		EndsAt:     input.EndsAt,
REDACTED
	if input.ActorID != nil && *input.ActorID > 0 {
		a.CreatedBy = input.ActorID
		a.UpdatedBy = input.ActorID
REDACTED

	if err := s.announcementRepo.Create(ctx, a); err != nil {
		return nil, fmt.Errorf("create announcement: %w", err)
REDACTED
	return a, nil
REDACTED

func (s *AnnouncementService) Update(ctx context.Context, id int64, input *UpdateAnnouncementInput) (*Announcement, error) {
	if input == nil {
		return nil, fmt.Errorf("update announcement: nil input")
REDACTED

	a, err := s.announcementRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
REDACTED

	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if title == "" || len(title) > 200 {
			return nil, fmt.Errorf("update announcement: invalid title")
	REDACTED
		a.Title = title
REDACTED
	if input.Content != nil {
		content := strings.TrimSpace(*input.Content)
		if content == "" {
			return nil, fmt.Errorf("update announcement: content is required")
	REDACTED
		a.Content = content
REDACTED
	if input.Status != nil {
		status := strings.TrimSpace(*input.Status)
		if !isValidAnnouncementStatus(status) {
			return nil, fmt.Errorf("update announcement: invalid status")
	REDACTED
		a.Status = status
REDACTED

	if input.NotifyMode != nil {
		notifyMode := strings.TrimSpace(*input.NotifyMode)
		if !isValidAnnouncementNotifyMode(notifyMode) {
			return nil, fmt.Errorf("update announcement: invalid notify_mode")
	REDACTED
		a.NotifyMode = notifyMode
REDACTED

	if input.Targeting != nil {
		targeting, err := domain.AnnouncementTargeting(*input.Targeting).NormalizeAndValidate()
		if err != nil {
			return nil, err
	REDACTED
		a.Targeting = targeting
REDACTED

	if input.StartsAt != nil {
		a.StartsAt = *input.StartsAt
REDACTED
	if input.EndsAt != nil {
		a.EndsAt = *input.EndsAt
REDACTED

	if a.StartsAt != nil && a.EndsAt != nil {
		if !a.StartsAt.Before(*a.EndsAt) {
			return nil, fmt.Errorf("update announcement: starts_at must be before ends_at")
	REDACTED
REDACTED

	if input.ActorID != nil && *input.ActorID > 0 {
		a.UpdatedBy = input.ActorID
REDACTED

	if err := s.announcementRepo.Update(ctx, a); err != nil {
		return nil, fmt.Errorf("update announcement: %w", err)
REDACTED
	return a, nil
REDACTED

func (s *AnnouncementService) Delete(ctx context.Context, id int64) error {
	if err := s.announcementRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete announcement: %w", err)
REDACTED
	return nil
REDACTED

func (s *AnnouncementService) GetByID(ctx context.Context, id int64) (*Announcement, error) {
	return s.announcementRepo.GetByID(ctx, id)
REDACTED

func (s *AnnouncementService) List(ctx context.Context, params pagination.PaginationParams, filters AnnouncementListFilters) ([]Announcement, *pagination.PaginationResult, error) {
	return s.announcementRepo.List(ctx, params, filters)
REDACTED

func (s *AnnouncementService) ListForUser(ctx context.Context, userID int64, unreadOnly bool) ([]UserAnnouncement, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
REDACTED

	activeSubs, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list active subscriptions: %w", err)
REDACTED
	activeGroupIDs := make(map[int64]struct{REDACTED, len(activeSubs))
	for i := range activeSubs {
		activeGroupIDs[activeSubs[i].GroupID] = struct{REDACTED{REDACTED
REDACTED

	now := time.Now()
	anns, err := s.announcementRepo.ListActive(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("list active announcements: %w", err)
REDACTED

	visible := make([]Announcement, 0, len(anns))
	ids := make([]int64, 0, len(anns))
	for i := range anns {
		a := anns[i]
		if !a.IsActiveAt(now) {
			continue
	REDACTED
		if !a.Targeting.Matches(user.Balance, activeGroupIDs) {
			continue
	REDACTED
		visible = append(visible, a)
		ids = append(ids, a.ID)
REDACTED

	if len(visible) == 0 {
		return []UserAnnouncement{REDACTED, nil
REDACTED

	readMap, err := s.readRepo.GetReadMapByUser(ctx, userID, ids)
	if err != nil {
		return nil, fmt.Errorf("get read map: %w", err)
REDACTED

	out := make([]UserAnnouncement, 0, len(visible))
	for i := range visible {
		a := visible[i]
		readAt, ok := readMap[a.ID]
		if unreadOnly && ok {
			continue
	REDACTED
		var ptr *time.Time
		if ok {
			t := readAt
			ptr = &t
	REDACTED
		out = append(out, UserAnnouncement{
			Announcement: a,
			ReadAt:       ptr,
	REDACTED)
REDACTED

	// 未读优先、同状态按创建时间倒序
	sort.Slice(out, func(i, j int) bool {
		ai, aj := out[i], out[j]
		if (ai.ReadAt == nil) != (aj.ReadAt == nil) {
			return ai.ReadAt == nil
	REDACTED
		return ai.Announcement.ID > aj.Announcement.ID
REDACTED)

	return out, nil
REDACTED

func (s *AnnouncementService) MarkRead(ctx context.Context, userID, announcementID int64) error {
	// 安全：仅允许标记当前用户“可见”的公告
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
REDACTED

	a, err := s.announcementRepo.GetByID(ctx, announcementID)
	if err != nil {
		return err
REDACTED

	now := time.Now()
	if !a.IsActiveAt(now) {
		return ErrAnnouncementNotFound
REDACTED

	activeSubs, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("list active subscriptions: %w", err)
REDACTED
	activeGroupIDs := make(map[int64]struct{REDACTED, len(activeSubs))
	for i := range activeSubs {
		activeGroupIDs[activeSubs[i].GroupID] = struct{REDACTED{REDACTED
REDACTED

	if !a.Targeting.Matches(user.Balance, activeGroupIDs) {
		return ErrAnnouncementNotFound
REDACTED

	if err := s.readRepo.MarkRead(ctx, announcementID, userID, now); err != nil {
		return fmt.Errorf("mark read: %w", err)
REDACTED
	return nil
REDACTED

func (s *AnnouncementService) ListUserReadStatus(
	ctx context.Context,
	announcementID int64,
	params pagination.PaginationParams,
	search string,
) ([]AnnouncementUserReadStatus, *pagination.PaginationResult, error) {
	ann, err := s.announcementRepo.GetByID(ctx, announcementID)
	if err != nil {
		return nil, nil, err
REDACTED

	filters := UserListFilters{
		Search: strings.TrimSpace(search),
REDACTED

	users, page, err := s.userRepo.ListWithFilters(ctx, params, filters)
	if err != nil {
		return nil, nil, fmt.Errorf("list users: %w", err)
REDACTED

	userIDs := make([]int64, 0, len(users))
	for i := range users {
		userIDs = append(userIDs, users[i].ID)
REDACTED

	readMap, err := s.readRepo.GetReadMapByUsers(ctx, announcementID, userIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("get read map: %w", err)
REDACTED

	out := make([]AnnouncementUserReadStatus, 0, len(users))
	for i := range users {
		u := users[i]
		subs, err := s.userSubRepo.ListActiveByUserID(ctx, u.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("list active subscriptions: %w", err)
	REDACTED
		activeGroupIDs := make(map[int64]struct{REDACTED, len(subs))
		for j := range subs {
			activeGroupIDs[subs[j].GroupID] = struct{REDACTED{REDACTED
	REDACTED

		readAt, ok := readMap[u.ID]
		var ptr *time.Time
		if ok {
			t := readAt
			ptr = &t
	REDACTED

		out = append(out, AnnouncementUserReadStatus{
			UserID:   u.ID,
			Email:    u.Email,
			Username: u.Username,
			Balance:  u.Balance,
			Eligible: domain.AnnouncementTargeting(ann.Targeting).Matches(u.Balance, activeGroupIDs),
			ReadAt:   ptr,
	REDACTED)
REDACTED

	return out, page, nil
REDACTED

func isValidAnnouncementStatus(status string) bool {
	switch status {
	case AnnouncementStatusDraft, AnnouncementStatusActive, AnnouncementStatusArchived:
		return true
	default:
		return false
REDACTED
REDACTED

func isValidAnnouncementNotifyMode(mode string) bool {
	switch mode {
	case AnnouncementNotifyModeSilent, AnnouncementNotifyModePopup:
		return true
	default:
		return false
REDACTED
REDACTED
