package handler

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

type bootstrapSpan struct {
	start, end int
	kind       byte
}
type bootstrapItem struct {
	bootstrapSpan
	fields                                   [6]bootstrapSpan
	typ, namespace, name, callID, id, output string
	automation, delegation                   bool
}

const (
	bootstrapType = iota
	bootstrapNamespace
	bootstrapName
	bootstrapCallID
	bootstrapID
	bootstrapOutput
)

func bootstrapField(key string) int {
	switch key {
	case "type":
		return bootstrapType
	case "namespace":
		return bootstrapNamespace
	case "name":
		return bootstrapName
	case "call_id":
		return bootstrapCallID
	case "id":
		return bootstrapID
	case "output":
		return bootstrapOutput
	}
	return -1
}

// normalizeCodexBootstrapKinds validates and indexes the original JSON once.
// Content strings (including image data) are scanned but not materialized. Only
// potential bootstrap outputs are decoded. Changes are decided after validation
// and history checks, then raw unchanged spans are copied into one final body.
func normalizeCodexBootstrapKinds(body []byte, automation, delegation bool) ([]byte, bool, bool) {
	scan := bootstrapScanner{body: body}
	_, err := scan.value(0, true, false, nil)
	scan.space()
	if err != nil || scan.pos != len(body) || !scan.rootObject {
		return body, false, false
	}
	for i := range scan.items {
		it := &scan.items[i]
		it.typ = scan.text(it.fields[bootstrapType])
		it.namespace = scan.text(it.fields[bootstrapNamespace])
		it.name = scan.text(it.fields[bootstrapName])
		it.callID = scan.text(it.fields[bootstrapCallID])
		it.id = scan.text(it.fields[bootstrapID])
		if it.typ != "function_call_output" || it.fields[bootstrapOutput].kind != '"' {
			continue
		}
		possibleAuto := automation && it.namespace == "codex_app" && it.name == "automation_update"
		possibleDelegation := delegation && isCodexDelegationTool(it.namespace, it.name)
		if !possibleAuto && !possibleDelegation {
			continue
		}
		it.output = scan.text(it.fields[bootstrapOutput])
		it.automation = possibleAuto && (validCodexAutomationBootstrap(it.output) || validCodexAutomationHeartbeat(it.output))
		it.delegation = possibleDelegation && validCodexDelegationEnvelope(it.output)
	}
	previousOK := scan.previous.kind == 0 || scan.previous.kind == '"'
	autoOK := automation && previousOK && strings.TrimSpace(scan.text(scan.previous)) == ""
	autoCount := 0
	for _, it := range scan.items {
		if it.automation {
			autoCount++
			if it.fields[bootstrapCallID].kind != 0 && (it.fields[bootstrapCallID].kind != '"' || strings.TrimSpace(it.callID) != "") {
				autoOK = false
			}
		} else if it.typ == "item_reference" || strings.HasSuffix(it.typ, "_call") || isResponsesCallOutputType(it.typ) {
			autoOK = false
		}
	}
	autoChanged := autoOK && autoCount > 0
	delegationOK := delegation && previousOK
	delegationCount := 0
	for _, it := range scan.items {
		// Model the original second pass after automation has turned its items into messages.
		if autoChanged && it.automation {
			continue
		}
		if it.delegation {
			delegationCount++
			if it.fields[bootstrapCallID].kind != 0 && (it.fields[bootstrapCallID].kind != '"' || strings.TrimSpace(it.callID) != "") {
				delegationOK = false
			}
		} else if it.typ == "item_reference" {
			if strings.TrimSpace(it.id) == "" {
				delegationOK = false
			}
		} else if strings.HasSuffix(it.typ, "_call") || isResponsesCallOutputType(it.typ) {
			if strings.TrimSpace(it.callID) == "" {
				delegationOK = false
			}
		}
	}
	delegationChanged := delegationOK && delegationCount > 0
	if !autoChanged && !delegationChanged {
		return body, false, false
	}
	type edit struct {
		bootstrapSpan
		replacement []byte
	}
	var edits []edit
	size := len(body)
	for _, it := range scan.items {
		changed := autoChanged && it.automation || delegationChanged && it.delegation
		if !changed {
			continue
		}
		replacement, err := json.Marshal(map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": it.output}}})
		if err != nil {
			return body, false, false
		}
		edits = append(edits, edit{it.bootstrapSpan, replacement})
		size += len(replacement) - (it.end - it.start)
	}
	out := make([]byte, 0, size)
	from := 0
	for _, e := range edits {
		out = append(out, body[from:e.start]...)
		out = append(out, e.replacement...)
		from = e.end
	}
	out = append(out, body[from:]...)
	return out, autoChanged, delegationChanged
}

var errBootstrapJSON = errors.New("invalid or duplicate-member bootstrap JSON")

type bootstrapScanner struct {
	body       []byte
	pos        int
	rootObject bool
	previous   bootstrapSpan
	items      []bootstrapItem
}

func (p *bootstrapScanner) space() {
	for p.pos < len(p.body) {
		switch p.body[p.pos] {
		case ' ', '\t', '\r', '\n':
			p.pos++
		default:
			return
		}
	}
}
func (p *bootstrapScanner) text(s bootstrapSpan) string {
	if s.kind != '"' {
		return ""
	}
	var text string
	_ = json.Unmarshal(p.body[s.start:s.end], &text)
	return text
}
func (p *bootstrapScanner) stringToken() error {
	p.pos++
	for p.pos < len(p.body) {
		c := p.body[p.pos]
		p.pos++
		if c == '"' {
			return nil
		}
		if c < 0x20 {
			return errBootstrapJSON
		}
		if c != '\\' {
			continue
		}
		if p.pos == len(p.body) {
			return errBootstrapJSON
		}
		c = p.body[p.pos]
		p.pos++
		switch c {
		case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
		case 'u':
			for j := 0; j < 4; j++ {
				if p.pos == len(p.body) {
					return errBootstrapJSON
				}
				h := p.body[p.pos]
				switch {
				case h >= '0' && h <= '9', h >= 'a' && h <= 'f', h >= 'A' && h <= 'F':
				default:
					return errBootstrapJSON
				}
				p.pos++
			}
		default:
			return errBootstrapJSON
		}
	}
	return errBootstrapJSON
}
func (p *bootstrapScanner) number() error {
	startNumber := p.pos
	if p.body[p.pos] == '-' {
		p.pos++
	}
	if p.pos == len(p.body) {
		return errBootstrapJSON
	}
	if p.body[p.pos] == '0' {
		p.pos++
	} else {
		if p.body[p.pos] < '1' || p.body[p.pos] > '9' {
			return errBootstrapJSON
		}
		p.digits()
	}
	if p.pos < len(p.body) && p.body[p.pos] == '.' {
		p.pos++
		start := p.pos
		p.digits()
		if start == p.pos {
			return errBootstrapJSON
		}
	}
	if p.pos < len(p.body) && (p.body[p.pos] == 'e' || p.body[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.body) && (p.body[p.pos] == '+' || p.body[p.pos] == '-') {
			p.pos++
		}
		start := p.pos
		p.digits()
		if start == p.pos {
			return errBootstrapJSON
		}
	}
	// The original duplicate-member check used Decoder.Token without
	// UseNumber, so it also rejected numbers outside float64's range.
	if _, err := strconv.ParseFloat(string(p.body[startNumber:p.pos]), 64); err != nil {
		return errBootstrapJSON
	}
	return nil
}
func (p *bootstrapScanner) digits() {
	for p.pos < len(p.body) && p.body[p.pos] >= '0' && p.body[p.pos] <= '9' {
		p.pos++
	}
}

// root/input describe structural position, never an arbitrary nested key named input.
func (p *bootstrapScanner) value(depth int, root, input bool, item *bootstrapItem) (bootstrapSpan, error) {
	p.space()
	s := bootstrapSpan{start: p.pos}
	if p.pos == len(p.body) {
		return s, errBootstrapJSON
	}
	s.kind = p.body[p.pos]
	switch s.kind {
	case '{', '[':
		if depth >= 10000 {
			return s, errBootstrapJSON
		}
		object := s.kind == '{'
		end := byte(']')
		if object {
			end = '}'
			if root {
				p.rootObject = true
			}
		}
		p.pos++
		p.space()
		if p.pos < len(p.body) && p.body[p.pos] == end {
			p.pos++
			break
		}
		var keys map[string]struct{}
		if object {
			keys = make(map[string]struct{})
		}
		for {
			key := ""
			if object {
				p.space()
				if p.pos == len(p.body) || p.body[p.pos] != '"' {
					return s, errBootstrapJSON
				}
				ks := bootstrapSpan{start: p.pos, kind: '"'}
				if err := p.stringToken(); err != nil {
					return s, err
				}
				ks.end = p.pos
				key = p.text(ks)
				if _, ok := keys[key]; ok {
					return s, errBootstrapJSON
				}
				keys[key] = struct{}{}
				p.space()
				if p.pos == len(p.body) || p.body[p.pos] != ':' {
					return s, errBootstrapJSON
				}
				p.pos++
			}
			var target *bootstrapItem
			if !object && input {
				target = &bootstrapItem{}
			}
			v, err := p.value(depth+1, false, root && object && key == "input", target)
			if err != nil {
				return s, err
			}
			if root && object && key == "previous_response_id" {
				p.previous = v
			}
			if object && item != nil {
				if index := bootstrapField(key); index >= 0 {
					item.fields[index] = v
				}
			}
			if target != nil && v.kind == '{' {
				target.bootstrapSpan = v
				p.items = append(p.items, *target)
			}
			p.space()
			if p.pos == len(p.body) {
				return s, errBootstrapJSON
			}
			c := p.body[p.pos]
			p.pos++
			if c == end {
				break
			}
			if c != ',' {
				return s, errBootstrapJSON
			}
		}
	case '"':
		if err := p.stringToken(); err != nil {
			return s, err
		}
	case 't', 'f', 'n':
		lit := "null"
		switch s.kind {
		case 't':
			lit = "true"
		case 'f':
			lit = "false"
		}
		if len(p.body)-p.pos < len(lit) || string(p.body[p.pos:p.pos+len(lit)]) != lit {
			return s, errBootstrapJSON
		}
		p.pos += len(lit)
	default:
		if err := p.number(); err != nil {
			return s, err
		}
	}
	s.end = p.pos
	return s, nil
}
