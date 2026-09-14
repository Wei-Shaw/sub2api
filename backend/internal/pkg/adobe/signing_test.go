//go:build unit

package adobe

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// makeJWT 拼一个仅供解析用的 JWT（签名段是占位符）。
func makeJWT(t *testing.T, payload map[string]any) string {
	t.Helper()
	enc := func(v any) string {
		raw, err := json.Marshal(v)
		require.NoError(t, err)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	return enc(map[string]any{"alg": "none"}) + "." + enc(payload) + ".sig"
}

func TestDecodeJWTPayload(t *testing.T) {
	claims := DecodeJWTPayload(makeJWT(t, map[string]any{"user_id": "u123", "exp": 999}))
	require.Equal(t, "u123", claims["user_id"])
	require.Equal(t, float64(999), claims["exp"])

	require.Empty(t, DecodeJWTPayload(""))
	require.Empty(t, DecodeJWTPayload("notajwt"))
	require.Empty(t, DecodeJWTPayload("a.!!!not-base64!!!.c"))
}

func TestAccountIDFromToken(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
		want    string
	}{
		{"user_id 优先", map[string]any{"user_id": "u1"}, "u1"},
		{"24 位 hex 原样返回", map[string]any{"user_id": "8bb5048b5fa5386f0a495f88"}, "8bb5048b5fa5386f0a495f88"},
		{"已带后缀原样返回", map[string]any{"user_id": "8BB5048B5FA5386F0A495F88@AdobeID"}, "8BB5048B5FA5386F0A495F88@AdobeID"},
		{"回落 aa_id", map[string]any{"aa_id": "a1"}, "a1"},
		{"回落 sub", map[string]any{"sub": "s1"}, "s1"},
		{"都没有返回空", map[string]any{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, AccountIDFromToken(makeJWT(t, tt.payload)))
		})
	}
}

func TestIsGuestToken(t *testing.T) {
	require.True(t, isGuestToken(makeJWT(t, map[string]any{"user_id": "1_x@GuestID"})))
	require.True(t, isGuestToken(makeJWT(t, map[string]any{"user_id": "1_x@guestid"})))
	require.False(t, isGuestToken(makeJWT(t, map[string]any{"user_id": "8BB5048B5FA5386F0A495F88@AdobeID"})))
	require.False(t, isGuestToken(makeJWT(t, map[string]any{"user_id": "u1"})))
}

func TestNormalizeAdobeAccountID(t *testing.T) {
	require.Equal(t, "8BB5048B5FA5386F0A495F88@AdobeID",
		normalizeAdobeAccountID("8bb5048b5fa5386f0a495f88"))
	require.Equal(t, "8BB5048B5FA5386F0A495F88@AdobeID",
		normalizeAdobeAccountID("8BB5048B5FA5386F0A495F88@AdobeID"))
	require.Equal(t, "user-1", normalizeAdobeAccountID("user-1"))
	require.Equal(t, "", normalizeAdobeAccountID(""))
}

func TestDecodeJWTExp(t *testing.T) {
	t.Run("直接取 exp", func(t *testing.T) {
		exp, ok := DecodeJWTExp(makeJWT(t, map[string]any{"exp": 1771862511}))
		require.True(t, ok)
		require.Equal(t, int64(1771862511), exp)
	})

	t.Run("created_at + expires_in 按毫秒归一", func(t *testing.T) {
		exp, ok := DecodeJWTExp(makeJWT(t, map[string]any{
			"created_at": 1771862511913,
			"expires_in": 86400000,
		}))
		require.True(t, ok)
		require.Equal(t, int64(1771862511+86400), exp)
	})

	t.Run("秒级 created_at + expires_in 不做归一", func(t *testing.T) {
		exp, ok := DecodeJWTExp(makeJWT(t, map[string]any{
			"created_at": 1771862511,
			"expires_in": 3600,
		}))
		require.True(t, ok)
		require.Equal(t, int64(1771862511+3600), exp)
	})

	t.Run("数字写成字符串也能解", func(t *testing.T) {
		exp, ok := DecodeJWTExp(makeJWT(t, map[string]any{
			"created_at": "1771862511",
			"expires_in": "3600",
		}))
		require.True(t, ok)
		require.Equal(t, int64(1771862511+3600), exp)
	})

	t.Run("无可判定字段", func(t *testing.T) {
		_, ok := DecodeJWTExp(makeJWT(t, map[string]any{}))
		require.False(t, ok)
	})
}

func TestIsTokenExpired(t *testing.T) {
	// 解不出 exp 时按未过期处理，把判定权留给上游的 401。
	require.False(t, IsTokenExpired(makeJWT(t, map[string]any{}), 0))
	require.True(t, IsTokenExpired(makeJWT(t, map[string]any{"exp": 1}), 0))
	require.False(t, IsTokenExpired(
		makeJWT(t, map[string]any{"exp": time.Now().Add(time.Hour).Unix()}), 0))

	// skew 让「还有 30 秒过期」提前判定为已过期。
	soon := makeJWT(t, map[string]any{"exp": time.Now().Add(30 * time.Second).Unix()})
	require.False(t, IsTokenExpired(soon, time.Second))
	require.True(t, IsTokenExpired(soon, 5*time.Minute))
}

func TestBuildSubmitNonce(t *testing.T) {
	t.Run("sha256(user_id-prompt) 且确定性", func(t *testing.T) {
		token := makeJWT(t, map[string]any{"user_id": "u123"})
		sum := sha256.Sum256([]byte("u123-hello"))
		require.Equal(t, hex.EncodeToString(sum[:]), BuildSubmitNonce(token, "hello"))
		require.Equal(t, BuildSubmitNonce(token, "hello"), BuildSubmitNonce(token, "hello"))
	})

	t.Run("缺 user_id 或 prompt 返回空", func(t *testing.T) {
		require.Empty(t, BuildSubmitNonce(makeJWT(t, map[string]any{}), "hello"))
		require.Empty(t, BuildSubmitNonce(makeJWT(t, map[string]any{"user_id": "u"}), ""))
	})

	t.Run("hex user_id 不把 @AdobeID 算进 nonce", func(t *testing.T) {
		token := makeJWT(t, map[string]any{"user_id": "8bb5048b5fa5386f0a495f88"})
		sum := sha256.Sum256([]byte("8bb5048b5fa5386f0a495f88-hello"))
		require.Equal(t, hex.EncodeToString(sum[:]), BuildSubmitNonce(token, "hello"))
	})

	t.Run("prompt 超长时只取前 256 个 UTF-16 码元", func(t *testing.T) {
		token := makeJWT(t, map[string]any{"user_id": "u1"})
		long := strings.Repeat("a", 300)
		sum := sha256.Sum256([]byte("u1-" + strings.Repeat("a", 256)))
		require.Equal(t, hex.EncodeToString(sum[:]), BuildSubmitNonce(token, long))
	})
}

// UTF-16 码元计数是与上游前端对齐的关键：JS 的 slice 按码元切，Go 的字节切片会
// 在 CJK/emoji 上给出不同的前缀，导致 nonce 对不上。
func TestTruncateUTF16(t *testing.T) {
	require.Equal(t, "", truncateUTF16("", 10))
	require.Equal(t, "", truncateUTF16("abc", 0))
	require.Equal(t, "abc", truncateUTF16("abc", 10))
	require.Equal(t, "ab", truncateUTF16("abc", 2))

	// 中文每字 1 个码元（3 字节），按码元截断应保留 2 个字。
	require.Equal(t, "中文", truncateUTF16("中文测试", 2))

	// emoji 是代理对（2 个码元），limit=2 恰好保留完整一个。
	require.Equal(t, "😀", truncateUTF16("😀😀", 2))

	// limit=1 切开代理对，落单代理还原成 U+FFFD——与 JS 侧转 UTF-8 的行为一致。
	require.Equal(t, "�", truncateUTF16("😀", 1))
}

func TestBuildARPSessionID(t *testing.T) {
	raw, err := base64.StdEncoding.DecodeString(BuildARPSessionID())
	require.NoError(t, err)

	var parsed struct {
		SID string `json:"sid"`
		FTR string `json:"ftr"`
	}
	require.NoError(t, json.Unmarshal(raw, &parsed))
	require.Regexp(t,
		regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`),
		parsed.SID)
	require.Contains(t, parsed.FTR, arpSessionMagic)

	// 字段顺序需与上游前端一致。
	require.True(t, strings.HasPrefix(string(raw), `{"sid":`))
	require.NotEqual(t, BuildARPSessionID(), BuildARPSessionID())
}
