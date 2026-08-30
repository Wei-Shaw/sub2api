package service

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseHerohaoOfficialPriceSnapshotUsesDecimalOfficialFields(t *testing.T) {
	snapshot, err := parseHerohaoOfficialPriceSnapshot([]byte(`{
		"currency":"CNY","fetchedAt":"2026-08-30T09:41:56Z","warning":null,
		"token":{"databaseUpdatedAt":"2026-08-30T09:30:48Z","models":[{
			"model":"glm-5.1","providerKey":"glm","updatedAt":"2026-08-30T09:30:48Z","enabled":true,
			"prices":{"input":{"official":"9.8000"},"output":{"official":30.9},"cacheWrite":{"official":null},"cacheRead":{"official":"1.9000"}}
		}]}}
	`))
	require.NoError(t, err)
	require.Len(t, snapshot.Models, 1)
	candidate := snapshot.Models["glm-5.1"]
	require.Equal(t, "9.8", candidate.Input.String())
	require.Equal(t, "30.9", candidate.Output.String())
	require.Nil(t, candidate.CacheWrite)
	require.Equal(t, "1.9", candidate.CacheRead.String())
	require.NotNil(t, snapshot.UpdatedAt)
}

func TestParseHerohaoOfficialPriceSnapshotRejectsDuplicateAndNegativePrices(t *testing.T) {
	base := `{"currency":"CNY","fetchedAt":"2026-08-30T09:41:56Z","token":{"models":%s}}`
	duplicate := `[{"model":"m","providerKey":"glm","enabled":true,"prices":{"input":{"official":"1"}}},{"model":"m","providerKey":"glm","enabled":true,"prices":{"input":{"official":"1"}}}]`
	_, err := parseHerohaoOfficialPriceSnapshot([]byte(fmt.Sprintf(base, duplicate)))
	require.ErrorContains(t, err, "duplicate model")

	negative := `[{"model":"m","providerKey":"glm","enabled":true,"prices":{"input":{"official":"-1"}}}]`
	_, err = parseHerohaoOfficialPriceSnapshot([]byte(fmt.Sprintf(base, negative)))
	require.ErrorContains(t, err, "non-negative")
}
