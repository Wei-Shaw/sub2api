package service

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const codexTimezoneExtraKey = "codex_timezone"

// Missing/empty inherits the global default; off explicitly preserves the client.
func ValidateCodexTimezoneExtra(extra map[string]any) error {
	raw, exists := extra[codexTimezoneExtraKey]
	if !exists || raw == nil {
		return nil
	}
	if value, ok := raw.(string); ok {
		if strings.TrimSpace(value) == "off" {
			return nil
		}
		if _, err := NormalizeCodexTimezone(value); err == nil {
			return nil
		}
	}
	return infraerrors.BadRequest("INVALID_CODEX_TIMEZONE", "codex_timezone must be an IANA timezone, empty to inherit, or off to preserve the client")
}

// NormalizeCodexTimezone accepts an explicit IANA timezone or an empty override.
// Local is deliberately excluded: it would silently depend on the server timezone.
func NormalizeCodexTimezone(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if value != "Local" && len(value) <= 128 {
		if _, err := time.LoadLocation(value); err == nil {
			return value, nil
		}
	}
	return "", fmt.Errorf("openai_codex_timezone must be an IANA timezone such as Asia/Taipei, or empty")
}

func (s *SettingService) GetOpenAICodexTimezone(ctx context.Context) string {
	if s == nil || s.settingRepo == nil {
		return ""
	}
	return s.getGatewayForwardingSettingsCached(ctx).openAICodexTimezone
}

var codexEnvironmentContextPattern = regexp.MustCompile(`(?s)<environment_context>.*?</environment_context>|<codex_apps_client_time_context>.*?</codex_apps_client_time_context>`)
var codexEnvironmentTimezonePattern = regexp.MustCompile(`<timezone>[^<]*</timezone>`)
var codexEnvironmentDatePattern = regexp.MustCompile(`<current_date>[^<]*</current_date>`)
var codexEnvironmentTimePattern = regexp.MustCompile(`<current_time>[^<]*</current_time>`)

func rewriteCodexEnvironmentTimezoneText(text, timezone string, now time.Time, seen map[string]bool) string {
	// Walk backwards, refreshing the latest occurrence of each field. Codex can
	// send incremental contexts: a later cwd/timezone-only update must not hide
	// the active date field from an earlier block. Older occurrences stay intact.
	overrides := []struct {
		pattern *regexp.Regexp
		value   string
	}{
		{codexEnvironmentTimezonePattern, "<timezone>" + timezone + "</timezone>"},
		{codexEnvironmentDatePattern, "<current_date>" + now.Format("2006-01-02") + "</current_date>"},
		{codexEnvironmentTimePattern, "<current_time>" + now.Format(time.RFC3339) + "</current_time>"},
	}
	matches := codexEnvironmentContextPattern.FindAllStringIndex(text, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		start, end := matches[i][0], matches[i][1]
		block := text[start:end]
		kind := block[:strings.IndexByte(block, '>')+1]
		next := block
		for _, override := range overrides {
			key := kind + override.pattern.String()
			if !seen[key] && override.pattern.MatchString(next) {
				seen[key] = true
				next = override.pattern.ReplaceAllString(next, override.value)
			}
		}
		if next != block {
			text = text[:start] + next + text[end:]
		}
	}
	return text
}

// rewriteCodexEnvironmentTimezone only touches text in request instructions and
// user/system/developer messages. Tool results, assistant messages, ordinary prose,
// historical contexts and unrelated JSON fields retain their original contents.
// Current dates/times use the server clock in the selected timezone. Missing
// fields are not injected. JSON path edits preserve large integers and raw fields.
func rewriteCodexEnvironmentTimezone(body []byte, timezone string) ([]byte, error) {
	return rewriteCodexEnvironmentTimezoneAt(body, timezone, time.Now())
}

func rewriteCodexEnvironmentTimezoneAt(body []byte, timezone string, now time.Time) ([]byte, error) {
	if timezone == "" {
		return body, nil
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("load Codex environment timezone: %w", err)
	}
	now = now.In(location)
	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("invalid JSON for Codex timezone override")
	}
	root := gjson.ParseBytes(body)
	type textField struct{ path, text string }
	var fields []textField
	collectText := func(path string, value gjson.Result) {
		if value.Type != gjson.String {
			return
		}
		fields = append(fields, textField{path, value.Str})
	}
	rewriteMessage := func(path string, message gjson.Result) {
		role := message.Get("role").String()
		if role != "user" && role != "system" && role != "developer" {
			return
		}
		if kind := message.Get("type").String(); kind != "" && kind != "message" {
			return
		}
		content := message.Get("content")
		if content.Type == gjson.String {
			collectText(path+".content", content)
			return
		}
		for i, part := range content.Array() {
			if kind := part.Get("type").String(); kind == "input_text" || kind == "text" {
				collectText(path+".content."+strconv.Itoa(i)+".text", part.Get("text"))
			}
		}
	}
	collectText("instructions", root.Get("instructions"))
	input := root.Get("input")
	if input.Type == gjson.String {
		collectText("input", input)
	} else {
		for i, message := range input.Array() {
			rewriteMessage("input."+strconv.Itoa(i), message)
		}
	}
	// Realtime-style WS frames may carry context outside response.create.input.
	switch root.Get("type").String() {
	case "session.update":
		collectText("session.instructions", root.Get("session.instructions"))
	case "conversation.item.create":
		rewriteMessage("item", root.Get("item"))
	}
	out := body
	seen := make(map[string]bool, 6)
	for i := len(fields) - 1; i >= 0; i-- {
		field := fields[i]
		next := rewriteCodexEnvironmentTimezoneText(field.text, timezone, now, seen)
		if next != field.text {
			out, err = sjson.SetBytes(out, field.path, next)
			if err != nil {
				return nil, fmt.Errorf("rewrite Codex environment time: %w", err)
			}
		}
	}
	return out, nil
}

func (s *OpenAIGatewayService) applyCodexTimezone(ctx context.Context, account *Account, body []byte) ([]byte, error) {
	if s == nil || account == nil || !account.IsOpenAIOAuthLike() {
		return body, nil
	}
	value, _ := account.Extra[codexTimezoneExtraKey].(string)
	value = strings.TrimSpace(value)
	if value == "off" {
		return body, nil
	}
	if value == "" {
		value = s.settingService.GetOpenAICodexTimezone(ctx)
	}
	return rewriteCodexEnvironmentTimezone(body, value)
}
