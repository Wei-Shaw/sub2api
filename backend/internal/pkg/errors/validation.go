package errors

import (
	"encoding/json"
	"strconv"
)

// FieldError 描述单个字段的校验失败。
//
// Path 用 JSON 路径字符串（如 "limiters[0].limit_value"）让前端能据此定位 UI 控件做行内红字渲染；
// Code 是错误码常量（如 REQUIRED / MUST_BE_POSITIVE / INVALID_VALUE），前端按 code 查 i18n 表。
//
// 这是项目级共享类型：service quota 的字段校验、handler 层 binding 错降级、其它 admin 模块的
// 表单校验都共用同一份结构，让前端 parseFieldErrors 不需要按 reason 分支。
type FieldError struct {
	Path string `json:"path"`
	Code string `json:"code"`
}

// FieldErrorCollector 收集多个 FieldError，最后一次性构造成 ApplicationError 返回。
//
// 设计要点：
//   - reason 由调用方传入（例如 SERVICE_QUOTA_VALIDATION_ERROR / INVALID_REQUEST_BODY），
//     共享 collector 不绑定具体业务错误码
//   - HasErrors() / Add() 让调用方可以条件累加；零错误时 Build() 返回 nil
//     以保持 if err := ...; err != nil { ... } 调用风格
//   - Build() 把 fields 切片 JSON 序列化塞进 metadata.fields（Metadata 是 map[string]string，
//     不能直接放数组对象），前端 JSON.parse 后渲染
type FieldErrorCollector struct {
	reason  string
	message string
	fields  []FieldError
}

// NewFieldErrorCollector 构造一个收集器。
//
// reason 是顶层 ApplicationError.Reason，前端按它识别"字段级校验错误"分支；
// message 是英文供开发者排查（前端 i18n 时不展示），缺省用 "validation failed"。
func NewFieldErrorCollector(reason string) *FieldErrorCollector {
	return &FieldErrorCollector{reason: reason, message: "validation failed"}
}

// WithMessage 自定义英文 message（替换默认 "validation failed"）。链式 API。
func (c *FieldErrorCollector) WithMessage(msg string) *FieldErrorCollector {
	c.message = msg
	return c
}

// Add 追加一个字段错误。
//
// path 是 JSON 路径字符串（嵌套场景调用方负责拼装："limiters[0].limit_value"）；
// code 必须是与前端 i18n 表对齐的常量字符串。空 path/code 也不报错——业务自行决定是否兜底。
func (c *FieldErrorCollector) Add(path, code string) {
	c.fields = append(c.fields, FieldError{Path: path, Code: code})
}

// HasErrors 是否已收集到任何字段错误。让调用方在 build 之前可以做条件分支。
func (c *FieldErrorCollector) HasErrors() bool {
	return len(c.fields) > 0
}

// Build 把收集到的字段错误打包成 ApplicationError。
//
// 没有错误时返回 nil（让 if err == nil 的检查惯性保持）。JSON 序列化失败兜底为只带 count 的错误，
// 让校验入口永远能给出可读响应。
func (c *FieldErrorCollector) Build() *ApplicationError {
	if !c.HasErrors() {
		return nil
	}
	return ValidationFailed(c.reason, c.fields).withMessageInternal(c.message)
}

// ValidationFailed 是字段级校验失败的便利构造器。
//
// 用于不需要逐步累加的场景（例如 BindJSONOrError 一次性把 validator.ValidationErrors 转成 fields）：
//
//	pkgerrors.ValidationFailed("INVALID_REQUEST_BODY", []pkgerrors.FieldError{
//	    {Path: "name", Code: "REQUIRED"},
//	})
//
// 内部把 fields 序列化为 JSON 字符串塞进 metadata.fields；JSON 失败兜底带 count。
// fields == nil / 空切片时仍会构造非 nil 错误（reason 已经是有意义的标识），不要传空切片调用此函数。
func ValidationFailed(reason string, fields []FieldError) *ApplicationError {
	meta := map[string]string{"count": strconv.Itoa(len(fields))}
	if payload, err := json.Marshal(fields); err == nil {
		meta["fields"] = string(payload)
	}
	return BadRequest(reason, "validation failed").WithMetadata(meta)
}

// withMessageInternal 是 Build() 用来在 BadRequest 之上覆盖默认 message 的内部 helper。
// 不暴露：调用方通过 WithMessage(msg).Build() 触达。
func (e *ApplicationError) withMessageInternal(msg string) *ApplicationError {
	clone := Clone(e)
	clone.Message = msg
	return clone
}
