//go:build unit

package admin

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAvailableChannelToAdminResponse_IncludesFullDTO(t *testing.T) {
	// 管理员视图应包含 id / status / billing_model_source / restrict_models 等
	// 管理字段；mapper 是纯透传，BillingModelSource 的默认回填由 service 层负责。
	input := service.AvailableChannel{
		ID:                 42,
		Name:               "ch",
		Description:        "d",
		Status:             service.StatusActive,
		BillingModelSource: service.BillingModelSourceChannelMapped,
		RestrictModels:     true,
		Groups: []service.AvailableGroupRef{
			{ID: 1, Name: "g1", Platform: "anthropic"},
		},
		SupportedModels: []service.SupportedModel{
			{Name: "claude-sonnet-4-6", Platform: "anthropic"},
		},
	}

	resp := availableChannelToAdminResponse(input)
	require.Equal(t, int64(42), resp.ID)
	require.Equal(t, "ch", resp.Name)
	require.Equal(t, service.StatusActive, resp.Status)
	require.Equal(t, service.BillingModelSourceChannelMapped, resp.BillingModelSource)
	require.True(t, resp.RestrictModels)
	require.Len(t, resp.Groups, 1)
	require.Len(t, resp.SupportedModels, 1)

	// JSON 层验证管理字段确实会被序列化。
	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	for _, key := range []string{"id", "status", "billing_model_source", "restrict_models", "groups", "supported_models"} {
		_, exists := decoded[key]
		require.Truef(t, exists, "admin DTO must expose %q", key)
	}
}

func TestAvailableChannelToAdminResponse_PreservesExplicitBillingSource(t *testing.T) {
	input := service.AvailableChannel{
		BillingModelSource: service.BillingModelSourceUpstream,
	}
	resp := availableChannelToAdminResponse(input)
	require.Equal(t, service.BillingModelSourceUpstream, resp.BillingModelSource)
}
