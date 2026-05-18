//go:build embed

package seo

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed data/seo.json
var seoJSON []byte

//go:embed data/faq.zh.json data/faq.en.json
var faqFS embed.FS

// Registry holds parsed SEO metadata loaded from embedded JSON.
type Registry struct {
	Site   SiteConfig
	Routes map[string]Record
}

// NewRegistry parses the embedded seo.json into a Registry.
// Returns error if JSON is malformed or required fields missing.
func NewRegistry() (*Registry, error) {
	var raw struct {
		Site   SiteConfig                 `json:"site"`
		Routes map[string]json.RawMessage `json:"routes"`
	}
	if err := json.Unmarshal(seoJSON, &raw); err != nil {
		return nil, fmt.Errorf("seo: parse seo.json: %w", err)
	}
	if raw.Site.Name == "" || len(raw.Site.SupportedLangs) == 0 {
		return nil, fmt.Errorf("seo: site.name and site.supportedLangs are required")
	}

	routes := make(map[string]Record, len(raw.Routes))
	for path, rawRec := range raw.Routes {
		rec, err := parseRecord(rawRec, raw.Site.SupportedLangs)
		if err != nil {
			return nil, fmt.Errorf("seo: route %q: %w", path, err)
		}
		routes[path] = rec
	}

	return &Registry{Site: raw.Site, Routes: routes}, nil
}

func parseRecord(raw json.RawMessage, langs []string) (Record, error) {
	var rec Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		return Record{}, err
	}
	// Pull lang payloads from arbitrary top-level keys (zh, en, ...).
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err != nil {
		return Record{}, err
	}
	rec.Langs = make(map[string]LangPayload, len(langs))
	for _, lang := range langs {
		v, ok := asMap[lang]
		if !ok {
			return Record{}, fmt.Errorf("missing lang block %q", lang)
		}
		var lp LangPayload
		if err := json.Unmarshal(v, &lp); err != nil {
			return Record{}, fmt.Errorf("lang %q: %w", lang, err)
		}
		if lp.Title == "" || lp.Description == "" {
			return Record{}, fmt.Errorf("lang %q: title and description required", lang)
		}
		rec.Langs[lang] = lp
	}
	return rec, nil
}
