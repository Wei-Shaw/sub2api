package main

// Capture is the top-level fingerprint payload written to stdout / -out file.
type Capture struct {
	CapturedAt        string   `json:"captured_at"`
	RemoteAddr        string   `json:"remote_addr"`
	ServerName        string   `json:"server_name,omitempty"`
	TLSVersion        string   `json:"tls_version"`
	NegotiatedProto   string   `json:"negotiated_proto"`
	ClientHelloRaw    string   `json:"client_hello_raw"`
	CipherSuites      []string `json:"cipher_suites"`
	Extensions        []string `json:"extensions"`
	Curves            []string `json:"curves"`
	PointFormats      []string `json:"point_formats"`
	SignatureAlgos    []string `json:"signature_algorithms"`
	SupportedVersions []string `json:"supported_versions"`
	KeyShareGroups    []string `json:"key_share_groups"`
	ALPNProtos        []string `json:"alpn_protos"`
	PSKModes          []string `json:"psk_modes"`
	JA3String         string   `json:"ja3_string"`
	JA3Hash           string   `json:"ja3_hash"`
	JA4               string   `json:"ja4"`

	HTTP2 *H2Capture `json:"http2,omitempty"`
	HTTP1 *H1Capture `json:"http1,omitempty"`
}

// H2Capture records the HTTP/2 view of one capture session.
type H2Capture struct {
	PrefaceSeen             bool              `json:"preface_seen"`
	ClientSettings          map[string]uint32 `json:"client_settings"`
	WindowUpdate            *uint32           `json:"initial_window_update,omitempty"`
	FirstHeadersPseudoOrder []string          `json:"first_headers_pseudo_order"`
	FirstHeadersAll         []string          `json:"first_headers_all"`
	FrameSequence           []string          `json:"frame_sequence"`
	// Requests records every HTTP/2 request (stream) the client opened on
	// this connection, in order seen. Used to discover what sidecar
	// endpoints real Claude Code calls beyond /v1/messages.
	Requests []H2Request `json:"requests,omitempty"`
}

// H2Request is one HTTP/2 request seen on a capture session.
type H2Request struct {
	StreamID    uint32   `json:"stream_id"`
	Method      string   `json:"method"`
	Authority   string   `json:"authority,omitempty"`
	Path        string   `json:"path"`
	Headers     []string `json:"headers,omitempty"`
	BodyPreview string   `json:"body_preview,omitempty"`
}

// H1Capture records the HTTP/1.1 fallback view of a capture session.
type H1Capture struct {
	RequestLine string   `json:"request_line"`
	Headers     []string `json:"headers_in_order"`
}
