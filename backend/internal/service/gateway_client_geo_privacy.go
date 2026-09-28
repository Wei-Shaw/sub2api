package service

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// These are metadata keys, not words to remove from arbitrary user content.
var clientGeoMetadataKeys = []string{
	"timezone", "time_zone", "user_timezone", "client_timezone", "local_timezone", "tz",
	"client_ip", "ip", "ip_address", "public_ip", "location", "geolocation", "client_location", "user_location",
	"country", "country_code", "region", "city", "latitude", "longitude", "postal_code",
	"locale", "client_locale", "user_locale",
}

var clientGeoFieldTags = func() []*regexp.Regexp {
	patterns := make([]*regexp.Regexp, 0, len(clientGeoMetadataKeys))
	for _, name := range clientGeoMetadataKeys {
		patterns = append(patterns, regexp.MustCompile(`(?is)^[ \t]*<`+name+`\s*>[^<]*</`+name+`\s*>[ \t]*(?:\r?\n|$)`))
	}
	return patterns
}()

var clientEnvironmentTag = regexp.MustCompile(`</?[A-Za-z_][A-Za-z0-9_.:-]*(?:\s[^<>]*)?\s*/?>`)

var clientGeoFieldLine = regexp.MustCompile(`(?i)^[ \t]*(?:[-*][ \t]+)?(?:user[ _-]+|client[ _-]+|local[ _-]+)?(?:time[ _-]?zone|tz|ip(?:[ _-]?address)?|public[ _-]?ip|country(?:[ _-]?code)?|region|city|location|geolocation|latitude|longitude|postal[ _-]?code)[ \t]*:[^\r\n]*$`)

func (s *GatewayService) clientGeoPrivacyEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.RedactClientGeoMetadata
}

// redactClientGeoMetadata only edits provider metadata and recognized client
// environment envelopes. Tool inputs/results, assistant messages, image data,
// ordinary text and paths are deliberately outside its scope. No location is
// fabricated, and the caller's JSON bytes are retained when no edit is needed.
func (s *GatewayService) redactClientGeoMetadata(body []byte) ([]byte, error) {
	if !s.clientGeoPrivacyEnabled() {
		return body, nil
	}
	return RedactClientGeoMetadata(body)
}

// RedactClientGeoMetadata filters known metadata locations in Anthropic,
// Responses, Chat Completions and Gemini requests. It never recursively walks
// arbitrary task objects. Callers own the opt-in configuration check.
func RedactClientGeoMetadata(body []byte) ([]byte, error) {
	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("invalid JSON while filtering client environment metadata")
	}
	out := body
	root := gjson.ParseBytes(body)
	requests := []struct {
		prefix string
		value  gjson.Result
	}{{"", root}, {"request.", root.Get("request")}, {"response.", root.Get("response")}}
	for _, request := range requests {
		metadata := request.value.Get("metadata")
		if !metadata.IsObject() {
			continue
		}
		for _, key := range clientGeoMetadataKeys {
			path := request.prefix + "metadata." + key
			if metadata.Get(key).Exists() {
				var err error
				out, err = sjson.DeleteBytes(out, path)
				if err != nil {
					return nil, fmt.Errorf("remove client metadata field: %w", err)
				}
			}
		}
	}
	type edit struct{ path, text string }
	var edits []edit
	collectText := func(value gjson.Result, path string, system bool) {
		if value.Type != gjson.String {
			return
		}
		original := value.String()
		if redacted := redactAutomaticEnvironmentText(original, system); redacted != original {
			edits = append(edits, edit{path, redacted})
		}
	}
	collectContent := func(value gjson.Result, path string, system bool) {
		if value.Type == gjson.String {
			collectText(value, path, system)
			return
		}
		if !value.IsArray() {
			return
		}
		value.ForEach(func(index, block gjson.Result) bool {
			if kind := block.Get("type").String(); kind == "text" || kind == "input_text" {
				collectText(block.Get("text"), path+"."+strconv.Itoa(int(index.Int()))+".text", system)
			}
			return true
		})
	}
	for _, request := range requests {
		if !request.value.IsObject() {
			continue
		}
		prefix, object := request.prefix, request.value
		collectContent(object.Get("system"), prefix+"system", true)
		collectText(object.Get("instructions"), prefix+"instructions", true)
		collectText(object.Get("input"), prefix+"input", false)
		for _, key := range []string{"messages", "input"} {
			value := object.Get(key)
			if !value.IsArray() {
				continue
			}
			value.ForEach(func(index, message gjson.Result) bool {
				role := message.Get("role").String()
				kind := message.Get("type").String()
				if (kind == "" || kind == "message") && (role == "user" || role == "system" || role == "developer") {
					collectContent(message.Get("content"), prefix+key+"."+strconv.Itoa(int(index.Int()))+".content", role != "user")
				}
				return true
			})
		}
		collectParts := func(value gjson.Result, path string, system bool) {
			if !value.IsArray() {
				return
			}
			value.ForEach(func(index, part gjson.Result) bool {
				// Function calls/responses and inline/file data have no direct text.
				collectText(part.Get("text"), path+"."+strconv.Itoa(int(index.Int()))+".text", system)
				return true
			})
		}
		for _, key := range []string{"systemInstruction", "system_instruction"} {
			collectParts(object.Get(key+".parts"), prefix+key+".parts", true)
		}
		if contents := object.Get("contents"); contents.IsArray() {
			contents.ForEach(func(index, message gjson.Result) bool {
				if role := message.Get("role").String(); role == "user" || role == "" {
					collectParts(message.Get("parts"), prefix+"contents."+strconv.Itoa(int(index.Int()))+".parts", false)
				}
				return true
			})
		}
	}
	for _, item := range edits {
		var err error
		out, err = sjson.SetBytes(out, item.path, item.text)
		if err != nil {
			return nil, fmt.Errorf("filter client environment field: %w", err)
		}
	}
	return out, nil
}

// Standalone envelopes are emitted by Codex and OpenCode. Free-form messages
// merely discussing a timezone, including quoted/fenced examples, are preserved.
// System instructions can embed the envelope among other instructions. The
// OAuth adapter's [System Instructions] wrapper preserves this same provenance.
func redactAutomaticEnvironmentText(text string, system bool) string {
	trimmed := strings.TrimSpace(text)
	wrappedSystem := strings.HasPrefix(trimmed, "[System Instructions]\n") || strings.HasPrefix(trimmed, "[System Instructions]\r\n")
	for _, tag := range []string{"environment_context", "environment", "env"} {
		open, close := "<"+tag+">", "</"+tag+">"
		if !system && !wrappedSystem && (!strings.HasPrefix(trimmed, open) || !strings.HasSuffix(trimmed, close)) {
			continue
		}
		cursor := 0
		var scope clientMetadataScope
		for cursor < len(text) {
			relative := strings.Index(text[cursor:], open)
			if relative < 0 {
				break
			}
			start := cursor + relative
			contentStart := start + len(open)
			endRelative := strings.Index(text[contentStart:], close)
			if endRelative < 0 {
				break
			}
			end := contentStart + endRelative
			// Scan each prefix once and require the envelope to start a line.
			for _, line := range strings.Split(text[cursor:start], "\n") {
				scope.consume(line)
			}
			lineStart := cursor + strings.LastIndex(text[cursor:start], "\n") + 1
			if scope.fence.size == 0 && !scope.comment && len(scope.stack) == 0 && strings.TrimSpace(text[lineStart:start]) == "" {
				original := text[contentStart:end]
				replacement := redactEnvironmentFields(original)
				text = text[:contentStart] + replacement + text[end:]
				end = contentStart + len(replacement)
			}
			cursor = end + len(close)
		}
	}
	return text
}

// Track enclosing documents conservatively so a system prompt's quoted file
// containing an environment example is not mistaken for client metadata.
type clientMetadataScope struct {
	fence   clientMetadataFence
	stack   []string
	comment bool
}

func (s *clientMetadataScope) consume(line string) {
	if s.comment || strings.Contains(line, "<!--") {
		s.comment = !strings.Contains(line, "-->")
		return
	}
	if s.fence.consume(line) {
		return
	}
	for _, tag := range clientEnvironmentTag.FindAllString(line, -1) {
		if strings.HasSuffix(tag, "/>") {
			continue
		}
		name := strings.Fields(strings.Trim(tag, "<>/ \t\r\n"))[0]
		if strings.HasPrefix(tag, "</") {
			if len(s.stack) > 0 && s.stack[len(s.stack)-1] == name {
				s.stack = s.stack[:len(s.stack)-1]
			}
		} else {
			s.stack = append(s.stack, name)
		}
	}
}

type clientMetadataFence struct {
	marker byte
	size   int
}

// consume reports whether the line is a fence or is inside one. A shorter
// fence (or one of the other kind) does not close a Markdown code block.
func (f *clientMetadataFence) consume(line string) bool {
	line = strings.TrimSpace(line)
	inside := f.size != 0
	if len(line) < 3 || (line[0] != '`' && line[0] != '~') {
		return inside
	}
	n := 0
	for n < len(line) && line[n] == line[0] {
		n++
	}
	if n < 3 {
		return inside
	}
	if f.size == 0 {
		f.marker, f.size = line[0], n
	} else if line[0] == f.marker && n >= f.size && strings.TrimSpace(line[n:]) == "" {
		f.size = 0
	}
	return true
}

func redactEnvironmentFields(content string) string {
	// Only direct, whole-line metadata fields are recognized. Nested documents,
	// inline prose and fenced examples stay untouched. Leaf XML values may span
	// lines; no arbitrary task text is searched for location names or addresses.
	var fence clientMetadataFence
	var stack []string
	var out strings.Builder
	inComment := false
	for len(content) > 0 {
		n := strings.IndexByte(content, '\n') + 1
		if n == 0 {
			n = len(content)
		}
		line := content[:n]
		if inComment || strings.Contains(line, "<!--") {
			inComment = !strings.Contains(line, "-->")
			_, _ = out.WriteString(line)
			content = content[n:]
			continue
		}
		if !fence.consume(line) {
			removed := 0
			if len(stack) == 0 {
				if clientGeoFieldLine.MatchString(strings.TrimRight(line, "\r\n")) {
					removed = n
				}
				if strings.HasPrefix(strings.TrimSpace(line), "<") {
					for _, pattern := range clientGeoFieldTags {
						if match := pattern.FindStringIndex(content); match != nil {
							candidate := content[:match[1]]
							if !strings.Contains(candidate, "```") && !strings.Contains(candidate, "~~~") {
								removed = match[1]
							}
							break
						}
					}
				}
			}
			if removed > 0 {
				content = content[removed:]
				continue
			}
			for _, tag := range clientEnvironmentTag.FindAllString(line, -1) {
				if strings.HasSuffix(tag, "/>") {
					continue
				}
				name := strings.Trim(tag, "<>/ \t\r\n")
				name = strings.Fields(name)[0]
				if strings.HasPrefix(tag, "</") {
					// On ambiguous/malformed nesting, preserve the remaining text.
					if len(stack) == 0 || stack[len(stack)-1] != name {
						return out.String() + content
					}
					stack = stack[:len(stack)-1]
				} else {
					stack = append(stack, name)
				}
			}
		}
		_, _ = out.WriteString(line)
		content = content[n:]
	}
	return out.String()
}

func (s *GatewayService) redactClientGeoHeaders(headers http.Header) {
	if !s.clientGeoPrivacyEnabled() {
		return
	}
	RedactClientGeoHeaders(headers)
}

// RedactClientGeoHeaders applies after account header overrides and before
// transport. Header casing cannot bypass the filter.
func RedactClientGeoHeaders(headers http.Header) {
	for key := range headers {
		lower := strings.ToLower(key)
		switch lower {
		case "forwarded", "x-forwarded-for", "x-real-ip", "true-client-ip", "cf-connecting-ip",
			"cf-ipcountry", "accept-language", "x-timezone", "x-time-zone", "x-client-timezone", "x-client-ip",
			"x-vercel-ip-country", "x-vercel-ip-country-region", "x-vercel-ip-city", "x-vercel-ip-latitude", "x-vercel-ip-longitude":
			delete(headers, key)
		}
	}
}

// ApplyClientGeoPrivacy is the shared HTTP egress boundary for all providers,
// including native and compatible APIs. Binary/multipart uploads remain streams.
// AWS-signed bodies must already be filtered by their builder, before signing.
func ApplyClientGeoPrivacy(req *http.Request, cfg *config.Config) error {
	if req == nil || cfg == nil || !cfg.Gateway.RedactClientGeoMetadata {
		return nil
	}
	RedactClientGeoHeaders(req.Header)
	mediaType, _, _ := mime.ParseMediaType(getHeaderRaw(req.Header, "Content-Type"))
	if req.Body == nil || (mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json")) {
		return nil
	}
	// All gateway JSON requests are bounded and buffered by ingress/builders.
	// Reuse GetBody where possible; this also leaves unchanged requests intact.
	reader := req.Body
	if req.GetBody != nil {
		var err error
		reader, err = req.GetBody()
		if err != nil {
			return fmt.Errorf("read client metadata for privacy filter: %w", err)
		}
		defer func() { _ = reader.Close() }()
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read client metadata for privacy filter: %w", err)
	}
	filtered, err := RedactClientGeoMetadata(body)
	if err != nil {
		return err
	}
	if bytes.Equal(body, filtered) && req.GetBody != nil {
		return nil
	}
	if !bytes.Equal(body, filtered) && strings.HasPrefix(getHeaderRaw(req.Header, "Authorization"), "AWS4-HMAC-SHA256 ") {
		return fmt.Errorf("client metadata must be filtered before signing the upstream request")
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(filtered))
	req.ContentLength = int64(len(filtered))
	deleteHeaderAllForms(req.Header, "Content-Length")
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(filtered)), nil }
	return nil
}
