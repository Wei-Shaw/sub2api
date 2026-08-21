// Package rpc 实现内部 API 的 tRPC-Go 服务（独立端口）。
package rpc

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/rpc/innerpb"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"trpc.group/trpc-go/trpc-go/errs"
)

// innerAPIServer 负责内部 API 的余额与素材方法。
type innerAPIServer struct {
	innerpb.UnimplementedInnerAPI
	ledger    *service.BalanceLedgerService
	materials *service.UserMaterialService
	users     service.UserAccountRepository
}

func newInnerAPIServer(ledger *service.BalanceLedgerService, materials *service.UserMaterialService, users service.UserAccountRepository) *innerAPIServer {
	return &innerAPIServer{ledger: ledger, materials: materials, users: users}
}

func (s *innerAPIServer) Deduct(ctx context.Context, req *innerpb.DeductRequest) (*innerpb.DeductResponse, error) {
	amount, err := parseAmount(req.GetAmount())
	if err != nil {
		return nil, toTRPCError(service.ErrLedgerInvalidAmount)
	}
	user, err := s.userByAccountID(ctx, req.GetAccountId())
	if err != nil {
		return nil, toTRPCError(err)
	}
	res, err := s.ledger.Deduct(ctx, &service.LedgerDeductCommand{
		AppID:       appIDFromContext(ctx),
		RequestID:   req.GetRequestId(),
		UserID:      user.ID,
		Amount:      amount,
		Description: req.GetDescription(),
		Extra:       req.GetExtra(),
	})
	if err != nil {
		return nil, toTRPCError(err)
	}
	return &innerpb.DeductResponse{
		Applied:         res.Applied,
		BalanceAfter:    formatAmount(res.BalanceAfter),
		PayerAccountId:  s.accountIDByUserID(ctx, res.PayerUserID),
		BalanceSource:   res.BalanceSource,
		OrganizationId:  int64Value(res.OrganizationID),
		AuthzGeneration: res.AuthzGeneration,
	}, nil
}

func (s *innerAPIServer) Refund(ctx context.Context, req *innerpb.RefundRequest) (*innerpb.RefundResponse, error) {
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
	return &innerpb.RefundResponse{
		Applied:         res.Applied,
		BalanceAfter:    formatAmount(res.BalanceAfter),
		RefundedTotal:   formatAmount(res.RefundedTotal),
		PayerAccountId:  s.accountIDByUserID(ctx, res.PayerUserID),
		BalanceSource:   res.BalanceSource,
		OrganizationId:  int64Value(res.OrganizationID),
		AuthzGeneration: res.AuthzGeneration,
	}, nil
}

func (s *innerAPIServer) GetBalance(ctx context.Context, req *innerpb.GetBalanceRequest) (*innerpb.GetBalanceResponse, error) {
	user, err := s.userByAccountID(ctx, req.GetAccountId())
	if err != nil {
		return nil, toTRPCError(err)
	}
	balance, err := s.ledger.GetBalance(ctx, user.ID)
	if err != nil {
		return nil, toTRPCError(err)
	}
	return &innerpb.GetBalanceResponse{Balance: formatAmount(balance.Balance), PayerAccountId: s.accountIDByUserID(ctx, balance.PayerUserID), BalanceSource: balance.BalanceSource, OrganizationId: int64Value(balance.OrganizationID), AuthzGeneration: balance.AuthzGeneration}, nil
}

func (s *innerAPIServer) ListMaterials(ctx context.Context, req *innerpb.ListMaterialsRequest) (*innerpb.ListMaterialsResponse, error) {
	if s.materials == nil {
		return nil, toTRPCError(service.ErrCOSNotConfigured)
	}
	user, err := s.userByAccountID(ctx, req.GetAccountId())
	if err != nil {
		return nil, toTRPCError(err)
	}
	items, total, err := s.materials.List(ctx, user.ID, req.GetKind(), req.GetKeyword(), int(req.GetPage()), int(req.GetPageSize()))
	if err != nil {
		return nil, toTRPCError(err)
	}
	out := make([]*innerpb.Material, 0, len(items))
	for _, item := range items {
		out = append(out, materialResponse(item, user.AccountID))
	}
	return &innerpb.ListMaterialsResponse{Items: out, Total: total, Page: req.GetPage(), PageSize: req.GetPageSize()}, nil
}

func (s *innerAPIServer) GetMaterial(ctx context.Context, req *innerpb.GetMaterialRequest) (*innerpb.Material, error) {
	if s.materials == nil {
		return nil, toTRPCError(service.ErrCOSNotConfigured)
	}
	user, err := s.userByAccountID(ctx, req.GetAccountId())
	if err != nil {
		return nil, toTRPCError(err)
	}
	item, err := s.materials.GetByID(ctx, user.ID, req.GetId())
	if err != nil {
		return nil, toTRPCError(err)
	}
	return materialResponse(item, user.AccountID), nil
}

func (s *innerAPIServer) UploadMaterial(ctx context.Context, req *innerpb.UploadMaterialRequest) (*innerpb.UploadMaterialResponse, error) {
	if s.materials == nil {
		return nil, toTRPCError(service.ErrCOSNotConfigured)
	}
	user, err := s.userByAccountID(ctx, req.GetAccountId())
	if err != nil {
		return nil, toTRPCError(err)
	}
	item, err := s.materials.UploadBytes(ctx, user.ID, req.GetFileName(), req.GetContentType(), req.GetData())
	if err != nil {
		return nil, toTRPCError(err)
	}
	material := materialResponse(item, user.AccountID)
	return &innerpb.UploadMaterialResponse{Material: material, FileUrl: material.GetUrl()}, nil
}

func (s *innerAPIServer) DeleteMaterial(ctx context.Context, req *innerpb.DeleteMaterialRequest) (*innerpb.DeleteMaterialResponse, error) {
	if s.materials == nil {
		return nil, toTRPCError(service.ErrCOSNotConfigured)
	}
	user, err := s.userByAccountID(ctx, req.GetAccountId())
	if err != nil {
		return nil, toTRPCError(err)
	}
	if err := s.materials.Delete(ctx, user.ID, req.GetId()); err != nil {
		return nil, toTRPCError(err)
	}
	return &innerpb.DeleteMaterialResponse{Id: req.GetId(), Deleted: true}, nil
}

func materialResponse(item *service.UserMaterial, accountID string) *innerpb.Material {
	if item == nil {
		return &innerpb.Material{}
	}
	createdAt := ""
	if !item.CreatedAt.IsZero() {
		createdAt = item.CreatedAt.UTC().Format(time.RFC3339)
	}
	return &innerpb.Material{
		Id: item.ID, AccountId: accountID, FileName: item.FileName, Url: item.CosURL,
		ContentType: item.ContentType, SizeBytes: item.SizeBytes, Kind: item.Kind,
		Source: item.Source, CreatedAt: createdAt,
	}
}

func (s *innerAPIServer) userByAccountID(ctx context.Context, accountID string) (*service.User, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" || s == nil || s.users == nil {
		return nil, service.ErrUserNotFound
	}
	return s.users.GetByAccountID(ctx, accountID)
}

func (s *innerAPIServer) accountIDByUserID(ctx context.Context, userID int64) string {
	if s == nil || s.users == nil || userID <= 0 {
		return ""
	}
	user, err := s.users.GetByIDIncludeDeleted(ctx, userID)
	if err != nil || user == nil {
		return ""
	}
	return user.AccountID
}

func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
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
