package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type codexDeviceSlotAdminStub struct {
	*stubAdminService
	slots   []service.CodexResolvedDeviceSlot
	deleted int64
}

func (s *codexDeviceSlotAdminStub) ListAccountCodexDeviceSlots(
	context.Context,
	int64,
	bool,
) ([]service.CodexResolvedDeviceSlot, error) {
	return s.slots, nil
}

func (s *codexDeviceSlotAdminStub) FinalizeAccountCodexDeviceSlots(context.Context, int64) (int64, error) {
	return s.deleted, nil
}

func TestAccountCodexDeviceSlotManagementDoesNotExposeBindingIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	proxyID := int64(9)
	adminSvc := &codexDeviceSlotAdminStub{
		stubAdminService: newStubAdminService(),
		deleted:          2,
		slots: []service.CodexResolvedDeviceSlot{{
			BindingID: 77, APIKeyID: 88, SlotID: 99,
			OSClass: service.CodexOSLinux, CanonicalSurface: service.CodexSurfaceDesktop,
			Architecture: service.CodexArchARM64, CatalogVersion: 1,
			Epoch: 2, SlotIndex: 0, State: "draining", ProxyID: &proxyID,
			ClientVersionMode: service.CodexClientVersionPinned, ClientVersion: "0.188.0",
			EffectiveClientVersion: "0.188.0", BindingCount: 1,
			ClientProfileVerification: service.CodexClientProfileUnverified, ClientProfileSource: "builtin",
		}},
	}
	handler := &AccountHandler{adminService: adminSvc}
	router := gin.New()
	router.GET("/accounts/:id/codex-device-slots", handler.ListCodexDeviceSlots)
	router.POST("/accounts/:id/codex-device-slots/finalize-draining", handler.FinalizeCodexDrainingSlots)

	listRecorder := httptest.NewRecorder()
	router.ServeHTTP(listRecorder, httptest.NewRequest(http.MethodGet, "/accounts/7/codex-device-slots?include_draining=true", nil))
	require.Equal(t, http.StatusOK, listRecorder.Code)
	require.NotContains(t, listRecorder.Body.String(), "binding_id")
	require.NotContains(t, listRecorder.Body.String(), "api_key_id")
	require.NotContains(t, listRecorder.Body.String(), "slot_id")
	require.Contains(t, listRecorder.Body.String(), `"state":"draining"`)
	require.Contains(t, listRecorder.Body.String(), `"client_version_mode":"pinned"`)
	require.Contains(t, listRecorder.Body.String(), `"effective_client_version":"0.188.0"`)
	require.Contains(t, listRecorder.Body.String(), `"client_profile_verification":"unverified"`)
	require.Contains(t, listRecorder.Body.String(), `"client_profile_source":"builtin"`)

	finalizeRecorder := httptest.NewRecorder()
	router.ServeHTTP(finalizeRecorder, httptest.NewRequest(http.MethodPost, "/accounts/7/codex-device-slots/finalize-draining", nil))
	require.Equal(t, http.StatusOK, finalizeRecorder.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(finalizeRecorder.Body.Bytes(), &payload))
	require.Contains(t, finalizeRecorder.Body.String(), `"deleted":2`)
}
