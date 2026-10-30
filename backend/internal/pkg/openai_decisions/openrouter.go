package openai_decisions

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
)

const OpenRouterModel = "openai/gpt-6-luna-decisions"
const OpenRouterPath = "/api/alpha/decisions"

// Messages remain structured multimodal evidence, never JSON-encoded prose.
func inputState(raw json.RawMessage) (any, error) {
	var text string
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte(`"`)) && json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var messages []struct {
		Role    string          `json:"role"`
		Type    string          `json:"type"`
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &messages) != nil || len(messages) == 0 {
		return nil, errors.New("input must be text or user messages")
	}
	state := make([]any, 0, len(messages))
	images := 0
	for _, m := range messages {
		if m.Role != "user" || (m.Type != "" && m.Type != "message") {
			return nil, errors.New("only user messages are supported")
		}
		if bytes.HasPrefix(bytes.TrimSpace(m.Content), []byte(`"`)) && json.Unmarshal(m.Content, &text) == nil {
			state = append(state, map[string]any{"type": "text", "text": text})
			continue
		}
		var parts []struct {
			Type     string  `json:"type"`
			Text     *string `json:"text"`
			ImageURL string  `json:"image_url"`
			Detail   string  `json:"detail"`
		}
		if json.Unmarshal(m.Content, &parts) != nil || len(parts) == 0 {
			return nil, errors.New("message content must be text or input parts")
		}
		content := make([]any, 0, len(parts))
		for _, p := range parts {
			switch p.Type {
			case "input_text":
				if p.Text == nil {
					return nil, errors.New("input_text requires text")
				}
				content = append(content, map[string]any{"type": "text", "text": *p.Text})
			case "input_image":
				prefix, data, ok := strings.Cut(p.ImageURL, ",")
				if !ok || !strings.HasPrefix(prefix, "data:image/") || !strings.HasSuffix(prefix, ";base64") {
					return nil, errors.New("images require inline base64 data URLs")
				}
				decoded, err := base64.StdEncoding.DecodeString(data)
				if err != nil || len(decoded) == 0 {
					return nil, errors.New("invalid image base64")
				}
				images++
				if images > 128 {
					return nil, errors.New("at most 128 images are supported")
				}
				image := map[string]any{"url": p.ImageURL}
				if p.Detail != "" {
					switch p.Detail {
					case "low", "high", "auto", "original":
						image["detail"] = p.Detail
					default:
						return nil, errors.New("invalid image detail")
					}
				}
				content = append(content, map[string]any{"type": "image_url", "image_url": image})
			default:
				return nil, errors.New("only input_text and input_image parts are supported")
			}
		}
		state = append(state, content...)
	}
	return state, nil
}

func questionID(index int) string { return "q" + strconv.Itoa(index) }
func choiceID(index int) string   { return "c" + strconv.Itoa(index) }

func EncodeOpenRouterRequest(body []byte) ([]byte, error) {
	req, err := ParseRequest(body)
	if err != nil {
		return nil, err
	}
	if req.ImageCount > 1 {
		return nil, errors.New("OpenRouter Decisions supports at most one image per request")
	}
	state, _ := inputState(req.Input)
	questions := map[string]any{}
	for i, q := range req.Questions {
		out := map[string]any{"type": q.Type, "instructions": q.Instructions}
		switch q.Type {
		case "predicate":
			out["type"] = "noul"
		case "choice":
			criteria := map[string]any{}
			for j, c := range q.Choices {
				description := c.Description
				if description == "" {
					v, _ := json.Marshal(c.Value)
					description = string(v)
				}
				criteria[choiceID(j)] = description
			}
			out["criteria"] = criteria
		case "score":
			levels := make([]string, len(q.Levels))
			for j, l := range q.Levels {
				levels[j] = l.Label
				if l.Description != "" {
					levels[j] += " — " + l.Description
				}
			}
			out["criteria"] = levels
		}
		questions[questionID(i)] = out
	}
	payload := map[string]any{"model": OpenRouterModel, "state": state, "questions": questions}
	if req.SafetyIdentifier != nil {
		payload["user"] = *req.SafetyIdentifier
	}
	return json.Marshal(payload)
}

type BridgeUsage struct {
	InputTokens  *int     `json:"input_tokens"`
	OutputTokens *int     `json:"output_tokens"`
	Cost         *float64 `json:"cost,omitempty"`
	InputDetails struct {
		CachedTokens     int `json:"cached_tokens"`
		CacheWriteTokens int `json:"cache_write_tokens"`
	} `json:"input_tokens_details"`
}
type openRouterResponse struct {
	Model   string                     `json:"model"`
	Answers map[string]json.RawMessage `json:"answers"`
	Usage   BridgeUsage                `json:"usage"`
}

func DecodeOpenRouterResponse(requestBody, body []byte) ([]byte, BridgeUsage, error) {
	req, err := ParseRequest(requestBody)
	if err != nil {
		return nil, BridgeUsage{}, err
	}
	var response openRouterResponse
	if json.Unmarshal(body, &response) != nil || !IsOpenRouterModel(response.Model) {
		return nil, BridgeUsage{}, errors.New("invalid upstream Decisions model or JSON")
	}
	u := response.Usage
	if u.InputTokens == nil || u.OutputTokens == nil || *u.InputTokens < 0 || *u.OutputTokens < 0 || u.InputDetails.CachedTokens < 0 || u.InputDetails.CacheWriteTokens < 0 || u.InputDetails.CachedTokens+u.InputDetails.CacheWriteTokens > *u.InputTokens || (u.Cost != nil && (!finite(*u.Cost) || *u.Cost < 0)) {
		return nil, u, errors.New("invalid upstream Decisions usage")
	}
	if len(response.Answers) != len(req.Questions) {
		return nil, u, errors.New("upstream Decisions answer count mismatch")
	}
	answers := make([]any, len(req.Questions))
	for i, q := range req.Questions {
		var a struct {
			Type          string              `json:"type"`
			Noul          *float64            `json:"noul"`
			Choice        string              `json:"choice"`
			Score         *float64            `json:"score"`
			Confidence    *float64            `json:"confidence"`
			Probabilities map[string]*float64 `json:"probabilities"`
		}
		if json.Unmarshal(response.Answers[questionID(i)], &a) != nil {
			return nil, u, errors.New("missing or invalid upstream Decisions answer")
		}
		out := map[string]any{"name": q.Name, "type": q.Type}
		if a.Type == "refusal" {
			out["type"] = "refusal"
			answers[i] = out
			continue
		}
		switch q.Type {
		case "predicate":
			if a.Type != "noul" || a.Noul == nil || !probability(*a.Noul) {
				return nil, u, errors.New("invalid predicate answer")
			}
			out["probability"] = *a.Noul
		case "choice", "score":
			if a.Type != q.Type || a.Confidence == nil || !probability(*a.Confidence) {
				return nil, u, errors.New("invalid answer type or confidence")
			}
			out["confidence"] = *a.Confidence
			count := len(q.Choices)
			if q.Type == "score" {
				count = len(q.Levels)
			}
			if len(a.Probabilities) != count {
				return nil, u, errors.New("incomplete probability distribution")
			}
			probs := make([]any, count)
			sum, expected := 0.0, 0.0
			selected := false
			for j := 0; j < count; j++ {
				id := choiceID(j)
				if q.Type == "score" {
					id = strconv.Itoa(j)
				}
				value, ok := a.Probabilities[id]
				if !ok || value == nil || !probability(*value) {
					return nil, u, errors.New("invalid probability distribution")
				}
				p := *value
				sum += p
				if q.Type == "choice" {
					probs[j] = map[string]any{"value": q.Choices[j].Value, "probability": p}
					if a.Choice == id {
						out["choice"] = q.Choices[j].Value
						selected = true
					}
				} else {
					probs[j] = map[string]any{"value": j, "label": q.Levels[j].Label, "probability": p}
					expected += float64(j) * p
				}
			}
			// OpenRouter rounds probabilities to two decimal places.
			if sum <= 0 || math.Abs(sum-1) > float64(count)*0.005+1e-9 {
				return nil, u, errors.New("probabilities must sum to one")
			}
			if q.Type == "choice" && !selected {
				return nil, u, errors.New("unknown selected choice")
			}
			if q.Type == "score" {
				if a.Score == nil || !finite(*a.Score) || *a.Score < 0 || *a.Score > float64(count-1) || math.Abs(*a.Score-expected) > 0.005+float64(count*(count-1))*0.0025+1e-9 {
					return nil, u, errors.New("invalid expected score")
				}
				out["score"] = *a.Score
			}
			out["probabilities"] = probs
		}
		answers[i] = out
	}
	model := req.Model
	if model == OpenRouterModel {
		model = Model
	}
	result, err := json.Marshal(map[string]any{"model": model, "answers": answers, "usage": map[string]any{"input_tokens": *u.InputTokens, "output_tokens": *u.OutputTokens, "total_tokens": *u.InputTokens + *u.OutputTokens, "input_tokens_details": map[string]int{"cached_tokens": u.InputDetails.CachedTokens, "cache_write_tokens": u.InputDetails.CacheWriteTokens}, "output_tokens_details": map[string]int{"reasoning_tokens": 0}}})
	return result, u, err
}
func finite(v float64) bool      { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func probability(v float64) bool { return finite(v) && v >= 0 && v <= 1 }

func IsOpenRouterModel(model string) bool {
	if model == OpenRouterModel {
		return true
	}
	snapshot := strings.TrimPrefix(model, OpenRouterModel+"-")
	if snapshot == model {
		return false
	}
	for _, layout := range []string{"20060102", "2006-01-02"} {
		if _, err := time.Parse(layout, snapshot); err == nil {
			return true
		}
	}
	return false
}
