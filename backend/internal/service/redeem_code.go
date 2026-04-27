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
	CreatedAt time.Time
	ExpiresAt *time.Time

	GroupID      *int64
	ValidityDays int

	User  *User
	Group *Group
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
