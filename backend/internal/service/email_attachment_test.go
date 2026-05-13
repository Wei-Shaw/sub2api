//go:build unit

package service

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildMultipartMessage_HasBodyAndAttachment(t *testing.T) {
	body := "<html><body>Hello 你好</body></html>"
	pdf := []byte("%PDF-1.4 dummy\n")
	att := EmailAttachment{Filename: "发票_001.pdf", MimeType: "application/pdf", Content: pdf}

	raw, err := buildMultipartMessage("from@example.com", "to@example.com", "测试主题 / Test Subject", body, []EmailAttachment{att})
	require.NoError(t, err)

	m, err := mail.ReadMessage(bytes.NewReader(raw))
	require.NoError(t, err)

	dec := new(mime.WordDecoder)
	gotSubject, err := dec.DecodeHeader(m.Header.Get("Subject"))
	require.NoError(t, err)
	require.Equal(t, "测试主题 / Test Subject", gotSubject)

	mediaType, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/mixed", mediaType)

	mr := multipart.NewReader(m.Body, params["boundary"])

	// First part: HTML body, quoted-printable.
	p1, err := mr.NextPart()
	require.NoError(t, err)
	require.Contains(t, p1.Header.Get("Content-Type"), "text/html")
	// multipart.Reader auto-hides Content-Transfer-Encoding: quoted-printable and
	// transparently decodes the body; verify the encoding directly on raw bytes
	// to confirm the implementation still emitted it.
	require.Contains(t, string(raw), "Content-Transfer-Encoding: quoted-printable")
	bodyBytes, err := io.ReadAll(p1)
	require.NoError(t, err)
	require.Equal(t, body, string(bodyBytes))

	// Second part: PDF attachment with Chinese filename round-trip.
	p2, err := mr.NextPart()
	require.NoError(t, err)
	require.Equal(t, "base64", p2.Header.Get("Content-Transfer-Encoding"))

	cd := p2.Header.Get("Content-Disposition")
	require.Contains(t, cd, "attachment")
	require.Contains(t, cd, "filename*=UTF-8''")

	_, cdParams, err := mime.ParseMediaType(cd)
	require.NoError(t, err)
	// mime.ParseMediaType decodes the RFC 5987 filename* form back to UTF-8
	// and stores it under the bare "filename" key (preferring the encoded
	// form over the ASCII fallback when both are present).
	require.Equal(t, "发票_001.pdf", cdParams["filename"])

	// multipart.Reader transparently decodes quoted-printable but NOT base64;
	// decode the attachment payload explicitly.
	pdfDecoded, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, p2))
	require.NoError(t, err)
	require.Equal(t, pdf, pdfDecoded)

	// No more parts.
	_, err = mr.NextPart()
	require.Equal(t, io.EOF, err)
}

func TestBuildMultipartMessage_NoAttachments(t *testing.T) {
	raw, err := buildMultipartMessage("a@b", "c@d", "s", "hi", nil)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(raw), "multipart/mixed"))
}
