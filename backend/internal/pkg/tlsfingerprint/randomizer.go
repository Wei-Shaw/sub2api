package tlsfingerprint

import (
	"math/rand/v2"
)

// GenerateRandomizedProfile returns a new Profile that perturbs the
// Claude Code 2.1.112 baseline along several account-safe axes so every
// account can carry a unique JA3 hash while still looking like "some
// Node.js-family TLS client."
//
// Intentionally conservative: the baseline cipher / signature-algorithm
// shape is preserved; only localized swaps happen within bands so the
// overall PFS-preferred ordering stays intact. Anthropic classifying on
// "JA3 equals exact Claude Code fingerprint" would lose this account,
// but classifying on "entire pool hashes identically" (the actual
// clustering risk) stops working.
//
// Axes randomized:
//  1. Cipher suites — 6–10 localized swaps within the PFS band [3:29]
//     and the RSA band [41:] (indices into the baseline order).
//  2. Signature algorithms — localized swaps within RSA-PSS-PSS,
//     RSA-PSS-RSAE, and legacy-DSA groups.
//  3. GREASE — 30% on, 70% off (matches the rare but plausible case of
//     a Chrome-behavior wrapper around the Node stack).
//  4. ALPN — always http/1.1 only. Do NOT advertise h2: Go's http.Transport
//     with a custom DialTLSContext cannot speak HTTP/2. If the server
//     negotiates h2 via ALPN, the transport writes HTTP/1.1 over the
//     connection and reads back HTTP/2 SETTINGS/WINDOW_UPDATE/GOAWAY
//     frames, surfacing as "malformed HTTP response" errors. Real Claude
//     Code only sends http/1.1 in ALPN anyway (see dialer.go:73).
//  5. Key share groups — always MLKEM768+X25519. Cannot drop MLKEM768
//     here: supported_groups still advertises it, and if the server HRRs
//     back to MLKEM768, utls's HRR path can't regenerate its dual-key
//     share and fails with "CurvePreferences includes unsupported curve".
//
// Returns a freshly-allocated Profile so callers may mutate safely.
// Uses math/rand/v2 package-global source (seeded by runtime).
func GenerateRandomizedProfile() *Profile {
	p := &Profile{}

	// --- cipher suites ---
	ciphers := make([]uint16, len(defaultCipherSuites))
	copy(ciphers, defaultCipherSuites)
	// Localized swaps in the PFS (post-TLS1.3) band. Index bounds derived
	// from the baseline grouping in dialer.go; keeps RSA ciphers at the
	// bottom so the real-world "prefer forward-secrecy" shape holds.
	pfsLo, pfsHi := 3, 29
	if pfsHi > len(ciphers) {
		pfsHi = len(ciphers)
	}
	swapCountPFS := 6 + rand.IntN(5) // 6..10
	for range swapCountPFS {
		a := pfsLo + rand.IntN(pfsHi-pfsLo)
		b := pfsLo + rand.IntN(pfsHi-pfsLo)
		ciphers[a], ciphers[b] = ciphers[b], ciphers[a]
	}
	// RSA band — smaller, fewer swaps so we don't completely scramble it.
	rsaLo := 41
	if rsaLo < len(ciphers) {
		swapCountRSA := 2 + rand.IntN(3) // 2..4
		for range swapCountRSA {
			a := rsaLo + rand.IntN(len(ciphers)-rsaLo)
			b := rsaLo + rand.IntN(len(ciphers)-rsaLo)
			ciphers[a], ciphers[b] = ciphers[b], ciphers[a]
		}
	}
	p.CipherSuites = ciphers

	// --- signature algorithms ---
	// defaultSignatureAlgorithms uses utls.SignatureScheme, convert to
	// uint16 for storage while we shuffle.
	sigs := make([]uint16, len(defaultSignatureAlgorithms))
	for i, s := range defaultSignatureAlgorithms {
		sigs[i] = uint16(s)
	}
	// Shuffle three groups that contain multiple peers. Indices into the
	// baseline ordering (see dialer.go:198). Keeping experimental TLS 1.3
	// (0..2) and ECDSA (3..5) unchanged — those are the strongest leading
	// signals real Node clients send in order.
	shuffleWithin := func(lo, hi int) {
		if hi > len(sigs) || hi-lo < 2 {
			return
		}
		sub := sigs[lo:hi]
		rand.Shuffle(len(sub), func(i, j int) { sub[i], sub[j] = sub[j], sub[i] })
	}
	shuffleWithin(11, 14) // RSA-PSS-PSS (0x0809..0x080b)
	shuffleWithin(14, 17) // RSA-PSS-RSAE (0x0804..0x0806)
	shuffleWithin(20, 26) // legacy SHA224/256/384/512 DSA/etc.
	p.SignatureAlgorithms = sigs

	// --- GREASE ---
	p.EnableGREASE = rand.IntN(10) < 3 // 30%

	// --- ALPN ---
	// Always http/1.1 only. See axis 4 in the doc comment for why advertising
	// h2 here breaks request forwarding.
	p.ALPNProtocols = []string{"http/1.1"}

	// --- Key share groups ---
	// Always MLKEM768+X25519 — see axis 5 in the doc comment for why
	// dropping MLKEM768 here is unsafe with the current utls HRR path.
	p.KeyShareGroups = []uint16{uint16(0x11ec), uint16(29)} // MLKEM768, X25519

	// Leave curves/point_formats/supported_versions/psk_modes/extensions
	// empty so the dialer falls back to baseline values. Perturbing those
	// axes further without live captures to validate would risk leaving
	// the plausible-Node-client envelope.

	return p
}
