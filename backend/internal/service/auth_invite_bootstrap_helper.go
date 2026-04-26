package service

import (
	"context"
	"errors"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"golang.org/x/crypto/bcrypt"
)

func createInviteBootstrapUserWithRedeem(
	ctx context.Context,
	entClient *dbent.Client,
	userRepo UserRepository,
	redeemRepo RedeemCodeRepository,
	cfg *config.Config,
	settingService *SettingService,
	invitationRedeemCode *RedeemCode,
) (*User, error) {
	if invitationRedeemCode == nil {
		return nil, ErrInvitationCodeInvalid
	}
	if redeemRepo == nil {
		return nil, ErrServiceUnavailable
	}

	createUser := func(runCtx context.Context) (*User, error) {
		return createInviteBootstrapUserWithoutRedeem(runCtx, userRepo, cfg, settingService)
	}

	if entClient != nil && dbent.TxFromContext(ctx) == nil {
		tx, err := entClient.Tx(ctx)
		if err != nil {
			return nil, ErrServiceUnavailable
		}
		txCtx := dbent.NewTxContext(ctx, tx)
		candidateUser, err := createUser(txCtx)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		if err := redeemRepo.Use(txCtx, invitationRedeemCode.ID, candidateUser.ID); err != nil {
			_ = tx.Rollback()
			return nil, ErrInvitationCodeInvalid
		}
		if err := tx.Commit(); err != nil {
			return nil, ErrServiceUnavailable
		}
		return candidateUser, nil
	}

	candidateUser, err := createUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := redeemRepo.Use(ctx, invitationRedeemCode.ID, candidateUser.ID); err != nil {
		return nil, ErrInvitationCodeInvalid
	}
	return candidateUser, nil
}

func createInviteBootstrapUserWithoutRedeem(
	ctx context.Context,
	userRepo UserRepository,
	cfg *config.Config,
	settingService *SettingService,
) (*User, error) {
	defaultBalance := 0.0
	defaultConcurrency := 0
	if cfg != nil {
		defaultBalance = cfg.Default.UserBalance
		defaultConcurrency = cfg.Default.UserConcurrency
	}
	if settingService != nil {
		defaultBalance = settingService.GetDefaultBalance(ctx)
		defaultConcurrency = settingService.GetDefaultConcurrency(ctx)
	}

	for attempt := 0; attempt < 3; attempt++ {
		randomSuffix, err := randomHexString(16)
		if err != nil {
			return nil, ErrServiceUnavailable
		}
		randomPassword, err := randomHexString(32)
		if err != nil {
			return nil, ErrServiceUnavailable
		}
		hashedPasswordBytes, err := bcrypt.GenerateFromPassword([]byte(randomPassword), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}
		hashedPassword := string(hashedPasswordBytes)

		candidateUser := &User{
			Email:        fmt.Sprintf("invite-%s@invite-login.invalid", randomSuffix),
			Username:     "invite-" + randomSuffix[:12],
			PasswordHash: hashedPassword,
			Role:         RoleUser,
			Balance:      defaultBalance,
			Concurrency:  defaultConcurrency,
			Status:       StatusActive,
		}
		if err := userRepo.Create(ctx, candidateUser); err != nil {
			if errors.Is(err, ErrEmailExists) {
				continue
			}
			return nil, ErrServiceUnavailable
		}
		return candidateUser, nil
	}

	return nil, ErrServiceUnavailable
}
