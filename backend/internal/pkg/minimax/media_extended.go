package minimax

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var voiceIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{6,254}[A-Za-z0-9]$`)

func parseVoice(body []byte, r *Request, kind string) (*Request, error) {
	if r.Stream {
		return nil, errors.New("voice previews do not support streaming")
	}
	if kind == "voice_clone" {
		if !voiceIDPattern.MatchString(r.VoiceID) || !validFileID(string(r.FileID)) {
			return nil, errors.New("voice_clone requires an uploaded file_id and an 8-256 character voice_id")
		}
		if utf8.RuneCountInString(r.Text) > 1000 || (r.Text != "" && !IsSpeech(r.Model)) {
			return nil, errors.New("clone preview requires a supported speech model and at most 1000 characters")
		}
		if r.ClonePrompt != nil && (!validFileID(string(r.ClonePrompt.PromptAudio)) || strings.TrimSpace(r.ClonePrompt.PromptText) == "") {
			return nil, errors.New("clone_prompt requires uploaded prompt_audio and prompt_text")
		}
		// A no-preview clone still freezes a valid pricing snapshot at zero usage.
		if r.Text == "" {
			r.Model = "voice_clone"
		}
	} else {
		if strings.TrimSpace(r.PromptText) == "" || strings.TrimSpace(r.PreviewText) == "" || utf8.RuneCountInString(r.PreviewText) > 500 {
			return nil, errors.New("voice_design requires prompt and preview_text (1-500 characters)")
		}
		r.Model = "voice_design"
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(body, &fields)
	for k := range fields {
		allowed := k == "voice_id" || k == "aigc_watermark"
		if kind == "voice_clone" {
			allowed = allowed || k == "file_id" || k == "clone_prompt" || k == "text" || k == "model" || k == "language_boost" || k == "need_noise_reduction" || k == "need_volume_normalization"
		}
		if kind == "voice_design" {
			allowed = allowed || k == "prompt" || k == "preview_text"
		}
		if !allowed {
			return nil, errors.New("unsupported voice request field: " + k)
		}
	}
	return r, nil
}
func validFileID(s string) bool { n, e := strconv.ParseInt(s, 10, 64); return e == nil && n > 0 }
func parseImage(body []byte, r *Request) (*Request, error) {
	if r.Model != Image || strings.TrimSpace(r.PromptText) == "" || utf8.RuneCountInString(r.PromptText) > 1500 || r.Stream {
		return nil, errors.New("image-01 requires a prompt of 1-1500 characters")
	}
	if r.N == 0 {
		var f map[string]json.RawMessage
		_ = json.Unmarshal(body, &f)
		if _, ok := f["n"]; ok {
			return nil, errors.New("n must be 1-9")
		}
		r.N = 1
	}
	if r.N < 1 || r.N > 9 {
		return nil, errors.New("n must be 1-9")
	}
	return r, nil
}
func ImageUsage(body []byte) (Usage, error) {
	var v struct {
		Base     *BaseResponse `json:"base_resp"`
		Metadata struct {
			Success json.RawMessage `json:"success_count"`
		} `json:"metadata"`
	}
	if json.Unmarshal(body, &v) != nil || v.Base == nil || v.Base.StatusCode != 0 {
		return Usage{}, errors.New("missing image success receipt")
	}
	raw := strings.Trim(string(v.Metadata.Success), `"`)
	n, e := strconv.Atoi(raw)
	if e != nil || n < 0 || n > 9 {
		return Usage{}, errors.New("invalid image success_count")
	}
	return Usage{OutputImages: n}, nil
}

// Voice preview responses omit the speech data.status field. Only authoritative
// usage_characters can settle a nonempty preview; text length is never a meter.
func VoicePreviewUsage(body []byte, hasPreview bool) (Usage, error) {
	var v struct {
		Base  *BaseResponse `json:"base_resp"`
		Extra *struct {
			Characters *int64 `json:"usage_characters"`
			VoiceCount int    `json:"usage_voice_count"`
		} `json:"extra_info"`
	}
	if json.Unmarshal(body, &v) != nil || v.Base == nil || v.Base.StatusCode != 0 {
		return Usage{}, errors.New("voice operation was not successful")
	}
	if v.Extra != nil && (v.Extra.VoiceCount != 0 || (v.Extra.Characters != nil && *v.Extra.Characters < 0)) {
		return Usage{}, errors.New("unexpected voice preview usage")
	}
	if !hasPreview {
		if v.Extra != nil && v.Extra.Characters != nil && *v.Extra.Characters != 0 {
			return Usage{}, errors.New("unexpected billed preview")
		}
		return Usage{}, nil
	}
	if v.Extra == nil || v.Extra.Characters == nil || *v.Extra.Characters <= 0 || v.Extra.VoiceCount != 0 {
		return Usage{}, errors.New("voice preview meter missing; reconciliation required")
	}
	return Usage{Characters: *v.Extra.Characters}, nil
}

// VoiceDesignUsage distinguishes provider meters from the input-based preview
// price used when a successful design response omits extra_info entirely.
func VoiceDesignUsage(body []byte, previewText string) (Usage, string, error) {
	u, err := VoicePreviewUsage(body, true)
	if err == nil {
		return u, "upstream_usage", nil
	}
	var v struct {
		Base       *BaseResponse   `json:"base_resp"`
		TrialAudio string          `json:"trial_audio"`
		Extra      json.RawMessage `json:"extra_info"`
	}
	n := utf8.RuneCountInString(previewText)
	if json.Unmarshal(body, &v) == nil && v.Base != nil && v.Base.StatusCode == 0 && v.TrialAudio != "" && len(v.Extra) == 0 && n > 0 && n <= 500 {
		return Usage{Characters: int64(n)}, "request_characters", nil
	}
	return Usage{}, "", err
}
