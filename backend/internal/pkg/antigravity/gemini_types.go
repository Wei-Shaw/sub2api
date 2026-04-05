package antigravity

// Gemini v1internal 请求/响应类型定义

// V1InternalRequest v1internal 请求包装
type V1InternalRequest struct {
	Project     string        `json:"project"`
	RequestID   string        `json:"requestId"`
	UserAgent   string        `json:"userAgent"`
	RequestType string        `json:"requestType,omitempty"`
	Model       string        `json:"model"`
	Request     GeminiRequest `json:"request"`
}

// GeminiRequest Gemini 请求内容
type GeminiRequest struct {
	Contents          []GeminiContent         `json:"contents"`
	SystemInstruction *GeminiContent          `json:"systemInstruction,omitempty"`
	GenerationConfig  *GeminiGenerationConfig `json:"generationConfig,omitempty"`
	Tools             []GeminiToolDeclaration `json:"tools,omitempty"`
	ToolConfig        *GeminiToolConfig       `json:"toolConfig,omitempty"`
	SafetySettings    []GeminiSafetySetting   `json:"safetySettings,omitempty"`
	SessionID         string                  `json:"sessionId,omitempty"`
}

// GeminiContent Gemini 内容
type GeminiContent struct {
	Role  string       `json:"role"` // user, model
	Parts []GeminiPart `json:"parts"`
}

// GeminiPart Gemini 内容部分
type GeminiPart struct {
	Text             string                  `json:"text,omitempty"`
	Thought          bool                    `json:"thought,omitempty"`
	ThoughtSignature string                  `json:"thoughtSignature,omitempty"`
	InlineData       *GeminiInlineData       `json:"inlineData,omitempty"`
	FunctionCall     *GeminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *GeminiFunctionResponse `json:"functionResponse,omitempty"`
}

// GeminiInlineData Gemini 内联数据（图片等）
type GeminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

// GeminiFunctionCall Gemini 函数调用
type GeminiFunctionCall struct {
	Name string `json:"name"`
	Args any    `json:"args,omitempty"`
	ID   string `json:"id,omitempty"`
}

// GeminiFunctionResponse Gemini 函数响应
type GeminiFunctionResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
	ID       string         `json:"id,omitempty"`
}

// GeminiGenerationConfig Gemini 生成配置
type GeminiGenerationConfig struct {
	MaxOutputTokens int                   `json:"maxOutputTokens,omitempty"`
	Temperature     *float64              `json:"temperature,omitempty"`
	TopP            *float64              `json:"topP,omitempty"`
	TopK            *int                  `json:"topK,omitempty"`
	ThinkingConfig  *GeminiThinkingConfig `json:"thinkingConfig,omitempty"`
	StopSequences   []string              `json:"stopSequences,omitempty"`
	ImageConfig     *GeminiImageConfig    `json:"imageConfig,omitempty"`
}

// GeminiImageConfig Gemini 图片生成配置（gemini-3-pro-image / gemini-3.1-flash-image 等图片模型支持）
type GeminiImageConfig struct {
	AspectRatio string `json:"aspectRatio,omitempty"` // "1:1", "16:9", "9:16", "4:3", "3:4"
	ImageSize   string `json:"imageSize,omitempty"`   // "1K", "2K", "4K"
}

// GeminiThinkingConfig Gemini thinking 配置
type GeminiThinkingConfig struct {
	IncludeThoughts bool `json:"includeThoughts"`
	ThinkingBudget  int  `json:"thinkingBudget,omitempty"`
}

// GeminiToolDeclaration Gemini 工具声明
type GeminiToolDeclaration struct {
	FunctionDeclarations []GeminiFunctionDecl `json:"functionDeclarations,omitempty"`
	GoogleSearch         *GeminiGoogleSearch  `json:"googleSearch,omitempty"`
}

// GeminiFunctionDecl Gemini 函数声明
type GeminiFunctionDecl struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// GeminiGoogleSearch Gemini Google 搜索工具
type GeminiGoogleSearch struct {
	EnhancedContent *GeminiEnhancedContent `json:"enhancedContent,omitempty"`
}

// GeminiEnhancedContent 增强内容配置
type GeminiEnhancedContent struct {
	ImageSearch *GeminiImageSearch `json:"imageSearch,omitempty"`
}

// GeminiImageSearch 图片搜索配置
type GeminiImageSearch struct {
	MaxResultCount int `json:"maxResultCount,omitempty"`
}

// GeminiToolConfig Gemini 工具配置
type GeminiToolConfig struct {
	FunctionCallingConfig *GeminiFunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

// GeminiFunctionCallingConfig 函数调用配置
type GeminiFunctionCallingConfig struct {
	Mode string `json:"mode,omitempty"` // VALIDATED, AUTO, NONE
}

// GeminiSafetySetting Gemini 安全设置
type GeminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// V1InternalResponse v1internal 响应包装
type V1InternalResponse struct {
	Response     GeminiResponse `json:"response"`
	ResponseID   string         `json:"responseId,omitempty"`
	ModelVersion string         `json:"modelVersion,omitempty"`
}

// GeminiResponse Gemini 响应
type GeminiResponse struct {
	Candidates    []GeminiCandidate    `json:"candidates,omitempty"`
	UsageMetadata *GeminiUsageMetadata `json:"usageMetadata,omitempty"`
	ResponseID    string               `json:"responseId,omitempty"`
	ModelVersion  string               `json:"modelVersion,omitempty"`
}

// GeminiCandidate Gemini 候选响应
type GeminiCandidate struct {
	Content           *GeminiContent           `json:"content,omitempty"`
	FinishReason      string                   `json:"finishReason,omitempty"`
	Index             int                      `json:"index,omitempty"`
	GroundingMetadata *GeminiGroundingMetadata `json:"groundingMetadata,omitempty"`
}

// GeminiTokenDetail Gemini token 详情（按模态分类）
type GeminiTokenDetail struct {
	Modality   string `json:"modality"`
	TokenCount int    `json:"tokenCount"`
}

// GeminiUsageMetadata Gemini 用量元数据
type GeminiUsageMetadata struct {
	PromptTokenCount        int                 `json:"promptTokenCount,omitempty"`
	CandidatesTokenCount    int                 `json:"candidatesTokenCount,omitempty"`
	CachedContentTokenCount int                 `json:"cachedContentTokenCount,omitempty"`
	TotalTokenCount         int                 `json:"totalTokenCount,omitempty"`
	ThoughtsTokenCount      int                 `json:"thoughtsTokenCount,omitempty"` // thinking tokens（按输出价格计费）
	CandidatesTokensDetails []GeminiTokenDetail `json:"candidatesTokensDetails,omitempty"`
	PromptTokensDetails     []GeminiTokenDetail `json:"promptTokensDetails,omitempty"`
}

// ImageOutputTokens 从 CandidatesTokensDetails 中提取 IMAGE 模态的 token 数
func (m *GeminiUsageMetadata) ImageOutputTokens() int {
	for _, d := range m.CandidatesTokensDetails {
		if d.Modality == "IMAGE" {
			return d.TokenCount
		}
	}
	return 0
}

// GeminiGroundingMetadata Gemini grounding 元数据（Web Search）
type GeminiGroundingMetadata struct {
	WebSearchQueries []string               `json:"webSearchQueries,omitempty"`
	GroundingChunks  []GeminiGroundingChunk `json:"groundingChunks,omitempty"`
}

// GeminiGroundingChunk Gemini grounding chunk
type GeminiGroundingChunk struct {
	Web *GeminiGroundingWeb `json:"web,omitempty"`
}

// GeminiGroundingWeb Gemini grounding web 信息
type GeminiGroundingWeb struct {
	Title string `json:"title,omitempty"`
	URI   string `json:"uri,omitempty"`
}

// DefaultSafetySettings 默认安全设置（关闭所有过滤）
var DefaultSafetySettings = []GeminiSafetySetting{
	{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "OFF"},
	{Category: "HARM_CATEGORY_HATE_SPEECH", Threshold: "OFF"},
	{Category: "HARM_CATEGORY_SEXUALLY_EXPLICIT", Threshold: "OFF"},
	{Category: "HARM_CATEGORY_DANGEROUS_CONTENT", Threshold: "OFF"},
	{Category: "HARM_CATEGORY_CIVIC_INTEGRITY", Threshold: "OFF"},
}

// DefaultStopSequences 默认停止序列
var DefaultStopSequences = []string{
	"<|user|>",
	"<|endoftext|>",
	"<|end_of_turn|>",
	"\n\nHuman:",
}
