//go:build embed

package seo

import (
	"encoding/json"
	"strings"
)

// BuildOrganization returns a schema.org Organization object as map.
func BuildOrganization(site SiteConfig) map[string]any {
	logo := site.Organization.Logo
	if !strings.HasPrefix(logo, "http") {
		logo = site.BaseURL + logo
	}
	obj := map[string]any{
		"@type": "Organization",
		"@id":   site.BaseURL + "#org",
		"name":  site.Organization.Name,
		"url":   site.BaseURL,
		"logo":  logo,
	}
	if len(site.Organization.SameAs) > 0 {
		obj["sameAs"] = site.Organization.SameAs
	}
	return obj
}

// BuildWebSite returns a schema.org WebSite object.
func BuildWebSite(site SiteConfig) map[string]any {
	return map[string]any{
		"@type": "WebSite",
		"@id":   site.BaseURL + "#website",
		"url":   site.BaseURL,
		"name":  site.Name,
		"potentialAction": map[string]any{
			"@type":       "SearchAction",
			"target":      site.BaseURL + "/faq?q={search_term_string}",
			"query-input": "required name=search_term_string",
		},
	}
}

// BuildSoftwareApplication returns a schema.org SoftwareApplication object.
func BuildSoftwareApplication(site SiteConfig, lp LangPayload) map[string]any {
	return map[string]any{
		"@type":               "SoftwareApplication",
		"name":                site.Name,
		"description":         lp.Description,
		"applicationCategory": "DeveloperApplication",
		"operatingSystem":     "Web",
		"offers": map[string]any{
			"@type":         "Offer",
			"price":         "0",
			"priceCurrency": "USD",
		},
	}
}

// BuildBreadcrumb returns a BreadcrumbList for the given path.
func BuildBreadcrumb(site SiteConfig, path string, langPayload LangPayload) map[string]any {
	items := []map[string]any{
		{
			"@type":    "ListItem",
			"position": 1,
			"name":     site.Name,
			"item":     site.BaseURL + "/home",
		},
	}
	if path != "/home" && path != "/" {
		items = append(items, map[string]any{
			"@type":    "ListItem",
			"position": 2,
			"name":     langPayload.Title,
			"item":     site.BaseURL + path,
		})
	}
	return map[string]any{
		"@type":           "BreadcrumbList",
		"itemListElement": items,
	}
}

// BuildArticle returns a schema.org Article for legal docs / blog posts.
func BuildArticle(site SiteConfig, lp LangPayload, url string) map[string]any {
	return map[string]any{
		"@type":       "Article",
		"headline":    lp.Title,
		"description": lp.Description,
		"url":         url,
		"publisher": map[string]any{
			"@id": site.BaseURL + "#org",
		},
	}
}

// BuildFAQPage returns a FAQPage object using embedded FAQ data.
func BuildFAQPage(lang string) (map[string]any, error) {
	items, err := LoadFAQ(lang)
	if err != nil {
		return nil, err
	}
	mainEntity := make([]map[string]any, 0, len(items))
	for _, it := range items {
		mainEntity = append(mainEntity, map[string]any{
			"@type": "Question",
			"name":  it.Q,
			"acceptedAnswer": map[string]any{
				"@type": "Answer",
				"text":  it.A,
			},
		})
	}
	return map[string]any{
		"@type":      "FAQPage",
		"mainEntity": mainEntity,
	}, nil
}

// RenderGraph composes the requested schema.org types into one @graph JSON-LD string.
// The output is safe to embed inside <script type="application/ld+json">.
func RenderGraph(site SiteConfig, rec Record, lp LangPayload, path, lang string) string {
	graph := make([]map[string]any, 0, len(rec.JsonLd))
	pageURL := site.BaseURL + path

	for _, t := range rec.JsonLd {
		switch t {
		case "Organization":
			graph = append(graph, BuildOrganization(site))
		case "WebSite":
			graph = append(graph, BuildWebSite(site))
		case "SoftwareApplication":
			graph = append(graph, BuildSoftwareApplication(site, lp))
		case "BreadcrumbList":
			graph = append(graph, BuildBreadcrumb(site, path, lp))
		case "Article":
			graph = append(graph, BuildArticle(site, lp, pageURL))
		case "FAQPage":
			if obj, err := BuildFAQPage(lang); err == nil {
				graph = append(graph, obj)
			}
		}
	}

	doc := map[string]any{
		"@context": "https://schema.org",
		"@graph":   graph,
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return "{}"
	}
	out := strings.ReplaceAll(string(b), "</", `<\/`)
	return out
}
