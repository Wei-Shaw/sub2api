//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIVideoDuplicateModelRejectedBeforeUpstream(t *testing.T) {
	for _, protocol := range []string{"", "grok", "zhongy"} {
		for _, models := range [][2]string{{"cheap-video", "expensive-video"}, {"expensive-video", "cheap-video"}, {"cheap-video", "cheap-video"}} {
			multipartBody, multipartType := videoMultipartTestBody(t, []videoMultipartTestPart{
				videoMultipartTestField("model", models[0]),
				videoMultipartTestField("seconds", "6"),
				videoMultipartTestField("size", "1280x720"),
				videoMultipartTestField("model", models[1]),
			})
			for _, tc := range []struct {
				name, contentType string
				body              []byte
			}{
				{"json", "application/json", []byte(`{"model":"` + models[0] + `","model":"` + models[1] + `","seconds":"6","size":"1280x720"}`)},
				{"escaped-json-key", "application/json", []byte(`{"model":"` + models[0] + `","mo\u0064el":"` + models[1] + `","seconds":"6","size":"1280x720"}`)},
				{"case-variant-json-key", "application/json", []byte(`{"model":"` + models[0] + `","Model":"` + models[1] + `","seconds":"6","size":"1280x720"}`)},
				{"multipart", multipartType, multipartBody},
			} {
				t.Run(protocol+"/"+models[0]+"-"+models[1]+"/"+tc.name, func(t *testing.T) {
					upstream := &grokMediaContentUpstreamStub{response: grokMediaContentStatusResponse(`{"id":"unexpected","status":"queued"}`)}
					svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
					account := openAIVideoTestAccount()
					account.Extra = map[string]any{"openai_video_protocol": protocol}
					c, recorder := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
					preflights, saves := 0, 0
					_, err := svc.ForwardOpenAIVideoCreate(context.Background(), c, account, tc.body, tc.contentType,
						func(*OpenAIForwardResult) error { saves++; return nil },
						func(*OpenAIForwardResult) error { preflights++; return nil })
					require.ErrorContains(t, err, "duplicate video model")
					require.Equal(t, http.StatusBadRequest, recorder.Code)
					require.Empty(t, upstream.requests)
					require.Zero(t, preflights)
					require.Zero(t, saves)
				})
			}
		}
	}
}
