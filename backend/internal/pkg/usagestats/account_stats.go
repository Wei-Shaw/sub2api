package usagestats

// AccountStats 账号使用统计
type AccountStats struct {
	Requests int64   `json:"requests"`
	Tokens   int64   `json:"tokens"`
	Cost     float64 `json:"cost"`
REDACTED
