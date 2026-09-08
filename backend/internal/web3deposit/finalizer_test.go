package web3deposit

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestFinalizerVerifiesClassifiesAndAdvancesIndependentCursor(t *testing.T) {
	candidates := []Deposit{
		finalizerTestDeposit(1, "0.999999"),
		finalizerTestDeposit(2, "1.000000"),
		finalizerTestDeposit(3, "10000.000001"),
	}
	canonical := finalizerCanonicalSource(candidates, 150)
	verifier, err := NewCanonicalDepositVerifier(canonical)
	require.NoError(t, err)
	batchStore := &finalizerBatchStoreStub{}
	finalizer, err := NewFinalizer(
		finalizerCursorSourceStub{cursor: ScannerCursor{LastScannedBlock: 140, LastFinalizedBlock: 100}},
		canonical,
		verifier,
		&pendingFinalizationSourceStub{deposits: candidates},
		finalizerEligibilityStub{eligible: true},
		batchStore,
		FinalizerOptions{
			ScannerKey:     "conflux_espace_mainnet:usdt0",
			BlockBatchSize: 20,
			ChainConfig: ChainConfig{
				MinimumDeposit:  decimal.RequireFromString("1"),
				AutoCreditLimit: decimal.RequireFromString("10000"),
			},
		},
	)
	require.NoError(t, err)
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)

	result, err := finalizer.FinalizeNext(context.Background(), "lease-token", now)

	require.NoError(t, err)
	require.Equal(t, uint64(150), result.FinalizedHead)
	require.Equal(t, uint64(100), result.FromBlock)
	require.Equal(t, uint64(119), result.ToBlock)
	require.Equal(t, 3, result.FinalizedCount)
	require.Equal(t, []FinalizedDepositDecision{
		{DepositID: 1, Status: DepositStatusBelowMinimum},
		{DepositID: 2, Status: DepositStatusReadyToCredit},
		{DepositID: 3, Status: DepositStatusManualReview, ReviewReason: ReviewReasonAboveAutoCreditLimit},
	}, batchStore.batch.Decisions)
	require.Equal(t, uint64(119), batchStore.batch.FinalizedThrough)
}

func TestFinalizerRoutesInactiveUserToManualReview(t *testing.T) {
	deposit := finalizerTestDeposit(1, "10")
	canonical := finalizerCanonicalSource([]Deposit{deposit}, 100)
	verifier, err := NewCanonicalDepositVerifier(canonical)
	require.NoError(t, err)
	batchStore := &finalizerBatchStoreStub{}
	finalizer, err := NewFinalizer(
		finalizerCursorSourceStub{cursor: ScannerCursor{LastScannedBlock: 100, LastFinalizedBlock: 100}},
		canonical,
		verifier,
		&pendingFinalizationSourceStub{deposits: []Deposit{deposit}},
		finalizerEligibilityStub{reason: ReviewReasonUserDeleted},
		batchStore,
		FinalizerOptions{ScannerKey: "scanner", BlockBatchSize: 10, ChainConfig: ChainConfig{
			MinimumDeposit: decimal.NewFromInt(1), AutoCreditLimit: decimal.NewFromInt(10000),
		}},
	)
	require.NoError(t, err)

	_, err = finalizer.FinalizeNext(context.Background(), "lease-token", time.Now())

	require.NoError(t, err)
	require.Equal(t, DepositStatusManualReview, batchStore.batch.Decisions[0].Status)
	require.Equal(t, ReviewReasonUserDeleted, batchStore.batch.Decisions[0].ReviewReason)
}

func TestFinalizerRoutesDisabledHistoricalAddressToManualReview(t *testing.T) {
	deposit := finalizerTestDeposit(1, "10")
	canonical := finalizerCanonicalSource([]Deposit{deposit}, 100)
	verifier, err := NewCanonicalDepositVerifier(canonical)
	require.NoError(t, err)
	batchStore := &finalizerBatchStoreStub{}
	finalizer, err := NewFinalizer(
		finalizerCursorSourceStub{cursor: ScannerCursor{LastScannedBlock: 100, LastFinalizedBlock: 100}},
		canonical,
		verifier,
		&pendingFinalizationSourceStub{deposits: []Deposit{deposit}},
		finalizerEligibilityStub{reason: ReviewReasonAddressDisabled},
		batchStore,
		FinalizerOptions{ScannerKey: "scanner", BlockBatchSize: 10, ChainConfig: ChainConfig{
			MinimumDeposit: decimal.NewFromInt(1), AutoCreditLimit: decimal.NewFromInt(10000),
		}},
	)
	require.NoError(t, err)

	_, err = finalizer.FinalizeNext(context.Background(), "lease-token", time.Now())

	require.NoError(t, err)
	require.Equal(t, DepositStatusManualReview, batchStore.batch.Decisions[0].Status)
	require.Equal(t, ReviewReasonAddressDisabled, batchStore.batch.Decisions[0].ReviewReason)
}

func TestFinalizerMarksCanonicalMismatchOrphaned(t *testing.T) {
	deposit := finalizerTestDeposit(1, "10")
	canonical := finalizerCanonicalSource([]Deposit{deposit}, 100)
	canonical.blocks[deposit.BlockNumber] = common.HexToHash("0x01")
	verifier, err := NewCanonicalDepositVerifier(canonical)
	require.NoError(t, err)
	batchStore := &finalizerBatchStoreStub{}
	finalizer, err := NewFinalizer(
		finalizerCursorSourceStub{cursor: ScannerCursor{LastScannedBlock: 100, LastFinalizedBlock: 100}},
		canonical,
		verifier,
		&pendingFinalizationSourceStub{deposits: []Deposit{deposit}},
		finalizerEligibilityStub{eligible: true},
		batchStore,
		FinalizerOptions{ScannerKey: "scanner", BlockBatchSize: 10, ChainConfig: ChainConfig{
			MinimumDeposit: decimal.NewFromInt(1), AutoCreditLimit: decimal.NewFromInt(10000),
		}},
	)
	require.NoError(t, err)

	result, err := finalizer.FinalizeNext(context.Background(), "lease-token", time.Now())

	require.NoError(t, err)
	require.Equal(t, 1, result.OrphanedCount)
	require.Equal(t, DepositStatusOrphaned, batchStore.batch.Decisions[0].Status)
	require.Equal(t, string(CanonicalMismatchBlockHash), batchStore.batch.Decisions[0].FailureReason)
}

func TestFinalizerScopesCandidatesAndIncludesHistoricalPendingDeposits(t *testing.T) {
	deposit := finalizerTestDeposit(1, "10")
	deposit.BlockNumber = 90
	canonical := finalizerCanonicalSource([]Deposit{deposit}, 150)
	verifier, err := NewCanonicalDepositVerifier(canonical)
	require.NoError(t, err)
	pending := &pendingFinalizationSourceStub{deposits: []Deposit{deposit}}
	batchStore := &finalizerBatchStoreStub{}
	finalizer, err := NewFinalizer(
		finalizerCursorSourceStub{cursor: ScannerCursor{LastScannedBlock: 140, LastFinalizedBlock: 100}},
		canonical,
		verifier,
		pending,
		finalizerEligibilityStub{eligible: true},
		batchStore,
		FinalizerOptions{ScannerKey: "scanner", BlockBatchSize: 20, ChainConfig: ChainConfig{
			ChainID: 1030, TokenAddress: common.HexToAddress("0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff"),
			MinimumDeposit: decimal.NewFromInt(1), AutoCreditLimit: decimal.NewFromInt(10000),
		}},
	)
	require.NoError(t, err)

	result, err := finalizer.FinalizeNext(context.Background(), "lease-token", time.Now())

	require.NoError(t, err)
	require.Equal(t, uint64(1030), pending.chainID)
	require.Equal(t, "0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff", pending.tokenContract)
	require.Equal(t, uint64(119), pending.toBlock)
	require.Equal(t, 1, result.FinalizedCount)
}

func TestFinalizerFailsSafeWhenAmountExceedsPlatformRange(t *testing.T) {
	deposit := finalizerTestDeposit(1, "1000000000000.000000")
	canonical := finalizerCanonicalSource([]Deposit{deposit}, 150)
	verifier, err := NewCanonicalDepositVerifier(canonical)
	require.NoError(t, err)
	batchStore := &finalizerBatchStoreStub{}
	finalizer, err := NewFinalizer(
		finalizerCursorSourceStub{cursor: ScannerCursor{LastScannedBlock: 140, LastFinalizedBlock: 100}},
		canonical,
		verifier,
		&pendingFinalizationSourceStub{deposits: []Deposit{deposit}},
		finalizerEligibilityStub{eligible: true},
		batchStore,
		FinalizerOptions{ScannerKey: "scanner", BlockBatchSize: 20, ChainConfig: ChainConfig{
			MinimumDeposit: decimal.NewFromInt(1), AutoCreditLimit: decimal.NewFromInt(10000),
		}},
	)
	require.NoError(t, err)

	result, err := finalizer.FinalizeNext(context.Background(), "lease-token", time.Now())

	require.NoError(t, err)
	require.Equal(t, 1, result.OverflowedCount)
	require.Equal(t, DepositStatusFailed, batchStore.batch.Decisions[0].Status)
	require.Equal(t, FailureReasonAmountExceedsPlatformBalance, batchStore.batch.Decisions[0].FailureReason)
}

func finalizerTestDeposit(id int64, amount string) Deposit {
	deposit := canonicalVerifierDeposit()
	deposit.ID = id
	deposit.TokenAmount = amount
	deposit.BlockNumber = 100 + uint64(id)
	deposit.BlockHash = common.BigToHash(decimal.RequireFromString(amount).BigInt()).Hex()
	deposit.TxHash = common.BigToHash(bigIntFromInt64(id)).Hex()
	return deposit
}

func bigIntFromInt64(value int64) *big.Int {
	return big.NewInt(value)
}

type finalizerCanonicalSourceStub struct {
	finalized uint64
	blocks    map[uint64]common.Hash
	receipts  map[common.Hash]CanonicalReceipt
}

func finalizerCanonicalSource(deposits []Deposit, finalized uint64) finalizerCanonicalSourceStub {
	source := finalizerCanonicalSourceStub{
		finalized: finalized,
		blocks:    make(map[uint64]common.Hash, len(deposits)),
		receipts:  make(map[common.Hash]CanonicalReceipt, len(deposits)),
	}
	for _, deposit := range deposits {
		source.blocks[deposit.BlockNumber] = common.HexToHash(deposit.BlockHash)
		source.receipts[common.HexToHash(deposit.TxHash)] = CanonicalReceipt{
			Status:    types.ReceiptStatusSuccessful,
			BlockHash: common.HexToHash(deposit.BlockHash),
			Logs:      []types.Log{canonicalVerifierTransferLog(deposit)},
		}
	}
	return source
}

func (s finalizerCanonicalSourceStub) FinalizedBlockNumber(context.Context) (uint64, error) {
	return s.finalized, nil
}

func (s finalizerCanonicalSourceStub) CanonicalBlockHash(_ context.Context, blockNumber uint64) (common.Hash, bool, error) {
	hash, ok := s.blocks[blockNumber]
	return hash, ok, nil
}

func (s finalizerCanonicalSourceStub) TransactionReceipt(_ context.Context, txHash common.Hash) (CanonicalReceipt, bool, error) {
	receipt, ok := s.receipts[txHash]
	return receipt, ok, nil
}

type finalizerCursorSourceStub struct{ cursor ScannerCursor }

func (s finalizerCursorSourceStub) GetByKey(context.Context, string) (ScannerCursor, error) {
	return s.cursor, nil
}

type pendingFinalizationSourceStub struct {
	deposits      []Deposit
	chainID       uint64
	tokenContract string
	toBlock       uint64
}

func (s *pendingFinalizationSourceStub) ListPendingFinalization(_ context.Context, chainID uint64, tokenContract string, toBlock uint64) ([]Deposit, error) {
	s.chainID = chainID
	s.tokenContract = tokenContract
	s.toBlock = toBlock
	return s.deposits, nil
}

type finalizerEligibilityStub struct {
	eligible bool
	reason   string
}

func (s finalizerEligibilityStub) CheckCreditEligibility(context.Context, Deposit) (DepositCreditEligibility, error) {
	return DepositCreditEligibility{Eligible: s.eligible, ReviewReason: s.reason}, nil
}

type finalizerBatchStoreStub struct{ batch FinalizerBatch }

func (s *finalizerBatchStoreStub) CommitFinalizedBatch(_ context.Context, batch FinalizerBatch) (int, error) {
	s.batch = batch
	return len(batch.Decisions), nil
}
