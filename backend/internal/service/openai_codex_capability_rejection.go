package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

// codexAccessProgramParam 是上游拒绝访问项目时错误体里的 param 值。
// 实测：HTTP 200 首帧 `event: error`，error.param="access_programs.cyber"，
// error.code 为 access_program_not_enabled 或 invalid_access_program。
const codexAccessProgramParam = "access_programs.cyber"

// isCodexAccessProgramRejection 识别上游对 Daybreak 访问项目的流内拒绝：事件 error
// （或 OAuth 路径改写后的 response.failed）且错误参数指向 access_programs.cyber。
// 不匹配 code：两种 code 都表示不允许，且上游可能新增。
func isCodexAccessProgramRejection(payload []byte) bool {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return false
	}
	switch strings.TrimSpace(gjson.GetBytes(payload, "type").String()) {
	case "error", "response.failed", "":
	default:
		return false
	}
	for _, path := range []string{"error.param", "response.error.param", "param"} {
		if strings.EqualFold(strings.TrimSpace(gjson.GetBytes(payload, path).String()), codexAccessProgramParam) {
			return true
		}
	}
	return false
}

// observeCodexCapabilityRejection 在所有检查 OAuth 上游错误事件的位置统一挂钩：命中访问项目
// 拒绝时对所选账号异步触发一次能力重探测（按账号冷却窗口）。它不改变事件透传、既有的
// bare error → response.failed 改写、failover 判定与账号副作用。返回是否发起了重探测。
func (s *OpenAIGatewayService) observeCodexCapabilityRejection(account *Account, payload []byte) bool {
	if s == nil || !isCodexCapabilityProbeAccount(account) || !isCodexAccessProgramRejection(payload) {
		return false
	}
	return s.getCodexCapabilityProbe().ReprobeAsync(account.ID)
}
