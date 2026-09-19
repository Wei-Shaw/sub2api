package admin

import (
	"testing"

	"github.com/gin-gonic/gin/binding"
	"github.com/stretchr/testify/require"
)

// monitorBindableProviders 是渠道监控 API 层应当放行的 provider 全集，与
// service.monitorProviders / ent enum / 迁移 CHECK 约束保持一致。
// binding tag 的 oneof 是手写字面量，容易在新增平台时漏改，
// 这里逐个过一遍 binding 校验兜住漂移。
var monitorBindableProviders = []string{
	"openai", "anthropic", "gemini", "grok",
	"antigravity", "kimi", "zhipu", "deepseek",
}

// template 只服务探活监控：仅 probe 四件套（quota-only provider 无探活 adapter）。
var monitorTemplateProviders = []string{"openai", "anthropic", "gemini", "grok"}

func TestChannelMonitorRequestValidationAcceptsAllProviders(t *testing.T) {
	for _, provider := range monitorBindableProviders {
		t.Run(provider, func(t *testing.T) {
			createReq := channelMonitorCreateRequest{
				Name:            "monitor",
				Provider:        provider,
				IntervalSeconds: 60,
			}
			require.NoError(t, binding.Validator.ValidateStruct(createReq))

			updateReq := channelMonitorUpdateRequest{Provider: &provider}
			require.NoError(t, binding.Validator.ValidateStruct(updateReq))
		})
	}

	for _, provider := range monitorTemplateProviders {
		templateReq := channelMonitorTemplateCreateRequest{
			Name:     "template",
			Provider: provider,
		}
		require.NoError(t, binding.Validator.ValidateStruct(templateReq))
	}
}

func TestChannelMonitorRequestValidationRejectsUnknownProvider(t *testing.T) {
	createReq := channelMonitorCreateRequest{
		Name:            "monitor",
		Provider:        "not-a-provider",
		IntervalSeconds: 60,
	}
	require.Error(t, binding.Validator.ValidateStruct(createReq))
}
