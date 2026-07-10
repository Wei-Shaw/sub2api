package service

import (
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	VideoContentTypeText     = "text"
	VideoContentTypeImageURL = "image_url"
	VideoContentTypeVideoURL = "video_url"
	VideoContentTypeAudioURL = "audio_url"

	VideoContentRoleFirstFrame     = "first_frame"
	VideoContentRoleLastFrame      = "last_frame"
	VideoContentRoleReferenceImage = "reference_image"
	VideoContentRoleReferenceVideo = "reference_video"
	VideoContentRoleReferenceAudio = "reference_audio"
)

func normalizeVideoTaskContent(p VideoTaskCreateParams) ([]VideoTaskContentItem, bool, error) {
	items := make([]VideoTaskContentItem, 0, len(p.Content)+3)
	prompt := strings.TrimSpace(p.Prompt)
	enforceModeMatch := len(p.Content) > 0

	hasText := false
	for _, raw := range p.Content {
		item, err := normalizeVideoContentItem(raw, p.TaskType)
		if err != nil {
			return nil, false, err
		}
		if item.Type == VideoContentTypeText {
			if hasText {
				return nil, false, badVideoContent("content may contain at most one text item")
			}
			hasText = true
			if strings.TrimSpace(item.Text) == "" {
				item.Text = prompt
			}
		}
		items = append(items, item)
	}

	if !hasText {
		items = append([]VideoTaskContentItem{{Type: VideoContentTypeText, Text: prompt}}, items...)
	}

	if legacyImageURL := strings.TrimSpace(p.ReferenceImageURL); legacyImageURL != "" {
		if isSafeDemoPlaceholderURL(legacyImageURL) && p.SafeDemoOnly {
			// Drama safe-demo tasks use internal asset placeholders, not provider-callable URLs.
		} else if isHTTPContentURL(legacyImageURL) {
			role := VideoContentRoleReferenceImage
			if strings.TrimSpace(p.TaskType) == VideoTaskTypeImageToVideo {
				role = VideoContentRoleFirstFrame
			}
			item, err := normalizeVideoContentItem(VideoTaskContentItem{Type: VideoContentTypeImageURL, Role: role, URL: legacyImageURL}, p.TaskType)
			if err != nil {
				return nil, false, err
			}
			items = append(items, item)
			enforceModeMatch = true
		} else {
			return nil, false, badVideoContent("reference_image_url must be an http(s) URL")
		}
	}
	if legacyVideoURL := strings.TrimSpace(p.ReferenceVideoURL); legacyVideoURL != "" {
		if isSafeDemoPlaceholderURL(legacyVideoURL) && p.SafeDemoOnly {
			// Drama safe-demo tasks use internal asset placeholders, not provider-callable URLs.
		} else if isHTTPContentURL(legacyVideoURL) {
			item, err := normalizeVideoContentItem(VideoTaskContentItem{Type: VideoContentTypeVideoURL, Role: VideoContentRoleReferenceVideo, URL: legacyVideoURL}, p.TaskType)
			if err != nil {
				return nil, false, err
			}
			items = append(items, item)
			enforceModeMatch = true
		} else {
			return nil, false, badVideoContent("reference_video_url must be an http(s) URL")
		}
	}

	if err := validateVideoContentContract(p.TaskType, items, enforceModeMatch); err != nil {
		return nil, false, err
	}

	hasVideoInput := false
	for _, item := range items {
		if item.Type == VideoContentTypeVideoURL {
			hasVideoInput = true
			break
		}
	}
	return items, hasVideoInput, nil
}

func normalizeVideoContentItem(raw VideoTaskContentItem, taskType string) (VideoTaskContentItem, error) {
	item := VideoTaskContentItem{
		Type: strings.ToLower(strings.TrimSpace(raw.Type)),
		Role: strings.ToLower(strings.TrimSpace(raw.Role)),
		URL:  strings.TrimSpace(raw.URL),
		Text: strings.TrimSpace(raw.Text),
	}
	if item.Type == "" {
		return item, badVideoContent("content item type is required")
	}

	switch item.Type {
	case VideoContentTypeText:
		item.Role = ""
		item.URL = ""
	case VideoContentTypeImageURL:
		if item.Role == "" {
			item.Role = VideoContentRoleReferenceImage
			if strings.TrimSpace(taskType) == VideoTaskTypeImageToVideo {
				item.Role = VideoContentRoleFirstFrame
			}
		}
		if item.Role != VideoContentRoleFirstFrame && item.Role != VideoContentRoleLastFrame && item.Role != VideoContentRoleReferenceImage {
			return item, badVideoContent("image_url role must be first_frame, last_frame, or reference_image")
		}
		if err := validateContentURL(item.URL, "image_url"); err != nil {
			return item, err
		}
	case VideoContentTypeVideoURL:
		if item.Role == "" {
			item.Role = VideoContentRoleReferenceVideo
		}
		if item.Role != VideoContentRoleReferenceVideo {
			return item, badVideoContent("video_url role must be reference_video")
		}
		if err := validateContentURL(item.URL, "video_url"); err != nil {
			return item, err
		}
	case VideoContentTypeAudioURL:
		if item.Role == "" {
			item.Role = VideoContentRoleReferenceAudio
		}
		if item.Role != VideoContentRoleReferenceAudio {
			return item, badVideoContent("audio_url role must be reference_audio")
		}
		if err := validateContentURL(item.URL, "audio_url"); err != nil {
			return item, err
		}
	default:
		return item, badVideoContent("content item type must be text, image_url, video_url, or audio_url")
	}
	return item, nil
}

func validateContentURL(rawURL, label string) error {
	if strings.TrimSpace(rawURL) == "" {
		return badVideoContent(fmt.Sprintf("%s url is required", label))
	}
	if err := validateExternalVideoURL(rawURL); err != nil {
		return infraerrors.BadRequest("VIDEO_UNSAFE_REFERENCE_URL", label+" failed SSRF/allowlist validation: "+err.Error())
	}
	return nil
}

func isHTTPContentURL(rawURL string) bool {
	value := strings.ToLower(strings.TrimSpace(rawURL))
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}

func isSafeDemoPlaceholderURL(rawURL string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(rawURL)), "safe-demo://")
}

func validateVideoContentContract(taskType string, items []VideoTaskContentItem, enforceModeMatch bool) error {
	var (
		textCount           int
		firstFrameCount     int
		lastFrameCount      int
		referenceImageCount int
		videoCount          int
		audioCount          int
	)

	for _, item := range items {
		switch item.Type {
		case VideoContentTypeText:
			textCount++
		case VideoContentTypeImageURL:
			switch item.Role {
			case VideoContentRoleFirstFrame:
				firstFrameCount++
			case VideoContentRoleLastFrame:
				lastFrameCount++
			case VideoContentRoleReferenceImage:
				referenceImageCount++
			}
		case VideoContentTypeVideoURL:
			videoCount++
		case VideoContentTypeAudioURL:
			audioCount++
		}
	}

	if textCount > 1 {
		return badVideoContent("content may contain at most one text item")
	}
	if firstFrameCount > 1 || lastFrameCount > 1 || firstFrameCount+lastFrameCount > 2 {
		return badVideoContent("first/last frame image mode allows at most one first_frame and one last_frame")
	}
	if referenceImageCount > 9 {
		return badVideoContent("reference_image content is limited to 9 images")
	}
	if videoCount > 3 {
		return badVideoContent("reference_video content is limited to 3 videos")
	}
	if audioCount > 3 {
		return badVideoContent("reference_audio content is limited to 3 audio clips")
	}

	frameMode := firstFrameCount+lastFrameCount > 0
	referenceMode := referenceImageCount+videoCount+audioCount > 0
	if frameMode && referenceMode {
		return badVideoContent("image modes are mutually exclusive: first/last frame cannot mix with reference media")
	}
	if audioCount > 0 && referenceImageCount+videoCount == 0 {
		return badVideoContent("audio_url content cannot be used without at least one image_url or video_url reference")
	}

	derived := VideoTaskTypeTextToVideo
	if frameMode {
		derived = VideoTaskTypeImageToVideo
	} else if referenceMode {
		derived = VideoTaskTypeReferenceToVideo
	}
	if enforceModeMatch && strings.TrimSpace(taskType) != derived {
		return badVideoContent(fmt.Sprintf("task_type %q does not match content mode %q", strings.TrimSpace(taskType), derived))
	}
	return nil
}

func validateVideoGenerationContract(provider, model string, duration int, resolution string) error {
	normalizedResolution := strings.ToLower(strings.TrimSpace(resolution))
	switch normalizedResolution {
	case "", "480p", "720p", "1080p":
	default:
		return badVideoContent("resolution must be one of 480p, 720p, or 1080p")
	}
	if strings.TrimSpace(provider) == VideoProviderSeedance {
		if strings.Contains(strings.ToLower(model), "fast") && normalizedResolution == "1080p" {
			return badVideoContent("Seedance 2.0 fast does not support 1080p resolution")
		}
		if duration != -1 && (duration < 4 || duration > 15) {
			return badVideoContent("Seedance duration must be -1 or between 4 and 15 seconds")
		}
		return nil
	}
	if duration == -1 {
		return nil
	}
	if duration < 1 || duration > 60 {
		return badVideoContent("duration must be -1 or between 1 and 60 seconds")
	}
	return nil
}

func badVideoContent(message string) error {
	return infraerrors.BadRequest("VIDEO_INVALID_CONTENT", message)
}
