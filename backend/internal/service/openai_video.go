package service

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func supportsOpenAIVideoEndpoint(endpoint GrokMediaEndpoint) bool {
	return endpoint == GrokMediaEndpointVideosGenerations || endpoint.IsVideoLookupRequest()
}

func buildOpenAIVideoURL(account *Account, cfg *config.Config, endpoint GrokMediaEndpoint, requestID string) (string, error) {
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey || !supportsOpenAIVideoEndpoint(endpoint) {
		return "", fmt.Errorf("OpenAI video requires an API-key account and a supported video endpoint")
	}
	svc := &OpenAIGatewayService{cfg: cfg}
	base, err := svc.validateUpstreamBaseURL(account.GetOpenAIBaseURL())
	if err != nil {
		return "", err
	}
	upstreamPath := "/v1/videos"
	if isQianDreamVideoProtocol(account) {
		upstreamPath = "/v1/video/generations"
	}
	target, err := url.Parse(buildOpenAIEndpointURL(base, upstreamPath))
	if err != nil {
		return "", err
	}
	if endpoint.IsVideoLookupRequest() {
		if err := validateUpstreamPathSegment("video task id", requestID); err != nil {
			return "", err
		}
		target.Path += "/" + strings.TrimSpace(requestID)
		if endpoint == GrokMediaEndpointVideoContent && !isQianDreamVideoProtocol(account) {
			target.Path += "/content"
		}
	}
	return target.String(), nil
}

// OpenAI video fields are not xAI image fields: preserve their wire format,
// including provider-specific model names, seconds, size, and reference uploads.
func ParseOpenAIVideoRequest(contentType string, body []byte) GrokMediaRequestInfo {
	info := GrokMediaRequestInfo{N: 1}
	fields := map[string]string{}
	if gjson.ValidBytes(body) {
		info = ParseGrokMediaRequest(contentType, body)
		fields["seconds"] = gjson.GetBytes(body, "seconds").String()
		fields["duration"] = gjson.GetBytes(body, "duration").String()
		fields["resolution"] = gjson.GetBytes(body, "resolution").String()
		if ref := extractGrokMediaImageURL(gjson.GetBytes(body, "input_reference")); ref != "" {
			info.InputImageURLs = append(info.InputImageURLs, ref)
		}
	} else if reader, err := openAIVideoMultipartReader(contentType, body); err == nil {
		for {
			part, err := reader.NextRawPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return GrokMediaRequestInfo{}
			}
			data, err := readOpenAIVideoMultipartPart(part)
			if err != nil {
				return GrokMediaRequestInfo{}
			}
			_ = part.Close()
			if strings.EqualFold(part.Header.Get("Content-Transfer-Encoding"), "quoted-printable") {
				data, err = readOpenAIVideoMultipartPart(quotedprintable.NewReader(bytes.NewReader(data)))
				if err != nil {
					return GrokMediaRequestInfo{}
				}
			}
			_, params, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
			if err != nil {
				return GrokMediaRequestInfo{}
			}
			name := part.FormName()
			if _, isFile := params["filename"]; !isFile {
				fields[name] = strings.TrimSpace(string(data))
			} else {
				upload := OpenAIImagesUpload{
					FieldName: name, FileName: part.FileName(),
					ContentType: part.Header.Get("Content-Type"), Data: data,
				}
				switch {
				case name == "mask":
					info.MaskUpload = &upload
				case name == "input_reference", name == "image", strings.HasPrefix(name, "image["):
					info.Uploads = append(info.Uploads, upload)
				}
			}
		}
		info.Model = fields["model"]
		info.Prompt = fields["prompt"]
		info.Size = fields["size"]
		info.AspectRatio = fields["aspect_ratio"]
		assignGrokMediaResolution(fields["resolution"], &info)
		info.ImageResolution = grokImagineImageResolution(info.ImageResolution)
		info.SizeTier = NormalizeImageBillingTierOrDefault(info.Size)
		if n, err := strconv.Atoi(fields["n"]); err == nil && n > 0 {
			info.N = n
		}
		for _, field := range []string{"image", "image_url", "input_reference"} {
			if ref := fields[field]; ref != "" {
				info.InputImageURLs = append(info.InputImageURLs, ref)
			}
		}
		info.MaskImageURL = firstNonEmpty(fields["mask_image_url"], fields["mask"])
	}
	// Do not invent xAI defaults for another provider. Submit/status responses
	// can supply the provider's actual defaults for deferred billing.
	info.DurationSeconds = 0
	for _, field := range []string{"seconds", "duration"} {
		if seconds, err := strconv.Atoi(fields[field]); err == nil && seconds > 0 {
			info.DurationSeconds = seconds
			break
		}
	}
	info.Resolution = ""
	if raw := strings.TrimSpace(fields["resolution"]); raw != "" {
		info.Resolution = NormalizeVideoBillingResolutionOrDefault(raw)
	}
	if resolution := openAIVideoSizeResolution(info.Size); resolution != "" {
		info.Resolution = resolution
	}
	if fixed := fixedOpenAIVideoResolution(info.Model); fixed != "" {
		info.Resolution = fixed
	}
	return info
}

func openAIVideoSizeResolution(size string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(size)), "x")
	if len(parts) != 2 {
		return ""
	}
	width, ew := strconv.Atoi(parts[0])
	height, eh := strconv.Atoi(parts[1])
	if ew != nil || eh != nil || width <= 0 || height <= 0 {
		return ""
	}
	short := min(width, height)
	return NormalizeVideoBillingResolutionOrDefault(strconv.Itoa(short) + "p")
}

func openAIVideoMultipartReader(contentType string, body []byte) (*multipart.Reader, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") || params["boundary"] == "" {
		return nil, fmt.Errorf("video request must be JSON or multipart/form-data")
	}
	return multipart.NewReader(bytes.NewReader(body), params["boundary"]), nil
}

func readOpenAIVideoMultipartPart(part io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(part, openAIImageMaxUploadPartSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > openAIImageMaxUploadPartSize {
		return nil, fmt.Errorf("video multipart part exceeds %d bytes", openAIImageMaxUploadPartSize)
	}
	return data, nil
}

func isOpenAIGrokVideoProtocol(account *Account) bool {
	if account == nil {
		return false
	}
	protocol, _ := account.Extra["openai_video_protocol"].(string)
	return protocol == "grok"
}

func fixedOpenAIVideoResolution(model string) string {
	lower := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(lower, "1440p"):
		return "1440p"
	case strings.Contains(lower, "768p"):
		return "768p"
	default:
		return ""
	}
}

func validateOpenAIVideoDuration(value string, previous *int, accounts ...*Account) error {
	maxDuration := VideoBillingMaxDurationSeconds
	if len(accounts) > 0 && accounts[0] != nil && accounts[0].Platform == PlatformOpenAI && !isOpenAIGrokVideoProtocol(accounts[0]) {
		maxDuration = 3600
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds < VideoBillingMinDurationSeconds || seconds > maxDuration {
		return fmt.Errorf("video seconds/duration must be an integer between 1 and %d", maxDuration)
	}
	if *previous != 0 && *previous != seconds {
		return fmt.Errorf("video seconds/duration fields must agree")
	}
	*previous = seconds
	return nil
}

func validateOpenAIVideoResponseBilling(body []byte, accounts ...*Account) error {
	var seconds int
	var validationErr error
	gjson.ParseBytes(body).ForEach(func(key, value gjson.Result) bool {
		switch key.String() {
		case "seconds", "duration":
			validationErr = validateOpenAIVideoDuration(value.String(), &seconds, accounts...)
		case "video":
			value.ForEach(func(key, value gjson.Result) bool {
				if key.String() == "duration" {
					validationErr = validateOpenAIVideoDuration(value.String(), &seconds, accounts...)
				}
				return validationErr == nil
			})
		}
		return validationErr == nil
	})
	if validationErr != nil {
		return validationErr
	}
	return validateOpenAIVideoOriginalResolution(body, "application/json")
}

func validateOpenAIVideoControlField(name string, isFile bool) error {
	canonical := strings.ToLower(name)
	switch canonical {
	case "model", "seconds", "duration", "size", "resolution":
		if name != canonical {
			return fmt.Errorf("video control field %q must use canonical name %q", name, canonical)
		}
		if isFile {
			return fmt.Errorf("video control field %q must not be a file part", name)
		}
	}
	return nil
}

func prepareOpenAIVideoBody(account *Account, body []byte, contentType string) ([]byte, string, GrokMediaRequestInfo, string, error) {
	info := ParseOpenAIVideoRequest(contentType, body)
	mapped := account.GetMappedModel(info.Model)
	if mapped == "" {
		mapped = info.Model
	}
	if gjson.ValidBytes(body) {
		if !gjson.ParseBytes(body).IsObject() {
			return nil, "", info, "", fmt.Errorf("video request must be an object")
		}
		modelCount := 0
		var controlErr error
		gjson.ParseBytes(body).ForEach(func(key, _ gjson.Result) bool {
			if controlErr == nil {
				controlErr = validateOpenAIVideoControlField(key.String(), false)
			}
			if strings.EqualFold(key.String(), "model") {
				modelCount++
			}
			return modelCount < 2
		})
		if modelCount > 1 {
			return nil, "", info, "", fmt.Errorf("duplicate video model fields are not allowed")
		}
		if controlErr != nil {
			return nil, "", info, "", controlErr
		}
		var seconds int
		var durationErr error
		gjson.ParseBytes(body).ForEach(func(key, value gjson.Result) bool {
			if key.String() == "seconds" || key.String() == "duration" {
				durationErr = validateOpenAIVideoDuration(value.String(), &seconds, account)
			}
			return durationErr == nil
		})
		if durationErr != nil {
			return nil, "", info, "", durationErr
		}
		out, err := sjson.SetBytes(body, "model", mapped)
		if err != nil {
			return nil, "", info, "", err
		}
		// Only explicitly configured Grok relays need size converted to aspect_ratio.
		if isOpenAIGrokVideoProtocol(account) && strings.TrimSpace(gjson.GetBytes(out, "aspect_ratio").String()) == "" {
			if aspectRatio := openAIVideoAspectRatio(info.Size); aspectRatio != "" {
				out, err = sjson.SetBytes(out, "aspect_ratio", aspectRatio)
				if err != nil {
					return nil, "", info, "", err
				}
				out, err = sjson.DeleteBytes(out, "size")
			}
		}
		return out, "application/json", info, mapped, err
	}
	reader, err := openAIVideoMultipartReader(contentType, body)
	if err != nil {
		return nil, "", info, "", err
	}
	// Scan before rewriting so an explicit aspect_ratio wins in either order.
	hasAspectRatio := false
	modelCount := 0
	var seconds int
	for {
		part, err := reader.NextRawPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", info, "", fmt.Errorf("invalid video multipart body: %w", err)
		}
		_, params, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		if err != nil {
			return nil, "", info, "", fmt.Errorf("invalid video multipart disposition: %w", err)
		}
		_, isFile := params["filename"]
		// Presence matters here: Go ReadForm treats filename="" as a scalar,
		// while other parsers treat it as a file. Neither is a valid control.
		if err := validateOpenAIVideoControlField(part.FormName(), isFile); err != nil {
			return nil, "", info, "", err
		}
		if !isFile && strings.EqualFold(part.FormName(), "model") {
			modelCount++
			if modelCount > 1 {
				return nil, "", info, "", fmt.Errorf("duplicate video model fields are not allowed")
			}
		}
		if part.FormName() == "aspect_ratio" && !isFile {
			hasAspectRatio = true
		}
		if name := part.FormName(); !isFile && (name == "seconds" || name == "duration") {
			var source io.Reader = part
			if strings.EqualFold(part.Header.Get("Content-Transfer-Encoding"), "quoted-printable") {
				source = quotedprintable.NewReader(part)
			}
			data, err := readOpenAIVideoMultipartPart(source)
			if err != nil {
				return nil, "", info, "", err
			}
			if err := validateOpenAIVideoDuration(string(data), &seconds, account); err != nil {
				return nil, "", info, "", err
			}
		}
		n, err := io.Copy(io.Discard, io.LimitReader(part, openAIImageMaxUploadPartSize+1))
		if err != nil {
			return nil, "", info, "", fmt.Errorf("invalid video multipart body: %w", err)
		}
		if n > openAIImageMaxUploadPartSize {
			return nil, "", info, "", fmt.Errorf("video multipart part exceeds %d bytes", openAIImageMaxUploadPartSize)
		}
		_ = part.Close()
	}
	reader, err = openAIVideoMultipartReader(contentType, body)
	if err != nil {
		return nil, "", info, "", err
	}
	var out bytes.Buffer
	writer := multipart.NewWriter(&out)
	for {
		part, err := reader.NextRawPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", info, "", fmt.Errorf("invalid video multipart body: %w", err)
		}
		data, err := readOpenAIVideoMultipartPart(part)
		if err != nil {
			return nil, "", info, "", err
		}
		_ = part.Close()
		header := part.Header
		disposition, params, err := mime.ParseMediaType(header.Get("Content-Disposition"))
		if err != nil {
			return nil, "", info, "", err
		}
		_, isFile := params["filename"]
		fieldName := part.FormName()
		rewritten := false
		quotedPrintable := strings.EqualFold(header.Get("Content-Transfer-Encoding"), "quoted-printable")
		if !isFile && (fieldName == "model" || (fieldName == "size" && !hasAspectRatio && isOpenAIGrokVideoProtocol(account))) {
			value := data
			if quotedPrintable {
				value, err = readOpenAIVideoMultipartPart(quotedprintable.NewReader(bytes.NewReader(data)))
				if err != nil {
					return nil, "", info, "", err
				}
			}
			if fieldName == "model" {
				data = []byte(mapped)
				rewritten = true
			} else if aspectRatio := openAIVideoAspectRatio(string(value)); aspectRatio != "" {
				params["name"] = "aspect_ratio"
				header.Set("Content-Disposition", mime.FormatMediaType(disposition, params))
				data = []byte(aspectRatio)
				rewritten = true
			}
		}
		dst, err := writer.CreatePart(header)
		if err != nil {
			return nil, "", info, "", err
		}
		if rewritten && quotedPrintable {
			encoded := quotedprintable.NewWriter(dst)
			if _, err = encoded.Write(data); err == nil {
				err = encoded.Close()
			}
		} else {
			_, err = dst.Write(data)
		}
		if err != nil {
			return nil, "", info, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", info, "", err
	}
	return out.Bytes(), writer.FormDataContentType(), info, mapped, nil
}

func openAIVideoAspectRatio(size string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(size)), "x")
	if len(parts) != 2 {
		return ""
	}
	width, ew := strconv.Atoi(parts[0])
	height, eh := strconv.Atoi(parts[1])
	if ew != nil || eh != nil || width <= 0 || height <= 0 {
		return ""
	}
	switch {
	case width == height:
		return "1:1"
	case width > height:
		return "16:9"
	default:
		return "9:16"
	}
}

// Native OpenAI completion has no video.url; the protected /content endpoint is
// the download contract. Do not relax xAI's separate done+video.url billing rule.
func openAIVideoCompletion(body []byte, taskID string) *OpenAIForwardResult {
	status := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "status").String()))
	if status != "completed" {
		return nil
	}
	seconds := 0
	for _, path := range []string{"seconds", "duration", "video.duration"} {
		if value, err := strconv.Atoi(gjson.GetBytes(body, path).String()); err == nil && value > 0 {
			seconds = value
			break
		}
	}
	resolution := openAIVideoSizeResolution(gjson.GetBytes(body, "size").String())
	if resolution == "" {
		resolution, _ = LookupVideoBillingResolution(gjson.GetBytes(body, "resolution").String())
	}
	return &OpenAIForwardResult{
		ResponseID:    firstNonEmpty(extractGrokMediaVideoRequestID(body), taskID),
		UpstreamModel: strings.TrimSpace(gjson.GetBytes(body, "model").String()),
		VideoCount:    1, VideoDurationSeconds: seconds,
		VideoResolution: resolution,
	}
}

func qianDreamVideoCompletion(body []byte, taskID string) *OpenAIForwardResult {
	status := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "status").String()))
	if status != "completed" || qianDreamVideoOutputURL(body) == "" {
		return nil
	}
	seconds := 0
	for _, path := range []string{"duration", "seconds", "video.duration"} {
		if value, err := strconv.Atoi(gjson.GetBytes(body, path).String()); err == nil && value > 0 {
			seconds = value
			break
		}
	}
	resolution := NormalizeVideoBillingResolutionOrDefault(gjson.GetBytes(body, "resolution").String())
	return &OpenAIForwardResult{
		ResponseID:    firstNonEmpty(extractGrokMediaVideoRequestID(body), taskID),
		UpstreamModel: strings.TrimSpace(gjson.GetBytes(body, "model").String()),
		VideoCount:    1, VideoDurationSeconds: seconds,
		VideoResolution: resolution, VideoURL: qianDreamVideoOutputURL(body),
	}
}

func openAIVideoCompletionForAccount(account *Account, body []byte, taskID string) *OpenAIForwardResult {
	if isQianDreamVideoProtocol(account) {
		return qianDreamVideoCompletion(body, taskID)
	}
	return openAIVideoCompletion(body, taskID)
}

func openAIVideoUsage(endpoint GrokMediaEndpoint, info GrokMediaRequestInfo, body []byte, taskID string) grokMediaUsageMetadata {
	meta := grokMediaUsageFromResponse(endpoint, info, body)
	if endpoint == GrokMediaEndpointVideosGenerations {
		// Prefer explicit upstream defaults over assumptions made before submit.
		if seconds, err := strconv.Atoi(gjson.GetBytes(body, "seconds").String()); err == nil && seconds > 0 {
			meta.VideoDurationSeconds = seconds
		}
		if resolution := openAIVideoSizeResolution(gjson.GetBytes(body, "size").String()); resolution != "" {
			meta.VideoResolution = resolution
		}
	} else if endpoint.IsVideoLookupRequest() {
		meta = grokMediaUsageMetadata{ResponseID: taskID}
		if result := openAIVideoCompletion(body, taskID); result != nil {
			meta.ResponseID = result.ResponseID
			meta.VideoCount = result.VideoCount
			meta.VideoDurationSeconds = result.VideoDurationSeconds
			meta.VideoResolution = result.VideoResolution
		}
	}
	return meta
}

func openAIVideoUsageForAccount(account *Account, endpoint GrokMediaEndpoint, info GrokMediaRequestInfo, body []byte, taskID string) grokMediaUsageMetadata {
	if !isQianDreamVideoProtocol(account) || !endpoint.IsVideoLookupRequest() {
		return openAIVideoUsage(endpoint, info, body, taskID)
	}
	meta := grokMediaUsageMetadata{ResponseID: taskID}
	if result := qianDreamVideoCompletion(body, taskID); result != nil {
		meta.ResponseID = result.ResponseID
		meta.Model = result.Model
		meta.BillingModel = result.BillingModel
		meta.VideoCount = result.VideoCount
		meta.VideoDurationSeconds = result.VideoDurationSeconds
		meta.VideoResolution = result.VideoResolution
	}
	return meta
}

func rewriteOpenAIVideoContentURLs(body []byte, taskID, proxyURL string) []byte {
	for _, path := range []string{"video.url", "url", "video_url", "result_url", "download_url"} {
		if value := gjson.GetBytes(body, path); value.Type == gjson.String && strings.TrimSpace(value.String()) != "" {
			if rewritten, err := sjson.SetBytes(body, path, proxyURL); err == nil {
				body = rewritten
			}
		}
	}
	return body
}

func stripOpenAIVideoContentHeaders(headers http.Header) {
	// A JSON response can still leak a signed result URL in navigation headers.
	// Downloads must always pass through the owned, settled content endpoint.
	for _, name := range []string{"Location", "Content-Location", "Link", "Refresh"} {
		headers.Del(name)
	}
}

func rejectOpenAIVideoRedirect(resp *http.Response) error {
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return fmt.Errorf("OpenAI video upstream redirect is not allowed")
	}
	return nil
}
