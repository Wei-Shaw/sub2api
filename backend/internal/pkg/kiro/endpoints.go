package kiro

import "strings"

// Endpoint represents one upstream URL Kiro routes through. Quota
// exhaustion on the primary triggers a fallback to the next entry.
type Endpoint struct {
	URL       string
	Origin    string
	AmzTarget string
	Name      string
}

// Endpoints in fallback priority order. All three accept the same payload
// shape; only the URL, Origin, and Amz-Target header differ.
var endpoints = []Endpoint{
	{
		URL:       "https://q.us-east-1.amazonaws.com/generateAssistantResponse",
		Origin:    "AI_EDITOR",
		AmzTarget: "",
		Name:      "Kiro IDE",
	},
	{
		URL:       "https://codewhisperer.us-east-1.amazonaws.com/generateAssistantResponse",
		Origin:    "AI_EDITOR",
		AmzTarget: "AmazonCodeWhispererStreamingService.GenerateAssistantResponse",
		Name:      "CodeWhisperer",
	},
	{
		URL:       "https://q.us-east-1.amazonaws.com/generateAssistantResponse",
		Origin:    "AI_EDITOR",
		AmzTarget: "AmazonQDeveloperStreamingService.SendMessage",
		Name:      "AmazonQ",
	},
}

// EndpointPreference picks the primary endpoint when an account has a
// stored preference. Empty / "auto" returns all endpoints in default
// order. Other recognized values: "kiro", "codewhisperer", "amazonq".
func EndpointPreference(preferred string) []Endpoint {
	switch strings.ToLower(strings.TrimSpace(preferred)) {
	case "kiro":
		return reorder(0)
	case "codewhisperer":
		return reorder(1)
	case "amazonq":
		return reorder(2)
	default:
		// "auto" or unknown: original order.
		out := make([]Endpoint, len(endpoints))
		copy(out, endpoints)
		return out
	}
}

// Endpoints returns a copy of the default ordering, for callers that
// don't care about preference resolution.
func Endpoints() []Endpoint {
	out := make([]Endpoint, len(endpoints))
	copy(out, endpoints)
	return out
}

func reorder(primaryIdx int) []Endpoint {
	if primaryIdx < 0 || primaryIdx >= len(endpoints) {
		return Endpoints()
	}
	out := make([]Endpoint, 0, len(endpoints))
	out = append(out, endpoints[primaryIdx])
	for i := range endpoints {
		if i != primaryIdx {
			out = append(out, endpoints[i])
		}
	}
	return out
}
