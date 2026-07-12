package fal

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// sizeEnumDims maps fal named size enums to their pixel dimensions.
var sizeEnumDims = map[string]ImageSizeDims{
	SizeSquareHD:     {Width: 1024, Height: 1024},
	SizeSquare:       {Width: 512, Height: 512},
	SizePortrait43:   {Width: 768, Height: 1024},
	SizePortrait169:  {Width: 576, Height: 1024},
	SizeLandscape43:  {Width: 1024, Height: 768},
	SizeLandscape169: {Width: 1024, Height: 576},
}

// dimsToSizeEnum is the reverse of sizeEnumDims for exact-match lookups.
var dimsToSizeEnum = func() map[ImageSizeDims]string {
	m := make(map[ImageSizeDims]string, len(sizeEnumDims))
	for enum, dims := range sizeEnumDims {
		m[dims] = enum
	}
	return m
}()

// MapSizeToFal converts an OpenAI-style size string into a fal image_size value.
//
// Rules:
//   - "" or "auto"            -> "auto"
//   - a known enum name       -> the enum string as-is
//   - "WxH" matching an enum  -> the matching enum string
//   - "WxH" otherwise         -> explicit ImageSizeDims{Width,Height}
func MapSizeToFal(size string) any {
	size = strings.TrimSpace(strings.ToLower(size))
	if size == "" || size == SizeAuto {
		return SizeAuto
	}
	// Already a named enum.
	if _, ok := sizeEnumDims[size]; ok {
		return size
	}
	if w, h, ok := parseWxH(size); ok {
		dims := ImageSizeDims{Width: w, Height: h}
		if enum, matched := dimsToSizeEnum[dims]; matched {
			return enum
		}
		return dims
	}
	// Unknown token: pass through so fal can validate / reject explicitly.
	return size
}

// MapSizeFromFal converts a fal image_size value back into an OpenAI-style
// "WxH" string. Returns "" when the size cannot be expressed (e.g. "auto").
func MapSizeFromFal(size any) string {
	switch v := size.(type) {
	case nil:
		return ""
	case string:
		s := strings.TrimSpace(strings.ToLower(v))
		if s == "" || s == SizeAuto {
			return ""
		}
		if dims, ok := sizeEnumDims[s]; ok {
			return fmt.Sprintf("%dx%d", dims.Width, dims.Height)
		}
		if w, h, ok := parseWxH(s); ok {
			return fmt.Sprintf("%dx%d", w, h)
		}
		return ""
	case ImageSizeDims:
		if v.Width > 0 && v.Height > 0 {
			return fmt.Sprintf("%dx%d", v.Width, v.Height)
		}
		return ""
	case map[string]any:
		w := toInt(v["width"])
		h := toInt(v["height"])
		if w > 0 && h > 0 {
			return fmt.Sprintf("%dx%d", w, h)
		}
		return ""
	default:
		return ""
	}
}

// MapQualityToFal converts an OpenAI-style quality value into a fal quality value.
//
//	hd       -> high
//	standard -> medium
//	low/medium/high/auto -> identity
//	""       -> "" (let fal use its default)
func MapQualityToFal(quality string) string {
	switch strings.TrimSpace(strings.ToLower(quality)) {
	case "":
		return ""
	case "hd":
		return QualityHigh
	case "standard":
		return QualityMedium
	case QualityLow:
		return QualityLow
	case QualityMedium:
		return QualityMedium
	case QualityHigh:
		return QualityHigh
	case QualityAuto:
		return QualityAuto
	default:
		// Unknown: pass through lowercased.
		return strings.TrimSpace(strings.ToLower(quality))
	}
}

// MapQualityFromFal converts a fal quality value back into an OpenAI-style
// quality value. fal's low/medium/high/auto are valid gpt-image qualities, so
// the mapping is effectively identity (normalized to lower-case).
func MapQualityFromFal(quality string) string {
	q := strings.TrimSpace(strings.ToLower(quality))
	switch q {
	case QualityLow, QualityMedium, QualityHigh, QualityAuto:
		return q
	case "":
		return ""
	default:
		return q
	}
}

// BuildRequest constructs a fal Request from a protocol-neutral input.
func BuildRequest(in ImageGenInput) *Request {
	r := &Request{
		Prompt:       strings.TrimSpace(in.Prompt),
		ImageSize:    MapSizeToFal(in.Size),
		Quality:      MapQualityToFal(in.Quality),
		OutputFormat: strings.TrimSpace(in.OutputFormat),
		SyncMode:     in.SyncMode,
	}
	if in.N > 0 {
		r.NumImages = in.N
	}
	if in.IsEdit {
		r.ImageURLs = trimNonEmpty(in.ImageURLs)
		if u := strings.TrimSpace(in.MaskURL); u != "" {
			r.MaskURL = u
		}
		// edits default image_size is "auto" per fal docs; honor when unset.
		if s, ok := r.ImageSize.(string); ok && s == "" {
			r.ImageSize = SizeAuto
		}
	}
	return r
}

// ToOpenAIResponse converts a fal result into an OpenAI Images response.
// When created <= 0 the current unix time is used.
func ToOpenAIResponse(resp *Response, created int64) *OpenAIImagesResponse {
	if created <= 0 {
		created = time.Now().Unix()
	}
	out := &OpenAIImagesResponse{Created: created, Data: []OpenAIImageData{}}
	if resp == nil {
		return out
	}
	for _, img := range resp.Images {
		url := strings.TrimSpace(img.URL)
		if url == "" {
			continue
		}
		out.Data = append(out.Data, OpenAIImageData{URL: url})
	}
	return out
}

// FalRequestToInput parses a fal Request into the protocol-neutral input,
// used when a fal-native facade request must be served by an OpenAI upstream.
func FalRequestToInput(r *Request) ImageGenInput {
	in := ImageGenInput{
		Prompt:       strings.TrimSpace(r.Prompt),
		Size:         MapSizeFromFal(r.ImageSize),
		Quality:      MapQualityFromFal(r.Quality),
		N:            r.NumImages,
		OutputFormat: strings.TrimSpace(r.OutputFormat),
		ImageURLs:    trimNonEmpty(r.ImageURLs),
		MaskURL:      strings.TrimSpace(r.MaskURL),
		SyncMode:     r.SyncMode,
	}
	if len(in.ImageURLs) > 0 || in.MaskURL != "" {
		in.IsEdit = true
	}
	return in
}

// OpenAIResponseToFal converts an OpenAI Images response into a fal Response,
// used when a fal-native facade returns the output of an OpenAI upstream.
func OpenAIResponseToFal(resp *OpenAIImagesResponse) *Response {
	out := &Response{Images: []Image{}}
	if resp == nil {
		return out
	}
	for _, d := range resp.Data {
		url := strings.TrimSpace(d.URL)
		if url == "" {
			continue
		}
		out.Images = append(out.Images, Image{URL: url})
	}
	return out
}

// ----- helpers -----

func parseWxH(s string) (int, int, bool) {
	parts := strings.SplitN(strings.TrimSpace(strings.ToLower(s)), "x", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case float32:
		return int(n)
	default:
		return 0
	}
}

func trimNonEmpty(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
