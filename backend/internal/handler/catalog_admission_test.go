package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFinalizeCatalogEffectiveModelReleasesSelectedSlotOnReject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	released := false
	mapping := &service.ChannelMappingResult{
		CatalogError: service.ErrCatalogModelDisabled,
	}

	ok := finalizeCatalogEffectiveModel(c, mapping, &service.Account{ID: 42}, "requested-model", func() {
		released = true
	})

	require.False(t, ok)
	require.True(t, released)
	require.Equal(t, 403, c.Writer.Status())
}

func TestFinalizeCatalogEffectiveModelLeavesSelectionOwnershipOnSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	released := false

	ok := finalizeCatalogEffectiveModel(c, &service.ChannelMappingResult{}, &service.Account{ID: 42}, "requested-model", func() {
		released = true
	})

	require.True(t, ok)
	require.False(t, released)
}
