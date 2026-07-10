package service

import (
	"context"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// GenerationContentAdoptionService handles employee API-key adoption feedback on owned video tasks.
type GenerationContentAdoptionService struct {
	video *VideoGatewayService
	repo  GenerationContentRepository
	cfg   *config.Config
}

func NewGenerationContentAdoptionService(
	video *VideoGatewayService,
	repo GenerationContentRepository,
	cfg *config.Config,
) *GenerationContentAdoptionService {
	return &GenerationContentAdoptionService{video: video, repo: repo, cfg: cfg}
}

// SubmitVideoTaskAdoption persists adoption feedback when the caller owns the video task.
func (s *GenerationContentAdoptionService) SubmitVideoTaskAdoption(
	ctx context.Context,
	userID int64,
	input GenerationContentAdoptionInput,
) (*GenerationContentAdoption, error) {
	if s == nil || s.repo == nil || s.video == nil {
		return nil, infraerrors.InternalServer("GENERATION_CONTENT_ADOPTION_UNAVAILABLE", "adoption service is not configured")
	}
	if userID <= 0 {
		return nil, infraerrors.Unauthorized("USER_REQUIRED", "authenticated user is required")
	}
	input.AdoptionStatus = normalizeAdoptionStatus(input.AdoptionStatus)
	if err := validateGenerationContentAdoptionInput(input); err != nil {
		return nil, err
	}
	if err := s.video.AssertTaskOwnedBy(ctx, input.TaskID, userID); err != nil {
		return nil, err
	}
	if !s.adoptionFeedbackEnabled() {
		return &GenerationContentAdoption{
			TaskID:         input.TaskID,
			AdoptionStatus: input.AdoptionStatus,
			QualityScore:   input.QualityScore,
			Notes:          input.Notes,
			Saved:          false,
		}, nil
	}
	return s.repo.UpdateVideoTaskAdoption(ctx, input)
}

func (s *GenerationContentAdoptionService) adoptionFeedbackEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.ContentCapture.Enabled
}

// AdoptionFeedbackEnabled reports whether content capture adoption writes are enabled.
func (s *GenerationContentAdoptionService) AdoptionFeedbackEnabled() bool {
	return s.adoptionFeedbackEnabled()
}

func validateGenerationContentAdoptionInput(input GenerationContentAdoptionInput) error {
	if input.TaskID <= 0 {
		return infraerrors.BadRequest("INVALID_TASK_ID", "invalid task_id")
	}
	if !isValidAdoptionStatus(input.AdoptionStatus) {
		return infraerrors.BadRequest("INVALID_ADOPTION_STATUS", "invalid adoption_status")
	}
	if input.QualityScore != nil && (*input.QualityScore < 0 || *input.QualityScore > 1) {
		return infraerrors.BadRequest("INVALID_QUALITY_SCORE", "quality_score must be between 0 and 1")
	}
	return nil
}

func normalizeAdoptionStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func isValidAdoptionStatus(status string) bool {
	switch status {
	case "adopted", "rejected", "pending":
		return true
	default:
		return false
	}
}

// AssertTaskOwnedBy returns Forbidden when the task exists but belongs to another user.
func (s *VideoGatewayService) AssertTaskOwnedBy(ctx context.Context, taskID, userID int64) error {
	if s == nil || s.repo == nil {
		return infraerrors.InternalServer("VIDEO_GATEWAY_UNAVAILABLE", "video gateway is not configured")
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, ErrVideoTaskNotFound) {
			return err
		}
		return err
	}
	if task.CreatedBy != userID {
		return infraerrors.Forbidden("VIDEO_TASK_FORBIDDEN", "you do not have access to this task")
	}
	return nil
}
