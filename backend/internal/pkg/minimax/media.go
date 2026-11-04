// Package minimax implements the native, metered MiniMax media protocols.
package minimax

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

const SpeechHD = "speech-2.8-hd"
const SpeechTurbo = "speech-2.8-turbo"
const H3 = "MiniMax-H3"
const H3Highspeed = "MiniMax-H3-Highspeed"
const H3Lite = "MiniMax-H3-Lite"
const H3Max = "MiniMax-H3-Max"
const Image = "image-01"

func IsSpeech(m string) bool {
	switch m {
	case SpeechHD, SpeechTurbo, "speech-2.6-hd", "speech-2.6-turbo", "speech-02-hd", "speech-02-turbo":
		return true
	}
	return false
}
func IsHDSpeech(m string) bool { return IsSpeech(m) && strings.HasSuffix(m, "-hd") }
func IsVideo(m string) bool    { return m == H3 || m == H3Highspeed || m == H3Lite || m == H3Max }
func ValidResolution(m, r string) bool {
	switch m {
	case H3:
		return r == "768P" || r == "2K"
	case H3Highspeed:
		return r == "768P"
	case H3Lite, H3Max:
		return r == "480P" || r == "768P"
	}
	return false
}

type Request struct {
	PromptText  string      `json:"prompt"`
	N           int         `json:"n"`
	FileID      json.Number `json:"file_id"`
	VoiceID     string      `json:"voice_id"`
	PreviewText string      `json:"preview_text"`
	ClonePrompt *struct {
		PromptAudio json.Number `json:"prompt_audio"`
		PromptText  string      `json:"prompt_text"`
	} `json:"clone_prompt"`

	Model        string `json:"model"`
	Text         string `json:"text"`
	Stream       bool   `json:"stream"`
	Resolution   string `json:"resolution"`
	Duration     int    `json:"duration"`
	Ratio        string `json:"ratio"`
	CallbackURL  string `json:"callback_url"`
	VoiceSetting struct {
		VoiceID           string `json:"voice_id"`
		TextNormalization bool   `json:"text_normalization"`
	} `json:"voice_setting"`
	PronunciationDictionary json.RawMessage `json:"pronunciation_dict"`
	TextNormalizationRules  json.RawMessage `json:"text_normalization_rules"`
	TextNormalizationMode   string          `json:"text_normalization_mode"`
	TimbreWeights           []struct {
		VoiceID string `json:"voice_id"`
	} `json:"timbre_weights"`
	Content []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		Role     string `json:"role"`
		ImageURL *Asset `json:"image_url"`
		VideoURL *Asset `json:"video_url"`
		AudioURL *Asset `json:"audio_url"`
	} `json:"content"`
}
type Asset struct {
	URL string `json:"url"`
}
type Usage struct {
	Characters        int64   `json:"usage_characters"`
	VoiceCount        int     `json:"usage_voice_count"`
	OutputImages      int     `json:"output_image_count"`
	OutputSeconds     float64 `json:"output_seconds"`
	InputSeconds      float64 `json:"input_seconds"`
	InputImages       int     `json:"input_image_count"`
	InputAudioSeconds float64 `json:"input_audio_seconds"`
}
type BaseResponse struct {
	StatusCode int    `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}
type SpeechResponse struct {
	Base *BaseResponse `json:"base_resp"`
	Data struct {
		Status int `json:"status"`
	} `json:"data"`
	Extra *struct {
		Characters *int64 `json:"usage_characters"`
		VoiceCount int    `json:"usage_voice_count"`
	} `json:"extra_info"`
}
type VideoTask struct {
	ID         string `json:"id"`
	Model      string `json:"model"`
	Status     string `json:"status"`
	Resolution string `json:"resolution"`
	Duration   int    `json:"duration"`
	TaskType   string `json:"task_type"`
	Content    struct {
		URL string `json:"url"`
	} `json:"content"`
	Usage *struct {
		OutputSeconds     *float64 `json:"output_seconds"`
		InputSeconds      *float64 `json:"input_seconds"`
		InputImages       *int     `json:"input_image_count"`
		InputAudioSeconds float64  `json:"input_audio_seconds"`
	} `json:"usage"`
}

// Decode rejects duplicate/case-variant keys recursively. A request must have
// one interpretation for admission, audit, reservation and upstream execution.
func Decode(body []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(body))
	var visit func() error
	visit = func() error {
		t, err := d.Token()
		if err != nil {
			return err
		}
		if delim, ok := t.(json.Delim); ok {
			switch delim {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					key, err := d.Token()
					if err != nil {
						return err
					}
					k, ok := key.(string)
					if !ok {
						return errors.New("invalid object key")
					}
					lower := strings.ToLower(k)
					if lower != k || seen[k] {
						return fmt.Errorf("ambiguous JSON key %q", k)
					}
					seen[k] = true
					if err := visit(); err != nil {
						return err
					}
				}
			case '[':
				for d.More() {
					if err := visit(); err != nil {
						return err
					}
				}
			default:
				return errors.New("invalid JSON delimiter")
			}
			_, err = d.Token()
			return err
		}
		return nil
	}
	if err := visit(); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("expected one JSON object")
	}
	return json.Unmarshal(body, target)
}

func ParseRequest(body []byte, kind string) (*Request, error) {
	var r Request
	if err := Decode(body, &r); err != nil {
		return nil, err
	}
	if kind == "image" {
		return parseImage(body, &r)
	}
	if kind == "voice_clone" || kind == "voice_design" {
		return parseVoice(body, &r, kind)
	}
	if kind == "speech" {
		if !IsSpeech(r.Model) {
			return nil, errors.New("unsupported MiniMax speech model")
		}
		n := utf8.RuneCountInString(r.Text)
		if strings.TrimSpace(r.Text) == "" || n >= 10000 {
			return nil, errors.New("text must contain 1 to 9999 characters")
		}
		if len(r.Voices()) == 0 {
			return nil, errors.New("a system voice is required")
		}
		return &r, nil
	}
	if kind != "video" || !ValidResolution(r.Model, r.Resolution) || r.Duration < 4 || r.Duration > 15 || (r.Model == H3Max && r.Duration < 5) {
		return nil, errors.New("unsupported MiniMax video model, resolution or duration")
	}
	// Limit this endpoint to the documented standard generation request. Future
	// options or other task types need their own admission and billing review.
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(body, &fields)
	for key := range fields {
		switch key {
		case "model", "content", "resolution", "duration", "ratio", "aigc_watermark":
		default:
			return nil, fmt.Errorf("unsupported standard H3 field %q", key)
		}
	}
	if r.CallbackURL != "" {
		return nil, errors.New("callback_url is not supported; query the local task instead")
	}
	texts, images, videos, audios, first, last, refs := 0, 0, 0, 0, 0, 0, false
	for _, c := range r.Content {
		var a *Asset
		assets := 0
		for _, asset := range []*Asset{c.ImageURL, c.VideoURL, c.AudioURL} {
			if asset != nil {
				assets++
			}
		}
		if (c.Type == "text" && assets != 0) || (c.Type != "text" && (assets != 1 || c.Text != "")) {
			return nil, errors.New("each content item must contain only its declared input type")
		}
		switch c.Type {
		case "text":
			texts++
			if c.Role != "" || strings.TrimSpace(c.Text) == "" || utf8.RuneCountInString(c.Text) > 7000 {
				return nil, errors.New("one nonempty prompt of at most 7000 characters is required")
			}
		case "image_url":
			images++
			a = c.ImageURL
			switch c.Role {
			case "", "first_frame":
				first++
			case "last_frame":
				last++
			case "reference_image":
				refs = true
			default:
				return nil, errors.New("invalid image role")
			}
		case "video_url":
			videos++
			a = c.VideoURL
			refs = true
			if c.Role != "reference_video" {
				return nil, errors.New("video requires reference_video role")
			}
		case "audio_url":
			audios++
			a = c.AudioURL
			refs = true
			if c.Role != "reference_audio" {
				return nil, errors.New("audio requires reference_audio role")
			}
		default:
			return nil, errors.New("unsupported content type")
		}
		if c.Type != "text" && (a == nil || !publicAssetURL(a.URL)) {
			return nil, errors.New("media must use an absolute HTTPS URL")
		}
	}
	if texts != 1 || images > 9 || videos > 3 || audios > 3 || images+videos+audios > 12 || first > 1 || last > 1 || (refs && first+last > 0) {
		return nil, errors.New("invalid media count or mixed frame/reference modes")
	}
	if images+videos+audios == 0 && (r.Ratio == "" || r.Ratio == "adaptive") {
		return nil, errors.New("text-to-video requires an explicit aspect ratio")
	}
	if r.Ratio != "" {
		switch r.Ratio {
		case "adaptive", "16:9", "9:16", "1:1", "4:3", "3:4", "21:9":
		default:
			return nil, errors.New("unsupported ratio")
		}
	}
	return &r, nil
}
func publicAssetURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil
}
func (r *Request) Voices() []string {
	var v []string
	if r.VoiceSetting.VoiceID != "" {
		v = append(v, r.VoiceSetting.VoiceID)
	}
	for _, t := range r.TimbreWeights {
		v = append(v, t.VoiceID)
	}
	return v
}

// Prompt includes all text that can affect generation or voice previews. Media
// URLs and raw voice samples are deliberately excluded from textual auditing.
func (r *Request) Prompt() string {
	var parts []string
	for _, text := range []string{r.PromptText, r.PreviewText, r.Text} {
		if strings.TrimSpace(text) != "" {
			parts = append(parts, text)
		}
	}
	if r.ClonePrompt != nil && strings.TrimSpace(r.ClonePrompt.PromptText) != "" {
		parts = append(parts, r.ClonePrompt.PromptText)
	}
	for _, c := range r.Content {
		if c.Type == "text" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// Literal speech uses the documented Han=2, other=1 count. Explicit text
// transforms can expand it, so those requests reserve the full text bound.
func (r *Request) Estimate(kind string) Usage {
	if kind == "image" {
		return Usage{OutputImages: r.N}
	}
	if kind == "voice_design" {
		return Usage{Characters: int64(utf8.RuneCountInString(r.PreviewText))}
	}
	if kind == "speech" || kind == "voice_clone" {
		if r.VoiceSetting.TextNormalization || r.TextNormalizationMode != "" || len(r.PronunciationDictionary) > 0 || len(r.TextNormalizationRules) > 0 {
			return Usage{Characters: 20000}
		}
		var n int64
		for _, ch := range r.Text {
			n++
			if unicode.Is(unicode.Han, ch) {
				n++
			}
		}
		return Usage{Characters: n}
	}
	u := Usage{OutputSeconds: float64(r.Duration)}
	for _, c := range r.Content {
		if c.Type == "image_url" {
			u.InputImages++
		}
		if c.Type == "video_url" {
			u.InputSeconds = 15
		}
	}
	return u
}
func SpeechUsage(body []byte) (Usage, bool, error) {
	var r SpeechResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return Usage{}, false, err
	}
	if r.Base == nil {
		return Usage{}, false, errors.New("missing base_resp")
	}
	if r.Base.StatusCode != 0 {
		return Usage{}, false, errors.New("MiniMax speech business error")
	}
	if r.Data.Status != 2 {
		return Usage{}, false, nil
	}
	if r.Extra == nil || r.Extra.Characters == nil || *r.Extra.Characters <= 0 || r.Extra.VoiceCount < 0 {
		return Usage{}, false, errors.New("missing billable characters or unexpected custom voice charge")
	}
	return Usage{Characters: *r.Extra.Characters, VoiceCount: r.Extra.VoiceCount}, true, nil
}
func (t VideoTask) Meter(model, resolution string) (Usage, error) {
	if t.Model != model || t.Resolution != resolution || (t.TaskType != "" && t.TaskType != "generation") {
		return Usage{}, errors.New("upstream task model/resolution/type mismatch")
	}
	if t.Usage == nil || t.Usage.OutputSeconds == nil || t.Usage.InputSeconds == nil || t.Usage.InputImages == nil {
		return Usage{}, errors.New("incomplete upstream video receipt")
	}
	u := Usage{OutputSeconds: *t.Usage.OutputSeconds, InputSeconds: *t.Usage.InputSeconds, InputImages: *t.Usage.InputImages, InputAudioSeconds: t.Usage.InputAudioSeconds}
	if err := u.Validate(); err != nil {
		return Usage{}, err
	}
	if u.OutputSeconds <= 0 {
		return Usage{}, errors.New("missing output seconds")
	}
	return u, nil
}
func (u Usage) Validate() error {
	if u.Characters < 0 || u.VoiceCount < 0 || u.VoiceCount > 10 || u.OutputImages < 0 || u.OutputImages > 9 || u.InputImages < 0 {
		return errors.New("invalid media usage")
	}
	for _, v := range []float64{u.OutputSeconds, u.InputSeconds, u.InputAudioSeconds} {
		if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return errors.New("invalid media seconds")
		}
	}
	if u.OutputSeconds > 15 || u.InputSeconds > 15 || u.InputImages > 9 || u.InputAudioSeconds > 15 {
		return errors.New("media usage exceeds the enabled model limits")
	}
	return nil
}
