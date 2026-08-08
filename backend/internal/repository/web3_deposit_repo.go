package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/ent/web3deposit"
	"github.com/Wei-Shaw/sub2api/ent/web3depositaddress"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	depositdomain "github.com/Wei-Shaw/sub2api/internal/web3deposit"
)

type Web3DepositRepository struct {
	client *dbent.Client
}

var _ depositdomain.DetectedDepositStore = (*Web3DepositRepository)(nil)
var _ depositdomain.DepositCreditEligibilitySource = (*Web3DepositRepository)(nil)
var _ depositdomain.PendingFinalizationSource = (*Web3DepositRepository)(nil)
var _ depositdomain.UserDepositReader = (*Web3DepositRepository)(nil)
var _ depositdomain.AdminDepositReader = (*Web3DepositRepository)(nil)

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

func (r *Web3DepositRepository) UpsertDetected(ctx context.Context, deposit depositdomain.Deposit) (depositdomain.Deposit, error) {
	created, err := r.Create(ctx, deposit)
	if err == nil {
		return created, nil
	}
	if !errors.Is(err, depositdomain.ErrDepositAlreadyExists) {
		return depositdomain.Deposit{}, fmt.Errorf("upsert detected web3 deposit: %w", err)
	}

	existing, err := r.GetByEvent(ctx, deposit.ChainID, deposit.TxHash, deposit.LogIndex)
	if err != nil {
		return depositdomain.Deposit{}, fmt.Errorf("get existing detected web3 deposit: %w", err)
	}
	return existing, nil
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

func (r *Web3DepositRepository) ListUserDeposits(ctx context.Context, userID int64, page, pageSize int) ([]depositdomain.Deposit, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	query := r.client.Web3Deposit.Query().Where(web3deposit.UserIDEQ(userID))
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count web3 deposits by user: %w", err)
	}
	entities, err := query.
		Order(
			dbent.Desc(web3deposit.FieldCreatedAt),
			dbent.Desc(web3deposit.FieldID),
		).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list paginated web3 deposits by user: %w", err)
	}

	deposits := make([]depositdomain.Deposit, 0, len(entities))
	for _, entity := range entities {
		deposits = append(deposits, web3DepositFromEnt(entity))
	}
	return deposits, int64(total), nil
}

func (r *Web3DepositRepository) GetUserDeposit(ctx context.Context, userID, depositID int64) (depositdomain.Deposit, error) {
	entity, err := r.client.Web3Deposit.Query().
		Where(
			web3deposit.IDEQ(depositID),
			web3deposit.UserIDEQ(userID),
		).
		Only(ctx)
	return web3DepositResult(entity, err, "get web3 deposit by user")
}

func (r *Web3DepositRepository) ListAdminDeposits(ctx context.Context, filter depositdomain.AdminDepositFilter) ([]depositdomain.Deposit, int64, error) {
	predicates := make([]predicate.Web3Deposit, 0, 6)
	if filter.Status != "" {
		predicates = append(predicates, web3deposit.StatusEQ(string(filter.Status)))
	}
	if filter.UserID > 0 {
		predicates = append(predicates, web3deposit.UserIDEQ(filter.UserID))
	}
	if value := strings.ToLower(strings.TrimSpace(filter.Address)); value != "" {
		predicates = append(predicates, web3deposit.ToAddressEQ(value))
	}
	if value := strings.ToLower(strings.TrimSpace(filter.TxHash)); value != "" {
		predicates = append(predicates, web3deposit.TxHashEQ(value))
	}
	if filter.CreatedAtFrom != nil {
		predicates = append(predicates, web3deposit.CreatedAtGTE(*filter.CreatedAtFrom))
	}
	if filter.CreatedAtTo != nil {
		predicates = append(predicates, web3deposit.CreatedAtLTE(*filter.CreatedAtTo))
	}
	page, pageSize := filter.Page, filter.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	query := r.client.Web3Deposit.Query().Where(predicates...)
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count admin web3 deposits: %w", err)
	}
	entities, err := query.Order(dbent.Desc(web3deposit.FieldCreatedAt), dbent.Desc(web3deposit.FieldID)).Offset((page - 1) * pageSize).Limit(pageSize).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list admin web3 deposits: %w", err)
	}
	items := make([]depositdomain.Deposit, 0, len(entities))
	for _, entity := range entities {
		items = append(items, web3DepositFromEnt(entity))
	}
	return items, int64(total), nil
}

func (r *Web3DepositRepository) GetAdminDeposit(ctx context.Context, depositID int64) (depositdomain.Deposit, error) {
	entity, err := r.client.Web3Deposit.Get(ctx, depositID)
	return web3DepositResult(entity, err, "get admin web3 deposit")
}

func (r *Web3DepositRepository) CountAdminDepositsByStatus(ctx context.Context) (map[depositdomain.DepositStatus]int64, error) {
	entities, err := r.client.Web3Deposit.Query().Select(web3deposit.FieldStatus).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("count admin web3 deposits by status: %w", err)
	}
	counts := make(map[depositdomain.DepositStatus]int64)
	for _, entity := range entities {
		counts[depositdomain.DepositStatus(entity.Status)]++
	}
	return counts, nil
}

func (r *Web3DepositRepository) ListPendingFinalization(ctx context.Context, fromBlock, toBlock uint64) ([]depositdomain.Deposit, error) {
	storedFromBlock, err := web3DepositUint64ToInt64(fromBlock, "finalizer from block")
	if err != nil {
		return nil, err
	}
	storedToBlock, err := web3DepositUint64ToInt64(toBlock, "finalizer to block")
	if err != nil {
		return nil, err
	}
	entities, err := r.client.Web3Deposit.Query().
		Where(
			web3deposit.BlockNumberGTE(storedFromBlock),
			web3deposit.BlockNumberLTE(storedToBlock),
			web3deposit.StatusIn(
				string(depositdomain.DepositStatusDetected),
				string(depositdomain.DepositStatusConfirming),
			),
		).
		Order(
			dbent.Asc(web3deposit.FieldBlockNumber),
			dbent.Asc(web3deposit.FieldID),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pending web3 deposits for finalization: %w", err)
	}
	deposits := make([]depositdomain.Deposit, 0, len(entities))
	for _, entity := range entities {
		deposits = append(deposits, web3DepositFromEnt(entity))
	}
	return deposits, nil
}

func (r *Web3DepositRepository) CheckCreditEligibility(ctx context.Context, deposit depositdomain.Deposit) (depositdomain.DepositCreditEligibility, error) {
	userEntity, err := r.client.User.Query().
		Where(user.IDEQ(deposit.UserID)).
		Only(mixins.SkipSoftDelete(ctx))
	if dbent.IsNotFound(err) {
		return ineligibleDeposit(depositdomain.ReviewReasonUserMissing), nil
	}
	if err != nil {
		return depositdomain.DepositCreditEligibility{}, fmt.Errorf("get web3 deposit user eligibility: %w", err)
	}
	if userEntity.DeletedAt != nil {
		return ineligibleDeposit(depositdomain.ReviewReasonUserDeleted), nil
	}
	if userEntity.Status != domain.StatusActive {
		return ineligibleDeposit(depositdomain.ReviewReasonUserInactive), nil
	}

	addressEntity, err := r.client.Web3DepositAddress.Query().
		Where(web3depositaddress.IDEQ(deposit.DepositAddressID)).
		Only(ctx)
	if dbent.IsNotFound(err) {
		return ineligibleDeposit(depositdomain.ReviewReasonAddressMissing), nil
	}
	if err != nil {
		return depositdomain.DepositCreditEligibility{}, fmt.Errorf("get web3 deposit address eligibility: %w", err)
	}
	if addressEntity.Status != string(depositdomain.AddressStatusActive) {
		return ineligibleDeposit(depositdomain.ReviewReasonAddressDisabled), nil
	}
	if addressEntity.UserID != deposit.UserID {
		return ineligibleDeposit(depositdomain.ReviewReasonAddressUserMismatch), nil
	}
	if !strings.EqualFold(addressEntity.NormalizedAddress, deposit.ToAddress) {
		return ineligibleDeposit(depositdomain.ReviewReasonAddressMismatch), nil
	}
	return depositdomain.DepositCreditEligibility{Eligible: true}, nil
}

func ineligibleDeposit(reason string) depositdomain.DepositCreditEligibility {
	return depositdomain.DepositCreditEligibility{ReviewReason: reason}
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
