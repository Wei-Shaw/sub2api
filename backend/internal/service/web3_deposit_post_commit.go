package service

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
)

type Web3TransferCacheInvalidator struct {
	billing *BillingCacheService
	auth    APIKeyAuthCacheInvalidator
}

func NewWeb3TransferCacheInvalidator(billing *BillingCacheService, auth APIKeyAuthCacheInvalidator) *Web3TransferCacheInvalidator {
	return &Web3TransferCacheInvalidator{billing: billing, auth: auth}
}

func (i *Web3TransferCacheInvalidator) InvalidateTransferCaches(ctx context.Context, userID int64) error {
	if i.auth != nil {
		i.auth.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if i.billing != nil {
		return i.billing.InvalidateUserBalance(ctx, userID)
	}
	return nil
}

type Web3TransferNotifier struct {
	users  UserRepository
	emails *NotificationEmailService
}

func NewWeb3TransferNotifier(users UserRepository, emails *NotificationEmailService) *Web3TransferNotifier {
	return &Web3TransferNotifier{users: users, emails: emails}
}

func (n *Web3TransferNotifier) NotifyTransferCompleted(ctx context.Context, transfer web3deposit.BalanceTransfer) error {
	if n == nil || n.emails == nil {
		return nil
	}
	user, err := n.users.GetByID(ctx, transfer.UserID)
	if err != nil {
		return fmt.Errorf("load web3 transfer notification user: %w", err)
	}
	return n.emails.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventBalanceRechargeSuccess,
		RecipientEmail: user.Email,
		RecipientName:  firstNonEmpty(user.Username, user.Email),
		UserID:         user.ID,
		SourceType:     "web3_balance_transfer",
		SourceID:       strconv.FormatInt(transfer.ID, 10),
		Variables: map[string]string{
			"recharge_amount": transfer.Amount,
			"current_balance": transfer.UserBalanceAfter,
			"order_id":        "WEB3-" + strconv.FormatInt(transfer.ID, 10),
		},
	})
}
