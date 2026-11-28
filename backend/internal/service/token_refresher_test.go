//go:build unit

package service

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClaudeTokenRefresher_NeedsRefresh(t *testing.T) {
	refresher := &ClaudeTokenRefresher{}
	refreshWindow := 30 * time.Minute

	// 毫秒级时间戳：Claude Code 的 ~/.claude/.credentials.json 用毫秒记录 expiresAt，
	// 导入凭据时可能被原样写入 credentials。
	expiredMillis := strconv.FormatInt(time.Now().Add(-1*time.Hour).UnixMilli(), 10)
	futureMillis := strconv.FormatInt(time.Now().Add(12*time.Hour).UnixMilli(), 10)

	tests := []struct {
		name        string
		credentials map[string]any
		wantRefresh bool
	}{
		{
			name: "expires_at as string - expired",
			credentials: map[string]any{
				"refresh_token": "rt-test",
				"expires_at":    "1000", // 1970-01-01 00:16:40 UTC, 已过期
			},
			wantRefresh: true,
		},
		{
			name: "expires_at as float64 - expired",
			credentials: map[string]any{
				"refresh_token": "rt-test",
				"expires_at":    float64(1000), // 数字类型，已过期
			},
			wantRefresh: true,
		},
		{
			name: "expires_at as RFC3339 - expired",
			credentials: map[string]any{
				"refresh_token": "rt-test",
				"expires_at":    "1970-01-01T00:00:00Z", // RFC3339 格式，已过期
			},
			wantRefresh: true,
		},
		{
			name: "expires_at as string - far future",
			credentials: map[string]any{
				"refresh_token": "rt-test",
				"expires_at":    "9999999999", // 远未来
			},
			wantRefresh: false,
		},
		{
			name: "expires_at as float64 - far future",
			credentials: map[string]any{
				"refresh_token": "rt-test",
				"expires_at":    float64(9999999999), // 远未来，数字类型
			},
			wantRefresh: false,
		},
		{
			name: "expires_at as RFC3339 - far future",
			credentials: map[string]any{
				"refresh_token": "rt-test",
				"expires_at":    "2099-12-31T23:59:59Z", // RFC3339 格式，远未来
			},
			wantRefresh: false,
		},
		{
			// 过期时间未知时刷新一次，避免旧 access_token 静默失效后只能拿到上游 401
			name: "expires_at missing - refresh to establish known state",
			credentials: map[string]any{
				"refresh_token": "rt-test",
			},
			wantRefresh: true,
		},
		{
			name: "expires_at is nil - refresh to establish known state",
			credentials: map[string]any{
				"refresh_token": "rt-test",
				"expires_at":    nil,
			},
			wantRefresh: true,
		},
		{
			name: "expires_at is invalid string - refresh to establish known state",
			credentials: map[string]any{
				"refresh_token": "rt-test",
				"expires_at":    "invalid",
			},
			wantRefresh: true,
		},
		{
			// 毫秒时间戳按秒解析会落到公元 5000 年之后，导致 token 永不刷新
			name: "expires_at as unix millis - expired",
			credentials: map[string]any{
				"refresh_token": "rt-test",
				"expires_at":    expiredMillis,
			},
			wantRefresh: true,
		},
		{
			name: "expires_at as unix millis - outside window",
			credentials: map[string]any{
				"refresh_token": "rt-test",
				"expires_at":    futureMillis,
			},
			wantRefresh: false,
		},
		{
			// 没有 refresh_token 就无从刷新，与 Grok/OpenAI 刷新器保持一致
			name: "no refresh_token - skipped even when expired",
			credentials: map[string]any{
				"expires_at": "1000",
			},
			wantRefresh: false,
		},
		{
			name:        "credentials is nil",
			credentials: nil,
			wantRefresh: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform:    PlatformAnthropic,
				Type:        AccountTypeOAuth,
				Credentials: tt.credentials,
			}

			got := refresher.NeedsRefresh(account, refreshWindow)
			require.Equal(t, tt.wantRefresh, got)
		})
	}
}

func TestClaudeTokenRefresher_NeedsRefresh_WithinWindow(t *testing.T) {
	refresher := &ClaudeTokenRefresher{}
	refreshWindow := 30 * time.Minute

	// 设置一个在刷新窗口内的时间（当前时间 + 15分钟）
	expiresAt := time.Now().Add(15 * time.Minute).Unix()

	tests := []struct {
		name        string
		credentials map[string]any
	}{
		{
			name: "string type - within refresh window",
			credentials: map[string]any{
				"refresh_token": "rt-test",
				"expires_at":    strconv.FormatInt(expiresAt, 10),
			},
		},
		{
			name: "float64 type - within refresh window",
			credentials: map[string]any{
				"refresh_token": "rt-test",
				"expires_at":    float64(expiresAt),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform:    PlatformAnthropic,
				Type:        AccountTypeOAuth,
				Credentials: tt.credentials,
			}

			got := refresher.NeedsRefresh(account, refreshWindow)
			require.True(t, got, "should need refresh when within window")
		})
	}
}

func TestClaudeTokenRefresher_NeedsRefresh_OutsideWindow(t *testing.T) {
	refresher := &ClaudeTokenRefresher{}
	refreshWindow := 30 * time.Minute

	// 设置一个在刷新窗口外的时间（当前时间 + 1小时）
	expiresAt := time.Now().Add(1 * time.Hour).Unix()

	tests := []struct {
		name        string
		credentials map[string]any
	}{
		{
			name: "string type - outside refresh window",
			credentials: map[string]any{
				"refresh_token": "rt-test",
				"expires_at":    strconv.FormatInt(expiresAt, 10),
			},
		},
		{
			name: "float64 type - outside refresh window",
			credentials: map[string]any{
				"refresh_token": "rt-test",
				"expires_at":    float64(expiresAt),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform:    PlatformAnthropic,
				Type:        AccountTypeOAuth,
				Credentials: tt.credentials,
			}

			got := refresher.NeedsRefresh(account, refreshWindow)
			require.False(t, got, "should not need refresh when outside window")
		})
	}
}

func TestClaudeTokenRefresher_CanRefresh(t *testing.T) {
	refresher := &ClaudeTokenRefresher{}

	tests := []struct {
		name     string
		platform string
		accType  string
		want     bool
	}{
		{
			name:     "anthropic oauth - can refresh",
			platform: PlatformAnthropic,
			accType:  AccountTypeOAuth,
			want:     true,
		},
		{
			name:     "anthropic setup-token - can refresh",
			platform: PlatformAnthropic,
			accType:  AccountTypeSetupToken,
			want:     true,
		},
		{
			name:     "anthropic api-key - cannot refresh",
			platform: PlatformAnthropic,
			accType:  AccountTypeAPIKey,
			want:     false,
		},
		{
			name:     "openai oauth - cannot refresh",
			platform: PlatformOpenAI,
			accType:  AccountTypeOAuth,
			want:     false,
		},
		{
			name:     "gemini oauth - cannot refresh",
			platform: PlatformGemini,
			accType:  AccountTypeOAuth,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform: tt.platform,
				Type:     tt.accType,
			}

			got := refresher.CanRefresh(account)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestOpenAITokenRefresher_CanRefresh(t *testing.T) {
	refresher := &OpenAITokenRefresher{}

	tests := []struct {
		name     string
		platform string
		accType  string
		want     bool
	}{
		{
			name:     "openai oauth - can refresh",
			platform: PlatformOpenAI,
			accType:  AccountTypeOAuth,
			want:     true,
		},
		{
			name:     "openai apikey - cannot refresh",
			platform: PlatformOpenAI,
			accType:  AccountTypeAPIKey,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{
				Platform: tt.platform,
				Type:     tt.accType,
			}
			require.Equal(t, tt.want, refresher.CanRefresh(account))
		})
	}
}
