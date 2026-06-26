package openai

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// hdr 构造一个 http.Header(键值对)。
func hdr(kv ...string) http.Header {
	h := http.Header{REDACTED
	for i := 0; i+1 < len(kv); i += 2 {
		h.Set(kv[i], kv[i+1])
REDACTED
	return h
REDACTED

func TestEvaluateEngineFingerprint_DefaultSeed(t *testing.T) {
	sigs := DefaultEngineFingerprintSignals // 仅 x-codex- 前缀 Required
	cases := []struct {
		name string
		h    http.Header
		body string
		want bool
REDACTED{
		{"R1 真CLI 带x-codex-window-id", hdr("x-codex-window-id", "a1", "session-id", "u1"), ``, trueREDACTED,
		{"R2 纯伪装 无指纹", hdr("user-agent", "codex/1"), ``, falseREDACTED,
		{"R3 仅body有", hdr(), `{"client_metadata":{"x-codex-window-id":"c3"REDACTEDREDACTED`, falseREDACTED,
		{"R4 旧版 仅session_id无x-codex-", hdr("session_id", "u4"), ``, falseREDACTED,
REDACTED
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, EvaluateEngineFingerprint(tc.h, []byte(tc.body), sigs))
	REDACTED)
REDACTED
REDACTED

func TestEvaluateEngineFingerprint_Rules(t *testing.T) {
	exactSession := EngineFingerprintSignal{Type: FingerprintSignalHeaderExact, Match: []string{"session-id", "session_id"REDACTED, Required: trueREDACTED
	prefixCodex := EngineFingerprintSignal{Type: FingerprintSignalHeaderPrefix, Match: []string{"x-codex-"REDACTED, Required: trueREDACTED
	bodyWin := EngineFingerprintSignal{Type: FingerprintSignalBodyPath, Match: []string{"client_metadata.x-codex-window-id"REDACTED, Required: trueREDACTED

	t.Run("行内变体OR: 配置session-id 命中下划线session_id", func(t *testing.T) {
		require.True(t, EvaluateEngineFingerprint(hdr("session_id", "x"), nil, []EngineFingerprintSignal{exactSessionREDACTED))
REDACTED)
	t.Run("跨条AND: 勾x-codex-与session 缺一即拒", func(t *testing.T) {
		both := []EngineFingerprintSignal{prefixCodex, exactSessionREDACTED
		require.True(t, EvaluateEngineFingerprint(hdr("x-codex-window-id", "a", "session-id", "b"), nil, both))
		require.False(t, EvaluateEngineFingerprint(hdr("session-id", "b"), nil, both)) // 缺 x-codex-
REDACTED)
	t.Run("body_path 命中/ body空", func(t *testing.T) {
		require.True(t, EvaluateEngineFingerprint(hdr(), []byte(`{"client_metadata":{"x-codex-window-id":"1"REDACTEDREDACTED`), []EngineFingerprintSignal{bodyWinREDACTED))
		require.False(t, EvaluateEngineFingerprint(hdr(), nil, []EngineFingerprintSignal{bodyWinREDACTED))
REDACTED)
	t.Run("无任何Required → true", func(t *testing.T) {
		none := []EngineFingerprintSignal{{Type: FingerprintSignalHeaderPrefix, Match: []string{"x-codex-"REDACTED, Required: falseREDACTEDREDACTED
		require.True(t, EvaluateEngineFingerprint(hdr(), nil, none))
		require.True(t, EvaluateEngineFingerprint(hdr(), nil, nil))
REDACTED)
REDACTED

func TestParseAndValidateEngineFingerprintSignals(t *testing.T) {
	t.Run("空串=合法空", func(t *testing.T) {
		sigs, ok := ParseEngineFingerprintSignals("")
		require.True(t, ok)
		require.Nil(t, sigs)
		require.NoError(t, ValidateEngineFingerprintSignalsJSON(""))
REDACTED)
	t.Run("合法数组", func(t *testing.T) {
		raw := `[{"type":"header_prefix","match":["x-codex-"],"required":trueREDACTED]`
		sigs, ok := ParseEngineFingerprintSignals(raw)
		require.True(t, ok)
		require.Len(t, sigs, 1)
		require.NoError(t, ValidateEngineFingerprintSignalsJSON(raw))
REDACTED)
	t.Run("非法JSON", func(t *testing.T) {
		_, ok := ParseEngineFingerprintSignals("not json")
		require.False(t, ok)
		require.Error(t, ValidateEngineFingerprintSignalsJSON("not json"))
REDACTED)
	t.Run("非法type 被校验拒绝", func(t *testing.T) {
		require.Error(t, ValidateEngineFingerprintSignalsJSON(`[{"type":"bogus","match":["x"]REDACTED]`))
REDACTED)
	t.Run("match全空 被校验拒绝", func(t *testing.T) {
		require.Error(t, ValidateEngineFingerprintSignalsJSON(`[{"type":"header_exact","match":["",""]REDACTED]`))
REDACTED)
	t.Run("默认种子JSON 可解析且只勾x-codex-", func(t *testing.T) {
		sigs, ok := ParseEngineFingerprintSignals(DefaultEngineFingerprintSignalsJSON())
		require.True(t, ok)
		requiredTypes := []string{REDACTED
		for _, s := range sigs {
			if s.Required {
				requiredTypes = append(requiredTypes, s.Type+":"+s.Match[0])
		REDACTED
	REDACTED
		require.Equal(t, []string{"header_prefix:x-codex-"REDACTED, requiredTypes)
REDACTED)
REDACTED
