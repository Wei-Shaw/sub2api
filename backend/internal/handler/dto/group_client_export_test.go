package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// TestGroupClientExportDTO exposes only public preferences to both admin and
// key-list clients, without leaking admin-only source account configuration.
func TestGroupClientExportDTO(t *testing.T) {
	group := &service.Group{ClientExportConfig: service.GroupClientExportConfig{CodexModel: "group-model"}, CodexModelsManifestConfig: service.GroupCodexModelsManifestConfig{Enabled: true, AccountIDs: []int64{99}}}
	for _, dto := range []any{GroupFromService(group), GroupFromServiceAdmin(group)} {
		data, err := json.Marshal(dto)
		require.NoError(t, err)
		require.Contains(t, string(data), `"client_export_config":{"codex_model":"group-model"}`)
	}
	data, err := json.Marshal(GroupFromService(group))
	require.NoError(t, err)
	require.NotContains(t, string(data), "account_ids")
}
