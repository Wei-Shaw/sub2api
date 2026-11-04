//go:build unit

package service

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChatGPTWebBrowserTimezoneDaylightSaving(t *testing.T) {
	for _, tc := range []struct {
		instant string
		date    string
		offset  int
	}{
		{"2026-01-15T12:00:00Z", "Thu Jan 15 2026 04:00:00 GMT-0800 (Pacific Standard Time)", 480},
		{"2026-10-15T12:00:00Z", "Thu Oct 15 2026 05:00:00 GMT-0700 (Pacific Daylight Time)", 420},
	} {
		now, err := time.Parse(time.RFC3339, tc.instant)
		require.NoError(t, err)
		require.Equal(t, tc.date, chatGPTWebBrowserDate(now))
		require.Equal(t, tc.offset, chatGPTWebTimezoneOffset(now))
	}
}

func chatGPTWebFixedPowConfig() []any {
	return []any{
		3000,
		"Mon Sep 01 2026 00:00:00 GMT-0500 (Eastern Standard Time)",
		4294705152,
		1,
		"mozilla",
		"https://chatgpt.com/backend-api/sentinel/sdk.js",
		"c/abcdef/_",
		"en-US",
		"en-US,es-US,en,es",
		1,
		"webdriver\u2212false",
		"location",
		"window",
		1234.5,
		"00000000-0000-0000-0000-000000000000",
		"",
		8,
		1700000000000.5,
		0, 0, 0, 0, 0, 0,
		0,
	}
}

func TestChatGPTWebPowGenerateSatisfiesDifficulty(t *testing.T) {
	seed, difficulty := "0.559779845730002", "0fffff"
	answer, solved := chatGPTWebPowGenerate(seed, difficulty, chatGPTWebFixedPowConfig(), 500000)
	require.True(t, solved)
	require.True(t, strings.HasSuffix(answer, "~S"))
	encoded := strings.TrimSuffix(answer, "~S")
	require.LessOrEqual(t, chatGPTWebPowHash(seed + encoded)[:len(difficulty)], difficulty)
	raw, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	var decoded []any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Len(t, decoded, 25)
	require.GreaterOrEqual(t, decoded[9].(float64), float64(0))
}

func TestChatGPTWebPowGenerateRejectsBadDifficulty(t *testing.T) {
	_, solved := chatGPTWebPowGenerate("seed", "zz", chatGPTWebFixedPowConfig(), 10)
	require.False(t, solved)
}

// Fixed vectors from the installed client's JavaScript H3t implementation.
func TestChatGPTWebPowMatchesReferenceVector(t *testing.T) {
	for input, expected := range map[string]string{"": "ab3e7c0b", "hello": "888d766e", "seedYWJj": "e9ea50dd"} {
		require.Equal(t, expected, chatGPTWebPowHash(input), input)
	}
}

func TestChatGPTWebBuildRequirementsTokenShape(t *testing.T) {
	token := chatGPTWebBuildRequirementsToken("mozilla", nil, "c/abcdef/_")
	require.True(t, strings.HasPrefix(token, "gAAAAAC"))
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(token, "gAAAAAC"))
	require.NoError(t, err)
	var decoded []any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Len(t, decoded, 25)
	require.Equal(t, "c/abcdef/_", decoded[6])
}

func TestChatGPTWebBuildProofTokenPrefix(t *testing.T) {
	token, err := chatGPTWebBuildProofToken("0.559779845730002", "0fffff", "mozilla", nil, "")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(token, "gAAAAAB"))
}

func TestParseChatGPTWebPowResources(t *testing.T) {
	html := `<html data-build="prod-abc"><head>` +
		`<script src="https://chatgpt.com/c/abcdef/_next/static/chunks/main.js"></script>` +
		`</head></html>`
	sources, dataBuild := parseChatGPTWebPowResources(html)
	require.Len(t, sources, 1)
	require.Equal(t, "c/abcdef/_", dataBuild)

	sources, dataBuild = parseChatGPTWebPowResources(`<html data-build="prod-fallback"><head></head></html>`)
	require.Equal(t, []string{chatGPTWebDefaultPowScript}, sources)
	require.Equal(t, "prod-fallback", dataBuild)
}
