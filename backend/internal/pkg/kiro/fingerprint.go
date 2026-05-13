package kiro

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type RuntimeFingerprint struct {
	OIDCSDKVersion      string
	RuntimeSDKVersion   string
	StreamingSDKVersion string
	OSType              string
	OSVersion           string
	NodeVersion         string
	KiroVersion         string
	KiroHash            string
}

type runtimeFingerprintManager struct {
	mu           sync.RWMutex
	fingerprints map[string]*RuntimeFingerprint
}

var (
	globalRuntimeFingerprintManager     *runtimeFingerprintManager
	globalRuntimeFingerprintManagerOnce sync.Once

	oidcSDKVersions      = []string{"3.980.0", "3.975.0", "3.972.0", "3.808.0", "3.738.0", "3.737.0", "3.736.0", "3.735.0"}
	runtimeSDKVersions   = []string{"1.0.0"}
	streamingSDKVersions = []string{"1.0.34"}
	osTypes              = []string{"darwin", "win32"}
	osVersions           = map[string][]string{
		"darwin": {"24.6.0"},
		"win32":  {"10.0.22631"},
	}
	nodeVersions = []string{"22.22.0"}
	kiroVersions = []string{
		"0.11.132", "0.11.131", "0.11.130",
	}
)

func globalRuntimeFingerprints() *runtimeFingerprintManager {
	globalRuntimeFingerprintManagerOnce.Do(func() {
		globalRuntimeFingerprintManager = &runtimeFingerprintManager{
			fingerprints: make(map[string]*RuntimeFingerprint),
		}
	})
	return globalRuntimeFingerprintManager
}

func (m *runtimeFingerprintManager) Get(accountKey, machineID string) *RuntimeFingerprint {
	lookupKey := fingerprintLookupKey(accountKey, "runtime")
	machineID = normalizeMachineIDOrFallback(machineID, lookupKey)

	m.mu.RLock()
	if fp, ok := m.fingerprints[lookupKey]; ok && fp.KiroHash == machineID {
		m.mu.RUnlock()
		return fp
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	if fp, ok := m.fingerprints[lookupKey]; ok && fp.KiroHash == machineID {
		return fp
	}
	fp := generateRuntimeFingerprint(lookupKey, machineID)
	m.fingerprints[lookupKey] = fp
	return fp
}

func generateRuntimeFingerprint(accountKey, machineID string) *RuntimeFingerprint {
	hash := sha256.Sum256([]byte(accountKey))
	seed := int64(binary.BigEndian.Uint64(hash[:8]))
	rng := rand.New(rand.NewSource(seed))

	osType := goOSToNodePlatform(runtime.GOOS)
	if !containsString(osTypes, osType) {
		osType = osTypes[rng.Intn(len(osTypes))]
	}
	osVersionPool := osVersions[osType]
	if len(osVersionPool) == 0 {
		osVersionPool = osVersions["darwin"]
	}

	return &RuntimeFingerprint{
		OIDCSDKVersion:      oidcSDKVersions[rng.Intn(len(oidcSDKVersions))],
		RuntimeSDKVersion:   runtimeSDKVersions[rng.Intn(len(runtimeSDKVersions))],
		StreamingSDKVersion: streamingSDKVersions[rng.Intn(len(streamingSDKVersions))],
		OSType:              osType,
		OSVersion:           osVersionPool[rng.Intn(len(osVersionPool))],
		NodeVersion:         nodeVersions[rng.Intn(len(nodeVersions))],
		KiroVersion:         kiroVersions[rng.Intn(len(kiroVersions))],
		KiroHash:            machineID,
	}
}

func goOSToNodePlatform(goos string) string {
	switch strings.TrimSpace(goos) {
	case "windows":
		return "win32"
	default:
		return strings.TrimSpace(goos)
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func BuildAccountKey(clientID, clientIDHash, refreshToken, profileArn string, accountID int64) string {
	switch {
	case strings.TrimSpace(clientIDHash) != "":
		return clientIDHash
	case strings.TrimSpace(clientID) != "":
		return shortSHA(clientID)
	case strings.TrimSpace(refreshToken) != "":
		return shortSHA(refreshToken)
	case strings.TrimSpace(profileArn) != "":
		return shortSHA(profileArn)
	case accountID > 0:
		return shortSHA(fmt.Sprintf("account:%d", accountID))
	default:
		return shortSHA(uuid.NewString())
	}
}

func NormalizeMachineID(machineID string) (string, bool) {
	trimmed := strings.TrimSpace(machineID)
	if len(trimmed) == 64 && isHexString(trimmed) {
		return strings.ToLower(trimmed), true
	}
	withoutDashes := strings.ReplaceAll(trimmed, "-", "")
	if len(withoutDashes) == 32 && isHexString(withoutDashes) {
		normalized := strings.ToLower(withoutDashes)
		return normalized + normalized, true
	}
	return "", false
}

func BuildMachineID(refreshToken, apiKey, fallbackKey string) string {
	if refreshToken = strings.TrimSpace(refreshToken); refreshToken != "" {
		return sha256Hex("KotlinNativeAPI/" + refreshToken)
	}
	if apiKey = strings.TrimSpace(apiKey); apiKey != "" {
		return sha256Hex("KiroAPIKey/" + apiKey)
	}
	if fallbackKey = strings.TrimSpace(fallbackKey); fallbackKey != "" {
		return sha256Hex("KiroFallback/" + fallbackKey)
	}
	return sha256Hex("KiroFallback/default")
}

func shortSHA(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:8])
}

func sha256Hex(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:])
}

func isHexString(value string) bool {
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

func normalizeMachineIDOrFallback(machineID, fallbackKey string) string {
	if normalized, ok := NormalizeMachineID(machineID); ok {
		return normalized
	}
	return BuildMachineID("", "", fallbackKey)
}

func fingerprintLookupKey(accountKey, fallback string) string {
	key := strings.TrimSpace(accountKey)
	if key != "" {
		return key
	}
	return fallback
}

func BuildRuntimeUserAgent(accountKey, machineID string) string {
	fp := globalRuntimeFingerprints().Get(accountKey, machineID)
	return fmt.Sprintf(
		"aws-sdk-js/%s ua/2.1 os/%s#%s lang/js md/nodejs#%s api/codewhispererstreaming#%s m/E KiroIDE-%s-%s",
		fp.StreamingSDKVersion,
		fp.OSType,
		fp.OSVersion,
		fp.NodeVersion,
		fp.StreamingSDKVersion,
		fp.KiroVersion,
		fp.KiroHash,
	)
}

func BuildRuntimeAmzUserAgent(accountKey, machineID string) string {
	fp := globalRuntimeFingerprints().Get(accountKey, machineID)
	return fmt.Sprintf(
		"aws-sdk-js/%s KiroIDE-%s-%s",
		fp.StreamingSDKVersion,
		fp.KiroVersion,
		fp.KiroHash,
	)
}

func BuildOIDCHeaders(accountKey, machineID string) map[string]string {
	fp := globalRuntimeFingerprints().Get(fingerprintLookupKey(accountKey, "oidc-session"), machineID)
	return map[string]string{
		"Content-Type":          "application/json",
		"x-amz-user-agent":      fmt.Sprintf("aws-sdk-js/%s KiroIDE", fp.OIDCSDKVersion),
		"User-Agent":            fmt.Sprintf("aws-sdk-js/%s ua/2.1 os/%s#%s lang/js md/nodejs#%s api/sso-oidc#%s m/E KiroIDE", fp.OIDCSDKVersion, fp.OSType, fp.OSVersion, fp.NodeVersion, fp.OIDCSDKVersion),
		"amz-sdk-invocation-id": uuid.NewString(),
		"amz-sdk-request":       "attempt=1; max=4",
	}
}

func BuildLoginHeaders(accountKey, machineID string) map[string]string {
	fp := globalRuntimeFingerprints().Get(fingerprintLookupKey(accountKey, "login"), machineID)
	return map[string]string{
		"Content-Type": "application/json",
		"User-Agent":   fmt.Sprintf("KiroIDE-%s-%s", fp.KiroVersion, fp.KiroHash),
		"Accept":       "application/json, text/plain, */*",
	}
}

func ExponentialBackoffWithJitter(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	delay := baseDelay << attempt
	if delay > maxDelay {
		delay = maxDelay
	}
	const jitterFactor = 0.3
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	jitter := 1 + ((seed.Float64()*2 - 1) * jitterFactor)
	return time.Duration(float64(delay) * jitter)
}
