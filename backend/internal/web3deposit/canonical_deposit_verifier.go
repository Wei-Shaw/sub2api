package web3deposit

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

var (
	ErrCanonicalDepositSourceInvalid = errors.New("canonical deposit source is invalid")
	ErrCanonicalDataUnavailable      = errors.New("canonical deposit data is temporarily unavailable")
)

type CanonicalMismatchReason string

const (
	CanonicalMismatchBlockMissing        CanonicalMismatchReason = "canonical_block_missing"
	CanonicalMismatchBlockHash           CanonicalMismatchReason = "canonical_block_hash_mismatch"
	CanonicalMismatchReceiptMissing      CanonicalMismatchReason = "transaction_receipt_missing"
	CanonicalMismatchReceiptFailed       CanonicalMismatchReason = "transaction_receipt_failed"
	CanonicalMismatchReceiptBlockHash    CanonicalMismatchReason = "transaction_receipt_block_hash_mismatch"
	CanonicalMismatchTransferLogMissing  CanonicalMismatchReason = "transfer_log_missing"
	CanonicalMismatchTransferToken       CanonicalMismatchReason = "transfer_token_mismatch"
	CanonicalMismatchTransferDestination CanonicalMismatchReason = "transfer_destination_mismatch"
	CanonicalMismatchTransferAmount      CanonicalMismatchReason = "transfer_amount_mismatch"
)

type CanonicalDepositSource interface {
	FinalizedBlockNumber(ctx context.Context) (uint64, error)
	CanonicalBlockHash(ctx context.Context, blockNumber uint64) (common.Hash, bool, error)
	TransactionReceipt(ctx context.Context, txHash common.Hash) (CanonicalReceipt, bool, error)
}

type CanonicalReceipt struct {
	Status    uint64
	BlockHash common.Hash
	Logs      []types.Log
}

type CanonicalDepositVerification struct {
	Valid  bool
	Reason CanonicalMismatchReason
}

type CanonicalDepositVerifier struct {
	source CanonicalDepositSource
}

func NewCanonicalDepositVerifier(source CanonicalDepositSource) (*CanonicalDepositVerifier, error) {
	if source == nil {
		return nil, ErrCanonicalDepositSourceInvalid
	}
	return &CanonicalDepositVerifier{source: source}, nil
}

func (v *CanonicalDepositVerifier) Verify(ctx context.Context, deposit Deposit) (CanonicalDepositVerification, error) {
	canonicalHash, found, err := v.source.CanonicalBlockHash(ctx, deposit.BlockNumber)
	if err != nil {
		return CanonicalDepositVerification{}, fmt.Errorf("read canonical deposit block: %w", err)
	}
	if !found {
		return CanonicalDepositVerification{}, fmt.Errorf("%w: block %d is missing", ErrCanonicalDataUnavailable, deposit.BlockNumber)
	}
	storedBlockHash := common.HexToHash(deposit.BlockHash)
	if canonicalHash != storedBlockHash {
		return canonicalMismatch(CanonicalMismatchBlockHash), nil
	}

	receipt, found, err := v.source.TransactionReceipt(ctx, common.HexToHash(deposit.TxHash))
	if err != nil {
		return CanonicalDepositVerification{}, fmt.Errorf("read canonical deposit receipt: %w", err)
	}
	if !found {
		return CanonicalDepositVerification{}, fmt.Errorf("%w: receipt %s is missing", ErrCanonicalDataUnavailable, deposit.TxHash)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return canonicalMismatch(CanonicalMismatchReceiptFailed), nil
	}
	if receipt.BlockHash != storedBlockHash {
		return canonicalMismatch(CanonicalMismatchReceiptBlockHash), nil
	}

	log, found := canonicalTransferLog(receipt.Logs, deposit.LogIndex)
	if !found {
		return canonicalMismatch(CanonicalMismatchTransferLogMissing), nil
	}
	if !strings.EqualFold(log.Address.Hex(), deposit.TokenContract) {
		return canonicalMismatch(CanonicalMismatchTransferToken), nil
	}
	event, err := ParseERC20TransferLog(deposit.ChainID, log.Address, log)
	if err != nil {
		return canonicalMismatch(CanonicalMismatchTransferLogMissing), nil
	}
	if !strings.EqualFold(event.To.Hex(), deposit.ToAddress) {
		return canonicalMismatch(CanonicalMismatchTransferDestination), nil
	}
	expectedAmount, ok := new(big.Int).SetString(deposit.RawAmount, 10)
	if !ok || event.RawAmount().Cmp(expectedAmount) != 0 {
		return canonicalMismatch(CanonicalMismatchTransferAmount), nil
	}
	return CanonicalDepositVerification{Valid: true}, nil
}

func canonicalTransferLog(logs []types.Log, logIndex uint64) (types.Log, bool) {
	for _, log := range logs {
		if uint64(log.Index) == logIndex {
			return log, true
		}
	}
	return types.Log{}, false
}

func canonicalMismatch(reason CanonicalMismatchReason) CanonicalDepositVerification {
	return CanonicalDepositVerification{Reason: reason}
}

type RPCCanonicalDepositSource struct {
	caller interface {
		CallContext(context.Context, any, string, ...any) error
	}
}

func NewRPCCanonicalDepositSource(caller interface {
	CallContext(context.Context, any, string, ...any) error
}) (*RPCCanonicalDepositSource, error) {
	if caller == nil {
		return nil, ErrCanonicalDepositSourceInvalid
	}
	return &RPCCanonicalDepositSource{caller: caller}, nil
}

func (s *RPCCanonicalDepositSource) FinalizedBlockNumber(ctx context.Context) (uint64, error) {
	var block *canonicalRPCBlock
	if err := s.caller.CallContext(ctx, &block, "eth_getBlockByNumber", "finalized", false); err != nil {
		return 0, fmt.Errorf("get finalized block: %w", err)
	}
	if block == nil || block.Number == nil {
		return 0, fmt.Errorf("get finalized block: block is missing")
	}
	return uint64(*block.Number), nil
}

func (s *RPCCanonicalDepositSource) CanonicalBlockHash(ctx context.Context, blockNumber uint64) (common.Hash, bool, error) {
	var block *canonicalRPCBlock
	if err := s.caller.CallContext(ctx, &block, "eth_getBlockByNumber", hexutil.EncodeUint64(blockNumber), false); err != nil {
		return common.Hash{}, false, fmt.Errorf("get canonical block %d: %w", blockNumber, err)
	}
	if block == nil {
		return common.Hash{}, false, nil
	}
	return block.Hash, true, nil
}

func (s *RPCCanonicalDepositSource) TransactionReceipt(ctx context.Context, txHash common.Hash) (CanonicalReceipt, bool, error) {
	var receipt *canonicalRPCReceipt
	if err := s.caller.CallContext(ctx, &receipt, "eth_getTransactionReceipt", txHash.Hex()); err != nil {
		return CanonicalReceipt{}, false, fmt.Errorf("get transaction receipt %s: %w", txHash.Hex(), err)
	}
	if receipt == nil {
		return CanonicalReceipt{}, false, nil
	}
	return CanonicalReceipt{
		Status:    uint64(receipt.Status),
		BlockHash: receipt.BlockHash,
		Logs:      receipt.Logs,
	}, true, nil
}

type canonicalRPCBlock struct {
	Number *hexutil.Uint64 `json:"number"`
	Hash   common.Hash     `json:"hash"`
}

type canonicalRPCReceipt struct {
	Status    hexutil.Uint64 `json:"status"`
	BlockHash common.Hash    `json:"blockHash"`
	Logs      []types.Log    `json:"logs"`
}
