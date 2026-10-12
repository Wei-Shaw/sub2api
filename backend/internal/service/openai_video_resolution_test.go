//go:build unit

package service

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestPrepareOpenAIVideoResolutionJSON(t *testing.T) {
	for _, tc := range []struct {
		name       string
		body       string
		grok       bool
		size       string
		resolution string
		wantError  bool
	}{
		{name: "native size", body: `{"size":"1280x720","custom":9007199254740993}`, resolution: "720p"},
		{name: "native resolution", body: `{"resolution":"1080p"}`, resolution: "1080p"},
		{name: "consistent fields", body: `{"size":"720x1280","resolution":"hd","resolution":720}`, resolution: "720p"},
		{name: "native missing", body: `{}`, wantError: true},
		{name: "native stale info not explicit", body: `{}`, size: "1280x720", wantError: true},
		{name: "unknown size", body: `{"size":"1792x1024"}`, wantError: true},
		{name: "unknown size despite valid resolution", body: `{"size":"1792x1024","resolution":"720p"}`, wantError: true},
		{name: "unknown resolution despite valid size", body: `{"size":"1280x720","resolution":"4k"}`, wantError: true},
		{name: "mismatch", body: `{"size":"1280x720","resolution":"480p"}`, wantError: true},
		{name: "duplicate mismatch", body: `{"resolution":"480p","resolution":"720p"}`, wantError: true},
		{name: "escaped duplicate", body: `{"resolution":"720p","resol\u0075tion":"1080p"}`, wantError: true},
		{name: "bad duplicate first", body: `{"resolution":null,"resolution":"720p"}`, wantError: true},
		{name: "bad duplicate last", body: `{"resolution":"720p","resolution":null}`, wantError: true},
		{name: "empty resolution", body: `{"resolution":""}`, grok: true, wantError: true},
		{name: "null size", body: `{"size":null}`, grok: true, wantError: true},
		{name: "empty size", body: `{"size":""}`, grok: true, wantError: true},
		{name: "object resolution", body: `{"resolution":{"value":"720p"}}`, wantError: true},
		{name: "boolean resolution", body: `{"resolution":true}`, wantError: true},
		{name: "duplicate size mismatch", body: `{"size":"1280x720","size":"1920x1080"}`, wantError: true},
		{name: "grok default", body: `{"aspect_ratio":"16:9"}`, grok: true, resolution: "480p"},
		{name: "grok converted size", body: `{"aspect_ratio":"16:9"}`, grok: true, size: "1280x720", resolution: "720p"},
		{name: "grok invalid original", body: `{"aspect_ratio":"16:9"}`, grok: true, size: "1792x1024", wantError: true},
		{name: "grok converted mismatch", body: `{"aspect_ratio":"16:9","resolution":"480p"}`, grok: true, size: "1280x720", wantError: true},
		{name: "grok explicit aliases", body: `{"resolution":"HD","resolution":720}`, grok: true, resolution: "720p"},
		{name: "grok size", body: `{"size":"1920x1080","custom":{"resolution":"4k"}}`, grok: true, resolution: "1080p"},
		{name: "malformed", body: `{"resolution":"720p"`, wantError: true},
		{name: "array", body: `[]`, wantError: true},
		{name: "trailing object", body: `{"resolution":"720p"}{}`, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{}
			if tc.grok {
				account.Extra = map[string]any{"openai_video_protocol": "grok"}
			}
			info := GrokMediaRequestInfo{Size: tc.size, Resolution: "480p", Model: "client-video", DurationSeconds: 6}
			body := []byte(tc.body)
			original := bytes.Clone(body)
			out, contentType, got, err := prepareOpenAIVideoResolution(account, body, "application/json", info)
			require.Equal(t, original, body, "must not mutate the input buffer")
			if tc.wantError {
				require.Error(t, err)
				require.Nil(t, out)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "application/json", contentType)
			info.Resolution = tc.resolution
			require.Equal(t, info, got)
			if !tc.grok {
				require.Equal(t, body, out, "native JSON must remain byte-identical")
			} else {
				count := 0
				gjson.ParseBytes(out).ForEach(func(key, value gjson.Result) bool {
					if key.String() == "resolution" {
						count++
						require.Equal(t, gjson.String, value.Type)
						require.Equal(t, tc.resolution, value.String())
					}
					return true
				})
				require.Positive(t, count)
			}
		})
	}
}

func TestPrepareOpenAIVideoResolutionAfterBodyPreparation(t *testing.T) {
	for _, multipartBody := range []bool{false, true} {
		for _, size := range []string{"854x480", "720x1280", "1920x1080", "1792x1024"} {
			t.Run(size+map[bool]string{false: "/json", true: "/multipart"}[multipartBody], func(t *testing.T) {
				account := &Account{Extra: map[string]any{"openai_video_protocol": "grok"}}
				body := []byte(`{"model":"video","seconds":"6","size":"` + size + `","provider":{"n":9007199254740993}}`)
				contentType := "application/json"
				if multipartBody {
					body, contentType = resolutionTestMultipart(t, []resolutionTestPart{
						resolutionTestField("model", "video"),
						resolutionTestField("seconds", "6"),
						resolutionTestField("size", size),
					})
				}
				prepared, preparedType, info, _, err := prepareOpenAIVideoBody(account, body, contentType)
				require.NoError(t, err)
				require.Equal(t, size, info.Size)
				out, outType, got, err := prepareOpenAIVideoResolution(account, prepared, preparedType, info)
				if size == "1792x1024" {
					require.Error(t, err)
					require.Nil(t, out)
					return
				}
				require.NoError(t, err)
				expected := map[string]string{"854x480": "480p", "720x1280": "720p", "1920x1080": "1080p"}[size]
				require.Equal(t, expected, got.Resolution)
				if multipartBody {
					parts := resolutionTestReadMultipart(t, out, outType)
					resolutionTestAssertField(t, parts[len(parts)-1], expected)
				} else {
					require.Equal(t, expected, gjson.GetBytes(out, "resolution").String())
					require.False(t, gjson.GetBytes(out, "size").Exists())
					require.Equal(t, "9007199254740993", gjson.GetBytes(out, "provider.n").Raw)
				}
			})
		}
	}
}

type resolutionTestPart struct {
	header textproto.MIMEHeader
	data   []byte
}

func resolutionTestField(name, value string) resolutionTestPart {
	return resolutionTestPart{
		header: textproto.MIMEHeader{"Content-Disposition": {mime.FormatMediaType("form-data", map[string]string{"name": name})}},
		data:   []byte(value),
	}
}

func resolutionTestMultipart(t *testing.T, parts []resolutionTestPart) ([]byte, string) {
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

func resolutionTestReadMultipart(t *testing.T, body []byte, contentType string) []resolutionTestPart {
	t.Helper()
	_, params, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	var parts []resolutionTestPart
	for {
		part, err := reader.NextRawPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		data, err := io.ReadAll(part)
		require.NoError(t, err)
		parts = append(parts, resolutionTestPart{header: part.Header, data: data})
	}
	return parts
}

func resolutionTestAssertField(t *testing.T, part resolutionTestPart, expected string) {
	t.Helper()
	disposition, params, err := mime.ParseMediaType(part.header.Get("Content-Disposition"))
	require.NoError(t, err)
	require.Equal(t, "form-data", disposition)
	require.Equal(t, "resolution", params["name"])
	_, isFile := params["filename"]
	require.False(t, isFile)
	require.Equal(t, expected, string(part.data))
}

func TestPrepareOpenAIVideoResolutionMultipart(t *testing.T) {
	for _, tc := range []struct {
		name       string
		fields     []resolutionTestPart
		size       string
		resolution string
		wantError  bool
	}{
		{name: "size", fields: []resolutionTestPart{resolutionTestField("size", "1280x720")}, resolution: "720p"},
		{name: "aliases", fields: []resolutionTestPart{resolutionTestField("resolution", "hd"), resolutionTestField("resolution", "720p")}, resolution: "720p"},
		{name: "quoted printable", fields: []resolutionTestPart{{
			header: textproto.MIMEHeader{"Content-Disposition": {`form-data; name="resolution"; provider="keep"`}, "Content-Transfer-Encoding": {"quoted-printable"}},
			data:   []byte("720=70"),
		}}, resolution: "720p"},
		{name: "unknown first duplicate", fields: []resolutionTestPart{resolutionTestField("resolution", "4k"), resolutionTestField("resolution", "720p")}, wantError: true},
		{name: "unknown last duplicate", fields: []resolutionTestPart{resolutionTestField("resolution", "720p"), resolutionTestField("resolution", "4k")}, wantError: true},
		{name: "conflicting duplicates", fields: []resolutionTestPart{resolutionTestField("resolution", "720p"), resolutionTestField("resolution", "1080p")}, wantError: true},
		{name: "mismatched size", fields: []resolutionTestPart{resolutionTestField("size", "1280x720"), resolutionTestField("resolution", "480p")}, wantError: true},
		{name: "unknown size", fields: []resolutionTestPart{resolutionTestField("size", "1792x1024")}, wantError: true},
		{name: "empty resolution", fields: []resolutionTestPart{resolutionTestField("resolution", "")}, wantError: true},
		{name: "converted size mismatch", size: "1280x720", fields: []resolutionTestPart{resolutionTestField("resolution", "480p")}, wantError: true},
		{name: "unknown transfer encoding", fields: []resolutionTestPart{{
			header: textproto.MIMEHeader{"Content-Disposition": {`form-data; name="resolution"`}, "Content-Transfer-Encoding": {"base64"}},
			data:   []byte("NzIwcA=="),
		}}, wantError: true},
	} {
		for _, grok := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/native", true: "/grok"}[grok], func(t *testing.T) {
				account := &Account{}
				if grok {
					account.Extra = map[string]any{"openai_video_protocol": "grok"}
				}
				body, contentType := resolutionTestMultipart(t, tc.fields)
				out, outType, info, err := prepareOpenAIVideoResolution(account, body, contentType, GrokMediaRequestInfo{Size: tc.size})
				if tc.wantError && (tc.name != "converted size mismatch" || grok) {
					require.Error(t, err)
					require.Nil(t, out)
					return
				}
				require.NoError(t, err)
				if tc.name != "converted size mismatch" {
					require.Equal(t, tc.resolution, info.Resolution)
				}
				require.Equal(t, contentType, outType)
				if !grok {
					require.Equal(t, body, out)
				} else {
					parts := resolutionTestReadMultipart(t, out, outType)
					for i, part := range parts {
						if i < len(tc.fields) {
							require.Equal(t, tc.fields[i].header, part.header)
						}
						_, params, err := mime.ParseMediaType(part.header.Get("Content-Disposition"))
						require.NoError(t, err)
						if params["name"] == "resolution" {
							resolutionTestAssertField(t, part, tc.resolution)
						}
					}
				}
			})
		}
	}
}

func TestPrepareOpenAIVideoResolutionMultipartPreservesUploads(t *testing.T) {
	parts := []resolutionTestPart{resolutionTestField("aspect_ratio", "16:9")}
	for _, field := range []string{"input_reference", "image", "image[0]"} {
		for _, filename := range []string{"frame.bin", ""} {
			parts = append(parts, resolutionTestPart{
				header: textproto.MIMEHeader{
					"Content-Disposition":       {mime.FormatMediaType("form-data", map[string]string{"name": field, "filename": filename})},
					"Content-Type":              {"application/octet-stream"},
					"Content-Transfer-Encoding": {"quoted-printable"},
					"X-Provider":                {"one", "two"},
				},
				data: []byte{0, 255, '\r', '\n', '=', '4', '1'},
			})
		}
	}
	body, contentType := resolutionTestMultipart(t, parts)
	account := &Account{Extra: map[string]any{"openai_video_protocol": "grok"}}
	out, outType, info, err := prepareOpenAIVideoResolution(account, body, contentType, GrokMediaRequestInfo{Resolution: "1080p"})
	require.NoError(t, err)
	require.Equal(t, "480p", info.Resolution, "ignore stale normalized metadata")
	got := resolutionTestReadMultipart(t, out, outType)
	require.Len(t, got, len(parts)+1)
	require.Equal(t, parts, got[:len(parts)])
	resolutionTestAssertField(t, got[len(parts)], "480p")
	_, _, _, err = prepareOpenAIVideoResolution(&Account{}, body, contentType, GrokMediaRequestInfo{})
	require.Error(t, err, "image uploads do not supply a native billing tier")
}

func TestPrepareOpenAIVideoResolutionMultipartMalformed(t *testing.T) {
	body, contentType := resolutionTestMultipart(t, []resolutionTestPart{resolutionTestField("resolution", "720p")})
	_, params, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	body = bytes.TrimSuffix(body, []byte("\r\n--"+params["boundary"]+"--\r\n"))
	out, _, _, err := prepareOpenAIVideoResolution(&Account{}, body, contentType, GrokMediaRequestInfo{})
	require.Error(t, err)
	require.Nil(t, out)
}

func TestValidateOpenAIVideoOriginalResolution(t *testing.T) {
	for _, tc := range []struct {
		name      string
		fields    [][2]string
		wantError bool
	}{
		{name: "absent", fields: nil},
		{name: "consistent", fields: [][2]string{{"size", "1280x720"}, {"resolution", "hd"}, {"resolution", "720p"}}},
		{name: "unknown consumed size first", fields: [][2]string{{"size", "1792x1024"}, {"size", "1280x720"}}, wantError: true},
		{name: "unknown consumed size last", fields: [][2]string{{"size", "1280x720"}, {"size", "1792x1024"}}, wantError: true},
		{name: "conflicting consumed sizes", fields: [][2]string{{"size", "1920x1080"}, {"size", "1280x720"}}, wantError: true},
		{name: "unknown resolution", fields: [][2]string{{"resolution", "4k"}, {"resolution", "720p"}}, wantError: true},
		{name: "size resolution conflict", fields: [][2]string{{"size", "1280x720"}, {"resolution", "480p"}}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var jsonBody bytes.Buffer
			jsonBody.WriteByte('{')
			var parts []resolutionTestPart
			for i, field := range tc.fields {
				if i > 0 {
					jsonBody.WriteByte(',')
				}
				jsonBody.WriteString(`"` + field[0] + `":"` + field[1] + `"`)
				parts = append(parts, resolutionTestField(field[0], field[1]))
			}
			jsonBody.WriteByte('}')
			multipartBody, contentType := resolutionTestMultipart(t, parts)
			for _, request := range []struct {
				body        []byte
				contentType string
			}{
				{body: jsonBody.Bytes(), contentType: "application/json"},
				{body: multipartBody, contentType: contentType},
			} {
				original := bytes.Clone(request.body)
				err := validateOpenAIVideoOriginalResolution(request.body, request.contentType)
				require.Equal(t, original, request.body)
				if tc.wantError {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			}
		})
	}
}
