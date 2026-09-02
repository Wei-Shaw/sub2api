package web3deposit

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

func TestTransferLogFetcherUsesFixedFilterAndSortsLogs(t *testing.T) {
	tokenAddress := common.HexToAddress("0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff")
	from := common.HexToAddress("0x1111111111111111111111111111111111111111")
	to := common.HexToAddress("0x2222222222222222222222222222222222222222")
	first := validERC20TransferLog(tokenAddress, from, to, big.NewInt(1))
	first.BlockNumber, first.TxIndex, first.Index = 10, 2, 3
	second := validERC20TransferLog(tokenAddress, from, to, big.NewInt(2))
	second.BlockNumber, second.TxIndex, second.Index = 10, 1, 5
	third := validERC20TransferLog(tokenAddress, from, to, big.NewInt(3))
	third.BlockNumber, third.TxIndex, third.Index = 11, 0, 0

	client := &transferLogRPCStub{call: func(_ context.Context, result any, method string, args ...any) error {
		require.Equal(t, "eth_getLogs", method)
		require.Len(t, args, 1)
		require.Equal(t, map[string]any{
			"address":   tokenAddress.Hex(),
			"topics":    []any{erc20TransferTopic.Hex()},
			"fromBlock": "0xa",
			"toBlock":   "0xb",
		}, args[0])
		logs, ok := result.(*[]types.Log)
		require.True(t, ok)
		*logs = []types.Log{third, first, second}
		return nil
	}}
	fetcher := NewTransferLogFetcher(client, 1030, tokenAddress, TransferLogFetcherOptions{MaxBlockRange: 2})

	events, err := fetcher.Fetch(context.Background(), 10, 11)
	require.NoError(t, err)
	require.Len(t, events, 3)
	require.Equal(t, []uint64{uint64(second.Index), uint64(first.Index), uint64(third.Index)}, []uint64{
		events[0].ID.LogIndex,
		events[1].ID.LogIndex,
		events[2].ID.LogIndex,
	})
}

func TestTransferLogFetcherShrinksOversizedRangesWithBoundedRetries(t *testing.T) {
	var ranges []string
	client := &transferLogRPCStub{call: func(_ context.Context, result any, _ string, args ...any) error {
		filter, ok := args[0].(map[string]any)
		require.True(t, ok)
		fromBlock, ok := filter["fromBlock"].(string)
		require.True(t, ok)
		toBlock, ok := filter["toBlock"].(string)
		require.True(t, ok)
		ranges = append(ranges, fromBlock+"-"+toBlock)
		if fromBlock == "0x1" && (toBlock == "0x5" || toBlock == "0x3") {
			return oversizedRangeError()
		}
		logs, ok := result.(*[]types.Log)
		require.True(t, ok)
		*logs = nil
		return nil
	}}
	fetcher := NewTransferLogFetcher(client, 1030, common.Address{}, TransferLogFetcherOptions{
		MaxBlockRange:  5,
		ShrinkAttempts: 3,
	})

	events, err := fetcher.Fetch(context.Background(), 1, 5)
	require.NoError(t, err)
	require.Empty(t, events)
	require.Equal(t, []string{"0x1-0x5", "0x1-0x3", "0x1-0x2", "0x3-0x4", "0x5-0x5"}, ranges)
}

func TestTransferLogFetcherStopsAfterShrinkLimit(t *testing.T) {
	client := &transferLogRPCStub{call: func(context.Context, any, string, ...any) error {
		return oversizedRangeError()
	}}
	fetcher := NewTransferLogFetcher(client, 1030, common.Address{}, TransferLogFetcherOptions{
		MaxBlockRange:  8,
		ShrinkAttempts: 2,
	})

	_, err := fetcher.Fetch(context.Background(), 1, 8)
	require.ErrorIs(t, err, ErrTransferLogShrinkExhausted)
	require.Equal(t, 3, client.calls)
}

func TestTransferLogFetcherRejectsInvalidBoundsWithoutRPC(t *testing.T) {
	client := &transferLogRPCStub{}
	fetcher := NewTransferLogFetcher(client, 1030, common.Address{}, TransferLogFetcherOptions{MaxBlockRange: 3})

	_, err := fetcher.Fetch(context.Background(), 4, 3)
	require.ErrorIs(t, err, ErrTransferLogRangeInvalid)
	_, err = fetcher.Fetch(context.Background(), 1, 4)
	require.ErrorIs(t, err, ErrTransferLogRangeTooLarge)
	require.Zero(t, client.calls)
}

func TestTransferLogFetcherRejectsMalformedOrOutOfRangeLogs(t *testing.T) {
	tokenAddress := common.HexToAddress("0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff")
	tests := []struct {
		name    string
		log     types.Log
		wantErr error
	}{
		{name: "wrong token", log: func() types.Log {
			log := validERC20TransferLog(common.HexToAddress("0x3333333333333333333333333333333333333333"), common.Address{}, common.Address{}, big.NewInt(1))
			log.BlockNumber = 10
			return log
		}(), wantErr: ErrTransferTokenMismatch},
		{name: "outside range", log: func() types.Log {
			log := validERC20TransferLog(tokenAddress, common.Address{}, common.Address{}, big.NewInt(1))
			log.BlockNumber = 9
			return log
		}(), wantErr: ErrTransferLogOutsideRange},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &transferLogRPCStub{call: func(_ context.Context, result any, _ string, _ ...any) error {
				logs, ok := result.(*[]types.Log)
				require.True(t, ok)
				*logs = []types.Log{test.log}
				return nil
			}}
			fetcher := NewTransferLogFetcher(client, 1030, tokenAddress, TransferLogFetcherOptions{MaxBlockRange: 1})

			_, err := fetcher.Fetch(context.Background(), 10, 10)
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}

func TestTransferLogFetcherDoesNotRetryOtherErrors(t *testing.T) {
	wantErr := errors.New("rpc unavailable")
	client := &transferLogRPCStub{call: func(context.Context, any, string, ...any) error { return wantErr }}
	fetcher := NewTransferLogFetcher(client, 1030, common.Address{}, TransferLogFetcherOptions{MaxBlockRange: 5})

	_, err := fetcher.Fetch(context.Background(), 1, 5)
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, 1, client.calls)
}

func TestTransferLogFetcherHonorsPreCanceledContext(t *testing.T) {
	client := &transferLogRPCStub{}
	fetcher := NewTransferLogFetcher(client, 1030, common.Address{}, TransferLogFetcherOptions{MaxBlockRange: 1})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fetcher.Fetch(ctx, 1, 1)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, client.calls)
}

func TestTransferLogFetcherReadsLatestBlock(t *testing.T) {
	client := &transferLogRPCStub{call: func(_ context.Context, result any, method string, args ...any) error {
		require.Equal(t, "eth_blockNumber", method)
		require.Empty(t, args)
		blockNumber, ok := result.(*hexutil.Uint64)
		require.True(t, ok)
		*blockNumber = hexutil.Uint64(12345)
		return nil
	}}
	fetcher := NewTransferLogFetcher(client, 1030, common.Address{}, TransferLogFetcherOptions{})

	blockNumber, err := fetcher.LatestBlock(context.Background())
	require.NoError(t, err)
	require.Equal(t, uint64(12345), blockNumber)
}

type transferLogRPCStub struct {
	calls int
	call  func(context.Context, any, string, ...any) error
}

func (s *transferLogRPCStub) CallContext(ctx context.Context, result any, method string, args ...any) error {
	s.calls++
	if s.call == nil {
		return fmt.Errorf("unexpected rpc call")
	}
	return s.call(ctx, result, method, args...)
}

func oversizedRangeError() error {
	return &ConfluxRPCPoolError{
		Method:        "eth_getLogs",
		EndpointIDs:   []string{"endpoint_1"},
		RangeTooLarge: true,
	}
}
