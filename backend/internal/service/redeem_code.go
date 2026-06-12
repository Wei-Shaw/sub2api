package service

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

const redeemCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

type RedeemCode struct {
	ID        int64
	Code      string
	Type      string
	Value     float64
	Status    string
	UsedBy    *int64
	UsedAt    *time.Time
	Notes     string
	CreatedBy *int64
	CreatedAt time.Time
	ExpiresAt *time.Time

	GroupID      *int64
	ValidityDays int

	UsagePolicy    string
	UsageScope     string
	MaxTotalUses   *int
	MaxUsesPerUser *int
	UsedCount      int

	User          *User
	Group         *Group
	CreatedByUser *User
}

type RedeemCodeUsage struct {
	ID                   int64
	RedeemCodeID         int64
	UsageScope           string
	UserID               int64
	CodeSnapshot         string
	TypeSnapshot         string
	ValueSnapshot        float64
	GroupIDSnapshot      *int64
	ValidityDaysSnapshot int
	UsedAt               time.Time
	Metadata             map[string]any
	RedeemCode           *RedeemCode
	User                 *User
}

func (r *RedeemCode) IsUsed() bool {
	return r.Status == StatusUsed
}

func (r *RedeemCode) IsExpired() bool {
	return r.IsExpiredAt(time.Now())
}

func (r *RedeemCode) IsExpiredAt(now time.Time) bool {
	if r == nil {
		return false
	}
	if r.Status == StatusExpired {
		return true
	}
	return r.Status == StatusUnused && r.ExpiresAt != nil && !r.ExpiresAt.After(now)
}

func (r *RedeemCode) CanUse() bool {
	return r.Status == StatusUnused && !r.IsExpired()
}

func NormalizeRedeemUsagePolicy(policy string) string {
	switch strings.TrimSpace(strings.ToLower(policy)) {
	case "", RedeemUsagePolicySingleUse:
		return RedeemUsagePolicySingleUse
	case RedeemUsagePolicyOncePerUser:
		return RedeemUsagePolicyOncePerUser
	default:
		return strings.TrimSpace(strings.ToLower(policy))
	}
}

func NormalizeRedeemUsageScope(scope string) string {
	return strings.TrimSpace(scope)
}

func (r *RedeemCode) EffectiveUsagePolicy() string {
	if r == nil {
		return RedeemUsagePolicySingleUse
	}
	return NormalizeRedeemUsagePolicy(r.UsagePolicy)
}

func (r *RedeemCode) EffectiveUsageScope() string {
	if r == nil {
		return ""
	}
	scope := NormalizeRedeemUsageScope(r.UsageScope)
	if scope != "" {
		return scope
	}
	return NormalizeRedeemCode(r.Code)
}

func (r *RedeemCode) HasTotalUsageRemaining() bool {
	if r == nil {
		return false
	}
	if r.MaxTotalUses == nil || *r.MaxTotalUses <= 0 {
		return true
	}
	return r.UsedCount < *r.MaxTotalUses
}

func GenerateRedeemCode() (string, error) {
	return GenerateRedeemCodeForType(RedeemTypeBalance)
}

func GenerateRedeemCodeForType(codeType string) (string, error) {
	prefix, err := redeemCodePrefix(codeType)
	if err != nil {
		return "", err
	}

	parts := make([]string, 3)
	for i := range parts {
		part, err := generateRedeemCodeBlock(4)
		if err != nil {
			return "", err
		}
		parts[i] = part
	}

	return prefix + "-" + strings.Join(parts, "-"), nil
}

func NormalizeRedeemCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func redeemCodePrefix(codeType string) (string, error) {
	switch codeType {
	case RedeemTypeBalance, AdjustmentTypeAdminBalance:
		return "BAL", nil
	case RedeemTypeSubscription:
		return "SUB", nil
	case RedeemTypeInvitation:
		return "INV", nil
	case RedeemTypeDeviceClaim:
		return "DCL", nil
	case RedeemTypeDeviceLogin:
		return "DLG", nil
	case RedeemTypeConcurrency, AdjustmentTypeAdminConcurrency:
		return "CON", nil
	default:
		return "", fmt.Errorf("unsupported redeem code type: %s", codeType)
	}
}

func generateRedeemCodeBlock(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	buf := make([]byte, length)
	for i, value := range b {
		buf[i] = redeemCodeAlphabet[int(value)%len(redeemCodeAlphabet)]
	}

	return string(buf), nil
}
