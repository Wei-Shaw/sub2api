//go:build unit

package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

const kiroTestValidProfileArn = "arn:aws:codewhisperer:us-east-1:123456789012:profile/abcdef123456"

// TestDiscoverProfileArnFindsProfileOnFirstRegion 覆盖最常见的成功路径：
// 第一个候选区域（us-east-1）就直接返回一条 profile，不需要再探测第二个
// 候选区域。
func TestDiscoverProfileArnFindsProfileOnFirstRegion(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"profiles": []map[string]string{
				{"arn": kiroTestValidProfileArn, "profileName": "default"},
			},
		})
	}))
	t.Cleanup(srv.Close)

	svc := NewKiroOAuthService(nil)
	t.Cleanup(svc.Stop)
	svc.listProfilesHost = func(string) string { return srv.URL }

	arn, err := svc.DiscoverProfileArn(t.Context(), "test-token", "machine-1", nil)
	require.NoError(t, err)
	require.Equal(t, kiroTestValidProfileArn, arn)
	require.EqualValues(t, 1, calls, "命中第一个候选区域就应该停止，不再探测第二个")
}

// TestDiscoverProfileArnFallsThroughToSecondRegion 覆盖第一个候选区域没有
// 可用 profile（空列表，不是错误）时必须继续探测第二个候选区域。
func TestDiscoverProfileArnFallsThroughToSecondRegion(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
		if calls == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{"profiles": []map[string]string{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"profiles": []map[string]string{{"arn": kiroTestValidProfileArn, "profileName": "default"}},
		})
	}))
	t.Cleanup(srv.Close)

	svc := NewKiroOAuthService(nil)
	t.Cleanup(svc.Stop)
	svc.listProfilesHost = func(string) string { return srv.URL }

	arn, err := svc.DiscoverProfileArn(t.Context(), "test-token", "machine-1", nil)
	require.NoError(t, err)
	require.Equal(t, kiroTestValidProfileArn, arn)
	require.EqualValues(t, 2, calls, "第一个候选区域没有可用 profile 时必须换第二个候选区域")
}

// TestDiscoverProfileArnFollowsPagination 覆盖翻页：第一页没有 profile 但
// 带 nextToken，必须继续请求下一页而不是直接当成"这个区域没有"。
func TestDiscoverProfileArnFollowsPagination(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			NextToken string `json:"nextToken"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusOK)
		if body.NextToken == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"profiles":  []map[string]string{},
				"nextToken": "page-2",
			})
			return
		}
		require.Equal(t, "page-2", body.NextToken)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"profiles": []map[string]string{{"arn": kiroTestValidProfileArn, "profileName": "default"}},
		})
	}))
	t.Cleanup(srv.Close)

	svc := NewKiroOAuthService(nil)
	t.Cleanup(svc.Stop)
	svc.listProfilesHost = func(string) string { return srv.URL }

	arn, err := svc.DiscoverProfileArn(t.Context(), "test-token", "machine-1", nil)
	require.NoError(t, err)
	require.Equal(t, kiroTestValidProfileArn, arn)
	require.EqualValues(t, 2, calls, "带 nextToken 的第一页必须触发第二页请求，都发生在同一个区域内")
}

// TestDiscoverProfileArnReturnsEmptyWithoutErrorWhenNoProfilesAnywhere 覆盖
// 两个候选区域都探测成功但都没有可用 profile 的情况——这不是错误，只是
// "没发现"，调用方（KiroTokenRefresher.Refresh / discoverKiroProfileArnIfMissing）
// 据此决定保留手填入口，不应该把这种情况当成硬失败。
func TestDiscoverProfileArnReturnsEmptyWithoutErrorWhenNoProfilesAnywhere(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"profiles": []map[string]string{}})
	}))
	t.Cleanup(srv.Close)

	svc := NewKiroOAuthService(nil)
	t.Cleanup(svc.Stop)
	svc.listProfilesHost = func(string) string { return srv.URL }

	arn, err := svc.DiscoverProfileArn(t.Context(), "test-token", "machine-1", nil)
	require.NoError(t, err)
	require.Empty(t, arn)
}

// TestDiscoverProfileArnSurfacesErrorWhenAllRegionsFail 覆盖所有候选区域
// 都真的失败（比如真实场景里 Builder ID 账号被 AWS 拒绝这个操作）的情况——
// 这时应该返回错误，而不是安静地当成"没发现"，方便调用方区分"探测正常
// 完成但没有 profile"和"探测本身没跑通"两种不同情况（虽然当前两个调用点
// 目前都选择把错误降级成 debug 日志，不阻断主流程，但这个区分本身应该
// 由 DiscoverProfileArn 忠实反映，不能在这一层就抹平）。
func TestDiscoverProfileArnSurfacesErrorWhenAllRegionsFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"AWS Builder ID is not supported for this operation"}`))
	}))
	t.Cleanup(srv.Close)

	svc := NewKiroOAuthService(nil)
	t.Cleanup(svc.Stop)
	svc.listProfilesHost = func(string) string { return srv.URL }

	arn, err := svc.DiscoverProfileArn(t.Context(), "test-token", "machine-1", nil)
	require.Error(t, err)
	require.Empty(t, arn)
}

func TestDiscoverProfileArnRejectsEmptyAccessToken(t *testing.T) {
	svc := NewKiroOAuthService(nil)
	t.Cleanup(svc.Stop)

	_, err := svc.DiscoverProfileArn(t.Context(), "", "machine-1", nil)
	require.Error(t, err)
}

// TestDiscoverProfileArnDropsInvalidArnAndKeepsLooking 覆盖上游返回了一条
// 格式不合法的 ARN（不应该发生，但不能因为一条脏数据就崩掉整个发现流程）
// 的情况——ParseListProfilesResponse 已经在协议层测过过滤逻辑本身，这里
// 补一层集成断言：过滤之后如果这一页/这个区域就没有合法 profile 了，必须
// 老实当成"没找到"继续换区域，而不是把无效 ARN 直接返回给调用方。
func TestDiscoverProfileArnDropsInvalidArnAndKeepsLooking(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
		if calls == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"profiles": []map[string]string{{"arn": "not-a-real-arn", "profileName": "garbage"}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"profiles": []map[string]string{{"arn": kiroTestValidProfileArn, "profileName": "default"}},
		})
	}))
	t.Cleanup(srv.Close)

	svc := NewKiroOAuthService(nil)
	t.Cleanup(svc.Stop)
	svc.listProfilesHost = func(string) string { return srv.URL }

	arn, err := svc.DiscoverProfileArn(t.Context(), "test-token", "machine-1", nil)
	require.NoError(t, err)
	require.Equal(t, kiroTestValidProfileArn, arn)
}
