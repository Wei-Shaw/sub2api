//go:build unit

package service

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeepseekOCRBoundedBillingCopy(t *testing.T) {
	long := strings.Repeat(`page \"text\" \u0041 `, 1<<17)
	for _, tc := range []struct {
		name, suffix string
		want         int
	}{
		{"large completed page", `"}],"object":"ocr.result","page_count":2}`, 2},
		{"error after large page", `"}],"page_count":2,"error":{"message":"timeout"}}`, 0},
		{"invalid escape in discarded text", `\x"}],"page_count":2}`, 0},
		{"truncated string", `\`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var capture deepseekOCRBillingCapture
			_, _ = capture.Write(bytes.Repeat([]byte("\n"), 1<<20))
			_, _ = capture.Write([]byte(`{"pages":[{"text":"`))
			for chunk := range slices.Chunk([]byte(long), 127) {
				_, _ = capture.Write(chunk)
			}
			_, _ = capture.Write([]byte(tc.suffix))
			require.Less(t, len(capture.body), 256)
			require.Equal(t, tc.want, extractDeepseekToolsUsage("ocr", nil, capture.body, 200).Units)
		})
	}
}
