package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"

	"github.com/gin-gonic/gin"
)

// RejectConflictingRequestFields 拒绝「同名字段重复声明且取值不一致」的网关请求。
//
// 挂载位置：每条网关链的 GroupModelAllowlist 之后、compositeTarget 之前，
// 白名单对不在名单内的模型仍按原有格式拒绝。与白名单准入不同，这里对所有分组
// 生效：重复 model 键让计费模型（gjson 取第一个）和实际转发的模型
// （encoding/json 重新序列化后取最后一个）分叉，和分组是否开启白名单无关；
// service_tier 等同样按首个值做策略判断的字段也是如此。
//
// 行为：
//   - 只检查带请求体的方法（POST/PUT/PATCH）。读体后用 PrereadBody 回填，
//     下游中间件和 handler 零拷贝重读。
//   - 判定规则见 requestmodel.ConflictingField：取值相同的重复字段放行，
//     不是 JSON 对象也不是 multipart 的请求体放行，交给 handler 自己校验。
//   - 拒绝：按入口协议格式返回 400，请求不会进入合成路由、调度和计费。
//
// Responses WebSocket 的帧不经过这里，由 ResponsesWebSocket 对每一帧执行同一规则。
func RejectConflictingRequestFields() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request == nil || c.Request.Body == nil {
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
		requestmodel.ResetRequestBody(c.Request, body)

		field, conflict := requestmodel.ConflictingField(c.GetHeader("Content-Type"), body)
		if !conflict {
			c.Next()
			return
		}
		// 字段名来自客户端请求体，回显时加引号并限制在 64 个字符以内。
		message := fmt.Sprintf("Request body declares field %.64q more than once with different values", field)
		writeRequestFieldConflictError(c, message)
		c.Abort()
	}
}

// writeRequestFieldConflictError 按入口协议输出 400 invalid_request_error。
// 协议划分与 groupModelAllowlistErrorWriter 相同；这里不复用它，是因为现有的
// Anthropic / OpenAI 写入器把非 404/403 的状态写成 api_error / model_not_found。
func writeRequestFieldConflictError(c *gin.Context, message string) {
	path := ""
	if c.Request.URL != nil {
		path = c.Request.URL.Path
	}
	switch {
	case strings.HasPrefix(path, "/v1beta") || strings.HasPrefix(path, "/antigravity/v1beta"):
		GoogleErrorWriter(c, http.StatusBadRequest, message)
	case strings.Contains(path, "/messages"):
		c.JSON(http.StatusBadRequest, gin.H{
			"type":  "error",
			"error": gin.H{"type": "invalid_request_error", "message": message},
		})
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{"type": "invalid_request_error", "message": message},
		})
	}
}
