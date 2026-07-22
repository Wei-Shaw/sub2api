//go:build unit

package service

import (
	"bufio"
	"bytes"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildEmailMessageEncodesUTF8ForLegacySMTP(t *testing.T) {
	config := &SMTPConfig{
		From:     "alert@tokensupply.net",
		FromName: "令牌供应",
	}
	now := time.Date(2026, time.July, 22, 12, 34, 56, 0, time.FixedZone("CST", 8*60*60))
	body := "<p>您的验证码是：123456</p>"

	raw, err := buildEmailMessage(
		config,
		"recipient@example.com",
		"[TokenSupply] 邮箱验证码",
		body,
		now,
		"<test-message@tokensupply.net>",
	)
	require.NoError(t, err)

	headerEnd := bytes.Index(raw, []byte("\r\n\r\n"))
	require.Positive(t, headerEnd)
	for _, value := range raw {
		require.Less(t, value, byte(0x80), "raw SMTP message must remain 7-bit ASCII")
	}

	message, err := mail.ReadMessage(bufio.NewReader(bytes.NewReader(raw)))
	require.NoError(t, err)

	decodedSubject, err := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
	require.NoError(t, err)
	require.Equal(t, "[TokenSupply] 邮箱验证码", decodedSubject)

	from, err := mail.ParseAddress(message.Header.Get("From"))
	require.NoError(t, err)
	require.Equal(t, "令牌供应", from.Name)
	require.Equal(t, "alert@tokensupply.net", from.Address)
	require.Equal(t, now.Format(time.RFC1123Z), message.Header.Get("Date"))
	require.Equal(t, "<test-message@tokensupply.net>", message.Header.Get("Message-ID"))
	require.Equal(t, "quoted-printable", message.Header.Get("Content-Transfer-Encoding"))

	decodedBody, err := io.ReadAll(quotedprintable.NewReader(message.Body))
	require.NoError(t, err)
	require.Equal(t, body, string(decodedBody))
}

func TestBuildEmailMessageKeepsASCIISubjectAndBlocksHeaderInjection(t *testing.T) {
	config := &SMTPConfig{
		From:     "alert@tokensupply.net\r\nBcc: attacker@example.com",
		FromName: "TokenSupply\r\nX-Injected: yes",
	}

	raw, err := buildEmailMessage(
		config,
		"recipient@example.com\r\nBcc: attacker@example.com",
		"Email verification code\r\nX-Injected: yes",
		"<p>hello</p>",
		time.Unix(0, 0).UTC(),
		"<test-message@tokensupply.net>",
	)
	require.NoError(t, err)

	message, err := mail.ReadMessage(bufio.NewReader(bytes.NewReader(raw)))
	require.NoError(t, err)
	require.Equal(t, "Email verification codeX-Injected: yes", message.Header.Get("Subject"))
	require.Empty(t, message.Header.Get("X-Injected"))
	require.Empty(t, message.Header.Get("Bcc"))
	require.NotContains(t, string(raw), "\r\nX-Injected:")
	require.NotContains(t, string(raw), "\r\nBcc:")
}

func TestGenerateEmailMessageIDUsesSenderDomain(t *testing.T) {
	messageID, err := generateEmailMessageID("alert@tokensupply.net")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(messageID, "<"))
	require.True(t, strings.HasSuffix(messageID, "@tokensupply.net>"))
}
