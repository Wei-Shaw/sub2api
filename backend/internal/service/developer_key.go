package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	developerKeyPrefix    = "dev_"
	developerKeyRandBytes = 32
	developerKeyNameMax   = 100
	developerKeyViewChars = 12
)

var (
	ErrDeveloperKeyNotFound = infraerrors.NotFound("DEVELOPER_KEY_NOT_FOUND", "developer key not found")
	ErrDeveloperKeyInvalid  = infraerrors.Unauthorized("INVALID_DEVELOPER_KEY", "invalid developer key")
	ErrDeveloperKeyName     = infraerrors.BadRequest("INVALID_DEVELOPER_KEY_NAME", "developer key name is invalid")
)

// DeveloperKey is the safe developer credential view. It never contains the
// plaintext credential or its digest.
type DeveloperKey struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"-"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type DeveloperKeyRepository interface {
	Create(ctx context.Context, key *DeveloperKey, hash string) (*DeveloperKey, error)
	ListByUserID(ctx context.Context, userID int64) ([]*DeveloperKey, error)
	DeleteByUserID(ctx context.Context, userID, id int64) error
	GetByHash(ctx context.Context, hash string) (*DeveloperKey, error)
	TouchLastUsed(ctx context.Context, id int64, at time.Time) error
}

type DeveloperKeyService struct {
	repo     DeveloperKeyRepository
	userRepo UserRepository
	randRead func([]byte) (int, error)
	now      func() time.Time
}

func NewDeveloperKeyService(repo DeveloperKeyRepository, userRepo UserRepository) *DeveloperKeyService {
	return &DeveloperKeyService{repo: repo, userRepo: userRepo, randRead: rand.Read, now: time.Now}
}

// Create returns a safe view and the plaintext credential. The plaintext is
// intentionally not recoverable after this call.
func (s *DeveloperKeyService) Create(ctx context.Context, userID int64, name string) (*DeveloperKey, string, error) {
	if s == nil || s.repo == nil {
		return nil, "", fmt.Errorf("developer key: nil repository")
	}
	if userID <= 0 {
		return nil, "", ErrDeveloperKeyInvalid
	}
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > developerKeyNameMax {
		return nil, "", ErrDeveloperKeyName
	}
	buf := make([]byte, developerKeyRandBytes)
	if _, err := s.randRead(buf); err != nil {
		return nil, "", fmt.Errorf("developer key: generate secret: %w", err)
	}
	plaintext := developerKeyPrefix + base64.RawURLEncoding.EncodeToString(buf)
	viewPrefix := plaintext
	if len(viewPrefix) > developerKeyViewChars {
		viewPrefix = viewPrefix[:developerKeyViewChars]
	}
	created, err := s.repo.Create(ctx, &DeveloperKey{
		UserID:    userID,
		Name:      name,
		KeyPrefix: viewPrefix,
	}, developerKeyHash(plaintext))
	if err != nil {
		return nil, "", err
	}
	return created, plaintext, nil
}

func (s *DeveloperKeyService) List(ctx context.Context, userID int64) ([]*DeveloperKey, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("developer key: nil repository")
	}
	if userID <= 0 {
		return nil, ErrDeveloperKeyInvalid
	}
	return s.repo.ListByUserID(ctx, userID)
}

func (s *DeveloperKeyService) Delete(ctx context.Context, userID, id int64) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("developer key: nil repository")
	}
	if userID <= 0 || id <= 0 {
		return ErrDeveloperKeyNotFound
	}
	return s.repo.DeleteByUserID(ctx, userID, id)
}

func (s *DeveloperKeyService) Authenticate(ctx context.Context, plaintext string) (*DeveloperKey, error) {
	if s == nil || s.repo == nil || s.userRepo == nil {
		return nil, fmt.Errorf("developer key: nil dependencies")
	}
	plaintext = strings.TrimSpace(plaintext)
	if !strings.HasPrefix(plaintext, developerKeyPrefix) || len(plaintext) != len(developerKeyPrefix)+base64.RawURLEncoding.EncodedLen(developerKeyRandBytes) {
		return nil, ErrDeveloperKeyInvalid
	}
	key, err := s.repo.GetByHash(ctx, developerKeyHash(plaintext))
	if err != nil {
		if errors.Is(err, ErrDeveloperKeyNotFound) {
			return nil, ErrDeveloperKeyInvalid
		}
		return nil, err
	}
	user, err := s.userRepo.GetByID(ctx, key.UserID)
	if err != nil || user == nil || !user.IsActive() {
		return nil, ErrDeveloperKeyInvalid
	}
	now := s.now().UTC()
	if err := s.repo.TouchLastUsed(ctx, key.ID, now); err != nil {
		return nil, err
	}
	key.LastUsedAt = &now
	return key, nil
}

func developerKeyHash(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
