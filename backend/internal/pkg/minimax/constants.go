package minimax

const (
	DefaultAnthropicBaseURL = "https://api.minimax.io/anthropic"
	DefaultOpenAIBaseURL    = "https://api.minimax.io/v1"
	CNAnthropicBaseURL      = "https://api.minimaxi.com/anthropic"
	CNOpenAIBaseURL         = "https://api.minimaxi.com/v1"
	DefaultTestModel        = "MiniMax-M3"
)

type Model struct {
	ID            string   `json:"id"`
	Object        string   `json:"object"`
	DisplayName   string   `json:"display_name"`
	ContextWindow int      `json:"context_window"`
	Modalities    []string `json:"input_modalities"`
	Thinking      []string `json:"thinking"`
}

var DefaultModels = []Model{
	{
		ID:            "MiniMax-M3",
		Object:        "model",
		DisplayName:   "MiniMax M3",
		ContextWindow: 1000000,
		Modalities:    []string{"text", "image", "video"},
		Thinking:      []string{"adaptive", "disabled"},
	},
	{
		ID:            "MiniMax-M2.7",
		Object:        "model",
		DisplayName:   "MiniMax M2.7",
		ContextWindow: 204800,
		Modalities:    []string{"text"},
		Thinking:      []string{"always_on"},
	},
}

func DefaultModelIDs() []string {
	ids := make([]string, 0, len(DefaultModels))
	for _, model := range DefaultModels {
		ids = append(ids, model.ID)
	}
	return ids
}
