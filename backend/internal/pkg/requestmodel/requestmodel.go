// Package requestmodel 统一从网关请求中提取客户端请求的模型名。
//
// 提取顺序与位置（JSON `model` / `session.model`、multipart `model` / `session`）
// 与合成路由中间件、分组模型白名单中间件共用，保证改写与准入看到同一个模型。
//
// 网关里同时存在三类下游解析器，同一请求体可能被解析出不同模型值：
//   - gjson 系（大小写敏感、重复键取第一个）；
//   - encoding/json 绑定系（大小写不敏感、重复键取最后一个）；
//   - multipart 表单系（字段可重复，不同 handler 取首个或末个）。
//
// 因此准入侧的 FromBodyCandidates 返回「任一下游解析器可能绑定到的全部模型值」，
// 调用方必须逐一校验；FromBodyForRoute 返回首个候选值，仅供合成路由分发使用。
package requestmodel

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/tidwall/gjson"
)

// liveRequestRoutes 是模型位于 session.model（而非顶层 model）的入口，
// 与 Live handler 的解析保持一致。
var liveRequestRoutes = map[string]bool{
	"/v1/live":                          true,
	"/live":                             true,
	"/backend-api/codex/realtime/calls": true,
}

// IsLiveRequestRoute 报告该路由模板的 handler 是否只读取 session.model。
func IsLiveRequestRoute(routePath string) bool {
	return liveRequestRoutes[strings.TrimSpace(routePath)]
}

// FromBodyCandidates 按入口返回所有可被下游解析器绑定的模型值：
// Live 入口取 session 候选（为空回落顶层候选），其余入口取顶层候选
// （为空回落 session 候选）。候选包含重复键、大小写变体的全部出现。
func FromBodyCandidates(routePath, contentType string, body []byte) []string {
	var topLevel, session []string
	if isMultipartContentType(contentType) {
		// 声明为 multipart 的请求体只做 multipart 解析——gjson 的部分扫描
		// 会在 multipart 字节流里误匹配 session JSON 内的 model 字段。
		models, sessions := multipartModelCandidates(contentType, body)
		topLevel = models
		session = sessionModelCandidates(sessions)
	} else {
		topLevel = jsonModelCandidates(body)
		session = jsonSessionModelCandidates(body)
	}

	if IsLiveRequestRoute(routePath) {
		if len(session) > 0 {
			return session
		}
		return topLevel
	}
	if len(topLevel) > 0 {
		return topLevel
	}
	return session
}

// FromBody 从 JSON 或 multipart 请求体提取客户端请求的模型名（首个候选）；
// 提取不到返回空串。顶层 `model` 优先，其次 `session.model`（Live 入口之外
// 的地方兜底）。准入校验请改用 FromBodyCandidates。
func FromBody(contentType string, body []byte) string {
	return FromBodyForRoute("", contentType, body)
}

// FromBodyForRoute 按入口选择与 handler 一致的模型提取规则，返回首个候选值，
// 仅供合成路由分发使用；准入校验必须用 FromBodyCandidates 的全部候选。
func FromBodyForRoute(routePath, contentType string, body []byte) string {
	candidates := FromBodyCandidates(routePath, contentType, body)
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

// FromJSON 从 JSON 请求体提取模型名：顶层 `model` 优先，其次 `session.model`（Live）。
func FromJSON(body []byte) string {
	model, _ := JSONModelPath(body)
	return model
}

// JSONModelPath 返回 JSON 请求体中的模型名与其 gjson 路径（用于合成路由改写定位）。
// 只匹配精确大小写的键；大小写变体由 FromBodyCandidates 的候选集覆盖。
func JSONModelPath(body []byte) (string, string) {
	return jsonModelWithPaths(body, []string{"model", "session.model"})
}

// JSONModelPathForRoute 按入口返回合成路由改写应写入的模型路径：Live 入口
// session.model 优先，其余入口顶层 model 优先。
func JSONModelPathForRoute(routePath string, body []byte) (string, string) {
	if IsLiveRequestRoute(routePath) {
		return jsonModelWithPaths(body, []string{"session.model", "model"})
	}
	return jsonModelWithPaths(body, []string{"model", "session.model"})
}

func jsonModelWithPaths(body []byte, paths []string) (string, string) {
	for _, path := range paths {
		model := gjson.GetBytes(body, path)
		if model.Type != gjson.String {
			continue
		}
		if value := strings.TrimSpace(model.String()); value != "" {
			return value, path
		}
	}
	return "", ""
}

// jsonModelCandidates 返回顶层所有键名大小写不敏感等于 "model" 的字符串值，
// 按出现顺序保留重复（覆盖 gjson 首 matches 与 encoding/json 末值绑定两类解析器）。
func jsonModelCandidates(body []byte) []string {
	var out []string
	gjson.ParseBytes(body).ForEach(func(key, value gjson.Result) bool {
		if value.Type == gjson.String && strings.EqualFold(key.String(), "model") {
			if trimmed := strings.TrimSpace(value.String()); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return true
	})
	return out
}

// jsonSessionModelCandidates 返回每个顶层 "session"（大小写不敏感）对象内
// 所有键名大小写不敏感等于 "model" 的字符串值。
func jsonSessionModelCandidates(body []byte) []string {
	var out []string
	gjson.ParseBytes(body).ForEach(func(key, value gjson.Result) bool {
		if !strings.EqualFold(key.String(), "session") {
			return true
		}
		value.ForEach(func(sessionKey, sessionValue gjson.Result) bool {
			if sessionValue.Type == gjson.String && strings.EqualFold(sessionKey.String(), "model") {
				if trimmed := strings.TrimSpace(sessionValue.String()); trimmed != "" {
					out = append(out, trimmed)
				}
			}
			return true
		})
		return true
	})
	return out
}

// sessionModelCandidates 对若干 session 字段原始值（JSON 文本）逐个提取模型候选。
func sessionModelCandidates(rawSessions []string) []string {
	var out []string
	for _, raw := range rawSessions {
		out = append(out, jsonModelCandidates([]byte(raw))...)
	}
	return out
}

// multipartModelCandidates 返回 multipart 表单中全部 "model" 字段值与全部
// "session" 字段原始值，按出现顺序保留重复，覆盖取首个或末个字段的两类 handler。
func multipartModelCandidates(contentType string, body []byte) (models, sessions []string) {
	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return nil, nil
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return nil, nil
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return models, sessions
		}
		if err != nil {
			return models, sessions
		}
		fieldName := strings.TrimSpace(part.FormName())
		if part.FileName() != "" || (fieldName != "model" && fieldName != "session") {
			continue
		}
		data, err := io.ReadAll(part)
		if err != nil {
			continue
		}
		if value := strings.TrimSpace(string(data)); value != "" {
			if fieldName == "model" {
				models = append(models, value)
			} else {
				sessions = append(sessions, value)
			}
		}
	}
}

// isMultipartContentType 判断请求是否声明为 multipart/form-data。
func isMultipartContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	return err == nil && strings.EqualFold(mediaType, "multipart/form-data")
}

// ResetRequestBody 把已读取（可能已被改写）的请求体回填到请求上，
// 供后续 handler 重新读取。使用 httputil.PrereadBody 回填，后续经
// ReadRequestBodyWithPrealloc 读取时零拷贝。
func ResetRequestBody(req *http.Request, body []byte) {
	if req == nil {
		return
	}
	req.Body = httputil.NewPrereadBody(body)
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))
}
