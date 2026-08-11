//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVideoAccountSupportsRequestRejectsAccountWhenVideoDisabled(t *testing.T) {
	service := &GatewayService{}
	for _, platform := range []string{PlatformFal, PlatformAtlasCloud, PlatformApiz} {
		t.Run(platform, func(t *testing.T) {
			account := &Account{
				Platform: platform,
				Extra:    map[string]any{},
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"bytedance/seedance-2.5/text-to-video": "upstream-model",
					},
				},
			}

			require.False(t, service.videoAccountSupportsRequest(
				context.Background(),
				account,
				"bytedance/seedance-2.5/text-to-video",
				"",
			))
		})
	}
}
