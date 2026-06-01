package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func requireRawJSONString(t testing.TB, raw json.RawMessage) string {
	t.Helper()

	var s string
	require.NoError(t, json.Unmarshal(raw, &s))
	return s
}
