//go:build embed

package seo

import (
	"encoding/json"
	"fmt"
)

// FAQItem mirrors shared/seo/faq.{zh,en}.json entries.
type FAQItem struct {
	ID      string `json:"id"`
	Q       string `json:"q"`
	A       string `json:"a"`
	Details string `json:"details,omitempty"`
}

// LoadFAQ returns the FAQ list for the given language ("zh" or "en").
func LoadFAQ(lang string) ([]FAQItem, error) {
	var name string
	switch lang {
	case "zh":
		name = "data/faq.zh.json"
	case "en":
		name = "data/faq.en.json"
	default:
		return nil, fmt.Errorf("seo: unsupported FAQ lang %q", lang)
	}

	raw, err := faqFS.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("seo: read %s: %w", name, err)
	}
	var items []FAQItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("seo: parse %s: %w", name, err)
	}
	return items, nil
}
