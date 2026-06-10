package service

import (
	"encoding/json"
	"strings"
	"time"
)

// RegistrationFingerprint stores OS/arch/runtime captured at account registration time (P1-1).
//
// Persisted in account.Extra["registration_fingerprint"] and used as fallback for non-CC clients
// in IdentityService.GetOrCreateFingerprintForAccount. Solves the problem where a macOS-registered
// OAuth account always reports Linux/arm64 (the global defaultFingerprint), which Anthropic can
// detect as a mismatch with the device used during OAuth registration.
type RegistrationFingerprint struct {
	OS             string `json:"os,omitempty"`              // e.g. "MacOS", "Windows", "Linux"
	Arch           string `json:"arch,omitempty"`            // e.g. "x64", "arm64"
	Runtime        string `json:"runtime,omitempty"`         // e.g. "node"
	RuntimeVersion string `json:"runtime_version,omitempty"` // e.g. "v24.3.0"
	UserAgent      string `json:"user_agent,omitempty"`      // raw UA string for audit
	CapturedAt     int64  `json:"captured_at,omitempty"`     // unix timestamp
}

// ExtraKeyRegistrationFingerprint is the account.Extra key under which registration fingerprint is stored.
const ExtraKeyRegistrationFingerprint = "registration_fingerprint"

// ParseRegistrationFingerprintFromUA extracts OS/arch hints from a browser User-Agent.
//
// Best-effort: unknown fields stay empty (caller falls back to defaults).
// Returns nil if UA is empty.
//
// Runtime is intentionally always set to "node"/"v24.3.0" (matching defaultFingerprint),
// because we ultimately mimic the Claude CLI — using a browser's real runtime would expose
// us as a proxy. We only capture the OS/arch since those are the most exposed dimensions
// in Anthropic's account-vs-client correlation.
func ParseRegistrationFingerprintFromUA(ua string) *RegistrationFingerprint {
	ua = strings.TrimSpace(ua)
	if ua == "" {
		return nil
	}

	fp := &RegistrationFingerprint{
		UserAgent:      ua,
		Runtime:        "node",
		RuntimeVersion: "v24.3.0",
		CapturedAt:     time.Now().Unix(),
	}

	lower := strings.ToLower(ua)

	// Detect OS first (order matters: Android contains "linux" in some UAs)
	switch {
	case strings.Contains(lower, "android"):
		fp.OS = "Android"
		fp.Arch = "arm64"
	case strings.Contains(lower, "iphone") || strings.Contains(lower, "ipad") || strings.Contains(lower, "ios"):
		fp.OS = "iOS"
		fp.Arch = "arm64"
	case strings.Contains(lower, "mac os x") || strings.Contains(lower, "macintosh"):
		fp.OS = "MacOS"
		// Apple Silicon vs Intel: hard to detect from UA reliably (most modern UAs hide it).
		// Default to arm64 since Apple Silicon dominates 2020+ Macs.
		fp.Arch = "arm64"
	case strings.Contains(lower, "windows nt"):
		fp.OS = "Windows"
		// WOW64/x64 hints
		if strings.Contains(lower, "wow64") || strings.Contains(lower, "win64") || strings.Contains(lower, "x64") || strings.Contains(lower, "x86_64") {
			fp.Arch = "x64"
		} else if strings.Contains(lower, "arm64") || strings.Contains(lower, "aarch64") {
			fp.Arch = "arm64"
		} else {
			fp.Arch = "x64" // default for Windows
		}
	case strings.Contains(lower, "linux"):
		fp.OS = "Linux"
		if strings.Contains(lower, "aarch64") || strings.Contains(lower, "arm64") {
			fp.Arch = "arm64"
		} else if strings.Contains(lower, "x86_64") || strings.Contains(lower, "x64") {
			fp.Arch = "x64"
		} else {
			fp.Arch = "x64"
		}
	}

	if fp.OS == "" {
		// Couldn't determine OS; still return non-nil so caller can record the raw UA for audit
		// but downstream merge logic skips empty fields (preserving defaults).
	}

	return fp
}

// GetRegistrationFingerprint reads the stored registration fingerprint from account.Extra.
// Returns nil if absent or malformed.
func GetRegistrationFingerprint(account *Account) *RegistrationFingerprint {
	if account == nil || account.Extra == nil {
		return nil
	}
	raw, ok := account.Extra[ExtraKeyRegistrationFingerprint]
	if !ok || raw == nil {
		return nil
	}

	// Two possible storage formats due to JSON round-tripping through the database:
	//   1. Already a *RegistrationFingerprint (in-memory after Set, before persist)
	//   2. map[string]any (after Postgres jsonb decode)
	switch v := raw.(type) {
	case *RegistrationFingerprint:
		if v == nil {
			return nil
		}
		return v
	case RegistrationFingerprint:
		return &v
	case map[string]any:
		// Re-marshal then unmarshal to robustly handle the dynamic type
		bs, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		fp := &RegistrationFingerprint{}
		if err := json.Unmarshal(bs, fp); err != nil {
			return nil
		}
		return fp
	default:
		return nil
	}
}

// SetRegistrationFingerprintInExtra writes a registration fingerprint into the supplied extra map.
// Mutates the map in place; creates the map if nil. Returns the (possibly newly created) map for
// chaining convenience.
func SetRegistrationFingerprintInExtra(extra map[string]any, fp *RegistrationFingerprint) map[string]any {
	if fp == nil {
		return extra
	}
	if extra == nil {
		extra = make(map[string]any)
	}
	extra[ExtraKeyRegistrationFingerprint] = fp
	return extra
}
