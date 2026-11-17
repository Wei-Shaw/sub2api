package upstreampricing

import (
	"context"
	"math/big"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var stationTimezone = time.FixedZone("Asia/Shanghai", 8*60*60)
var walletNumberPattern = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]{1,2})?$`)

func activityWindow(now time.Time, completed *time.Time, sameIdentity bool) (time.Time, time.Time) {
	local := now.In(stationTimezone)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, stationTimezone)
	if sameIdentity && completed != nil && completed.After(start) && !completed.After(now) {
		start = *completed
	}
	return start, now
}

// Only normalized identifiers, model names and timings are returned. Never
// persist a station's raw log payload, messages, cookies or panel credentials.
func (r *Remote) activityRecords(ctx context.Context, c Config, start, end time.Time) ([]TimingRecord, error) {
	rows := []TimingRecord{}
	seen := map[string]bool{}
	total := int64(-1)
	var previous time.Time
	for page := 1; page <= 500; page++ {
		q := url.Values{"page_size": {"100"}}
		path := "/api/v1/usage"
		if c.Provider == "newapi" {
			path = "/api/log/self"
			q.Set("p", strconv.Itoa(page))
			q.Set("type", "2")
			q.Set("start_timestamp", strconv.FormatInt(start.Unix(), 10))
			q.Set("end_timestamp", strconv.FormatInt(end.Unix(), 10))
			q.Set("group", c.Group)
		} else {
			q.Set("page", strconv.Itoa(page))
			q.Set("timezone", "Asia/Shanghai")
			q.Set("start_date", start.In(stationTimezone).Format("2006-01-02"))
			q.Set("end_date", end.In(stationTimezone).Format("2006-01-02"))
			q.Set("sort_by", "created_at")
			q.Set("sort_order", "desc")
		}
		v, err := r.get(ctx, c, path+"?"+q.Encode())
		if err != nil {
			return nil, err
		}
		data := object(v["data"])
		items, ok := data["items"].([]any)
		n := timingInteger(data["total"])
		if !ok || n == nil || (total >= 0 && *n != total) || len(items) > 100 || (len(items) == 0 && int64((page-1)*100) < *n) {
			return nil, safeError("站点明细分页不兼容或查询期间发生变化，将在下个周期重试")
		}
		total = *n
		pastStart := false
		for _, value := range items {
			raw := object(value)
			if c.Provider == "newapi" {
				if stringValue(raw["type"]) == "" || stringValue(raw["group"]) == "" {
					return nil, safeError("站点明细缺少类型或分组，无法验证查询范围")
				}
				if stringValue(raw["type"]) != "2" || stringValue(raw["group"]) != c.Group {
					continue
				}
			}
			row, valid := normalizeTiming(raw, c.Provider)
			if !valid || seen[row.ID] {
				return nil, safeError("站点明细包含缺失或重复标识，将在下个周期重试")
			}
			seen[row.ID] = true
			if c.Provider == "sub2api" {
				if !previous.IsZero() && row.CreatedAt.After(previous) {
					return nil, safeError("站点明细未按时间倒序返回，未推进同步时间")
				}
				previous = row.CreatedAt
				pastStart = pastStart || row.CreatedAt.Before(start)
			}
			if !row.CreatedAt.Before(start) && !row.CreatedAt.After(end) {
				rows = append(rows, row)
			}
		}
		if pastStart || int64((page-1)*100+len(items)) >= total {
			return rows, nil
		}
		if len(items) < 100 {
			return nil, safeError("站点明细分页不完整，未推进同步时间")
		}
	}
	return nil, safeError("本次明细超过 500 页，未推进同步时间")
}

func (r *Remote) wallet(ctx context.Context, c Config) (string, error) {
	if c.Provider != "sub2api" {
		return "", safeError("当前站点类型暂不支持钱包余额查询")
	}
	v, err := r.get(ctx, c, "/api/v1/auth/me?timezone=Asia%2FShanghai")
	if err != nil {
		return "", err
	}
	text := stringValue(object(v["data"])["balance"])
	if len(text) == 0 || len(text) > 80 || !walletNumberPattern.MatchString(text) {
		return "", safeError("站点未返回有效的钱包余额")
	}
	// Bound scientific exponents before big.Rat allocates their expanded value.
	if i := strings.IndexAny(text, "eE"); i >= 0 {
		exponent, parseErr := strconv.Atoi(text[i+1:])
		if parseErr != nil || exponent < -40 || exponent > 40 {
			return "", safeError("站点钱包余额超出支持范围")
		}
	}
	value, ok := new(big.Rat).SetString(text)
	if !ok || len(value.Num().String()) > 100 || len(value.Denom().String()) > 100 {
		return "", safeError("站点未返回有效的钱包余额")
	}
	// Negative and zero balances are legitimate; never turn missing data into zero.
	return text, nil
}
