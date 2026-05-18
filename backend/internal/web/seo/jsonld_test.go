//go:build embed

package seo

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildOrganization(t *testing.T) {
	reg, err := NewRegistry()
	require.NoError(t, err)
	reg.Site.BaseURL = "https://example.com"

	obj := BuildOrganization(reg.Site)

	b, _ := json.Marshal(obj)
	s := string(b)
	assert.Contains(t, s, `"@type":"Organization"`)
	assert.Contains(t, s, `"name":"Sub2API"`)
	assert.Contains(t, s, `"url":"https://example.com"`)
	assert.Contains(t, s, `"logo":"https://example.com/logo.png"`)
}

func TestBuildWebSite(t *testing.T) {
	site := SiteConfig{Name: "Sub2API", BaseURL: "https://example.com"}
	obj := BuildWebSite(site)

	b, _ := json.Marshal(obj)
	s := string(b)
	assert.Contains(t, s, `"@type":"WebSite"`)
	assert.Contains(t, s, `"url":"https://example.com"`)
}

func TestBuildSoftwareApplication(t *testing.T) {
	site := SiteConfig{Name: "Sub2API", BaseURL: "https://example.com"}
	lp := LangPayload{Title: "Sub2API", Description: "AI gateway"}
	obj := BuildSoftwareApplication(site, lp)

	b, _ := json.Marshal(obj)
	s := string(b)
	assert.Contains(t, s, `"@type":"SoftwareApplication"`)
	assert.Contains(t, s, `"applicationCategory":"DeveloperApplication"`)
	assert.Contains(t, s, `"operatingSystem":"Web"`)
}

func TestRenderGraph_serializesValidJSON(t *testing.T) {
	site := SiteConfig{Name: "Sub2API", BaseURL: "https://example.com",
		Organization: Organization{Name: "Sub2API", Logo: "/logo.png"}}
	rec := Record{JsonLd: []string{"Organization", "WebSite"}}
	lp := LangPayload{Title: "T", Description: "D"}

	out := RenderGraph(site, rec, lp, "/home", "zh")

	var v any
	require.NoError(t, json.Unmarshal([]byte(out), &v))
	assert.Contains(t, out, `"@graph"`)
	assert.False(t, strings.Contains(out, "</script>"), "must escape </script>")
}

func TestBuildFAQPage_fromEmbeddedData(t *testing.T) {
	obj, err := BuildFAQPage("zh")
	require.NoError(t, err)
	assert.Equal(t, "FAQPage", obj["@type"])
	entities, ok := obj["mainEntity"].([]map[string]any)
	require.True(t, ok)
	assert.GreaterOrEqual(t, len(entities), 10)
	assert.Equal(t, "Question", entities[0]["@type"])
}

func TestBuildBreadcrumb_homeOnlyOneItem(t *testing.T) {
	site := SiteConfig{Name: "Sub2API", BaseURL: "https://example.com"}
	lp := LangPayload{Title: "Home"}
	obj := BuildBreadcrumb(site, "/home", lp)
	items, _ := obj["itemListElement"].([]map[string]any)
	assert.Len(t, items, 1)
}

func TestBuildBreadcrumb_subpathHasTwoItems(t *testing.T) {
	site := SiteConfig{Name: "Sub2API", BaseURL: "https://example.com"}
	lp := LangPayload{Title: "FAQ"}
	obj := BuildBreadcrumb(site, "/faq", lp)
	items, _ := obj["itemListElement"].([]map[string]any)
	assert.Len(t, items, 2)
}
