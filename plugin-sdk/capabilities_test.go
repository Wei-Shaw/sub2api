package pluginsdk

import "testing"

// TestCapabilityMatches_NewCanonicalSettingsRead is the regression test for
// the bug where runner.go only matched the legacy "settings_extension"
// alias, so a manifest declaring CapabilitySettingsOwnRead silently fell
// back to nilSettingsClient. See task T3 and the comments in runner.go
// around CapabilityGrantedAny(approved, CapabilitySettingsOwnRead).
//
// Verifying CapabilityMatches directly is the smallest proof that the fix
// is in place — runner.go's switch statement was replaced with a call to
// CapabilityGrantedAny which delegates here.
func TestCapabilityMatches_NewCanonicalSettingsRead(t *testing.T) {
	cases := []struct {
		name    string
		granted string
		wanted  string
		want    bool
	}{
		{"canonical read matches itself", CapabilitySettingsOwnRead, CapabilitySettingsOwnRead, true},
		{"legacy settings_extension matches canonical read", CapabilitySettingsExtension, CapabilitySettingsOwnRead, true},
		{"legacy settings_extension matches canonical write", CapabilitySettingsExtension, CapabilitySettingsOwnWrite, true},
		{"canonical write matches itself", CapabilitySettingsOwnWrite, CapabilitySettingsOwnWrite, true},
		{"canonical read does NOT match write", CapabilitySettingsOwnRead, CapabilitySettingsOwnWrite, false},
		{"unrelated capability does not match", CapabilityRedisRaw, CapabilitySettingsOwnRead, false},
		{"redis legacy matches redis canonical", CapabilityRedisRawKeys, CapabilityRedisRaw, true},
		{"secrets legacy matches secrets canonical", CapabilitySecretEncryption, CapabilitySecretsEncrypt, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CapabilityMatches(tc.granted, tc.wanted); got != tc.want {
				t.Fatalf("CapabilityMatches(%q,%q) = %v, want %v", tc.granted, tc.wanted, got, tc.want)
			}
		})
	}
}

// TestCapabilityGrantedAny_PluginDeclaresNewName verifies the runner.go
// fix end-to-end at the helper boundary: a plugin manifest that ONLY
// declares the new canonical CapabilitySettingsOwnRead (channel-management
// is the live example) gets settingsEnabled=true.
func TestCapabilityGrantedAny_PluginDeclaresNewName(t *testing.T) {
	// approved is the slice that PluginInitRequest.capabilities would
	// carry after host-side approveCapabilities + expandWithLegacyAliases.
	// The host expands canonical → legacy when the SDK registry has an
	// alias entry, so we test both shapes the runner might receive.
	cases := []struct {
		name              string
		approved          []string
		wantSettingsRead  bool
		wantSettingsWrite bool
		wantRedisRaw      bool
		wantSecrets       bool
	}{
		{
			name: "plugin declares canonical read; host expands legacy alias which" +
				" also satisfies write (legacy settings_extension implied both)",
			approved:          []string{CapabilitySettingsOwnRead, CapabilitySettingsExtension},
			wantSettingsRead:  true,
			wantSettingsWrite: true, // alias triggers both — see CapabilityRegistry note
		},
		{
			name:             "plugin declares only canonical read; host did not expand alias",
			approved:         []string{CapabilitySettingsOwnRead},
			wantSettingsRead: true,
		},
		{
			name:              "plugin declares only canonical write",
			approved:          []string{CapabilitySettingsOwnWrite},
			wantSettingsWrite: true,
		},
		{
			name: "plugin still uses legacy settings_extension — implies both" +
				" read and write to preserve pre-P12 behaviour",
			approved:          []string{CapabilitySettingsExtension},
			wantSettingsRead:  true,
			wantSettingsWrite: true,
		},
		{
			name:         "plugin declares only canonical redis.raw",
			approved:     []string{CapabilityRedisRaw},
			wantRedisRaw: true,
		},
		{
			name:        "plugin declares only canonical secrets.encrypt",
			approved:    []string{CapabilitySecretsEncrypt},
			wantSecrets: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CapabilityGrantedAny(tc.approved, CapabilitySettingsOwnRead); got != tc.wantSettingsRead {
				t.Fatalf("CapabilityGrantedAny SettingsOwnRead = %v, want %v", got, tc.wantSettingsRead)
			}
			if got := CapabilityGrantedAny(tc.approved, CapabilitySettingsOwnWrite); got != tc.wantSettingsWrite {
				t.Fatalf("CapabilityGrantedAny SettingsOwnWrite = %v, want %v", got, tc.wantSettingsWrite)
			}
			if got := CapabilityGrantedAny(tc.approved, CapabilityRedisRaw); got != tc.wantRedisRaw {
				t.Fatalf("CapabilityGrantedAny RedisRaw = %v, want %v", got, tc.wantRedisRaw)
			}
			if got := CapabilityGrantedAny(tc.approved, CapabilitySecretsEncrypt); got != tc.wantSecrets {
				t.Fatalf("CapabilityGrantedAny SecretsEncrypt = %v, want %v", got, tc.wantSecrets)
			}
		})
	}
}

// TestCanonicalCapability_KnownAndUnknown locks down the rename helper used
// by the host's normalizeCapability. Unknown / canonical inputs round-trip
// unchanged; legacy aliases collapse to their canonical with renamed=true.
func TestCanonicalCapability_KnownAndUnknown(t *testing.T) {
	t.Run("legacy renamed", func(t *testing.T) {
		canon, renamed := CanonicalCapability(CapabilityRedisRawKeys)
		if !renamed || canon != CapabilityRedisRaw {
			t.Fatalf("got (%q,%v), want (%q,true)", canon, renamed, CapabilityRedisRaw)
		}
	})
	t.Run("canonical untouched", func(t *testing.T) {
		canon, renamed := CanonicalCapability(CapabilityRedisRaw)
		if renamed || canon != CapabilityRedisRaw {
			t.Fatalf("got (%q,%v), want (%q,false)", canon, renamed, CapabilityRedisRaw)
		}
	})
	t.Run("unknown untouched", func(t *testing.T) {
		canon, renamed := CanonicalCapability("totally.made.up")
		if renamed || canon != "totally.made.up" {
			t.Fatalf("got (%q,%v), want (totally.made.up,false)", canon, renamed)
		}
	})
}

// TestLegacyAliasesFor_SharedAlias documents the multi-canonical alias
// case: settings_extension is listed under both SettingsOwnRead and
// SettingsOwnWrite so the host can expand either canonical back to the
// legacy string when shipping PluginInitRequest.
func TestLegacyAliasesFor_SharedAlias(t *testing.T) {
	read := LegacyAliasesFor(CapabilitySettingsOwnRead)
	if len(read) != 1 || read[0] != CapabilitySettingsExtension {
		t.Fatalf("LegacyAliasesFor(read) = %v, want [%q]", read, CapabilitySettingsExtension)
	}
	write := LegacyAliasesFor(CapabilitySettingsOwnWrite)
	if len(write) != 1 || write[0] != CapabilitySettingsExtension {
		t.Fatalf("LegacyAliasesFor(write) = %v, want [%q]", write, CapabilitySettingsExtension)
	}
	if got := LegacyAliasesFor(CapabilityRedisOwn); got != nil {
		t.Fatalf("LegacyAliasesFor(redis.own) = %v, want nil (no legacy alias)", got)
	}
}

// TestCallerMetadataKey_NotEmpty is a paranoia check: the wire contract
// shared with the host depends on this constant; reading "" off the
// header would silently drop every caller into the anonymous bucket.
func TestCallerMetadataKey_NotEmpty(t *testing.T) {
	if CallerMetadataKey == "" {
		t.Fatal("CallerMetadataKey is empty; gRPC metadata gating would break")
	}
}
