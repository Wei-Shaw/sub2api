//go:build unit

package service

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIVideoAmbiguousControlsRejectedBeforeUpstream(t *testing.T) {
	controls := []struct{ name, value, attack string }{
		{"model", "cheap-video", "expensive-video"},
		{"seconds", "6", "15"},
		{"duration", "6", "15"},
		{"size", "1280x720", "1920x1080"},
		{"resolution", "720p", "1080p"},
	}
	var baseParts []videoMultipartTestPart
	var baseJSON []string
	for _, control := range controls {
		baseParts = append(baseParts, videoMultipartTestField(control.name, control.value))
		baseJSON = append(baseJSON, fmt.Sprintf("%q:%q", control.name, control.value))
	}
	for _, protocol := range []string{"", "grok", "zhongy"} {
		for _, control := range controls {
			variant := strings.ToUpper(control.name[:1]) + control.name[1:]
			for _, before := range []bool{false, true} {
				for _, kind := range []string{"file", "empty-filename", "case-multipart", "case-json"} {
					t.Run(fmt.Sprintf("%s/%s/%s/before=%t", protocol, control.name, kind, before), func(t *testing.T) {
						contentType := "application/json"
						var body []byte
						if kind == "case-json" {
							fields := append([]string(nil), baseJSON...)
							attack := fmt.Sprintf("%q:%q", variant, control.attack)
							if before {
								fields = append([]string{attack}, fields...)
							} else {
								fields = append(fields, attack)
							}
							body = []byte("{" + strings.Join(fields, ",") + "}")
						} else {
							attack := videoMultipartTestField(control.name, control.attack)
							if kind == "case-multipart" {
								attack = videoMultipartTestField(variant, control.attack)
							} else {
								filename := "control.bin"
								if kind == "empty-filename" {
									filename = ""
								}
								attack.header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": control.name, "filename": filename}))
								attack.header.Set("Content-Type", "application/octet-stream")
							}
							parts := append([]videoMultipartTestPart(nil), baseParts...)
							if before {
								parts = append([]videoMultipartTestPart{attack}, parts...)
							} else {
								parts = append(parts, attack)
							}
							body, contentType = videoMultipartTestBody(t, parts)
						}
						upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"id":"unexpected","status":"queued"}`)}
						svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
						account := openAIVideoTestAccount()
						account.Extra = map[string]any{"openai_video_protocol": protocol}
						c, recorder := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
						preflights, saves := 0, 0
						_, err := svc.ForwardOpenAIVideoCreate(context.Background(), c, account, body, contentType,
							func(*OpenAIForwardResult) error { saves++; return nil },
							func(*OpenAIForwardResult) error { preflights++; return nil })
						if control.name == "model" && kind == "case-json" {
							require.ErrorContains(t, err, "duplicate video model")
						} else {
							require.ErrorContains(t, err, "video control field")
						}
						require.Equal(t, http.StatusBadRequest, recorder.Code)
						require.Empty(t, upstream.requests)
						require.Zero(t, preflights)
						require.Zero(t, saves)
					})
				}
			}
		}
	}
}
