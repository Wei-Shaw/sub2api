package backend

type OpenAIAdapter struct{}

func NewOpenAIAdapter() *OpenAIAdapter {
	return &OpenAIAdapter{}
}

func (*OpenAIAdapter) Platform() string {
	return "openai"
}

func (*OpenAIAdapter) Descriptor() PlatformDescriptor {
	return PlatformDescriptor{Key: "openai", DisplayName: "OpenAI", Protocol: "transparent", DefaultBaseURL: "https://api.openai.com/v1"}
}
