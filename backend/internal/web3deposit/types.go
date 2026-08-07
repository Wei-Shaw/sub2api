package web3deposit

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
)

var (
	ErrRawAmountRequired = errors.New("web3 deposit raw amount is required")
	ErrRawAmountNegative = errors.New("web3 deposit raw amount must not be negative")
)

type ChainConfig struct {
	ChainID         uint64
	TokenAddress    common.Address
	TokenDecimals   int32
	WalletID        string
	AccountPath     string
	ScanStartBlock  uint64
	MinimumDeposit  decimal.Decimal
	AutoCreditLimit decimal.Decimal
}

type DepositEventID struct {
	ChainID  uint64
	TxHash   common.Hash
	LogIndex uint64
}

type TransferEvent struct {
	ID          DepositEventID
	BlockNumber uint64
	BlockHash   common.Hash
	From        common.Address
	To          common.Address
	rawAmount   *big.Int
}

func NewTransferEvent(
	id DepositEventID,
	blockNumber uint64,
	blockHash common.Hash,
	from common.Address,
	to common.Address,
	rawAmount *big.Int,
) (TransferEvent, error) {
	if err := validateUint256(rawAmount); err != nil {
		return TransferEvent{}, err
	}
	return TransferEvent{
		ID:          id,
		BlockNumber: blockNumber,
		BlockHash:   blockHash,
		From:        from,
		To:          to,
		rawAmount:   new(big.Int).Set(rawAmount),
	}, nil
}

func (e TransferEvent) RawAmount() *big.Int {
	if e.rawAmount == nil {
		return nil
	}
	return new(big.Int).Set(e.rawAmount)
}
