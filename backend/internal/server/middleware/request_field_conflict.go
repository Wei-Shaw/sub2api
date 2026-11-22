package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"

	"github.com/gin-gonic/gin"
)

// RejectConflictingRequestFields 拒绝「同名字段重复声明」的网关请求。
//
// 挂载位置：每条网关链的 GroupModelAllowlist 之后、compositeTarget 之前，
// 白名单对不在名单内的模型仍按原有格式拒绝。与白名单准入不同，这里对所有分组
// 生效：重复 model 键让计费模型（gjson 取第一个）和实际转发的模型
// （encoding/json 重新序列化后取最后一个）分叉，和分组是否开启白名单无关；
// service_tier、reasoning.effort 等按首个值做计费/策略判断、由 sjson 只改首个的
// 字段也是如此（GHSA-p3r4-jcm3-2hph）。
//
// 行为：
//   - 只检查带请求体的方法（POST/PUT/PATCH）。
//   - JSON 按内容识别（不看 Content-Type），开头的 UTF-8 BOM 在这里剥掉后回填
//     （PrereadBody，下游零拷贝重读），与 handler 宽容读取的规则一致，保证
//     合成路由、handler 与这里检查的是同一份字节。
//   - 判定规则见 requestmodel.ConflictingField；不是 JSON 也不是 multipart 的
//     请求体放行，交给 handler 自己校验。
//   - 拒绝：按入口协议格式返回 400，标记 ingress 拒绝原因
//     conflicting_request_fields，请求不会进入合成路由、调度和计费。
//
// Responses WebSocket 的帧不经过这里，由 ResponsesWebSocket 对每一帧执行同一规则。
func RejectConflictingRequestFields() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request == nil || c.Request.Body == nil || c.Request.Body == http.NoBody {
			c.Next()
			return
		}
		switch c.Request.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
		default:
			c.Next()
			return
		}

		body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil {
			if RejectRequestBodyReadError(c, "middleware.request_field_conflict", err) {
				return
			}
			AbortWithRequestBodyReadFailure(c, "middleware.request_field_conflict", err)
			return
		}
		if doc := requestmodel.TrimUTF8BOM(body); len(doc) != len(body) && requestmodel.LooksLikeJSON(doc) {
			body = doc
		}
		requestmodel.ResetRequestBody(c.Request, body)

		field, conflict := requestmodel.ConflictingField(c.GetHeader("Content-Type"), body)
		if !conflict {
			c.Next()
			return
		}
		MarkIngressRejected(c, IngressRejectConflictingRequestFields)
		// 字段名来自客户端请求体，回显时加引号并限制在 64 个字符以内。
		writeGatewayInvalidRequest(c, http.StatusBadRequest, fmt.Sprintf("Request body declares field %.64q more than once", field))
		c.Abort()
	}
}

// writeGatewayInvalidRequest 按入口协议输出 invalid_request_error：
// Gemini 原生（/v1beta、/antigravity/v1beta）用 Google 格式，Messages 入口用
// Anthropic 格式，其余用 OpenAI 格式。协议划分与 groupModelAllowlistErrorWriter
// 相同；不复用它，是因为现有的 Anthropic / OpenAI 写入器把非 404/403 的状态写成
// api_error / model_not_found。
func writeGatewayInvalidRequest(c *gin.Context, status int, message string) {
	path := ""
	if c.Request != nil && c.Request.URL != nil {
		path = c.Request.URL.Path
	}
	switch {
	case strings.HasPrefix(path, "/v1beta") || strings.HasPrefix(path, "/antigravity/v1beta"):
		GoogleErrorWriter(c, status, message)
	case strings.Contains(path, "/messages"):
		c.JSON(status, gin.H{
			"type":  "error",
			"error": gin.H{"type": "invalid_request_error", "message": message},
		})
	default:
		c.JSON(status, gin.H{
			"error": gin.H{"type": "invalid_request_error", "message": message},
		})
	}
}
