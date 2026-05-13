package service

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"
)

// invoiceEmailSiteName resolves the site name for the invoice email, with a
// 3-second timeout and a "Sub2API" fallback.
func (s *PaymentService) invoiceEmailSiteName(ctx context.Context) string {
	if s == nil || s.invoiceSettingService == nil {
		return "Sub2API"
	}
	subCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if name := s.invoiceSettingService.GetSiteName(subCtx); strings.TrimSpace(name) != "" {
		return name
	}
	return "Sub2API"
}

// buildInvoiceAttachmentEmailBody renders the HTML body shown to customers
// when the invoice is delivered as an email attachment.
func buildInvoiceAttachmentEmailBody(req *InvoiceRequest, invoiceNo, siteName string) string {
	title := html.EscapeString(req.ProfileSnapshot.Title)
	serial := html.EscapeString(req.SerialNo)
	site := html.EscapeString(siteName)
	invoiceNoHTML := html.EscapeString(invoiceNo)
	return `<!DOCTYPE html><html><body style="font-family:Arial,sans-serif;color:#333;">
<p>您好，</p>
<p>您在 <strong>` + site + `</strong> 提交的开票申请已完成开具。<strong>发票文件已作为附件随本邮件发送</strong>，请查收。</p>
<table cellpadding="6" style="border-collapse:collapse;font-size:14px;">
  <tr><td style="color:#666;">申请单号 / Serial</td><td><strong>` + serial + `</strong></td></tr>
  <tr><td style="color:#666;">发票抬头 / Title</td><td>` + title + `</td></tr>
  <tr><td style="color:#666;">发票号码 / Invoice No.</td><td><strong>` + invoiceNoHTML + `</strong></td></tr>
  <tr><td style="color:#666;">开票金额 / Amount</td><td>` + fmt.Sprintf("%.2f", req.TotalAmount) + `</td></tr>
</table>
<p>如未看到附件，请检查邮件客户端的附件区域或垃圾邮件文件夹。</p>
<p>Your invoice has been issued and is attached to this email. If you do not see the attachment, please check your spam folder.</p>
<hr style="border:none;border-top:1px solid #eee;">
<p style="font-size:12px;color:#999;">本邮件由系统自动发送，请勿直接回复。</p>
</body></html>`
}
