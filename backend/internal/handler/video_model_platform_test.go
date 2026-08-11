//go:build unit

package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestVideoModelSlugsForAccountUsesPlatformMappingDirection(t *testing.T) {
	const publicModel = "bytedance/seedance-2.5/text-to-video"
	for _, testCase := range []struct {
		platform string
		mapping  map[string]any
	}{
		{platform: domain.PlatformFal, mapping: map[string]any{"alias": publicModel}},
		{platform: domain.PlatformAtlasCloud, mapping: map[string]any{publicModel: "atlas-internal-model"}},
		{platform: domain.PlatformApiz, mapping: map[string]any{publicModel: "apiz-internal-model"}},
	} {
		t.Run(testCase.platform, func(t *testing.T) {
			account := &service.Account{
				Platform:    testCase.platform,
				Credentials: map[string]any{"model_mapping": testCase.mapping},
			}
			require.Equal(t, []string{publicModel}, videoModelSlugsForAccount(account))
		})
	}
}
