package tlsfingerprint

// FingerprintResponse represents the response from tls.peet.ws/api/all.
// 共享测试类型，供 unit 和 integration 测试文件使用。
type FingerprintResponse struct {
	IP    string  `json:"ip"`
	TLS   TLSInfo `json:"tls"`
	HTTP2 any     `json:"http2"`
}

// TestProfileExpectation defines expected fingerprint values for a profile.
type TestProfileExpectation struct {
	Profile       *Profile
	ExpectedJA3   string // Expected JA3 hash (empty = don't check)
	ExpectedJA4   string // Expected full JA4 (empty = don't check)
	JA4CipherHash string // Expected JA4 cipher hash - the stable middle part (empty = don't check)
}

// TLSInfo contains TLS fingerprint details.
type TLSInfo struct {
	JA3           string `json:"ja3"`
	JA3Hash       string `json:"ja3_hash"`
	JA4           string `json:"ja4"`
	PeetPrint     string `json:"peetprint"`
	PeetPrintHash string `json:"peetprint_hash"`
	ClientRandom  string `json:"client_random"`
	SessionID     string `json:"session_id"`
}
