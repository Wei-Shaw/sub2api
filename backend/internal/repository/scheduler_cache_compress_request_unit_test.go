//go:build unit

package repository

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/pkg/reqcompress"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// TestSchedulerCachePreservesCompressRequestEnabled pins the account snapshot's
// explicit field list: the full and metadata payloads must both carry the
// per-account request compression switch, and decoding either one must refresh
// the process-local negotiation registry.
func TestSchedulerCachePreservesCompressRequestEnabled(t *testing.T) {
	const accountID int64 = 9003
	enabled := true
	account := service.Account{
		ID:                     accountID,
		Name:                   "compress-request-snapshot",
		Platform:               service.PlatformOpenAI,
		Type:                   service.AccountTypeAPIKey,
		Status:                 service.StatusActive,
		Schedulable:            true,
		CompressRequestEnabled: &enabled,
		UpdatedAt:              time.Now().UTC(),
	}
	t.Cleanup(func() { reqcompress.SyncAccount(accountID, false, time.Time{}) })

	full, meta, err := marshalSchedulerCacheAccount(account)
	require.NoError(t, err)

	for name, payload := range map[string][]byte{"full": full, "metadata": meta} {
		decoded, decodeErr := decodeCachedAccount(payload)
		require.NoError(t, decodeErr, name)
		require.True(t, decoded.RequestCompressionEnabled(), name)
		require.True(t, reqcompress.Enabled(accountID), name)
	}

	// A lagging payload written before the admin switched the account off must
	// not re-enable it.
	stale := account
	stale.UpdatedAt = account.UpdatedAt.Add(-time.Minute)
	_, staleMeta, err := marshalSchedulerCacheAccount(stale)
	require.NoError(t, err)
	reqcompress.SyncAccount(accountID, false, account.UpdatedAt)
	_, err = decodeCachedAccount(staleMeta)
	require.NoError(t, err)
	require.False(t, reqcompress.Enabled(accountID))
}

// TestSchedulerCacheLegacyPayloadKeepsNegotiatedCompression pins the case that
// dropped the negotiated state in the field: payloads written before the switch
// existed carry no value, and decoding that absence as "off" cleared the
// registry on every cache read, so every request re-probed the upstream. Absence
// must stay unknown instead.
func TestSchedulerCacheLegacyPayloadKeepsNegotiatedCompression(t *testing.T) {
	const accountID int64 = 9004
	t.Cleanup(func() { reqcompress.SyncAccount(accountID, false, time.Time{}) })

	enabled := true
	current := service.Account{
		ID:                     accountID,
		Name:                   "compress-request-legacy-payload",
		Platform:               service.PlatformOpenAI,
		Type:                   service.AccountTypeAPIKey,
		Status:                 service.StatusActive,
		Schedulable:            true,
		CompressRequestEnabled: &enabled,
		UpdatedAt:              time.Now().UTC(),
	}
	currentPayload, _, err := marshalSchedulerCacheAccount(current)
	require.NoError(t, err)

	decoded, err := decodeCachedAccount(currentPayload)
	require.NoError(t, err)
	require.True(t, decoded.RequestCompressionEnabled())
	require.True(t, reqcompress.Enabled(accountID))

	// Same snapshot age, but written before the switch existed: the account must
	// stay enabled and keep its negotiated coding.
	legacyPayload := withoutJSONField(t, currentPayload, "CompressRequestEnabled")
	legacy, err := decodeCachedAccount(legacyPayload)
	require.NoError(t, err)
	require.Nil(t, legacy.CompressRequestEnabled, "absence must decode as unknown")
	require.True(t, reqcompress.Enabled(accountID), "a legacy payload must not disable compression")

	// An explicit value from the same payload still wins.
	disabled := false
	current.CompressRequestEnabled = &disabled
	current.UpdatedAt = current.UpdatedAt.Add(time.Second)
	disabledPayload, _, err := marshalSchedulerCacheAccount(current)
	require.NoError(t, err)
	_, err = decodeCachedAccount(disabledPayload)
	require.NoError(t, err)
	require.False(t, reqcompress.Enabled(accountID))
}

func withoutJSONField(t *testing.T, payload []byte, field string) []byte {
	t.Helper()
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))
	require.Contains(t, decoded, field)
	delete(decoded, field)
	out, err := json.Marshal(decoded)
	require.NoError(t, err)
	return out
}
