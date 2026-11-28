package handler

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const imageSafetyRecordTimeout = 30 * time.Second

// recordOpenAIImagesSafetyRefusal 在图像上游以安全系统拒绝终结请求时，异步把事件
// 写入风控中心。拒绝已经返回给用户，这里只做留证，不影响响应。
func (h *OpenAIGatewayHandler) recordOpenAIImagesSafetyRefusal(c *gin.Context, apiKey *service.APIKey, account *service.Account, model string, prompt string, upErr *service.OpenAIImagesUpstreamError) {
	cmSvc := h.contentModerationService
	if cmSvc == nil || c == nil || !service.IsOpenAIImagesSafetyRefusal(upErr) {
		return
	}
	// 在 gin.Context 回收前把需要的值拍成标量。
	// request_id 取 context 中网关自己的 ID（与运维错误日志、转发前审核日志同源）：
	// 响应头里的 X-Request-Id 属于可透传头，可能已被上游的 x-request-id 覆盖。
	requestID := ""
	if c.Request != nil {
		requestID = contentModerationRequestID(c.Request.Context())
	}
	if requestID == "" {
		requestID = c.Writer.Header().Get("X-Request-Id")
	}
	in := service.ImageSafetyRecordInput{
		RequestID:         requestID,
		Endpoint:          GetInboundEndpoint(c),
		Model:             model,
		Prompt:            prompt,
		UpstreamStatus:    upErr.StatusCode,
		UpstreamType:      upErr.ErrorType,
		UpstreamCode:      upErr.Code,
		UpstreamMessage:   upErr.Message,
		UpstreamRequestID: upErr.UpstreamRequestID,
	}
	if apiKey != nil {
		in.APIKeyID = apiKey.ID
		in.APIKeyName = apiKey.Name
		in.GroupID = apiKey.GroupID
		if apiKey.User != nil {
			in.UserID = apiKey.User.ID
			in.UserEmail = apiKey.User.Email
		}
		if apiKey.Group != nil {
			in.GroupName = apiKey.Group.Name
		}
	}
	if account != nil {
		in.Provider = account.Platform
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), imageSafetyRecordTimeout)
		defer cancel()
		cmSvc.RecordImageSafetyEvent(ctx, in)
	}()
}
