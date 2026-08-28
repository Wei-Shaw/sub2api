//go:build unit

package routes

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGatewayAPIKeyAuthIsImmediatelyFollowedByConnectionSignal(t *testing.T) {
	source, err := os.ReadFile("gateway.go")
	require.NoError(t, err)

	apiKeyAuth := regexp.MustCompile(`gin\.HandlerFunc\(apiKeyAuth\)`)
	matches := apiKeyAuth.FindAllStringIndex(string(source), -1)
	require.NotEmpty(t, matches)
	for _, match := range matches {
		tail := string(source)[match[1]:]
		lineEnd := strings.IndexByte(tail, '\n')
		if lineEnd >= 0 {
			tail = tail[:lineEnd]
		}
		require.Truef(t, strings.HasPrefix(tail, ", postAuthSignal"),
			"every gateway apiKeyAuth surface must emit its signal immediately after auth: %s", strings.TrimSpace(tail))
	}
}
