package adobe

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/google/uuid"
)

// arpSessionMagic 是 x-arp-session-id 的 ftr 字段尾部的固定串（逆向所得，含义未知）。
const arpSessionMagic = "dUAL43-mnts-ants-d4_31ck__tt"

// defaultTokenExpirySkew 是判定 token 过期时预留的提前量。
const defaultTokenExpirySkew = 60 * time.Second

// DecodeJWTPayload 解出 IMS token 的 claims（JWT 第二段，base64url）。
// 任何解析失败都返回空 map 而非错误——调用方一律按「拿不到 claims」处理。
func DecodeJWTPayload(token string) map[string]any {
	raw := strings.TrimSpace(token)
	if raw == "" {
		return map[string]any{}
	}
	parts := strings.Split(raw, ".")
	if len(parts) < 2 {
		return map[string]any{}
	}
	payload := strings.TrimSpace(parts[1])
	if payload == "" {
		return map[string]any{}
	}
	// IMS 的 JWT 段可能带也可能不带 padding，统一去掉后按 Raw 解码。
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(payload, "="))
	if err != nil {
		return map[string]any{}
	}
	var claims map[string]any
	if err := json.Unmarshal(decoded, &claims); err != nil || claims == nil {
		return map[string]any{}
	}
	return claims
}

// adobeAuthIDPattern 匹配 IMS 账号 id 的 24 位十六进制前缀（不含 @AdobeID）。
var adobeAuthIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{24}$`)

// AccountIDFromToken 取 token 里的账号标识，依次尝试 user_id / aa_id / sub。
// 原样返回 claims 值——x-nonce 用裸 user_id 哈希，不要在这里补 @AdobeID。
func AccountIDFromToken(token string) string {
	claims := DecodeJWTPayload(token)
	for _, key := range []string{"user_id", "aa_id", "sub"} {
		if v := strings.TrimSpace(claimString(claims, key)); v != "" {
			return v
		}
	}
	return ""
}

// isGuestToken 判定 IMS 是否发了访客 token（user_id / aa_id / sub 以 @GuestID 结尾）。
func isGuestToken(token string) bool {
	id := strings.ToLower(AccountIDFromToken(token))
	return strings.HasSuffix(id, "@guestid")
}

// normalizeAdobeAccountID 把 24 位 hex 补成 `hex@AdobeID`。已带 @ 后缀的原样返回。
func normalizeAdobeAccountID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || strings.Contains(id, "@") {
		return id
	}
	if adobeAuthIDPattern.MatchString(id) {
		return strings.ToUpper(id) + "@AdobeID"
	}
	return id
}

// DecodeJWTExp 解出 token 的过期时间（秒级 Unix 时间戳）。
//
// 优先取 exp；没有 exp 时用 created_at + expires_in 推算——IMS 这两个字段既可能是
// 秒也可能是毫秒，按量级归一（created_at 超过 1e10 视为毫秒，expires_in 超过两天
// 视为毫秒）。无法判定时 ok 为 false。
func DecodeJWTExp(token string) (exp int64, ok bool) {
	claims := DecodeJWTPayload(token)
	if len(claims) == 0 {
		return 0, false
	}
	if v, found := claimInt(claims, "exp"); found {
		return v, true
	}

	createdAt, okCreated := claimInt(claims, "created_at")
	expiresIn, okExpires := claimInt(claims, "expires_in")
	if !okCreated || !okExpires || createdAt <= 0 || expiresIn <= 0 {
		return 0, false
	}
	if createdAt > 10_000_000_000 {
		createdAt /= 1000
	}
	if expiresIn > 86400*2 {
		expiresIn /= 1000
	}
	return createdAt + expiresIn, true
}

// IsTokenExpired 判定 token 是否已过期（含 skew 提前量）。
// 解不出过期时间时按「未过期」处理，把判定权留给上游的 401。
func IsTokenExpired(token string, skew time.Duration) bool {
	if skew <= 0 {
		skew = defaultTokenExpirySkew
	}
	exp, ok := DecodeJWTExp(token)
	if !ok {
		return false
	}
	return exp-int64(skew.Seconds()) <= time.Now().Unix()
}

// BuildSubmitNonce 构造提交请求的 x-nonce。
//
// Firefly 前端（clio-playground-web）提交 generate-async 时仍带该头。
// 算法：sha256("<user_id>-<prompt 前 256 字符>")。
// 缺 user_id 或 prompt 时返回空串（调用方此时不带该头）。
func BuildSubmitNonce(token, prompt string) string {
	userID := AccountIDFromToken(token)
	promptPrefix := truncateUTF16(prompt, 256)
	if userID == "" || promptPrefix == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(userID + "-" + promptPrefix))
	return hex.EncodeToString(sum[:])
}

// BuildARPSessionID 构造 x-arp-session-id。
//
// Firefly 前端提交 generate-async 时仍带该头。
// 形状：base64({"sid":<uuid>,"ftr":<指纹串>})。
// 含随机数与当前时间，故非确定性。
func BuildARPSessionID() string {
	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		// crypto/rand 失败时退化为时间派生值：这个字段只需唯一，不需要不可预测。
		randBytes = []byte(strconv.FormatInt(time.Now().UnixNano(), 16))
	}
	ftr := fmt.Sprintf("%s_%d_%d_%s",
		hex.EncodeToString(randBytes),
		time.Now().UnixMilli(),
		os.Getpid(),
		arpSessionMagic,
	)
	// 字段顺序需与上游前端一致，故用结构体而非 map。
	raw, err := json.Marshal(struct {
		SID string `json:"sid"`
		FTR string `json:"ftr"`
	}{SID: uuid.NewString(), FTR: ftr})
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// truncateUTF16 按 UTF-16 码元截断，复刻 JS 的 String.prototype.slice 语义。
//
// 这里不能用 Go 的字节切片：nonce 要与上游前端算出同一个值，而前端用的是 JS 的
// 码元计数。对纯 ASCII 两者一致，对 CJK/emoji 则不同。
func truncateUTF16(s string, limit int) string {
	if s == "" || limit <= 0 {
		return ""
	}
	units := utf16.Encode([]rune(s))
	if len(units) <= limit {
		return s
	}
	// 截断可能切开代理对，utf16.Decode 会把落单的代理项还原成 U+FFFD——
	// 与 JS 侧把落单代理写成 UTF-8 时的行为一致。
	return string(utf16.Decode(units[:limit]))
}

// claimString 取 claim 的字符串形式，兼容上游把数字型 id 写成 number 的情况。
func claimString(claims map[string]any, key string) string {
	switch v := claims[key].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

// claimInt 取 claim 的整数形式，兼容 number 与数字字符串两种写法。
func claimInt(claims map[string]any, key string) (int64, bool) {
	switch v := claims[key].(type) {
	case float64:
		return int64(v), true
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}
