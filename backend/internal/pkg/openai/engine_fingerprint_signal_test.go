package openai

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// hdr 构造一个 http.Header(键值对)。
func hdr(kv ...string) http.Header {
	h := http.Header{}
	for i := 0; i+1 < len(kv); i += 2 {
		h.Set(kv[i], kv[i+1])
	}
	return h
}

func TestEvaluateEngineFingerprint_DefaultSeed(t *testing.T) {
	sigs := DefaultEngineFingerprintSignals // 仅 x-codex- 前缀 Required
	cases := []struct {
		name string
		h    http.Header
		body string
		want bool
	}{
		{"R1 真CLI 带x-codex-window-id", hdr("x-codex-window-id", "a1", "session-id", "u1"), ``, true},
		{"R2 纯伪装 无指纹", hdr("user-agent", "codex/1"), ``, false},
		{"R3 仅body有", hdr(), `{"client_metadata":{"x-codex-window-id":"c3"}}`, false},
		{"R4 旧版 仅session_id无x-codex-", hdr("session_id", "u4"), ``, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, EvaluateEngineFingerprint(tc.h, []byte(tc.body), sigs))
		})
	}
}

func TestEvaluateEngineFingerprint_Rules(t *testing.T) {
	exactSession := EngineFingerprintSignal{Type: FingerprintSignalHeaderExact, Match: []string{"session-id", "session_id"}, Required: true}
	prefixCodex := EngineFingerprintSignal{Type: FingerprintSignalHeaderPrefix, Match: []string{"x-codex-"}, Required: true}
	bodyWin := EngineFingerprintSignal{Type: FingerprintSignalBodyPath, Match: []string{"client_metadata.x-codex-window-id"}, Required: true}

	t.Run("行内变体OR: 配置session-id 命中下划线session_id", func(t *testing.T) {
		require.True(t, EvaluateEngineFingerprint(hdr("session_id", "x"), nil, []EngineFingerprintSignal{exactSession}))
	})
	t.Run("跨条AND: 勾x-codex-与session 缺一即拒", func(t *testing.T) {
		both := []EngineFingerprintSignal{prefixCodex, exactSession}
		require.True(t, EvaluateEngineFingerprint(hdr("x-codex-window-id", "a", "session-id", "b"), nil, both))
		require.False(t, EvaluateEngineFingerprint(hdr("session-id", "b"), nil, both)) // 缺 x-codex-
	})
	t.Run("body_path 命中/ body空", func(t *testing.T) {
		require.True(t, EvaluateEngineFingerprint(hdr(), []byte(`{"client_metadata":{"x-codex-window-id":"1"}}`), []EngineFingerprintSignal{bodyWin}))
		require.False(t, EvaluateEngineFingerprint(hdr(), nil, []EngineFingerprintSignal{bodyWin}))
	})
	t.Run("无任何Required → true", func(t *testing.T) {
		none := []EngineFingerprintSignal{{Type: FingerprintSignalHeaderPrefix, Match: []string{"x-codex-"}, Required: false}}
		require.True(t, EvaluateEngineFingerprint(hdr(), nil, none))
		require.True(t, EvaluateEngineFingerprint(hdr(), nil, nil))
	})
}

func TestParseAndValidateEngineFingerprintSignals(t *testing.T) {
	t.Run("空串=合法空", func(t *testing.T) {
		sigs, ok := ParseEngineFingerprintSignals("")
		require.True(t, ok)
		require.Nil(t, sigs)
		require.NoError(t, ValidateEngineFingerprintSignalsJSON(""))
	})
	t.Run("合法数组", func(t *testing.T) {
		raw := `[{"type":"header_prefix","match":["x-codex-"],"required":true}]`
		sigs, ok := ParseEngineFingerprintSignals(raw)
		require.True(t, ok)
		require.Len(t, sigs, 1)
		require.NoError(t, ValidateEngineFingerprintSignalsJSON(raw))
	})
	t.Run("非法JSON", func(t *testing.T) {
		_, ok := ParseEngineFingerprintSignals("not json")
		require.False(t, ok)
		require.Error(t, ValidateEngineFingerprintSignalsJSON("not json"))
	})
	t.Run("非法type 被校验拒绝", func(t *testing.T) {
		require.Error(t, ValidateEngineFingerprintSignalsJSON(`[{"type":"bogus","match":["x"]}]`))
	})
	t.Run("match全空 被校验拒绝", func(t *testing.T) {
		require.Error(t, ValidateEngineFingerprintSignalsJSON(`[{"type":"header_exact","match":["",""]}]`))
	})
	t.Run("默认种子JSON 可解析且只勾x-codex-", func(t *testing.T) {
		sigs, ok := ParseEngineFingerprintSignals(DefaultEngineFingerprintSignalsJSON())
		require.True(t, ok)
		requiredTypes := []string{}
		for _, s := range sigs {
			if s.Required {
				requiredTypes = append(requiredTypes, s.Type+":"+s.Match[0])
			}
		}
		require.Equal(t, []string{"header_prefix:x-codex-"}, requiredTypes)
	})
}
