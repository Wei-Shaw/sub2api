package fal

import (
	"reflect"
	"testing"
)

func TestMapSizeToFal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want any
	}{
		{"", SizeAuto},
		{"auto", SizeAuto},
		{"AUTO", SizeAuto},
		{"1024x1024", SizeSquareHD},
		{"1024x768", SizeLandscape43},
		{"768x1024", SizePortrait43},
		{"square_hd", SizeSquareHD},
		{"1024x1536", ImageSizeDims{Width: 1024, Height: 1536}},
	}
	for _, c := range cases {
		got := MapSizeToFal(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Fatalf("MapSizeToFal(%q)=%#v want %#v", c.in, got, c.want)
		}
	}
}

func TestMapSizeFromFal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   any
		want string
	}{
		{SizeAuto, ""},
		{"", ""},
		{SizeSquareHD, "1024x1024"},
		{SizeLandscape43, "1024x768"},
		{ImageSizeDims{Width: 1024, Height: 1536}, "1024x1536"},
		{map[string]any{"width": float64(800), "height": float64(600)}, "800x600"},
		{nil, ""},
	}
	for _, c := range cases {
		if got := MapSizeFromFal(c.in); got != c.want {
			t.Fatalf("MapSizeFromFal(%#v)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestMapQualityToFal(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":         "",
		"hd":       QualityHigh,
		"HD":       QualityHigh,
		"standard": QualityMedium,
		"low":      QualityLow,
		"medium":   QualityMedium,
		"high":     QualityHigh,
		"auto":     QualityAuto,
	}
	for in, want := range cases {
		if got := MapQualityToFal(in); got != want {
			t.Fatalf("MapQualityToFal(%q)=%q want %q", in, got, want)
		}
	}
}

func TestMapQualityFromFal(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"high":   "high",
		"low":    "low",
		"medium": "medium",
		"auto":   "auto",
		"":       "",
	}
	for in, want := range cases {
		if got := MapQualityFromFal(in); got != want {
			t.Fatalf("MapQualityFromFal(%q)=%q want %q", in, got, want)
		}
	}
}

func TestBuildRequest_Generations(t *testing.T) {
	t.Parallel()
	in := ImageGenInput{
		Prompt:  "a cat",
		Size:    "1024x1024",
		Quality: "hd",
		N:       2,
	}
	r := BuildRequest(in)
	if r.Prompt != "a cat" {
		t.Fatalf("prompt mismatch: %q", r.Prompt)
	}
	if r.ImageSize != SizeSquareHD {
		t.Fatalf("image_size mismatch: %#v", r.ImageSize)
	}
	if r.Quality != QualityHigh {
		t.Fatalf("quality mismatch: %q", r.Quality)
	}
	if r.NumImages != 2 {
		t.Fatalf("num_images mismatch: %d", r.NumImages)
	}
	if len(r.ImageURLs) != 0 {
		t.Fatalf("generations should not set image_urls")
	}
}

func TestBuildRequest_Edits(t *testing.T) {
	t.Parallel()
	in := ImageGenInput{
		Prompt:    "make it night",
		IsEdit:    true,
		ImageURLs: []string{"https://x/a.png", " "},
		MaskURL:   "https://x/mask.png",
	}
	r := BuildRequest(in)
	if !reflect.DeepEqual(r.ImageURLs, []string{"https://x/a.png"}) {
		t.Fatalf("image_urls mismatch: %#v", r.ImageURLs)
	}
	if r.MaskURL != "https://x/mask.png" {
		t.Fatalf("mask_url mismatch: %q", r.MaskURL)
	}
}

func TestToOpenAIResponse(t *testing.T) {
	t.Parallel()
	resp := &Response{Images: []Image{{URL: "https://x/1.png"}, {URL: " "}, {URL: "https://x/2.png"}}}
	out := ToOpenAIResponse(resp, 100)
	if out.Created != 100 {
		t.Fatalf("created mismatch: %d", out.Created)
	}
	if len(out.Data) != 2 {
		t.Fatalf("expected 2 data entries, got %d", len(out.Data))
	}
	if out.Data[0].URL != "https://x/1.png" || out.Data[1].URL != "https://x/2.png" {
		t.Fatalf("data url mismatch: %#v", out.Data)
	}
}

// TestRoundTrip_OpenAIToFalToOpenAI exercises the four upstream combinations'
// conversion primitives: an OpenAI input -> fal request -> (fal result) ->
// OpenAI response, and the reverse fal-request parsing path.
func TestRoundTrip_FalRequestToInput(t *testing.T) {
	t.Parallel()
	r := &Request{
		Prompt:    "edit me",
		ImageURLs: []string{"https://x/in.png"},
		MaskURL:   "https://x/m.png",
		ImageSize: SizeLandscape43,
		Quality:   QualityHigh,
		NumImages: 3,
	}
	in := FalRequestToInput(r)
	if !in.IsEdit {
		t.Fatalf("expected IsEdit true")
	}
	if in.Size != "1024x768" {
		t.Fatalf("size mismatch: %q", in.Size)
	}
	if in.Quality != "high" {
		t.Fatalf("quality mismatch: %q", in.Quality)
	}
	if in.N != 3 {
		t.Fatalf("n mismatch: %d", in.N)
	}
}

func TestOpenAIResponseToFal(t *testing.T) {
	t.Parallel()
	resp := &OpenAIImagesResponse{Data: []OpenAIImageData{{URL: "https://x/1.png"}, {B64JSON: "abc"}}}
	out := OpenAIResponseToFal(resp)
	if len(out.Images) != 1 {
		t.Fatalf("expected 1 image (url only), got %d", len(out.Images))
	}
	if out.Images[0].URL != "https://x/1.png" {
		t.Fatalf("url mismatch: %q", out.Images[0].URL)
	}
}
