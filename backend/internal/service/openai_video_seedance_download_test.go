package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIVideoSeedanceSignedContentRedirectNoCredentials(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{
		{StatusCode: 302, Header: http.Header{"Location": []string{"https://cdn.example/result%2Fclip.mp4?sig=abc%2F123"}}, Body: io.NopCloser(strings.NewReader(""))},
		{StatusCode: 206, Header: http.Header{"Content-Type": []string{"video/mp4"}}, Body: io.NopCloser(strings.NewReader("mp4!"))},
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	req, err := http.NewRequestWithContext(context.Background(), "GET", "https://download.example/result", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer never-forward")
	req.Header.Set("Range", "bytes=0-3")
	req.Header.Set("X-Api-Key", "never-forward")
	resp, err := svc.doZhongYVideoDownload(req, "", seedanceAccount())
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 206, resp.StatusCode)
	require.Len(t, upstream.requests, 2)
	for _, r := range upstream.requests {
		require.Empty(t, r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("X-Api-Key"))
		require.Equal(t, "bytes=0-3", r.Header.Get("Range"))
		require.True(t, HTTPUpstreamPublicHostsOnly(r.Context()))
		require.True(t, HTTPUpstreamRedirectsDisabled(r.Context()))
	}
	require.Equal(t, "https://cdn.example/result%2Fclip.mp4?sig=abc%2F123", upstream.requests[1].URL.String())
}
func TestOpenAIVideoSeedanceDownloadBlocksPrivateAndUnsafeURLs(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	for _, target := range []string{"http://cdn.example/video", "https://127.0.0.1/video", "https://169.254.169.254/metadata", "https://user:pass@cdn.example/video", "https://localhost/video"} {
		_, err := svc.zhongYVideoDownloadURL(seedanceAccount(), target)
		require.Error(t, err)
	}
}

func TestOpenAIVideoAllowsAliyunResultHostOnlyForAliyunProviderAccount(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
		Enabled:       true,
		UpstreamHosts: []string{"api.aliyun.com"},
	}}}}

	account := seedanceAccount()
	account.Credentials["base_url"] = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	target, err := svc.zhongYVideoDownloadURL(account, "https://dashscope-result-shanghai.oss-cn-shanghai.aliyuncs.com/video.mp4?signature=redacted")
	require.NoError(t, err)
	require.Contains(t, target, "dashscope-result-shanghai.oss-cn-shanghai.aliyuncs.com")

	account.Credentials["base_url"] = "https://relay.example/v1"
	_, err = svc.zhongYVideoDownloadURL(account, "https://dashscope-result-shanghai.oss-cn-shanghai.aliyuncs.com/video.mp4")
	require.Error(t, err, "non-Aliyun upstream accounts must not inherit the exception")

	for _, host := range []string{"aliyuncs.com.evil.example", "evilaliyuncs.com"} {
		_, err := svc.zhongYVideoDownloadURL(account, "https://"+host+"/video.mp4")
		require.Error(t, err, "host %q must not be allowed", host)
	}
}

func TestOpenAIVideoSeedanceProtectedSameOriginContentKeepsAuthentication(t *testing.T) {
	upstream := &grokMediaContentUpstreamStub{responses: []*http.Response{
		grokMediaContentStatusResponse(`{"id":"task-1","status":"completed","seconds":"30","download_url":"/v1/videos/task-1/content"}`),
		{StatusCode: 200, Header: http.Header{"Content-Type": []string{"video/mp4"}}, Body: io.NopCloser(strings.NewReader("mp4!"))},
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	c, _ := grokMediaContentTestContext(http.MethodGet, "https://api.example/v1/videos/task-1/content", nil)
	result, err := svc.ForwardGrokMedia(context.Background(), c, seedanceAccount(), GrokMediaEndpointVideoContent, "task-1", nil, "")
	require.NoError(t, err)
	require.Equal(t, 30, result.VideoDurationSeconds)
	require.NotEmpty(t, upstream.requests[1].Header.Get("Authorization"))
	require.Equal(t, upstream.requests[0].URL.Host, upstream.requests[1].URL.Host)
}
