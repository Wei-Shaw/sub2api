package service

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// newGinContextWithUserID 构造一个带 SubjectUserID 的 *gin.Context，用于注入测试。
func newGinContextWithUserID(userID int64) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("POST", "/v1/messages", nil)
	c.Request = req
	if userID != 0 {
		c.Set(string(ctxkey.SubjectUserID), userID)
	}
	return c
}

// newTestGatewayServiceForInject 构造仅带 inject 相关依赖的 GatewayService，
// 避免拉起 wire 全图。其它字段保持 nil；inject 路径只读 cfg.Gateway 和 identityProfileService。
func newTestGatewayServiceForInject(injectEnabled bool, secret string) *GatewayService {
	cfg := &config.Config{}
	cfg.Gateway.IdentityProfileInjectEnabled = injectEnabled
	return &GatewayService{
		cfg:                    cfg,
		identityProfileService: NewIdentityProfileService(secret, 14),
	}
}

func newTestOpenAIGatewayServiceForInject(injectEnabled bool, secret string) *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Gateway.IdentityProfileInjectEnabled = injectEnabled
	return &OpenAIGatewayService{
		cfg:                    cfg,
		identityProfileService: NewIdentityProfileService(secret, 14),
	}
}

// TestResolveSubjectUserID 验证 ctx 路径与 gin.Context 路径都能取到 user_id。
func TestResolveSubjectUserID(t *testing.T) {
	t.Run("nil ctx 返回 0", func(t *testing.T) {
		require.Equal(t, int64(0), resolveSubjectUserID(nil))
	})

	t.Run("c.Get 路径", func(t *testing.T) {
		c := newGinContextWithUserID(42)
		require.Equal(t, int64(42), resolveSubjectUserID(c))
	})

	t.Run("Request.Context 路径", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("POST", "/v1/messages", nil)
		req = req.WithContext(context.WithValue(req.Context(), ctxkey.SubjectUserID, int64(99)))
		c.Request = req
		require.Equal(t, int64(99), resolveSubjectUserID(c))
	})

	t.Run("缺失返回 0", func(t *testing.T) {
		c := newGinContextWithUserID(0)
		require.Equal(t, int64(0), resolveSubjectUserID(c))
	})

	t.Run("非 int64 类型 返回 0", func(t *testing.T) {
		c := newGinContextWithUserID(0)
		c.Set(string(ctxkey.SubjectUserID), "not-an-int64")
		require.Equal(t, int64(0), resolveSubjectUserID(c))
	})
}

// TestApplyIdentityProfileToAnthropicBody_Disabled 当 inject 关闭时必须 noop。
func TestApplyIdentityProfileToAnthropicBody_Disabled(t *testing.T) {
	svc := newTestGatewayServiceForInject(false, testIdentityProfileSecret)
	c := newGinContextWithUserID(42)

	// 新格式 metadata.user_id（JSON）
	body := []byte(`{"metadata":{"user_id":"{\"device_id\":\"aaaa\",\"account_uuid\":\"acc-uuid\",\"session_id\":\"sess-uuid\"}"}}`)
	out := svc.applyIdentityProfileToAnthropicBody(c, body)
	require.Equal(t, string(body), string(out), "inject 关闭时必须返回原 body")
}

// TestApplyIdentityProfileToAnthropicBody_NoUserID userID==0 时也应 noop。
func TestApplyIdentityProfileToAnthropicBody_NoUserID(t *testing.T) {
	svc := newTestGatewayServiceForInject(true, testIdentityProfileSecret)
	c := newGinContextWithUserID(0) // 没写 user_id

	body := []byte(`{"metadata":{"user_id":"{\"device_id\":\"aaaa\",\"account_uuid\":\"acc-uuid\",\"session_id\":\"sess-uuid\"}"}}`)
	out := svc.applyIdentityProfileToAnthropicBody(c, body)
	require.Equal(t, string(body), string(out), "userID==0 时必须返回原 body")
}

// TestApplyIdentityProfileToAnthropicBody_NewFormat 新格式 JSON 应只覆盖 device_id。
func TestApplyIdentityProfileToAnthropicBody_NewFormat(t *testing.T) {
	svc := newTestGatewayServiceForInject(true, testIdentityProfileSecret)
	c := newGinContextWithUserID(42)

	originalDevice := "0000000000000000000000000000000000000000000000000000000000000000"
	originalSession := "11111111-1111-1111-1111-111111111111"
	originalUUID := "acc-aaaa-bbbb"
	body := []byte(`{"messages":[{"role":"user","content":"hi"}],"metadata":{"user_id":"{\"device_id\":\"` + originalDevice + `\",\"account_uuid\":\"` + originalUUID + `\",\"session_id\":\"` + originalSession + `\"}"}}`)

	out := svc.applyIdentityProfileToAnthropicBody(c, body)
	require.NotEqual(t, string(body), string(out), "inject 启用 + 有效 userID 应改写 body")

	rewritten := gjson.GetBytes(out, "metadata.user_id").String()
	parsed := ParseMetadataUserID(rewritten)
	require.NotNil(t, parsed)

	// account_uuid + session_id 必须保留
	require.Equal(t, originalUUID, parsed.AccountUUID, "account_uuid 必须保留")
	require.Equal(t, originalSession, parsed.SessionID, "session_id 必须保留")

	// device_id 应为 64 hex 且不等于原值
	require.Len(t, parsed.DeviceID, 64)
	require.NotEqual(t, originalDevice, parsed.DeviceID, "device_id 应被覆盖")
	for _, ch := range parsed.DeviceID {
		require.True(t,
			(ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f'),
			"device_id 应为 hex",
		)
	}

	// messages 字段必须保留（不能误改）
	require.Equal(t, "hi", gjson.GetBytes(out, "messages.0.content").String())
}

// TestApplyIdentityProfileToAnthropicBody_LegacyFormat 旧格式（user_xx_account_yy_session_zz）应保持旧格式。
func TestApplyIdentityProfileToAnthropicBody_LegacyFormat(t *testing.T) {
	svc := newTestGatewayServiceForInject(true, testIdentityProfileSecret)
	c := newGinContextWithUserID(42)

	hex64 := "0000000000000000000000000000000000000000000000000000000000000000"
	uuid := "11111111-1111-1111-1111-111111111111"
	body := []byte(`{"metadata":{"user_id":"user_` + hex64 + `_account__session_` + uuid + `"}}`)

	out := svc.applyIdentityProfileToAnthropicBody(c, body)

	rewritten := gjson.GetBytes(out, "metadata.user_id").String()
	require.True(t, strings.HasPrefix(rewritten, "user_"), "旧格式应保持 user_ 前缀")
	require.Contains(t, rewritten, "_session_"+uuid, "session_id 必须保留")

	parsed := ParseMetadataUserID(rewritten)
	require.NotNil(t, parsed)
	require.False(t, parsed.IsNewFormat, "应保持旧格式")
	require.NotEqual(t, hex64, parsed.DeviceID, "device_id 应被覆盖")
	require.Len(t, parsed.DeviceID, 64)
}

// TestApplyIdentityProfileToAnthropicBody_InvalidUserID metadata.user_id 解析失败应 noop。
func TestApplyIdentityProfileToAnthropicBody_InvalidUserID(t *testing.T) {
	svc := newTestGatewayServiceForInject(true, testIdentityProfileSecret)
	c := newGinContextWithUserID(42)

	cases := []string{
		`{"metadata":{"user_id":"random-junk"}}`,                 // 既不是 JSON 也不是 legacy
		`{"metadata":{"user_id":""}}`,                            // 空
		`{"metadata":{}}`,                                        // 无 user_id
		`{}`,                                                     // 无 metadata
		`{"metadata":{"user_id":"{\"device_id\":\"\"}"}}`,        // JSON 但 device_id 空
	}
	for i, body := range cases {
		out := svc.applyIdentityProfileToAnthropicBody(c, []byte(body))
		require.Equal(t, body, string(out), "case %d: should be noop on invalid input", i)
	}
}

// TestApplyIdentityProfileToAnthropicBody_DeterministicAcrossCalls 同 user×同时刻多次调用应得到相同 device_id。
func TestApplyIdentityProfileToAnthropicBody_DeterministicAcrossCalls(t *testing.T) {
	svc := newTestGatewayServiceForInject(true, testIdentityProfileSecret)
	c := newGinContextWithUserID(42)

	originalDevice := "0000000000000000000000000000000000000000000000000000000000000000"
	body := []byte(`{"metadata":{"user_id":"{\"device_id\":\"` + originalDevice + `\",\"account_uuid\":\"acc-1\",\"session_id\":\"11111111-1111-1111-1111-111111111111\"}"}}`)

	out1 := svc.applyIdentityProfileToAnthropicBody(c, body)
	out2 := svc.applyIdentityProfileToAnthropicBody(c, body)
	require.Equal(t, string(out1), string(out2), "同一用户同一窗口应得到相同改写结果")
}

// TestApplyIdentityProfileToAnthropicBody_DifferentUsersGetDifferentDeviceIDs 不同 user 应得到不同 device_id。
func TestApplyIdentityProfileToAnthropicBody_DifferentUsersGetDifferentDeviceIDs(t *testing.T) {
	svc := newTestGatewayServiceForInject(true, testIdentityProfileSecret)

	body := []byte(`{"metadata":{"user_id":"{\"device_id\":\"0000000000000000000000000000000000000000000000000000000000000000\",\"account_uuid\":\"acc-1\",\"session_id\":\"11111111-1111-1111-1111-111111111111\"}"}}`)

	out1 := svc.applyIdentityProfileToAnthropicBody(newGinContextWithUserID(1), body)
	out2 := svc.applyIdentityProfileToAnthropicBody(newGinContextWithUserID(2), body)

	d1 := ParseMetadataUserID(gjson.GetBytes(out1, "metadata.user_id").String())
	d2 := ParseMetadataUserID(gjson.GetBytes(out2, "metadata.user_id").String())
	require.NotNil(t, d1)
	require.NotNil(t, d2)
	require.NotEqual(t, d1.DeviceID, d2.DeviceID, "不同 user 应得到不同 device_id")
}

// TestStableOpenAISessionUserSeed 验证 OpenAI 用户种子。
func TestStableOpenAISessionUserSeed(t *testing.T) {
	t.Run("inject 关闭返回空", func(t *testing.T) {
		svc := newTestOpenAIGatewayServiceForInject(false, testIdentityProfileSecret)
		c := newGinContextWithUserID(42)
		require.Empty(t, svc.stableOpenAISessionUserSeed(c))
	})

	t.Run("inject 启用 + userID==0 返回空", func(t *testing.T) {
		svc := newTestOpenAIGatewayServiceForInject(true, testIdentityProfileSecret)
		c := newGinContextWithUserID(0)
		require.Empty(t, svc.stableOpenAISessionUserSeed(c))
	})

	t.Run("inject 启用 + 有效 user 返回 32 hex MachineID", func(t *testing.T) {
		svc := newTestOpenAIGatewayServiceForInject(true, testIdentityProfileSecret)
		c := newGinContextWithUserID(42)
		seed := svc.stableOpenAISessionUserSeed(c)
		require.Len(t, seed, 32, "MachineID 必须是 32 hex")
	})

	t.Run("identityProfileService 为 nil 时返回空", func(t *testing.T) {
		svc := &OpenAIGatewayService{
			cfg: &config.Config{
				Gateway: config.GatewayConfig{IdentityProfileInjectEnabled: true},
			},
			identityProfileService: nil,
		}
		c := newGinContextWithUserID(42)
		require.Empty(t, svc.stableOpenAISessionUserSeed(c))
	})

	t.Run("同 user 多次调用幂等", func(t *testing.T) {
		svc := newTestOpenAIGatewayServiceForInject(true, testIdentityProfileSecret)
		c := newGinContextWithUserID(42)
		a := svc.stableOpenAISessionUserSeed(c)
		b := svc.stableOpenAISessionUserSeed(c)
		require.Equal(t, a, b)
	})

	t.Run("不同 user 得到不同种子", func(t *testing.T) {
		svc := newTestOpenAIGatewayServiceForInject(true, testIdentityProfileSecret)
		a := svc.stableOpenAISessionUserSeed(newGinContextWithUserID(1))
		b := svc.stableOpenAISessionUserSeed(newGinContextWithUserID(2))
		require.NotEqual(t, a, b)
	})
}

// TestIsolateOpenAISessionIDWithUserSeed 验证 P0-3 §4.4 task 2 的 session 种子注入。
func TestIsolateOpenAISessionIDWithUserSeed(t *testing.T) {
	t.Run("空 userSeed 与 isolateOpenAISessionID 等价", func(t *testing.T) {
		legacy := isolateOpenAISessionID(123, "session-abc")
		withEmpty := isolateOpenAISessionIDWithUserSeed(123, "", "session-abc")
		require.Equal(t, legacy, withEmpty, "userSeed=\"\" 必须向后兼容")
	})

	t.Run("userSeed 改变结果", func(t *testing.T) {
		a := isolateOpenAISessionIDWithUserSeed(123, "user-a", "session-abc")
		b := isolateOpenAISessionIDWithUserSeed(123, "user-b", "session-abc")
		require.NotEqual(t, a, b, "不同 userSeed 应得到不同结果")
	})

	t.Run("apiKeyID 变了也不能撞", func(t *testing.T) {
		a := isolateOpenAISessionIDWithUserSeed(1, "user-a", "session-abc")
		b := isolateOpenAISessionIDWithUserSeed(2, "user-a", "session-abc")
		require.NotEqual(t, a, b, "不同 apiKeyID 应得到不同结果（即使 userSeed 相同）")
	})

	t.Run("空 raw 返回空", func(t *testing.T) {
		require.Empty(t, isolateOpenAISessionIDWithUserSeed(123, "user-a", ""))
		require.Empty(t, isolateOpenAISessionIDWithUserSeed(123, "user-a", "   "))
	})

	t.Run("结果长度 16 hex", func(t *testing.T) {
		out := isolateOpenAISessionIDWithUserSeed(123, "user-a", "session-abc")
		require.Len(t, out, 16, "isolateOpenAISessionID 输出必须是 16 hex")
	})

	t.Run("幂等", func(t *testing.T) {
		a := isolateOpenAISessionIDWithUserSeed(123, "user-a", "session-abc")
		b := isolateOpenAISessionIDWithUserSeed(123, "user-a", "session-abc")
		require.Equal(t, a, b)
	})
}

// TestIdentityProfileInjectEnabled_NilSafe 验证 nil cfg 时不 panic。
func TestIdentityProfileInjectEnabled_NilSafe(t *testing.T) {
	require.NotPanics(t, func() {
		var s *GatewayService
		_ = s
		// 直接访问需要避免空指针；通过 service-level helper 调用
		gs := &GatewayService{cfg: nil}
		require.False(t, gs.identityProfileInjectEnabled())
	})
}

// 用一个不会被使用的 time.Time 引用确保上面的测试导入了 time 包（避免编辑器整理掉 import）。
var _ = time.Now
