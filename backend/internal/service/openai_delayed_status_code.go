package service

import "github.com/gin-gonic/gin"

// openAIDelayedStatusCodeEnabled 报告当前请求所属分组是否启用「状态码延迟返回」。
//
// 开启后，流式路径在首个语义输出前不发送 keepalive 心跳：任何写出都会提交 HTTP 200，
// 使 c.Writer.Written() 变真，进而堵死上游流内错误的 failover 前置门槛。
//
// 流式处理函数的签名不含 apiKey，因此直接从 gin.Context 读取——四条流式路径共用同一个
// context，无需在各 forward 入口分别绑定。未认证或分组缺失时返回 false，保证默认行为
// 与开关关闭时完全一致。
func openAIDelayedStatusCodeEnabled(c *gin.Context) bool {
	if c == nil {
		return false
	}
	apiKey := getAPIKeyFromContext(c)
	return apiKey != nil && apiKey.Group != nil && apiKey.Group.DelayedStatusCode
}
