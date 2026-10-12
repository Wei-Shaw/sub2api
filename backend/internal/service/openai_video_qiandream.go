package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const qianDreamVideoProtocol = "qiandream"

func isQianDreamVideoProtocol(account *Account) bool {
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
		return false
	}
	protocol, _ := account.Extra["openai_video_protocol"].(string)
	return strings.EqualFold(strings.TrimSpace(protocol), qianDreamVideoProtocol)
}

func parseQianDreamVideoRequest(body []byte) (GrokMediaRequestInfo, error) {
	var info GrokMediaRequestInfo
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return info, fmt.Errorf("QianDream video requires application/json")
	}
	info = ParseOpenAIVideoRequest("application/json", body)
	if strings.TrimSpace(info.Model) == "" {
		return info, fmt.Errorf("model is required")
	}
	if strings.TrimSpace(info.Prompt) == "" && len(info.InputImageURLs) == 0 {
		return info, fmt.Errorf("prompt or reference image is required")
	}
	durationValue := gjson.GetBytes(body, "duration")
	if !durationValue.Exists() {
		durationValue = gjson.GetBytes(body, "seconds")
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(durationValue.String()))
	if !durationValue.Exists() || (durationValue.Type != gjson.Number && durationValue.Type != gjson.String) || err != nil || seconds < VideoBillingMinDurationSeconds || seconds > 3600 {
		return info, fmt.Errorf("duration must be an integer between %d and 3600", VideoBillingMinDurationSeconds)
	}
	info.DurationSeconds = seconds
	if strings.TrimSpace(info.Resolution) == "" {
		return info, fmt.Errorf("resolution is required")
	}
	return info, nil
}

func prepareQianDreamVideoBody(account *Account, body []byte, info GrokMediaRequestInfo) ([]byte, string, GrokMediaRequestInfo, string, error) {
	// Reuse the native OpenAI control validation so the new adapter retains
	// duplicate-field and conflicting-resolution protections on /v1/videos.
	if _, _, _, _, err := prepareOpenAIVideoBody(account, body, "application/json"); err != nil {
		return nil, "", info, "", err
	}
	parsed, err := parseQianDreamVideoRequest(body)
	if err != nil {
		return nil, "", parsed, "", err
	}
	mapped := strings.TrimSpace(account.GetMappedModel(parsed.Model))
	if mapped == "" {
		mapped = parsed.Model
	}
	var source map[string]json.RawMessage
	if err := json.Unmarshal(body, &source); err != nil {
		return nil, "", parsed, "", fmt.Errorf("invalid QianDream video request: %w", err)
	}
	result := make(map[string]json.RawMessage, len(source)+4)
	for _, field := range []string{
		"model", "prompt", "resolution", "ratio", "images", "videos", "audios",
		"generate_audio", "negative_prompt", "seed", "watermark",
	} {
		if value, ok := source[field]; ok {
			result[field] = value
		}
	}
	modelJSON, _ := json.Marshal(mapped)
	result["model"] = modelJSON
	promptJSON, _ := json.Marshal(parsed.Prompt)
	result["prompt"] = promptJSON
	durationJSON, _ := json.Marshal(parsed.DurationSeconds)
	result["duration"] = durationJSON
	resolutionJSON, _ := json.Marshal(parsed.Resolution)
	result["resolution"] = resolutionJSON
	delete(result, "ratio")
	ratio := strings.TrimSpace(gjson.GetBytes(body, "ratio").String())
	if ratio == "" {
		ratio = strings.TrimSpace(gjson.GetBytes(body, "aspect_ratio").String())
	}
	if ratio != "" {
		ratioJSON, _ := json.Marshal(ratio)
		result["ratio"] = ratioJSON
	}
	if _, exists := result["images"]; !exists && len(parsed.InputImageURLs) > 0 {
		imagesJSON, marshalErr := json.Marshal(parsed.InputImageURLs)
		if marshalErr != nil {
			return nil, "", parsed, "", marshalErr
		}
		result["images"] = imagesJSON
	}
	out, err := json.Marshal(result)
	if err != nil {
		return nil, "", parsed, "", err
	}
	return out, "application/json", parsed, mapped, nil
}

func qianDreamVideoOutputURL(body []byte) string {
	if !gjson.ValidBytes(body) {
		return ""
	}
	var find func(gjson.Result) string
	find = func(value gjson.Result) string {
		if !value.Exists() {
			return ""
		}
		for _, path := range []string{"video_url", "download_url", "content.video_url", "output.video_url", "video.url"} {
			if candidate := strings.TrimSpace(value.Get(path).String()); candidate != "" {
				return candidate
			}
		}
		if candidate := strings.TrimSpace(value.Get("url").String()); candidate != "" {
			return candidate
		}
		for _, key := range []string{"data", "result", "task", "content", "output"} {
			child := value.Get(key)
			if child.IsArray() {
				for _, item := range child.Array() {
					if candidate := find(item); candidate != "" {
						return candidate
					}
				}
			} else if candidate := find(child); candidate != "" {
				return candidate
			}
		}
		return ""
	}
	return find(gjson.ParseBytes(body))
}

func qianDreamVideoStatus(body []byte) string {
	if status := strings.ToLower(strings.TrimSpace(qianDreamVideoField(body, "status").String())); status != "" {
		return status
	}
	return ""
}

func qianDreamVideoField(body []byte, field string) gjson.Result {
	if !gjson.ValidBytes(body) {
		return gjson.Result{}
	}
	var find func(gjson.Result) gjson.Result
	find = func(value gjson.Result) gjson.Result {
		if direct := value.Get(field); direct.Exists() {
			return direct
		}
		for _, key := range []string{"data", "result", "task", "content", "output"} {
			child := value.Get(key)
			if child.IsArray() {
				for _, item := range child.Array() {
					if found := find(item); found.Exists() {
						return found
					}
				}
			} else if child.Exists() {
				if found := find(child); found.Exists() {
					return found
				}
			}
		}
		return gjson.Result{}
	}
	return find(gjson.ParseBytes(body))
}

func normalizeQianDreamVideoResponse(body []byte) []byte {
	if !gjson.ValidBytes(body) {
		return body
	}
	id := extractGrokMediaVideoRequestID(body)
	if id == "" {
		id = qianDreamVideoRequestID(body)
	}
	if id != "" {
		body, _ = sjson.SetBytes(body, "id", id)
		body, _ = sjson.SetBytes(body, "task_id", id)
		body, _ = sjson.SetBytes(body, "object", "video")
	}
	if output := qianDreamVideoOutputURL(body); output != "" {
		body, _ = sjson.SetBytes(body, "video_url", output)
		body, _ = sjson.SetBytes(body, "download_url", output)
		body, _ = sjson.SetBytes(body, "video.url", output)
	}
	if duration := qianDreamVideoField(body, "duration"); duration.Exists() && !gjson.GetBytes(body, "duration").Exists() {
		body, _ = sjson.SetBytes(body, "duration", duration.Value())
	}
	if resolution := qianDreamVideoField(body, "resolution"); resolution.Exists() && !gjson.GetBytes(body, "resolution").Exists() {
		body, _ = sjson.SetBytes(body, "resolution", resolution.Value())
	}
	switch qianDreamVideoStatus(body) {
	case "pending", "queued":
		body, _ = sjson.SetBytes(body, "status", "queued")
	case "running", "processing":
		body, _ = sjson.SetBytes(body, "status", "in_progress")
	case "succeeded", "success", "done", "completed":
		body, _ = sjson.SetBytes(body, "status", "completed")
	}
	return body
}

func qianDreamVideoRequestID(body []byte) string {
	if !gjson.ValidBytes(body) {
		return ""
	}
	var find func(gjson.Result) string
	find = func(value gjson.Result) string {
		for _, key := range []string{"request_id", "task_id", "id"} {
			if id := strings.TrimSpace(value.Get(key).String()); id != "" {
				return id
			}
		}
		for _, key := range []string{"data", "result", "task", "content", "output"} {
			child := value.Get(key)
			if child.IsArray() {
				for _, item := range child.Array() {
					if id := find(item); id != "" {
						return id
					}
				}
			} else if child.Exists() {
				if id := find(child); id != "" {
					return id
				}
			}
		}
		return ""
	}
	return find(gjson.ParseBytes(body))
}

func (s *OpenAIGatewayService) qianDreamVideoDownloadURL(account *Account, raw string) (string, error) {
	base, err := url.Parse(account.GetOpenAIBaseURL())
	if err != nil {
		return "", fmt.Errorf("invalid video download URL")
	}
	target, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("invalid video download URL")
	}
	target = base.ResolveReference(target)
	if !strings.EqualFold(target.Scheme, "https") || target.User != nil || target.Fragment != "" {
		return "", fmt.Errorf("unsupported video download URL")
	}
	if _, err = urlvalidator.ValidateHTTPSURL(target.String(), urlvalidator.ValidationOptions{}); err != nil {
		return "", fmt.Errorf("video download URL rejected")
	}
	if _, err = s.validateOutboundURL(target.String()); err != nil {
		return "", fmt.Errorf("video download URL rejected by outbound policy")
	}
	return target.String(), nil
}

// QianDream returns public result URLs rather than an authenticated /content
// payload. Follow only validated public redirects and never attach the API key.
func (s *OpenAIGatewayService) doQianDreamVideoDownload(req *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	ctx := WithHTTPUpstreamPublicHostsOnly(WithHTTPUpstreamRedirectsDisabled(req.Context()))
	current := req.URL.String()
	for hop := 0; hop < 6; hop++ {
		target, err := s.qianDreamVideoDownloadURL(account, current)
		if err != nil {
			return nil, err
		}
		next, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, err
		}
		next.Header.Set("Accept", "*/*")
		if value := req.Header.Get("Range"); value != "" {
			next.Header.Set("Range", value)
		}
		resp, err := s.httpUpstream.Do(next, proxyURL, account.ID, account.Concurrency)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode < 300 || resp.StatusCode >= 400 {
			return resp, nil
		}
		location := strings.TrimSpace(resp.Header.Get("Location"))
		_ = resp.Body.Close()
		if location == "" {
			return nil, fmt.Errorf("video download redirect missing location")
		}
		ref, err := url.Parse(location)
		if err != nil {
			return nil, fmt.Errorf("invalid video download redirect")
		}
		current = next.URL.ResolveReference(ref).String()
	}
	return nil, fmt.Errorf("too many video download redirects")
}
