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

type PromptRecordingState interface {
	PromptRecordingEnabled() bool
}

type PromptResponseRecordingState interface {
	PromptResponseRecordingEnabled() bool
}

type Coordinator struct {
	legacy LegacyEngine
	prompt PromptEngine
	record PromptRecordRecorder
}

func NewCoordinator(legacy LegacyEngine, prompt PromptEngine, recorders ...PromptRecordRecorder) *Coordinator {
	var record PromptRecordRecorder
	if len(recorders) > 0 {
		record = recorders[0]
	}
	return &Coordinator{legacy: legacy, prompt: prompt, record: record}
}

func (c *Coordinator) Check(ctx context.Context, req Request) Decision {
	if c == nil {
		return allowDecision(nil, nil)
	}
	if c.record != nil && ShouldRecordPromptRequest(req) {
		c.recordPromptSafely(ctx, req)
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

func (c *Coordinator) RecordResponse(ctx context.Context, req Request, response PromptResponse) {
	if c == nil || c.record == nil {
		return
	}
	recorder, ok := c.record.(PromptResponseRecorder)
	if !ok {
		return
	}
	func() {
		defer func() {
			if recover() != nil {
				LogError(EventPromptRecordPersistFailed, mergeLogFields(requestLogFields(req), map[string]any{
					"status": "failed", "error_code": "prompt_record_response_callback_panic",
				}))
			}
		}()
		recorder.RecordResponse(ctx, req, response)
	}()
}

func (c *Coordinator) PreparePromptRecording(req Request) (Request, Request) {
	responseReference := req
	responseReference.Body = nil
	responseReference.Headers = nil
	if c == nil || !c.PromptResponseRecordingEnabled() || !ShouldRecordPromptRequest(req) {
		return req, responseReference
	}
	return newPromptRecordingRequestPair(req)
}

func (c *Coordinator) recordPromptSafely(ctx context.Context, req Request) {
	defer func() {
		if recover() != nil {
			if req.recordingCorrelation != nil {
				req.recordingCorrelation.complete(PromptRecordKey{}, false)
			}
			LogError(EventPromptRecordPersistFailed, mergeLogFields(requestLogFields(req), map[string]any{
				"status": "failed", "error_code": "prompt_record_callback_panic",
			}))
		}
	}()
	c.record.RecordPrompt(ctx, req)
}

func (c *Coordinator) PromptRecordingEnabled() (enabled bool) {
	defer func() {
		if recover() != nil {
			enabled = false
		}
	}()
	if c == nil || c.record == nil {
		return false
	}
	state, ok := c.record.(PromptRecordingState)
	if !ok {
		return true
	}
	return state.PromptRecordingEnabled()
}

func (c *Coordinator) PromptResponseRecordingEnabled() (enabled bool) {
	defer func() {
		if recover() != nil {
			enabled = false
		}
	}()
	if !c.PromptRecordingEnabled() {
		return false
	}
	state, ok := c.record.(PromptResponseRecordingState)
	if !ok {
		return true
	}
	return state.PromptResponseRecordingEnabled()
}

func (c *Coordinator) checkBlocking(ctx context.Context, req Request) Decision {
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
		if c.prompt == nil {
			prompt = unavailablePromptDecision(ErrorCodeUnavailable)
			return
		}
		result, err := c.prompt.Evaluate(ctx, req.Clone())
		if err != nil {
			var guardErr *GuardError
			if errors.As(err, &guardErr) && guardErr.Code == ErrorCodeInvalidResponse {
				prompt = unavailablePromptDecision(ErrorCodeInvalidResponse)
				return
			}
			prompt = unavailablePromptDecision(ErrorCodeUnavailable)
			return
		}
		if result == nil {
			prompt = unavailablePromptDecision(ErrorCodeUnavailable)
			return
		}
		prompt = result
	}()
	wg.Wait()
	return prioritize(legacy, prompt)
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
