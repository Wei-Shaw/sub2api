package repository

import (
	"context"
	"fmt"
	"math"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/web3deposit"
	depositdomain "github.com/Wei-Shaw/sub2api/internal/web3deposit"
)

type Web3DepositRepository struct {
	client *dbent.Client
}

func NewWeb3DepositRepository(client *dbent.Client) *Web3DepositRepository {
	return &Web3DepositRepository{client: client}
}

func (r *Web3DepositRepository) Create(ctx context.Context, deposit depositdomain.Deposit) (depositdomain.Deposit, error) {
	chainID, err := web3DepositUint64ToInt64(deposit.ChainID, "chain ID")
	if err != nil {
		return depositdomain.Deposit{}, err
	}
	logIndex, err := web3DepositUint64ToInt64(deposit.LogIndex, "log index")
	if err != nil {
		return depositdomain.Deposit{}, err
	}
	blockNumber, err := web3DepositUint64ToInt64(deposit.BlockNumber, "block number")
	if err != nil {
		return depositdomain.Deposit{}, err
	}
	if deposit.TokenDecimals < 0 || deposit.TokenDecimals > 255 {
		return depositdomain.Deposit{}, fmt.Errorf("web3 deposit token decimals must be between 0 and 255")
	}

	create := r.client.Web3Deposit.Create().
		SetUserID(deposit.UserID).
		SetDepositAddressID(deposit.DepositAddressID).
		SetChainID(chainID).
		SetTokenContract(deposit.TokenContract).
		SetTxHash(deposit.TxHash).
		SetLogIndex(logIndex).
		SetBlockNumber(blockNumber).
		SetBlockHash(deposit.BlockHash).
		SetFromAddress(deposit.FromAddress).
		SetToAddress(deposit.ToAddress).
		SetRawAmount(deposit.RawAmount).
		SetTokenDecimals(int16(deposit.TokenDecimals)).
		SetTokenAmount(deposit.TokenAmount).
		SetRetryCount(deposit.RetryCount)
	if deposit.CreditedAmount != nil {
		create.SetCreditedAmount(*deposit.CreditedAmount)
	}
	if deposit.Status != "" {
		create.SetStatus(string(deposit.Status))
	}
	if deposit.ReviewReason != nil {
		create.SetReviewReason(*deposit.ReviewReason)
	}
	if deposit.FailureReason != nil {
		create.SetFailureReason(*deposit.FailureReason)
	}
	if deposit.NextRetryAt != nil {
		create.SetNextRetryAt(*deposit.NextRetryAt)
	}
	if !deposit.DetectedAt.IsZero() {
		create.SetDetectedAt(deposit.DetectedAt)
	}
	if deposit.FinalizedAt != nil {
		create.SetFinalizedAt(*deposit.FinalizedAt)
	}
	if deposit.CreditedAt != nil {
		create.SetCreditedAt(*deposit.CreditedAt)
	}

	entity, err := create.Save(ctx)
	if isWeb3DepositEventUniqueConstraint(err) {
		return depositdomain.Deposit{}, depositdomain.ErrDepositAlreadyExists
	}
	if err != nil {
		return depositdomain.Deposit{}, fmt.Errorf("create web3 deposit: %w", err)
	}
	return web3DepositFromEnt(entity), nil
}

func (r *Web3DepositRepository) GetByEvent(ctx context.Context, chainID uint64, txHash string, logIndex uint64) (depositdomain.Deposit, error) {
	storedChainID, err := web3DepositUint64ToInt64(chainID, "chain ID")
	if err != nil {
		return depositdomain.Deposit{}, err
	}
	storedLogIndex, err := web3DepositUint64ToInt64(logIndex, "log index")
	if err != nil {
		return depositdomain.Deposit{}, err
	}

	entity, err := r.client.Web3Deposit.Query().
		Where(
			web3deposit.ChainIDEQ(storedChainID),
			web3deposit.TxHashEQ(txHash),
			web3deposit.LogIndexEQ(storedLogIndex),
		).
		Only(ctx)
	return web3DepositResult(entity, err, "get web3 deposit by event")
}

func (r *Web3DepositRepository) ListByUser(ctx context.Context, userID int64) ([]depositdomain.Deposit, error) {
	entities, err := r.client.Web3Deposit.Query().
		Where(web3deposit.UserIDEQ(userID)).
		Order(
			dbent.Desc(web3deposit.FieldCreatedAt),
			dbent.Desc(web3deposit.FieldID),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list web3 deposits by user: %w", err)
	}

	deposits := make([]depositdomain.Deposit, 0, len(entities))
	for _, entity := range entities {
		deposits = append(deposits, web3DepositFromEnt(entity))
	}
	return deposits, nil
}

func web3DepositResult(entity *dbent.Web3Deposit, err error, operation string) (depositdomain.Deposit, error) {
	if dbent.IsNotFound(err) {
		return depositdomain.Deposit{}, depositdomain.ErrDepositNotFound
	}
	if err != nil {
		return depositdomain.Deposit{}, fmt.Errorf("%s: %w", operation, err)
	}
	return web3DepositFromEnt(entity), nil
}

func web3DepositFromEnt(entity *dbent.Web3Deposit) depositdomain.Deposit {
	return depositdomain.Deposit{
		ID:               entity.ID,
		UserID:           entity.UserID,
		DepositAddressID: entity.DepositAddressID,
		ChainID:          uint64(entity.ChainID),
		TokenContract:    entity.TokenContract,
		TxHash:           entity.TxHash,
		LogIndex:         uint64(entity.LogIndex),
		BlockNumber:      uint64(entity.BlockNumber),
		BlockHash:        entity.BlockHash,
		FromAddress:      entity.FromAddress,
		ToAddress:        entity.ToAddress,
		RawAmount:        entity.RawAmount,
		TokenDecimals:    int32(entity.TokenDecimals),
		TokenAmount:      entity.TokenAmount,
		CreditedAmount:   entity.CreditedAmount,
		Status:           depositdomain.DepositStatus(entity.Status),
		ReviewReason:     entity.ReviewReason,
		FailureReason:    entity.FailureReason,
		RetryCount:       entity.RetryCount,
		NextRetryAt:      entity.NextRetryAt,
		DetectedAt:       entity.DetectedAt,
		FinalizedAt:      entity.FinalizedAt,
		CreditedAt:       entity.CreditedAt,
		CreatedAt:        entity.CreatedAt,
		UpdatedAt:        entity.UpdatedAt,
	}
}

func web3DepositUint64ToInt64(value uint64, fieldName string) (int64, error) {
	if value > math.MaxInt64 {
		return 0, fmt.Errorf("web3 deposit %s exceeds PostgreSQL BIGINT", fieldName)
	}
	return int64(value), nil
}

func isWeb3DepositEventUniqueConstraint(err error) bool {
	if !dbent.IsConstraintError(err) {
		return false
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "web3_deposits_event_uniq") ||
		strings.Contains(message, "web3deposit_chain_id_tx_hash_log_index") {
		return true
	}
	return strings.Contains(message, "unique") &&
		strings.Contains(message, "chain_id") &&
		strings.Contains(message, "tx_hash") &&
		strings.Contains(message, "log_index")
}
