package typesafe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
)

const (
	DefaultBaseURL = "https://api.typesafe.ai"
	SystemOnePath  = "/v1/systemone"
	JevLatestModel = "jev-latest"
)

type Question struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
}

type Request struct {
	Model     string              `json:"model"`
	State     string              `json:"state"`
	Questions map[string]Question `json:"questions"`
}

type Result struct {
	Model  string
	Scores map[string]float64
	Usage  Usage
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type SystemOneResponse struct {
	Body  []byte
	Model string
	Usage Usage
}

func NewSystemOneRequest(ctx context.Context, baseURL, key string, body []byte) (*http.Request, error) {
	endpoint, err := url.JoinPath(strings.TrimRight(baseURL, "/"), SystemOnePath)
	if err != nil {
		return nil, errors.New("typesafe invalid endpoint")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("typesafe invalid request")
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// MaxSystemOneResponseBytes bounds a buffered System One response body.
const MaxSystemOneResponseBytes = 4 << 20

var ErrSystemOneResponseTooLarge = errors.New("typesafe response exceeds size limit")

func DecodeSystemOneResponse(r io.Reader) (*SystemOneResponse, error) {
	body, err := io.ReadAll(io.LimitReader(r, MaxSystemOneResponseBytes+1))
	if err != nil {
		return nil, errors.New("typesafe invalid response")
	}
	// A truncated body would otherwise surface as a misleading "invalid JSON".
	if len(body) > MaxSystemOneResponseBytes {
		return nil, ErrSystemOneResponseTooLarge
	}
	if !json.Valid(body) {
		return nil, errors.New("typesafe invalid response")
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil || envelope == nil {
		return nil, errors.New("typesafe invalid response")
	}
	// The upstream already answered (and charged); an unexpected model or usage
	// shape must not discard the answer, so both are decoded leniently.
	var model string
	_ = json.Unmarshal(envelope["model"], &model)
	var usage map[string]json.RawMessage
	_ = json.Unmarshal(envelope["usage"], &usage)
	return &SystemOneResponse{
		Body:  body,
		Model: model,
		Usage: Usage{
			InputTokens:  systemOneTokenCount(usage["input_tokens"]),
			OutputTokens: systemOneTokenCount(usage["output_tokens"]),
		},
	}, nil
}

// maxSystemOneTokenCount bounds a reported token count before int conversion.
const maxSystemOneTokenCount = 1 << 40

// systemOneTokenCount accepts integer, float, or numeric-string token counts.
func systemOneTokenCount(raw json.RawMessage) int {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return 0
	}
	if raw[0] == '"' {
		var text string
		if json.Unmarshal(raw, &text) != nil {
			return 0
		}
		raw = json.RawMessage(strings.TrimSpace(text))
	}
	var number json.Number
	if json.Unmarshal(raw, &number) != nil {
		return 0
	}
	value, err := number.Float64()
	if err != nil || math.IsNaN(value) || value <= 0 {
		return 0
	}
	if value > maxSystemOneTokenCount {
		value = maxSystemOneTokenCount
	}
	return int(math.Round(value))
}

// Evaluate performs one attempt. The caller owns timeouts, retries and key rotation.
func Evaluate(ctx context.Context, client *http.Client, baseURL, key string, input Request) (*Result, int, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, 0, errors.New("typesafe invalid request")
	}
	req, err := NewSystemOneRequest(ctx, baseURL, key, body)
	if err != nil {
		return nil, 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		return nil, 0, errors.New("typesafe transport unavailable")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Do not log provider error bodies: they may echo user input or credentials.
		return nil, resp.StatusCode, fmt.Errorf("typesafe API status %d", resp.StatusCode)
	}
	var out struct {
		Model   string `json:"model"`
		Usage   Usage  `json:"usage"`
		Answers map[string]struct {
			Type string   `json:"type"`
			Noul *float64 `json:"noul"`
		} `json:"answers"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil || strings.TrimSpace(out.Model) == "" {
		return nil, resp.StatusCode, errors.New("typesafe invalid response")
	}
	result := &Result{Model: out.Model, Usage: out.Usage, Scores: make(map[string]float64, len(input.Questions))}
	for id := range input.Questions {
		answer, ok := out.Answers[id]
		if !ok || answer.Type != "noul" || answer.Noul == nil || math.IsNaN(*answer.Noul) || math.IsInf(*answer.Noul, 0) || *answer.Noul < 0 || *answer.Noul > 1 {
			return nil, resp.StatusCode, fmt.Errorf("typesafe invalid answer for %s", id)
		}
		result.Scores[id] = *answer.Noul
	}
	return result, resp.StatusCode, nil
}
