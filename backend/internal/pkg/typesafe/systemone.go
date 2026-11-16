package typesafe

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrStreamingUnsupported = errors.New("typesafe system one does not support streaming")

type systemOneEnvelope struct {
	Model     json.RawMessage `json:"model"`
	State     json.RawMessage `json:"state"`
	Questions json.RawMessage `json:"questions"`
	Stream    json.RawMessage `json:"stream"`
}

var (
	systemOneRequestFields  = []string{"model", "state", "questions", "stream"}
	systemOneQuestionFields = []string{"type", "instructions", "criteria"}
)

func ValidateSystemOneRequest(body []byte) (string, error) {
	var envelope systemOneEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", errors.New("invalid JSON request")
	}
	// encoding/json matches struct fields case-insensitively and keeps the last
	// duplicate, while the raw body is forwarded upstream unchanged. Reject any
	// spelling the upstream could read differently from this validator.
	if err := checkSystemOneObjectKeys(body, "request", systemOneRequestFields); err != nil {
		return "", err
	}

	model, err := requiredString(envelope.Model, "model")
	if err != nil {
		return "", err
	}
	if model != JevLatestModel {
		return "", fmt.Errorf("model must be %s", JevLatestModel)
	}
	if err := validateStringObjectOrArray(envelope.State, "state"); err != nil {
		return "", err
	}
	questionsRaw := bytes.TrimSpace(envelope.Questions)
	var questions map[string]json.RawMessage
	if len(questionsRaw) == 0 || questionsRaw[0] != '{' || json.Unmarshal(questionsRaw, &questions) != nil || len(questions) == 0 {
		return "", errors.New("questions must be a non-empty object")
	}
	if err := checkSystemOneObjectKeys(questionsRaw, "questions", nil); err != nil {
		return "", err
	}
	if len(envelope.Stream) > 0 && string(envelope.Stream) != "null" {
		var stream bool
		if err := json.Unmarshal(envelope.Stream, &stream); err != nil {
			return "", errors.New("stream must be a boolean")
		}
		if stream {
			return "", ErrStreamingUnsupported
		}
	}
	for id, raw := range questions {
		if err := validateQuestion(id, raw); err != nil {
			return "", err
		}
	}
	return model, nil
}

func validateQuestion(id string, raw json.RawMessage) error {
	raw = bytes.TrimSpace(raw)
	var question struct {
		Type         json.RawMessage `json:"type"`
		Instructions json.RawMessage `json:"instructions"`
		Criteria     json.RawMessage `json:"criteria"`
	}
	if len(raw) == 0 || raw[0] != '{' || json.Unmarshal(raw, &question) != nil {
		return fmt.Errorf("question %q must be an object", id)
	}
	if err := checkSystemOneObjectKeys(raw, fmt.Sprintf("question %q", id), systemOneQuestionFields); err != nil {
		return err
	}
	typ, err := requiredString(question.Type, "question type")
	if err != nil {
		return fmt.Errorf("question %q: %w", id, err)
	}
	// Mirror the wire schema generated from https://api.typesafe.ai/openapi.json
	// in TypeSafe SDK v0.5.7: instructions are optional and nullable for all types.
	if err := validateOptionalDescription(question.Instructions, "instructions"); err != nil {
		return fmt.Errorf("question %q: %w", id, err)
	}

	switch typ {
	case "noul":
		criteria := bytes.TrimSpace(question.Criteria)
		if len(criteria) == 0 || bytes.Equal(criteria, []byte("null")) {
			return nil
		}
		var descriptions map[string]json.RawMessage
		if criteria[0] != '{' || json.Unmarshal(criteria, &descriptions) != nil {
			return fmt.Errorf("question %q: noul criteria must be an object", id)
		}
		for _, outcome := range []string{"true", "false"} {
			if err := validateOptionalDescription(descriptions[outcome], "noul criteria "+outcome); err != nil {
				return fmt.Errorf("question %q: %w", id, err)
			}
		}
	case "choice":
		var criteria map[string]json.RawMessage
		if json.Unmarshal(question.Criteria, &criteria) != nil || criteria == nil {
			return fmt.Errorf("question %q: choice criteria must be an object", id)
		}
		for _, value := range criteria {
			if err := validateOptionalDescription(value, "choice criteria value"); err != nil {
				return fmt.Errorf("question %q: %w", id, err)
			}
		}
	case "score":
		var criteria []json.RawMessage
		if json.Unmarshal(question.Criteria, &criteria) != nil || len(criteria) == 0 {
			return fmt.Errorf("question %q: score criteria must contain at least one level", id)
		}
		for _, value := range criteria {
			if err := validateStringObjectOrArray(value, "score criteria value"); err != nil {
				return fmt.Errorf("question %q: %w", id, err)
			}
		}
	default:
		return fmt.Errorf("question %q: unsupported type %q", id, typ)
	}
	return nil
}

func validateOptionalDescription(raw json.RawMessage, name string) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	return validateStringObjectOrArray(raw, name)
}

func requiredString(raw json.RawMessage, name string) (string, error) {
	var value string
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s must be a non-empty string", name)
	}
	return value, nil
}

func validateStringObjectOrArray(raw json.RawMessage, name string) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return fmt.Errorf("%s is required", name)
	}
	switch raw[0] {
	case '{', '[':
		return nil
	case '"':
		if rawString(raw) {
			return nil
		}
	}
	return fmt.Errorf("%s must be a string, object, or array", name)
}

func rawString(raw json.RawMessage) bool {
	var value string
	return json.Unmarshal(raw, &value) == nil
}

// checkSystemOneObjectKeys rejects duplicate keys and non-canonical spellings
// (any case variant) of the known fields in one JSON object level.
func checkSystemOneObjectKeys(raw []byte, scope string, canonical []string) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("%s must be an object", scope)
	}
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return errors.New("invalid JSON request")
		}
		key, _ := token.(string)
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("%s contains duplicate field %q", scope, key)
		}
		seen[key] = struct{}{}
		for _, field := range canonical {
			if key != field && strings.EqualFold(key, field) {
				return fmt.Errorf("%s field %q must be written as %q", scope, key, field)
			}
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return errors.New("invalid JSON request")
		}
	}
	return nil
}
