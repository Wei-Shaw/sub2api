package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/minimax"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func (h *OpenAIGatewayHandler) miniMaxMediaKey(c *gin.Context) (*service.APIKey, bool) {
	key, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || key == nil || key.Group == nil || h.minimaxMedia == nil {
		h.errorResponse(c, 503, "api_error", "MiniMax media is unavailable")
		return nil, false
	}
	if key.Group.Platform != service.PlatformMiniMax && key.Group.Platform != service.PlatformComposite {
		h.errorResponse(c, 404, "not_found_error", "MiniMax media is not supported by this group")
		return nil, false
	}
	if target, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context()); ok && target != service.PlatformMiniMax {
		h.errorResponse(c, 400, "invalid_request_error", "Media route must target MiniMax")
		return nil, false
	}
	return key, true
}

// MiniMaxMedia accepts the supported native media protocols. The request is
// submitted once, after a durable reservation has committed.
func (h *OpenAIGatewayHandler) MiniMaxMedia(c *gin.Context, kind string) {
	startedAt := time.Now()
	key, ok := h.miniMaxMediaKey(c)
	if !ok {
		return
	}
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		h.errorResponse(c, 400, "invalid_request_error", "Unable to read media request")
		return
	}
	previous, err := h.minimaxMedia.Replay(c.Request.Context(), key, kind, c.GetHeader("Idempotency-Key"), body)
	if err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, service.ErrMiniMaxMediaConflict) {
			status = http.StatusConflict
		}
		h.errorResponse(c, status, "invalid_request_error", err.Error())
		return
	}
	if previous != nil {
		miniMaxReplayResponse(c, previous)
		return
	}
	req, err := minimax.ParseRequest(body, kind)
	if err != nil {
		h.errorResponse(c, 400, "invalid_request_error", err.Error())
		return
	}
	if err = h.minimaxMedia.ValidateRequest(c.Request.Context(), key.Group, req.Model); err != nil {
		h.errorResponse(c, 400, "invalid_request_error", err.Error())
		return
	}
	if (kind == "image" || kind == "video") && !service.GroupAllowsImageGeneration(key.Group) {
		h.errorResponse(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
		return
	}
	if _, err = h.minimaxMedia.Price(key.Group, req.Model, req.Resolution); err != nil {
		h.errorResponse(c, 400, "invalid_request_error", err.Error())
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, 401, "authentication_error", "Authentication required")
		return
	}
	setOpsRequestContext(c, req.Model, req.Stream)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(req.Stream, false)))
	reqLog := requestLogger(c, "handler.minimax_media")
	auditBody, _ := json.Marshal(map[string]string{"prompt": req.Prompt()})
	if decision := h.checkSecurityAudit(c, reqLog, key, subject, service.ContentModerationProtocolOpenAIImages, req.Model, auditBody); decision != nil && !decision.AllowNextStage {
		h.openAISecurityAuditError(c, decision)
		return
	}
	sub, _ := middleware2.GetSubscriptionFromContext(c)
	if err = h.billingCacheService.CheckBillingEligibility(c.Request.Context(), key.User, key, key.Group, sub, service.PlatformMiniMax); err != nil {
		status, code, message, _ := billingErrorDetails(err)
		h.errorResponse(c, status, code, message)
		return
	}
	userSlot, err := h.minimaxMedia.AcquireUser(c.Request.Context(), subject.UserID, subject.Concurrency)
	if err != nil || userSlot == nil || !userSlot.Acquired {
		h.errorResponse(c, 429, "rate_limit_error", "User concurrency limit reached")
		return
	}
	defer userSlot.ReleaseFunc()
	account, releaseAccount, err := h.minimaxMedia.SelectRequest(c.Request.Context(), key, req, kind)
	if err != nil {
		h.errorResponse(c, 503, "api_error", err.Error())
		return
	}
	defer releaseAccount()
	setOpsSelectedAccount(c, account.ID, account.Platform)

	task, created, err := h.minimaxMedia.Reserve(c.Request.Context(), key, sub, account, req, kind, c.GetHeader("Idempotency-Key"), body, service.MiniMaxMediaRequestInfo{StartedAt: startedAt, UserAgent: c.Request.UserAgent(), IPAddress: c.ClientIP()})
	if err != nil {
		status := 400
		if errors.Is(err, service.ErrMiniMaxMediaFunds) {
			status = 402
		}
		if errors.Is(err, service.ErrMiniMaxMediaConflict) {
			status = 409
		}
		h.errorResponse(c, status, "invalid_request_error", err.Error())
		return
	}
	c.Header("X-Sub2API-Task-ID", task.ID)
	if !created {
		miniMaxReplayResponse(c, task)
		return
	}
	// After the hold commits, client disconnects must not cancel receipt reading.
	// Bound detached work to five minutes, and drain at most 60 seconds after a
	// disconnect. Shutdown cancellation still terminates it.
	callCtx, cancel := h.minimaxMedia.Detached(5 * time.Minute)
	defer cancel()
	stopDisconnect := context.AfterFunc(c.Request.Context(), func() { time.AfterFunc(60*time.Second, cancel) })
	defer stopDisconnect()
	path := "/v1/t2a_v2"
	switch kind {
	case "video":
		path = "/v2/video_generation"
	case "image":
		path = "/v1/image_generation"
	case "voice_clone":
		path = "/v1/voice_clone"
	case "voice_design":
		path = "/v1/voice_design"
	}
	if kind == "speech" && req.Stream {
		// The final packet must carry the meter, not a second copy of all audio.
		body, err = sjson.SetBytes(body, "stream_options.exclude_aggregated_audio", true)
		if err != nil {
			_ = h.minimaxMedia.Complete(callCtx, task, "failed", minimax.Usage{}, nil)
			h.errorResponse(c, 400, "invalid_request_error", "Invalid stream options")
			return
		}
	}
	resp, err := h.minimaxMedia.Do(callCtx, account, task.Origin, http.MethodPost, path, body)
	if err != nil {
		h.minimaxMedia.Unknown(callCtx, task)
		h.errorResponse(c, 502, "upstream_error", "MiniMax submission outcome is unknown; inspect the task receipt and do not resubmit")
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			_ = h.minimaxMedia.Complete(callCtx, task, "failed", minimax.Usage{}, nil)
		} else {
			h.minimaxMedia.Unknown(callCtx, task)
		}
		h.errorResponse(c, 502, "upstream_error", "MiniMax rejected the media request; inspect the task receipt")
		return
	}
	if kind == "speech" && req.Stream && strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		h.miniMaxSpeechStream(c, callCtx, task, resp.Body)
		return
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20+1))
	if err != nil || len(data) > 64<<20 {
		h.minimaxMedia.Unknown(callCtx, task)
		h.errorResponse(c, 502, "upstream_error", "Incomplete MiniMax media response; billing is pending reconciliation")
		return
	}
	if kind == "video" {
		var result struct {
			TaskID string `json:"task_id"`
		}
		if json.Unmarshal(data, &result) != nil || result.TaskID == "" {
			h.minimaxMedia.Unknown(callCtx, task)
			h.errorResponse(c, 502, "upstream_error", "MiniMax task receipt is missing; do not resubmit")
			return
		}
		if err = h.minimaxMedia.Submitted(callCtx, task, result.TaskID); err != nil {
			h.errorResponse(c, 503, "api_error", "Task submitted; receipt persistence needs reconciliation. Do not resubmit")
			return
		}
		c.JSON(200, gin.H{"task_id": task.ID})
		return
	}
	u, final, meterErr := minimax.SpeechUsage(data)
	meterSource := ""
	if kind == "image" {
		u, meterErr = minimax.ImageUsage(data)
		final = meterErr == nil
		if u.OutputImages > req.N {
			meterErr = errors.New("image count exceeds request")
			final = false
		}
	}
	if kind == "voice_clone" || kind == "voice_design" {
		u, meterErr = minimax.VoicePreviewUsage(data, req.Text != "" || req.PreviewText != "")
		if kind == "voice_design" {
			u, meterSource, meterErr = minimax.VoiceDesignUsage(data, req.PreviewText)
		}
		final = meterErr == nil
		if final {
			voiceID := req.VoiceID
			if kind == "voice_design" {
				voiceID = gjson.GetBytes(data, "voice_id").String()
			}
			if voiceID == "" {
				meterErr = errors.New("missing created voice ID")
				final = false
			} else if err = h.minimaxMedia.SaveAsset(callCtx, &service.MiniMaxMediaAsset{Kind: "voice", ID: voiceID, Category: kind, UserID: key.UserID, APIKeyID: key.ID, AccountID: account.ID}); err != nil {
				meterErr = err
				final = false
			}
		}
	}

	if meterErr == nil && final {
		if err = h.minimaxMedia.Complete(callCtx, task, "succeeded", u, nil, meterSource); err != nil {
			c.Header("X-Sub2API-Billing-State", "pending")
		} else {
			c.Header("X-Sub2API-Billing-State", "settled")
		}
	} else if miniMaxSpeechRejected(data) {
		_ = h.minimaxMedia.Complete(callCtx, task, "failed", minimax.Usage{}, nil)
	} else {
		h.minimaxMedia.Unknown(callCtx, task)
		c.Header("X-Sub2API-Billing-State", "pending")
	}
	c.Data(200, "application/json", data)
}
func miniMaxReplayResponse(c *gin.Context, task *service.MiniMaxMediaTask) {
	c.Header("X-Sub2API-Task-ID", task.ID)
	if task.Kind == "video" {
		c.JSON(http.StatusOK, gin.H{"task_id": task.ID, "sub2api_billing": mediaPublicTask(task)})
		return
	}
	c.JSON(http.StatusConflict, gin.H{"error": gin.H{"type": "idempotency_replay", "message": "This media request was already submitted; inspect the media task receipt", "task_id": task.ID}})
}

func miniMaxSpeechRejected(data []byte) bool {
	code := gjson.GetBytes(data, "base_resp.status_code")
	return code.Exists() && code.Int() != 0 && gjson.GetBytes(data, "data.audio").String() == "" && gjson.GetBytes(data, "extra_info.usage_characters").Int() == 0 && gjson.GetBytes(data, "extra_info.usage_voice_count").Int() == 0 && gjson.GetBytes(data, "metadata.success_count").Int() == 0 && gjson.GetBytes(data, "trial_audio").String() == "" && gjson.GetBytes(data, "demo_audio").String() == ""
}
func (h *OpenAIGatewayHandler) miniMaxSpeechStream(c *gin.Context, ctx context.Context, t *service.MiniMaxMediaTask, reader io.Reader) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 16<<20)
	complete, audioSeen, clientGone := false, false, false
	for scanner.Scan() {
		line := scanner.Bytes()
		if bytes.HasPrefix(line, []byte("data:")) {
			data := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
			if len(data) > 0 && !bytes.Equal(data, []byte("[DONE]")) {
				u, final, err := minimax.SpeechUsage(data)
				if final && complete {
					continue
				}
				if final && err == nil && !complete {
					// Receipt is stored before final SSE is delivered. A ledger failure is
					// recoverable without asking the provider to generate again.
					_ = h.minimaxMedia.Complete(ctx, t, "succeeded", u, nil)
					complete = true
					if audioSeen {
						if clean, e := sjson.DeleteBytes(data, "data.audio"); e == nil {
							line = append([]byte("data: "), clean...)
						}
					}
				}
				if gjson.GetBytes(data, "data.audio").String() != "" {
					audioSeen = true
				}
				if !audioSeen && miniMaxSpeechRejected(data) && !complete {
					_ = h.minimaxMedia.Complete(ctx, t, "failed", minimax.Usage{}, nil)
					complete = true
				}
			}
		}
		if !clientGone {
			_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(15 * time.Second))
			if _, err := c.Writer.Write(append(append([]byte(nil), line...), '\n')); err != nil {
				clientGone = true
			} else {
				c.Writer.Flush()
			}
		}
	}
	if !complete {
		h.minimaxMedia.Unknown(ctx, t)
	}
}
func (h *OpenAIGatewayHandler) MiniMaxVoices(c *gin.Context) {
	key, ok := h.miniMaxMediaKey(c)
	if !ok {
		return
	}
	voiceType := "system"
	if c.Request.Body != nil {
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 4097))
		if err != nil || len(body) > 4096 {
			h.errorResponse(c, 400, "invalid_request_error", "Invalid voice list request")
			return
		}
		if len(bytes.TrimSpace(body)) > 0 {
			var r struct {
				Type string `json:"voice_type"`
			}
			if minimax.Decode(body, &r) != nil {
				h.errorResponse(c, 400, "invalid_request_error", "Invalid voice list request")
				return
			}
			if r.Type != "" {
				voiceType = r.Type
			}
		}
	}
	if voiceType != "system" && voiceType != "all" && voiceType != "voice_cloning" && voiceType != "voice_generation" {
		h.errorResponse(c, 400, "invalid_request_error", "Unsupported voice_type")
		return
	}
	account, release, err := h.minimaxMedia.Select(c.Request.Context(), *key.GroupID)
	if err != nil {
		h.errorResponse(c, 503, "api_error", err.Error())
		return
	}
	defer release()
	body, _, err := h.minimaxMedia.SystemVoices(c.Request.Context(), account)
	if err != nil {
		h.errorResponse(c, 502, "upstream_error", err.Error())
		return
	}
	body, err = h.minimaxMedia.OwnedVoices(c.Request.Context(), key, body, voiceType)
	if err != nil {
		h.errorResponse(c, 502, "upstream_error", err.Error())
		return
	}
	c.Data(200, "application/json", body)
}

// This route only reads local, owner-scoped data, so billing eligibility is
// deliberately not checked again after a generation has consumed the balance.
func (h *OpenAIGatewayHandler) MiniMaxTask(c *gin.Context) {
	key, ok := h.miniMaxMediaKey(c)
	if !ok {
		return
	}
	id := c.Param("task_id")
	if id == "" {
		tasks, err := h.minimaxMedia.List(c.Request.Context(), key.UserID, key.ID)
		if err != nil {
			h.errorResponse(c, 500, "api_error", "Unable to read media tasks")
			return
		}
		items := []any{}
		for _, t := range tasks {
			if t.Kind == "video" || strings.HasPrefix(c.Request.URL.Path, "/v1/media/tasks") {
				items = append(items, mediaPublicTask(t))
			}
		}
		c.JSON(200, gin.H{"tasks": items})
		return
	}
	t, err := h.minimaxMedia.Get(c.Request.Context(), id, key.UserID, key.ID)
	if err != nil {
		h.errorResponse(c, 404, "not_found_error", "Media task not found")
		return
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v2/") {
		t = h.minimaxMedia.RefreshCompleted(c.Request.Context(), t)
		var task map[string]any
		_ = json.Unmarshal(t.Result, &task)
		if task == nil {
			task = map[string]any{"id": t.ID, "model": t.Model, "status": t.Status}
		}
		task["sub2api_billing"] = mediaPublicTask(t)
		c.JSON(200, gin.H{"task": task})
		return
	}
	c.JSON(200, mediaPublicTask(t))
}
func mediaPublicTask(t *service.MiniMaxMediaTask) gin.H {
	// Retail prices and usage are public; supplier costs, account identity and
	// exchange-rate/margin configuration are only for operators.
	var bill any
	if t.Bill != nil {
		bill = gin.H{"usage": t.Bill.Usage, "meter_source": t.Bill.MeterSource, "reconciliation_required": t.Bill.ReconciliationRequired, "billable_images": t.Bill.BillableImages, "retail_usd": t.Bill.RetailUSD, "charged_usd": t.Bill.ChargedUSD}
	}
	return gin.H{"task_id": t.ID, "kind": t.Kind, "model": t.Model, "resolution": t.Resolution, "duration": t.Duration, "status": t.Status, "billing_state": t.BillingState, "reserved_usd": t.ReservedUSD, "billing": bill, "unit_prices_usd": t.Pricing.Retail, "multiplier": t.Pricing.Multiplier, "created_at": t.CreatedAt}
}

func (h *OpenAIGatewayHandler) MiniMaxBillingDetail(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 || h.minimaxMedia == nil {
		response.NotFound(c, "Media task not found")
		return
	}
	t, err := h.minimaxMedia.Get(c.Request.Context(), c.Param("task_id"), subject.UserID, 0)
	if err != nil {
		response.NotFound(c, "Media task not found")
		return
	}
	response.Success(c, mediaPublicTask(t))
}

// Registered exclusively under the administrator middleware chain.
func (h *OpenAIGatewayHandler) AdminMiniMaxBillingDetail(c *gin.Context) {
	if h.minimaxMedia == nil {
		response.NotFound(c, "Media task not found")
		return
	}
	if id := c.Param("task_id"); id != "" {
		t, err := h.minimaxMedia.Get(c.Request.Context(), id, 0, 0)
		if err != nil {
			response.NotFound(c, "Media task not found")
			return
		}
		response.Success(c, mediaAdminTask(t))
		return
	}
	tasks, err := h.minimaxMedia.List(c.Request.Context(), 0, 0)
	if err != nil {
		response.InternalError(c, "Unable to read media tasks")
		return
	}
	items := []any{}
	for _, t := range tasks {
		items = append(items, mediaAdminTask(t))
	}
	response.Success(c, items)
}
func mediaAdminTask(t *service.MiniMaxMediaTask) any {
	data := mediaPublicTask(t)
	data["account_id"], data["user_id"], data["api_key_id"], data["upstream_id"] = t.AccountID, t.UserID, t.APIKeyID, t.UpstreamID
	data["pricing"], data["billing"] = t.Pricing, t.Bill
	return data
}
