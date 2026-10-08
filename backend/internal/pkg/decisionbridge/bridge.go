// Package decisionbridge translates only the lossless, non-streaming subset of
// OpenAI Decisions and TypeSafe System One. Unsupported evidence and metadata
// fail before dispatch rather than silently changing the question being asked.
package decisionbridge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	decisions "github.com/Wei-Shaw/sub2api/internal/pkg/openai_decisions"
	"github.com/Wei-Shaw/sub2api/internal/pkg/typesafe"
)

var ErrNonrepresentable = errors.New("decision bridge: nonrepresentable protocol value")

func reject(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrNonrepresentable, fmt.Sprintf(format, args...))
}

func object(raw json.RawMessage, fields ...string) (map[string]json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		return nil, reject("expected object")
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return nil, reject("invalid object")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	_, _ = decoder.Token() // json.Unmarshal above already validated the complete object.
	seen := make(map[string]bool, len(m))
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, reject("invalid object key")
		}
		key := token.(string)
		if seen[key] {
			return nil, reject("duplicate field %q", key)
		}
		seen[key] = true
		if len(fields) > 0 {
			found := false
			for _, f := range fields {
				if key == f {
					found = true
					break
				}
			}
			if !found {
				return nil, reject("unsupported field %q", key)
			}
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, reject("invalid object value")
		}
	}
	return m, nil
}
func str(raw json.RawMessage) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '"' {
		return "", reject("expected string")
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return "", reject("invalid string")
	}
	return s, nil
}
func encode(v any) ([]byte, error) { return json.Marshal(v) }

// ToSystemOne requires plain text evidence; TypeSafe's state schema is not an
// OpenAI message envelope. Safety identifiers cannot be sent to this upstream.
func ToSystemOne(body []byte) ([]byte, error) {
	req, err := decisions.ParseRequest(body)
	if err != nil {
		return nil, err
	}
	if _, err = object(body, "model", "input", "questions", "safety_identifier"); err != nil {
		return nil, err
	}
	if req.SafetyIdentifier != nil {
		return nil, reject("safety_identifier has no System One equivalent")
	}
	state, err := str(req.Input)
	if err != nil {
		return nil, reject("only plain text input is supported")
	}
	var raw struct {
		Questions []json.RawMessage `json:"questions"`
	}
	if err = json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	questions := map[string]any{}
	for i, q := range req.Questions {
		fields, err := object(raw.Questions[i], "type", "name", "instructions", "choices", "levels")
		if err != nil {
			return nil, err
		}
		for _, key := range []string{"name"} {
			if v, ok := fields[key]; ok && !bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
				if _, err = str(v); err != nil {
					return nil, err
				}
			}
		}
		id := "q" + strconv.Itoa(i)
		out := map[string]any{"instructions": q.Instructions}
		switch q.Type {
		case "predicate":
			out["type"] = "noul"
		case "choice":
			out["type"] = "choice"
			criteria := map[string]string{}
			// A string choice value is the option's label when no description is
			// supplied. Generated cN identifiers would erase that meaning.
			var parts []json.RawMessage
			if json.Unmarshal(fields["choices"], &parts) != nil || len(parts) != len(q.Choices) {
				return nil, reject("choice options must be objects")
			}
			for j, c := range q.Choices {
				if _, err = object(parts[j], "value", "description"); err != nil {
					return nil, err
				}
				key := choiceKey(q.Choices, j)
				if !allStringChoices(q.Choices) && c.Description == "" {
					return nil, reject("non-string choices require descriptions")
				}
				if key == "" {
					return nil, reject("empty choice label cannot be represented")
				}
				criteria[key] = c.Description
			}
			out["criteria"] = criteria
		case "score":
			out["type"] = "score"
			levels := make([]string, len(q.Levels))
			var parts []json.RawMessage
			if json.Unmarshal(fields["levels"], &parts) != nil || len(parts) != len(q.Levels) {
				return nil, reject("score levels must be objects")
			}
			for j, l := range q.Levels {
				if _, err = object(parts[j], "label", "description"); err != nil {
					return nil, err
				}
				levels[j] = l.Label
				if l.Description != "" {
					levels[j] += " — " + l.Description
				}
			}
			out["criteria"] = levels
		default:
			return nil, reject("unsupported question type")
		}
		questions[id] = out
	}
	result, err := encode(map[string]any{"model": typesafe.JevLatestModel, "state": state, "questions": questions})
	if err == nil {
		_, err = typesafe.ValidateSystemOneRequest(result)
	}
	return result, err
}
func mustJSON(v any) json.RawMessage { raw, _ := json.Marshal(v); return raw }

func allStringChoices(choices []decisions.Choice) bool {
	for _, choice := range choices {
		if _, ok := choice.Value.(string); !ok {
			return false
		}
	}
	return true
}

func choiceKey(choices []decisions.Choice, index int) string {
	if allStringChoices(choices) {
		return choices[index].Value.(string)
	}
	return "c" + strconv.Itoa(index)
}

// ToDecisions supports string state and string-only criteria. Structured state,
// rich rubrics, and unknown request fields have no lossless Decisions equivalent.
func ToDecisions(body []byte) ([]byte, error) {
	if _, err := typesafe.ValidateSystemOneRequest(body); err != nil {
		return nil, err
	}
	root, err := object(body, "model", "state", "questions", "stream")
	if err != nil {
		return nil, err
	}
	if raw, ok := root["stream"]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("false")) {
		return nil, reject("stream is unsupported")
	}
	state, err := str(root["state"])
	if err != nil {
		return nil, reject("only string state is supported")
	}
	rawQuestions, err := object(root["questions"])
	if err != nil {
		return nil, err
	}
	if len(rawQuestions) > 200 {
		return nil, reject("more than 200 questions")
	}
	keys := make([]string, 0, len(rawQuestions))
	for k := range rawQuestions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	questions := make([]any, 0, len(keys))
	for _, id := range keys {
		q, err := object(rawQuestions[id], "type", "instructions", "criteria")
		if err != nil {
			return nil, err
		}
		typ, _ := str(q["type"])
		instruction, err := str(q["instructions"])
		if err != nil {
			return nil, reject("question %q requires string instructions", id)
		}
		out := map[string]any{"name": id, "instructions": instruction}
		switch typ {
		case "noul":
			if raw, ok := q["criteria"]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return nil, reject("noul criteria cannot be represented")
			}
			out["type"] = "predicate"
		case "choice":
			out["type"] = "choice"
			rawChoices, e := object(q["criteria"])
			if e != nil || len(rawChoices) == 0 {
				return nil, reject("choice criteria required")
			}
			ids := make([]string, 0, len(rawChoices))
			for key := range rawChoices {
				ids = append(ids, key)
			}
			sort.Strings(ids)
			choices := make([]any, 0, len(ids))
			for _, key := range ids {
				desc, e := str(rawChoices[key])
				if e != nil {
					return nil, reject("choice description must be string")
				}
				choices = append(choices, map[string]any{"value": key, "description": desc})
			}
			out["choices"] = choices
		case "score":
			out["type"] = "score"
			var rawLevels []json.RawMessage
			if json.Unmarshal(q["criteria"], &rawLevels) != nil || len(rawLevels) == 0 {
				return nil, reject("score criteria required")
			}
			levels := make([]any, 0, len(rawLevels))
			for _, raw := range rawLevels {
				label, e := str(raw)
				if e != nil {
					return nil, reject("score criterion must be string")
				}
				levels = append(levels, map[string]any{"label": label})
			}
			out["levels"] = levels
		default:
			return nil, reject("question type %q", typ)
		}
		questions = append(questions, out)
	}
	result, err := encode(map[string]any{"model": decisions.Model, "input": state, "questions": questions})
	if err == nil {
		_, err = decisions.ParseRequest(result)
	}
	return result, err
}

func nonnegativeInt(raw json.RawMessage, field string) (int, error) {
	var n int
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &n) != nil || n < 0 {
		return 0, reject("usage %s must be a nonnegative integer", field)
	}
	return n, nil
}

func usage(raw json.RawMessage) (map[string]any, error) {
	m, err := object(raw, "input_tokens", "output_tokens", "total_tokens", "input_tokens_details", "output_tokens_details")
	if err != nil {
		return nil, err
	}
	result := map[string]any{}
	for _, k := range []string{"input_tokens", "output_tokens"} {
		n, err := nonnegativeInt(m[k], k)
		if err != nil {
			return nil, err
		}
		result[k] = n
	}
	if total, ok := m["total_tokens"]; ok {
		n, err := nonnegativeInt(total, "total_tokens")
		if err != nil {
			return nil, err
		}
		if n != result["input_tokens"].(int)+result["output_tokens"].(int) {
			return nil, reject("usage total_tokens mismatch")
		}
	}
	for detailsKey, fields := range map[string][]string{
		"input_tokens_details":  {"cached_tokens", "cache_write_tokens"},
		"output_tokens_details": {"reasoning_tokens"},
	} {
		if rawDetails, ok := m[detailsKey]; ok {
			details, err := object(rawDetails, fields...)
			if err != nil {
				return nil, err
			}
			for key, value := range details {
				n, err := nonnegativeInt(value, key)
				if err != nil {
					return nil, err
				}
				if n != 0 {
					return nil, reject("nonzero %s cannot be represented as System One usage", key)
				}
			}
		}
	}
	return result, nil
}
func probability(raw json.RawMessage) (float64, error) {
	var p float64
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &p) != nil || p < 0 || p > 1 {
		return 0, reject("invalid probability")
	}
	return p, nil
}

// FromSystemOne validates all answers before exposing a Decisions envelope.
func FromSystemOne(decisionsRequest, body []byte) ([]byte, error) {
	req, err := decisions.ParseRequest(decisionsRequest)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Model   string                     `json:"model"`
		Usage   json.RawMessage            `json:"usage"`
		Answers map[string]json.RawMessage `json:"answers"`
	}
	root, err := object(body, "model", "answers", "usage")
	if err != nil {
		return nil, err
	}
	if json.Unmarshal(body, &envelope) != nil || !strings.HasPrefix(envelope.Model, "jev-") || len(envelope.Answers) != len(req.Questions) {
		return nil, reject("invalid System One response envelope")
	}
	u, err := usage(envelope.Usage)
	if err != nil {
		return nil, err
	}
	answersMap, err := object(root["answers"])
	if err != nil || len(answersMap) != len(req.Questions) {
		return nil, reject("invalid System One answers")
	}
	answers := make([]any, len(req.Questions))
	for i, q := range req.Questions {
		id := "q" + strconv.Itoa(i)
		raw, ok := answersMap[id]
		if !ok {
			return nil, reject("missing answer %s", id)
		}
		a, err := object(raw, "type", "noul", "choice", "score", "confidence", "probabilities", "legend")
		if err != nil {
			return nil, err
		}
		allowed := []string{"type", "noul"}
		if q.Type == "choice" {
			allowed = []string{"type", "choice", "confidence", "probabilities"}
		} else if q.Type == "score" {
			allowed = []string{"type", "score", "confidence", "probabilities", "legend"}
		}
		if _, err = object(raw, allowed...); err != nil {
			return nil, err
		}
		typ, err := str(a["type"])
		if err != nil {
			return nil, err
		}
		out := map[string]any{"type": q.Type}
		if q.Name != nil {
			out["name"] = *q.Name
		}
		switch q.Type {
		case "predicate":
			if typ != "noul" {
				return nil, reject("answer type mismatch")
			}
			p, e := probability(a["noul"])
			if e != nil {
				return nil, e
			}
			out["probability"] = p
		case "choice", "score":
			if typ != q.Type {
				return nil, reject("answer type mismatch")
			}
			confidence, e := probability(a["confidence"])
			if e != nil {
				return nil, e
			}
			out["confidence"] = confidence
			probs, e := object(a["probabilities"])
			if e != nil {
				return nil, reject("missing or invalid probabilities")
			}
			count := len(q.Choices)
			if q.Type == "score" {
				count = len(q.Levels)
			}
			if len(probs) != count {
				return nil, reject("incomplete distribution")
			}
			list := make([]any, count)
			for j := 0; j < count; j++ {
				key := strconv.Itoa(j)
				if q.Type == "choice" {
					key = choiceKey(q.Choices, j)
				}
				p, e := probability(probs[key])
				if e != nil {
					return nil, e
				}
				if q.Type == "choice" {
					list[j] = map[string]any{"value": q.Choices[j].Value, "probability": p}
				} else {
					list[j] = map[string]any{"value": j, "label": q.Levels[j].Label, "probability": p}
				}
			}
			out["probabilities"] = list
			if q.Type == "choice" {
				sel, e := str(a["choice"])
				if e != nil {
					return nil, e
				}
				selected := false
				for j, choice := range q.Choices {
					key := choiceKey(q.Choices, j)
					if sel == key {
						out["choice"] = choice.Value
						selected = true
						break
					}
				}
				if !selected {
					return nil, reject("unknown choice")
				}
			} else {
				score, e := probabilityScore(a["score"])
				if e != nil {
					return nil, e
				}
				out["score"] = score
				legend, e := object(a["legend"])
				if e != nil || len(legend) != count {
					return nil, reject("invalid score legend")
				}
				for j, l := range q.Levels {
					want := l.Label
					if l.Description != "" {
						want += " — " + l.Description
					}
					v, e := str(legend[strconv.Itoa(j)])
					if e != nil || v != want {
						return nil, reject("score legend differs from criteria")
					}
				}
			}
		}
		answers[i] = out
	}
	u["total_tokens"] = u["input_tokens"].(int) + u["output_tokens"].(int)
	result, err := encode(map[string]any{"model": decisions.Model, "answers": answers, "usage": u})
	if err != nil {
		return nil, err
	}
	if _, err = decisions.DecodeResponse(decisionsRequest, result); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNonrepresentable, err)
	}
	return result, nil
}
func probabilityScore(raw json.RawMessage) (float64, error) {
	var p float64
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &p) != nil {
		return 0, reject("invalid score")
	}
	return p, nil
}

// FromDecisions validates the native answer first, then maps it to the native
// keyed System One response. Refusals have no TypeSafe answer representation.
func FromDecisions(systemOneRequest, decisionsRequest, body []byte) ([]byte, error) {
	if _, err := decisions.DecodeResponse(decisionsRequest, body); err != nil {
		return nil, err
	}
	if _, err := object(body, "model", "answers", "usage"); err != nil {
		return nil, err
	}
	var req struct {
		Questions []decisions.Question `json:"questions"`
	}
	_ = json.Unmarshal(decisionsRequest, &req)
	var resp struct {
		Answers []json.RawMessage `json:"answers"`
		Usage   json.RawMessage   `json:"usage"`
	}
	_ = json.Unmarshal(body, &resp)
	u, err := usage(resp.Usage)
	if err != nil {
		return nil, err
	}
	answers := map[string]any{}
	for i, q := range req.Questions {
		raw, err := object(resp.Answers[i], "type", "name", "probability", "choice", "confidence", "score", "probabilities")
		if err != nil {
			return nil, err
		}
		if name, present := raw["name"]; present {
			actual, e := str(name)
			if e != nil || q.Name == nil || actual != *q.Name {
				return nil, reject("answer name does not match request")
			}
		}
		allowed := []string{"type", "name", "probability"}
		if q.Type == "choice" {
			allowed = []string{"type", "name", "choice", "confidence", "probabilities"}
		} else if q.Type == "score" {
			allowed = []string{"type", "name", "score", "confidence", "probabilities"}
		}
		if _, err = object(resp.Answers[i], allowed...); err != nil {
			return nil, err
		}
		if q.Type == "choice" || q.Type == "score" {
			var entries []json.RawMessage
			if json.Unmarshal(raw["probabilities"], &entries) != nil {
				return nil, reject("invalid distribution")
			}
			fields := []string{"value", "probability"}
			if q.Type == "score" {
				fields = append(fields, "label")
			}
			for _, entry := range entries {
				if _, err = object(entry, fields...); err != nil {
					return nil, err
				}
			}
		}
		typ, _ := str(raw["type"])
		if typ == "refusal" {
			return nil, reject("refusal has no System One equivalent")
		}
		id := "q" + strconv.Itoa(i)
		out := map[string]any{}
		if q.Name != nil {
			id = *q.Name
		}
		switch q.Type {
		case "predicate":
			out["type"] = "noul"
			out["noul"] = json.RawMessage(raw["probability"])
		case "choice":
			out["type"] = "choice"
			out["confidence"] = json.RawMessage(raw["confidence"])
			var probs []struct {
				Value       any     `json:"value"`
				Probability float64 `json:"probability"`
			}
			_ = json.Unmarshal(raw["probabilities"], &probs)
			values := map[string]any{}
			for _, p := range probs {
				value, ok := p.Value.(string)
				if !ok {
					return nil, reject("choice key must be string")
				}
				values[value] = p.Probability
			}
			out["probabilities"] = values
			var selected any
			_ = json.Unmarshal(raw["choice"], &selected)
			found := false
			for _, c := range q.Choices {
				if bytes.Equal(mustJSON(c.Value), mustJSON(selected)) {
					out["choice"] = c.Value
					found = true
					break
				}
			}
			if !found {
				return nil, reject("unknown choice")
			}
		case "score":
			out["type"] = "score"
			out["score"] = json.RawMessage(raw["score"])
			out["confidence"] = json.RawMessage(raw["confidence"])
			var probs []struct {
				Value       float64 `json:"value"`
				Probability float64 `json:"probability"`
			}
			if json.Unmarshal(raw["probabilities"], &probs) != nil || len(probs) != len(q.Levels) {
				return nil, reject("invalid score distribution")
			}
			values := map[string]any{}
			legend := map[string]string{}
			seen := map[int]bool{}
			for _, p := range probs {
				if math.IsNaN(p.Value) || math.IsInf(p.Value, 0) || p.Value != math.Trunc(p.Value) || p.Value < 0 || p.Value >= float64(len(q.Levels)) {
					return nil, reject("score distribution value is not an integral level")
				}
				value := int(p.Value)
				if seen[value] {
					return nil, reject("duplicate score distribution value")
				}
				seen[value] = true
				values[strconv.Itoa(value)] = p.Probability
			}
			for j, l := range q.Levels {
				label := l.Label
				if l.Description != "" {
					label += " — " + l.Description
				}
				legend[strconv.Itoa(j)] = label
			}
			out["probabilities"] = values
			out["legend"] = legend
		}
		answers[id] = out
	}
	result, err := encode(map[string]any{"model": typesafe.JevLatestModel, "answers": answers, "usage": map[string]int{"input_tokens": u["input_tokens"].(int), "output_tokens": u["output_tokens"].(int)}})
	return result, err
}
