package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestParsedRequestScalarsDoNotAliasRequestBody(t *testing.T) {
	for _, replace := range []bool{false, true} {
		for _, suffix := range []string{"", "[1m]"} {
			t.Run(fmt.Sprintf("replace=%t/suffix=%s", replace, suffix), func(t *testing.T) {
				body := []byte(fmt.Sprintf(`{"model":"claude-sonnet-4%s","metadata":{"user_id":"session-123"},"output_config":{"effort":" high "},"speed":"fast","system":"instructions","messages":[{"role":"user","content":"%s"}]}`, suffix, strings.Repeat("x", 64<<10)))
				var parsed *ParsedRequest
				var err error
				if replace {
					parsed, err = ParseGatewayRequest(NewRequestBodyRef([]byte(`{}`)), domain.PlatformAnthropic)
					require.NoError(t, err)
					err = parsed.ReplaceBody(body)
				} else {
					parsed, err = ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformAnthropic)
				}
				require.NoError(t, err)

				// Large payload sections must still refer directly to the current
				// body; only persistent scalar strings should get their own storage.
				currentBody := parsed.Body.Bytes()
				require.True(t, &parsed.SystemRaw()[0] == &currentBody[parsed.systemRange.start])
				require.True(t, &parsed.MessagesRaw()[0] == &currentBody[parsed.messagesRange.start])
				model, userID, effort, speed := parsed.Model, parsed.MetadataUserID, parsed.OutputEffort, parsed.Speed
				parsed.Body.Replace(nil)

				// Reusing the source storage must not change metadata retained by
				// queued billing work. Also overwrite the pre-normalization body:
				// stripping [1m] must not leave Model pointing into that old buffer.
				for i := range body {
					body[i] = '!'
				}
				for i := range currentBody {
					currentBody[i] = '!'
				}
				require.Equal(t, "claude-sonnet-4", model)
				require.Equal(t, "session-123", userID)
				require.Equal(t, "high", effort)
				require.Equal(t, "fast", speed)
			})
		}
	}
}
