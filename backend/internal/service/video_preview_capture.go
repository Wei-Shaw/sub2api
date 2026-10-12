package service

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// Capture the successful status BEFORE public response URL rewriting. Queuing a
// download is independent of settlement and never invokes a gateway handler.
func (s *OpenAIGatewayService) captureCompletedVideo(c *gin.Context, account *Account, taskID string, body []byte) {
	if s == nil || s.videoPreviewCache == nil || c == nil || account == nil {
		return
	}
	keyID := getAPIKeyIDFromContext(c)
	if keyID <= 0 {
		keyID, _ = c.GetInt64(videoPreviewCacheOwnerKey), false
	}
	if keyID <= 0 || !gjson.ValidBytes(body) {
		return
	}
	id := extractGrokMediaVideoRequestID(body)
	if taskID == "" {
		taskID = id
	}
	if validateVideoTaskID(taskID) != nil || (id != "" && id != strings.TrimPrefix(taskID, "seedance:")) {
		return
	}
	var rawURL string
	if strings.HasPrefix(taskID, "seedance:") {
		if gjson.GetBytes(body, "status").String() != "succeeded" {
			return
		}
		rawURL = gjson.GetBytes(body, "content.video_url").String()
	} else if account.IsGrok() {
		if ExtractGrokVideoBillingFromStatusBody(body, nil, taskID) == nil {
			return
		}
		var err error
		rawURL, err = grokMediaSignedVideoContentURL(body, taskID)
		if err != nil {
			return
		}
	} else if account.IsOpenAIApiKey() {
		if openAIVideoCompletionForAccount(account, body, taskID) == nil {
			return
		}
		if isQianDreamVideoProtocol(account) {
			rawURL = qianDreamVideoOutputURL(body)
		} else {
			for _, field := range []string{"download_url", "video.url", "video_url", "url", "result_url"} {
				if value := gjson.GetBytes(body, field); value.Type == gjson.String && strings.TrimSpace(value.String()) != "" {
					rawURL = strings.TrimSpace(value.String())
					break
				}
			}
		}
	} else {
		return
	}
	if isGrokMediaVideoContentURL(rawURL, taskID) {
		rawURL = ""
	}
	// Copy the request's account snapshot; the pooled Gin context is never kept
	// by background jobs. Credentials are used only for the configured API host.
	snapshot := *account
	snapshot.Credentials, snapshot.Extra = maps.Clone(account.Credentials), maps.Clone(account.Extra)
	key := videoPreviewCacheKey(snapshot.ID, keyID, taskID)
	s.videoPreviewCache.enqueue(key, func(ctx context.Context, w http.ResponseWriter) error {
		capture, _ := gin.CreateTestContext(w)
		capture.Request, _ = http.NewRequestWithContext(ctx, http.MethodGet, "/internal-video-cache", nil)
		if rawURL == "" {
			return s.StreamAdminVideoPreview(capture, &snapshot, taskID)
		}
		if snapshot.IsGrok() {
			return s.streamGrokSignedVideoPreviewURL(capture, &snapshot, rawURL)
		}
		return s.streamVideoPreviewURL(capture, &snapshot, rawURL)
	})
}

// Reuse the validated public-media downloader: no credentials on signed links,
// no private DNS destinations, and every redirect is checked. GET only.
func (s *OpenAIGatewayService) streamVideoPreviewURL(c *gin.Context, account *Account, rawURL string) error {
	target, err := s.zhongYVideoDownloadURL(account, rawURL)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", c.GetHeader("Range"))
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.doZhongYVideoDownload(req, proxyURL, account)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return ErrAdminVideoPreviewUnavailable
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("video content unavailable")
	}
	contentType := strings.ToLower(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if !strings.HasPrefix(contentType, "video/") && contentType != "application/octet-stream" {
		return fmt.Errorf("upstream did not return video content")
	}
	return writeGrokMediaContentResponse(c, resp)
}

func (s *OpenAIGatewayService) streamGrokSignedVideoPreviewURL(c *gin.Context, account *Account, rawURL string) error {
	if !isTrustedVideoPreviewURL(rawURL) {
		return fmt.Errorf("invalid signed video URL")
	}
	req, err := http.NewRequestWithContext(WithHTTPUpstreamPublicHostsOnly(WithHTTPUpstreamRedirectsDisabled(c.Request.Context())), http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("signed video download returned status %d", resp.StatusCode)
	}
	contentType := strings.ToLower(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if !strings.HasPrefix(contentType, "video/") && contentType != "application/octet-stream" {
		return fmt.Errorf("upstream did not return video content")
	}
	return writeGrokMediaContentResponse(c, resp)
}
