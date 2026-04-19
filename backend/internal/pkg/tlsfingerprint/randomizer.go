package tlsfingerprint

import (
	"math/rand/v2"
)

// GenerateRandomizedProfile returns a new Profile that perturbs the
// Claude Code 2.1.114 baseline along several account-safe axes so every
// account can carry a unique JA3 hash while still looking like "some
// Node.js-family TLS client."
//
// Intentionally conservative: the baseline cipher / signature-algorithm
// shape is preserved; only localized swaps happen within bands so the
// overall "TLS 1.3 first, ECDHE before RSA, AES-GCM before CBC" ordering
// stays intact. Anthropic classifying on "JA3 equals exact Claude Code
// fingerprint" would lose this account, but classifying on "entire pool
// hashes identically" (the actual clustering risk) stops working.
//
// Axes randomized (for the 17-cipher / 9-sigalg 2.1.114 baseline):
//  1. Cipher suites — 2–4 localized swaps within the ECDHE PFS band [3:13]
//     and 0–1 swaps within the RSA band [13:]. TLS 1.3 ciphers [0:3] stay
//     in baseline order — reordering those is the strongest mimic tell.
//  2. Signature algorithms — localized swaps within the SHA-384 pair and
//     the SHA-512 pair. The leading ECDSA-SHA256/RSA-PSS-rsae-SHA256/
//     RSA-PKCS1-SHA256 triple stays pinned (it's a stable Node.js
//     signature and reordering it is detectable).
//  3. GREASE — 30% on, 70% off. Separate from ECH GREASE (that's rolled
//     per-handshake in the dialer).
//  4. ALPN — always http/1.1 only. Do NOT advertise h2: Go's http.Transport
//     with a custom DialTLSContext cannot speak HTTP/2. If the server
//     negotiates h2 via ALPN, the transport writes HTTP/1.1 over the
//     connection and reads back HTTP/2 SETTINGS/WINDOW_UPDATE/GOAWAY
//     frames, surfacing as "malformed HTTP response" errors. Real Claude
//     Code 2.1.114 only sends http/1.1 in ALPN anyway (see dialer.go).
//  5. Key share groups — always X25519 only. MLKEM768 was dropped in the
//     2.1.114 baseline; adding it here would leave "JA3 says no PQ hybrid
//     but key_share sends one" as a detection signal.
//
// Returns a freshly-allocated Profile so callers may mutate safely.
// Uses math/rand/v2 package-global source (seeded by runtime).
func GenerateRandomizedProfile() *Profile {
	p := &Profile{}

	// --- cipher suites ---
	ciphers := make([]uint16, len(defaultCipherSuites))
	copy(ciphers, defaultCipherSuites)
	// ECDHE PFS band — indices 3..12 in the 2.1.114 baseline: ECDHE+AES-GCM
	// (3..6), ChaCha20 pair (7..8), ECDHE+AES-CBC-SHA (9..12). Localized
	// swaps here produce many JA3 variants without leaving the band.
	pfsLo, pfsHi := 3, 13
	if pfsHi > len(ciphers) {
		pfsHi = len(ciphers)
	}
	if pfsHi-pfsLo >= 2 {
		swapCountPFS := 2 + rand.IntN(3) // 2..4
		for range swapCountPFS {
			a := pfsLo + rand.IntN(pfsHi-pfsLo)
			b := pfsLo + rand.IntN(pfsHi-pfsLo)
			ciphers[a], ciphers[b] = ciphers[b], ciphers[a]
		}
	}
	// RSA band — indices 13..16 (4 ciphers: RSA+AES-GCM-128/256, RSA+CBC-128/256).
	// One swap max — the band is small enough that more scrambling would be
	// obviously non-baseline.
	rsaLo := 13
	if rsaLo+2 <= len(ciphers) && rand.IntN(2) == 0 {
		a := rsaLo + rand.IntN(len(ciphers)-rsaLo)
		b := rsaLo + rand.IntN(len(ciphers)-rsaLo)
		ciphers[a], ciphers[b] = ciphers[b], ciphers[a]
	}
	p.CipherSuites = ciphers

	// --- signature algorithms ---
	// defaultSignatureAlgorithms uses utls.SignatureScheme, convert to
	// uint16 for storage while we shuffle. Baseline order:
	//   0:ecdsa_sha256, 1:rsa_pss_rsae_sha256, 2:rsa_pkcs1_sha256,
	//   3:ecdsa_sha384, 4:rsa_pss_rsae_sha384, 5:rsa_pkcs1_sha384,
	//   6:rsa_pss_rsae_sha512, 7:rsa_pkcs1_sha512,
	//   8:rsa_pkcs1_sha1
	sigs := make([]uint16, len(defaultSignatureAlgorithms))
	for i, s := range defaultSignatureAlgorithms {
		sigs[i] = uint16(s)
	}
	shuffleWithin := func(lo, hi int) {
		if hi > len(sigs) || hi-lo < 2 {
			return
		}
		sub := sigs[lo:hi]
		rand.Shuffle(len(sub), func(i, j int) { sub[i], sub[j] = sub[j], sub[i] })
	}
	// Shuffle the SHA384 and SHA512 pairs locally. The SHA256 triple and
	// legacy SHA1 floor (0..2, 8) stay pinned — those are strong mimic tells.
	shuffleWithin(4, 6) // rsa_pss_rsae_sha384 ↔ rsa_pkcs1_sha384
	shuffleWithin(6, 8) // rsa_pss_rsae_sha512 ↔ rsa_pkcs1_sha512
	p.SignatureAlgorithms = sigs

	// --- GREASE ---
	p.EnableGREASE = rand.IntN(10) < 3 // 30%

	// --- ALPN ---
	// Always http/1.1 only. See axis 4 in the doc comment for why advertising
	// h2 here breaks request forwarding.
	p.ALPNProtocols = []string{"http/1.1"}

	// --- Key share groups ---
	// Always X25519 only — matches 2.1.114 baseline.
	p.KeyShareGroups = []uint16{uint16(29)} // X25519

	// Leave curves/point_formats/supported_versions/psk_modes/extensions
	// empty so the dialer falls back to baseline values (incl. the
	// probabilistic ECH GREASE roll in defaultExtensionOrder).

	return p
}
