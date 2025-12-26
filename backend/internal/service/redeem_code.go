package service

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

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

	GroupID      *int64
	ValidityDays int

	User  *User
	Group *Group
REDACTED

func (r *RedeemCode) IsUsed() bool {
	return r.Status == StatusUsed
REDACTED

func (r *RedeemCode) CanUse() bool {
	return r.Status == StatusUnused
REDACTED

func GenerateRedeemCode() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
REDACTED
	return hex.EncodeToString(b), nil
REDACTED
