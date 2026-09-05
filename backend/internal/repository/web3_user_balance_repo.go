package repository

import (
	"context"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/web3userbalance"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
)

type Web3UserBalanceRepository struct {
	client *dbent.Client
}

func NewWeb3UserBalanceRepository(client *dbent.Client) *Web3UserBalanceRepository {
	return &Web3UserBalanceRepository{client: client}
}

func (r *Web3UserBalanceRepository) CreateOrGet(ctx context.Context, userID int64, assetKey string) (web3deposit.UserBalance, error) {
	entity, err := r.client.Web3UserBalance.Create().
		SetUserID(userID).
		SetAssetKey(assetKey).
		Save(ctx)
	if isWeb3UserBalanceUniqueConstraint(err) {
		return r.GetByUserAndAsset(ctx, userID, assetKey)
	}
	if err != nil {
		return web3deposit.UserBalance{}, fmt.Errorf("create web3 user balance: %w", err)
	}
	return web3UserBalanceFromEnt(entity), nil
}

func isWeb3UserBalanceUniqueConstraint(err error) bool {
	if !dbent.IsConstraintError(err) {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") &&
		(strings.Contains(message, "web3_user_balances_user_asset_uniq") ||
			(strings.Contains(message, "user_id") && strings.Contains(message, "asset_key")))
}

func (r *Web3UserBalanceRepository) GetByUserAndAsset(ctx context.Context, userID int64, assetKey string) (web3deposit.UserBalance, error) {
	entity, err := r.client.Web3UserBalance.Query().
		Where(
			web3userbalance.UserIDEQ(userID),
			web3userbalance.AssetKeyEQ(assetKey),
		).
		Only(ctx)
	if dbent.IsNotFound(err) {
		return web3deposit.UserBalance{}, web3deposit.ErrBalanceNotFound
	}
	if err != nil {
		return web3deposit.UserBalance{}, fmt.Errorf("get web3 user balance by user and asset: %w", err)
	}
	return web3UserBalanceFromEnt(entity), nil
}

func (r *Web3UserBalanceRepository) ListUserBalances(ctx context.Context, userID int64) ([]web3deposit.UserBalance, error) {
	entities, err := r.client.Web3UserBalance.Query().
		Where(web3userbalance.UserIDEQ(userID)).
		Order(dbent.Asc(web3userbalance.FieldAssetKey)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list web3 user balances: %w", err)
	}
	balances := make([]web3deposit.UserBalance, 0, len(entities))
	for _, entity := range entities {
		balances = append(balances, web3UserBalanceFromEnt(entity))
	}
	return balances, nil
}

func web3UserBalanceFromEnt(entity *dbent.Web3UserBalance) web3deposit.UserBalance {
	return web3deposit.UserBalance{
		ID:               entity.ID,
		UserID:           entity.UserID,
		AssetKey:         entity.AssetKey,
		AvailableAmount:  entity.AvailableAmount,
		TotalDeposited:   entity.TotalDeposited,
		TotalTransferred: entity.TotalTransferred,
		BalanceVersion:   entity.BalanceVersion,
		CreatedAt:        entity.CreatedAt,
		UpdatedAt:        entity.UpdatedAt,
	}
}
