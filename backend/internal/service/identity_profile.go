package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
)

// IdentityProfile 是 P0-3 中为 (sub2api_user, platform) 维护的伪指纹画像。
//
// 设计目的：让同一 sub2api 用户在某个平台上的连续请求，对外呈现一致的
// machine_id / os / arch / locale / timezone / cli_version 组合。理想状态是
// 一个上游账号被同一组「真实设备特征」长期使用，而不是每次请求都暴露不同
// 的客户端组合（这是 sub2api 此前最容易暴露代理身份的地方）。
//
// 字段语义（按 hot-path 注入位点划分）：
//   - MachineID（32 hex / 16 字节）：用于 OpenAI originator / session_id 派生。
//   - DeviceIDHex64（64 hex / 32 字节）：用于 Anthropic metadata.user_id 中
//     device_id 段；该段历史上一直是 64 hex，所以这里给一个独立长度的派生而不
//     是把 MachineID 拼接两次。
//   - OS / Arch / Locale / Timezone / UserAgentVersion：候选池采样，用于未来
//     X-Stainless-* 等头注入（当前不直接落到 hot-path）。
//
// 注意：本结构体目前仅用于读取层（hot-path 注入由 P0-3 §4.4 切片完成）。所有
// 字段都是从候选池里通过 HMAC 派生的「拟真」候选，不是从数据库查出来的真实
// 客户端值。
type IdentityProfile struct {
	UserID           int64
	Platform         string
	MachineID        string
	DeviceIDHex64    string
	OS               string
	Arch             string
	Locale           string
	Timezone         string
	UserAgentVersion string
	RotationSalt     string
	GeneratedAt      time.Time
}

// IdentityProfileService 按 (user_id, platform, rotation_salt) 维度生成稳定的
// 伪指纹画像。所有派生都是确定性的（HMAC + 候选池索引），不依赖数据库；
// secret 来源是 cfg.JWT.Secret（由 security_secrets 表 bootstrap），即使重启
// 也保持不变。
type IdentityProfileService struct {
	secret       []byte
	rotationDays int
}

// 候选池（exported via lower-case 包级变量给测试断言用）。值都是真实客户端
// 上常见的字符串；池子大小≥4 即可，对外无意义巨大池反而会让我们的指纹
// 看上去不像真实设备。
var (
	identityProfileOSPool = []string{
		"MacOS",
		"Linux",
		"Windows",
	}
	identityProfileArchPool = []string{
		"arm64",
		"x64",
	}
	identityProfileLocalePool = []string{
		"en-US",
		"en-GB",
		"zh-CN",
		"ja-JP",
		"de-DE",
		"fr-FR",
	}
	identityProfileTimezonePool = []string{
		"America/Los_Angeles",
		"America/New_York",
		"Europe/London",
		"Europe/Berlin",
		"Asia/Tokyo",
		"Asia/Shanghai",
		"Asia/Singapore",
		"Australia/Sydney",
	}
)

const (
	// identityProfileRotationDaysDefault 是当外部传入 ttlDays<=0 时使用的兜底
	// 旋转周期（天）。30 天与「真实开发机器很少彻底换设备指纹」的经验一致。
	identityProfileRotationDaysDefault = 30

	// identityProfileSecretFallback 仅用于配置层尚未 bootstrap secret 的极端
	// 情况；它本身不能脱密任何数据，只是让 hash 在 zero-input 时仍然稳定。
	identityProfileSecretFallback = "sub2api-identity-profile-fallback-v1"
)

// NewIdentityProfileService 构造一个 IdentityProfileService。
//
//   - secret：HMAC 派生用密钥，建议直接传 cfg.JWT.Secret（bootstrap 后稳定）。
//     传空串时退化到内部 fallback 常量；这只影响"集群间是否共享同一指纹"，
//     不影响进程内的稳定性。
//   - rotationDays：profile 多久旋转一次（天）。<=0 时按 30 天兜底。
//     传 ltb_ttl_days 即可让 P0-2 / P0-3 的旋转节奏一致。
func NewIdentityProfileService(secret string, rotationDays int) *IdentityProfileService {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		trimmed = identityProfileSecretFallback
	}
	if rotationDays <= 0 {
		rotationDays = identityProfileRotationDaysDefault
	}
	return &IdentityProfileService{
		secret:       []byte(trimmed),
		rotationDays: rotationDays,
	}
}

// Profile 返回 (userID, platform, now) 在当前 rotation 窗口下的伪指纹画像。
// 输出在同一窗口内严格幂等。
func (s *IdentityProfileService) Profile(userID int64, platform string, now time.Time) IdentityProfile {
	plat := strings.TrimSpace(platform)
	if plat == "" {
		plat = "unknown"
	}
	rotationSalt := s.rotationSalt(now)
	seed := fmt.Sprintf("identity-profile|%d|%s|%s", userID, plat, rotationSalt)

	machineID := s.deriveHexID(seed, "machine", 16)
	deviceIDHex64 := s.deriveHexID(seed, "device_id_hex64", 32)
	osValue := pickFromPool(s.deriveUint32(seed, "os"), identityProfileOSPool)
	arch := pickFromPool(s.deriveUint32(seed, "arch"), identityProfileArchPool)
	locale := pickFromPool(s.deriveUint32(seed, "locale"), identityProfileLocalePool)
	timezone := pickFromPool(s.deriveUint32(seed, "timezone"), identityProfileTimezonePool)
	uaVersion := s.deriveUserAgentVersion(seed)

	return IdentityProfile{
		UserID:           userID,
		Platform:         plat,
		MachineID:        machineID,
		DeviceIDHex64:    deviceIDHex64,
		OS:               osValue,
		Arch:             arch,
		Locale:           locale,
		Timezone:         timezone,
		UserAgentVersion: uaVersion,
		RotationSalt:     rotationSalt,
		GeneratedAt:      now.UTC(),
	}
}

// RotationDays 暴露当前旋转周期（便于 metrics / 上层调试）。
func (s *IdentityProfileService) RotationDays() int {
	if s == nil {
		return 0
	}
	return s.rotationDays
}

func (s *IdentityProfileService) rotationSalt(now time.Time) string {
	if s == nil || s.rotationDays <= 0 {
		return "rot-0"
	}
	// 以 UTC 整数天 / rotationDays 作为旋转 epoch；这样同一旋转窗口内的所有
	// 调用（无论时区）都会落到同一 salt。
	day := now.UTC().Unix() / 86400
	bucket := day / int64(s.rotationDays)
	return fmt.Sprintf("rot-%d", bucket)
}

func (s *IdentityProfileService) deriveBytes(seed, scope string) []byte {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(seed))
	_, _ = mac.Write([]byte("|"))
	_, _ = mac.Write([]byte(scope))
	return mac.Sum(nil)
}

func (s *IdentityProfileService) deriveHexID(seed, scope string, byteLen int) string {
	raw := s.deriveBytes(seed, scope)
	if byteLen <= 0 || byteLen > len(raw) {
		byteLen = len(raw)
	}
	return hex.EncodeToString(raw[:byteLen])
}

func (s *IdentityProfileService) deriveUint32(seed, scope string) uint32 {
	raw := s.deriveBytes(seed, scope)
	return binary.BigEndian.Uint32(raw[:4])
}

func (s *IdentityProfileService) deriveUserAgentVersion(seed string) string {
	recent := GetCachedRecentVersions()
	if len(recent) == 0 {
		recent = []string{claude.GetCLICurrentVersion()}
	}
	idx := int(s.deriveUint32(seed, "cli_version") % uint32(len(recent)))
	return recent[idx]
}

func pickFromPool(value uint32, pool []string) string {
	if len(pool) == 0 {
		return ""
	}
	return pool[int(value)%len(pool)]
}
