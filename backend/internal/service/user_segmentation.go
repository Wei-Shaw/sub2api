package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	ErrUserTagNotFound = errors.New("user tag not found")
	ErrUserTagExists   = errors.New("user tag already exists")
)

// UserTag is an administrative user-profile label. It is deliberately separate
// from Group, which is a model routing/billing group.
type UserTag struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	NormalizedName string    `json:"normalized_name,omitempty"`
	Color          string    `json:"color"`
	Description    string    `json:"description"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// UserTagRepository owns the user profile segmentation tables. Keeping this
// port independent avoids expanding UserRepository and its many test doubles.
type UserTagRepository interface {
	List(ctx context.Context) ([]UserTag, error)
	Create(ctx context.Context, tag *UserTag) error
	Update(ctx context.Context, tag *UserTag) error
	Delete(ctx context.Context, id int64) error
	ListByUserIDs(ctx context.Context, userIDs []int64) (map[int64][]UserTag, error)
	ReplaceUserTags(ctx context.Context, userID int64, tagIDs []int64) error
	BatchAdd(ctx context.Context, userIDs, tagIDs []int64) (int, error)
	BatchRemove(ctx context.Context, userIDs, tagIDs []int64) (int, error)
	BatchReplaceTags(ctx context.Context, userIDs, tagIDs []int64) (int, error)
	FilterUserIDs(ctx context.Context, tagIDs []int64, match string) ([]int64, error)
	ListHiddenGroupIDsByUserIDs(ctx context.Context, userIDs []int64) (map[int64][]int64, error)
	ReplaceHiddenGroups(ctx context.Context, userID int64, groupIDs []int64) error
	BatchReplaceHiddenGroups(ctx context.Context, userIDs, groupIDs []int64) (int, error)
}

// UserSegmentationService contains user tags and user-specific hidden model
// groups. The latter changes visibility only; it does not grant/revoke access
// to Group and does not modify API key bindings.
type UserSegmentationService struct {
	repo UserTagRepository
}

func NewUserSegmentationService(repo UserTagRepository) *UserSegmentationService {
	return &UserSegmentationService{repo: repo}
}

func (s *UserSegmentationService) ListTags(ctx context.Context) ([]UserTag, error) {
	if s == nil || s.repo == nil {
		return []UserTag{}, nil
	}
	return s.repo.List(ctx)
}

func (s *UserSegmentationService) CreateTag(ctx context.Context, name, color, description string) (*UserTag, error) {
	tag, err := normalizeUserTagInput(name, color, description)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, tag); err != nil {
		return nil, err
	}
	return tag, nil
}

func (s *UserSegmentationService) UpdateTag(ctx context.Context, id int64, name, color, description string) (*UserTag, error) {
	if id <= 0 {
		return nil, ErrUserTagNotFound
	}
	tag, err := normalizeUserTagInput(name, color, description)
	if err != nil {
		return nil, err
	}
	tag.ID = id
	if err := s.repo.Update(ctx, tag); err != nil {
		return nil, err
	}
	return tag, nil
}

func (s *UserSegmentationService) DeleteTag(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrUserTagNotFound
	}
	return s.repo.Delete(ctx, id)
}

func (s *UserSegmentationService) BatchUpdateTags(ctx context.Context, userIDs, tagIDs []int64, mode string) (int, error) {
	userIDs = cleanPositiveIDs(userIDs)
	tagIDs = cleanPositiveIDs(tagIDs)
	if len(userIDs) == 0 {
		return 0, nil
	}
	switch mode {
	case "add":
		if len(tagIDs) == 0 {
			return 0, nil
		}
		return s.repo.BatchAdd(ctx, userIDs, tagIDs)
	case "remove":
		if len(tagIDs) == 0 {
			return 0, nil
		}
		return s.repo.BatchRemove(ctx, userIDs, tagIDs)
	case "replace":
		return s.repo.BatchReplaceTags(ctx, userIDs, tagIDs)
	default:
		return 0, fmt.Errorf("invalid tag update mode: %s", mode)
	}
}

func (s *UserSegmentationService) BatchReplaceHiddenGroups(ctx context.Context, userIDs, groupIDs []int64) (int, error) {
	userIDs = cleanPositiveIDs(userIDs)
	groupIDs = cleanPositiveIDs(groupIDs)
	if len(userIDs) == 0 {
		return 0, nil
	}
	return s.repo.BatchReplaceHiddenGroups(ctx, userIDs, groupIDs)
}

func (s *UserSegmentationService) TagsByUserIDs(ctx context.Context, userIDs []int64) (map[int64][]UserTag, error) {
	return s.repo.ListByUserIDs(ctx, cleanPositiveIDs(userIDs))
}

func (s *UserSegmentationService) HiddenGroupsByUserIDs(ctx context.Context, userIDs []int64) (map[int64][]int64, error) {
	return s.repo.ListHiddenGroupIDsByUserIDs(ctx, cleanPositiveIDs(userIDs))
}

func cleanPositiveIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

var userTagColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func normalizeUserTagInput(name, color, description string) (*UserTag, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 80 {
		return nil, fmt.Errorf("tag name must be between 1 and 80 characters")
	}
	color = strings.TrimSpace(color)
	if color == "" {
		color = "#6366f1"
	}
	if !userTagColorPattern.MatchString(color) {
		return nil, fmt.Errorf("tag color must be a six-digit hex color")
	}
	return &UserTag{
		Name:           name,
		NormalizedName: strings.ToLower(name),
		Color:          strings.ToLower(color),
		Description:    strings.TrimSpace(description),
	}, nil
}
