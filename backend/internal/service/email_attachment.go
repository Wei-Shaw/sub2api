package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/smtp"
	"net/textproto"
	"net/url"
	"strings"
)

// EmailAttachment represents a single file attached to an outgoing email.
//
// Filename supports any UTF-8 text (Chinese, etc.); it will be encoded
// via RFC 2047 (for the Content-Type "name" parameter) and RFC 5987
// (for the Content-Disposition "filename*" parameter).
type EmailAttachment struct {
	Filename string
	MimeType string
	Content  []byte
}

// SendEmailWithAttachment sends an HTML email with one or more file attachments.
// Reuses the SMTP transport (sendMailTLS / sendMailPlain) from email_service.go.
func (s *EmailService) SendEmailWithAttachment(ctx context.Context, to, subject, body string, attachments []EmailAttachment) error {
	config, err := s.GetSMTPConfig(ctx)
	if err != nil {
		return err
	}

	to = sanitizeEmailHeader(to)
	subject = sanitizeEmailHeader(subject)

	from := sanitizeEmailHeader(config.From)
	if config.FromName != "" {
		from = fmt.Sprintf("%s <%s>", sanitizeEmailHeader(config.FromName), sanitizeEmailHeader(config.From))
	}

	msg, err := buildMultipartMessage(from, to, subject, body, attachments)
	if err != nil {
		return fmt.Errorf("build multipart message: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	auth := smtp.PlainAuth("", config.Username, config.Password, config.Host)
	if config.UseTLS {
		return s.sendMailTLS(addr, auth, config.From, to, msg, config.Host)
	}
	return s.sendMailPlain(addr, auth, config.From, to, msg, config.Host)
}

// buildMultipartMessage assembles an RFC 5322 message with a multipart/mixed body.
// Part 1 is the HTML body (quoted-printable). Parts 2..N are the attachments (base64).
func buildMultipartMessage(from, to, subject, body string, attachments []EmailAttachment) ([]byte, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	headers := make(textproto.MIMEHeader)
	headers.Set("From", from)
	headers.Set("To", to)
	headers.Set("Subject", mime.QEncoding.Encode("UTF-8", subject))
	headers.Set("MIME-Version", "1.0")
	headers.Set("Content-Type", "multipart/mixed; boundary="+mw.Boundary())
	for k, vs := range headers {
		for _, v := range vs {
			fmt.Fprintf(&buf, "%s: %s\r\n", k, v)
		}
	}
	buf.WriteString("\r\n")

	// HTML body part.
	bodyHeader := make(textproto.MIMEHeader)
	bodyHeader.Set("Content-Type", "text/html; charset=UTF-8")
	bodyHeader.Set("Content-Transfer-Encoding", "quoted-printable")
	bw, err := mw.CreatePart(bodyHeader)
	if err != nil {
		return nil, err
	}
	qw := quotedprintable.NewWriter(bw)
	if _, err := qw.Write([]byte(body)); err != nil {
		return nil, err
	}
	if err := qw.Close(); err != nil {
		return nil, err
	}

	// Attachment parts.
	for _, att := range attachments {
		mimeType := strings.TrimSpace(att.MimeType)
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		filename := att.Filename
		if filename == "" {
			filename = "attachment"
		}
		encodedName := mime.BEncoding.Encode("UTF-8", filename)
		asciiFallback := asciiFallbackFilename(filename)
		percentName := url.PathEscape(filename)

		ah := make(textproto.MIMEHeader)
		ah.Set("Content-Type", fmt.Sprintf(`%s; name="%s"`, mimeType, encodedName))
		ah.Set("Content-Transfer-Encoding", "base64")
		ah.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, asciiFallback, percentName))

		aw, err := mw.CreatePart(ah)
		if err != nil {
			return nil, err
		}
		b64 := base64.NewEncoder(base64.StdEncoding, lineWrappingWriter{aw})
		if _, err := b64.Write(att.Content); err != nil {
			return nil, err
		}
		if err := b64.Close(); err != nil {
			return nil, err
		}
	}

	if err := mw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// asciiFallbackFilename returns a 7-bit-ASCII filename usable in the
// non-extended Content-Disposition "filename" parameter. Non-ASCII bytes
// are replaced with '_'.
func asciiFallbackFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			b.WriteByte('_')
		} else {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "attachment"
	}
	return out
}

// lineWrappingWriter wraps base64 output at 76 chars per RFC 2045.
type lineWrappingWriter struct{ inner interface{ Write([]byte) (int, error) } }

func (w lineWrappingWriter) Write(p []byte) (int, error) {
	const lineLen = 76
	written := 0
	for len(p) > 0 {
		chunk := p
		if len(chunk) > lineLen {
			chunk = chunk[:lineLen]
		}
		n, err := w.inner.Write(chunk)
		written += n
		if err != nil {
			return written, err
		}
		if _, err := w.inner.Write([]byte("\r\n")); err != nil {
			return written, err
		}
		p = p[len(chunk):]
	}
	return written, nil
}
