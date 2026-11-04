//go:build unit

package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptnetwork"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestUpdateSettingsChatGPTIPv6PersistsAndToggles(t *testing.T) {
	old := chatgptnetwork.Enabled()
	t.Cleanup(func() { chatgptnetwork.SetEnabled(old) })
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})
	for _, enabled := range []bool{true, false} {
		rec := doUpdateSettings(t, h, map[string]any{"chatgpt_ipv6_only": enabled}, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, enabled, repo.values["chatgpt_ipv6_only"] == "true")
		require.Equal(t, enabled, chatgptnetwork.Enabled())
		rec = doUpdateSettings(t, h, map[string]any{"site_name": "unchanged route"}, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, enabled, repo.values["chatgpt_ipv6_only"] == "true")
	}
}
