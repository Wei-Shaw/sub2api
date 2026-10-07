package usagestats

import (
	"github.com/tidwall/gjson"
	"math"
)

// OptionalTokenCount preserves missing versus explicit zero without estimating counts.
// Counts must fit the INTEGER usage-log column; malformed optional data stays unknown.
func OptionalTokenCount(value gjson.Result, paths ...string) *int {
	for _, path := range paths {
		count := value.Get(path)
		if count.Type == gjson.Number && count.Num >= 0 && count.Num <= math.MaxInt32 && math.Trunc(count.Num) == count.Num {
			result := int(count.Num)
			return &result
		}
	}
	return nil
}
