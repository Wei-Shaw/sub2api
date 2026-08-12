package web3deposit

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

func TestCanonicalDepositVerifierAcceptsMatchingReceiptLogByGlobalIndex(t *testing.T) {
	deposit := canonicalVerifierDeposit()
	source := canonicalDepositSourceStub{
		blockHash: common.HexToHash(deposit.BlockHash),
		receipt: CanonicalReceipt{
			Status:    types.ReceiptStatusSuccessful,
			BlockHash: common.HexToHash(deposit.BlockHash),
			Logs: []types.Log{
				{Index: 1},
				canonicalVerifierTransferLog(deposit),
			},
		},
	}
	verifier, err := NewCanonicalDepositVerifier(source)
	require.NoError(t, err)

	result, err := verifier.Verify(context.Background(), deposit)

	require.NoError(t, err)
	require.True(t, result.Valid)
	require.Empty(t, result.Reason)
}

func TestCanonicalDepositVerifierRejectsCanonicalMismatches(t *testing.T) {
	deposit := canonicalVerifierDeposit()
	tests := []struct {
		name       string
		mutate     func(*canonicalDepositSourceStub)
		wantReason CanonicalMismatchReason
	}{
		{name: "block hash", mutate: func(source *canonicalDepositSourceStub) { source.blockHash = common.HexToHash("0x02") }, wantReason: CanonicalMismatchBlockHash},
		{name: "receipt failed", mutate: func(source *canonicalDepositSourceStub) { source.receipt.Status = types.ReceiptStatusFailed }, wantReason: CanonicalMismatchReceiptFailed},
		{name: "receipt block hash", mutate: func(source *canonicalDepositSourceStub) { source.receipt.BlockHash = common.HexToHash("0x03") }, wantReason: CanonicalMismatchReceiptBlockHash},
		{name: "log missing", mutate: func(source *canonicalDepositSourceStub) { source.receipt.Logs = nil }, wantReason: CanonicalMismatchTransferLogMissing},
		{name: "token", mutate: func(source *canonicalDepositSourceStub) {
			source.receipt.Logs[0].Address = common.HexToAddress("0x0000000000000000000000000000000000000001")
		}, wantReason: CanonicalMismatchTransferToken},
		{name: "destination", mutate: func(source *canonicalDepositSourceStub) {
			source.receipt.Logs[0].Topics[2] = common.BytesToHash(common.HexToAddress("0x0000000000000000000000000000000000000002").Bytes())
		}, wantReason: CanonicalMismatchTransferDestination},
		{name: "amount", mutate: func(source *canonicalDepositSourceStub) {
			source.receipt.Logs[0].Data = common.LeftPadBytes(big.NewInt(999).Bytes(), common.HashLength)
		}, wantReason: CanonicalMismatchTransferAmount},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := canonicalDepositSourceStub{
				blockHash: common.HexToHash(deposit.BlockHash),
				receipt: CanonicalReceipt{
					Status:    types.ReceiptStatusSuccessful,
					BlockHash: common.HexToHash(deposit.BlockHash),
					Logs:      []types.Log{canonicalVerifierTransferLog(deposit)},
				},
			}
			test.mutate(&source)
			verifier, err := NewCanonicalDepositVerifier(source)
			require.NoError(t, err)

			result, err := verifier.Verify(context.Background(), deposit)

			require.NoError(t, err)
			require.False(t, result.Valid)
			require.Equal(t, test.wantReason, result.Reason)
		})
	}
}

func TestCanonicalDepositVerifierRetriesMissingCanonicalData(t *testing.T) {
	deposit := canonicalVerifierDeposit()
	tests := []struct {
		name   string
		mutate func(*canonicalDepositSourceStub)
	}{
		{name: "block missing", mutate: func(source *canonicalDepositSourceStub) { source.blockFound = boolPointer(false) }},
		{name: "receipt missing", mutate: func(source *canonicalDepositSourceStub) { source.receiptFound = boolPointer(false) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := canonicalDepositSourceStub{
				blockHash: common.HexToHash(deposit.BlockHash),
				receipt: CanonicalReceipt{
					Status:    types.ReceiptStatusSuccessful,
					BlockHash: common.HexToHash(deposit.BlockHash),
					Logs:      []types.Log{canonicalVerifierTransferLog(deposit)},
				},
			}
			test.mutate(&source)
			verifier, err := NewCanonicalDepositVerifier(source)
			require.NoError(t, err)

			_, err = verifier.Verify(context.Background(), deposit)

			require.ErrorIs(t, err, ErrCanonicalDataUnavailable)
		})
	}
}

func TestCanonicalDepositVerifierReturnsSourceErrors(t *testing.T) {
	wantErr := errors.New("rpc unavailable")
	verifier, err := NewCanonicalDepositVerifier(canonicalDepositSourceStub{blockErr: wantErr})
	require.NoError(t, err)

	_, err = verifier.Verify(context.Background(), canonicalVerifierDeposit())

	require.ErrorIs(t, err, wantErr)
}

func TestRPCCanonicalDepositSourceUsesFinalizedTagAndGlobalReceiptFields(t *testing.T) {
	caller := &canonicalRPCCallerStub{}
	source, err := NewRPCCanonicalDepositSource(caller)
	require.NoError(t, err)

	finalized, err := source.FinalizedBlockNumber(context.Background())
	require.NoError(t, err)
	require.Equal(t, uint64(150), finalized)
	hash, found, err := source.CanonicalBlockHash(context.Background(), 100)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, common.HexToHash("0x64"), hash)
	receipt, found, err := source.TransactionReceipt(context.Background(), common.HexToHash("0xaa"))
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, uint64(types.ReceiptStatusSuccessful), receipt.Status)
	require.Equal(t, uint(17), receipt.Logs[0].Index)
	require.Equal(t, []canonicalRPCCall{
		{method: "eth_getBlockByNumber", args: []any{"finalized", false}},
		{method: "eth_getBlockByNumber", args: []any{"0x64", false}},
		{method: "eth_getTransactionReceipt", args: []any{common.HexToHash("0xaa").Hex()}},
	}, caller.calls)
}

func canonicalVerifierDeposit() Deposit {
	return Deposit{
		ChainID:       1030,
		TokenContract: "0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff",
		TxHash:        "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		LogIndex:      17,
		BlockNumber:   100,
		BlockHash:     "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		FromAddress:   "0x1111111111111111111111111111111111111111",
		ToAddress:     "0x2222222222222222222222222222222222222222",
		RawAmount:     "1234567",
	}
}

func canonicalVerifierTransferLog(deposit Deposit) types.Log {
	rawAmount, _ := new(big.Int).SetString(deposit.RawAmount, 10)
	return types.Log{
		Address: common.HexToAddress(deposit.TokenContract),
		Topics: []common.Hash{
			erc20TransferTopic,
			common.BytesToHash(common.HexToAddress(deposit.FromAddress).Bytes()),
			common.BytesToHash(common.HexToAddress(deposit.ToAddress).Bytes()),
		},
		Data:        common.LeftPadBytes(rawAmount.Bytes(), common.HashLength),
		BlockNumber: deposit.BlockNumber,
		TxHash:      common.HexToHash(deposit.TxHash),
		BlockHash:   common.HexToHash(deposit.BlockHash),
		Index:       uint(deposit.LogIndex),
	}
}

type canonicalDepositSourceStub struct {
	blockHash    common.Hash
	blockFound   *bool
	blockErr     error
	receipt      CanonicalReceipt
	receiptFound *bool
	receiptErr   error
}

func (s canonicalDepositSourceStub) FinalizedBlockNumber(context.Context) (uint64, error) {
	return 0, nil
}

func (s canonicalDepositSourceStub) CanonicalBlockHash(context.Context, uint64) (common.Hash, bool, error) {
	return s.blockHash, s.blockFound == nil || *s.blockFound, s.blockErr
}

func (s canonicalDepositSourceStub) TransactionReceipt(context.Context, common.Hash) (CanonicalReceipt, bool, error) {
	return s.receipt, s.receiptFound == nil || *s.receiptFound, s.receiptErr
}

func boolPointer(value bool) *bool {
	return &value
}

type canonicalRPCCall struct {
	method string
	args   []any
}

type canonicalRPCCallerStub struct {
	calls []canonicalRPCCall
}

func (s *canonicalRPCCallerStub) CallContext(_ context.Context, result any, method string, args ...any) error {
	s.calls = append(s.calls, canonicalRPCCall{method: method, args: args})
	switch method {
	case "eth_getBlockByNumber":
		number := hexutil.Uint64(100)
		hash := common.HexToHash("0x64")
		if args[0] == "finalized" {
			number = hexutil.Uint64(150)
			hash = common.HexToHash("0x96")
		}
		*(result.(**canonicalRPCBlock)) = &canonicalRPCBlock{Number: &number, Hash: hash}
	case "eth_getTransactionReceipt":
		*(result.(**canonicalRPCReceipt)) = &canonicalRPCReceipt{
			Status:    hexutil.Uint64(types.ReceiptStatusSuccessful),
			BlockHash: common.HexToHash("0x64"),
			Logs:      []types.Log{{Index: 17}},
		}
	}
	return nil
}
