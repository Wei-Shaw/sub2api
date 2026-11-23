package repository

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func zstdAccountsWith(ids ...int64) *upstreamZstdAccounts {
	r := &upstreamZstdAccounts{}
	m := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	r.ids.Store(&m)
	r.loadedAt.Store(time.Now().UnixNano())
	return r
}

// 账号打开了开关就压，不需要 host 在列表里；别的账号照常原样发。
func TestHTTPUpstreamZstdRequestBodyAccountSwitch(t *testing.T) {
	srv, got := newZstdUpstream(t, false)
	s := zstdTestService(t, nil)
	s.zstdAccounts = zstdAccountsWith(7)
	body := compressibleBody(64 << 10)

	for _, accountID := range []int64{7, 8} {
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/messages", bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		resp, err := s.Do(req, "", accountID, 1)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}

	require.Equal(t, []string{"zstd", ""}, got.encoding)
	require.Less(t, got.wire[0], int64(len(body))/2)
	require.Equal(t, int64(len(body)), got.wire[1])
	require.Equal(t, body, got.decoded[0])
	require.Equal(t, body, got.decoded[1])
}

// 账号测试勾选了压缩：几百字节的 JSON 也压（正常门槛 16KB 一个都不会压），并记到探针里；
// 同一个 ctx 下的表单请求（比如 token 刷新）不压。
func TestHTTPUpstreamZstdRequestBodyProbeForcesSmallJSON(t *testing.T) {
	srv, got := newZstdUpstream(t, false)
	s := zstdTestService(t, nil)
	probe := &service.UpstreamRequestZstdProbe{Compress: true}
	ctx := service.WithUpstreamRequestZstdProbe(context.Background(), probe)

	jsonBody := []byte(`{"model":"deepseek-v4.1-flash","max_tokens":16,"messages":[{"role":"user","content":"hi"}],"stream":true}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/v1/messages", bytes.NewReader(jsonBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.Do(req, "", 0, 1)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	form := []byte("grant_type=refresh_token&refresh_token=abc")
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/oauth/token", bytes.NewReader(form))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err = s.Do(req, "", 0, 1)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	require.Equal(t, []string{"zstd", ""}, got.encoding)
	require.Equal(t, jsonBody, got.decoded[0], "upstream sub2api decoder must restore the exact body")
	require.Equal(t, form, got.decoded[1])

	attempts := probe.Attempts()
	require.Len(t, attempts, 1)
	require.Equal(t, hostOf(t, srv.URL), strings.Split(attempts[0].Host, ":")[0])
	require.Equal(t, len(jsonBody), attempts[0].RawBytes)
	require.Equal(t, int(got.wire[0]), attempts[0].WireBytes)
}

// 不带探针时，小请求体照旧不压，探针相关逻辑不影响正常路径。
func TestHTTPUpstreamZstdRequestBodyAccountSwitchKeepsThreshold(t *testing.T) {
	srv, got := newZstdUpstream(t, false)
	s := zstdTestService(t, nil)
	s.zstdAccounts = zstdAccountsWith(7)

	small := compressibleBody(4 << 10)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/messages", bytes.NewReader(small))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.Do(req, "", 7, 1)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	require.Equal(t, []string{""}, got.encoding)
}

func TestUpstreamZstdAccountsRefresh(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	query := regexp.QuoteMeta(upstreamZstdAccountsQuery)

	r := &upstreamZstdAccounts{db: db}
	mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)).AddRow(int64(3)))
	r.refresh(context.Background())
	require.True(t, r.enabled(42))
	require.True(t, r.enabled(3))
	require.False(t, r.enabled(4))

	// 查询失败时保留旧列表。
	mock.ExpectQuery(query).WillReturnError(errors.New("connection refused"))
	r.refresh(context.Background())
	require.True(t, r.enabled(42))

	// 列表过期后 enabled 在后台刷新：关掉开关的账号随之失效。
	mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(3)))
	r.loadedAt.Store(time.Now().Add(-2 * upstreamZstdAccountsRefreshInterval).UnixNano())
	_ = r.enabled(42)
	require.Eventually(t, func() bool { return !r.enabled(42) && r.enabled(3) }, 2*time.Second, 10*time.Millisecond)
	require.NoError(t, mock.ExpectationsWereMet())
}

// 启动时同步加载一次，开了开关的账号从第一个请求起就压缩。
func TestProvideHTTPUpstreamLoadsZstdAccounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectQuery(regexp.QuoteMeta(upstreamZstdAccountsQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)))

	s, ok := ProvideHTTPUpstream(&config.Config{}, db).(*httpUpstreamService)
	require.True(t, ok)
	require.True(t, s.wantsZstdRequestBody("relay.example", 42))
	require.False(t, s.wantsZstdRequestBody("relay.example", 43))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamZstdAccountsNilSafe(t *testing.T) {
	var r *upstreamZstdAccounts
	require.False(t, r.enabled(1))
	require.NotNil(t, ProvideHTTPUpstream(nil, nil))
}
