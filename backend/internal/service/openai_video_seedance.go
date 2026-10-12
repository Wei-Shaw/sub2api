package service

import (
	"bytes"
	"fmt"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"io"
	"mime"
	"mime/multipart"
	"strconv"
	"strings"
)

// Select by account protocol, never by a particular model name.
func isZhongYVideoProtocol(account *Account) bool {
	if account == nil {
		return false
	}
	protocol, _ := account.Extra["openai_video_protocol"].(string)
	return account.Platform == PlatformOpenAI && protocol == "zhongy"
}

func NormalizeVideoDurationForPlatform(group *Group, seconds int) int {
	if group != nil && group.Platform == PlatformOpenAI && seconds > 0 {
		return seconds
	}
	return NormalizeVideoBillingDurationSecondsOrDefault(seconds)
}

func (s *OpenAIGatewayService) calculateVideoCostForPlatform(key *APIKey, model, resolution string, count, seconds int, cfg *VideoPriceConfig, multiplier float64) *CostBreakdown {
	if key != nil && key.Group != nil && key.Group.Platform == PlatformOpenAI && seconds > 0 {
		cost := s.billingService.CalculateVideoCost(model, resolution, count, 1, cfg, multiplier)
		cost.TotalCost *= float64(seconds)
		cost.ActualCost *= float64(seconds)
		return cost
	}
	return s.billingService.CalculateVideoCost(model, resolution, count, seconds, cfg, multiplier)
}

// ZhongY's current Seedance endpoint requires seconds as a string. Keep
// provider fields, translate OpenAI geometry, and preserve explicit aspect_ratio.
func prepareZhongYVideoBody(body []byte, contentType string, info GrokMediaRequestInfo) ([]byte, string, error) {
	fixedResolution := fixedOpenAIVideoResolution(info.Model)
	if gjson.ValidBytes(body) {
		out, err := sjson.SetBytes(body, "seconds", strconv.Itoa(info.DurationSeconds))
		if err != nil {
			return nil, "", err
		}
		out, err = sjson.DeleteBytes(out, "duration")
		if err != nil {
			return nil, "", err
		}
		if fixedResolution != "" {
			out, err = sjson.DeleteBytes(out, "resolution")
			if err != nil {
				return nil, "", err
			}
		}
		if info.Size != "" && fixedResolution == "" {
			if !gjson.GetBytes(out, "aspect_ratio").Exists() {
				out, err = sjson.SetBytes(out, "aspect_ratio", openAIVideoAspectRatio(info.Size))
				if err != nil {
					return nil, "", err
				}
			}
			if !gjson.GetBytes(out, "resolution").Exists() {
				out, err = sjson.SetBytes(out, "resolution", info.Resolution)
				if err != nil {
					return nil, "", err
				}
			}
			out, err = sjson.DeleteBytes(out, "size")
		}
		return out, "application/json", err
	}
	reader, err := openAIVideoMultipartReader(contentType, body)
	if err != nil {
		return nil, "", err
	}
	var out bytes.Buffer
	writer := multipart.NewWriter(&out)
	hasAspect, hasResolution := false, false
	for {
		part, err := reader.NextRawPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", err
		}
		data, err := readOpenAIVideoMultipartPart(part)
		if err != nil {
			return nil, "", err
		}
		_, params, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		if err != nil {
			return nil, "", err
		}
		name := part.FormName()
		_, file := params["filename"]
		if !file {
			if name == "seconds" || name == "duration" || name == "size" || (name == "resolution" && fixedResolution != "") {
				continue
			}
			hasAspect = hasAspect || name == "aspect_ratio"
			hasResolution = hasResolution || name == "resolution"
		}
		dst, err := writer.CreatePart(part.Header)
		if err != nil {
			return nil, "", err
		}
		if _, err = dst.Write(data); err != nil {
			return nil, "", err
		}
	}
	fields := map[string]string{"seconds": strconv.Itoa(info.DurationSeconds)}
	if info.Size != "" && fixedResolution == "" {
		if !hasAspect {
			fields["aspect_ratio"] = openAIVideoAspectRatio(info.Size)
		}
		if !hasResolution {
			fields["resolution"] = info.Resolution
		}
	}
	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return out.Bytes(), writer.FormDataContentType(), nil
}

func normalizeZhongYVideoResponse(body []byte) []byte {
	if data := gjson.GetBytes(body, "data"); data.IsObject() && (data.Get("status").Exists() || data.Get("id").Exists() || data.Get("task_id").Exists()) {
		body = []byte(data.Raw)
	}
	id := extractGrokMediaVideoRequestID(body)
	if id != "" {
		body, _ = sjson.SetBytes(body, "id", id)
		body, _ = sjson.SetBytes(body, "task_id", id)
		body, _ = sjson.SetBytes(body, "object", "video")
	}
	switch strings.ToLower(gjson.GetBytes(body, "status").String()) {
	case "processing", "running":
		body, _ = sjson.SetBytes(body, "status", "in_progress")
	case "pending":
		body, _ = sjson.SetBytes(body, "status", "queued")
	case "done", "succeeded", "success":
		body, _ = sjson.SetBytes(body, "status", "completed")
	}
	if duration := gjson.GetBytes(body, "duration"); duration.Exists() && !gjson.GetBytes(body, "seconds").Exists() {
		body, _ = sjson.SetBytes(body, "seconds", duration.String())
	}
	return body
}

// Unwrap nested ZhongY gateway error envelopes into one readable API error.
func normalizeZhongYVideoError(body []byte) []byte {
	for depth := 0; depth < 8; depth++ {
		value := gjson.ParseBytes(body)
		if !value.IsObject() {
			break
		}
		if e := value.Get("error"); e.IsObject() {
			body = []byte(e.Raw)
			continue
		}
		message := value.Get("message").String()
		if !gjson.Valid(message) {
			break
		}
		body = []byte(message)
	}
	message := gjson.GetBytes(body, "message").String()
	code := gjson.GetBytes(body, "code").String()
	if message == "" {
		message = fmt.Sprint("Seedance upstream request failed")
	}
	out, _ := sjson.Set("{}", "error.message", message)
	out, _ = sjson.Set(out, "error.code", code)
	return []byte(out)
}
