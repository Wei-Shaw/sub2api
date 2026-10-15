package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

// Evaluation requests must stop on cancellation, unlike customer requests that
// continue draining upstream for billing. Only this opt-in context changes that.
type modelEvaluationContextKey struct{}
type modelEvaluationUsageKey struct{}

// Preserve original usage before protocol conversion drops optional details.
// Normal gateway requests do not install this collector.
func captureModelEvaluationUsage(ctx context.Context, usage any) {
	if target, ok := ctx.Value(modelEvaluationUsageKey{}).(*[]byte); ok {
		*target, _ = json.Marshal(usage)
	}
}

type ModelEvaluationRequest struct {
	TargetType string `json:"target_type"`
	TargetID   int64  `json:"target_id"`
	Model      string `json:"model"`
	Effort     string `json:"effort"`
}

type ModelEvaluationResult struct {
	ID              string    `json:"id"`
	StartedAt       time.Time `json:"started_at"`
	Benchmark       string    `json:"benchmark"`
	TargetType      string    `json:"target_type"`
	TargetID        int64     `json:"target_id"`
	TargetName      string    `json:"target_name"`
	AccountID       int64     `json:"account_id"`
	AccountName     string    `json:"account_name"`
	RequestedModel  string    `json:"requested_model"`
	UpstreamModel   string    `json:"upstream_model"`
	ReportedModel   string    `json:"reported_model"`
	RequestedEffort string    `json:"requested_effort"`
	EffectiveEffort string    `json:"effective_effort"`
	Endpoint        string    `json:"endpoint"`
	Status          string    `json:"status"`
	Answer          *int      `json:"answer"`
	Text            string    `json:"text"`
	InputTokens     *int64    `json:"input_tokens"`
	OutputTokens    *int64    `json:"output_tokens"`
	ReasoningTokens *int64    `json:"reasoning_tokens"`
	CachedTokens    *int64    `json:"cached_tokens"`
	DurationMs      int64     `json:"duration_ms"`
	RequestID       string    `json:"request_id"`
	Error           string    `json:"error,omitempty"`
}

type modelEvaluationGateway interface {
	ResolveChannelMappingAndRestrict(context.Context, *int64, string) (ChannelMappingResult, bool)
	SelectAccountWithSchedulerForCapability(context.Context, *int64, string, string, string, map[int64]struct{}, OpenAIUpstreamTransport, OpenAIEndpointCapability, bool, bool, bool, ...string) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error)
	Forward(context.Context, *gin.Context, *Account, []byte) (*OpenAIForwardResult, error)
}

type modelEvaluationSlots interface {
	AcquireAccountSlot(context.Context, int64, int) (*AcquireResult, error)
}

// ModelEvaluationService runs one bounded round. The UI sequences rounds, so
// closing/canceling never leaves a detached background batch running.
type ModelEvaluationService struct {
	accounts AccountRepository
	groups   GroupRepository
	gateway  modelEvaluationGateway
	slots    modelEvaluationSlots
	active   chan struct{}
}

func NewModelEvaluationService(accounts AccountRepository, groups GroupRepository, gateway *OpenAIGatewayService, slots *ConcurrencyService) *ModelEvaluationService {
	return &ModelEvaluationService{accounts: accounts, groups: groups, gateway: gateway, slots: slots, active: make(chan struct{}, 2)}
}

func validateModelEvaluationRequest(req ModelEvaluationRequest) error {
	if req.TargetID <= 0 || (req.TargetType != "account" && req.TargetType != "group") || req.Model == "" || len(req.Model) > 200 {
		return infraerrors.BadRequest("INVALID_EVALUATION_TARGET", "请选择有效的分组或账号，并填写文本模型名称")
	}
	switch req.Effort {
	case "low", "medium", "high", "xhigh", "max":
	default:
		return infraerrors.BadRequest("INVALID_EVALUATION_EFFORT", "推理强度必须为 low、medium、high、xhigh 或 max")
	}
	if !modelEvaluationTextModel(req.Model) {
		return infraerrors.BadRequest("INVALID_EVALUATION_MODEL", "评测只支持文本推理模型，请选择文本模型后重试")
	}
	return nil
}

func (s *ModelEvaluationService) Run(ctx context.Context, req ModelEvaluationRequest) (*ModelEvaluationResult, error) {
	req.Model = strings.TrimSpace(req.Model)
	if err := validateModelEvaluationRequest(req); err != nil {
		return nil, err
	}
	select {
	case s.active <- struct{}{}:
		defer func() { <-s.active }()
	default:
		return nil, infraerrors.New(http.StatusTooManyRequests, "EVALUATION_BUSY", "评测正在运行，请等待当前评测结束后重试")
	}
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, modelEvaluationContextKey{}, true)
	var originalUsage []byte
	ctx = context.WithValue(ctx, modelEvaluationUsageKey{}, &originalUsage)
	ctx = WithRequestedReasoningEffort(ctx, req.Effort)
	result := &ModelEvaluationResult{
		ID: uuid.NewString(), StartedAt: time.Now().UTC(), Benchmark: modelEvaluationBenchmark,
		TargetType: req.TargetType, TargetID: req.TargetID, RequestedModel: req.Model,
		RequestedEffort: req.Effort, EffectiveEffort: req.Effort, Status: "error",
	}
	defer func() { result.DurationMs = time.Since(result.StartedAt).Milliseconds() }()
	model := req.Model
	body, _ := json.Marshal(map[string]any{
		"model": model, "stream": false, "store": false,
		"instructions": "独立解答用户的数学题，不使用外部工具。最后一行严格使用 FINAL_ANSWER: 整数 格式。",
		"input":        []map[string]any{{"role": "user", "content": modelEvaluationPrompt}},
		"reasoning":    map[string]string{"effort": req.Effort},
		"tools":        []any{}, "max_output_tokens": 16384,
	})
	var account *Account
	var selection *AccountSelectionResult
	var err error
	if req.TargetType == "group" {
		group, loadErr := s.groups.GetByID(ctx, req.TargetID)
		if loadErr != nil || group == nil {
			return nil, infraerrors.BadRequest("EVALUATION_GROUP_UNAVAILABLE", "分组不存在或读取失败，请刷新分组列表后重试")
		}
		result.TargetName = group.Name
		if group.Platform != PlatformOpenAI || !group.IsActive() {
			return nil, infraerrors.BadRequest("EVALUATION_GROUP_UNSUPPORTED", "请选择启用中的 OpenAI 分组")
		}
		if !group.ModelAllowlist.Allows(model) {
			return nil, infraerrors.BadRequest("EVALUATION_MODEL_DENIED", "所选模型不在分组白名单中，请选择允许的模型")
		}
		body, _, err = ApplyOpenAIReasoningEffortPolicy(body, group.MaxReasoningEffort, group.ReasoningEffortMappings, group.MaxReasoningEffortOverLimit)
		if err != nil {
			return nil, infraerrors.BadRequest("EVALUATION_EFFORT_DENIED", "推理强度被分组策略拒绝，请降低强度或检查分组策略")
		}
		result.EffectiveEffort = gjson.GetBytes(body, "reasoning.effort").String()
		mapping, restricted := s.gateway.ResolveChannelMappingAndRestrict(ctx, &group.ID, model)
		if restricted {
			return nil, infraerrors.BadRequest("EVALUATION_CHANNEL_DENIED", "模型被分组渠道限制，请检查渠道模型配置")
		}
		if mapping.Mapped && mapping.MappedModel != "" {
			model = mapping.MappedModel
			var payload map[string]any
			_ = json.Unmarshal(body, &payload)
			payload["model"] = model
			body, _ = json.Marshal(payload)
		}
		ctx = WithOpenAIForwardModel(ctx, model, false)
		selection, _, err = s.gateway.SelectAccountWithSchedulerForCapability(ctx, &group.ID, "", "", model, nil, OpenAIUpstreamTransportHTTPSSE, "", false, false, false, PlatformOpenAI)
		if selection != nil && selection.ReleaseFunc != nil {
			defer selection.ReleaseFunc()
		}
		if err != nil || selection == nil || selection.Account == nil {
			result.Error = "分组没有可用账号；请检查模型支持、账号状态、额度和并发后重试"
			return result, nil
		}
		account = selection.Account
	} else {
		account, err = s.accounts.GetByID(ctx, req.TargetID)
		if err != nil || account == nil {
			return nil, infraerrors.BadRequest("EVALUATION_ACCOUNT_UNAVAILABLE", "账号不存在或读取失败，请刷新账号列表后重试")
		}
		result.TargetName = account.Name
	}
	result.AccountID, result.AccountName = account.ID, account.Name
	if account.Platform != PlatformOpenAI || (account.Type != AccountTypeOAuth && account.Type != AccountTypeAPIKey) || account.IsShadow() || account.IsOpenAIFirstServe() {
		result.Error = "该账号暂不支持评测；请选择普通 OpenAI OAuth 或 API Key 账号（不含影子和首发账号）"
		return result, nil
	}
	if !account.IsSchedulable() || !account.IsModelSupported(model) {
		result.Error = "账号当前不可调度或不支持该模型；请检查状态、额度和模型映射后重试"
		return result, nil
	}
	result.UpstreamModel = account.GetMappedModel(model)
	if !modelEvaluationTextModel(result.UpstreamModel) {
		result.Error = "模型映射到了非文本模型；请调整模型映射后重试"
		return result, nil
	}
	if selection != nil && selection.ProfitGateActive() {
		// Administrative probes have no customer billing context. Do not bypass
		// the final profit admission gate by pretending they are billable traffic.
		result.Error = "该分组启用了利润准入策略；请改用固定账号评测"
		return result, nil
	}
	if selection == nil || !selection.Acquired {
		slot, slotErr := s.slots.AcquireAccountSlot(ctx, account.ID, account.Concurrency)
		if slotErr != nil || slot == nil || !slot.Acquired {
			result.Error = "账号并发已满或无法获取并发槽位；请稍后重试"
			return result, nil
		}
		if slot.ReleaseFunc != nil {
			defer slot.ReleaseFunc()
		}
	}
	if err := ctx.Err(); err != nil {
		result.Error = "评测已取消或超时；需要时请重新测试"
		return result, nil
	}
	writer := &modelEvaluationWriter{header: make(http.Header)}
	c, _ := gin.CreateTestContext(writer)
	c.Request, _ = http.NewRequestWithContext(ctx, http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	forward, forwardErr := s.gateway.Forward(ctx, c, account, body)
	if forward != nil {
		result.RequestID, result.ReportedModel, result.Endpoint = forward.RequestID, forward.UpstreamResponseModel, forward.UpstreamEndpoint
		if forward.UpstreamModel != "" {
			result.UpstreamModel = forward.UpstreamModel
		}
		if forward.ReasoningEffort != nil {
			result.EffectiveEffort = *forward.ReasoningEffort
		} else {
			result.EffectiveEffort = ""
		}
	}
	if forwardErr != nil || writer.status >= 400 || writer.overflow || ctx.Err() != nil {
		result.Error = modelEvaluationError(ctx, forwardErr, writer.status, writer.overflow)
		return result, nil
	}
	parseModelEvaluationResponse(writer.body.Bytes(), result)
	if len(originalUsage) > 0 {
		setModelEvaluationUsage(gjson.ParseBytes(originalUsage), result)
	}
	return result, nil
}

func modelEvaluationTextModel(model string) bool {
	model = strings.ToLower(model)
	for _, prefix := range []string{"gpt-image", "dall-e", "whisper", "tts-", "text-embedding", "omni-moderation", "sora"} {
		if strings.HasPrefix(model, prefix) {
			return false
		}
	}
	return !strings.Contains(model, "realtime") && !strings.Contains(model, "audio")
}

func modelEvaluationError(ctx context.Context, err error, status int, overflow bool) string {
	if ctx.Err() != nil {
		return "评测已取消或超过 180 秒；请降低推理强度后重试"
	}
	if overflow {
		return "回答超过 2 MiB；请降低推理强度后重试"
	}
	var upstream *UpstreamFailoverError
	if errors.As(err, &upstream) {
		status = upstream.StatusCode
	}
	if status >= 400 {
		return fmt.Sprintf("上游请求失败（HTTP %d）；请检查账号授权、额度、模型与代理后重试", status)
	}
	// Upstream errors can contain credentials, proxy URLs or response bodies.
	return "上游请求失败或回答不完整；请检查账号连通性后重试"
}

type modelEvaluationWriter struct {
	header   http.Header
	body     bytes.Buffer
	status   int
	overflow bool
}

func (w *modelEvaluationWriter) Header() http.Header    { return w.header }
func (w *modelEvaluationWriter) WriteHeader(status int) { w.status = status }
func (w *modelEvaluationWriter) Write(p []byte) (int, error) {
	if w.body.Len()+len(p) > 2<<20 {
		w.overflow = true
		return 0, errors.New("evaluation response too large")
	}
	return w.body.Write(p)
}
func (w *modelEvaluationWriter) Flush() {}
