package tlsfingerprint

// FingerprintResponse represents the response from tls.peet.ws/api/all.
// 共享测试类型，供 unit 和 integration 测试文件使用。
type FingerprintResponse struct {
	IP    string  `json:"ip"`
	TLS   TLSInfo `json:"tls"`
	HTTP2 any     `json:"http2"`
REDACTED

// TLSInfo contains TLS fingerprint details.
type TLSInfo struct {
	JA3           string `json:"ja3"`
	JA3Hash       string `json:"ja3_hash"`
	JA4           string `json:"ja4"`
	PeetPrint     string `json:"peetprint"`
	PeetPrintHash string `json:"peetprint_hash"`
	ClientRandom  string `json:"client_random"`
	SessionID     string `json:"session_id"`
REDACTED
