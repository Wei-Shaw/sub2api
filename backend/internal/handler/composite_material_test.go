package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateCompositeEditURLsAllowsHTTPURLs(t *testing.T) {
	err := validateCompositeEditURLs(map[string]any{
		"image_urls": []any{"https://example.test/reference.png"},
		"mask_url":   "https://example.test/mask.png",
	})
	require.NoError(t, err)

	err = validateCompositeEditURLs(map[string]any{
		"image_urls": []any{"HTTP://example.test/reference.png"},
	})
	require.NoError(t, err)
}

func TestValidateCompositeEditURLsIdentifiesInvalidParameter(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
		message string
	}{
		{
			name:    "image urls is not an array",
			payload: map[string]any{"image_urls": "https://example.test/reference.png"},
			message: "invalid parameter 'image_urls': must be an array",
		},
		{
			name:    "image url is not a string",
			payload: map[string]any{"image_urls": []any{123}},
			message: "invalid parameter 'image_urls[0]': must be a string containing an HTTP or HTTPS URL",
		},
		{
			name:    "image url is a data URL",
			payload: map[string]any{"image_urls": []any{"https://example.test/reference.png", "data:image/png;base64,aW1hZ2U="}},
			message: "invalid parameter 'image_urls[1]': must be a valid HTTP or HTTPS URL",
		},
		{
			name:    "image url is relative",
			payload: map[string]any{"image_urls": []any{"/reference.png"}},
			message: "invalid parameter 'image_urls[0]': must be a valid HTTP or HTTPS URL",
		},
		{
			name:    "mask is a data URL",
			payload: map[string]any{"mask_url": "data:image/png;base64,aW1hZ2U="},
			message: "invalid parameter 'mask_url': must be a valid HTTP or HTTPS URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCompositeEditURLs(tt.payload)
			require.EqualError(t, err, tt.message)
		})
	}
}
