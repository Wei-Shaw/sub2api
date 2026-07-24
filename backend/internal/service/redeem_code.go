package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// Type aliases keep existing service call sites compiling while the redeem BC
// owns its domain types. Mirror of promo_code.go / announcement.go.

type RedeemCode = domain.RedeemCode

type RedeemUsageUser = domain.RedeemUsageUser

type RedeemUsageGroup = domain.RedeemUsageGroup

// GenerateRedeemCode re-exports the domain helper.
func GenerateRedeemCode() (string, error) {
	return domain.GenerateRedeemCode()
}
