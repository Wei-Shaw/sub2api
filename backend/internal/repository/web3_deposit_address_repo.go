package repository

import (
	"context"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/web3depositaddress"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
)

type Web3DepositAddressRepository struct {
	client *dbent.Client
}

var _ web3deposit.DepositAddressStore = (*Web3DepositAddressRepository)(nil)

func NewWeb3DepositAddressRepository(client *dbent.Client) *Web3DepositAddressRepository {
	return &Web3DepositAddressRepository{client: client}
}

func (r *Web3DepositAddressRepository) Create(ctx context.Context, address web3deposit.DepositAddress) (web3deposit.DepositAddress, error) {
	create := r.client.Web3DepositAddress.Create().
		SetUserID(address.UserID).
		SetWalletID(address.WalletID).
		SetDerivationIndex(address.DerivationIndex).
		SetAddress(address.Address).
		SetNormalizedAddress(address.NormalizedAddress)
	if address.Status != "" {
		create.SetStatus(string(address.Status))
	}
	if !address.AllocatedAt.IsZero() {
		create.SetAllocatedAt(address.AllocatedAt)
	}
	if address.DisabledAt != nil {
		create.SetDisabledAt(*address.DisabledAt)
	}
	if address.LastDepositAt != nil {
		create.SetLastDepositAt(*address.LastDepositAt)
	}

	entity, err := create.Save(ctx)
	if isWeb3DepositAddressUniqueConstraint(err) {
		return web3deposit.DepositAddress{}, web3deposit.ErrAddressAlreadyExists
	}
	if err != nil {
		return web3deposit.DepositAddress{}, fmt.Errorf("create web3 deposit address: %w", err)
	}
	return web3DepositAddressFromEnt(entity), nil
}

func isWeb3DepositAddressUniqueConstraint(err error) bool {
	if !dbent.IsConstraintError(err) {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") && strings.Contains(message, "web3_deposit_addresses")
}

func (r *Web3DepositAddressRepository) GetByUserAndWallet(ctx context.Context, userID int64, walletID string) (web3deposit.DepositAddress, error) {
	entity, err := r.client.Web3DepositAddress.Query().
		Where(
			web3depositaddress.UserIDEQ(userID),
			web3depositaddress.WalletIDEQ(walletID),
		).
		Only(ctx)
	return web3DepositAddressResult(entity, err, "get web3 deposit address by user and wallet")
}

func (r *Web3DepositAddressRepository) GetByNormalizedAddress(ctx context.Context, normalizedAddress string) (web3deposit.DepositAddress, error) {
	entity, err := r.client.Web3DepositAddress.Query().
		Where(web3depositaddress.NormalizedAddressEQ(normalizedAddress)).
		Only(ctx)
	return web3DepositAddressResult(entity, err, "get web3 deposit address by normalized address")
}

func (r *Web3DepositAddressRepository) ListByUser(ctx context.Context, userID int64) ([]web3deposit.DepositAddress, error) {
	entities, err := r.client.Web3DepositAddress.Query().
		Where(web3depositaddress.UserIDEQ(userID)).
		Order(
			dbent.Desc(web3depositaddress.FieldCreatedAt),
			dbent.Desc(web3depositaddress.FieldID),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list web3 deposit addresses by user: %w", err)
	}

	addresses := make([]web3deposit.DepositAddress, 0, len(entities))
	for _, entity := range entities {
		addresses = append(addresses, web3DepositAddressFromEnt(entity))
	}
	return addresses, nil
}

func web3DepositAddressResult(entity *dbent.Web3DepositAddress, err error, operation string) (web3deposit.DepositAddress, error) {
	if dbent.IsNotFound(err) {
		return web3deposit.DepositAddress{}, web3deposit.ErrAddressNotFound
	}
	if err != nil {
		return web3deposit.DepositAddress{}, fmt.Errorf("%s: %w", operation, err)
	}
	return web3DepositAddressFromEnt(entity), nil
}

func web3DepositAddressFromEnt(entity *dbent.Web3DepositAddress) web3deposit.DepositAddress {
	return web3deposit.DepositAddress{
		ID:                entity.ID,
		UserID:            entity.UserID,
		WalletID:          entity.WalletID,
		DerivationIndex:   entity.DerivationIndex,
		Address:           entity.Address,
		NormalizedAddress: entity.NormalizedAddress,
		Status:            web3deposit.AddressStatus(entity.Status),
		AllocatedAt:       entity.AllocatedAt,
		DisabledAt:        entity.DisabledAt,
		LastDepositAt:     entity.LastDepositAt,
		CreatedAt:         entity.CreatedAt,
		UpdatedAt:         entity.UpdatedAt,
	}
}
