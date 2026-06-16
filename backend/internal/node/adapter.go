package node

// xiugai 添加节点功能

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// accountRepoLister 将 service.AccountRepository 适配为 AccountLister 接口，
// 通过分页查询将全量账号数据逐批加载后返回。
type accountRepoLister struct {
	// repo 底层账号数据库访问层接口。
	repo service.AccountRepository
}

// NewRepoAccountLister 将 service.AccountRepository 包装为 AccountLister，
// 供 Reporter 调用以获取本地全量账号数据。
//
// 参数：
//   - repo：账号数据库访问层接口，由 Wire 注入。
//
// 返回值：
//   - AccountLister：可被 Reporter 使用的账号列表接口实现。
func NewRepoAccountLister(repo service.AccountRepository) AccountLister {
	return &accountRepoLister{repo: repo}
}

// ListAll 返回数据库中所有账号（不限状态），采用分页查询避免一次性加载过多数据。
// 每批最多查询 1000 条，直到取完所有记录为止。
//
// 参数：
//   - ctx：查询上下文，用于超时和取消控制。
//
// 返回值：
//   - []NodeAccount：所有账号的 ID、名称、状态切片。
//   - error：数据库查询失败时返回错误。
func (l *accountRepoLister) ListAll(ctx context.Context) ([]NodeAccount, error) {
	const pageSize = 1000

	var all []NodeAccount
	page := 1
	for {
		rows, result, err := l.repo.List(ctx, pagination.PaginationParams{
			Page:      page,
			PageSize:  pageSize,
			SortOrder: pagination.SortOrderAsc,
		})
		if err != nil {
			return nil, err
		}

		for _, a := range rows {
			all = append(all, NodeAccount{
				ID:                     a.ID,
				Name:                   a.Name,
				Status:                 a.Status,
				Schedulable:            a.Schedulable,
				RateLimitResetAt:       a.RateLimitResetAt,
				OverloadUntil:          a.OverloadUntil,
				TempUnschedulableUntil: a.TempUnschedulableUntil,
				QuotaExceeded:          a.IsQuotaExceeded(),
			})
		}

		// 已取完所有记录或当前批次不足一页时退出循环。
		if int64(len(all)) >= result.Total || len(rows) < pageSize {
			break
		}
		page++
	}

	return all, nil
}

// end
