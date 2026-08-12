package securityaudit

import (
	"context"
	"errors"
	"net/http"
	"sync"
)

type LegacyEngine interface {
	Check(ctx context.Context, req Request) (*LegacyDecision, error)
}

type PromptEngine interface {
	EffectiveMode() Mode
	Enqueue(ctx context.Context, req Request) error
	Evaluate(ctx context.Context, req Request) (*PromptDecision, error)
}

type PromptPolicy struct {
	EngineType      string
	CompositionMode string
}

type PromptPolicyProvider interface {
	Policy(req Request) PromptPolicy
}

type ShadowComparisonRecorder interface {
	RecordShadowComparison(context.Context, string, ShadowComparison)
}

type Coordinator struct {
	legacy LegacyEngine
	prompt PromptEngine
}

func NewCoordinator(legacy LegacyEngine, prompt PromptEngine) *Coordinator {
	return &Coordinator{legacy: legacy, prompt: prompt}
}

func (c *Coordinator) Check(ctx context.Context, req Request) Decision {
	if c == nil {
		return allowDecision(nil, nil)
	}
	mode := ModeOff
	if c.prompt != nil {
		mode = c.prompt.EffectiveMode()
	}
	switch mode {
	case ModeAsync:
		// Enqueue is deliberately best-effort. The implementation owns a bounded
		// context and copies request memory before it can outlive the Handler.
		_ = c.prompt.Enqueue(ctx, req.Clone())
		legacy, _ := c.checkLegacy(ctx, req)
		return prioritize(legacy, nil)
	case ModeBlocking:
		return c.checkBlocking(ctx, req)
	default:
		legacy, _ := c.checkLegacy(ctx, req)
		return prioritize(legacy, nil)
	}
}

func (c *Coordinator) checkBlocking(ctx context.Context, req Request) Decision {
	policy := PromptPolicy{EngineType: EngineQwen3Guard, CompositionMode: "combined"}
	if provider, ok := c.prompt.(PromptPolicyProvider); ok {
		policy = provider.Policy(req)
	}
	if policy.EngineType == EngineGenericLLM {
		switch policy.CompositionMode {
		case "keyword_first":
			legacy, _ := c.checkLegacy(ctx, req)
			if legacy != nil && legacy.Blocked {
				return prioritize(legacy, nil)
			}
			prompt := c.evaluatePrompt(ctx, req)
			c.recordShadowComparison(ctx, req.RequestID, policy.CompositionMode, legacy, prompt)
			return prioritize(legacy, prompt)
		case "llm_only":
			return prioritize(nil, c.evaluatePrompt(ctx, req))
		}
	}
	var wg sync.WaitGroup
	wg.Add(2)
	var legacy *LegacyDecision
	var prompt *PromptDecision
	go func() {
		defer wg.Done()
		legacy, _ = c.checkLegacy(ctx, req)
	}()
	go func() {
		defer wg.Done()
		prompt = c.evaluatePrompt(ctx, req)
	}()
	wg.Wait()
	if policy.EngineType == EngineGenericLLM {
		c.recordShadowComparison(ctx, req.RequestID, policy.CompositionMode, legacy, prompt)
	}
	return prioritize(legacy, prompt)
}

func (c *Coordinator) recordShadowComparison(ctx context.Context, requestID, mode string, legacy *LegacyDecision, prompt *PromptDecision) {
	recorder, ok := c.prompt.(ShadowComparisonRecorder)
	if !ok || prompt == nil || prompt.Result == nil || prompt.Result.Stage != "shadow" {
		return
	}
	keywordDecision := "allow"
	if legacy != nil && legacy.Blocked {
		keywordDecision = "block"
	}
	llmDecision := prompt.Kind
	switch prompt.Result.Action {
	case ActionBlock:
		llmDecision = DecisionBlock
	case ActionWarn:
		llmDecision = DecisionFlag
	}
	recorder.RecordShadowComparison(ctx, requestID, ShadowComparison{
		CompositionMode: mode, KeywordDecision: keywordDecision, LLMDecision: llmDecision,
		Agreement: (keywordDecision == "block") == (llmDecision == DecisionBlock),
	})
}

func (c *Coordinator) evaluatePrompt(ctx context.Context, req Request) *PromptDecision {
	if c.prompt == nil {
		return unavailablePromptDecision(ErrorCodeUnavailable)
	}
	result, err := c.prompt.Evaluate(ctx, req.Clone())
	if err != nil {
		var guardErr *GuardError
		if errors.As(err, &guardErr) && guardErr.Code == ErrorCodeInvalidResponse {
			return unavailablePromptDecision(ErrorCodeInvalidResponse)
		}
		return unavailablePromptDecision(ErrorCodeUnavailable)
	}
	if result == nil {
		return unavailablePromptDecision(ErrorCodeUnavailable)
	}
	return result
}

func (c *Coordinator) checkLegacy(ctx context.Context, req Request) (*LegacyDecision, error) {
	if c.legacy == nil {
		return nil, nil
	}
	return c.legacy.Check(ctx, req)
}

func prioritize(legacy *LegacyDecision, prompt *PromptDecision) Decision {
	if legacy != nil && legacy.Blocked {
		status := legacy.StatusCode
		if status < 400 || status > 599 {
			status = http.StatusForbidden
		}
		code := legacy.ErrorCode
		if code == "" {
			code = "content_policy_violation"
		}
		return Decision{
			Kind: DecisionBlock, HTTPStatus: status, ErrorCode: code, ClientMessage: legacy.Message,
			Legacy: legacy, Prompt: prompt, AllowNextStage: false,
		}
	}
	if prompt == nil {
		return allowDecision(legacy, nil)
	}
	switch prompt.Kind {
	case DecisionBlock:
		return Decision{Kind: DecisionBlock, HTTPStatus: http.StatusForbidden, ErrorCode: ErrorCodeBlocked,
			ClientMessage: "提示词安全审计拒绝了该请求，请调整输入后重试", Legacy: legacy, Prompt: prompt}
	case DecisionInvalid:
		return Decision{Kind: DecisionInvalid, HTTPStatus: http.StatusServiceUnavailable, ErrorCode: ErrorCodeInvalidResponse,
			ClientMessage: "提示词安全审计暂时不可用，请稍后重试", Legacy: legacy, Prompt: prompt}
	case DecisionUnavailable:
		return Decision{Kind: DecisionUnavailable, HTTPStatus: http.StatusServiceUnavailable, ErrorCode: ErrorCodeUnavailable,
			ClientMessage: "提示词安全审计暂时不可用，请稍后重试", Legacy: legacy, Prompt: prompt}
	case DecisionFlag:
		return Decision{Kind: DecisionFlag, HTTPStatus: http.StatusOK, Legacy: legacy, Prompt: prompt, AllowNextStage: true}
	default:
		return allowDecision(legacy, prompt)
	}
}

func allowDecision(legacy *LegacyDecision, prompt *PromptDecision) Decision {
	return Decision{Kind: DecisionAllow, HTTPStatus: http.StatusOK, Legacy: legacy, Prompt: prompt, AllowNextStage: true}
}

func unavailablePromptDecision(code string) *PromptDecision {
	kind := DecisionUnavailable
	if code == ErrorCodeInvalidResponse {
		kind = DecisionInvalid
	}
	return &PromptDecision{Kind: kind, ErrorCode: code, AllowNextStage: false}
}
