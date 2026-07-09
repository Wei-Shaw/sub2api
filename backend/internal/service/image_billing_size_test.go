package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyImageBillingTier(t *testing.T) {
	tests := []struct {
		name     string
		size     string
		wantTier string
		wantOK   bool
	}{
		{name: "exact 1M area is 1k", size: "1024x1024", wantTier: "1K", wantOK: true},
		{name: "official standard portrait is 1k", size: "1152x1536", wantTier: "1K", wantOK: true},
		{name: "user standard example is 1k", size: "1088x1440", wantTier: "1K", wantOK: true},
		{name: "1.5k midpoint boundary is 2k", size: "1536x1536", wantTier: "2K", wantOK: true},
		{name: "explicit 2k square", size: "2048x2048", wantTier: "2K", wantOK: true},
		{name: "explicit 2k landscape", size: "2048x1152", wantTier: "2K", wantOK: true},
		{name: "uhd is 4k", size: "3840x2160", wantTier: "4K", wantOK: true},
		{name: "uhd portrait is 4k", size: "2160x3840", wantTier: "4K", wantOK: true},
		{name: "2.5k midpoint boundary is 4k", size: "2560x2560", wantTier: "4K", wantOK: true},
		{name: "above 2.5k midpoint is 4k", size: "2560x2561", wantTier: "4K", wantOK: true},
		{name: "long edge 1k", size: "1024X768", wantTier: "1K", wantOK: true},
		{name: "area below 1M is 1k", size: "1280x768", wantTier: "1K", wantOK: true},
		{name: "area below 4M is 2k", size: "2560x1600", wantTier: "2K", wantOK: true},
		{name: "tier string 1k", size: "1k", wantTier: "1K", wantOK: true},
		{name: "empty", size: "", wantOK: false},
		{name: "auto", size: "auto", wantOK: false},
		{name: "invalid", size: "not-a-size", wantOK: false},
		{name: "1080x1920 is 1k", size: "1080x1920", wantTier: "1K", wantOK: true},
		{name: "8192x8192 is 4k", size: "8192x8192", wantTier: "4K", wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTier, gotOK := ClassifyImageBillingTier(tt.size)
			require.Equal(t, tt.wantOK, gotOK)
			require.Equal(t, tt.wantTier, gotTier)
		})
	}
}

func TestComputePixelArea(t *testing.T) {
	tests := []struct {
		name string
		size string
		want int
	}{
		{name: "1024x1024", size: "1024x1024", want: 1048576},
		{name: "1920x1080", size: "1920x1080", want: 2073600},
		{name: "auto", size: "auto", want: 0},
		{name: "empty", size: "", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputePixelArea(tt.size)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestMaxPixelArea(t *testing.T) {
	area := MaxPixelArea("1024x1024", "3840x2160", "auto")
	require.Equal(t, 8294400, area)

	area = MaxPixelArea("auto")
	require.Equal(t, 0, area)
}

func TestResolveImageBillingSize(t *testing.T) {
	tests := []struct {
		name          string
		inputSize     string
		outputSizes   []string
		wantBilling   string
		wantOutput    string
		wantSource    string
		wantBreakdown map[string]int
	}{
		{
			name:          "output wins over input",
			inputSize:     "1024x1024",
			outputSizes:   []string{"3840x2160"},
			wantBilling:   "4K",
			wantOutput:    "3840x2160",
			wantSource:    ImageSizeSourceOutput,
			wantBreakdown: map[string]int{"4K": 1},
		},
		{
			name:        "input fallback",
			inputSize:   "1024x1024",
			wantBilling: "1K",
			wantSource:  ImageSizeSourceInput,
		},
		{
			name:        "auto defaults",
			inputSize:   "auto",
			wantBilling: "2K",
			wantSource:  ImageSizeSourceDefault,
		},
		{
			name:        "empty defaults",
			inputSize:   "",
			wantBilling: "2K",
			wantSource:  ImageSizeSourceDefault,
		},
		{
			name:        "invalid defaults",
			inputSize:   "largest",
			wantBilling: "2K",
			wantSource:  ImageSizeSourceDefault,
		},
		{
			name:          "mixed output chooses highest tier",
			inputSize:     "1024x1024",
			outputSizes:   []string{"1024x1024", "3840x2160", "1280x720"},
			wantBilling:   "4K",
			wantOutput:    "1024x1024",
			wantSource:    ImageSizeSourceOutput,
			wantBreakdown: map[string]int{"1K": 2, "4K": 1},
		},
		{
			name:        "unparseable output falls back to parseable input",
			inputSize:   "2048x1152",
			outputSizes: []string{"auto"},
			wantBilling: "2K",
			wantOutput:  "auto",
			wantSource:  ImageSizeSourceInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveImageBillingSize(tt.inputSize, tt.outputSizes)
			require.Equal(t, tt.wantBilling, got.BillingSize)
			require.Equal(t, tt.inputSize, got.InputSize)
			require.Equal(t, tt.wantOutput, got.OutputSize)
			require.Equal(t, tt.wantSource, got.Source)
			require.Equal(t, tt.wantBreakdown, got.Breakdown)
		})
	}
}
