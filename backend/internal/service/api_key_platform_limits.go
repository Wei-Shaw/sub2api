package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// 按来源（平台）细分的 key 限额错误。与 key 级限额一样映射为 HTTP 429，
// 但错误码单独区分，便于调用方判断是「哪一个上游来源」触顶。
var (
	ErrAPIKeyPlatformQuotaExhausted   = infraerrors.TooManyRequests("API_KEY_PLATFORM_QUOTA_EXHAUSTED", "api key 在该来源上的额度已用完")
	ErrAPIKeyPlatformRate5hExceeded   = infraerrors.TooManyRequests("API_KEY_PLATFORM_RATE_5H_EXCEEDED", "api key 在该来源上的 5 小时限额已用完")
	ErrAPIKeyPlatformRate1dExceeded   = infraerrors.TooManyRequests("API_KEY_PLATFORM_RATE_1D_EXCEEDED", "api key 在该来源上的日限额已用完")
	ErrAPIKeyPlatformRate7dExceeded   = infraerrors.TooManyRequests("API_KEY_PLATFORM_RATE_7D_EXCEEDED", "api key 在该来源上的 7 天限额已用完")
	ErrAPIKeyPlatformLimitInvalidName = infraerrors.BadRequest("API_KEY_PLATFORM_LIMIT_INVALID_PLATFORM", "unsupported platform in api key platform limits")
)

// APIKeyPlatformUsageRecord 是单个 (api_key, platform) 的用量与窗口。
// 列语义与 api_keys 上的同名列完全一致：nil 窗口起点视为已过期。
type APIKeyPlatformUsageRecord struct {
	APIKeyID      int64
	Platform      string
	QuotaUsed     float64
	Usage5h       float64
	Usage1d       float64
	Usage7d       float64
	Window5hStart *time.Time
	Window1dStart *time.Time
	Window7dStart *time.Time
}

// APIKeyPlatformUsageRepository 读写按来源细分的 key 用量。
//
// 只有显式配置了 platform_limits 的 (key, platform) 才会有行：未配置的组合
// 不读不写，因此对既有部署是零额外负载。
type APIKeyPlatformUsageRepository interface {
	// Get 返回 (key, platform) 的用量；无记录时返回 nil, nil。
	Get(ctx context.Context, apiKeyID int64, platform string) (*APIKeyPlatformUsageRecord, error)
	// ListByAPIKey 返回该 key 的全部来源用量，供后台回显。
	ListByAPIKey(ctx context.Context, apiKeyID int64) ([]APIKeyPlatformUsageRecord, error)
	// IncrementUsage 累加一次消费：行不存在时创建，窗口过期时先归零再累加。
	IncrementUsage(ctx context.Context, apiKeyID int64, platform string, cost float64, now time.Time) error
	// ResetUsage 将指定来源的用量与窗口全部归零（管理员重置）。
	// platforms 为空表示重置该 key 的全部来源。
	ResetUsage(ctx context.Context, apiKeyID int64, platforms []string) error
}

// EffectiveUsage 按窗口有效期折算用量：窗口已过期的返回 0。
// 与 key 级 evaluateRateLimits 的判定保持一致（nil 起点 = 过期）。
func (r *APIKeyPlatformUsageRecord) EffectiveUsage() (usage5h, usage1d, usage7d float64) {
	if r == nil {
		return 0, 0, 0
	}
	if !IsWindowExpired(r.Window5hStart, RateLimitWindow5h) {
		usage5h = r.Usage5h
	}
	if !IsWindowExpired(r.Window1dStart, RateLimitWindow1d) {
		usage1d = r.Usage1d
	}
	if !IsWindowExpired(r.Window7dStart, RateLimitWindow7d) {
		usage7d = r.Usage7d
	}
	return usage5h, usage1d, usage7d
}

// EvaluateAPIKeyPlatformLimits 用一条用量记录判定某来源上的限额是否触顶。
//
// usage 为 nil（该来源尚无任何消费）时恒放行。总额度 quota 没有窗口概念，
// 因此直接与累计 quota_used 比较；三个滚动窗口沿用 key 级的过期判定。
func EvaluateAPIKeyPlatformLimits(limit APIKeyPlatformLimit, usage *APIKeyPlatformUsageRecord) error {
	if !limit.HasAnyLimit() || usage == nil {
		return nil
	}
	if limit.Quota > 0 && usage.QuotaUsed >= limit.Quota {
		return ErrAPIKeyPlatformQuotaExhausted
	}
	usage5h, usage1d, usage7d := usage.EffectiveUsage()
	if limit.RateLimit5h > 0 && usage5h >= limit.RateLimit5h {
		return ErrAPIKeyPlatformRate5hExceeded
	}
	if limit.RateLimit1d > 0 && usage1d >= limit.RateLimit1d {
		return ErrAPIKeyPlatformRate1dExceeded
	}
	if limit.RateLimit7d > 0 && usage7d >= limit.RateLimit7d {
		return ErrAPIKeyPlatformRate7dExceeded
	}
	return nil
}

// NormalizeAPIKeyPlatformLimits 校验并归一化用户提交的平台限额：
//   - 平台名必须在 AllowedQuotaPlatforms 内
//   - 任何一档必须是有限非负数（复用 key 级限额的校验）
//   - 四档全为 0 的条目视为「未配置」而被丢弃，避免建出永不生效的空行
//
// 返回的 map 为 nil 表示没有任何来源级限额。
func NormalizeAPIKeyPlatformLimits(in APIKeyPlatformLimits) (APIKeyPlatformLimits, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make(APIKeyPlatformLimits, len(in))
	for platform, limit := range in {
		if !IsAllowedQuotaPlatform(platform) {
			return nil, ErrAPIKeyPlatformLimitInvalidName
		}
		for _, v := range []float64{limit.Quota, limit.RateLimit5h, limit.RateLimit1d, limit.RateLimit7d} {
			if err := validateAPIKeyLimit(v); err != nil {
				return nil, err
			}
		}
		if !limit.HasAnyLimit() {
			continue
		}
		out[platform] = limit
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
