package admin

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// InvoiceHandler handles admin operations on invoice requests.
type InvoiceHandler struct {
	paymentService      *service.PaymentService
	emailService        *service.EmailService
	settingService      *service.SettingService
	notificationService *service.NotificationService
}

// NewInvoiceHandler creates a new admin InvoiceHandler.
func NewInvoiceHandler(
	paymentService *service.PaymentService,
	emailService *service.EmailService,
	settingService *service.SettingService,
	notificationService *service.NotificationService,
) *InvoiceHandler {
	return &InvoiceHandler{
		paymentService:      paymentService,
		emailService:        emailService,
		settingService:      settingService,
		notificationService: notificationService,
	}
}

// ListInvoiceRequests returns a paginated list of invoice requests across all users.
// GET /api/v1/admin/payment/invoices
func (h *InvoiceHandler) ListInvoiceRequests(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	var userID int64
	if uid := strings.TrimSpace(c.Query("user_id")); uid != "" {
		if v, err := strconv.ParseInt(uid, 10, 64); err == nil {
			userID = v
		}
	}
	params := service.AdminInvoiceListParams{
		Page:      page,
		PageSize:  pageSize,
		Status:    c.Query("status"),
		Keyword:   c.Query("keyword"),
		UserID:    userID,
		StartTime: c.Query("start_time"),
		EndTime:   c.Query("end_time"),
	}
	requests, total, err := h.paymentService.AdminListInvoiceRequests(c.Request.Context(), params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, requests, int64(total), page, pageSize)
}

// GetInvoiceRequest returns the detail of a single invoice request.
// GET /api/v1/admin/payment/invoices/:id
func (h *InvoiceHandler) GetInvoiceRequest(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	req, err := h.paymentService.GetInvoiceRequestForAdmin(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, req)
}

// RejectInvoiceRequest marks a pending invoice request as rejected.
// POST /api/v1/admin/payment/invoices/:id/reject
type rejectInvoiceRequestBody struct {
	Reason string `json:"reason"`
}

func (h *InvoiceHandler) RejectInvoiceRequest(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}
	var body rejectInvoiceRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	req, err := h.paymentService.RejectInvoiceRequest(c.Request.Context(), subject.UserID, id, body.Reason)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	go h.notifyInvoiceRejected(req)
	h.writeUserNotification(req, false)
	response.Success(c, req)
}

// CompleteInvoiceRequest accepts an uploaded invoice file (multipart) and marks the request as completed.
// POST /api/v1/admin/payment/invoices/:id/complete
//
// Multipart form fields:
//   - invoice_no (text): the real invoice number
//   - file (file):       the invoice PDF/image
func (h *InvoiceHandler) CompleteInvoiceRequest(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}

	invoiceNo := strings.TrimSpace(c.PostForm("invoice_no"))
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "Invoice file is required")
		return
	}

	req, err := h.paymentService.CompleteInvoiceRequest(c.Request.Context(), subject.UserID, id, service.CompleteInvoiceRequestInput{
		InvoiceNo: invoiceNo,
		File:      fileHeader,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.writeUserNotification(req, true)
	response.Success(c, req)
}

// writeUserNotification persists an in-app notification for the requester.
// Best-effort — failures are logged but not surfaced to the admin caller.
func (h *InvoiceHandler) writeUserNotification(req *service.InvoiceRequest, completed bool) {
	if req == nil || h.notificationService == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var title, body, link string
	if completed {
		title = "您的开票申请已完成"
		invoiceNo := ""
		if req.InvoiceNo != nil {
			invoiceNo = *req.InvoiceNo
		}
		body = fmt.Sprintf("申请单号 %s 已开具，发票号码：%s。", req.SerialNo, invoiceNo)
	} else {
		title = "您的开票申请已被拒绝"
		reason := ""
		if req.RejectReason != nil {
			reason = *req.RejectReason
		}
		body = fmt.Sprintf("申请单号 %s 未通过审核。原因：%s", req.SerialNo, reason)
	}
	link = "/invoice"

	if _, err := h.notificationService.Create(ctx, service.CreateUserNotificationInput{
		UserID:   req.UserID,
		Category: service.NotificationCategoryInvoice,
		Title:    title,
		Body:     body,
		Link:     link,
		Metadata: map[string]any{"invoice_request_id": req.ID, "serial_no": req.SerialNo},
	}); err != nil {
		slog.Warn("failed to write invoice notification", "user_id", req.UserID, "request_id", req.ID, "error", err)
	}
}

// --- Email notifications (best-effort, async) ---

func (h *InvoiceHandler) notifyInvoiceRejected(req *service.InvoiceRequest) {
	if h.emailService == nil || req == nil {
		return
	}
	to := strings.TrimSpace(req.ProfileSnapshot.Email)
	if to == "" {
		return
	}
	siteName := h.lookupSiteName()
	subject := fmt.Sprintf("[%s] 开票申请未通过 / Invoice Request Rejected", sanitizeHeader(siteName))
	reason := ""
	if req.RejectReason != nil {
		reason = *req.RejectReason
	}
	body := buildInvoiceRejectedBody(req, reason, siteName)
	h.dispatchEmail(to, subject, body)
}

func (h *InvoiceHandler) dispatchEmail(to, subject, body string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := h.emailService.SendEmail(ctx, to, subject, body); err != nil {
		slog.Warn("invoice notification email failed", "to", to, "error", err)
	}
}

func (h *InvoiceHandler) lookupSiteName() string {
	if h.settingService == nil {
		return "Sub2API"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if name := h.settingService.GetSiteName(ctx); strings.TrimSpace(name) != "" {
		return name
	}
	return "Sub2API"
}

func buildInvoiceRejectedBody(req *service.InvoiceRequest, reason, siteName string) string {
	serial := html.EscapeString(req.SerialNo)
	title := html.EscapeString(req.ProfileSnapshot.Title)
	reasonHTML := html.EscapeString(reason)
	site := html.EscapeString(siteName)
	return `<!DOCTYPE html><html><body style="font-family:Arial,sans-serif;color:#333;">
<p>您好，</p>
<p>您在 <strong>` + site + `</strong> 提交的开票申请未通过审核。</p>
<table cellpadding="6" style="border-collapse:collapse;font-size:14px;">
  <tr><td style="color:#666;">申请单号 / Serial</td><td><strong>` + serial + `</strong></td></tr>
  <tr><td style="color:#666;">发票抬头 / Title</td><td>` + title + `</td></tr>
  <tr><td style="color:#666;">拒绝原因 / Reason</td><td>` + reasonHTML + `</td></tr>
</table>
<p>对应的订单已自动恢复为可开票状态，请修改后重新提交。</p>
<p>Your invoice request was rejected. The associated orders are returned to the invoiceable pool.</p>
<hr style="border:none;border-top:1px solid #eee;">
<p style="font-size:12px;color:#999;">本邮件由系统自动发送，请勿直接回复。</p>
</body></html>`
}

func sanitizeHeader(value string) string {
	v := strings.ReplaceAll(value, "\r", " ")
	v = strings.ReplaceAll(v, "\n", " ")
	return strings.TrimSpace(v)
}

