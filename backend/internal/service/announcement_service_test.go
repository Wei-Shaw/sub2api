package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type announcementRepoStub struct {
	item *Announcement
REDACTED

func (s *announcementRepoStub) Create(_ context.Context, a *Announcement) error {
	s.item = a
	return nil
REDACTED

func (s *announcementRepoStub) GetByID(_ context.Context, _ int64) (*Announcement, error) {
	if s.item == nil {
		return nil, ErrAnnouncementNotFound
REDACTED
	return s.item, nil
REDACTED

func (s *announcementRepoStub) Update(_ context.Context, a *Announcement) error {
	s.item = a
	return nil
REDACTED

func (*announcementRepoStub) Delete(context.Context, int64) error {
	return nil
REDACTED

func (*announcementRepoStub) List(context.Context, pagination.PaginationParams, AnnouncementListFilters) ([]Announcement, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED

func (*announcementRepoStub) ListActive(context.Context, time.Time) ([]Announcement, error) {
	return nil, nil
REDACTED

func TestAnnouncementServiceCreateRejectsEqualStartEndTimes(t *testing.T) {
	repo := &announcementRepoStub{REDACTED
	svc := NewAnnouncementService(repo, nil, nil, nil)
	now := time.Unix(1776790020, 0)

	_, err := svc.Create(context.Background(), &CreateAnnouncementInput{
		Title:      "公告",
		Content:    "内容",
		Status:     AnnouncementStatusActive,
		NotifyMode: AnnouncementNotifyModePopup,
		StartsAt:   &now,
		EndsAt:     &now,
REDACTED)
	require.ErrorIs(t, err, ErrAnnouncementInvalidSchedule)
REDACTED

func TestAnnouncementServiceUpdateRejectsEqualStartEndTimes(t *testing.T) {
	repo := &announcementRepoStub{
		item: &Announcement{
			ID:         1,
			Title:      "公告",
			Content:    "内容",
			Status:     AnnouncementStatusActive,
			NotifyMode: AnnouncementNotifyModePopup,
	REDACTED,
REDACTED
	svc := NewAnnouncementService(repo, nil, nil, nil)
	now := time.Unix(1776790020, 0)
	startsAt := &now
	endsAt := &now

	_, err := svc.Update(context.Background(), 1, &UpdateAnnouncementInput{
		StartsAt: &startsAt,
		EndsAt:   &endsAt,
REDACTED)
	require.ErrorIs(t, err, ErrAnnouncementInvalidSchedule)
REDACTED
