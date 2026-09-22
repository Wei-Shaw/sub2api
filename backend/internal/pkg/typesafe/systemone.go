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
	Model     json.RawMessage            `json:"model"`
	State     json.RawMessage            `json:"state"`
	Questions map[string]json.RawMessage `json:"questions"`
	Stream    json.RawMessage            `json:"stream"`
}

func ValidateSystemOneRequest(body []byte) (string, error) {
	var envelope systemOneEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", errors.New("invalid JSON request")
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
	if len(envelope.Questions) == 0 {
		return "", errors.New("questions must be a non-empty object")
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
	for id, raw := range envelope.Questions {
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
	typ, err := requiredString(question.Type, "question type")
	if err != nil {
		return fmt.Errorf("question %q: %w", id, err)
	}
	if err := validateStringObjectOrArray(question.Instructions, "instructions"); err != nil {
		return fmt.Errorf("question %q: %w", id, err)
	}

	switch typ {
	case "noul":
		criteria := bytes.TrimSpace(question.Criteria)
		if len(criteria) > 0 && criteria[0] != '{' {
			return fmt.Errorf("question %q: noul criteria must be an object", id)
		}
	case "choice":
		var criteria map[string]json.RawMessage
		if json.Unmarshal(question.Criteria, &criteria) != nil || len(criteria) == 0 {
			return fmt.Errorf("question %q: choice criteria must be a non-empty object", id)
		}
		for _, value := range criteria {
			if string(value) != "null" && !rawString(value) {
				return fmt.Errorf("question %q: choice criteria values must be strings or null", id)
			}
		}
	case "score":
		var criteria []json.RawMessage
		if json.Unmarshal(question.Criteria, &criteria) != nil || len(criteria) < 2 {
			return fmt.Errorf("question %q: score criteria must contain at least two strings", id)
		}
		for _, value := range criteria {
			if !rawString(value) {
				return fmt.Errorf("question %q: score criteria must contain only strings", id)
			}
		}
	default:
		return fmt.Errorf("question %q: unsupported type %q", id, typ)
	}
	return nil
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
