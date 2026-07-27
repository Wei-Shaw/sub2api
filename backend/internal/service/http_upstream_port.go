package service

import (
	"github.com/Wei-Shaw/sub2api/internal/port/upstream"
)

// HTTPUpstream 上游 HTTP 请求接口
// 用于向上游 API（Claude、OpenAI、Gemini 等）发送请求
//
// Alias: 契约定义已迁移至 internal/port/upstream，打破 repository→service
// 反向依赖。保留 service.HTTPUpstream 名称以兼容现有调用方与测试桩。
type HTTPUpstream = upstream.HTTPUpstream
