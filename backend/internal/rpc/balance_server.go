// Package rpc 实现余额账本的 tRPC-Go 服务（独立端口）。
package rpc

import (
	"context"
	"math"
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/rpc/balancepb"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"trpc.group/trpc-go/trpc-go/errs"
)

// balanceLedgerServer 把 tRPC 请求转调薄账本服务。
type balanceLedgerServer struct {
	balancepb.UnimplementedBalanceLedger
	ledger *service.BalanceLedgerService
}

func newBalanceLedgerServer(ledger *service.BalanceLedgerService) *balanceLedgerServer {
	return &balanceLedgerServer{ledger: ledger}
}

func (s *balanceLedgerServer) Deduct(ctx context.Context, req *balancepb.DeductRequest) (*balancepb.DeductResponse, error) {
	amount, err := parseAmount(req.GetAmount())
	if err != nil {
		return nil, toTRPCError(service.ErrLedgerInvalidAmount)
	}
	res, err := s.ledger.Deduct(ctx, &service.LedgerDeductCommand{
		AppID:       appIDFromContext(ctx),
		RequestID:   req.GetRequestId(),
		UserID:      req.GetUserId(),
		Amount:      amount,
		Description: req.GetDescription(),
		Extra:       req.GetExtra(),
	})
	if err != nil {
		return nil, toTRPCError(err)
	}
	return &balancepb.DeductResponse{
		Applied:      res.Applied,
		BalanceAfter: formatAmount(res.BalanceAfter),
	}, nil
}

func (s *balanceLedgerServer) Refund(ctx context.Context, req *balancepb.RefundRequest) (*balancepb.RefundResponse, error) {
	amount, err := parseAmount(req.GetAmount())
	if err != nil {
		return nil, toTRPCError(service.ErrLedgerInvalidAmount)
	}
	res, err := s.ledger.Refund(ctx, &service.LedgerRefundCommand{
		AppID:             appIDFromContext(ctx),
		RefundRequestID:   req.GetRefundRequestId(),
		OriginalRequestID: req.GetOriginalRequestId(),
		Amount:            amount,
		Description:       req.GetDescription(),
		Extra:             req.GetExtra(),
	})
	if err != nil {
		return nil, toTRPCError(err)
	}
	return &balancepb.RefundResponse{
		Applied:       res.Applied,
		BalanceAfter:  formatAmount(res.BalanceAfter),
		RefundedTotal: formatAmount(res.RefundedTotal),
	}, nil
}

func (s *balanceLedgerServer) GetBalance(ctx context.Context, req *balancepb.GetBalanceRequest) (*balancepb.GetBalanceResponse, error) {
	balance, err := s.ledger.GetBalance(ctx, req.GetUserId())
	if err != nil {
		return nil, toTRPCError(err)
	}
	return &balancepb.GetBalanceResponse{Balance: formatAmount(balance)}, nil
}

// parseAmount 解析十进制字符串金额；拒绝空 / 非法 / 负数 / NaN / Inf。
func parseAmount(s string) (float64, error) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, service.ErrLedgerInvalidAmount
	}
	if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
		return 0, service.ErrLedgerInvalidAmount
	}
	return v, nil
}

// formatAmount 输出最短可往返的十进制字符串。
func formatAmount(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// toTRPCError 把应用错误映射为 tRPC 错误：保留 infraerrors 的 code 与 message。
func toTRPCError(err error) error {
	if err == nil {
		return nil
	}
	return errs.New(infraerrors.Code(err), infraerrors.Message(err))
}
