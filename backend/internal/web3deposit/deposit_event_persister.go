package web3deposit

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrDepositEventChainMismatch  = errors.New("web3 deposit event chain does not match configured chain")
	ErrDepositDecimalsUnsupported = errors.New("web3 deposit token decimals are unsupported")
)

type DetectedDepositStore interface {
	UpsertDetected(ctx context.Context, deposit Deposit) (Deposit, error)
}

type DepositEventPersister struct {
	store  DetectedDepositStore
	config ChainConfig
}

func NewDepositEventPersister(store DetectedDepositStore, config ChainConfig) *DepositEventPersister {
	return &DepositEventPersister{
		store:  store,
		config: config,
	}
}

func (p *DepositEventPersister) PersistDetected(ctx context.Context, matches []MatchedTransferEvent) ([]Deposit, error) {
	if len(matches) == 0 {
		return []Deposit{}, nil
	}

	deposits := make([]Deposit, 0, len(matches))
	for _, match := range matches {
		deposit, err := p.detectedDeposit(match)
		if err != nil {
			return nil, err
		}
		stored, err := p.store.UpsertDetected(ctx, deposit)
		if err != nil {
			return nil, fmt.Errorf("persist detected web3 deposit event: %w", err)
		}
		deposits = append(deposits, stored)
	}
	return deposits, nil
}

func (p *DepositEventPersister) detectedDeposit(match MatchedTransferEvent) (Deposit, error) {
	event := match.Event
	if event.ID.ChainID != p.config.ChainID {
		return Deposit{}, ErrDepositEventChainMismatch
	}
	if p.config.TokenDecimals != USDT0Decimals {
		return Deposit{}, ErrDepositDecimalsUnsupported
	}

	rawAmount := event.RawAmount()
	amounts, err := ConvertUSDT0Amount(rawAmount)
	if err != nil {
		return Deposit{}, fmt.Errorf("convert detected web3 deposit amount: %w", err)
	}
	return Deposit{
		UserID:           match.UserID,
		DepositAddressID: match.DepositAddressID,
		ChainID:          event.ID.ChainID,
		TokenContract:    normalizeEVMAddress(p.config.TokenAddress),
		TxHash:           event.ID.TxHash.Hex(),
		LogIndex:         event.ID.LogIndex,
		BlockNumber:      event.BlockNumber,
		BlockHash:        event.BlockHash.Hex(),
		FromAddress:      normalizeEVMAddress(event.From),
		ToAddress:        normalizeEVMAddress(event.To),
		RawAmount:        rawAmount.String(),
		TokenDecimals:    p.config.TokenDecimals,
		TokenAmount:      amounts.TokenAmount.StringFixed(p.config.TokenDecimals),
		Status:           DepositStatusDetected,
	}, nil
}
