// Package openai_decisions validates the public OpenAI Decisions wire contract.
package openai_decisions

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"unicode/utf8"
)

const (
	Endpoint = "/v1/decisions"
	Model    = "gpt-6-luna"
)

type Choice struct {
	Value       any    `json:"value"`
	Description string `json:"description,omitempty"`
}

type Level struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type Question struct {
	Type         string   `json:"type"`
	Name         *string  `json:"name,omitempty"`
	Instructions string   `json:"instructions"`
	Choices      []Choice `json:"choices,omitempty"`
	Levels       []Level  `json:"levels,omitempty"`
}

type Request struct {
	Model            string          `json:"model"`
	Input            json.RawMessage `json:"input"`
	Questions        []Question      `json:"questions"`
	SafetyIdentifier *string         `json:"safety_identifier,omitempty"`
	ImageCount       int             `json:"-"`
}

type Usage struct {
	InputTokens      int
	OutputTokens     int
	CachedTokens     int
	CacheWriteTokens int
}

func ParseRequest(body []byte) (*Request, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		return nil, errors.New("request must be a JSON object")
	}
	for key := range fields {
		switch key {
		case "model", "input", "questions", "safety_identifier":
		default:
			return nil, fmt.Errorf("unsupported Decisions parameter: %s", key)
		}
	}
	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, errors.New("invalid Decisions request")
	}
	if req.Model != Model {
		return nil, fmt.Errorf("unsupported Decisions model")
	}
	if req.SafetyIdentifier != nil && utf8.RuneCountInString(*req.SafetyIdentifier) > 128 {
		return nil, errors.New("safety_identifier exceeds 128 characters")
	}
	if len(req.Questions) == 0 || len(req.Questions) > 200 {
		return nil, errors.New("questions must contain 1 to 200 entries")
	}
	imageCount, inputErr := validateInput(req.Input)
	if inputErr != nil {
		return nil, inputErr
	}
	req.ImageCount = imageCount
	var rawQuestions []map[string]json.RawMessage
	if err := json.Unmarshal(fields["questions"], &rawQuestions); err != nil || len(rawQuestions) != len(req.Questions) {
		return nil, errors.New("questions must contain objects")
	}
	for i, q := range req.Questions {
		if err := validateQuestion(rawQuestions[i], q); err != nil {
			return nil, fmt.Errorf("question %d: %w", i, err)
		}
	}
	return &req, nil
}

func validateInput(raw json.RawMessage) (int, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return -1, errors.New("input is required")
	}
	var text string
	if raw[0] == '"' && json.Unmarshal(raw, &text) == nil {
		return 0, nil
	}
	var messages []struct {
		Role    string          `json:"role"`
		Type    string          `json:"type"`
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &messages) != nil || len(messages) == 0 {
		return -1, errors.New("input must be text or user messages")
	}
	images := 0
	for _, message := range messages {
		if message.Role != "user" || (message.Type != "" && message.Type != "message") {
			return -1, errors.New("only user messages are supported")
		}
		content := bytes.TrimSpace(message.Content)
		if len(content) > 0 && content[0] == '"' && json.Unmarshal(content, &text) == nil {
			continue
		}
		var parts []struct {
			Type     string  `json:"type"`
			Text     *string `json:"text"`
			ImageURL string  `json:"image_url"`
			Detail   string  `json:"detail"`
		}
		if json.Unmarshal(content, &parts) != nil || len(parts) == 0 {
			return -1, errors.New("message content must be text or input parts")
		}
		for _, part := range parts {
			switch part.Type {
			case "input_text":
				if part.Text == nil {
					return -1, errors.New("input_text requires text")
				}
			case "input_image":
				prefix, encoded, ok := strings.Cut(part.ImageURL, ",")
				if !ok || !strings.HasPrefix(prefix, "data:image/") || !strings.HasSuffix(prefix, ";base64") {
					return -1, errors.New("images require inline base64 data URLs")
				}
				decoded, err := base64.StdEncoding.DecodeString(encoded)
				if err != nil || len(decoded) == 0 {
					return -1, errors.New("invalid image base64")
				}
				if part.Detail != "" && part.Detail != "low" && part.Detail != "high" && part.Detail != "auto" && part.Detail != "original" {
					return -1, errors.New("invalid image detail")
				}
				images++
				if images > 128 {
					return -1, errors.New("at most 128 images are supported")
				}
			default:
				return -1, errors.New("only input_text and input_image parts are supported")
			}
		}
	}
	return images, nil
}

func validateQuestion(raw map[string]json.RawMessage, q Question) error {
	for key := range raw {
		switch key {
		case "type", "name", "instructions", "choices", "levels":
		default:
			return fmt.Errorf("unsupported parameter: %s", key)
		}
	}
	var instructions string
	if rawInstructions, ok := raw["instructions"]; !ok || !bytes.HasPrefix(bytes.TrimSpace(rawInstructions), []byte(`"`)) || json.Unmarshal(rawInstructions, &instructions) != nil {
		return errors.New("instructions must be a string")
	}
	var typ string
	if rawType, ok := raw["type"]; !ok || json.Unmarshal(rawType, &typ) != nil || typ != q.Type {
		return errors.New("type must be a string")
	}
	switch q.Type {
	case "predicate":
		if len(q.Choices) != 0 || len(q.Levels) != 0 {
			return errors.New("predicate cannot contain choices or levels")
		}
	case "choice":
		if len(q.Choices) == 0 || len(q.Levels) != 0 {
			return errors.New("choice requires choices and cannot contain levels")
		}
		seen := map[string]bool{}
		for _, choice := range q.Choices {
			switch choice.Value.(type) {
			case string, bool:
			default:
				return errors.New("choice value must be string or boolean")
			}
			encoded, _ := json.Marshal(choice.Value)
			if seen[string(encoded)] {
				return errors.New("duplicate choice value")
			}
			seen[string(encoded)] = true
		}
	case "score":
		if len(q.Levels) == 0 || len(q.Choices) != 0 {
			return errors.New("score requires levels and cannot contain choices")
		}
		for _, level := range q.Levels {
			if strings.TrimSpace(level.Label) == "" {
				return errors.New("score levels require labels")
			}
		}
	default:
		return errors.New("question type must be predicate, choice, or score")
	}
	return nil
}

func DecodeResponse(requestBody, body []byte) (Usage, error) {
	req, err := ParseRequest(requestBody)
	if err != nil {
		return Usage{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		return Usage{}, errors.New("invalid Decisions response")
	}
	var envelope struct {
		Model   string            `json:"model"`
		Answers []json.RawMessage `json:"answers"`
		Usage   struct {
			InputTokens       *int `json:"input_tokens"`
			OutputTokens      *int `json:"output_tokens"`
			InputTokenDetails struct {
				CachedTokens     int `json:"cached_tokens"`
				CacheWriteTokens int `json:"cache_write_tokens"`
			} `json:"input_tokens_details"`
		} `json:"usage"`
	}
	if _, ok := fields["model"]; !ok {
		return Usage{}, errors.New("invalid Decisions response")
	}
	if _, ok := fields["answers"]; !ok {
		return Usage{}, errors.New("invalid Decisions response")
	}
	if _, ok := fields["usage"]; !ok {
		return Usage{}, errors.New("invalid Decisions response")
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Model != Model {
		return Usage{}, errors.New("invalid Decisions response")
	}
	if len(envelope.Answers) != len(req.Questions) || envelope.Usage.InputTokens == nil || envelope.Usage.OutputTokens == nil || *envelope.Usage.InputTokens < 0 || *envelope.Usage.OutputTokens < 0 || envelope.Usage.InputTokenDetails.CachedTokens < 0 || envelope.Usage.InputTokenDetails.CacheWriteTokens < 0 {
		return Usage{}, errors.New("invalid Decisions response usage or answer count")
	}
	if envelope.Usage.InputTokenDetails.CachedTokens+envelope.Usage.InputTokenDetails.CacheWriteTokens > *envelope.Usage.InputTokens {
		return Usage{}, errors.New("cache tokens exceed input tokens")
	}
	for i, raw := range envelope.Answers {
		if err := validateAnswer(raw, req.Questions[i]); err != nil {
			return Usage{}, fmt.Errorf("answer %d: %w", i, err)
		}
	}
	return Usage{InputTokens: *envelope.Usage.InputTokens, OutputTokens: *envelope.Usage.OutputTokens, CachedTokens: envelope.Usage.InputTokenDetails.CachedTokens, CacheWriteTokens: envelope.Usage.InputTokenDetails.CacheWriteTokens}, nil
}

func validateAnswer(raw json.RawMessage, q Question) error {
	var answer struct {
		Type          string   `json:"type"`
		Probability   *float64 `json:"probability"`
		Choice        any      `json:"choice"`
		Confidence    *float64 `json:"confidence"`
		Score         *float64 `json:"score"`
		Probabilities []struct {
			Value       any      `json:"value"`
			Label       string   `json:"label"`
			Probability *float64 `json:"probability"`
		} `json:"probabilities"`
	}
	if json.Unmarshal(raw, &answer) != nil {
		return errors.New("answer must be an object")
	}
	if answer.Type == "refusal" {
		return nil
	}
	finiteProbability := func(value float64) bool {
		return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
	}
	switch q.Type {
	case "predicate":
		if answer.Type != "predicate" || answer.Probability == nil || !finiteProbability(*answer.Probability) {
			return errors.New("invalid predicate answer")
		}
	case "choice":
		if answer.Type != "choice" || answer.Confidence == nil || !finiteProbability(*answer.Confidence) || len(answer.Probabilities) != len(q.Choices) {
			return errors.New("invalid choice answer")
		}
		if !finiteProbability(*answer.Confidence) {
			return errors.New("invalid choice confidence")
		}
		sum := 0.0
		seen := map[string]bool{}
		for _, probability := range answer.Probabilities {
			if probability.Probability == nil || !finiteProbability(*probability.Probability) {
				return errors.New("invalid choice probability")
			}
			valid := false
			for _, choice := range q.Choices {
				if reflect.DeepEqual(probability.Value, choice.Value) {
					valid = true
					break
				}
			}
			encoded, _ := json.Marshal(probability.Value)
			if !valid || seen[string(encoded)] {
				return errors.New("choice distribution values do not match request")
			}
			seen[string(encoded)] = true
			sum += *probability.Probability
		}
		if math.Abs(sum-1) > float64(len(q.Choices))*0.01+1e-9 {
			return errors.New("choice probabilities must sum to one")
		}
		selected := false
		for _, choice := range q.Choices {
			if reflect.DeepEqual(choice.Value, answer.Choice) {
				selected = true
				break
			}
		}
		if !selected {
			return errors.New("choice answer is not one of the allowed values")
		}
	case "score":
		if answer.Type != "score" || answer.Confidence == nil || answer.Score == nil || !finiteProbability(*answer.Confidence) || math.IsNaN(*answer.Score) || math.IsInf(*answer.Score, 0) || *answer.Score < 0 || *answer.Score > float64(len(q.Levels)-1) || len(answer.Probabilities) != len(q.Levels) {
			return errors.New("invalid score answer")
		}
		sum := 0.0
		seen := map[int]bool{}
		expected := 0.0
		for _, probability := range answer.Probabilities {
			if probability.Probability == nil || !finiteProbability(*probability.Probability) {
				return errors.New("invalid score probability")
			}
			value, ok := probability.Value.(float64)
			if !ok || value != math.Trunc(value) || value < 0 || value >= float64(len(q.Levels)) || seen[int(value)] || probability.Label != q.Levels[int(value)].Label {
				return errors.New("score distribution does not match request levels")
			}
			seen[int(value)] = true
			expected += value * *probability.Probability
			sum += *probability.Probability
		}
		if math.Abs(*answer.Score-expected) > 0.005+float64(len(q.Levels)*(len(q.Levels)-1))*0.0025+1e-9 {
			return errors.New("invalid expected score")
		}
		if math.Abs(sum-1) > float64(len(q.Levels))*0.01+1e-9 {
			return errors.New("score probabilities must sum to one")
		}
	}
	return nil
}
