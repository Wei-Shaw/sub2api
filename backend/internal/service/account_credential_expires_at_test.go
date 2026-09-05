//go:build unit

package service

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// GetCredentialAsTime 必须同时兼容秒级与毫秒级 Unix 时间戳。
// Claude Code 的 ~/.claude/.credentials.json 用毫秒记录 expiresAt，
// 导入凭据时可能被原样写入 credentials；若按秒解析会落到公元 5000 年之后，
// 使 time.Until() 永远大于刷新窗口，token 因此永不刷新，
// 直到 access_token 到期后请求只能拿到上游 401。
func TestGetCredentialAsTime_UnitDetection(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  time.Time
	}{
		{
			name:  "unix seconds string",
			value: "1700000000",
			want:  time.Unix(1700000000, 0),
		},
		{
			name:  "unix seconds number",
			value: float64(1700000000),
			want:  time.Unix(1700000000, 0),
		},
		{
			name:  "unix milliseconds string",
			value: "1700000000000",
			want:  time.UnixMilli(1700000000000),
		},
		{
			name:  "unix milliseconds number",
			value: float64(1700000000000),
			want:  time.UnixMilli(1700000000000),
		},
		{
			name:  "rfc3339 string",
			value: "2023-11-14T22:13:20Z",
			want:  time.Unix(1700000000, 0),
		},
		{
			// 边界：秒级时间戳的合理上限之内仍按秒解析
			name:  "large but plausible unix seconds",
			value: "9999999999",
			want:  time.Unix(9999999999, 0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{Credentials: map[string]any{"expires_at": tt.value}}

			got := account.GetCredentialAsTime("expires_at")

			require.NotNil(t, got)
			require.True(t, tt.want.Equal(*got), "want %s, got %s", tt.want.UTC(), got.UTC())
		})
	}
}

func TestGetCredentialAsTime_Invalid(t *testing.T) {
	tests := []struct {
		name        string
		credentials map[string]any
	}{
		{name: "missing", credentials: map[string]any{}},
		{name: "nil value", credentials: map[string]any{"expires_at": nil}},
		{name: "invalid string", credentials: map[string]any{"expires_at": "invalid"}},
		{name: "nil credentials", credentials: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{Credentials: tt.credentials}

			require.Nil(t, account.GetCredentialAsTime("expires_at"))
		})
	}
}

// 毫秒时间戳被按秒解析时的具体后果：一个早已过期的 token 看起来还有数万年寿命。
func TestGetCredentialAsTime_MillisecondsDoNotLandInTheFarFuture(t *testing.T) {
	expiredMillis := strconv.FormatInt(time.Now().Add(-1*time.Hour).UnixMilli(), 10)

	account := &Account{Credentials: map[string]any{"expires_at": expiredMillis}}

	got := account.GetCredentialAsTime("expires_at")

	require.NotNil(t, got)
	require.True(t, got.Before(time.Now()), "an expired millisecond timestamp must parse as expired, got %s", got.UTC())
	require.Negative(t, time.Until(*got))
}
