package service

import (
	"context"
	"errors"
	"testing"
)

type fakeBalanceLedgerRepo struct {
	findCalled   bool
	deductCalled bool
	refundCalled bool
	deductRes    *LedgerDeductResult
	refundRes    *LedgerRefundResult
	err          error
	existing     *LedgerDeductResult
}

type failingBillingContextRepository struct {
	OrganizationRepository
	err error
}

func (r *failingBillingContextRepository) ResolveBillingContext(context.Context, int64) (*BillingContext, error) {
	return nil, r.err
}

func (f *fakeBalanceLedgerRepo) FindDeduct(_ context.Context, _ *LedgerDeductCommand) (*LedgerDeductResult, error) {
	f.findCalled = true
	if f.err != nil {
		return nil, f.err
	}
	return f.existing, nil
}

func (f *fakeBalanceLedgerRepo) Deduct(_ context.Context, _ *LedgerDeductCommand) (*LedgerDeductResult, error) {
	f.deductCalled = true
	if f.err != nil {
		return nil, f.err
	}
	return f.deductRes, nil
}

func (f *fakeBalanceLedgerRepo) Refund(_ context.Context, _ *LedgerRefundCommand) (*LedgerRefundResult, error) {
	f.refundCalled = true
	if f.err != nil {
		return nil, f.err
	}
	return f.refundRes, nil
}

func (f *fakeBalanceLedgerRepo) AppStats(_ context.Context, appID string) (*AppLedgerStats, error) {
	return &AppLedgerStats{AppID: appID}, nil
}

func TestBalanceLedgerService_Deduct_Validation(t *testing.T) {
	repo := &fakeBalanceLedgerRepo{deductRes: &LedgerDeductResult{Applied: true, BalanceAfter: 5}}
	// cache 传 nil：invalidateBalance 对 nil cache 是 no-op。
	svc := NewBalanceLedgerService(repo, nil)
	ctx := context.Background()

	// amount <= 0 被拒，且不触达仓储。
	if _, err := svc.Deduct(ctx, &LedgerDeductCommand{Amount: 0, Description: "x"}); !errors.Is(err, ErrLedgerInvalidAmount) {
		t.Fatalf("zero amount: expected ErrLedgerInvalidAmount, got %v", err)
	}
	if repo.deductCalled {
		t.Fatal("repo.Deduct should not be called on invalid amount")
	}

	// description 空被拒。
	if _, err := svc.Deduct(ctx, &LedgerDeductCommand{Amount: 1, Description: "  "}); !errors.Is(err, ErrLedgerDescriptionRequired) {
		t.Fatalf("empty description: expected ErrLedgerDescriptionRequired, got %v", err)
	}

	// 合法入参透传仓储。
	res, err := svc.Deduct(ctx, &LedgerDeductCommand{Amount: 1, Description: "ok", UserID: 1})
	if err != nil {
		t.Fatalf("valid deduct: %v", err)
	}
	if !repo.deductCalled || res == nil || !res.Applied {
		t.Fatalf("expected repo called and applied result, got called=%v res=%+v", repo.deductCalled, res)
	}
}

func TestBalanceLedgerService_Refund_Validation(t *testing.T) {
	repo := &fakeBalanceLedgerRepo{refundRes: &LedgerRefundResult{Applied: true, UserID: 1, BalanceAfter: 9, RefundedTotal: 1}}
	svc := NewBalanceLedgerService(repo, nil)
	ctx := context.Background()

	if _, err := svc.Refund(ctx, &LedgerRefundCommand{Amount: -1, Description: "x"}); !errors.Is(err, ErrLedgerInvalidAmount) {
		t.Fatalf("negative amount: expected ErrLedgerInvalidAmount, got %v", err)
	}
	if _, err := svc.Refund(ctx, &LedgerRefundCommand{Amount: 1, Description: ""}); !errors.Is(err, ErrLedgerDescriptionRequired) {
		t.Fatalf("empty description: expected ErrLedgerDescriptionRequired, got %v", err)
	}
	if repo.refundCalled {
		t.Fatal("repo.Refund should not be called on invalid input")
	}

	res, err := svc.Refund(ctx, &LedgerRefundCommand{Amount: 1, Description: "ok"})
	if err != nil {
		t.Fatalf("valid refund: %v", err)
	}
	if !repo.refundCalled || res == nil || res.RefundedTotal != 1 {
		t.Fatalf("expected repo called, got called=%v res=%+v", repo.refundCalled, res)
	}
}

func TestBalanceLedgerService_DeductReplayDoesNotResolveCurrentAuthorization(t *testing.T) {
	existing := &LedgerDeductResult{Applied: false, PayerUserID: 99, BalanceSource: "shared", BalanceAfter: 7}
	repo := &fakeBalanceLedgerRepo{existing: existing}
	svc := NewBalanceLedgerService(repo, nil)
	svc.SetBillingContextResolver(NewBillingContextResolver(&failingBillingContextRepository{err: errors.New("organization suspended")}))

	result, err := svc.Deduct(context.Background(), &LedgerDeductCommand{
		AppID: "billing-app", RequestID: "request-1", UserID: 22, Amount: 1, Description: "usage",
	})

	if err != nil {
		t.Fatalf("replay must not resolve current authorization: %v", err)
	}
	if result != existing || repo.deductCalled {
		t.Fatalf("expected committed replay without a new deduction, result=%+v called=%v", result, repo.deductCalled)
	}
}
