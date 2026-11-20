package openaiasync

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

func ValidEndpoint(endpoint string) bool {
	switch endpoint {
	case "/v1/responses", "/v1/chat/completions", "/v1/embeddings":
		return true
	}
	return false
}

func ParseInput(body []byte, endpoint string, maxItems, maxLineBytes int) ([]Item, error) {
	if !ValidEndpoint(endpoint) {
		return nil, fmt.Errorf("unsupported local batch endpoint")
	}
	if !utf8.Valid(body) {
		return nil, fmt.Errorf("batch input must be UTF-8 JSONL")
	}
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 4096), maxLineBytes)
	items := []Item{}
	seen := map[string]bool{}
	model := ""
	for line := 1; scanner.Scan(); line++ {
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			return nil, fmt.Errorf("line %d: empty lines are not allowed", line)
		}
		var item Item
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&item); err != nil {
			return nil, fmt.Errorf("line %d: invalid batch request: %w", line, err)
		}
		if err := dec.Decode(new(any)); err != io.EOF {
			return nil, fmt.Errorf("line %d: expected one JSON object", line)
		}
		if item.Method != "POST" || item.URL != endpoint {
			return nil, fmt.Errorf("line %d: method must be POST and url must match endpoint", line)
		}
		if strings.TrimSpace(item.CustomID) == "" || len(item.CustomID) > 512 || strings.ContainsRune(item.CustomID, '\x00') || seen[item.CustomID] {
			return nil, fmt.Errorf("line %d: custom_id must be non-empty, unique, and at most 512 bytes", line)
		}
		var request map[string]json.RawMessage
		if json.Unmarshal(item.Body, &request) != nil || request == nil {
			return nil, fmt.Errorf("line %d: body must be a JSON object", line)
		}
		var requestModel string
		if json.Unmarshal(request["model"], &requestModel) != nil || strings.TrimSpace(requestModel) == "" {
			return nil, fmt.Errorf("line %d: model is required", line)
		}
		if model != "" && model != requestModel {
			return nil, fmt.Errorf("line %d: all requests must use the same model", line)
		}
		model = requestModel
		if stream, ok := request["stream"]; ok && !bytes.Equal(bytes.TrimSpace(stream), []byte("false")) {
			return nil, fmt.Errorf("line %d: streaming is not supported in batches", line)
		}
		// Canonicalize top-level keys so duplicate stream/model fields cannot
		// give validation and the gateway's JSON readers different values.
		canonical, err := json.Marshal(request)
		if err != nil {
			return nil, err
		}
		item.Body = canonical
		// No create/delete/URL override escape hatch exists: execution targets
		// only the fixed authenticated endpoint selected for this whole batch.
		item.Ordinal = len(items)
		items = append(items, item)
		seen[item.CustomID] = true
		if len(items) > maxItems {
			return nil, fmt.Errorf("batch exceeds the local limit of %d requests", maxItems)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("batch input line exceeds the local size limit")
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("batch input is empty")
	}
	return items, nil
}
