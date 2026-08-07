package web3deposit

import (
	"errors"
	"time"
)

var (
	ErrAddressNotFound           = errors.New("web3 deposit address not found")
	ErrAddressAlreadyExists      = errors.New("web3 deposit address already exists")
	ErrAddressDisabled           = errors.New("web3 deposit address is disabled")
	ErrAddressAllocationConflict = errors.New("web3 deposit address allocation conflict")
)

type AddressStatus string

const (
	AddressStatusActive   AddressStatus = "active"
	AddressStatusDisabled AddressStatus = "disabled"
)

func (s AddressStatus) IsValid() bool {
	return s == AddressStatusActive || s == AddressStatusDisabled
}

type DepositAddress struct {
	ID                int64
	UserID            int64
	WalletID          string
	DerivationIndex   int64
	Address           string
	NormalizedAddress string
	Status            AddressStatus
	AllocatedAt       time.Time
	DisabledAt        *time.Time
	LastDepositAt     *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
