package dto

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/web3deposit"
)

type AdminWeb3Deposit struct {
	ID             int64                     `json:"id"`
	UserID         int64                     `json:"user_id"`
	ChainID        uint64                    `json:"chain_id"`
	TokenContract  string                    `json:"token_contract"`
	TxHash         string                    `json:"tx_hash"`
	LogIndex       uint64                    `json:"log_index"`
	BlockNumber    uint64                    `json:"block_number"`
	FromAddress    string                    `json:"from_address"`
	ToAddress      string                    `json:"to_address"`
	TokenAmount    string                    `json:"token_amount"`
	CreditedAmount *string                   `json:"credited_amount,omitempty"`
	Status         web3deposit.DepositStatus `json:"status"`
	ReviewReason   *string                   `json:"review_reason,omitempty"`
	FailureReason  *string                   `json:"failure_reason,omitempty"`
	DetectedAt     time.Time                 `json:"detected_at"`
	FinalizedAt    *time.Time                `json:"finalized_at,omitempty"`
	CreditedAt     *time.Time                `json:"credited_at,omitempty"`
}

func Web3DepositFromDomainAdmin(deposit web3deposit.Deposit) AdminWeb3Deposit {
	return AdminWeb3Deposit{
		ID:             deposit.ID,
		UserID:         deposit.UserID,
		ChainID:        deposit.ChainID,
		TokenContract:  deposit.TokenContract,
		TxHash:         deposit.TxHash,
		LogIndex:       deposit.LogIndex,
		BlockNumber:    deposit.BlockNumber,
		FromAddress:    deposit.FromAddress,
		ToAddress:      deposit.ToAddress,
		TokenAmount:    deposit.TokenAmount,
		CreditedAmount: deposit.CreditedAmount,
		Status:         deposit.Status,
		ReviewReason:   deposit.ReviewReason,
		FailureReason:  deposit.FailureReason,
		DetectedAt:     deposit.DetectedAt,
		FinalizedAt:    deposit.FinalizedAt,
		CreditedAt:     deposit.CreditedAt,
	}
}
