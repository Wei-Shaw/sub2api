// connect.go 实现 Devin Connect-RPC wire 协议层：envelope 帧、请求头、
// 流式帧读取与 Connect 错误解析。
//
// 传输形态逐字节对齐 devin-connect 插件的 connect.ts：
//   - unary: POST application/proto，body 是裸 protobuf（无 envelope）；
//     非 2xx 时 body 是 Connect JSON 错误 {"code","message"}。
//   - stream: POST application/connect+proto，body 是单帧 envelope
//     （flags=0x00，不压缩——48KB 抓包也是裸发）；响应是
//     [flags(1B)][len(4B BE)][payload] 帧序列，flags 0x01=gzip、
//     0x02=end（payload 是 JSON trailer，可能携带 {"error":{...}}）。
package devin

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// RPC 路径（devin-connect connect.ts）。
const (
	PathGetChatMessage       = "/exa.api_server_pb.ApiServerService/GetChatMessage"
	PathGetCliModelConfigs   = "/exa.api_server_pb.ApiServerService/GetCliModelConfigs"
	PathAssignModel          = "/exa.api_server_pb.ApiServerService/AssignModel"
	PathGetUserStatus        = "/exa.seat_management_pb.SeatManagementService/GetUserStatus"
	PathExchangeDevinCLIPKCE = "/exa.seat_management_pb.SeatManagementService/ExchangeDevinCLIPKCECode"
	PathExchangePKCEAuthCode = "/exa.seat_management_pb.SeatManagementService/ExchangePKCEAuthorizationCode"
	PathGetPrimaryAPIKey     = "/exa.seat_management_pb.SeatManagementService/GetPrimaryApiKeyForDevsOnly"
)

// Connect envelope flags。
const (
	envelopeFlagCompressed = 0x01
	envelopeFlagEnd        = 0x02
)

// authScheme 决定 Authorization 头的形态。
type authScheme int

const (
	// authBasic 是 ApiServer 通道的认证：Basic <token>-<token>。
	authBasic authScheme = iota
	// authBearer 用于 Seat 通道个别需要 Bearer 的调用。
	authBearer
	// authNone 不携带 Authorization（PKCE 交换请求本身尚无凭据）。
	authNone
)

// ConnectError 是上游 Connect 协议错误（unary JSON body 或 stream
// end-trailer 中的 {"error":{"code","message"}}）。Code 是 Connect
// 字符串码（"unauthenticated"、"resource_exhausted"、
// "invalid_argument"、"permission_denied"、"not_found"、"unavailable" 等）。
type ConnectError struct {
	Code    string
	Message string
	// HTTPStatus 是 unary 失败时的 HTTP 状态码（0 表示错误来自流内 trailer）。
	HTTPStatus int
}

func (e *ConnectError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

// IsCode 报告错误是否为给定 Connect code。
func IsCode(err error, code string) bool {
	var connectErr *ConnectError
	return errors.As(err, &connectErr) && connectErr.Code == code
}

// IsUnauthenticated 判断错误是否为上游 unauthenticated（凭据失效）。
func IsUnauthenticated(err error) bool {
	return IsCode(err, "unauthenticated")
}

// InvalidRequestError 包装本地请求形状错误（校验失败、tool_choice 非法、
// 图片越界等）：它发生在请求离开发送方之前，重试/换号只会复现同样的
// 失败，必须与瞬时传输错误区分开。
type InvalidRequestError struct{ Err error }

func (e *InvalidRequestError) Error() string { return e.Err.Error() }
func (e *InvalidRequestError) Unwrap() error { return e.Err }

// IsInvalidRequest 报告错误是否为本地请求形状错误。
func IsInvalidRequest(err error) bool {
	var invalid *InvalidRequestError
	return errors.As(err, &invalid)
}

// NewInvalidRequest 构造本地请求形状错误；nil 透传。
func NewInvalidRequest(err error) error {
	if err == nil {
		return nil
	}
	return &InvalidRequestError{Err: err}
}

// IsTransientTransportError 判断建立阶段错误是否值得重试：只对非
// Connect 协议的传输错误（EOF、连接重置、超时）重试。Connect 语义错误
// （含 upstream unavailable 的固定模板文案）重试只会复现同样失败。
// 对齐 devin2api isTransientConnectError。
func IsTransientTransportError(err error) bool {
	if err == nil {
		return false
	}
	var connectErr *ConnectError
	if errors.As(err, &connectErr) {
		return false
	}
	// 本地请求形状错误不是传输问题。
	if IsInvalidRequest(err) {
		return false
	}
	// ctx 取消不是瞬时传输错误。
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}

// sentryTrace 生成抓包形态 <32hex>-<16hex>-1 的 sentry-trace 头。
func sentryTrace() string {
	var trace [16]byte
	var span [8]byte
	_, _ = rand.Read(trace[:])
	_, _ = rand.Read(span[:])
	return hex.EncodeToString(trace[:]) + "-" + hex.EncodeToString(span[:]) + "-1"
}

// connectHeaders 构造抓包对齐的请求头：authorization、sentry-trace、
// content-type、connect-protocol-version、accept:* /*、content-length。
// 不发 User-Agent/Connection/*-Encoding，不做请求压缩。
func connectHeaders(token string, streaming bool, contentLength int, scheme authScheme) http.Header {
	contentType := "application/proto"
	if streaming {
		contentType = "application/connect+proto"
	}
	header := http.Header{
		"Connect-Protocol-Version": {"1"},
		"Content-Type":             {contentType},
		"Accept":                   {"*/*"},
		"sentry-trace":             {sentryTrace()},
		"Content-Length":           {fmt.Sprintf("%d", contentLength)},
		// 真实 CLI 不发送 User-Agent；置空串让 net/http 整体省略该头。
		"User-Agent": {""},
	}
	switch scheme {
	case authBearer:
		if token != "" {
			header.Set("Authorization", "Bearer "+token)
		}
	case authBasic:
		if token != "" {
			header.Set("Authorization", "Basic "+token+"-"+token)
		}
	}
	return header
}

// NewUnaryRequest 构造 unary Connect 请求（裸 protobuf body）。
func NewUnaryRequest(baseURL, path, token string, body []byte) (*http.Request, error) {
	return newConnectRequest(baseURL, path, token, body, false, authBasic)
}

// NewUnaryRequestAuth 同 NewUnaryRequest，可指定认证形态。
func NewUnaryRequestAuth(baseURL, path, token string, body []byte, scheme authScheme) (*http.Request, error) {
	return newConnectRequest(baseURL, path, token, body, false, scheme)
}

// NewUnaryRequestNoAuth 构造无 Authorization 的 unary 请求——PKCE 交换
// （ExchangePKCEAuthorizationCode / ExchangeDevinCLIPKCECode）在拿到凭据
// 之前调用，抓包确认这两个端点不带认证头。
func NewUnaryRequestNoAuth(baseURL, path string, body []byte) (*http.Request, error) {
	return newConnectRequest(baseURL, path, "", body, false, authNone)
}

// NewStreamRequest 构造流式 Connect 请求：body 为单帧 envelope
// （flags=0x00，未压缩——对齐抓包）。
func NewStreamRequest(baseURL, path, token string, body []byte) (*http.Request, error) {
	framed := encodeEnvelope(body, 0x00)
	return newConnectRequest(baseURL, path, token, framed, true, authBasic)
}

func newConnectRequest(baseURL, path, token string, body []byte, streaming bool, scheme authScheme) (*http.Request, error) {
	url := strings.TrimSuffix(baseURL, "/") + path
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = connectHeaders(token, streaming, len(body), scheme)
	req.ContentLength = int64(len(body))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	return req, nil
}

// encodeEnvelope 构造 Connect envelope 帧头 + payload。
func encodeEnvelope(payload []byte, flags byte) []byte {
	out := make([]byte, 5+len(payload))
	out[0] = flags
	binary.BigEndian.PutUint32(out[1:5], uint32(len(payload)))
	copy(out[5:], payload)
	return out
}

// ReadUnaryResponse 校验 unary 响应并返回 protobuf body。
// 非 2xx 时按 Connect JSON 错误解析（{"code","message"}），
// 解析失败回退为带 HTTP 状态码的原文错误。
func ReadUnaryResponse(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseConnectError(resp.StatusCode, body)
	}
	return body, nil
}

// parseConnectError 解析 Connect 错误 body（JSON {"code":...,"message":...}
// 或 trailer 形态 {"error":{...}}）。
func parseConnectError(status int, body []byte) error {
	var parsed struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil {
		if parsed.Error != nil && (parsed.Error.Code != "" || parsed.Error.Message != "") {
			return &ConnectError{Code: parsed.Error.Code, Message: parsed.Error.Message, HTTPStatus: status}
		}
		if parsed.Code != "" || parsed.Message != "" {
			return &ConnectError{Code: parsed.Code, Message: parsed.Message, HTTPStatus: status}
		}
	}
	text := strings.TrimSpace(string(body))
	if len(text) > 400 {
		text = text[:400]
	}
	if text == "" {
		text = http.StatusText(status)
	}
	return &ConnectError{Code: http.StatusText(status), Message: text, HTTPStatus: status}
}

// StreamFrame 是流式响应的一帧。
type StreamFrame struct {
	// End 为 true 表示 Connect end 帧，Payload 是 JSON trailer。
	End     bool
	Payload []byte
}

// ConnectFrameReader 从响应 body 迭代 Connect 帧。
// 用法：for r.Next() { f := r.Frame() } ; err := r.Err()
type ConnectFrameReader struct {
	reader io.Reader // 通常是 resp.Body
	buf    []byte
	frame  StreamFrame
	err    error
	done   bool
}

// NewConnectFrameReader 包装流式响应 body。传入时应已校验 2xx。
func NewConnectFrameReader(reader io.Reader) *ConnectFrameReader {
	return &ConnectFrameReader{reader: reader, buf: make([]byte, 0, 64<<10)}
}

// Next 前进到下一帧；流结束或出错返回 false。
func (r *ConnectFrameReader) Next() bool {
	if r.done || r.err != nil {
		return false
	}
	for len(r.buf) < 5 {
		if !r.fill() {
			return false
		}
	}
	flags := r.buf[0]
	length := int(binary.BigEndian.Uint32(r.buf[1:5]))
	for len(r.buf) < 5+length {
		if !r.fill() {
			return false
		}
	}
	// payload 必须先拷出——随后的左移 append 会复用同一底层数组，
	// 把下一帧的数据覆盖到已返回的 payload 上。
	payload := append([]byte(nil), r.buf[5:5+length]...)
	r.buf = append(r.buf[:0], r.buf[5+length:]...)
	if flags&envelopeFlagCompressed != 0 {
		if decoded, err := gunzip(payload); err == nil {
			payload = decoded
		}
	}
	r.frame = StreamFrame{End: flags&envelopeFlagEnd != 0, Payload: payload}
	if r.frame.End {
		r.done = true
	}
	return true
}

func (r *ConnectFrameReader) fill() bool {
	chunk := make([]byte, 32<<10)
	n, err := r.reader.Read(chunk)
	if n > 0 {
		r.buf = append(r.buf, chunk[:n]...)
		return true
	}
	if err != nil {
		if err == io.EOF {
			if len(r.buf) > 0 {
				r.err = errors.New("devin: truncated Connect stream")
			} else {
				r.err = io.EOF
			}
		} else {
			r.err = err
		}
		return false
	}
	// n==0, err==nil：继续读。
	return true
}

// Frame 返回最近一次 Next 取得的帧。
func (r *ConnectFrameReader) Frame() StreamFrame { return r.frame }

// Err 返回终止错误。正常结束（end 帧或干净 EOF）返回 nil；
// 截断/传输错误返回非 nil。
func (r *ConnectFrameReader) Err() error {
	if r.err == io.EOF {
		return nil
	}
	return r.err
}

// ReadStreamResponse 校验流式响应的 HTTP 状态；非 2xx 时按 Connect
// 错误解析 body（上游在建立阶段失败时回 JSON 错误而非帧序列）。
func ReadStreamResponse(resp *http.Response) (*ConnectFrameReader, error) {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, parseConnectError(resp.StatusCode, body)
	}
	return NewConnectFrameReader(resp.Body), nil
}

// ParseEndTrailer 解析 end 帧 trailer：{"error":{...}} 时返回
// ConnectError，否则 nil（正常收尾 trailer 只带 metadata）。
func ParseEndTrailer(payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	var parsed struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		// 非 JSON trailer——按插件行为，内容里出现 error 字样也算失败。
		if bytes := strings.TrimSpace(string(payload)); strings.Contains(bytes, "error") {
			if len(bytes) > 500 {
				bytes = bytes[:500]
			}
			return &ConnectError{Code: "internal", Message: bytes}
		}
		return nil
	}
	if parsed.Error != nil {
		return &ConnectError{Code: parsed.Error.Code, Message: parsed.Error.Message}
	}
	return nil
}

func gunzip(payload []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	return io.ReadAll(reader)
}
