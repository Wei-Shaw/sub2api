package dto

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupMappersSeparateEffectiveAndAdminKiroCacheConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		group *service.Group
	}{
		{
			name: "disabled group retains stored ratios for admin editing",
			group: &service.Group{
				Platform:                        service.PlatformKiro,
				KiroCacheEmulationEnabled:       false,
				KiroCacheEmulationRatio:         1,
				KiroCacheEmulationMode:          "",
				KiroCacheCreationEmulationRatio: 1,
				KiroCacheReadEmulationRatio:     1,
			},
		},
		{
			name: "zero independent ratios retain the stored enabled flag for admin",
			group: &service.Group{
				Platform:                        service.PlatformKiro,
				KiroCacheEmulationEnabled:       true,
				KiroCacheEmulationRatio:         0.6,
				KiroCacheEmulationMode:          service.KiroCacheEmulationModeIndependent,
				KiroCacheCreationEmulationRatio: 0,
				KiroCacheReadEmulationRatio:     0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publicDTO := GroupFromServiceShallow(tt.group)
			adminDTO := GroupFromServiceAdmin(tt.group)

			require.False(t, publicDTO.KiroCacheEmulationEnabled)
			require.Equal(t, tt.group.EffectiveKiroCacheEmulationRatio(), publicDTO.KiroCacheEmulationRatio)
			require.Zero(t, publicDTO.KiroCacheCreationEmulationRatio)
			require.Zero(t, publicDTO.KiroCacheReadEmulationRatio)

			require.Equal(t, tt.group.KiroCacheEmulationEnabled, adminDTO.KiroCacheEmulationEnabled)
			require.Equal(t, tt.group.KiroCacheEmulationRatio, adminDTO.KiroCacheEmulationRatio)
			require.Equal(t, tt.group.KiroCacheEmulationMode, adminDTO.KiroCacheEmulationMode)
			require.Equal(t, tt.group.KiroCacheCreationEmulationRatio, adminDTO.KiroCacheCreationEmulationRatio)
			require.Equal(t, tt.group.KiroCacheReadEmulationRatio, adminDTO.KiroCacheReadEmulationRatio)
		})
	}
}

func TestGroupFromServiceShallowUsesUniformEffectiveRatios(t *testing.T) {
	group := &service.Group{
		Platform:                        service.PlatformKiro,
		KiroCacheEmulationEnabled:       true,
		KiroCacheEmulationRatio:         0.8,
		KiroCacheEmulationMode:          service.KiroCacheEmulationModeUniform,
		KiroCacheCreationEmulationRatio: 0.5,
		KiroCacheReadEmulationRatio:     0.5,
	}

	publicDTO := GroupFromServiceShallow(group)
	require.True(t, publicDTO.KiroCacheEmulationEnabled)
	require.InDelta(t, 0.8, publicDTO.KiroCacheEmulationRatio, 1e-12)
	require.InDelta(t, 0.8, publicDTO.KiroCacheCreationEmulationRatio, 1e-12)
	require.InDelta(t, 0.8, publicDTO.KiroCacheReadEmulationRatio, 1e-12)
}
