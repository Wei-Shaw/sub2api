package service

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexTurnMetadataRewritePreservesHeaderSafeJSON(t *testing.T) {
	account := newTestOAuthAccount(1, map[string]any{codexFingerprintModeExtraKey: "session"})
	account.Credentials = map[string]any{"chatgpt_account_id": "test-account"}
	ids := resolveCodexFingerprintIDsFromRequest(account, http.Header{})
	require.NotNil(t, ids)

	rewriters := map[string]func(*testing.T, string) string{
		"account_header": func(t *testing.T, raw string) string {
			h := make(http.Header)
			h.Set(openAIWSTurnMetadataHeader, raw)
			applyCodexAccountIdentityHeaders(h, account, 77)
			return h.Get(openAIWSTurnMetadataHeader)
		},
		"account_body_map": func(t *testing.T, raw string) string {
			cm := map[string]any{openAIWSTurnMetadataHeader: raw}
			require.True(t, applyCodexAccountIdentityClientMetadataMap(map[string]any{"client_metadata": cm}, account, 77))
			value, ok := cm[openAIWSTurnMetadataHeader].(string)
			require.True(t, ok)
			return value
		},
		"account_body_raw": func(t *testing.T, raw string) string {
			body, err := json.Marshal(map[string]any{"client_metadata": map[string]any{openAIWSTurnMetadataHeader: raw}})
			require.NoError(t, err)
			updated, changed, err := applyCodexAccountIdentityClientMetadataRaw(body, account, 77)
			require.NoError(t, err)
			require.True(t, changed)
			return gjson.GetBytes(updated, "client_metadata."+openAIWSTurnMetadataHeader).String()
		},
		"fingerprint_header": func(t *testing.T, raw string) string {
			h := make(http.Header)
			h.Set(openAIWSTurnMetadataHeader, raw)
			applyCodexFingerprintHeaders(h, ids)
			return h.Get(openAIWSTurnMetadataHeader)
		},
		"fingerprint_body_map": func(t *testing.T, raw string) string {
			cm := map[string]any{openAIWSTurnMetadataHeader: raw}
			require.True(t, applyCodexFingerprintClientMetadata(map[string]any{"client_metadata": cm}, ids))
			value, ok := cm[openAIWSTurnMetadataHeader].(string)
			require.True(t, ok)
			return value
		},
		"fingerprint_body_raw": func(t *testing.T, raw string) string {
			body, err := json.Marshal(map[string]any{"client_metadata": map[string]any{openAIWSTurnMetadataHeader: raw}})
			require.NoError(t, err)
			updated, changed, err := applyCodexFingerprintClientMetadataRaw(body, ids)
			require.NoError(t, err)
			require.True(t, changed)
			return gjson.GetBytes(updated, "client_metadata."+openAIWSTurnMetadataHeader).String()
		},
	}

	for _, raw := range []string{
		`{"installation_id":"client-install","workspaces":{"C:\\work\\中文🚀":{}}}`,
		`{"installation_id":"client-install","workspaces":{"C:\\work\\中文🚀":{"label":"café"}},"literal":"\\u4e2d","quote":"\"\\","controls":"\b\f\n\r\t\u0000\u007f","timestamp":1789302780858,"enabled":true}`,
		`{"installation_id":"client-install","workspaces":{"C:\\work\\\u4e2d\u6587\ud83d\ude80":{"label":"caf\u00e9"}},"literal":"\\u4e2d","quote":"\"\\","controls":"\b\f\n\r\t\u0000\u007f","timestamp":1789302780858,"enabled":true}`,
		`{"installation_id":"client-install","workspaces":{"C:\\work\\ascii":{}},"literal":"\\u4e2d"}`,
	} {
		var original map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &original))
		for name, rewrite := range rewriters {
			t.Run(name, func(t *testing.T) {
				updated := rewrite(t, raw)
				var decoded map[string]any
				require.NoError(t, json.Unmarshal([]byte(updated), &decoded))
				for key, value := range original {
					if key != "installation_id" {
						require.Equal(t, value, decoded[key], key)
					}
				}
				require.NotEqual(t, original["installation_id"], decoded["installation_id"])
				for _, b := range []byte(updated) {
					require.True(t, b >= 0x20 && b < 0x7f, "non-printable ASCII header byte: 0x%02x", b)
				}
			})
		}
	}
}
