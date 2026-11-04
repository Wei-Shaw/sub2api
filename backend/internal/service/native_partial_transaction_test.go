package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Identical input for baseline, modified and rollback verification.
func TestNativePartialTransaction(t *testing.T) {
	partial := 9999
	req := &OpenAIImagesRequest{Endpoint: openAIImagesEditsEndpoint, Model: "gpt-image-2.5-sunburst", Prompt: "make blue", N: 1, Stream: true, PartialImages: &partial, Quality: "xhigh", Size: "1152x2048", OutputFormat: "png", InputImageURLs: []string{"data:image/png;base64," + encodeOpenAIImageTestPNG(t, 32, 32)}}
	account := nativeImageTestAccount()
	compatible := accountSupportsOpenAIImageRequestOptions(WithOpenAIImagesRequestOptions(context.Background(), req), account)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, openAIImagesEditsEndpoint, nil)
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"data":[{"b64_json":%q}],"quality":"low","size":"32x32","output_format":"png"}`, encodeOpenAIImageTestPNG(t, 32, 32))))}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	_, err := svc.ForwardImages(context.Background(), c, account, nil, req, "")
	fmt.Printf("partial_images=9999 stream=true compatible=%v HTTP=%d upstream_called=%v\n", compatible, rec.Code, upstream.lastReq != nil)
	if !compatible {
		require.Error(t, err)
		require.Nil(t, upstream.lastReq)
		fmt.Printf("response=%s\n", rec.Body.String())
		return
	}
	require.NoError(t, err)
	require.Equal(t, "/backend-api/codex/images/edits", upstream.lastReq.URL.Path)
	require.Equal(t, "gpt-image-2.5-sunburst", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "xhigh", gjson.GetBytes(upstream.lastBody, "quality").String())
	require.Equal(t, "1152x2048", gjson.GetBytes(upstream.lastBody, "size").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "partial_images").Exists())
	events := parseOpenAIImageTestSSEEvents(rec.Body.String())
	require.Len(t, events, 1)
	require.Equal(t, "image_edit.completed", events[0].Name)
	require.Equal(t, "low", gjson.Get(events[0].Data, "quality").String())
	require.Equal(t, "final-only", rec.Header().Get("X-Sub2api-Image-Preview"))
	fmt.Printf("event=%s preview=%s actual_quality=low actual_size=32x32\n", events[0].Name, rec.Header().Get("X-Sub2api-Image-Preview"))
}
