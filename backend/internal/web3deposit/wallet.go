package web3deposit

import (
	"errors"
	"time"
)

const MaxDerivationIndexExclusive int64 = 1 << 31

var (
	ErrWalletNotFound      = errors.New("web3 deposit wallet not found")
	ErrWalletAlreadyExists = errors.New("web3 deposit wallet already exists")
)

type WalletStatus string

const (
	WalletStatusActive   WalletStatus = "active"
	WalletStatusDisabled WalletStatus = "disabled"
)

func (s WalletStatus) IsValid() bool {
	return s == WalletStatusActive || s == WalletStatusDisabled
}

type WalletMetadata struct {
	ID                  int64
	WalletID            string
	AccountPath         string
	XPubFingerprint     string
	NextDerivationIndex int64
	Status              WalletStatus
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
