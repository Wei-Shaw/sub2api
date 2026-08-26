package cursor

import (
	"crypto/sha256"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	DefaultClientVersion = "3.16.17"
	DefaultClientCommit  = "6b2afae0257df2bb5e1835f15165dc2f0de056b0"
	DefaultUserAgent     = "connect-es/1.6.1"
)

// Credentials holds the authentication data for a Cursor account.
// MachineID / MacMachineID must be the telemetry IDs from the Cursor install
// (storage.json telemetry.machineId / telemetry.macMachineId), not serviceMachineId.
type Credentials struct {
	AccessToken   string
	MachineID     string
	MacMachineID  string
	ClientVersion string // defaults to DefaultClientVersion if empty
	ClientCommit  string // only sent if set; official IDE sends this only for Anysphere users
	SessionID     string // cppSessionId; omitted when empty
	GhostMode     bool
	ClientLayout  string // "editor", "unifiedAgent", or "glass"; defaults to "editor"
}

// BuildHeaders constructs the HTTP headers Cursor 3.16's setCommonHeaders/bFg send.
func BuildHeaders(creds Credentials) map[string]string {
	version := creds.ClientVersion
	if version == "" {
		version = DefaultClientVersion
	}

	tokenHash := sha256Hex(creds.AccessToken)
	requestID := uuid.New().String()

	ghostMode := "false"
	if creds.GhostMode {
		ghostMode = "true"
	}

	layout := creds.ClientLayout
	if layout == "" {
		layout = "editor"
	}

	h := map[string]string{
		"authorization":               fmt.Sprintf("Bearer %s", creds.AccessToken),
		"content-type":                "application/connect+proto",
		"connect-protocol-version":    "1",
		"user-agent":                  DefaultUserAgent,
		"x-amzn-trace-id":             fmt.Sprintf("Root=%s", requestID),
		"x-client-key":                tokenHash,
		"x-cursor-checksum":           GenerateChecksumAt(creds.MachineID, creds.MacMachineID, time.Now()),
		"x-cursor-client-version":     version,
		"x-cursor-client-type":        "ide",
		"x-cursor-client-layout":      layout,
		"x-cursor-client-os":          nodeOS(),
		"x-cursor-client-arch":        nodeArch(),
		"x-cursor-client-device-type": "desktop",
		"x-cursor-timezone":           clientTimezone(),
		"x-ghost-mode":                ghostMode,
		"x-new-onboarding-completed":  "false",
		"x-request-id":                requestID,
	}
	if creds.ClientCommit != "" {
		h["x-cursor-client-commit"] = creds.ClientCommit
	}
	if creds.SessionID != "" {
		h["x-session-id"] = creds.SessionID
	}
	return h
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h)
}

// nodeOS matches process.platform in Cursor's Electron/Node runtime.
func nodeOS() string {
	switch runtime.GOOS {
	case "windows":
		return "win32"
	default:
		return runtime.GOOS
	}
}

// nodeArch matches process.arch in Cursor's Electron/Node runtime.
func nodeArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "386":
		return "ia32"
	default:
		return runtime.GOARCH
	}
}

func clientTimezone() string {
	out, err := exec.Command("readlink", "/etc/localtime").Output()
	if err == nil {
		s := strings.TrimSpace(string(out))
		if i := strings.Index(s, "zoneinfo/"); i >= 0 {
			tz := s[i+len("zoneinfo/"):]
			if tz != "" {
				return tz
			}
		}
	}
	return "UTC"
}

func osRelease() string {
	out, err := exec.Command("uname", "-r").Output()
	if err == nil {
		if s := strings.TrimSpace(string(out)); s != "" {
			return s
		}
	}
	return ""
}
