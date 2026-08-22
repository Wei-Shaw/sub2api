package handler

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

func validateCompositeEditURLs(payload map[string]any) error {
	if raw, ok := payload["image_urls"]; ok {
		items, ok := raw.([]any)
		if !ok {
			return errors.New("invalid parameter 'image_urls': must be an array")
		}
		for idx, item := range items {
			value, ok := item.(string)
			if !ok {
				return fmt.Errorf("invalid parameter 'image_urls[%d]': must be a string containing an HTTP or HTTPS URL", idx)
			}
			if !isHTTPURL(value) {
				return fmt.Errorf("invalid parameter 'image_urls[%d]': must be a valid HTTP or HTTPS URL", idx)
			}
		}
	}
	if raw, ok := payload["mask_url"]; ok {
		value, ok := raw.(string)
		if !ok {
			return errors.New("invalid parameter 'mask_url': must be a string containing an HTTP or HTTPS URL")
		}
		if value != "" && !isHTTPURL(value) {
			return errors.New("invalid parameter 'mask_url': must be a valid HTTP or HTTPS URL")
		}
	}
	return nil
}

func isHTTPURL(value string) bool {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")
}
