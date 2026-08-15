package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupEndpointDefaultRoutingDTOBoundary(t *testing.T) {
	group := &service.Group{ID: 1, EndpointDefaultRoutingEnabled: true}

	adminJSON, err := json.Marshal(GroupFromServiceAdmin(group))
	require.NoError(t, err)
	var adminPayload map[string]any
	require.NoError(t, json.Unmarshal(adminJSON, &adminPayload))
	require.Equal(t, true, adminPayload["endpoint_default_routing_enabled"])

	publicJSON, err := json.Marshal(GroupFromService(group))
	require.NoError(t, err)
	var publicPayload map[string]any
	require.NoError(t, json.Unmarshal(publicJSON, &publicPayload))
	_, exposed := publicPayload["endpoint_default_routing_enabled"]
	require.False(t, exposed)
}
