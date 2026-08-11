package web3deposit

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestDepositEventIDIncludesLogIndexAndChain(t *testing.T) {
	t.Parallel()

	txHash := common.HexToHash("0x1234")
	first := DepositEventID{ChainID: 1030, TxHash: txHash, LogIndex: 1}
	secondLog := DepositEventID{ChainID: 1030, TxHash: txHash, LogIndex: 2}
	secondChain := DepositEventID{ChainID: 71, TxHash: txHash, LogIndex: 1}

	require.NotEqual(t, first, secondLog)
	require.NotEqual(t, first, secondChain)
	require.Len(t, map[DepositEventID]struct{}{first: {}, secondLog: {}, secondChain: {}}, 3)
}

func TestNewTransferEventCopiesRawAmount(t *testing.T) {
	t.Parallel()

	rawAmount := big.NewInt(1_000_000)
	event, err := NewTransferEvent(
		DepositEventID{ChainID: 1030, TxHash: common.HexToHash("0x1234"), LogIndex: 7},
		123,
		common.HexToHash("0xabcd"),
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
		rawAmount,
	)
	require.NoError(t, err)

	rawAmount.SetInt64(2)
	require.Equal(t, "1000000", event.RawAmount().String())

	returnedAmount := event.RawAmount()
	returnedAmount.SetInt64(3)
	require.Equal(t, "1000000", event.RawAmount().String())
}

func TestNewTransferEventValidatesRawAmount(t *testing.T) {
	t.Parallel()

	aboveUint256 := new(big.Int).Lsh(big.NewInt(1), 256)
	tests := []struct {
		name      string
		rawAmount *big.Int
		wantErr   error
	}{
		{name: "nil", wantErr: ErrRawAmountRequired},
		{name: "negative", rawAmount: big.NewInt(-1), wantErr: ErrRawAmountNegative},
		{name: "above uint256", rawAmount: aboveUint256, wantErr: ErrRawAmountExceedsUint256},
		{name: "zero", rawAmount: new(big.Int)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			event, err := NewTransferEvent(DepositEventID{}, 0, common.Hash{}, common.Address{}, common.Address{}, test.rawAmount)
			if test.wantErr != nil {
				require.True(t, errors.Is(err, test.wantErr))
				return
			}
			require.NoError(t, err)
			require.Zero(t, event.RawAmount().Sign())
		})
	}
}
