package service

import "html"

// brandedEmailDocumentOptions contains already-safe HTML fragments. Callers
// must escape runtime values before passing them here. Notification templates
// pass placeholders which are escaped by renderNotificationEmail.
type brandedEmailDocumentOptions struct {
	Lang      string
	Brand     string
	Accent    string
	Eyebrow   string
	Title     string
	Content   string
	Footer    string
	Preheader string
}

func buildBrandedEmailDocument(options brandedEmailDocumentOptions) string {
	lang := options.Lang
	if lang == "" {
		lang = "en"
	}
	accent := options.Accent
	if accent == "" {
		accent = "#2563eb"
	}
	preheader := options.Preheader
	if preheader == "" {
		preheader = options.Title
	}

	return `<!DOCTYPE html>
<html lang="` + lang + `">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <meta name="color-scheme" content="light">
  <meta name="supported-color-schemes" content="light">
  <title>` + options.Title + `</title>
  <style>
    html, body { margin: 0 !important; padding: 0 !important; width: 100% !important; background: #f4f4f5; }
    body, table, td, a { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", Arial, sans-serif; }
    table { border-collapse: collapse; border-spacing: 0; }
    img { border: 0; display: block; }
    .preheader { display: none !important; max-height: 0; max-width: 0; opacity: 0; overflow: hidden; color: transparent; mso-hide: all; }
    .email-bg { width: 100%; background: #f4f4f5; }
    .email-shell { width: 100%; max-width: 680px; background: #ffffff; border: 1px solid #e4e4e7; border-radius: 16px; overflow: hidden; }
    .brand-bar { padding: 20px 28px; background: #09090b; }
    .brand-mark { width: 34px; height: 34px; padding-top: 6px; border-radius: 8px; background: #ffffff; box-sizing: border-box; }
    .brand-mark-bar { display: block; height: 6px; margin: 0 auto 2px; border-radius: 3px; background: #111317; font-size: 0; line-height: 0; }
    .brand-mark-bar-wide { width: 23px; }
    .brand-mark-bar-medium { width: 15px; height: 5px; }
    .brand-mark-bar-short { width: 7px; margin-bottom: 0; }
    .brand-name { color: #ffffff; font-size: 16px; font-weight: 700; line-height: 20px; }
    .brand-tagline { margin-top: 2px; color: #a1a1aa; font-size: 9px; font-weight: 700; line-height: 12px; letter-spacing: 1.5px; text-transform: uppercase; }
    .brand-rule { height: 3px; background: #2563eb; font-size: 0; line-height: 0; }
    .hero { padding: 32px 32px 18px; background: #ffffff; }
    .eyebrow { margin: 0 0 10px; color: #52525b; font-size: 11px; font-weight: 700; line-height: 16px; letter-spacing: 1.3px; text-transform: uppercase; }
    .status-dot { display: inline-block; width: 7px; height: 7px; margin-right: 8px; border-radius: 999px; background: ` + accent + `; vertical-align: 1px; }
    h1 { margin: 0; color: #09090b; font-size: 27px; font-weight: 750; line-height: 35px; letter-spacing: -0.5px; }
    .content { padding: 0 32px 34px; color: #3f3f46; font-size: 15px; line-height: 1.72; }
    .content p { margin: 0 0 16px; }
    .content p:last-child { margin-bottom: 0; }
    .content strong { color: #09090b; font-weight: 700; }
    .content a { color: #2563eb; }
    .button { display: inline-block; margin-top: 6px; padding: 12px 20px; border-radius: 9px; background: #2563eb; color: #ffffff !important; text-decoration: none; font-weight: 700; line-height: 20px; }
    .muted { padding: 14px 16px; border: 1px solid #e4e4e7; border-radius: 10px; background: #fafafa; color: #71717a; font-size: 12px; line-height: 1.65; overflow-wrap: anywhere; word-break: break-word; }
    .verification-code { margin: 22px 0 !important; padding: 18px 12px; border: 1px solid #d4d4d8; border-left: 4px solid #2563eb; border-radius: 10px; background: #fafafa; color: #09090b; font-family: "SFMono-Regular", Consolas, "Liberation Mono", monospace; font-size: 32px; font-weight: 800; line-height: 40px; letter-spacing: 8px; text-align: center; }
    .content table { width: 100%; margin: 18px 0; border: 1px solid #e4e4e7; border-radius: 10px; background: #ffffff; }
    .content table td { padding: 10px 12px; border-bottom: 1px solid #e4e4e7; color: #52525b; font-size: 13px; vertical-align: top; }
    .content table tr:last-child td { border-bottom: 0; }
    .content table td:last-child { color: #18181b; font-weight: 600; }
    .meta { background: #fafafa !important; }
    .meta-label { width: 112px; color: #71717a !important; font-weight: 600; }
    .section-title { margin: 28px 0 12px; color: #09090b; font-size: 15px; font-weight: 750; line-height: 1.4; }
    .metric-grid { border: 0 !important; border-collapse: separate !important; border-spacing: 8px !important; margin: -8px !important; }
    .metric-cell { width: 50%; padding: 15px 16px !important; border: 1px solid #e4e4e7 !important; border-radius: 10px; background: #ffffff; }
    .metric-label { display: block; color: #71717a; font-size: 11px; font-weight: 600; line-height: 1.4; text-transform: uppercase; letter-spacing: 0.5px; }
    .metric-value { display: block; margin-top: 7px; color: #09090b; font-size: 21px; font-weight: 750; line-height: 1.2; }
    .metric-value.good { color: #2563eb; }
    .metric-value.alert { color: #dc2626; }
    .detail td:first-child { width: 56%; color: #71717a; font-weight: 500; }
    .detail td:last-child { color: #09090b; font-weight: 700; text-align: right; }
    .report-detail { margin-top: 28px; }
    .report-detail:empty { display: none; }
    .footer { padding: 18px 32px; border-top: 1px solid #e4e4e7; background: #fafafa; color: #71717a; font-size: 11px; line-height: 1.65; }
    .footer-brand { color: #27272a; font-weight: 700; }
    .footer-blue { color: #2563eb; }
    @media only screen and (max-width: 620px) {
      .email-pad { padding: 0 !important; }
      .email-shell { border: 0 !important; border-radius: 0 !important; }
      .brand-bar { padding: 18px 20px !important; }
      .hero { padding: 26px 20px 16px !important; }
      .content { padding: 0 20px 28px !important; }
      .footer { padding: 16px 20px !important; }
      h1 { font-size: 24px !important; line-height: 31px !important; }
      .metric-grid, .metric-grid tbody, .metric-grid tr, .metric-cell { display: block !important; width: 100% !important; box-sizing: border-box !important; }
      .metric-cell { margin: 8px 0 !important; }
    }
  </style>
</head>
<body>
  <span class="preheader">` + preheader + `</span>
  <table class="email-bg" role="presentation" width="100%" cellpadding="0" cellspacing="0">
    <tr>
      <td class="email-pad" align="center" style="padding: 28px 12px;">
        <table class="email-shell" role="presentation" width="680" cellpadding="0" cellspacing="0">
          <tr>
            <td class="brand-bar">
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
                <tr>
                  <td width="46" style="width: 46px;">
                    <div class="brand-mark" role="img" aria-label="TokenSupply">
                      <span class="brand-mark-bar brand-mark-bar-wide">&nbsp;</span>
                      <span class="brand-mark-bar brand-mark-bar-medium">&nbsp;</span>
                      <span class="brand-mark-bar brand-mark-bar-short">&nbsp;</span>
                    </div>
                  </td>
                  <td>
                    <div class="brand-name">` + options.Brand + `</div>
                    <div class="brand-tagline">Yet another API platform</div>
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          <tr><td class="brand-rule">&nbsp;</td></tr>
          <tr>
            <td class="hero">
              <p class="eyebrow"><span class="status-dot"></span>` + options.Eyebrow + `</p>
              <h1>` + options.Title + `</h1>
            </td>
          </tr>
          <tr><td class="content">` + options.Content + `</td></tr>
          <tr>
            <td class="footer">
              <span class="footer-blue">●</span>&nbsp;
              <span class="footer-brand">` + options.Brand + `</span>&nbsp;&nbsp;·&nbsp;&nbsp;` + options.Footer + `
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`
}

// BuildSMTPTestEmailBody keeps the administrator's SMTP test message on the
// same visual system as all production notification emails.
func BuildSMTPTestEmailBody(siteName string) string {
	escapedSiteName := html.EscapeString(siteName)
	return buildBrandedEmailDocument(brandedEmailDocumentOptions{
		Lang:      "en",
		Brand:     escapedSiteName,
		Accent:    "#2563eb",
		Eyebrow:   "Delivery check",
		Title:     "Email configuration successful",
		Content:   `<p>Your SMTP settings are working correctly.</p><p class="muted">This test confirms that ` + escapedSiteName + ` can connect to the configured mail server and deliver branded HTML email.</p>`,
		Footer:    "Automated delivery test. No action is required.",
		Preheader: "SMTP delivery test completed successfully",
	})
}
