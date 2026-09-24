package repository

import (
	"context"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikeyplatformusage"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// apiKeyPlatformUsageRepository 维护 (api_key, platform) 维度的用量与滚动窗口。
//
// 只有显式配置了 api_keys.platform_limits 的组合才会被读写，因此未使用该特性的
// 部署完全不触碰这张表。
type apiKeyPlatformUsageRepository struct {
	client *dbent.Client
}

// NewAPIKeyPlatformUsageRepository 创建按来源细分的 key 用量仓储。
func NewAPIKeyPlatformUsageRepository(client *dbent.Client) service.APIKeyPlatformUsageRepository {
	return &apiKeyPlatformUsageRepository{client: client}
}

func (r *apiKeyPlatformUsageRepository) Get(ctx context.Context, apiKeyID int64, platform string) (*service.APIKeyPlatformUsageRecord, error) {
	if apiKeyID <= 0 || platform == "" {
		return nil, nil
	}
	client := clientFromContext(ctx, r.client)
	row, err := client.APIKeyPlatformUsage.Query().
		Where(
			apikeyplatformusage.APIKeyIDEQ(apiKeyID),
			apikeyplatformusage.PlatformEQ(platform),
		).
		Only(ctx)
	if dbent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return entPlatformUsageToRecord(row), nil
}

func (r *apiKeyPlatformUsageRepository) ListByAPIKey(ctx context.Context, apiKeyID int64) ([]service.APIKeyPlatformUsageRecord, error) {
	if apiKeyID <= 0 {
		return nil, nil
	}
	client := clientFromContext(ctx, r.client)
	rows, err := client.APIKeyPlatformUsage.Query().
		Where(apikeyplatformusage.APIKeyIDEQ(apiKeyID)).
		Order(dbent.Asc(apikeyplatformusage.FieldPlatform)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]service.APIKeyPlatformUsageRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, *entPlatformUsageToRecord(row))
	}
	return out, nil
}

// IncrementUsage 累加一次消费。行不存在时按 cost 建行；任一窗口过期时该窗口先归零
// 再累加，并把窗口起点重置为 now（与 key 级 usage_5h/1d/7d 的语义一致）。
func (r *apiKeyPlatformUsageRepository) IncrementUsage(ctx context.Context, apiKeyID int64, platform string, cost float64, now time.Time) error {
	if apiKeyID <= 0 || platform == "" || cost <= 0 {
		return nil
	}
	err := r.incrementUsageOnce(ctx, apiKeyID, platform, cost, now)
	if dbent.IsConstraintError(err) {
		// 并发首次写入：另一个请求刚插入同一 (api_key, platform)，重试一次走更新分支。
		return r.incrementUsageOnce(ctx, apiKeyID, platform, cost, now)
	}
	return err
}

func (r *apiKeyPlatformUsageRepository) incrementUsageOnce(ctx context.Context, apiKeyID int64, platform string, cost float64, now time.Time) error {
	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		existing, err := txClient.APIKeyPlatformUsage.Query().
			Where(
				apikeyplatformusage.APIKeyIDEQ(apiKeyID),
				apikeyplatformusage.PlatformEQ(platform),
			).
			ForUpdate().
			Only(txCtx)
		if dbent.IsNotFound(err) {
			return txClient.APIKeyPlatformUsage.Create().
				SetAPIKeyID(apiKeyID).
				SetPlatform(platform).
				SetQuotaUsed(cost).
				SetUsage5h(cost).
				SetUsage1d(cost).
				SetUsage7d(cost).
				SetWindow5hStart(now).
				SetWindow1dStart(now).
				SetWindow7dStart(now).
				Exec(txCtx)
		}
		if err != nil {
			return err
		}

		usage5h, start5h := advanceRollingWindow(existing.Usage5h, existing.Window5hStart, service.RateLimitWindow5h, cost, now)
		usage1d, start1d := advanceRollingWindow(existing.Usage1d, existing.Window1dStart, service.RateLimitWindow1d, cost, now)
		usage7d, start7d := advanceRollingWindow(existing.Usage7d, existing.Window7dStart, service.RateLimitWindow7d, cost, now)

		return existing.Update().
			SetQuotaUsed(existing.QuotaUsed + cost).
			SetUsage5h(usage5h).
			SetUsage1d(usage1d).
			SetUsage7d(usage7d).
			SetWindow5hStart(start5h).
			SetWindow1dStart(start1d).
			SetWindow7dStart(start7d).
			Exec(txCtx)
	})
}

// ResetUsage 将指定来源（platforms 为空则全部）的用量与窗口归零。
func (r *apiKeyPlatformUsageRepository) ResetUsage(ctx context.Context, apiKeyID int64, platforms []string) error {
	if apiKeyID <= 0 {
		return nil
	}
	client := clientFromContext(ctx, r.client)
	upd := client.APIKeyPlatformUsage.Update().
		Where(apikeyplatformusage.APIKeyIDEQ(apiKeyID))
	if len(platforms) > 0 {
		upd = upd.Where(apikeyplatformusage.PlatformIn(platforms...))
	}
	_, err := upd.
		SetQuotaUsed(0).
		SetUsage5h(0).
		SetUsage1d(0).
		SetUsage7d(0).
		ClearWindow5hStart().
		ClearWindow1dStart().
		ClearWindow7dStart().
		Save(ctx)
	return err
}

func (r *apiKeyPlatformUsageRepository) withTx(ctx context.Context, fn func(txCtx context.Context, txClient *dbent.Client) error) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return fn(ctx, tx.Client())
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin api_key_platform_usage transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := fn(txCtx, tx.Client()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit api_key_platform_usage transaction: %w", err)
	}
	return nil
}

// advanceRollingWindow 返回累加后的窗口用量与窗口起点：
// 窗口已过期（含起点为 NULL）时用量重置为 cost 并以 now 作为新起点，否则累加并保留原起点。
func advanceRollingWindow(prevUsage float64, start *time.Time, window time.Duration, cost float64, now time.Time) (float64, time.Time) {
	if start == nil || now.Sub(*start) >= window {
		return cost, now
	}
	return prevUsage + cost, *start
}

func entPlatformUsageToRecord(e *dbent.APIKeyPlatformUsage) *service.APIKeyPlatformUsageRecord {
	if e == nil {
		return nil
	}
	return &service.APIKeyPlatformUsageRecord{
		APIKeyID:      e.APIKeyID,
		Platform:      e.Platform,
		QuotaUsed:     e.QuotaUsed,
		Usage5h:       e.Usage5h,
		Usage1d:       e.Usage1d,
		Usage7d:       e.Usage7d,
		Window5hStart: e.Window5hStart,
		Window1dStart: e.Window1dStart,
		Window7dStart: e.Window7dStart,
	}
}
