package service

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sentinel PoW follows the browser client's FNV1a32 plus avalanche hash.
// Both the hash-prefix comparison and the ~S suffix are part of the protocol.

const (
	chatGPTWebDefaultPowScript  = "https://chatgpt.com/backend-api/sentinel/sdk.js"
	chatGPTWebPowFallbackPrefix = "wQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D"
	chatGPTWebPowMaxAttempts    = 500000
)

// 指纹候选值尽量贴近真实浏览器，避免与账号既有设备画像冲突。
var chatGPTWebNavigatorKeys = []string{
	"registerProtocolHandler\u2212function registerProtocolHandler() { [native code] }",
	"storage\u2212[object StorageManager]",
	"locks\u2212[object LockManager]",
	"appCodeName\u2212Mozilla",
	"permissions\u2212[object Permissions]",
	"share\u2212function share() { [native code] }",
	"webdriver\u2212false",
	"managed\u2212[object NavigatorManagedData]",
	"canShare\u2212function canShare() { [native code] }",
	"vendor\u2212Google Inc.",
	"mediaDevices\u2212[object MediaDevices]",
	"vibrate\u2212function vibrate() { [native code] }",
	"storageBuckets\u2212[object StorageBucketManager]",
	"mediaCapabilities\u2212[object MediaCapabilities]",
	"cookieEnabled\u2212true",
	"virtualKeyboard\u2212[object VirtualKeyboard]",
	"product\u2212Gecko",
	"presentation\u2212[object Presentation]",
	"onLine\u2212true",
	"mimeTypes\u2212[object MimeTypeArray]",
	"credentials\u2212[object CredentialsContainer]",
	"serviceWorker\u2212[object ServiceWorkerContainer]",
	"keyboard\u2212[object Keyboard]",
	"gpu\u2212[object GPU]",
	"doNotTrack",
	"serial\u2212[object Serial]",
	"pdfViewerEnabled\u2212true",
	"language\u2212zh-CN",
	"geolocation\u2212[object Geolocation]",
	"userAgentData\u2212[object NavigatorUAData]",
	"getUserMedia\u2212function getUserMedia() { [native code] }",
	"sendBeacon\u2212function sendBeacon() { [native code] }",
	"hardwareConcurrency\u221232",
	"windowControlsOverlay\u2212[object WindowControlsOverlay]",
}

var chatGPTWebWindowKeys = []string{
	"0", "window", "self", "document", "name", "location", "customElements", "history",
	"navigation", "innerWidth", "innerHeight", "scrollX", "scrollY", "visualViewport",
	"screenX", "screenY", "outerWidth", "outerHeight", "devicePixelRatio", "screen",
	"chrome", "navigator", "onresize", "performance", "crypto", "indexedDB",
	"sessionStorage", "localStorage", "scheduler", "alert", "atob", "btoa", "fetch",
	"matchMedia", "postMessage", "queueMicrotask", "requestAnimationFrame", "setInterval",
	"setTimeout", "caches", "__NEXT_DATA__", "__BUILD_MANIFEST", "__NEXT_PRELOADREADY",
}

var chatGPTWebDocumentKeys = []string{
	"__reactContainer$fzelfjyxej8", "_reactListening5dehydibo78", "location",
}

var chatGPTWebScreenResolutions = [][2]int{{1920, 1080}, {1440, 900}, {2560, 1440}, {3840, 2160}}

var chatGPTWebPowCores = []int{8, 16, 24, 32}

var (
	chatGPTWebScriptSrcPattern    = regexp.MustCompile(`<script[^>]+src="([^"]+)"`)
	chatGPTWebDataBuildSrcPattern = regexp.MustCompile(`c/[^/]*/_`)
	chatGPTWebDataBuildAttrPatt   = regexp.MustCompile(`<html[^>]*data-build="([^"]*)"`)
)

// chatGPTWebProcessStart 提供 config 中的 performance.now 基准（进程内单调时钟）。
var chatGPTWebProcessStart = time.Now()

// Keep conversation metadata and the Sentinel date in the same browser timezone.
// This is independent of the server's timezone used for business data and logs.
var chatGPTWebBrowserLocation = func() *time.Location {
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return time.UTC
	}
	return location
}()

func chatGPTWebTimezoneOffset(now time.Time) int {
	_, secondsEast := now.In(chatGPTWebBrowserLocation).Zone()
	// JavaScript getTimezoneOffset reports UTC minus local time.
	return -secondsEast / 60
}

func chatGPTWebPerfNowMillis() float64 {
	return float64(time.Since(chatGPTWebProcessStart).Microseconds()) / 1000.0
}

func chatGPTWebBrowserDate(now time.Time) string {
	local := now.In(chatGPTWebBrowserLocation)
	zone, _ := local.Zone()
	switch zone {
	case "PST":
		zone = "Pacific Standard Time"
	case "PDT":
		zone = "Pacific Daylight Time"
	case "JST":
		zone = "Japan Standard Time"
	}
	return local.Format("Mon Jan 02 2006 15:04:05 GMT-0700") + " (" + zone + ")"
}

func chatGPTWebCompactJSON(value any) string {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return ""
	}
	return strings.TrimRight(buf.String(), "\n")
}

func chatGPTWebPickString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(values))))
	if err != nil {
		return values[0]
	}
	return values[index.Int64()]
}

func chatGPTWebRandomFloat() float64 {
	index, err := rand.Int(rand.Reader, big.NewInt(1<<53))
	if err != nil {
		return 0.5
	}
	return float64(index.Int64()) / float64(int64(1)<<53)
}

func chatGPTWebRandomInt(limit int) int {
	if limit <= 1 {
		return 0
	}
	index, err := rand.Int(rand.Reader, big.NewInt(int64(limit)))
	if err != nil {
		return 0
	}
	return int(index.Int64())
}

// chatGPTWebPowConfig 构造与浏览器一致的指纹数组，顺序不可调整。
func chatGPTWebPowConfig(userAgent string, scriptSources []string, dataBuild string) []any {
	resolution := chatGPTWebScreenResolutions[chatGPTWebRandomInt(len(chatGPTWebScreenResolutions))]
	scriptSource := chatGPTWebDefaultPowScript
	if len(scriptSources) > 0 {
		scriptSource = chatGPTWebPickString(scriptSources)
	}
	perfNow := chatGPTWebPerfNowMillis()
	return []any{
		resolution[0] + resolution[1],
		chatGPTWebBrowserDate(time.Now()),
		int64(4294705152),
		1,
		userAgent,
		scriptSource,
		dataBuild,
		"en-US",
		"en-US,en",
		chatGPTWebRandomFloat(),
		chatGPTWebPickString(chatGPTWebNavigatorKeys),
		chatGPTWebPickString(chatGPTWebDocumentKeys),
		chatGPTWebPickString(chatGPTWebWindowKeys),
		perfNow,
		uuid.NewString(),
		"",
		chatGPTWebPowCores[chatGPTWebRandomInt(len(chatGPTWebPowCores))],
		float64(time.Now().UnixMilli()) - perfNow,
		0, 0, 0, 0, 0, 0,
		0,
	}
}

// parseChatGPTWebPowResources 从首页 HTML 里解析 sentinel sdk 脚本地址与构建号。
// 两者会进入指纹 JSON，缺失时退化为默认脚本地址，不影响 PoW 求解。
func parseChatGPTWebPowResources(html string) ([]string, string) {
	matches := chatGPTWebScriptSrcPattern.FindAllStringSubmatch(html, -1)
	sources := make([]string, 0, len(matches))
	dataBuild := ""
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		src := strings.TrimSpace(match[1])
		if src == "" {
			continue
		}
		sources = append(sources, src)
		if dataBuild == "" {
			dataBuild = chatGPTWebDataBuildSrcPattern.FindString(src)
		}
	}
	if dataBuild == "" {
		if match := chatGPTWebDataBuildAttrPatt.FindStringSubmatch(html); len(match) >= 2 {
			dataBuild = match[1]
		}
	}
	if len(sources) == 0 {
		sources = []string{chatGPTWebDefaultPowScript}
	}
	return sources, dataBuild
}

// chatGPTWebBuildRequirementsToken 生成 chat-requirements/prepare 的 `p` 字段。
func chatGPTWebBuildRequirementsToken(userAgent string, scriptSources []string, dataBuild string) string {
	config := chatGPTWebPowConfig(userAgent, scriptSources, dataBuild)
	payload := chatGPTWebCompactJSON(config)
	return "gAAAAAC" + base64.StdEncoding.EncodeToString([]byte(payload))
}

// chatGPTWebBuildProofToken 解出 proofofwork 挑战并返回 OpenAI-Sentinel-Proof-Token。
func chatGPTWebBuildProofToken(seed, difficulty, userAgent string, scriptSources []string, dataBuild string) (string, error) {
	config := chatGPTWebPowConfig(userAgent, scriptSources, dataBuild)
	if strings.TrimSpace(difficulty) == "" {
		difficulty = "0"
	}
	answer, solved := chatGPTWebPowGenerate(seed, difficulty, config, chatGPTWebPowMaxAttempts)
	if !solved {
		return "", fmt.Errorf("chatgpt web proof token unsolved: difficulty=%s", difficulty)
	}
	return "gAAAAAB" + answer, nil
}

// chatGPTWebPowHash matches H3t in the client. Inputs here are the ASCII seed
// followed by a base64-encoded UTF-8 fingerprint.
func chatGPTWebPowHash(value string) string {
	hash := uint32(2166136261)
	for i := 0; i < len(value); i++ {
		hash ^= uint32(value[i])
		hash *= 16777619
	}
	hash ^= hash >> 16
	hash *= 2246822507
	hash ^= hash >> 13
	hash *= 3266489909
	hash ^= hash >> 16
	return fmt.Sprintf("%08x", hash)
}

func chatGPTWebPowGenerate(seed, difficulty string, config []any, limit int) (string, bool) {
	if len(config) < 11 || len(difficulty) == 0 || len(difficulty) > 8 {
		return "", false
	}
	for _, c := range difficulty {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return "", false
		}
	}
	head := strings.TrimSuffix(chatGPTWebCompactJSON(config[:3]), "]") + ","
	middle := "," + strings.TrimSuffix(strings.TrimPrefix(chatGPTWebCompactJSON(config[4:9]), "["), "]") + ","
	tail := "," + strings.TrimPrefix(chatGPTWebCompactJSON(config[10:]), "[")
	started := time.Now()
	for i := 0; i < limit; i++ {
		payload := head + strconv.Itoa(i) + middle + strconv.FormatInt(time.Since(started).Milliseconds(), 10) + tail
		encoded := base64.StdEncoding.EncodeToString([]byte(payload))
		if chatGPTWebPowHash(seed + encoded)[:len(difficulty)] <= difficulty {
			return encoded + "~S", true
		}
	}
	return "", false
}
