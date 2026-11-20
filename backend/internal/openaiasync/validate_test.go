package openaiasync

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseInput(t *testing.T) {
	line := `{"custom_id":"one","method":"POST","url":"/v1/responses","body":{"model":"model-a","input":"hi","stream":false}}`
	items, err := ParseInput([]byte(line+"\n"), "/v1/responses", 3, 4096)
	require.NoError(t, err)
	require.Len(t, items, 1)
	for name, input := range map[string]string{
		"duplicate ids":          line + "\n" + line,
		"nul custom id":          strings.Replace(line, `"one"`, `"one\u0000"`, 1),
		"wrong url":              strings.Replace(line, "/v1/responses", "http://127.0.0.1/admin", 1),
		"stream":                 strings.Replace(line, `"stream":false`, `"stream":true`, 1),
		"null stream":            strings.Replace(line, `"stream":false`, `"stream":null`, 1),
		"missing model":          strings.Replace(line, `"model":"model-a",`, "", 1),
		"blank line":             line + "\n\n",
		"extra json":             line + ` {}`,
		"unknown envelope field": strings.Replace(line, `"method"`, `"other":1,"method"`, 1),
		"multiple models":        line + "\n" + strings.ReplaceAll(strings.Replace(line, `"one"`, `"two"`, 1), "model-a", "model-b"),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseInput([]byte(input), "/v1/responses", 3, 4096)
			require.Error(t, err)
		})
	}
	_, err = ParseInput([]byte(line+"\n"+strings.Replace(line, `"one"`, `"two"`, 1)), "/v1/responses", 1, 4096)
	require.Error(t, err)
}
