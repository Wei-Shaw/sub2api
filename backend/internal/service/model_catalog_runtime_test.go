package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestModelCatalogProjectionRuntimeNormalizesIndependentReadModes(t *testing.T) {
	runtime := NewModelCatalogProjectionRuntimeWithModes(nil, nil, "SHADOW", time.Minute, ModelCatalogReadModes{
		ImportMode:      "PUBLISH",
		ListReadMode:    "DB",
		PricingReadMode: "unexpected",
		AdmissionMode:   "OBSERVE",
	})

	require.Equal(t, "shadow", runtime.Mode())
	require.Equal(t, ModelCatalogReadModes{
		ImportMode:      "publish",
		ListReadMode:    "db",
		PricingReadMode: "legacy",
		AdmissionMode:   "observe",
	}, runtime.ReadModes())
}

func TestModelCatalogProjectionRuntimeLegacyDefaultsKeepImporterOff(t *testing.T) {
	runtime := NewModelCatalogProjectionRuntime(nil, nil, "legacy", time.Minute)
	require.Equal(t, ModelCatalogReadModes{
		ImportMode:      "off",
		ListReadMode:    "legacy",
		PricingReadMode: "legacy",
		AdmissionMode:   "off",
	}, runtime.ReadModes())
}
