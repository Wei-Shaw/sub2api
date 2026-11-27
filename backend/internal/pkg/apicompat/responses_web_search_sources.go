package apicompat

import (
	"encoding/json"
	"slices"
)

const webSearchSourcesInclude = "web_search_call.action.sources"

// IncludeWebSearchSources asks a hosted web_search to report the pages it
// used; without it a web_search_call carries only its query.
func IncludeWebSearchSources(req *ResponsesRequest) {
	for _, tool := range req.Tools {
		if tool.Type == "web_search" {
			if !slices.Contains(req.Include, webSearchSourcesInclude) {
				req.Include = append(req.Include, webSearchSourcesInclude)
			}
			return
		}
	}
}

type webSearchResult struct {
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

// webSearchCallResults is the web_search_tool_result content for one call: a
// search's action.sources, or the single page an open_page opened. Callers
// pass completed calls only. Sources and the page URL are decoded leniently, so
// a malformed value cannot fail the terminal event that carries the call.
func webSearchCallResults(item *ResponsesOutput) json.RawMessage {
	results := []webSearchResult{}
	if action := item.Action; action != nil {
		var sources []json.RawMessage
		if json.Unmarshal(action.Sources, &sources) == nil && sources != nil {
			for _, raw := range sources {
				var source map[string]any
				_ = json.Unmarshal(raw, &source)
				if rawURL, ok := source["url"].(string); ok && rawURL != "" {
					results = append(results, newWebSearchResult(rawURL))
				}
			}
		} else if action.Type == "open_page" {
			var pageURL string
			if json.Unmarshal(action.URL, &pageURL) == nil && pageURL != "" {
				results = append(results, newWebSearchResult(pageURL))
			}
		}
	}
	out, _ := json.Marshal(results)
	return out
}

// newWebSearchResult uses the URL as the title because Responses sources carry
// no title and the Anthropic block requires one.
func newWebSearchResult(rawURL string) webSearchResult {
	return webSearchResult{Type: "web_search_result", URL: rawURL, Title: rawURL}
}
