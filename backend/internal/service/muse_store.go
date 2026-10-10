package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/muse"
)

type MuseSettlement struct {
	Applied bool
	Result  *UsageBillingApplyResult
	Command UsageBillingCommand
	Log     UsageLog
}

type MuseProviderStore interface {
	SaveProfile(context.Context, *Account, *muse.Observation) error
	Profile(context.Context, *Account) (*muse.Observation, error)
	FreezeCharge(context.Context, string, *UsageBillingCommand, *UsageLog) error
	Settle(context.Context, string) (*MuseSettlement, error)
	PendingRecovery(context.Context, int) ([]int64, error)
	ResolveOwnerReview(context.Context, string, muse.Actor, muse.State) error
	EnableVerified(context.Context, *Account) error
	WithSession(context.Context, *Account, func(map[string]any, func(map[string]any) error) error) error
	PendingBalance(context.Context, int64) (float64, error)
	AvailableBalance(context.Context, int64) (float64, error)
	RenewSession(context.Context, *Account, func(context.Context) (map[string]any, error)) error
	DueRenewal(context.Context, int) ([]int64, error)
	DeferRenewal(context.Context, int64) error
	DeferSettlement(context.Context, string) error
	PendingSettlement(context.Context, int) ([]string, error)
	PendingTurns(context.Context, int64) ([]*muse.Turn, error)
	RecordResult(context.Context, string, *muse.Result) error
	AdminTurn(context.Context, string) (*muse.Turn, error)
}
