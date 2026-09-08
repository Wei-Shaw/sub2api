package backend

type PlatformDescriptor struct {
	Key            string `json:"key"`
	DisplayName    string `json:"display_name"`
	Operation      string `json:"operation"`
	Protocol       string `json:"protocol"`
	DefaultBaseURL string `json:"default_base_url,omitempty"`
}
