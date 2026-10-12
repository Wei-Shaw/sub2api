//go:build unit

package service

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/textproto"
	"testing"

	"github.com/stretchr/testify/require"
)

type videoMultipartTestPart struct {
	header textproto.MIMEHeader
	data   []byte
}

func videoMultipartTestField(name, value string) videoMultipartTestPart {
	return videoMultipartTestPart{
		header: textproto.MIMEHeader{"Content-Disposition": {mime.FormatMediaType("form-data", map[string]string{"name": name})}},
		data:   []byte(value),
	}
}

func videoMultipartTestBody(t *testing.T, parts []videoMultipartTestPart) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range parts {
		dst, err := writer.CreatePart(part.header)
		require.NoError(t, err)
		_, err = dst.Write(part.data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return body.Bytes(), writer.FormDataContentType()
}

func videoMultipartTestPrepare(t *testing.T, parts []videoMultipartTestPart, protocol string) []videoMultipartTestPart {
	t.Helper()
	body, contentType := videoMultipartTestBody(t, parts)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"model_mapping": map[string]any{"client-video": "provider-video"},
	}}
	if protocol != "" {
		account.Extra = map[string]any{"openai_video_protocol": protocol}
	}
	out, outType, _, _, err := prepareOpenAIVideoBody(account, body, contentType)
	require.NoError(t, err)
	_, params, err := mime.ParseMediaType(outType)
	require.NoError(t, err)
	reader := multipart.NewReader(bytes.NewReader(out), params["boundary"])
	var got []videoMultipartTestPart
	for {
		part, err := reader.NextRawPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		data, err := io.ReadAll(part)
		require.NoError(t, err)
		got = append(got, videoMultipartTestPart{header: part.Header, data: data})
	}
	return got
}

func TestOpenAIVideoMultipartExistingAspectRatio(t *testing.T) {
	for _, value := range []string{"4:3", ""} {
		for _, before := range []bool{true, false} {
			t.Run(value+map[bool]string{true: "/before", false: "/after"}[before], func(t *testing.T) {
				size := videoMultipartTestField("size", "1280x720")
				aspect := videoMultipartTestField("aspect_ratio", value)
				parts := []videoMultipartTestPart{size, aspect}
				if before {
					parts[0], parts[1] = parts[1], parts[0]
				}
				require.Equal(t, parts, videoMultipartTestPrepare(t, parts, "grok"))
			})
		}
	}
}

func TestOpenAIVideoMultipartInvalidSizeUnchanged(t *testing.T) {
	for _, value := range []string{"", "auto", "1280", "0x720", "-1x720", "1280xnope"} {
		t.Run(value, func(t *testing.T) {
			part := videoMultipartTestField("size", value)
			part.header.Set("X-Provider-Option", "keep")
			require.Equal(t, []videoMultipartTestPart{part}, videoMultipartTestPrepare(t, []videoMultipartTestPart{part}, "grok"))
		})
	}
}

func TestOpenAIVideoMultipartPreservesHeadersAndFiles(t *testing.T) {
	model := videoMultipartTestField("model", "client-video")
	model.header.Set("X-Provider-Model", "keep")
	size := videoMultipartTestField("size", "720x1280")
	size.header.Set("Content-Disposition", `form-data; name="size"; provider="geometry"`)
	size.header.Set("Content-Type", "text/plain; charset=utf-8")
	size.header["X-Provider-Option"] = []string{"one", "two"}
	parts := []videoMultipartTestPart{model, size}
	for _, name := range []string{"input_reference", "image", "image[0]", "image[1]"} {
		for _, filename := range []string{"frame.bin", ""} {
			parts = append(parts, videoMultipartTestPart{
				header: textproto.MIMEHeader{
					"Content-Disposition":       {mime.FormatMediaType("form-data", map[string]string{"name": name, "filename": filename})},
					"Content-Type":              {"application/octet-stream"},
					"Content-Transfer-Encoding": {"quoted-printable"},
					"X-Provider-File":           {"keep"},
				},
				data: []byte("=00=FF\r\n=41"),
			})
		}
	}
	parts = append(parts, videoMultipartTestPart{
		header: textproto.MIMEHeader{
			"Content-Disposition": {`form-data; name="input_reference"; filename="raw.bin"`},
			"Content-Type":        {"application/octet-stream"},
		},
		data: []byte{0, 255, '\r', '\n', '=', '4', '1'},
	})
	got := videoMultipartTestPrepare(t, parts, "grok")
	require.Len(t, got, len(parts))
	require.Equal(t, model.header, got[0].header)
	require.Equal(t, []byte("provider-video"), got[0].data)
	require.Equal(t, []byte("9:16"), got[1].data)
	_, params, err := mime.ParseMediaType(got[1].header.Get("Content-Disposition"))
	require.NoError(t, err)
	require.Equal(t, map[string]string{"name": "aspect_ratio", "provider": "geometry"}, params)
	for _, key := range []string{"Content-Type", "X-Provider-Option"} {
		require.Equal(t, size.header[key], got[1].header[key])
	}
	require.Equal(t, parts[2:], got[2:])
	imageFiles := 0
	for _, part := range got {
		_, disposition, err := mime.ParseMediaType(part.header.Get("Content-Disposition"))
		require.NoError(t, err)
		if _, isFile := disposition["filename"]; isFile && disposition["name"] == "image" {
			imageFiles++
			require.Equal(t, []byte("=00=FF\r\n=41"), part.data, "image upload bytes must remain unchanged")
		}
	}
	require.Equal(t, 2, imageFiles, "preserve image files with both nonempty and empty filename attributes")
}

func TestOpenAIVideoMultipartQuotedPrintableFields(t *testing.T) {
	parts := []videoMultipartTestPart{
		videoMultipartTestField("model", "client=2Dvideo"),
		videoMultipartTestField("size", "720=781280"),
	}
	for _, part := range parts {
		part.header.Set("Content-Transfer-Encoding", "quoted-printable")
	}
	got := videoMultipartTestPrepare(t, parts, "grok")
	require.Len(t, got, 2)
	for i, expected := range []string{"provider-video", "9:16"} {
		require.Equal(t, "quoted-printable", got[i].header.Get("Content-Transfer-Encoding"))
		decoded, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(got[i].data)))
		require.NoError(t, err)
		require.Equal(t, expected, string(decoded))
	}
}

func TestOpenAIVideoMultipartEachSizeValidated(t *testing.T) {
	parts := []videoMultipartTestPart{
		videoMultipartTestField("size", "auto"),
		videoMultipartTestField("size", "720x1280"),
	}
	got := videoMultipartTestPrepare(t, parts, "grok")
	require.Len(t, got, 2)
	require.Equal(t, parts[0], got[0])
	_, params, err := mime.ParseMediaType(got[1].header.Get("Content-Disposition"))
	require.NoError(t, err)
	require.Equal(t, "aspect_ratio", params["name"])
	require.Equal(t, []byte("9:16"), got[1].data)
}

func TestOpenAIVideoMultipartGenericPreservesSize(t *testing.T) {
	for _, protocol := range []string{"", "openai", "GROK", " grok "} {
		t.Run(protocol, func(t *testing.T) {
			model := videoMultipartTestField("model", "client-video")
			size := videoMultipartTestField("size", "1792x1024")
			size.header.Set("X-Provider-Option", "keep")
			got := videoMultipartTestPrepare(t, []videoMultipartTestPart{model, size}, protocol)
			require.Len(t, got, 2)
			require.Equal(t, model.header, got[0].header)
			require.Equal(t, []byte("provider-video"), got[0].data)
			require.Equal(t, size, got[1])
		})
	}
}

func TestOpenAIVideoMultipartProtocolGuardJSON(t *testing.T) {
	for _, protocol := range []string{"", "openai", "grok"} {
		t.Run(protocol, func(t *testing.T) {
			account := &Account{}
			if protocol != "" {
				account.Extra = map[string]any{"openai_video_protocol": protocol}
			}
			body := []byte(`{"model":"client-video","size":"1792x1024","provider_option":{"keep":true}}`)
			out, contentType, info, _, err := prepareOpenAIVideoBody(account, body, "application/json")
			require.NoError(t, err)
			require.Equal(t, "application/json", contentType)
			require.Equal(t, "1792x1024", info.Size)
			if protocol == "grok" {
				require.JSONEq(t, `{"model":"client-video","aspect_ratio":"16:9","provider_option":{"keep":true}}`, string(out))
			} else {
				require.JSONEq(t, string(body), string(out))
			}
		})
	}
}

func TestOpenAIVideoMultipartProtocolOptIn(t *testing.T) {
	require.False(t, isOpenAIGrokVideoProtocol(nil))
	require.False(t, isOpenAIGrokVideoProtocol(&Account{}))
	require.False(t, isOpenAIGrokVideoProtocol(&Account{Extra: map[string]any{"openai_video_protocol": true}}))
	require.True(t, isOpenAIGrokVideoProtocol(&Account{Extra: map[string]any{"openai_video_protocol": "grok"}}))
}

func TestOpenAIVideoMultipartParseMetadata(t *testing.T) {
	parts := []videoMultipartTestPart{
		videoMultipartTestField("model", " client-video "),
		videoMultipartTestField("prompt", " a scene "),
		videoMultipartTestField("size", "720x1280"),
		videoMultipartTestField("aspect_ratio", "4:3"),
		videoMultipartTestField("seconds", "8"),
		videoMultipartTestField("duration", "10"),
		videoMultipartTestField("n", "2"),
		{
			header: textproto.MIMEHeader{
				"Content-Disposition":       {`form-data; name="input_reference"; filename=""`},
				"Content-Type":              {"image/png"},
				"Content-Transfer-Encoding": {"quoted-printable"},
			},
			data: []byte("=00=FF=41"),
		},
	}
	body, contentType := videoMultipartTestBody(t, parts)
	info := ParseOpenAIVideoRequest(contentType, body)
	require.Equal(t, "client-video", info.Model)
	require.Equal(t, "a scene", info.Prompt)
	require.Equal(t, "720x1280", info.Size)
	require.Equal(t, "4:3", info.AspectRatio)
	require.Equal(t, 8, info.DurationSeconds)
	require.Equal(t, "720p", info.Resolution)
	require.Equal(t, 2, info.N)
	require.Len(t, info.Uploads, 1)
	require.Equal(t, "input_reference", info.Uploads[0].FieldName)
	require.Empty(t, info.Uploads[0].FileName)
	require.Equal(t, "image/png", info.Uploads[0].ContentType)
	require.Equal(t, []byte{0, 255, 'A'}, info.Uploads[0].Data)
	require.Empty(t, info.InputImageURLs)
}

func TestOpenAIVideoMultipartReadLimit(t *testing.T) {
	for _, field := range []string{"input_reference", "prompt"} {
		for _, extra := range []int{0, 1} {
			t.Run(field+map[int]string{0: "/at_limit", 1: "/over_limit"}[extra], func(t *testing.T) {
				part := videoMultipartTestField(field, "")
				if field == "input_reference" {
					part.header.Set("Content-Disposition", `form-data; name="input_reference"; filename="frame.bin"`)
				}
				part.data = bytes.Repeat([]byte{'x'}, openAIImageMaxUploadPartSize+extra)
				body, contentType := videoMultipartTestBody(t, []videoMultipartTestPart{part})
				info := ParseOpenAIVideoRequest(contentType, body)
				out, _, _, _, err := prepareOpenAIVideoBody(&Account{}, body, contentType)
				if extra > 0 {
					require.ErrorContains(t, err, "multipart part exceeds")
					require.Nil(t, out)
					require.Empty(t, info.Uploads, "oversized uploads must not enter moderation")
					require.Empty(t, info.Prompt, "oversized fields must not be silently truncated")
				} else {
					require.NoError(t, err)
					if field == "input_reference" {
						require.Len(t, info.Uploads, 1)
						require.Equal(t, part.data, info.Uploads[0].Data)
					} else {
						require.Len(t, info.Prompt, openAIImageMaxUploadPartSize)
					}
				}
			})
		}
	}
}

func TestOpenAIVideoMultipartMalformedBody(t *testing.T) {
	body, contentType := videoMultipartTestBody(t, []videoMultipartTestPart{videoMultipartTestField("model", "client-video")})
	_, params, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	body = bytes.TrimSuffix(body, []byte("\r\n--"+params["boundary"]+"--\r\n"))
	out, _, _, _, err := prepareOpenAIVideoBody(&Account{}, body, contentType)
	require.Error(t, err)
	require.Nil(t, out)
}
