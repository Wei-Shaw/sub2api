package repository

import (
	"context"
	entsql "entgo.io/ent/dialect/sql"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/proxy"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ListOptions 为受限管理密钥在数据库内分页并只投影公开字段。
func (r *proxyRepository) ListOptions(ctx context.Context, params pagination.PaginationParams, search string) ([]service.Proxy, *pagination.PaginationResult, error) {
	q := r.client.Proxy.Query().Where(proxy.StatusEQ(service.StatusActive))
	if search = strings.TrimSpace(search); search != "" {
		q = q.Where(proxyOptionSearchPredicate(search))
	}
	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	records, err := q.Select(proxy.FieldID, proxy.FieldName, proxy.FieldProtocol, proxy.FieldHost, proxy.FieldPort, proxy.FieldStatus, proxy.FieldExpiresAt).Order(dbent.Desc(proxy.FieldID)).Offset(params.Offset()).Limit(params.Limit()).All(ctx)
	if err != nil {
		return nil, nil, err
	}
	items := make([]service.Proxy, 0, len(records))
	for _, record := range records {
		items = append(items, *proxyEntityToService(record))
	}
	return items, paginationResultFromTotal(int64(total), params), nil
}

// SampleOptions 先过滤全池，再由数据库随机排序取 50 条，禁止先固定分页再洗牌。
func (r *proxyRepository) SampleOptions(ctx context.Context, search string) ([]service.Proxy, int64, error) {
	q := r.client.Proxy.Query().Where(proxy.StatusEQ(service.StatusActive), proxy.Or(proxy.ExpiresAtIsNil(), proxy.ExpiresAtGT(time.Now())))
	if search = strings.TrimSpace(search); search != "" {
		q = q.Where(proxyOptionSearchPredicate(search))
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	records, err := q.Select(proxy.FieldID, proxy.FieldName, proxy.FieldProtocol, proxy.FieldHost, proxy.FieldPort, proxy.FieldStatus, proxy.FieldExpiresAt).
		Order(func(s *entsql.Selector) { s.OrderExpr(entsql.Expr("RANDOM()")) }).Limit(50).All(ctx)
	if err != nil {
		return nil, 0, err
	}
	items := make([]service.Proxy, 0, len(records))
	for _, record := range records {
		items = append(items, *proxyEntityToService(record))
	}
	return items, int64(total), nil
}
