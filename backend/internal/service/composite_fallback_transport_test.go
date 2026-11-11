package service

import (
	"context"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCompositeFallbackPreOutputReadError(t *testing.T) {
	for _, mode := range []string{"unwritten", "headers only", "heartbeat only", "semantic output"} {
		t.Run(mode, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			decision := CompositeRouteDecision{Matched: true, FallbackTargets: []CompositeFallbackTarget{{TargetPlatform: PlatformDeepseek, UpstreamModel: "deepseek-flash"}}}
			ctx := WithCompositeRouteDecision(context.Background(), decision)
			c.Request = httptest.NewRequest("POST", "/v1/messages", nil).WithContext(ctx)
			switch mode {
			case "headers only":
				c.Writer.WriteHeaderNow()
			case "heartbeat only":
				n, err := c.Writer.WriteString(": ping\n\n")
				require.NoError(t, err)
				c.Set(openAIStreamKeepaliveBytesKey, n)
			case "semantic output":
				_, err := c.Writer.WriteString("data: semantic\n\n")
				require.NoError(t, err)
			}
			if mode == "semantic output" {
				require.Positive(t, OpenAIStreamKeepaliveAdjustedWrittenSize(c))
				require.Nil(t, compositeFallbackPreOutputReadError(ctx, c, io.ErrUnexpectedEOF))
			} else {
				require.Equal(t, -1, OpenAIStreamKeepaliveAdjustedWrittenSize(c))
				require.NotNil(t, compositeFallbackPreOutputReadError(ctx, c, io.ErrUnexpectedEOF))
			}
			require.Nil(t, compositeFallbackPreOutputReadError(ctx, c, context.Canceled))
			require.Nil(t, compositeFallbackPreOutputReadError(ctx, c, ErrUpstreamResponseBodyTooLarge))
			require.Nil(t, compositeFallbackPreOutputReadError(context.Background(), c, io.ErrUnexpectedEOF))
		})
	}
}
