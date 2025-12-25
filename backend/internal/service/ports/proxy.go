package ports

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type ProxyRepository interface {
	Create(ctx context.Context, proxy *model.Proxy) error
	GetByID(ctx context.Context, id int64) (*model.Proxy, error)
	Update(ctx context.Context, proxy *model.Proxy) error
	Delete(ctx context.Context, id int64) error

	List(ctx context.Context, params pagination.PaginationParams) ([]model.Proxy, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]model.Proxy, *pagination.PaginationResult, error)
	ListActive(ctx context.Context) ([]model.Proxy, error)
	ListActiveWithAccountCount(ctx context.Context) ([]model.ProxyWithAccountCount, error)

	ExistsByHostPortAuth(ctx context.Context, host string, port int, username, password string) (bool, error)
	CountAccountsByProxyID(ctx context.Context, proxyID int64) (int64, error)
}
