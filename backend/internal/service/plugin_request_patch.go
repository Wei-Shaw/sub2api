package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"golang.org/x/net/http/httpguts"
	"google.golang.org/grpc"
)

const (
	pluginRequestPatchTimeout   = 5 * time.Second
	pluginRequestPatchMaxHeader = 64 << 10
	pluginRequestPatchMaxFields = 256
)

// PatchAnthropicOAuthRequest 仅修改最终出站请求，实际发送与响应处理仍由宿主完成。
// 未绑定时保持原行为；命中绑定后的补丁异常必须明确返回，不能静默绕过插件。
func (m *PluginManager) PatchAnthropicOAuthRequest(ctx context.Context, request *http.Request, body []byte, account *Account) (*http.Request, []byte, error) {
	if m == nil || account == nil || account.Platform != PlatformAnthropic || account.Type != AccountTypeOAuth {
		return request, body, nil
	}
	route := m.anthropicRoute.Load()
	if route == nil || route.rolloutPercent <= 0 || int(stablePluginBucket(account.ID)) >= route.rolloutPercent {
		return request, body, nil
	}
	if request == nil || request.URL == nil {
		return nil, nil, errors.New("Anthropic OAuth 请求补丁缺少请求参数")
	}
	if route.runtime == nil {
		return nil, nil, fmt.Errorf("Anthropic OAuth 插件不可用: %s", route.unavailable)
	}
	runtime := route.runtime
	if runtime.client == nil || runtime.client.Exited() {
		err := errors.New("Anthropic OAuth 插件进程已退出")
		return nil, nil, errors.Join(err, m.markRuntimeUnavailable(route, err.Error()))
	}
	if runtime.patcher == nil {
		return nil, nil, errors.New("Anthropic OAuth 插件未提供请求补丁服务")
	}
	if !runtime.beginRequest() {
		return nil, nil, errors.New("Anthropic OAuth 插件正在停止")
	}
	defer runtime.finishRequest()
	metadata, err := pluginPatchRequestMetadata(request, body, account)
	if err != nil {
		return nil, nil, err
	}
	patchCtx, cancel := context.WithTimeout(ctx, pluginRequestPatchTimeout)
	defer cancel()
	patch, err := runtime.patcher.PatchRequest(patchCtx, &pluginv1.RequestPatchRequest{
		Request: metadata, BodyJson: body,
	}, grpc.MaxCallSendMsgSize(pluginv1.RequestPatchMaxMessageBytes), grpc.MaxCallRecvMsgSize(pluginv1.RequestPatchMaxMessageBytes))
	if err != nil {
		if runtime.client.Exited() {
			_ = m.markRuntimeUnavailable(route, "请求补丁服务进程已退出")
		}
		return nil, nil, normalizePluginRPCError(patchCtx, "应用 Anthropic OAuth 请求补丁", err, false)
	}
	return applyPluginRequestPatch(request, body, patch)
}

func pluginPatchProtectedHeader(key string) bool {
	switch strings.ToLower(key) {
	case "authorization", "proxy-authorization", "x-api-key", "api-key", "cookie", "set-cookie",
		"host", "connection", "proxy-connection", "keep-alive", "te", "trailer", "transfer-encoding", "upgrade",
		"content-length", "content-encoding", "content-type", "expect", "forwarded", "x-goog-api-key":
		return true
	default:
		return strings.HasPrefix(strings.ToLower(key), "x-forwarded-")
	}
}

func pluginPatchRequestMetadata(request *http.Request, body []byte, account *Account) (*pluginv1.ForwardRequestStart, error) {
	if _, err := pluginPatchJSONObject(body); err != nil {
		return nil, fmt.Errorf("插件请求正文: %w", err)
	}
	headers := make(http.Header)
	for key, values := range request.Header {
		if !pluginPatchProtectedHeader(key) {
			headers[key] = append([]string(nil), values...)
		}
	}
	if err := validatePluginPatchHeaders(headersToPlugin(headers), nil); err != nil {
		return nil, err
	}
	// URL 查询串和 userinfo 可能带有自定义中继、代理凭据，补丁服务不需要这些信息。
	endpoint := *request.URL
	endpoint.User, endpoint.RawQuery, endpoint.Fragment = nil, "", ""
	endpoint.ForceQuery = false
	return &pluginv1.ForwardRequestStart{
		RequestId: strconv.FormatInt(time.Now().UnixNano(), 36), Method: request.Method,
		Url: endpoint.String(), Host: endpoint.Host, Headers: headersToPlugin(headers),
		AccountId: account.ID, AccountConcurrency: int32(account.Concurrency),
		Platform: account.Platform, AccountType: account.Type,
		ContentLength: int64(len(body)), HasBody: len(body) > 0,
	}, nil
}

func pluginPatchJSONObject(body []byte) (map[string]json.RawMessage, error) {
	if len(body) == 0 || len(body) > pluginv1.RequestPatchMaxBodyBytes {
		return nil, errors.New("正文必须是大小不超过 64 MiB 的 JSON 对象")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil || object == nil {
		return nil, errors.New("正文必须是有效的 JSON 对象")
	}
	// 拒绝重复的顶层字段，避免不同 JSON 解析器对 model/stream 取值不一致。
	decoder := json.NewDecoder(bytes.NewReader(body))
	_, _ = decoder.Token()
	seen := make(map[string]bool, len(object))
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := key.(string)
		if !ok {
			return nil, errors.New("JSON 对象字段名称必须是字符串")
		}
		if seen[name] {
			return nil, fmt.Errorf("JSON 对象包含重复的顶层字段: %s", name)
		}
		seen[name] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
	}
	return object, nil
}

func validatePluginPatchHeaders(set map[string]*pluginv1.HeaderValues, remove []string) error {
	if len(set)+len(remove) > pluginRequestPatchMaxFields {
		return errors.New("请求补丁的请求头数量超过限制")
	}
	size := 0
	valueCount := len(remove)
	seen := make(map[string]bool, len(set))
	for key, values := range set {
		lower := strings.ToLower(key)
		if !httpguts.ValidHeaderFieldName(key) || pluginPatchProtectedHeader(key) || seen[lower] {
			return fmt.Errorf("请求补丁包含不允许修改或重复的请求头: %q", key)
		}
		seen[lower] = true
		size += len(key)
		if values != nil {
			valueCount += len(values.Values)
			if valueCount > pluginRequestPatchMaxFields {
				return errors.New("请求补丁的请求头值数量超过限制")
			}
			for _, value := range values.Values {
				if !httpguts.ValidHeaderFieldValue(value) {
					return errors.New("请求补丁包含无效的请求头值")
				}
				size += len(value)
				if size > pluginRequestPatchMaxHeader {
					return errors.New("请求补丁的请求头大小超过限制")
				}
			}
		}
	}
	for _, key := range remove {
		if !httpguts.ValidHeaderFieldName(key) || pluginPatchProtectedHeader(key) {
			return fmt.Errorf("请求补丁不允许移除请求头: %q", key)
		}
		size += len(key)
	}
	if size > pluginRequestPatchMaxHeader {
		return errors.New("请求补丁的请求头大小超过限制")
	}
	return nil
}

func applyPluginRequestPatch(request *http.Request, body []byte, patch *pluginv1.RequestPatchResponse) (*http.Request, []byte, error) {
	if patch == nil {
		return nil, nil, errors.New("插件未返回请求补丁结果")
	}
	if err := validatePluginPatchHeaders(patch.SetHeaders, patch.RemoveHeaders); err != nil {
		return nil, nil, err
	}
	nextBody := body
	if len(patch.BodyJson) > 0 {
		original, err := pluginPatchJSONObject(body)
		if err != nil {
			return nil, nil, err
		}
		next, err := pluginPatchJSONObject(patch.BodyJson)
		if err != nil {
			return nil, nil, err
		}
		// 模型和流模式参与路由、计费与响应解析，不能由请求补丁改变。
		for _, field := range []string{"model", "stream"} {
			before, beforeExists := original[field]
			after, afterExists := next[field]
			var beforeValue, afterValue any
			_ = json.Unmarshal(before, &beforeValue)
			_ = json.Unmarshal(after, &afterValue)
			if beforeExists != afterExists || !reflect.DeepEqual(beforeValue, afterValue) {
				return nil, nil, fmt.Errorf("请求补丁不允许改变 %s", field)
			}
		}
		nextBody = bytes.Clone(patch.BodyJson)
	}
	// 校验完成后再构造新请求，任何异常都不会留下部分应用的请求头或正文。
	next := request.Clone(request.Context())
	for _, key := range patch.RemoveHeaders {
		deleteHeaderAllForms(next.Header, key)
	}
	for key, values := range patch.SetHeaders {
		deleteHeaderAllForms(next.Header, key)
		if values != nil && len(values.Values) > 0 {
			next.Header[key] = append([]string(nil), values.Values...)
		}
	}
	if len(patch.BodyJson) > 0 {
		next.Body = io.NopCloser(bytes.NewReader(nextBody))
		next.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(nextBody)), nil }
		next.ContentLength = int64(len(nextBody))
		for _, key := range []string{"Content-Length", "Content-MD5", "Digest", "Content-Digest", "Repr-Digest"} {
			deleteHeaderAllForms(next.Header, key)
		}
	}
	return next, nextBody, nil
}

// SetPluginManager 在启动装配时注入，避免网关构造器与插件管理器之间形成依赖环。
func (s *GatewayService) SetPluginManager(manager *PluginManager) {
	s.pluginManager = manager
}
