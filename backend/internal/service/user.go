package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	portuser "github.com/Wei-Shaw/sub2api/internal/port/user"
)

// Type aliases keep existing service call sites compiling while the user BC
// owns its domain types. Mirror of group/proxy/redeem/promo/announcement.
type User = domain.User
type UserListFilters = domain.UserListFilters
type UserAvatar = domain.UserAvatar
type UpsertUserAvatarInput = domain.UpsertUserAvatarInput
type UserAuthIdentityRecord = domain.UserAuthIdentityRecord
type NotifyEmailEntry = domain.NotifyEmailEntry
type UserRepository = portuser.Repository
type RedeemUserAdjustmentRepository = portuser.RedeemAdjustmentRepository

// Synthetic email domain aliases live in domain_constants.go.

// Notify email helpers re-exported from domain.
func ParseNotifyEmails(raw string) []NotifyEmailEntry {
	return domain.ParseNotifyEmails(raw)
}

func MarshalNotifyEmails(entries []NotifyEmailEntry) string {
	return domain.MarshalNotifyEmails(entries)
}

// IsSyntheticEmail re-exports the domain helper.
func IsSyntheticEmail(email string) bool {
	return domain.IsSyntheticEmail(email)
}
