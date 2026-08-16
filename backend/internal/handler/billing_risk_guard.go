package handler

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/tidwall/gjson"
)

type billingRiskLeaseState uint8

type billingRiskLeaseContextKey struct{}

const (
	billingRiskLeaseOpen billingRiskLeaseState = iota
	billingRiskLeaseHandedOff
	billingRiskLeaseReleased
	billingRiskLeaseUncertain
)

// billingRiskLease 只管理 handler 到 usage worker 交接前的许可生命周期。
type billingRiskLease struct {
	permit *service.BillingRiskPermit
	mu     sync.Mutex
	state  billingRiskLeaseState
	stop   chan struct{}
	wg     sync.WaitGroup
}

type billingRiskTurnLeases struct {
	mu     sync.Mutex
	leases map[int]*billingRiskLease
	closed bool
}

func newBillingRiskTurnLeases() *billingRiskTurnLeases {
	return &billingRiskTurnLeases{leases: make(map[int]*billingRiskLease)}
}

func (r *billingRiskTurnLeases) Store(turn int, lease *billingRiskLease) {
	if r == nil || turn <= 0 || lease == nil {
		return
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		lease.Close(context.Background())
		return
	}
	previous := r.leases[turn]
	r.leases[turn] = lease
	r.mu.Unlock()
	if previous != nil && previous != lease {
		previous.Close(context.Background())
	}
}

func (r *billingRiskTurnLeases) Has(turn int) bool {
	if r == nil || turn <= 0 {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.leases[turn] != nil
}

func (r *billingRiskTurnLeases) Take(turn int) *billingRiskLease {
	if r == nil || turn <= 0 {
		return nil
	}
	r.mu.Lock()
	lease := r.leases[turn]
	delete(r.leases, turn)
	r.mu.Unlock()
	return lease
}

func (r *billingRiskTurnLeases) CloseAll(ctx context.Context) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.closed = true
	leases := make([]*billingRiskLease, 0, len(r.leases))
	for turn, lease := range r.leases {
		delete(r.leases, turn)
		leases = append(leases, lease)
	}
	r.mu.Unlock()
	for _, lease := range leases {
		lease.Close(ctx)
	}
}

func newBillingRiskLease(permit *service.BillingRiskPermit) *billingRiskLease {
	if permit == nil {
		return nil
	}
	lease := &billingRiskLease{
		permit: permit,
		state:  billingRiskLeaseOpen,
		stop:   make(chan struct{}),
	}
	if permit.RefreshInterval > 0 {
		lease.wg.Add(1)
		go lease.refreshLoop()
	}
	return lease
}

func (l *billingRiskLease) refreshLoop() {
	defer l.wg.Done()
	ticker := time.NewTicker(l.permit.RefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), min(l.permit.RefreshInterval, 3*time.Second))
			err := l.permit.Refresh(ctx)
			cancel()
			if err != nil {
				slog.Warn("余额风险租约续期失败", "user_id", l.permit.UserID, "lease_id", l.permit.LeaseID, "error", err)
				if errors.Is(err, service.ErrBillingRiskLeaseLost) {
					return
				}
			}
		}
	}
}

func (l *billingRiskLease) stopRefreshLocked(next billingRiskLeaseState) bool {
	if l == nil || l.state != billingRiskLeaseOpen {
		return false
	}
	l.state = next
	close(l.stop)
	return true
}

// Handoff 把 Permit 交给最终 usage 账务；之后 Close 不再释放。
func (l *billingRiskLease) Handoff() *service.BillingRiskPermit {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	changed := false
	var permit *service.BillingRiskPermit
	switch l.state {
	case billingRiskLeaseOpen:
		changed = l.stopRefreshLocked(billingRiskLeaseHandedOff)
		permit = l.permit
	case billingRiskLeaseHandedOff:
		permit = l.permit
	}
	l.mu.Unlock()
	if changed {
		l.wg.Wait()
	}
	return permit
}

func handoffBillingRiskLeaseForCyberUsage(l *billingRiskLease, asyncBillingWillRun bool) *service.BillingRiskPermit {
	if !asyncBillingWillRun || l == nil {
		return nil
	}
	return l.Handoff()
}

// Close 释放尚未访问成功上游、也未移交账务的许可。
func (l *billingRiskLease) Close(ctx context.Context) {
	if l == nil {
		return
	}
	l.mu.Lock()
	changed := l.stopRefreshLocked(billingRiskLeaseReleased)
	permit := l.permit
	l.mu.Unlock()
	if !changed {
		return
	}
	l.wg.Wait()
	lifecycleCtx, cancel := billingRiskLifecycleContext(ctx)
	defer cancel()
	if err := permit.Release(lifecycleCtx); err != nil {
		slog.Warn("释放余额风险租约失败", "user_id", permit.UserID, "lease_id", permit.LeaseID, "error", err)
	}
}

func (l *billingRiskLease) MarkUncertain(ctx context.Context) {
	if l == nil {
		return
	}
	l.mu.Lock()
	changed := l.stopRefreshLocked(billingRiskLeaseUncertain)
	permit := l.permit
	l.mu.Unlock()
	if !changed {
		return
	}
	l.wg.Wait()
	lifecycleCtx, cancel := billingRiskLifecycleContext(ctx)
	defer cancel()
	if err := permit.MarkUncertain(lifecycleCtx); err != nil {
		slog.Warn("余额风险租约转入异常冷却失败", "user_id", permit.UserID, "lease_id", permit.LeaseID, "error", err)
	}
}

func billingRiskLifecycleContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	} else {
		parent = context.WithoutCancel(parent)
	}
	return context.WithTimeout(parent, 3*time.Second)
}

func billingRiskAdmissionModel(mapping service.ChannelMappingResult, requestedModel string) (string, bool) {
	requestedModel = strings.TrimSpace(requestedModel)
	mappedModel := requestedModel
	if mapping.Mapped && strings.TrimSpace(mapping.MappedModel) != "" {
		mappedModel = strings.TrimSpace(mapping.MappedModel)
	}
	switch strings.TrimSpace(mapping.BillingModelSource) {
	case service.BillingModelSourceRequested:
		return requestedModel, false
	case service.BillingModelSourceUpstream, service.BillingModelSourceResponse:
		return mappedModel, true
	case "", service.BillingModelSourceChannelMapped:
		return mappedModel, false
	default:
		return mappedModel, true
	}
}

func billingRiskAdmissionInputForMapping(
	input service.BillingRiskAdmissionInput,
	mapping service.ChannelMappingResult,
	requestedModel string,
) service.BillingRiskAdmissionInput {
	model, uncertain := billingRiskAdmissionModel(mapping, requestedModel)
	input.BillingModel = model
	input.ConservativeUnknown = input.ConservativeUnknown || uncertain
	return input
}

func billingRiskInputTokens(body []byte) int {
	if len(body) == 0 {
		return 0
	}
	// 3 bytes/token 对英文和中文请求体都偏保守，且只在受保护路径计算。
	return (len(body) + 2) / 3
}

func billingRiskMaxOutputTokens(body []byte, parsedMax int) int {
	if parsedMax > 0 {
		return parsedMax
	}
	for _, path := range []string{"max_completion_tokens", "max_output_tokens", "max_tokens", "generationConfig.maxOutputTokens"} {
		value := gjson.GetBytes(body, path)
		if !value.Exists() || value.Type != gjson.Number {
			continue
		}
		n := value.Int()
		if n > 0 && n <= math.MaxInt && value.Float() == float64(n) {
			return int(n)
		}
	}
	return 0
}

func billingRiskHasBillableSearchTool(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return false
	}
	found := false
	tools.ForEach(func(_, tool gjson.Result) bool {
		toolType := strings.ToLower(strings.TrimSpace(tool.Get("type").String()))
		name := strings.ToLower(strings.TrimSpace(tool.Get("name").String()))
		if name == "" {
			name = strings.ToLower(strings.TrimSpace(tool.Get("function.name").String()))
		}
		switch toolType {
		case "web_search", "x_search", "tool_search":
			found = true
		case "", "function", "custom":
			found = name == "web_search" || name == "x_search" || name == "tool_search"
		}
		return !found
	})
	return found
}

func billingRiskHasImageInput(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	var visit func(gjson.Result) bool
	visit = func(value gjson.Result) bool {
		if value.IsArray() {
			found := false
			value.ForEach(func(_, child gjson.Result) bool {
				found = visit(child)
				return !found
			})
			return found
		}
		if !value.IsObject() {
			return false
		}
		itemType := strings.ToLower(strings.TrimSpace(value.Get("type").String()))
		if itemType == "image" || itemType == "image_url" || itemType == "input_image" {
			if billingRiskHasNonEmptyURL(value.Get("url")) || billingRiskHasNonEmptyURL(value.Get("image_url")) {
				return true
			}
		}
		found := false
		value.ForEach(func(key, child gjson.Result) bool {
			if strings.EqualFold(strings.TrimSpace(key.String()), "image_url") && billingRiskHasNonEmptyURL(child) {
				found = true
				return false
			}
			found = visit(child)
			return !found
		})
		return found
	}
	return visit(gjson.ParseBytes(body))
}

func billingRiskHasNonEmptyURL(value gjson.Result) bool {
	if value.Type == gjson.String {
		return strings.TrimSpace(value.String()) != ""
	}
	if value.IsObject() {
		return strings.TrimSpace(value.Get("url").String()) != ""
	}
	return false
}

func geminiBillingRiskAdmissionInput(
	apiKey *service.APIKey,
	subscription *service.UserSubscription,
	mapping service.ChannelMappingResult,
	requestedModel string,
	body []byte,
	pricingAt time.Time,
) service.BillingRiskAdmissionInput {
	billingModel, uncertainModel := billingRiskAdmissionModel(mapping, requestedModel)
	kind := service.BillingRiskRequestText
	sizeTier := ""
	intentModel := billingModel
	if mapping.Mapped && strings.TrimSpace(mapping.MappedModel) != "" {
		intentModel = strings.TrimSpace(mapping.MappedModel)
	}
	imageIntent := service.IsGeminiImageGenerationRequest(requestedModel, intentModel, body)
	if imageIntent {
		kind = service.BillingRiskRequestSyncImage
		for _, path := range []string{"generationConfig.imageConfig.imageSize", "generation_config.image_config.image_size"} {
			sizeTier = strings.TrimSpace(gjson.GetBytes(body, path).String())
			if sizeTier != "" {
				break
			}
		}
	}
	return service.BillingRiskAdmissionInput{
		APIKey:                apiKey,
		Subscription:          subscription,
		Kind:                  kind,
		BillingModel:          billingModel,
		InputTokens:           billingRiskInputTokens(body),
		MaxOutputTokens:       billingRiskMaxOutputTokens(body, 0),
		RequestCount:          1,
		SizeTier:              sizeTier,
		PricingAt:             pricingAt,
		LongContextThreshold:  200_000,
		LongContextMultiplier: 2,
		ConservativeUnknown:   uncertainModel || imageIntent,
	}
}

func billingRiskErrorDetails(err error) (status int, code, message string) {
	status = infraerrors.Code(err)
	message = infraerrors.Message(err)
	if message == "" {
		message = "余额风险保护暂时不可用，请稍后重试"
	}
	switch status {
	case http.StatusTooManyRequests:
		return status, "rate_limit_error", message
	case http.StatusServiceUnavailable:
		return status, "billing_service_error", message
	default:
		return status, "billing_error", message
	}
}

func acquireBillingRiskLease(
	ctx context.Context,
	admission *service.BillingRiskAdmissionService,
	input service.BillingRiskAdmissionInput,
) (*billingRiskLease, error) {
	if lease := billingRiskLeaseFromContext(ctx); lease != nil {
		return lease, nil
	}
	if admission == nil {
		return nil, nil
	}
	permit, err := admission.Acquire(ctx, input)
	if err != nil {
		return nil, err
	}
	return newBillingRiskLease(permit), nil
}

func withBillingRiskLease(ctx context.Context, lease *billingRiskLease) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if lease == nil {
		return ctx
	}
	return context.WithValue(ctx, billingRiskLeaseContextKey{}, lease)
}

func billingRiskLeaseFromContext(ctx context.Context) *billingRiskLease {
	if ctx == nil {
		return nil
	}
	lease, _ := ctx.Value(billingRiskLeaseContextKey{}).(*billingRiskLease)
	return lease
}

func billingRiskServiceTier(body []byte) string {
	return strings.TrimSpace(gjson.GetBytes(body, "service_tier").String())
}

func wrapBillingRiskUsageRecordTask(permit *service.BillingRiskPermit, task service.UsageRecordTask) service.UsageRecordTask {
	wrapped, _ := wrapBillingRiskUsageRecordTaskWithAbandon(permit, task)
	return wrapped
}

func wrapBillingRiskUsageRecordTaskWithAbandon(
	permit *service.BillingRiskPermit,
	task service.UsageRecordTask,
) (service.UsageRecordTask, service.UsageRecordTask) {
	if permit == nil || task == nil {
		return task, nil
	}
	stopRefresh := func() {}
	if permit.RefreshInterval > 0 {
		stop := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			ticker := time.NewTicker(permit.RefreshInterval)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					ctx, cancel := context.WithTimeout(context.Background(), min(permit.RefreshInterval, 3*time.Second))
					err := permit.Refresh(ctx)
					cancel()
					if err != nil {
						slog.Warn("usage 账务等待期间续期余额风险租约失败", "user_id", permit.UserID, "lease_id", permit.LeaseID, "error", err)
						if errors.Is(err, service.ErrBillingRiskLeaseLost) {
							return
						}
					}
				}
			}
		}()
		var stopOnce sync.Once
		stopRefresh = func() {
			stopOnce.Do(func() {
				close(stop)
				<-done
			})
		}
	}

	var lifecycleMu sync.Mutex
	started := false
	abandoned := false
	wrapped := func(ctx context.Context) {
		lifecycleMu.Lock()
		if abandoned {
			lifecycleMu.Unlock()
			return
		}
		started = true
		lifecycleMu.Unlock()
		defer stopRefresh()
		task(ctx)
	}
	abandon := func(ctx context.Context) {
		lifecycleMu.Lock()
		if started || abandoned {
			lifecycleMu.Unlock()
			return
		}
		abandoned = true
		lifecycleMu.Unlock()
		stopRefresh()
		lifecycleCtx, cancel := billingRiskLifecycleContext(ctx)
		defer cancel()
		if err := permit.MarkUncertain(lifecycleCtx); err != nil {
			slog.Warn("放弃 usage 账务任务时转入余额风险异常冷却失败", "user_id", permit.UserID, "lease_id", permit.LeaseID, "error", err)
		}
	}
	return wrapped, abandon
}

func (h *GatewayHandler) submitBillingRiskUsageRecordTask(parent context.Context, permit *service.BillingRiskPermit, task service.UsageRecordTask) {
	if permit != nil {
		h.submitMandatoryUsageRecordTask(parent, wrapBillingRiskUsageRecordTask(permit, task))
		return
	}
	h.submitUsageRecordTask(parent, task)
}

func (h *OpenAIGatewayHandler) submitBillingRiskUsageRecordTask(
	parent context.Context,
	permit *service.BillingRiskPermit,
	result *service.OpenAIForwardResult,
	task service.UsageRecordTask,
) {
	if permit != nil {
		wrapped, abandon := wrapBillingRiskUsageRecordTaskWithAbandon(permit, task)
		if collectAsyncImageUsageTaskWithAbandon(parent, wrapUsageRecordTaskContext(parent, wrapped), abandon) {
			return
		}
		h.submitMandatoryUsageRecordTask(parent, wrapped)
		return
	}
	h.submitOpenAIUsageRecordTask(parent, result, task)
}
