//go:build unit

package service

import (
	"bytes"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildSMTPMessageProducesStandardsCompliantMIME(t *testing.T) {
	config := &SMTPConfig{
		Host:     "smtp.example.com",
		From:     "reply@example.com",
		FromName: "Sub2API 通知",
REDACTED
	body := "<html>\n<body>验证码：123456 &amp; ready</body>\n</html>"

	message, err := buildSMTPMessage(config, "User <user@example.net>", "邮箱验证码", body)
REDACTED
	require.Equal(t, "reply@example.com", message.envelopeFrom)
	require.Equal(t, "user@example.net", message.envelopeTo)

	parsed, err := mail.ReadMessage(bytes.NewReader(message.data))
REDACTED

	from, err := mail.ParseAddress(parsed.Header.Get("From"))
REDACTED
	require.Equal(t, "Sub2API 通知", from.Name)
	require.Equal(t, "reply@example.com", from.Address)

	recipient, err := mail.ParseAddress(parsed.Header.Get("To"))
REDACTED
	require.Equal(t, "User", recipient.Name)
	require.Equal(t, "user@example.net", recipient.Address)

	decodedSubject, err := new(mime.WordDecoder).DecodeHeader(parsed.Header.Get("Subject"))
REDACTED
	require.Equal(t, "邮箱验证码", decodedSubject)
	require.NotEmpty(t, parsed.Header.Get("Date"))
	_, err = mail.ParseDate(parsed.Header.Get("Date"))
REDACTED
	require.Regexp(t, regexp.MustCompile(`^<[0-9a-f]{32REDACTED@example\.com>$`), parsed.Header.Get("Message-ID"))
	require.Equal(t, "1.0", parsed.Header.Get("MIME-Version"))
	require.Equal(t, "quoted-printable", parsed.Header.Get("Content-Transfer-Encoding"))

	mediaType, params, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
REDACTED
	require.Equal(t, "text/html", mediaType)
	require.Equal(t, "UTF-8", params["charset"])

	decodedBody, err := io.ReadAll(quotedprintable.NewReader(parsed.Body))
REDACTED
	require.Equal(t, strings.ReplaceAll(body, "\n", "\r\n"), string(decodedBody))
REDACTED

func TestBuildSMTPMessagePreventsHeaderInjection(t *testing.T) {
	config := &SMTPConfig{
		Host:     "smtp.example.com",
		From:     "reply@example.com",
		FromName: "Sender\r\nBcc: hidden@example.com",
REDACTED

	message, err := buildSMTPMessage(config, "user@example.net", "Subject\r\nCc: hidden@example.com", "body")
REDACTED

	parsed, err := mail.ReadMessage(bytes.NewReader(message.data))
REDACTED
	require.Empty(t, parsed.Header.Get("Bcc"))
	require.Empty(t, parsed.Header.Get("Cc"))

	decodedSubject, err := new(mime.WordDecoder).DecodeHeader(parsed.Header.Get("Subject"))
REDACTED
	require.Equal(t, "SubjectCc: hidden@example.com", decodedSubject)
REDACTED

func TestBuildSMTPMessageRejectsInvalidConfiguration(t *testing.T) {
	_, err := buildSMTPMessage(nil, "user@example.net", "subject", "body")
	require.ErrorContains(t, err, "missing SMTP configuration")

	_, err = buildSMTPMessage(&SMTPConfig{Host: "smtp.example.com"REDACTED, "user@example.net", "subject", "body")
	require.ErrorContains(t, err, "invalid SMTP from address")

	_, err = buildSMTPMessage(&SMTPConfig{
		Host: "smtp.example.com",
		From: "reply@example.com",
REDACTED, "invalid recipient <>", "subject", "body")
	require.ErrorContains(t, err, "invalid SMTP recipient address")

	_, err = buildSMTPMessage(&SMTPConfig{
		Host: "smtp.example.com",
		From: "reply@example.com",
REDACTED, "user@example.net\r\nBcc: hidden@example.net", "subject", "body")
	require.ErrorContains(t, err, "invalid SMTP recipient address")
REDACTED

func TestBuildSMTPMessageUsesUniqueMessageIDs(t *testing.T) {
	config := &SMTPConfig{Host: "smtp.example.com", From: "reply@example.com"REDACTED

	first, err := buildSMTPMessage(config, "user@example.net", "subject", "body")
REDACTED
	second, err := buildSMTPMessage(config, "user@example.net", "subject", "body")
REDACTED

	firstParsed, err := mail.ReadMessage(bytes.NewReader(first.data))
REDACTED
	secondParsed, err := mail.ReadMessage(bytes.NewReader(second.data))
REDACTED
	require.NotEqual(t, firstParsed.Header.Get("Message-ID"), secondParsed.Header.Get("Message-ID"))
REDACTED
