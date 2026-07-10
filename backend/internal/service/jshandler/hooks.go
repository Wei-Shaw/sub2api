package jshandler

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/dop251/goja"
)

type RequestHookInput struct {
	Body            []byte
	Headers         http.Header
	Model           string
	SourceFormat    string
	ToFormat        string
	AccountPlatform string
	MappedModel     string
	RequestID       string
}

type RequestHookOutput struct {
	Body         []byte
	Headers      http.Header
	ClearHeaders []string
}

type ResponseHookInput struct {
	Body            []byte
	RequestBody     []byte
	RequestHeaders  map[string]any
	ResponseHeaders http.Header
	Model           string
	Protocol        string
	RequestID       string
}

type ResponseHookOutput struct {
	Body         []byte
	Headers      http.Header
	ClearHeaders []string
}

type StreamChunkHookInput struct {
	Chunk           string
	HistoryChunks   []string
	RequestBody     []byte
	RequestHeaders  map[string]any
	ResponseHeaders http.Header
	Model           string
	Protocol        string
	RequestID       string
}

type StreamChunkHookOutput struct {
	Chunk      string
	DropChunk  bool
	Headers    http.Header
	ClearHeaders []string
}

func applyJSRequestHook(scriptPath, hookName string, timeout time.Duration, in RequestHookInput) (RequestHookOutput, error) {
	out := RequestHookOutput{
		Body:    append([]byte(nil), in.Body...),
		Headers: cloneHeader(in.Headers),
	}
	program, err := getJSProgram(scriptPath)
	if err != nil {
		return out, err
	}
	engine := newJSEngine(nil)
	jsCtx := map[string]any{
		"id":            in.RequestID,
		"body":          string(in.Body),
		"headers":       headerToAnyMap(in.Headers),
		"url":           "",
		"model":         in.Model,
		"protocol":      in.SourceFormat,
		"source_format": in.SourceFormat,
		"to_format":     in.ToFormat,
		"sourceFormat":  in.SourceFormat,
		"toFormat":      in.ToFormat,
	}
	if in.AccountPlatform != "" {
		jsCtx["account_platform"] = in.AccountPlatform
	}
	if in.MappedModel != "" {
		jsCtx["mapped_model"] = in.MappedModel
	}
	jsVal, err := engine.runProgramAndCall(program, hookName, timeout, jsCtx)
	if err != nil {
		if errors.Is(err, ErrFunctionNotFound) {
			return out, nil
		}
		return out, err
	}
	return exportRequestHookResult(jsVal, out)
}

func exportRequestHookResult(jsVal goja.Value, out RequestHookOutput) (RequestHookOutput, error) {
	if jsVal == nil || goja.IsUndefined(jsVal) || goja.IsNull(jsVal) {
		return out, nil
	}
	exported := jsVal.Export()
	if exported == nil {
		return out, nil
	}
	var clearHeaders []string
	if objMap, ok := exported.(map[string]any); ok {
		if headersVal, exists := objMap["headers"]; exists {
			clearHeaders = updateHeaderFromAny(out.Headers, headersVal)
		}
		if bodyVal, exists := objMap["body"]; exists {
			if bodyStr, ok := bodyVal.(string); ok {
				out.Body = []byte(bodyStr)
				out.ClearHeaders = clearHeaders
				return out, nil
			}
		}
		out.ClearHeaders = clearHeaders
		return out, nil
	}
	if bodyStr, ok := exported.(string); ok {
		out.Body = []byte(bodyStr)
		return out, nil
	}
	return out, nil
}

func applyJSNonStreamResponseHook(scriptPath string, timeout time.Duration, in ResponseHookInput) (ResponseHookOutput, error) {
	out := ResponseHookOutput{
		Body:    append([]byte(nil), in.Body...),
		Headers: cloneHeader(in.ResponseHeaders),
	}
	program, err := getJSProgram(scriptPath)
	if err != nil {
		return out, err
	}
	engine := newJSEngine(nil)
	reqCtx := map[string]any{
		"body":    string(in.RequestBody),
		"headers": in.RequestHeaders,
		"url":     "",
	}
	jsCtx := map[string]any{
		"id":             in.RequestID,
		"body":           string(in.Body),
		"req":            reqCtx,
		"protocol":       in.Protocol,
		"headers":        headerToAnyMap(in.ResponseHeaders),
		"chunk":          nil,
		"history_chunks": nil,
	}
	jsVal, err := engine.runProgramAndCall(program, "on_after_nonstream_response", timeout, jsCtx)
	if err != nil {
		if errors.Is(err, ErrFunctionNotFound) {
			return out, nil
		}
		return out, err
	}
	return exportResponseHookResult(jsVal, out)
}

func exportStreamChunkHookResult(jsVal goja.Value, out StreamChunkHookOutput) (StreamChunkHookOutput, error) {
	if jsVal == nil || goja.IsUndefined(jsVal) || goja.IsNull(jsVal) {
		return out, nil
	}
	exported := jsVal.Export()
	if exported == nil {
		return out, nil
	}
	var clearHeaders []string
	if objMap, ok := exported.(map[string]any); ok {
		if headersVal, exists := objMap["headers"]; exists {
			clearHeaders = updateHeaderFromAny(out.Headers, headersVal)
		}
		if chunkVal, exists := objMap["chunk"]; exists {
			if cStr, ok := chunkVal.(string); ok {
				out.Chunk = cStr
				out.ClearHeaders = clearHeaders
				if cStr == "" {
					out.DropChunk = true
				}
				return out, nil
			}
		}
		out.ClearHeaders = clearHeaders
		return out, nil
	}
	if strVal, ok := exported.(string); ok {
		out.Chunk = strVal
		if strVal == "" {
			out.DropChunk = true
		}
		return out, nil
	}
	return out, nil
}

func exportResponseHookResult(jsVal goja.Value, out ResponseHookOutput) (ResponseHookOutput, error) {
	if jsVal == nil || goja.IsUndefined(jsVal) || goja.IsNull(jsVal) {
		return out, nil
	}
	exported := jsVal.Export()
	if exported == nil {
		return out, nil
	}
	var clearHeaders []string
	if objMap, ok := exported.(map[string]any); ok {
		if headersVal, exists := objMap["headers"]; exists {
			clearHeaders = updateHeaderFromAny(out.Headers, headersVal)
		}
		if bodyVal, exists := objMap["body"]; exists {
			if bStr, ok := bodyVal.(string); ok {
				out.Body = []byte(bStr)
				out.ClearHeaders = clearHeaders
				return out, nil
			}
		}
		out.ClearHeaders = clearHeaders
		return out, nil
	}
	if strVal, ok := exported.(string); ok {
		out.Body = []byte(strVal)
		return out, nil
	}
	return out, nil
}

func headerToAnyMap(h http.Header) map[string]any {
	m := make(map[string]any)
	if h == nil {
		return m
	}
	for k, v := range h {
		switch len(v) {
		case 0:
			continue
		case 1:
			m[k] = v[0]
		default:
			m[k] = append([]string(nil), v...)
		}
	}
	return m
}

func updateHeaderFromAny(h http.Header, val interface{}) []string {
	var clearHeaders []string
	if h == nil || val == nil {
		return clearHeaders
	}
	rv := reflect.ValueOf(val)
	if rv.Kind() != reflect.Map {
		return clearHeaders
	}
	for _, key := range rv.MapKeys() {
		kStr := key.String()
		vVal := rv.MapIndex(key).Interface()
		if vVal == nil {
			h.Del(kStr)
			clearHeaders = append(clearHeaders, kStr)
		} else if valStr, ok := vVal.(string); ok {
			h.Set(kStr, valStr)
		} else {
			values, ok := stringSliceFromAny(vVal)
			if !ok {
				h.Set(kStr, fmt.Sprintf("%v", vVal))
				continue
			}
			if len(values) == 0 {
				h.Del(kStr)
				clearHeaders = append(clearHeaders, kStr)
			} else {
				h[http.CanonicalHeaderKey(kStr)] = values
			}
		}
	}
	return clearHeaders
}

func cloneHeader(h http.Header) http.Header {
	cloned := make(http.Header, len(h))
	for key, values := range h {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}

func stringSliceFromAny(val any) ([]string, bool) {
	switch typed := val.(type) {
	case []string:
		return append([]string(nil), typed...), true
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			itemStr, ok := item.(string)
			if !ok {
				return nil, false
			}
			values = append(values, itemStr)
		}
		return values, true
	}
	rv := reflect.ValueOf(val)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}
	values := make([]string, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		item, ok := rv.Index(i).Interface().(string)
		if !ok {
			return nil, false
		}
		values = append(values, item)
	}
	return values, true
}