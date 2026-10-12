package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/textproto"
	"strconv"
	"strings"
)

type openAIVideoResolutionField struct {
	name  string
	value json.RawMessage
}

type openAIVideoResolutionPart struct {
	header     textproto.MIMEHeader
	data       []byte
	resolution bool
}

type openAIVideoResolutionBody struct {
	fields   []openAIVideoResolutionField
	parts    []openAIVideoResolutionPart
	boundary string
	tier     string
}

// Validate before prepareOpenAIVideoBody consumes size fields. The prepared-body
// helper cannot recover every original size occurrence from the single info.Size.
func validateOpenAIVideoOriginalResolution(body []byte, contentType string) error {
	_, err := parseOpenAIVideoResolutionBody(body, contentType)
	return err
}

func prepareOpenAIVideoResolution(account *Account, body []byte, contentType string, info GrokMediaRequestInfo) ([]byte, string, GrokMediaRequestInfo, error) {
	parsed, err := parseOpenAIVideoResolutionBody(body, contentType)
	if err != nil {
		return nil, "", info, err
	}
	grok := isOpenAIGrokVideoProtocol(account)
	if grok && info.Size != "" {
		if err := parsed.add("size", info.Size); err != nil {
			return nil, "", info, err
		}
	}
	if fixed := fixedOpenAIVideoResolution(info.Model); fixed != "" {
		// H3 models encode their actual output tier in the model ID. Ignore any
		// generic 720p/1080p billing hint supplied by an OpenAI client.
		parsed.tier = fixed
	} else if parsed.tier == "" {
		if !grok {
			return nil, "", info, fmt.Errorf("video requires an explicit billable size or resolution")
		}
		parsed.tier = VideoBillingResolution480P
	}
	info.Resolution = parsed.tier
	if !grok {
		return body, contentType, info, nil
	}
	out, err := parsed.withResolution()
	if err != nil {
		return nil, "", info, err
	}
	return out, contentType, info, nil
}

func (b *openAIVideoResolutionBody) add(name, value string) error {
	var tier string
	var ok bool
	if name == "size" {
		parts := strings.Split(strings.ToLower(strings.TrimSpace(value)), "x")
		if len(parts) == 2 {
			width, ew := strconv.Atoi(parts[0])
			height, eh := strconv.Atoi(parts[1])
			if ew == nil && eh == nil && width > 0 && height > 0 {
				tier, ok = LookupVideoBillingResolution(strconv.Itoa(min(width, height)) + "p")
			}
		}
	} else {
		tier, ok = LookupVideoBillingResolution(value)
	}
	if !ok {
		return fmt.Errorf("unsupported video %s: expected a 480p, 720p, 768p, 1080p or 1440p billing tier", name)
	}
	if b.tier != "" && b.tier != tier {
		return fmt.Errorf("video size and resolution fields must specify a consistent billing tier")
	}
	b.tier = tier
	return nil
}

func parseOpenAIVideoResolutionBody(body []byte, contentType string) (*openAIVideoResolutionBody, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, fmt.Errorf("invalid video content type: %w", err)
	}
	parsed := &openAIVideoResolutionBody{}
	if mediaType == "application/json" || strings.HasSuffix(mediaType, "+json") {
		decoder := json.NewDecoder(bytes.NewReader(body))
		token, err := decoder.Token()
		if err != nil || token != json.Delim('{') {
			return nil, fmt.Errorf("video request must be a JSON object")
		}
		// Decode each occurrence separately; a map would hide duplicate fields.
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, fmt.Errorf("invalid video JSON field: %w", err)
			}
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("invalid video JSON field name")
			}
			var value json.RawMessage
			if err := decoder.Decode(&value); err != nil {
				return nil, fmt.Errorf("invalid video JSON value: %w", err)
			}
			if name == "size" || name == "resolution" {
				var text string
				if err := json.Unmarshal(value, &text); err != nil {
					// Numeric resolution aliases (480/720/1080) are also known tiers.
					if name != "resolution" {
						return nil, fmt.Errorf("video size must be a string")
					}
					text = string(value)
				}
				if err := parsed.add(name, text); err != nil {
					return nil, err
				}
			}
			parsed.fields = append(parsed.fields, openAIVideoResolutionField{name: name, value: value})
		}
		if _, err := decoder.Token(); err != nil {
			return nil, fmt.Errorf("invalid video JSON object: %w", err)
		}
		if _, err := decoder.Token(); err != io.EOF {
			return nil, fmt.Errorf("unexpected trailing video JSON data")
		}
		return parsed, nil
	}
	if mediaType != "multipart/form-data" || params["boundary"] == "" {
		return nil, fmt.Errorf("video request must be JSON or multipart/form-data")
	}
	parsed.boundary = params["boundary"]
	reader := multipart.NewReader(bytes.NewReader(body), parsed.boundary)
	for {
		part, err := reader.NextRawPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid video multipart body: %w", err)
		}
		data, err := readOpenAIVideoMultipartPart(part)
		if err != nil {
			return nil, err
		}
		_ = part.Close()
		_, disposition, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		if err != nil {
			return nil, fmt.Errorf("invalid video multipart disposition: %w", err)
		}
		name := part.FormName()
		_, isFile := disposition["filename"]
		if !isFile && (name == "size" || name == "resolution") {
			value := data
			switch strings.ToLower(strings.TrimSpace(part.Header.Get("Content-Transfer-Encoding"))) {
			case "", "7bit", "8bit", "binary":
			case "quoted-printable":
				value, err = readOpenAIVideoMultipartPart(quotedprintable.NewReader(bytes.NewReader(data)))
				if err != nil {
					return nil, err
				}
			default:
				return nil, fmt.Errorf("unsupported video %s transfer encoding", name)
			}
			if err := parsed.add(name, string(value)); err != nil {
				return nil, err
			}
		}
		parsed.parts = append(parsed.parts, openAIVideoResolutionPart{
			header: part.Header, data: data, resolution: !isFile && name == "resolution",
		})
	}
	return parsed, nil
}

func (b *openAIVideoResolutionBody) withResolution() ([]byte, error) {
	var out bytes.Buffer
	hasResolution := false
	if b.boundary == "" {
		out.WriteByte('{')
		for i, field := range b.fields {
			if i > 0 {
				out.WriteByte(',')
			}
			key, _ := json.Marshal(field.name)
			out.Write(key)
			out.WriteByte(':')
			if field.name == "resolution" {
				value, _ := json.Marshal(b.tier)
				out.Write(value)
				hasResolution = true
			} else {
				out.Write(field.value)
			}
		}
		if !hasResolution {
			if len(b.fields) > 0 {
				out.WriteByte(',')
			}
			out.WriteString(`"resolution":"` + b.tier + `"`)
		}
		out.WriteByte('}')
		return out.Bytes(), nil
	}
	writer := multipart.NewWriter(&out)
	if err := writer.SetBoundary(b.boundary); err != nil {
		return nil, err
	}
	for _, part := range b.parts {
		dst, err := writer.CreatePart(part.header)
		if err != nil {
			return nil, err
		}
		data := part.data
		if part.resolution {
			// Canonical tiers contain only QP-safe ASCII; retain the original headers.
			data = []byte(b.tier)
			hasResolution = true
		}
		if _, err := dst.Write(data); err != nil {
			return nil, err
		}
	}
	if !hasResolution {
		if err := writer.WriteField("resolution", b.tier); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
