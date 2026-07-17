package securityaudit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeLegacyEngine struct {
	decision *LegacyDecision
	err      error
	calls    atomic.Int64
REDACTED

func (f *fakeLegacyEngine) Check(context.Context, Request) (*LegacyDecision, error) {
	f.calls.Add(1)
	return f.decision, f.err
REDACTED

type fakePromptEngine struct {
	mode      Mode
	decision  *PromptDecision
	err       error
	enqueues  atomic.Int64
	evaluates atomic.Int64
REDACTED

func (f *fakePromptEngine) EffectiveMode() Mode { return f.mode REDACTED
func (f *fakePromptEngine) Enqueue(context.Context, Request) error {
	f.enqueues.Add(1)
	return f.err
REDACTED
func (f *fakePromptEngine) Evaluate(context.Context, Request) (*PromptDecision, error) {
	f.evaluates.Add(1)
	return f.decision, f.err
REDACTED

func TestCoordinatorModesAndPriority(t *testing.T) {
	tests := []struct {
		name           string
		mode           Mode
		legacy         *LegacyDecision
		prompt         *PromptDecision
		promptErr      error
		wantKind       DecisionKind
		wantCode       string
		wantEnqueue    int64
		wantEvaluation int64
REDACTED{
		{name: "off", mode: ModeOff, wantKind: DecisionAllowREDACTED,
		{name: "async only enqueues", mode: ModeAsync, wantKind: DecisionAllow, wantEnqueue: 1REDACTED,
		{name: "prompt block", mode: ModeBlocking, prompt: &PromptDecision{Kind: DecisionBlockREDACTED, wantKind: DecisionBlock, wantCode: ErrorCodeBlocked, wantEvaluation: 1REDACTED,
		{name: "prompt unavailable", mode: ModeBlocking, promptErr: errors.New("down"), wantKind: DecisionUnavailable, wantCode: ErrorCodeUnavailable, wantEvaluation: 1REDACTED,
		{name: "legacy wins both block", mode: ModeBlocking,
			legacy: &LegacyDecision{Blocked: true, StatusCode: http.StatusForbidden, ErrorCode: "content_policy_violation", Message: "legacy"REDACTED,
			prompt: &PromptDecision{Kind: DecisionBlockREDACTED, wantKind: DecisionBlock, wantCode: "content_policy_violation", wantEvaluation: 1REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			legacy := &fakeLegacyEngine{decision: tt.legacyREDACTED
			prompt := &fakePromptEngine{mode: tt.mode, decision: tt.prompt, err: tt.promptErrREDACTED
			decision := NewCoordinator(legacy, prompt).Check(context.Background(), Request{Body: []byte(`{REDACTED`)REDACTED)
			require.Equal(t, tt.wantKind, decision.Kind)
			require.Equal(t, tt.wantCode, decision.ErrorCode)
			require.Equal(t, int64(1), legacy.calls.Load())
			require.Equal(t, tt.wantEnqueue, prompt.enqueues.Load())
			require.Equal(t, tt.wantEvaluation, prompt.evaluates.Load())
	REDACTED)
REDACTED
REDACTED

func TestCoordinatorDoesNotMutateRequestBody(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"hello"REDACTED]REDACTED`)
	original := append([]byte(nil), body...)
	prompt := &fakePromptEngine{mode: ModeAsyncREDACTED
	decision := NewCoordinator(&fakeLegacyEngine{REDACTED, prompt).Check(context.Background(), Request{Body: bodyREDACTED)
	require.True(t, decision.AllowNextStage)
	require.Equal(t, original, body)
REDACTED

func TestCoordinatorBlockingPriorityCoversBothEngineDecisionMatrix(t *testing.T) {
	legacyCases := []struct {
		name     string
		decision *LegacyDecision
REDACTED{
		{name: "allow", decision: &LegacyDecision{Allowed: true, StatusCode: http.StatusOK, Action: "allow"REDACTEDREDACTED,
		{name: "flag", decision: &LegacyDecision{Allowed: true, Flagged: true, StatusCode: http.StatusOK, Action: "flag"REDACTEDREDACTED,
		{name: "block", decision: &LegacyDecision{Blocked: true, StatusCode: http.StatusForbidden, ErrorCode: "legacy_exact_code", Message: "legacy exact message", Action: "block"REDACTEDREDACTED,
REDACTED
	promptCases := []struct {
		name     string
		decision *PromptDecision
		wantKind DecisionKind
		wantCode string
REDACTED{
		{name: "allow", decision: &PromptDecision{Kind: DecisionAllow, AllowNextStage: trueREDACTED, wantKind: DecisionAllowREDACTED,
		{name: "flag", decision: &PromptDecision{Kind: DecisionFlag, AllowNextStage: trueREDACTED, wantKind: DecisionFlagREDACTED,
		{name: "block", decision: &PromptDecision{Kind: DecisionBlockREDACTED, wantKind: DecisionBlock, wantCode: ErrorCodeBlockedREDACTED,
		{name: "unavailable", decision: &PromptDecision{Kind: DecisionUnavailable, ErrorCode: ErrorCodeUnavailableREDACTED, wantKind: DecisionUnavailable, wantCode: ErrorCodeUnavailableREDACTED,
		{name: "invalid", decision: &PromptDecision{Kind: DecisionInvalid, ErrorCode: ErrorCodeInvalidResponseREDACTED, wantKind: DecisionInvalid, wantCode: ErrorCodeInvalidResponseREDACTED,
REDACTED

	for _, legacyCase := range legacyCases {
		for _, promptCase := range promptCases {
			t.Run(fmt.Sprintf("legacy_%s_prompt_%s", legacyCase.name, promptCase.name), func(t *testing.T) {
				legacy := &fakeLegacyEngine{decision: legacyCase.decisionREDACTED
				prompt := &fakePromptEngine{mode: ModeBlocking, decision: promptCase.decisionREDACTED
				decision := NewCoordinator(legacy, prompt).Check(context.Background(), Request{REDACTED)

				require.Same(t, legacyCase.decision, decision.Legacy)
				require.Same(t, promptCase.decision, decision.Prompt)
				require.Equal(t, int64(1), legacy.calls.Load())
				require.Equal(t, int64(1), prompt.evaluates.Load())
				if legacyCase.name == "block" {
					require.Equal(t, DecisionBlock, decision.Kind)
					require.Equal(t, "legacy_exact_code", decision.ErrorCode)
					require.Equal(t, "legacy exact message", decision.ClientMessage)
					require.False(t, decision.AllowNextStage)
					return
			REDACTED
				require.Equal(t, promptCase.wantKind, decision.Kind)
				require.Equal(t, promptCase.wantCode, decision.ErrorCode)
				require.Equal(t, promptCase.decision.AllowNextStage, decision.AllowNextStage)
		REDACTED)
	REDACTED
REDACTED
REDACTED

func TestCoordinatorPreservesIndependentEngineFactsAndMapsOnlyGatewayOutcome(t *testing.T) {
	legacyDecision := &LegacyDecision{
		Allowed: true, Flagged: true, Message: "legacy finding", StatusCode: http.StatusAccepted,
		ErrorCode: "legacy_observation", Action: "legacy_action",
REDACTED
	promptResult := &NormalizedResult{
		Decision: EventCritical, RiskLevel: RiskCritical, Action: ActionBlock,
		Categories: []string{"pii"REDACTED, ScannerScores: map[string]float64{"pii": 1REDACTED,
REDACTED
	promptDecision := &PromptDecision{Kind: DecisionBlock, Result: promptResultREDACTED
	decision := NewCoordinator(
		&fakeLegacyEngine{decision: legacyDecisionREDACTED,
		&fakePromptEngine{mode: ModeBlocking, decision: promptDecisionREDACTED,
	).Check(context.Background(), Request{REDACTED)

	require.Same(t, legacyDecision, decision.Legacy)
	require.Same(t, promptDecision, decision.Prompt)
	require.Same(t, promptResult, decision.Prompt.Result)
	require.Equal(t, "legacy finding", decision.Legacy.Message)
	require.Equal(t, []string{"pii"REDACTED, decision.Prompt.Result.Categories)
	require.Equal(t, ErrorCodeBlocked, decision.ErrorCode)
REDACTED

func TestCoordinatorAsyncEnqueueFailuresNeverChangeResponseOrDownstreamDispatch(t *testing.T) {
	for _, enqueueErr := range []error{ErrQueueFull, ErrQueueAdmissionBusy, errors.New("redis unavailable"), errors.New("publish failed")REDACTED {
		prompt := &fakePromptEngine{mode: ModeAsync, err: enqueueErrREDACTED
		decision := NewCoordinator(&fakeLegacyEngine{decision: &LegacyDecision{Allowed: trueREDACTEDREDACTED, prompt).Check(context.Background(), Request{REDACTED)
		downstreamDispatches := 0
		status := http.StatusOK
		responseBody := "unchanged-upstream-response"
		if decision.AllowNextStage {
			downstreamDispatches++
	REDACTED else {
			status = decision.HTTPStatus
			responseBody = decision.ClientMessage
	REDACTED
		require.Equal(t, http.StatusOK, status)
		require.Equal(t, "unchanged-upstream-response", responseBody)
		require.Equal(t, 1, downstreamDispatches)
		require.Equal(t, int64(1), prompt.enqueues.Load())
		require.Zero(t, prompt.evaluates.Load())
REDACTED
REDACTED
