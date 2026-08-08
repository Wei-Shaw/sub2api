package web3deposit

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

const (
	DefaultTransferLogMaxBlockRange  uint64 = 500
	DefaultTransferLogShrinkAttempts        = 16
)

var (
	ErrTransferLogRangeInvalid    = errors.New("transfer log block range is invalid")
	ErrTransferLogRangeTooLarge   = errors.New("transfer log block range exceeds configured maximum")
	ErrTransferLogShrinkExhausted = errors.New("transfer log range shrink attempts exhausted")
	ErrTransferLogOutsideRange    = errors.New("transfer log is outside requested block range")
)

type RPCRequestClient interface {
	CallContext(context.Context, any, string, ...any) error
}

type TransferLogFetcherOptions struct {
	MaxBlockRange  uint64
	ShrinkAttempts int
}

type TransferLogFetcher struct {
	client         RPCRequestClient
	chainID        uint64
	tokenAddress   common.Address
	maxBlockRange  uint64
	shrinkAttempts int
}

func NewTransferLogFetcher(
	client RPCRequestClient,
	chainID uint64,
	tokenAddress common.Address,
	options TransferLogFetcherOptions,
) *TransferLogFetcher {
	if options.MaxBlockRange == 0 {
		options.MaxBlockRange = DefaultTransferLogMaxBlockRange
	}
	if options.ShrinkAttempts < 0 {
		options.ShrinkAttempts = 0
	}
	if options.ShrinkAttempts == 0 {
		options.ShrinkAttempts = DefaultTransferLogShrinkAttempts
	}
	return &TransferLogFetcher{
		client:         client,
		chainID:        chainID,
		tokenAddress:   tokenAddress,
		maxBlockRange:  options.MaxBlockRange,
		shrinkAttempts: options.ShrinkAttempts,
	}
}

func (f *TransferLogFetcher) Fetch(ctx context.Context, fromBlock, toBlock uint64) ([]TransferEvent, error) {
	if fromBlock > toBlock {
		return nil, ErrTransferLogRangeInvalid
	}
	if toBlock-fromBlock >= f.maxBlockRange {
		return nil, ErrTransferLogRangeTooLarge
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	events := make([]TransferEvent, 0)
	currentBlock := fromBlock
	batchSize := toBlock - fromBlock + 1
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		remaining := toBlock - currentBlock + 1
		if batchSize > remaining {
			batchSize = remaining
		}

		logs, fetchedTo, reducedBatchSize, err := f.fetchBatch(ctx, currentBlock, batchSize)
		if err != nil {
			return nil, err
		}
		batchSize = reducedBatchSize
		for _, log := range logs {
			if log.BlockNumber < currentBlock || log.BlockNumber > fetchedTo {
				return nil, ErrTransferLogOutsideRange
			}
			event, err := ParseERC20TransferLog(f.chainID, f.tokenAddress, log)
			if err != nil {
				return nil, fmt.Errorf("parse fetched transfer log: %w", err)
			}
			events = append(events, event)
		}

		if fetchedTo == toBlock {
			break
		}
		currentBlock = fetchedTo + 1
	}
	return events, nil
}

func (f *TransferLogFetcher) fetchBatch(
	ctx context.Context,
	fromBlock uint64,
	initialBatchSize uint64,
) ([]types.Log, uint64, uint64, error) {
	batchSize := initialBatchSize
	for attempt := 0; ; attempt++ {
		toBlock := fromBlock + batchSize - 1
		var logs []types.Log
		err := f.client.CallContext(ctx, &logs, "eth_getLogs", transferLogFilter(f.tokenAddress, fromBlock, toBlock))
		if err == nil {
			sort.Slice(logs, func(left, right int) bool {
				if logs[left].BlockNumber != logs[right].BlockNumber {
					return logs[left].BlockNumber < logs[right].BlockNumber
				}
				if logs[left].TxIndex != logs[right].TxIndex {
					return logs[left].TxIndex < logs[right].TxIndex
				}
				return logs[left].Index < logs[right].Index
			})
			return logs, toBlock, batchSize, nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, 0, 0, ctxErr
		}
		if !IsConfluxRPCRangeTooLarge(err) {
			return nil, 0, 0, err
		}
		if batchSize == 1 || attempt >= f.shrinkAttempts {
			return nil, 0, 0, fmt.Errorf("fetch transfer logs from block %d: %w", fromBlock, ErrTransferLogShrinkExhausted)
		}
		batchSize = (batchSize + 1) / 2
	}
}

func transferLogFilter(tokenAddress common.Address, fromBlock, toBlock uint64) map[string]any {
	return map[string]any{
		"address":   tokenAddress.Hex(),
		"topics":    []any{erc20TransferTopic.Hex()},
		"fromBlock": hexutil.EncodeUint64(fromBlock),
		"toBlock":   hexutil.EncodeUint64(toBlock),
	}
}
