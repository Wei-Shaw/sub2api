//go:build unit

package service

import (
	"encoding/json"
	"math"
)

// parseIntegralNumber 将 JSON 解码后的数字安全转换为 int。
// 仅接受“整数值”的输入，小数/NaN/Inf/越界值都会返回 false。
//
// 说明：
//   - 该函数当前仅用于 unit 测试中的 map-based 解析逻辑验证，因此放在 unit build tag 下，
//     避免在默认构建中触发 unused lint。
func parseIntegralNumber(raw any) (int, bool) {
	switch v := raw.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v != math.Trunc(v) {
			return 0, false
		}
		if v > float64(math.MaxInt) || v < float64(math.MinInt) {
			return 0, false
		}
		return int(v), true
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		if v > int64(math.MaxInt) || v < int64(math.MinInt) {
			return 0, false
		}
		return int(v), true
	case json.Number:
		i64, err := v.Int64()
		if err != nil {
			return 0, false
		}
		if i64 > int64(math.MaxInt) || i64 < int64(math.MinInt) {
			return 0, false
		}
		return int(i64), true
	default:
		return 0, false
	}
}
