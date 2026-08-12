package securityaudit

type eventMetadata struct {
	EngineType        string            `json:"engine_type,omitempty"`
	AuditModel        string            `json:"audit_model,omitempty"`
	SchemaVersion     int               `json:"schema_version,omitempty"`
	Confidence        float64           `json:"confidence,omitempty"`
	EnforcementStage  string            `json:"enforcement_stage,omitempty"`
	FailurePolicy     string            `json:"failure_policy,omitempty"`
	PromptTokens      int               `json:"prompt_tokens,omitempty"`
	CompletionTokens  int               `json:"completion_tokens,omitempty"`
	TotalTokens       int               `json:"total_tokens,omitempty"`
	UnknownCategories []string          `json:"unknown_categories,omitempty"`
	ShadowComparison  *ShadowComparison `json:"shadow_comparison,omitempty"`
	ErrorCode         string            `json:"error_code,omitempty"`
	ErrorMessage      string            `json:"error_message,omitempty"`
	FailedChunkIndex  int               `json:"failed_chunk_index,omitempty"`
}

func eventMetadataFromResult(result *NormalizedResult) eventMetadata {
	if result == nil {
		return eventMetadata{}
	}
	return eventMetadata{
		EngineType: result.EngineType, AuditModel: result.ScannerVersion,
		SchemaVersion: result.SchemaVersion, Confidence: result.Confidence,
		EnforcementStage: result.Stage, FailurePolicy: result.FailurePolicy,
		PromptTokens: result.PromptTokens, CompletionTokens: result.CompletionTokens,
		TotalTokens: result.TotalTokens, UnknownCategories: result.UnknownCategories,
	}
}

func (metadata eventMetadata) apply(event *Event) {
	if event == nil {
		return
	}
	event.EngineType, event.AuditModel = metadata.EngineType, metadata.AuditModel
	event.SchemaVersion, event.Confidence = metadata.SchemaVersion, metadata.Confidence
	event.EnforcementStage, event.FailurePolicy = metadata.EnforcementStage, metadata.FailurePolicy
	event.PromptTokens, event.CompletionTokens, event.TotalTokens = metadata.PromptTokens, metadata.CompletionTokens, metadata.TotalTokens
	event.UnknownCategories = append([]string(nil), metadata.UnknownCategories...)
	event.ShadowComparison = metadata.ShadowComparison
	event.ErrorCode, event.ErrorMessage = metadata.ErrorCode, metadata.ErrorMessage
	event.FailedChunkIndex = metadata.FailedChunkIndex
}
