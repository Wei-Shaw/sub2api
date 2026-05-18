//go:build embed

package seo

// SiteConfig is the top-level site metadata in seo.json.
type SiteConfig struct {
	Name           string       `json:"name"`
	BaseURL        string       `json:"baseUrl"`
	DefaultLang    string       `json:"defaultLang"`
	SupportedLangs []string     `json:"supportedLangs"`
	TwitterHandle  string       `json:"twitterHandle"`
	Organization   Organization `json:"organization"`
}

// Organization mirrors schema.org/Organization minimal shape.
type Organization struct {
	Name   string   `json:"name"`
	Logo   string   `json:"logo"`
	SameAs []string `json:"sameAs,omitempty"`
}

// Record is per-route SEO metadata.
// Langs is populated from arbitrary "zh"/"en" keys via parseRecord.
type Record struct {
	Indexable  bool                   `json:"indexable"`
	Priority   float64                `json:"priority,omitempty"`
	Changefreq string                 `json:"changefreq,omitempty"`
	Dynamic    string                 `json:"dynamic,omitempty"`
	JsonLd     []string               `json:"jsonLd,omitempty"`
	Langs      map[string]LangPayload `json:"-"`
}

// LangPayload is language-specific SEO text for a route.
type LangPayload struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Keywords    []string `json:"keywords,omitempty"`
	OgImage     string   `json:"ogImage,omitempty"`
}
