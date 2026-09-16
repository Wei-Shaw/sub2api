package adobe

import (
	"encoding/json"
	"errors"
	"strings"
)

// 错误来源分类，对应 TS 侧的 AdobeErrorType。
const (
	ErrorTypeStatus     = "status"     // 上游返回了非 2xx 状态码
	ErrorTypeTimeout    = "timeout"    // 请求超时
	ErrorTypeConnection = "connection" // 连接失败
	ErrorTypeProxy      = "proxy"      // 代理故障
	ErrorTypeNetwork    = "network"    // 其它网络层错误
)

// RequestError 是 Firefly 直连的基础错误类型，同时也是「终态错误」的语义：
// 请求本身有问题（4xx 校验失败、内容拒绝、模型不支持等），换账号重试无用。
//
// AuthError / QuotaExhaustedError / NotEntitledError / UpstreamTemporaryError /
// ContentRejectedError 内嵌本类型并实现 Unwrap，因此 errors.As(err, &*RequestError)
// 对六者都成立；要区分具体类别时用对应的具体类型做 errors.As。
type RequestError struct {
	Message string
	// StatusCode 是上游 HTTP 状态码；网络层错误时为 0。
	StatusCode int
	// ErrorType 是来源分类，取上面的 ErrorType* 常量。
	ErrorType string
	// UserMessage 是可直接展示给终端用户的文案；为空时回落到 Message。
	UserMessage string
}

func (e *RequestError) Error() string { return e.Message }

// User 返回面向用户的文案。
func (e *RequestError) User() string {
	if msg := strings.TrimSpace(e.UserMessage); msg != "" {
		return msg
	}
	return strings.TrimSpace(e.Message)
}

// AuthError 表示 token 失效或过期（401/403）。
type AuthError struct{ RequestError }

func (e *AuthError) Unwrap() error { return &e.RequestError }

// QuotaExhaustedError 表示该 Adobe 账号配额耗尽
// （401/403 且响应头 x-access-error: taste_exhausted）。
type QuotaExhaustedError struct{ RequestError }

func (e *QuotaExhaustedError) Unwrap() error { return &e.RequestError }

// NotEntitledError 表示账号没有请求的模型或质量档权益（典型：403 +
// error_code=model_not_entitled / user_not_entitled）。Cookie 仍有效，只是套餐不够；
// 组里另一张更高档的号可能成功，故可换号，但不能当成 token 失效去刷新凭据。
type NotEntitledError struct{ RequestError }

func (e *NotEntitledError) Unwrap() error { return &e.RequestError }

// UpstreamTemporaryError 表示上游临时故障（429/451/5xx 或网络层），可重试。
type UpstreamTemporaryError struct{ RequestError }

func (e *UpstreamTemporaryError) Unwrap() error { return &e.RequestError }

// ContentRejectedError 表示上游按内容安全拒绝了这次请求（典型：451 +
// error_code=image_unsafe）。换另一个 Firefly Cookie 号也会被同一 prompt 打中，
// 只能跳到协议不同的中转号，或直接把拒绝回给调用方。
type ContentRejectedError struct{ RequestError }

func (e *ContentRejectedError) Unwrap() error { return &e.RequestError }

// ErrorCodeImageUnsafe 是 Firefly 内容安全拒绝出图时的 error_code。
const ErrorCodeImageUnsafe = "image_unsafe"

// Firefly 在账号没有该模型 / 质量档权益时返回的 error_code（也见于 x-access-error）。
const (
	ErrorCodeModelNotEntitled = "model_not_entitled"
	ErrorCodeUserNotEntitled  = "user_not_entitled"
)

const notEntitledUserMessage = "This Adobe account is not entitled to the requested model or quality"

// NewRequestError 构造终态错误。
func NewRequestError(message string) *RequestError {
	return &RequestError{Message: message}
}

// NewAuthError 构造鉴权错误。
func NewAuthError(message string, statusCode int) *AuthError {
	return &AuthError{RequestError{Message: message, StatusCode: statusCode, ErrorType: ErrorTypeStatus}}
}

// NewQuotaExhaustedError 构造配额耗尽错误。
func NewQuotaExhaustedError(message string, statusCode int) *QuotaExhaustedError {
	return &QuotaExhaustedError{RequestError{Message: message, StatusCode: statusCode, ErrorType: ErrorTypeStatus}}
}

// NewNotEntitledError 构造权益不足错误。userMessage 对外展示；为空时用固定英文，
// 避免把上游 JSON 泄漏给客户端，也避免被误读成 token 过期。
func NewNotEntitledError(message string, statusCode int, userMessage string) *NotEntitledError {
	if strings.TrimSpace(userMessage) == "" {
		userMessage = notEntitledUserMessage
	}
	return &NotEntitledError{RequestError{
		Message:     message,
		StatusCode:  statusCode,
		ErrorType:   ErrorTypeStatus,
		UserMessage: userMessage,
	}}
}

// NewUpstreamTemporaryError 构造上游临时错误。
func NewUpstreamTemporaryError(message string, statusCode int, errorType string) *UpstreamTemporaryError {
	if errorType == "" {
		errorType = ErrorTypeStatus
	}
	return &UpstreamTemporaryError{RequestError{Message: message, StatusCode: statusCode, ErrorType: errorType}}
}

// NewContentRejectedError 构造内容安全拒绝。userMessage 对外展示；为空时用
// 固定英文，避免把上游整段 JSON 泄漏给客户端。
func NewContentRejectedError(message string, statusCode int, userMessage string) *ContentRejectedError {
	if strings.TrimSpace(userMessage) == "" {
		userMessage = "Image content was rejected by the upstream safety filter"
	}
	return &ContentRejectedError{RequestError{
		Message:     message,
		StatusCode:  statusCode,
		ErrorType:   ErrorTypeStatus,
		UserMessage: userMessage,
	}}
}

// IsRetryableStatus 判定状态码是否属于可重试的上游临时故障。
//
// 408 在这里不是「客户端请求超时」的字面含义：Adobe 用它表达降载
// （body 形如 {"error_code":"timeout_error","message":"system under load"}）。
// 归为可重试后，网关侧 runAdobeImagesFailover 会换下一个账号，
// 测试弹窗也会走 formatAdobeTestError 的 UpstreamTemporaryError 分支拿到友好文案；
// 此前它落成普通 RequestError，一次瞬时限流就让整个请求失败且不换号。
func IsRetryableStatus(status int) bool {
	return status == 408 || status == 429 || status == 451 || status >= 500
}

// IsRotatable 判定错误是否属于「换 token/账号可能成功」类：上游临时故障、账号配额
// 耗尽、账号权益不足、token 鉴权失效、以及内容安全拒绝（后者不能再换 Firefly Cookie
// 号，但可以跳到组内 OpenAI 形中转号）。
//
// 非此类（请求本身 4xx、模型不支持等）换号也无用，应直接上抛。
func IsRotatable(err error) bool {
	if err == nil {
		return false
	}
	var temporary *UpstreamTemporaryError
	if errors.As(err, &temporary) {
		return true
	}
	var quota *QuotaExhaustedError
	if errors.As(err, &quota) {
		return true
	}
	var entitled *NotEntitledError
	if errors.As(err, &entitled) {
		return true
	}
	var auth *AuthError
	if errors.As(err, &auth) {
		return true
	}
	var rejected *ContentRejectedError
	return errors.As(err, &rejected)
}

// ErrorCode 从上游 JSON 里取出 error_code；解析失败或字段缺失时返回空串。
func ErrorCode(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	var parsed struct {
		ErrorCode string `json:"error_code"`
	}
	if json.Unmarshal([]byte(body), &parsed) != nil {
		return ""
	}
	return strings.TrimSpace(parsed.ErrorCode)
}

// IsContentRejectedCode 判定 error_code 是否属于内容安全拒绝。
func IsContentRejectedCode(code string) bool {
	return strings.EqualFold(strings.TrimSpace(code), ErrorCodeImageUnsafe)
}

// IsContentRejectedBody 判定响应体是否带有内容安全拒绝的 error_code。
func IsContentRejectedBody(body string) bool {
	return IsContentRejectedCode(ErrorCode(body))
}

// IsNotEntitledCode 判定 error_code / x-access-error 是否属于套餐权益不足。
func IsNotEntitledCode(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case ErrorCodeModelNotEntitled, ErrorCodeUserNotEntitled:
		return true
	default:
		return false
	}
}

// IsNotEntitledBody 判定响应体是否带有权益不足的 error_code。
func IsNotEntitledBody(body string) bool {
	return IsNotEntitledCode(ErrorCode(body))
}
