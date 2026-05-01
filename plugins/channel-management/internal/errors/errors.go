// Package errors provides a minimal copy of the core's ApplicationError
// abstraction so the plugin's services and handlers can raise structured
// errors without importing the core's internal package.
//
// The shape mirrors backend/internal/pkg/errors so HTTP responses produced by
// the plugin look identical to the rest of the API.
package errors

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// causeLogMaxLen 是 cause error 在结构化日志中允许的最大字节数。设置
// 在 500 字节，覆盖典型的 SQL/HTTP 错误结构 (`{"error": ...}`、`pq: ...`)
// 同时阻断把整条参数化查询 / 用户输入回贴到日志里。
const causeLogMaxLen = 500

// causeRedactedTokens 是命中后 cause 文本会被截断的关键词。它们通常出现
// 在数据库 / 网络错误中，紧跟其后的就是用户输入或敏感参数（搜索字符串、
// 密文片段、查询绑定值等）。命中后只保留关键词前的部分加 "..." 后缀。
var causeRedactedTokens = []string{
	" detail:", " DETAIL:",
	" hint:", " HINT:",
	" where:", " WHERE:",
	" query:", " QUERY:",
	" SQL:",
}

// SanitizeCauseForLog 把任意 cause error 的文本压缩到 causeLogMaxLen，
// 并在命中 SQL 错误中常见的敏感关键词后截断。返回值适合给 slog 的 "error"
// 字段使用，避免把搜索文本 / 参数化查询的绑定值写入日志。
func SanitizeCauseForLog(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	for _, tok := range causeRedactedTokens {
		// 大小写敏感比较以避免误伤合法文本；上面 token 列表已覆盖大写/小写两种。
		if idx := strings.Index(msg, tok); idx >= 0 {
			msg = msg[:idx] + " ...[redacted]"
			break
		}
	}
	if len(msg) > causeLogMaxLen {
		msg = msg[:causeLogMaxLen] + "..."
	}
	return msg
}

const (
	UnknownCode    = http.StatusInternalServerError
	UnknownReason  = ""
	UnknownMessage = "internal error"
)

// Status is the JSON-serialisable shape of an error.
type Status struct {
	Code     int32             `json:"code"`
	Reason   string            `json:"reason,omitempty"`
	Message  string            `json:"message"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ApplicationError carries an HTTP status code, a stable reason key, a
// human-readable message and optional metadata.
type ApplicationError struct {
	Status
	cause error
}

func (e *ApplicationError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.cause == nil {
		return fmt.Sprintf("error: code=%d reason=%q message=%q metadata=%v", e.Code, e.Reason, e.Message, e.Metadata)
	}
	// cause 走 sanitize 通道：截断 + 去除 SQL DETAIL/QUERY 等可能含用户输入的字段。
	// 调用方还可以通过 errors.Unwrap(e) 拿到完整 cause 用作内部诊断，但 Error()
	// 输出（往往会进结构化日志）只暴露脱敏后的文本。
	return fmt.Sprintf("error: code=%d reason=%q message=%q metadata=%v cause=%s",
		e.Code, e.Reason, e.Message, e.Metadata, SanitizeCauseForLog(e.cause))
}

func (e *ApplicationError) Unwrap() error { return e.cause }

func (e *ApplicationError) Is(err error) bool {
	if se := new(ApplicationError); errors.As(err, &se) {
		return se.Code == e.Code && se.Reason == e.Reason
	}
	return false
}

func (e *ApplicationError) WithCause(cause error) *ApplicationError {
	out := Clone(e)
	out.cause = cause
	return out
}

func (e *ApplicationError) WithMetadata(md map[string]string) *ApplicationError {
	out := Clone(e)
	if md == nil {
		out.Metadata = nil
		return out
	}
	out.Metadata = make(map[string]string, len(md))
	for k, v := range md {
		out.Metadata[k] = v
	}
	return out
}

func New(code int, reason, message string) *ApplicationError {
	return &ApplicationError{Status: Status{Code: int32(code), Message: message, Reason: reason}}
}

func Clone(err *ApplicationError) *ApplicationError {
	if err == nil {
		return nil
	}
	var metadata map[string]string
	if err.Metadata != nil {
		metadata = make(map[string]string, len(err.Metadata))
		for k, v := range err.Metadata {
			metadata[k] = v
		}
	}
	return &ApplicationError{
		cause:  err.cause,
		Status: Status{Code: err.Code, Reason: err.Reason, Message: err.Message, Metadata: metadata},
	}
}

func FromError(err error) *ApplicationError {
	if err == nil {
		return nil
	}
	if se := new(ApplicationError); errors.As(err, &se) {
		return se
	}
	return New(UnknownCode, UnknownReason, UnknownMessage).WithCause(err)
}

// BadRequest returns a 400 ApplicationError.
func BadRequest(reason, message string) *ApplicationError {
	return New(http.StatusBadRequest, reason, message)
}

// NotFound returns a 404 ApplicationError.
func NotFound(reason, message string) *ApplicationError {
	return New(http.StatusNotFound, reason, message)
}

// Conflict returns a 409 ApplicationError.
func Conflict(reason, message string) *ApplicationError {
	return New(http.StatusConflict, reason, message)
}

// InternalServer returns a 500 ApplicationError.
func InternalServer(reason, message string) *ApplicationError {
	return New(http.StatusInternalServerError, reason, message)
}

// ToHTTP turns any error into a status code + JSON body.
func ToHTTP(err error) (int, Status) {
	if err == nil {
		return http.StatusOK, Status{Code: int32(http.StatusOK)}
	}
	appErr := FromError(err)
	if appErr == nil {
		return http.StatusOK, Status{Code: int32(http.StatusOK)}
	}
	body := Status{Code: appErr.Code, Reason: appErr.Reason, Message: appErr.Message}
	if appErr.Metadata != nil {
		body.Metadata = make(map[string]string, len(appErr.Metadata))
		for k, v := range appErr.Metadata {
			body.Metadata[k] = v
		}
	}
	return int(appErr.Code), body
}
