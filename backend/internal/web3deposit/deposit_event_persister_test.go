package web3deposit

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestDepositEventPersisterStoresMatchedEventsInOrder(t *testing.T) {
	config := testDepositPersisterChainConfig()
	firstEvent := newDepositPersisterEvent(t, config.ChainID, 7, 1_234_567)
	secondEvent := newDepositPersisterEvent(t, config.ChainID, 8, 2_000_000)
	matches := []MatchedTransferEvent{
		{Event: firstEvent, DepositAddressID: 11, UserID: 101},
		{Event: secondEvent, DepositAddressID: 12, UserID: 102},
	}
	store := &detectedDepositStoreStub{}

	deposits, err := NewDepositEventPersister(store, config).PersistDetected(context.Background(), matches)
	require.NoError(t, err)
	require.Len(t, deposits, 2)
	require.Equal(t, []int64{1, 2}, []int64{deposits[0].ID, deposits[1].ID})
	require.Equal(t, []Deposit{deposits[0], deposits[1]}, store.deposits)

	first := deposits[0]
	require.Equal(t, int64(101), first.UserID)
	require.Equal(t, int64(11), first.DepositAddressID)
	require.Equal(t, config.ChainID, first.ChainID)
	require.Equal(t, "0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff", first.TokenContract)
	require.Equal(t, firstEvent.ID.TxHash.Hex(), first.TxHash)
	require.Equal(t, firstEvent.ID.LogIndex, first.LogIndex)
	require.Equal(t, firstEvent.BlockNumber, first.BlockNumber)
	require.Equal(t, firstEvent.BlockHash.Hex(), first.BlockHash)
	require.Equal(t, "0x1111111111111111111111111111111111111111", first.FromAddress)
	require.Equal(t, "0x1234567890abcdef1234567890abcdef12345678", first.ToAddress)
	require.Equal(t, "1234567", first.RawAmount)
	require.Equal(t, int32(6), first.TokenDecimals)
	require.Equal(t, "1.234567", first.TokenAmount)
	require.Nil(t, first.CreditedAmount)
	require.Equal(t, DepositStatusDetected, first.Status)
}

func TestDepositEventPersisterFailsWholeResultOnStoreError(t *testing.T) {
	config := testDepositPersisterChainConfig()
	wantErr := errors.New("database unavailable")
	store := &detectedDepositStoreStub{errAtCall: 2, err: wantErr}
	matches := []MatchedTransferEvent{
		{Event: newDepositPersisterEvent(t, config.ChainID, 7, 1_000_000), DepositAddressID: 11, UserID: 101},
		{Event: newDepositPersisterEvent(t, config.ChainID, 8, 2_000_000), DepositAddressID: 12, UserID: 102},
	}

	deposits, err := NewDepositEventPersister(store, config).PersistDetected(context.Background(), matches)
	require.ErrorIs(t, err, wantErr)
	require.Nil(t, deposits)
	require.Len(t, store.deposits, 2)
}

func TestDepositEventPersisterRejectsInvalidEventsBeforeWriting(t *testing.T) {
	config := testDepositPersisterChainConfig()
	tests := []struct {
		name    string
		event   TransferEvent
		wantErr error
	}{
		{
			name:    "wrong chain",
			event:   newDepositPersisterEvent(t, 71, 7, 1_000_000),
			wantErr: ErrDepositEventChainMismatch,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &detectedDepositStoreStub{}
			deposits, err := NewDepositEventPersister(store, config).PersistDetected(context.Background(), []MatchedTransferEvent{{
				Event:            test.event,
				DepositAddressID: 11,
				UserID:           101,
			}})
			require.ErrorIs(t, err, test.wantErr)
			require.Nil(t, deposits)
			require.Empty(t, store.deposits)
		})
	}
}

func TestDepositEventPersisterSkipsZeroAmountWithoutBlockingLaterEvents(t *testing.T) {
	config := testDepositPersisterChainConfig()
	store := &detectedDepositStoreStub{}
	matches := []MatchedTransferEvent{
		{Event: newDepositPersisterEvent(t, config.ChainID, 7, 0), DepositAddressID: 11, UserID: 101},
		{Event: newDepositPersisterEvent(t, config.ChainID, 8, 2_000_000), DepositAddressID: 12, UserID: 102},
	}

	deposits, err := NewDepositEventPersister(store, config).PersistDetected(context.Background(), matches)

	require.NoError(t, err)
	require.Len(t, deposits, 1)
	require.Equal(t, uint64(8), deposits[0].LogIndex)
	require.Len(t, store.deposits, 1)
}

func TestDepositEventPersisterRejectsUnsupportedTokenDecimals(t *testing.T) {
	config := testDepositPersisterChainConfig()
	config.TokenDecimals = 18
	store := &detectedDepositStoreStub{}

	deposits, err := NewDepositEventPersister(store, config).PersistDetected(context.Background(), []MatchedTransferEvent{{
		Event:            newDepositPersisterEvent(t, config.ChainID, 7, 1_000_000),
		DepositAddressID: 11,
		UserID:           101,
	}})
	require.ErrorIs(t, err, ErrDepositDecimalsUnsupported)
	require.Nil(t, deposits)
	require.Empty(t, store.deposits)
}

func TestDepositEventPersisterSkipsStoreForEmptyMatches(t *testing.T) {
	store := &detectedDepositStoreStub{}

	deposits, err := NewDepositEventPersister(store, testDepositPersisterChainConfig()).PersistDetected(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, deposits)
	require.Empty(t, deposits)
	require.Empty(t, store.deposits)
}

func testDepositPersisterChainConfig() ChainConfig {
	return ChainConfig{
		ChainID:       1030,
		TokenAddress:  common.HexToAddress("0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff"),
		TokenDecimals: USDT0Decimals,
	}
}

func newDepositPersisterEvent(t *testing.T, chainID, logIndex uint64, rawAmount int64) TransferEvent {
	t.Helper()
	event, err := NewTransferEvent(
		DepositEventID{
			ChainID:  chainID,
			TxHash:   common.BigToHash(new(big.Int).SetUint64(logIndex)),
			LogIndex: logIndex,
		},
		12345+logIndex,
		common.BigToHash(new(big.Int).SetUint64(100+logIndex)),
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678"),
		big.NewInt(rawAmount),
	)
	require.NoError(t, err)
	return event
}

type detectedDepositStoreStub struct {
	deposits  []Deposit
	errAtCall int
	err       error
}

func (s *detectedDepositStoreStub) UpsertDetected(_ context.Context, deposit Deposit) (Deposit, error) {
	s.deposits = append(s.deposits, deposit)
	if len(s.deposits) == s.errAtCall {
		return Deposit{}, s.err
	}
	deposit.ID = int64(len(s.deposits))
	s.deposits[len(s.deposits)-1] = deposit
	return deposit, nil
}
