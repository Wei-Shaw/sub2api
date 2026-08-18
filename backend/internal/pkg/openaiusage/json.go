package openaiusage

import (
	"strconv"

	"github.com/tidwall/gjson"
)

// JSONNumberInt64 accepts only JSON integer numbers representable by int64.
func JSONNumberInt64(value gjson.Result) (int64, bool) {
	if !value.Exists() || value.Type != gjson.Number {
		return 0, false
	}
	n, err := strconv.ParseInt(value.Raw, 10, 64)
	return n, err == nil
}
