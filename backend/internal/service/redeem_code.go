package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const redeemValiditySecondsNotePrefix = "validity_seconds="

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
	// ValiditySeconds is used when a subscription redeem code needs sub-day
	// precision. Existing validity_days semantics stay unchanged.
	ValiditySeconds int64

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
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func NormalizeRedeemValidityDays(validityDays float64) (int, int64, error) {
	if validityDays == 0 {
		return 0, 0, nil
	}
	if math.IsNaN(validityDays) || math.IsInf(validityDays, 0) {
		return 0, 0, fmt.Errorf("validity_days must be a finite number")
	}
	if validityDays < 0 {
		if validityDays != math.Trunc(validityDays) {
			return 0, 0, fmt.Errorf("negative fractional validity_days is not supported")
		}
		return int(validityDays), 0, nil
	}

	seconds := int64(math.Round(validityDays * 24 * float64(time.Hour/time.Second)))
	if seconds <= 0 {
		seconds = 1
	}
	if seconds%(24*int64(time.Hour/time.Second)) == 0 {
		return int(seconds / (24 * int64(time.Hour/time.Second))), 0, nil
	}
	return 0, seconds, nil
}

func EncodeRedeemValiditySecondsNote(notes string, seconds int64) string {
	cleaned, _ := DecodeRedeemValiditySecondsNote(notes)
	if seconds <= 0 {
		return cleaned
	}
	line := redeemValiditySecondsNotePrefix + strconv.FormatInt(seconds, 10)
	if strings.TrimSpace(cleaned) == "" {
		return line
	}
	return cleaned + "\n" + line
}

func DecodeRedeemValiditySecondsNote(notes string) (string, int64) {
	if notes == "" {
		return "", 0
	}
	lines := strings.Split(notes, "\n")
	cleaned := make([]string, 0, len(lines))
	var seconds int64
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, redeemValiditySecondsNotePrefix) {
			if parsed, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(trimmed, redeemValiditySecondsNotePrefix)), 10, 64); err == nil && parsed > 0 {
				seconds = parsed
			}
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n")), seconds
}
