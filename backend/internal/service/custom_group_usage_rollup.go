package service

import (
	"context"
	"time"

	appTimezone "github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

const groupUsageDateFormat = "2006-01-02"

// GroupUsageRollupRepository 是分组日汇总的可选持久化能力。
type GroupUsageRollupRepository interface {
	SyncGroupUsageRollups(ctx context.Context, todayStart time.Time) error
}

// GroupUsageTimezoneName 返回服务端配置的时区名称。
func GroupUsageTimezoneName() string {
	return appTimezone.Location().String()
}

// GroupUsageTodayStart 返回指定时刻在服务端配置时区内的自然日 UTC 起点。
func GroupUsageTodayStart(at time.Time) time.Time {
	return appTimezone.StartOfDay(at).UTC()
}

// GroupUsageYesterdayStart 返回指定时刻前一个本地日历日的 UTC 起点。
func GroupUsageYesterdayStart(at time.Time) time.Time {
	return appTimezone.StartOfDay(at).AddDate(0, 0, -1).UTC()
}

// GroupUsageDate 返回指定时刻在服务端配置时区内的日期。
func GroupUsageDate(at time.Time) string {
	return at.In(appTimezone.Location()).Format(groupUsageDateFormat)
}

// ParseGroupUsageDate 解析服务端配置时区内的日期并返回其零点。
func ParseGroupUsageDate(value string) (time.Time, error) {
	return appTimezone.ParseInLocation(groupUsageDateFormat, value)
}
