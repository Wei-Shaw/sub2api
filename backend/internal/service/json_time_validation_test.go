package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAnnouncementServiceRejectsOutOfRangeDates(t *testing.T) {
	for _, year := range []int{-1, 10000} {
		date := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		for _, field := range []string{"start", "end"} {
			t.Run(date.String()+field, func(t *testing.T) {
				repo := &announcementRepoStub{}
				svc := NewAnnouncementService(repo, nil, nil, nil)
				input := &CreateAnnouncementInput{Title: "test", Content: "test"}
				if field == "start" {
					input.StartsAt = &date
				} else {
					input.EndsAt = &date
				}
				_, err := svc.Create(context.Background(), input)
				require.ErrorIs(t, err, ErrAnnouncementInvalidSchedule)
				require.Nil(t, repo.item, "invalid dates must never reach persistence")

				repo.item = &Announcement{ID: 1, Title: "test", Content: "test"}
				update := &UpdateAnnouncementInput{}
				ptr := &date
				if field == "start" {
					update.StartsAt = &ptr
				} else {
					update.EndsAt = &ptr
				}
				_, err = svc.Update(context.Background(), 1, update)
				require.ErrorIs(t, err, ErrAnnouncementInvalidSchedule)
				require.Nil(t, repo.item.StartsAt)
				require.Nil(t, repo.item.EndsAt)
			})
		}
	}
}

func TestAnnouncementServiceAcceptsJSONDateBoundariesAndClear(t *testing.T) {
	start := time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	repo := &announcementRepoStub{}
	svc := NewAnnouncementService(repo, nil, nil, nil)
	a, err := svc.Create(context.Background(), &CreateAnnouncementInput{Title: "test", Content: "test", StartsAt: &start, EndsAt: &end})
	require.NoError(t, err)
	_, err = json.Marshal(a)
	require.NoError(t, err)
	var clear *time.Time
	a, err = svc.Update(context.Background(), 1, &UpdateAnnouncementInput{StartsAt: &clear, EndsAt: &clear})
	require.NoError(t, err)
	require.Nil(t, a.StartsAt)
	require.Nil(t, a.EndsAt)
}
