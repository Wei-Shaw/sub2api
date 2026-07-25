package domain

import (
	"encoding/json"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

// User-domain errors used by persistence and application layers.
// ErrUserNotFound already lives in affiliate.go as a shared sentinel.
var (
	ErrEmailExists             = infraerrors.Conflict("EMAIL_EXISTS", "email already exists")
	ErrIdentityProviderInvalid = infraerrors.BadRequest("IDENTITY_PROVIDER_INVALID", "identity provider is invalid")
)

// Synthetic email domains for OAuth providers that do not supply a real email
// (RFC reserved .invalid TLD).
const (
	LinuxDoConnectSyntheticEmailDomain  = "@linuxdo-connect.invalid"
	OIDCConnectSyntheticEmailDomain     = "@oidc-connect.invalid"
	WeChatConnectSyntheticEmailDomain   = "@wechat-connect.invalid"
	DingTalkConnectSyntheticEmailDomain = "@dingtalk-connect.invalid"
)

// NotifyEmailEntry is a notification email with enable/disable and verification state.
type NotifyEmailEntry struct {
	Email    string `json:"email"`
	Disabled bool   `json:"disabled"`
	Verified bool   `json:"verified"`
}

// ParseNotifyEmails parses a JSON string into []NotifyEmailEntry.
// Supports both old ["email"] and new [{email,disabled,verified}] formats.
func ParseNotifyEmails(raw string) []NotifyEmailEntry {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}

	var entries []NotifyEmailEntry
	if err := json.Unmarshal([]byte(raw), &entries); err == nil && len(entries) > 0 {
		if !isOldStringArrayFormat(raw) {
			return entries
		}
	}

	var emails []string
	if err := json.Unmarshal([]byte(raw), &emails); err == nil {
		result := make([]NotifyEmailEntry, 0, len(emails))
		for _, e := range emails {
			e = strings.TrimSpace(e)
			if e != "" {
				result = append(result, NotifyEmailEntry{
					Email:    e,
					Disabled: false,
					Verified: false,
				})
			}
		}
		return result
	}

	return nil
}

func isOldStringArrayFormat(raw string) bool {
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &arr); err != nil || len(arr) == 0 {
		return false
	}
	first := strings.TrimSpace(string(arr[0]))
	return len(first) > 0 && first[0] == '"'
}

// MarshalNotifyEmails serializes []NotifyEmailEntry to JSON string.
func MarshalNotifyEmails(entries []NotifyEmailEntry) string {
	if len(entries) == 0 {
		return "[]"
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// UserListFilters contains filter options for listing users.
type UserListFilters struct {
	Status    string
	Role      string
	Search    string
	GroupName string
	// APIKeyGroupID filters users who own at least one non-soft-deleted API key
	// bound to this group. 0 = no filter.
	APIKeyGroupID int64
	Attributes    map[int64]string
	// IncludeSubscriptions controls whether ListWithFilters loads active subscriptions.
	// nil means default (load for backward compatibility).
	IncludeSubscriptions *bool
	// IncludeDeleted bypasses soft-delete filtering when true.
	IncludeDeleted bool
}

// UserAvatar is a stored avatar reference for a user.
type UserAvatar struct {
	StorageProvider string
	StorageKey      string
	URL             string
	ContentType     string
	ByteSize        int
	SHA256          string
}

// UpsertUserAvatarInput is the write payload for avatar upserts.
type UpsertUserAvatarInput struct {
	StorageProvider string
	StorageKey      string
	URL             string
	ContentType     string
	ByteSize        int
	SHA256          string
}

// UserAuthIdentityRecord is a bound third-party identity for a user.
type UserAuthIdentityRecord struct {
	ProviderType    string
	ProviderKey     string
	ProviderSubject string
	VerifiedAt      *time.Time
	Issuer          *string
	Metadata        map[string]any
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// User is the core user aggregate.
// Nested API keys are intentionally omitted here (never populated by persistence);
// active subscriptions may be loaded by list filters.
type User struct {
	ID             int64
	Email          string
	Username       string
	Notes          string
	AvatarURL      string
	AvatarSource   string
	AvatarMIME     string
	AvatarByteSize int
	AvatarSHA256   string
	PasswordHash   string
	Role           string
	Balance        float64
	FrozenBalance  float64
	Concurrency    int
	Status         string
	AllowedGroups  []int64
	TokenVersion   int64
	// TokenVersionResolved indicates TokenVersion already contains the
	// fingerprint-derived value expected in JWT claims / refresh state.
	TokenVersionResolved bool
	SignupSource         string
	LastLoginAt          *time.Time
	LastActiveAt         *time.Time
	LastUsedAt           *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
	DeletedAt            *time.Time

	// GroupRates is per-user exclusive group rate config: map[groupID]rate.
	GroupRates map[int64]float64

	// TOTP fields.
	TotpSecretEncrypted *string
	TotpEnabled         bool
	TotpEnabledAt       *time.Time

	// Balance notification settings.
	BalanceNotifyEnabled       bool
	BalanceNotifyThresholdType string // "fixed" (default) | "percentage"
	BalanceNotifyThreshold     *float64
	BalanceNotifyExtraEmails   []NotifyEmailEntry
	TotalRecharged             float64

	// RPMLimit is the user-level requests-per-minute cap (0 = unlimited).
	RPMLimit int

	// UserGroupRPMOverride is a non-persisted auth-cache snapshot field.
	UserGroupRPMOverride *int

	// Optional projections populated by some list paths.
	Subscriptions []UserSubscription
}

func (u *User) IsAdmin() bool {
	return u != nil && u.Role == RoleAdmin
}

func (u *User) IsActive() bool {
	return u != nil && u.Status == StatusActive
}

// CanBindGroup reports whether a user can bind to a group.
// Public (non-exclusive) groups are open to all users; exclusive groups require
// membership in AllowedGroups.
func (u *User) CanBindGroup(groupID int64, isExclusive bool) bool {
	if u == nil {
		return false
	}
	if !isExclusive {
		return true
	}
	for _, id := range u.AllowedGroups {
		if id == groupID {
			return true
		}
	}
	return false
}

// SetPassword hashes and stores a password.
func (u *User) SetPassword(password string) error {
	if u == nil {
		return infraerrors.BadRequest("INVALID_USER", "user is nil")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hash)
	return nil
}

// CheckPassword verifies a plaintext password against PasswordHash.
func (u *User) CheckPassword(password string) bool {
	if u == nil {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) == nil
}

// IsSyntheticEmail reports whether email uses a reserved OAuth synthetic domain.
func IsSyntheticEmail(email string) bool {
	normalized := strings.ToLower(strings.TrimSpace(email))
	return strings.HasSuffix(normalized, LinuxDoConnectSyntheticEmailDomain) ||
		strings.HasSuffix(normalized, OIDCConnectSyntheticEmailDomain) ||
		strings.HasSuffix(normalized, WeChatConnectSyntheticEmailDomain) ||
		strings.HasSuffix(normalized, DingTalkConnectSyntheticEmailDomain)
}
