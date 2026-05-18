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

// Resolve returns the SEO record and lang payload for the given request path and language.
// settingsJSON is the public-settings payload (used for dynamic routes like /legal/:id).
// If path or lang cannot be matched, returns a default noindex record so callers
// never need to nil-check.
func (r *Registry) Resolve(path, lang string, settingsJSON []byte) (Record, LangPayload) {
	if !contains(r.Site.SupportedLangs, lang) {
		lang = r.Site.DefaultLang
	}

	if rec, ok := r.Routes[path]; ok {
		if rec.Dynamic != "" {
			return r.resolveDynamic(rec, path, lang, settingsJSON)
		}
		return rec, rec.Langs[lang]
	}

	for pat, rec := range r.Routes {
		if rec.Dynamic == "" {
			continue
		}
		if matchPattern(pat, path) {
			return r.resolveDynamic(rec, path, lang, settingsJSON)
		}
	}

	return r.defaultRecord(lang), r.defaultLangPayload(lang)
}

func (r *Registry) defaultRecord(_ string) Record {
	return Record{Indexable: false}
}

func (r *Registry) defaultLangPayload(_ string) LangPayload {
	return LangPayload{
		Title:       r.Site.Name,
		Description: r.Site.Name,
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// matchPattern returns true if path matches a pattern with ":param" placeholders.
// Example: matchPattern("/legal/:id", "/legal/terms") == true.
func matchPattern(pattern, path string) bool {
	pp := splitPath(pattern)
	xp := splitPath(path)
	if len(pp) != len(xp) {
		return false
	}
	for i := range pp {
		if len(pp[i]) > 0 && pp[i][0] == ':' {
			if xp[i] == "" {
				return false
			}
			continue
		}
		if pp[i] != xp[i] {
			return false
		}
	}
	return true
}

func splitPath(p string) []string {
	out := []string{}
	cur := ""
	for _, ch := range p {
		if ch == '/' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(ch)
	}
	out = append(out, cur)
	return out
}

// resolveDynamic dispatches to per-Dynamic-type handlers. T7 fills in legalDoc.
func (r *Registry) resolveDynamic(rec Record, _ string, lang string, _ []byte) (Record, LangPayload) {
	return rec, rec.Langs[lang]
}
