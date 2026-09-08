package backend

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestImageModelCapability(t *testing.T) {
	tests := []struct {
		name           string
		model          string
		wantReferences int
		wantOutputs    int
	}{
		{name: "gpt image 2", model: "gpt-image-2", wantReferences: 16, wantOutputs: 10},
		{name: "gpt image 1", model: "gpt-image-1", wantReferences: 16, wantOutputs: 10},
		{name: "gemini flash image", model: "gemini-2.5-flash-image", wantReferences: 10, wantOutputs: 4},
		{name: "unknown model", model: "custom-image-model", wantReferences: 1, wantOutputs: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capability := imageModelCapability(tt.model)
			if capability.MaxReferenceImages != tt.wantReferences || capability.MaxOutputImages != tt.wantOutputs {
				t.Fatalf("capability = %#v, want references=%d outputs=%d", capability, tt.wantReferences, tt.wantOutputs)
			}
		})
	}
}

func TestImageModelCapabilityIncludesParameterDescriptors(t *testing.T) {
	capability, ok := configuredImageModelCapability("gemini-2.5-flash-image")
	if !ok {
		t.Fatal("gemini capability is missing")
	}
	wantRatios := []string{"1:1", "2:3", "3:2", "3:4", "4:3", "4:5", "5:4", "9:16", "16:9", "21:9"}
	if capability.AspectRatios == nil || !reflect.DeepEqual(capability.AspectRatios.Values, wantRatios) {
		t.Fatalf("aspect ratios = %#v, want %#v", capability.AspectRatios, wantRatios)
	}
	if capability.Resolutions == nil || capability.Resolutions.Default != "1K" {
		t.Fatalf("resolutions = %#v, want default 1K", capability.Resolutions)
	}
}

func TestUnknownImageModelHasNoAdvancedDescriptors(t *testing.T) {
	capability := imageModelCapability("custom-image-model")
	if capability.Quality != nil || capability.AspectRatios != nil || capability.Resolutions != nil {
		t.Fatalf("unknown capability exposes advanced parameters: %#v", capability)
	}
	if capability.MaxReferenceImages != 1 || capability.MaxOutputImages != 1 {
		t.Fatalf("unknown capability limits = %#v", capability)
	}
}

func TestNormalizeImageParametersAppliesAdvertisedDefaults(t *testing.T) {
	req := GenerateRequest{Model: "gpt-image-1"}
	if err := normalizeImageParameters(&req); err != nil {
		t.Fatal(err)
	}
	if req.Size != "1024x1024" || req.Quality != "auto" || req.OutputFormat != "png" || req.Background != "auto" {
		t.Fatalf("normalized request = %#v", req)
	}
}

func TestNormalizeImageParametersSupportsGeminiRatiosAndResolution(t *testing.T) {
	req := GenerateRequest{Model: "gemini-2.5-flash-image", AspectRatio: "21:9", Resolution: "4K"}
	if err := normalizeImageParameters(&req); err != nil {
		t.Fatal(err)
	}
	if req.AspectRatio != "21:9" || req.Resolution != "4K" || req.Size != "" {
		t.Fatalf("normalized request = %#v", req)
	}
}

func TestNormalizeImageParametersRejectsUnsupportedValue(t *testing.T) {
	req := GenerateRequest{Model: "gpt-image-1", Quality: "ultra"}
	err := normalizeImageParameters(&req)
	if !errors.Is(err, ErrInvalidImageParameter) {
		t.Fatalf("error = %v, want ErrInvalidImageParameter", err)
	}
	if !strings.Contains(err.Error(), "gpt-image-1") || !strings.Contains(err.Error(), "quality") || !strings.Contains(err.Error(), "ultra") {
		t.Fatalf("error lacks parameter context: %v", err)
	}
}

func TestNormalizeImageParametersRejectsAdvancedFieldsForUnknownModel(t *testing.T) {
	req := GenerateRequest{Model: "custom-image-model", Quality: "high"}
	if err := normalizeImageParameters(&req); !errors.Is(err, ErrInvalidImageParameter) {
		t.Fatalf("error = %v, want ErrInvalidImageParameter", err)
	}
}

func TestNormalizeImageParametersValidatesCompression(t *testing.T) {
	outOfRange := 101
	req := GenerateRequest{Model: "gpt-image-1", OutputFormat: "webp", OutputCompression: &outOfRange}
	if err := normalizeImageParameters(&req); !errors.Is(err, ErrInvalidImageParameter) {
		t.Fatalf("out-of-range error = %v, want ErrInvalidImageParameter", err)
	}

	valid := 82
	req = GenerateRequest{Model: "gpt-image-1", OutputFormat: "png", OutputCompression: &valid}
	if err := normalizeImageParameters(&req); !errors.Is(err, ErrInvalidImageParameter) {
		t.Fatalf("png compression error = %v, want ErrInvalidImageParameter", err)
	}
}

func TestValidateOutputCount(t *testing.T) {
	if got, err := validateOutputCount("gpt-image-1", 0); err != nil || got != 1 {
		t.Fatalf("default output count = %d, err = %v", got, err)
	}
	if _, err := validateOutputCount("gemini-2.5-flash-image", 5); !errors.Is(err, ErrInvalidOutputCount) {
		t.Fatalf("error = %v, want ErrInvalidOutputCount", err)
	}
}

func TestValidateReferenceImageCountRejectsOverLimit(t *testing.T) {
	err := validateReferenceImageCount("custom-image-model", make([]ReferenceImage, 2))
	if !errors.Is(err, ErrTooManyReferenceImages) {
		t.Fatalf("error = %v, want ErrTooManyReferenceImages", err)
	}
}
