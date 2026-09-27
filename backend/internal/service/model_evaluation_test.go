package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const evaluationResponse = `{"status":"completed","model":"reported","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"选择 9 圆和 12 星。\nFINAL_ANSWER: 21"}]}],"usage":{"input_tokens":120,"output_tokens":44,"output_tokens_details":{"reasoning_tokens":30},"input_tokens_details":{"cached_tokens":0}}}`

func TestModelEvaluationGrade(t *testing.T) {
	for _, tt := range []struct{ text, status string }{
		{"FINAL_ANSWER: 21", "correct"},
		{"推理\r\nFINAL_ANSWER: 21\r\n", "correct"},
		{"21 不够\nFINAL_ANSWER: 29", "incorrect"},
		{"FINAL_ANSWER: 121", "incorrect"},
		{"FINAL_ANSWER: 21.5", "ungraded"},
		{"FINAL_ANSWER: 021", "ungraded"},
		{"答案 21", "ungraded"},
		{"FINAL_ANSWER: 21\n但答案是 29", "ungraded"},
		{"FINAL_ANSWER: 29\nFINAL_ANSWER: 21", "ungraded"},
		{"", "ungraded"},
	} {
		t.Run(tt.text, func(t *testing.T) {
			status, answer := gradeModelEvaluation(tt.text)
			require.Equal(t, tt.status, status)
			if tt.status == "ungraded" {
				require.Nil(t, answer)
			} else {
				require.NotNil(t, answer)
			}
		})
	}
}

func TestModelEvaluationParseUsageAndIncomplete(t *testing.T) {
	result := &ModelEvaluationResult{Status: "error"}
	parseModelEvaluationResponse([]byte(evaluationResponse), result)
	require.Equal(t, "correct", result.Status)
	require.EqualValues(t, 30, *result.ReasoningTokens)
	require.EqualValues(t, 0, *result.CachedTokens)
	result = &ModelEvaluationResult{Status: "error"}
	parseModelEvaluationResponse([]byte(`{"choices":[{"message":{"content":"FINAL_ANSWER: 21"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2}}`), result)
	require.Equal(t, "correct", result.Status)
	require.Nil(t, result.ReasoningTokens)
	require.EqualValues(t, 4, *result.InputTokens)
	for _, body := range []string{
		strings.Replace(evaluationResponse, `"completed"`, `"incomplete"`, 1),
		`{"choices":[{"message":{"content":"FINAL_ANSWER: 21"},"finish_reason":"length"}]}`,
		`{"status":"completed","output":[{"type":"reasoning","content":[{"type":"output_text","text":"FINAL_ANSWER: 21"}]}]}`,
		`data: {"type":"response.completed"}`,
		`{"status":"completed","output":[]}`,
	} {
		result = &ModelEvaluationResult{Status: "error"}
		parseModelEvaluationResponse([]byte(body), result)
		require.Equal(t, "error", result.Status)
		require.NotEmpty(t, result.Error)
		require.Nil(t, result.Answer)
	}
}

type evaluationAccountsStub struct {
	AccountRepository
	account *Account
}

func (s evaluationAccountsStub) GetByID(context.Context, int64) (*Account, error) {
	return s.account, nil
}

type evaluationGroupsStub struct {
	GroupRepository
	group *Group
}

func (s evaluationGroupsStub) GetByID(context.Context, int64) (*Group, error) { return s.group, nil }

type evaluationSlotsStub struct {
	calls, releases int
	acquired        bool
}

func (s *evaluationSlotsStub) AcquireAccountSlot(context.Context, int64, int) (*AcquireResult, error) {
	s.calls++
	return &AcquireResult{Acquired: s.acquired, ReleaseFunc: func() { s.releases++ }}, nil
}

type evaluationGatewayStub struct {
	selection                 *AccountSelectionResult
	selectCalls, forwardCalls int
	selectedModel             string
	forwardAccount            int64
	body                      []byte
	mapping                   ChannelMappingResult
	restricted                bool
	err                       error
}

func (s *evaluationGatewayStub) ResolveChannelMappingAndRestrict(context.Context, *int64, string) (ChannelMappingResult, bool) {
	return s.mapping, s.restricted
}
func (s *evaluationGatewayStub) SelectAccountWithSchedulerForCapability(_ context.Context, _ *int64, previous, session, model string, _ map[int64]struct{}, _ OpenAIUpstreamTransport, _ OpenAIEndpointCapability, _, _, _ bool, _ ...string) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	s.selectCalls++
	s.selectedModel = model
	if previous != "" || session != "" {
		return nil, OpenAIAccountScheduleDecision{}, errors.New("session contamination")
	}
	return s.selection, OpenAIAccountScheduleDecision{}, nil
}
func (s *evaluationGatewayStub) Forward(ctx context.Context, c *gin.Context, account *Account, body []byte) (*OpenAIForwardResult, error) {
	s.forwardCalls++
	s.forwardAccount, s.body = account.ID, body
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("missing deadline")
	}
	if s.err != nil {
		return nil, s.err
	}
	c.Data(http.StatusOK, "application/json", []byte(evaluationResponse))
	effort := gjson.GetBytes(body, "reasoning.effort").String()
	return &OpenAIForwardResult{UpstreamModel: "mapped-upstream", UpstreamResponseModel: "reported", ReasoningEffort: &effort, UpstreamEndpoint: "/v1/responses"}, nil
}

func newEvaluationTestService() (*ModelEvaluationService, *evaluationGatewayStub, *evaluationSlotsStub) {
	account := &Account{ID: 42, Name: "target", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	gateway := &evaluationGatewayStub{selection: &AccountSelectionResult{Account: account}}
	slots := &evaluationSlotsStub{acquired: true}
	svc := &ModelEvaluationService{
		accounts: evaluationAccountsStub{account: account},
		groups:   evaluationGroupsStub{group: &Group{ID: 3, Name: "group", Platform: PlatformOpenAI, Status: StatusActive}},
		gateway:  gateway, slots: slots, active: make(chan struct{}, 2),
	}
	return svc, gateway, slots
}

func TestModelEvaluationValidationNeverReachesUpstream(t *testing.T) {
	s, gateway, _ := newEvaluationTestService()
	for _, req := range []ModelEvaluationRequest{
		{TargetType: "account", TargetID: 0, Model: "gpt-5.4", Effort: "high"},
		{TargetType: "account", TargetID: 42, Model: "gpt-image-2", Effort: "high"},
		{TargetType: "account", TargetID: 42, Model: "gpt-5.4", Effort: "garbage"},
	} {
		_, err := s.Run(context.Background(), req)
		require.Error(t, err)
	}
	require.Zero(t, gateway.forwardCalls)
}

func TestModelEvaluationFixedAccountAndFreshPrompt(t *testing.T) {
	s, gateway, slots := newEvaluationTestService()
	result, err := s.Run(context.Background(), ModelEvaluationRequest{TargetType: "account", TargetID: 42, Model: "gpt-5.4", Effort: "high"})
	require.NoError(t, err)
	require.Equal(t, "correct", result.Status)
	require.EqualValues(t, 42, gateway.forwardAccount)
	require.Zero(t, gateway.selectCalls)
	require.Equal(t, 1, slots.releases)
	require.Equal(t, "mapped-upstream", result.UpstreamModel)
	require.Equal(t, "reported", result.ReportedModel)
	require.Contains(t, gjson.GetBytes(gateway.body, "input.0.content").String(), "不能识别口味")
	require.NotContains(t, string(gateway.body), "FINAL_ANSWER: 21")
	require.False(t, gjson.GetBytes(gateway.body, "previous_response_id").Exists())
	require.Empty(t, gjson.GetBytes(gateway.body, "tools").Array())
	require.False(t, gjson.GetBytes(gateway.body, "store").Bool())
}

func TestModelEvaluationGroupPolicyAndAcquiredSlot(t *testing.T) {
	s, gateway, slots := newEvaluationTestService()
	groups, ok := s.groups.(evaluationGroupsStub)
	require.True(t, ok)
	group := groups.group
	group.MaxReasoningEffort = "medium"
	group.ModelAllowlist = GroupModelAllowlist{Enabled: true, Models: []string{"public-model"}}
	gateway.mapping = ChannelMappingResult{Mapped: true, MappedModel: "gpt-5.4"}
	released := 0
	gateway.selection.Acquired = true
	gateway.selection.ReleaseFunc = func() { released++ }
	result, err := s.Run(context.Background(), ModelEvaluationRequest{TargetType: "group", TargetID: 3, Model: "public-model", Effort: "high"})
	require.NoError(t, err)
	require.Equal(t, "correct", result.Status)
	require.Equal(t, "medium", result.EffectiveEffort)
	require.Equal(t, "high", result.RequestedEffort)
	require.Equal(t, "gpt-5.4", gateway.selectedModel)
	require.Equal(t, "gpt-5.4", gjson.GetBytes(gateway.body, "model").String())
	require.Equal(t, "public-model", result.RequestedModel)
	require.Equal(t, 1, released)
	require.Zero(t, slots.calls)
	_, err = s.Run(context.Background(), ModelEvaluationRequest{TargetType: "group", TargetID: 3, Model: "other", Effort: "high"})
	require.Error(t, err)
	require.Equal(t, 1, gateway.forwardCalls)
	group.MaxReasoningEffortOverLimit = "deny"
	_, err = s.Run(context.Background(), ModelEvaluationRequest{TargetType: "group", TargetID: 3, Model: "public-model", Effort: "high"})
	require.Error(t, err)
	require.Equal(t, 1, gateway.selectCalls)
}

func TestModelEvaluationErrorsReleaseSlotsAndNeverLeakUpstreamDetails(t *testing.T) {
	s, gateway, slots := newEvaluationTestService()
	gateway.err = &UpstreamFailoverError{StatusCode: 429, ResponseBody: []byte("secret-token")}
	result, err := s.Run(context.Background(), ModelEvaluationRequest{TargetType: "account", TargetID: 42, Model: "gpt-5.4", Effort: "high"})
	require.NoError(t, err)
	require.Equal(t, "error", result.Status)
	require.Contains(t, result.Error, "429")
	require.NotContains(t, result.Error, "secret-token")
	require.Equal(t, 1, slots.releases)
	require.Equal(t, 1, gateway.forwardCalls)
	require.Zero(t, gateway.selectCalls)
	require.Empty(t, s.active)
}

func TestModelEvaluationScopeAndConcurrency(t *testing.T) {
	for _, change := range []func(*Account){
		func(a *Account) { a.Platform = PlatformAnthropic },
		func(a *Account) { a.Schedulable = false },
		func(a *Account) { id := int64(9); a.ParentAccountID = &id },
		func(a *Account) {
			a.Credentials = map[string]any{"model_mapping": map[string]any{"gpt-5.4": "gpt-image-2"}}
		},
	} {
		s, gateway, _ := newEvaluationTestService()
		accounts, ok := s.accounts.(evaluationAccountsStub)
		require.True(t, ok)
		change(accounts.account)
		result, err := s.Run(context.Background(), ModelEvaluationRequest{TargetType: "account", TargetID: 42, Model: "gpt-5.4", Effort: "high"})
		require.NoError(t, err)
		require.Equal(t, "error", result.Status)
		require.Zero(t, gateway.forwardCalls)
	}
	s, gateway, slots := newEvaluationTestService()
	slots.acquired = false
	result, err := s.Run(context.Background(), ModelEvaluationRequest{TargetType: "account", TargetID: 42, Model: "gpt-5.4", Effort: "high"})
	require.NoError(t, err)
	require.Equal(t, "error", result.Status)
	require.Zero(t, gateway.forwardCalls)
	s.active <- struct{}{}
	s.active <- struct{}{}
	_, err = s.Run(context.Background(), ModelEvaluationRequest{TargetType: "account", TargetID: 42, Model: "gpt-5.4", Effort: "high"})
	require.Error(t, err)
}

func TestModelEvaluationCancellationOnlyChangesEvaluationContexts(t *testing.T) {
	for _, evaluation := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		if evaluation {
			ctx = context.WithValue(ctx, modelEvaluationContextKey{}, true)
		}
		upstream, release := detachUpstreamContext(ctx)
		stream, streamRelease := detachStreamUpstreamContext(ctx, true)
		cancel()
		if evaluation {
			require.ErrorIs(t, upstream.Err(), context.Canceled)
			require.ErrorIs(t, stream.Err(), context.Canceled)
		} else {
			require.NoError(t, upstream.Err())
			require.NoError(t, stream.Err())
		}
		release()
		streamRelease()
	}
}
