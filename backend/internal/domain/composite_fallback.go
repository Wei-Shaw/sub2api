package domain

type CompositeFallbackTarget struct {
	TargetPlatform string `json:"target_platform"`
	UpstreamModel  string `json:"upstream_model"`
}
