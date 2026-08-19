//go:build unit

package service

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type zhipuRateLimitRepoStub struct {
	mockAccountRepoForGemini
	rateLimitedID int64
	resetAt       time.Time
}

func (r *zhipuRateLimitRepoStub) SetRateLimited(_ context.Context, id int64, resetAt time.Time) error {
	r.rateLimitedID = id
	r.resetAt = resetAt
	return nil
}

func TestParseZhipuRateLimitResetTime(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantTime string
	}{
		{
			name:     "code 1308 5小时限额",
			body:     `{"type":"error","error":{"type":"rate_limit_error","code":"1308","message":"[1308][已达到 5 小时的使用上限。您的限额将在 2026-08-06 18:42:29 重置。][202608061831009dcd1382523549e4]"},"request_id":"202608061831009dcd1382523549e4"}`,
			wantTime: "2026-08-06 18:42:29",
		},
		{
			name:     "code 1310 月度限额",
			body:     `{"type":"error","error":{"type":"rate_limit_error","code":"1310","message":"[1310][您已达到每周/每月使用上限，您的限额将在 2026-08-05 15:16:26 重置。][foo]"},"request_id":"123"}`,
			wantTime: "2026-08-05 15:16:26",
		},
		{
			name:     "无智谱错误码",
			body:     `{"type":"error","error":{"type":"rate_limit_error","code":"9999","message":"[9999][其它错误]"},"request_id":"123"}`,
			wantTime: "",
		},
	}

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.Local
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseZhipuRateLimitResetTime([]byte(tt.body))
			if tt.wantTime == "" {
				require.Nil(t, got)
			} else {
				require.NotNil(t, got)
				expectedTime, err := time.ParseInLocation("2006-01-02 15:04:05", tt.wantTime, loc)
				require.NoError(t, err)
				require.Equal(t, expectedTime.Unix(), *got)
			}
		})
	}
}

func TestRateLimitServiceHandle429_ZhipuPersistsParsedResetTime(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{name: "code 1308", code: "1308"},
		{name: "code 1310", code: "1310"},
	}

	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expected := time.Now().In(loc).Add(2 * time.Hour).Truncate(time.Second)
			body := fmt.Sprintf(`{"error":{"code":"%s","message":"[%s][您的限额将在 %s 重置。][foo]"}}`, tt.code, tt.code, expected.Format("2006-01-02 15:04:05"))
			repo := &zhipuRateLimitRepoStub{}
			svc := NewRateLimitService(repo, nil, nil, nil, nil)
			account := &Account{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeAPIKey}

			svc.handle429(context.Background(), account, http.Header{}, []byte(body))

			require.Equal(t, account.ID, repo.rateLimitedID)
			require.Equal(t, expected.Unix(), repo.resetAt.Unix())
		})
	}
}
