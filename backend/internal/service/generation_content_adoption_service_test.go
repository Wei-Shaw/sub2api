package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type adoptionGenContentRepoStub struct {
	input  GenerationContentAdoptionInput
	called bool
}

func (s *adoptionGenContentRepoStub) Create(context.Context, *GenerationContent) error { return nil }
func (s *adoptionGenContentRepoStub) CreateVideoTaskContent(context.Context, *GenerationContent) error {
	return nil
}
func (s *adoptionGenContentRepoStub) UpdateVideoTaskAdoption(_ context.Context, input GenerationContentAdoptionInput) (*GenerationContentAdoption, error) {
	s.called = true
	s.input = input
	return &GenerationContentAdoption{TaskID: input.TaskID, AdoptionStatus: input.AdoptionStatus, Saved: true}, nil
}
func (s *adoptionGenContentRepoStub) GetWeeklyReport(context.Context, time.Time, time.Time) (*GenerationContentWeeklyReport, error) {
	return nil, nil
}
func (s *adoptionGenContentRepoStub) GetCaptureStats(context.Context) (*GenerationContentStats, error) {
	return &GenerationContentStats{}, nil
}
func (s *adoptionGenContentRepoStub) GetRecent(context.Context, int) ([]GenerationContentSample, error) {
	return nil, nil
}
func (s *adoptionGenContentRepoStub) PurgeExpiredContent(context.Context, time.Time, int, bool) (int64, error) {
	return 0, nil
}

func TestGenerationContentAdoptionServiceSubmitOwnedTask(t *testing.T) {
	repo := &adoptionGenContentRepoStub{}
	videoRepo := newMemoryVideoGatewayRepo()
	require.NoError(t, videoRepo.CreateTask(context.Background(), &VideoTask{CreatedBy: 7}))
	taskID := int64(1)
	cfg := &config.Config{}
	cfg.Gateway.ContentCapture.Enabled = true
	videoSvc := NewVideoGatewayService(videoRepo, noopVideoKeyEncryptor{}, cfg)
	svc := NewGenerationContentAdoptionService(videoSvc, repo, cfg)

	score := 0.8
	got, err := svc.SubmitVideoTaskAdoption(context.Background(), 7, GenerationContentAdoptionInput{
		TaskID:         taskID,
		AdoptionStatus: "adopted",
		QualityScore:   &score,
		Notes:          "good",
	})
	require.NoError(t, err)
	require.True(t, repo.called)
	require.True(t, got.Saved)
	require.Equal(t, "adopted", repo.input.AdoptionStatus)
}

func TestGenerationContentAdoptionServiceRejectsForeignTask(t *testing.T) {
	repo := &adoptionGenContentRepoStub{}
	videoRepo := newMemoryVideoGatewayRepo()
	require.NoError(t, videoRepo.CreateTask(context.Background(), &VideoTask{CreatedBy: 99}))
	taskID := int64(1)
	cfg := &config.Config{}
	cfg.Gateway.ContentCapture.Enabled = true
	videoSvc := NewVideoGatewayService(videoRepo, noopVideoKeyEncryptor{}, cfg)
	svc := NewGenerationContentAdoptionService(videoSvc, repo, cfg)

	_, err := svc.SubmitVideoTaskAdoption(context.Background(), 7, GenerationContentAdoptionInput{
		TaskID:         taskID,
		AdoptionStatus: "adopted",
	})
	require.Error(t, err)
	require.Equal(t, 403, infraerrors.Code(err))
	require.False(t, repo.called)
}

func TestVideoGatewayServiceAssertTaskOwnedByForbidden(t *testing.T) {
	videoRepo := newMemoryVideoGatewayRepo()
	require.NoError(t, videoRepo.CreateTask(context.Background(), &VideoTask{CreatedBy: 2}))
	svc := NewVideoGatewayService(videoRepo, noopVideoKeyEncryptor{}, &config.Config{})
	err := svc.AssertTaskOwnedBy(context.Background(), 1, 7)
	require.Error(t, err)
	require.Equal(t, 403, infraerrors.Code(err))
}
