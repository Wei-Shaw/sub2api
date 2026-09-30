package basispoints

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
)

type unknownClientToolError struct{}

func (unknownClientToolError) Error() string {
	return "basispoints returned a tool outside the client's catalog"
}

type RepairToolCall func(context.Context) (io.ReadCloser, error)

func (b *Bridge) canRepair(response object, err error) bool {
	var unknown unknownClientToolError
	if len(b.tools) == 0 || b.hasToolHistory || !errors.As(err, &unknown) || text(response["status"]) != "completed" {
		return false
	}
	output, _ := response["output"].([]any)
	count := 0
	for i, raw := range output {
		item, _ := raw.(object)
		if !isTool(item) {
			continue
		}
		count++
		name := text(item["name"])
		if i != len(output)-1 || text(item["type"]) != "function_call" || (name != "run_officejs" && name != "functions.run_officejs") {
			return false
		}
	}
	return count == 1
}

// The correction reuses the already prepared body, including uploaded file IDs,
// selected model, account-scoped metadata, and expanded history. It adds no tool
// result and never represents the rejected call as executed.
func RepairRequest(prepared []byte) ([]byte, error) {
	var request object
	if decode(prepared, &request) != nil {
		return nil, fmt.Errorf("invalid prepared Basispoints request")
	}
	input, ok := request["input"].([]any)
	if !ok {
		return nil, fmt.Errorf("missing Basispoints request history")
	}
	reminder := message("developer", "The previous response selected a tool absent from the current client catalog. No client tool from that response was executed. Correct this once using exactly one tool declared in the client catalog and its documented run_officejs transport. Do not call executor-internal helpers as standalone tools. Preserve the original task and use only the declared argument schema.")
	index := len(input)
	if index > 0 {
		last, _ := input[index-1].(object)
		if text(last["type"]) == "compaction_trigger" {
			index--
		}
	}
	next := make([]any, 0, len(input)+1)
	next = append(next, input[:index]...)
	next = append(next, reminder)
	next = append(next, input[index:]...)
	request["input"] = next
	return json.Marshal(request)
}

func readRepairResponse(reader io.Reader) (object, error) {
	var response object
	var observedUsage object
	pending := make(map[string]bool)
	total := 0
	err := readEvents(reader, func(eventName string, data []byte) error {
		total += len(data)
		if total > 32<<20 {
			return fmt.Errorf("basispoints correction response exceeds 32 MiB")
		}
		if string(data) == "[DONE]" {
			return nil
		}
		var event object
		if decode(data, &event) != nil {
			return fmt.Errorf("invalid Basispoints correction event")
		}
		if current, ok := event["response"].(object); ok {
			if usage, ok := current["usage"].(object); ok {
				observedUsage = maxReportedUsage(observedUsage, usage)
			}
		}
		if usage, ok := event["usage"].(object); ok {
			observedUsage = maxReportedUsage(observedUsage, usage)
		}
		kind := text(event["type"])
		if kind == "" {
			kind = eventName
		}
		if kind == "response.output_item.done" {
			item, _ := event["item"].(object)
			if isTool(item) {
				if len(pending) >= 1024 {
					return fmt.Errorf("too many Basispoints correction tool items")
				}
				pending[text(item["call_id"])+"\x00"+text(item["id"])] = true
			}
		}
		switch kind {
		case "response.completed", "response.incomplete", "response.failed", "response.cancelled":
			response, _ = event["response"].(object)
			if response == nil {
				return fmt.Errorf("missing Basispoints correction response")
			}
			if kind != "response.completed" {
				response["status"] = "failed"
				if failure := classifyUpstreamFailure(kind, event); failure != nil {
					return failure
				}
			}
			if kind == "response.completed" {
				output, _ := response["output"].([]any)
				for _, raw := range output {
					item, _ := raw.(object)
					if isTool(item) {
						delete(pending, text(item["call_id"])+"\x00"+text(item["id"]))
					}
				}
				if len(pending) != 0 {
					return fmt.Errorf("basispoints correction omitted an original tool item")
				}
			}
			return io.EOF
		case "error":
			return classifyUpstreamFailure(kind, event)
		}
		return nil
	})
	if response == nil {
		response = object{"usage": observedUsage}
	} else if response["usage"] == nil {
		response["usage"] = observedUsage
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return response, err
	}
	if text(response["status"]) == "" {
		return response, io.ErrUnexpectedEOF
	}
	return response, nil
}

// Token counters are JSON integers. Preserve exact counters without float64,
// and combine nested cached/reasoning details for both provider attempts.
func mergeUsage(first, second object) object {
	result := make(object)
	for key, v := range first {
		result[key] = v
	}
	for key, v := range second {
		if nested, ok := v.(object); ok {
			before, _ := result[key].(object)
			result[key] = mergeUsage(before, nested)
			continue
		}
		a, okA := new(big.Int).SetString(fmt.Sprint(result[key]), 10)
		n, okB := new(big.Int).SetString(fmt.Sprint(v), 10)
		if okB && n.Sign() >= 0 {
			if okA && a.Sign() >= 0 {
				n.Add(a, n)
			}
			result[key] = json.Number(n.String())
		}
	}
	return result
}

func (b *Bridge) repairResponse(ctx context.Context, original object, repair RepairToolCall) (object, int, error) {
	reader, err := repair(ctx)
	if err != nil {
		return nil, -1, err
	}
	defer func() { _ = reader.Close() }()
	corrected, err := readRepairResponse(reader)
	first, _ := original["usage"].(object)
	second, _ := corrected["usage"].(object)
	original["usage"] = mergeUsage(first, second)
	if err != nil {
		return nil, -1, err
	}
	if text(corrected["status"]) != "completed" {
		return nil, -1, fmt.Errorf("basispoints tool correction did not complete")
	}
	output, _ := original["output"].([]any)
	replacement, ok := corrected["output"].([]any)
	if !ok || len(replacement) == 0 {
		return nil, -1, fmt.Errorf("basispoints tool correction returned no output")
	}
	index := len(output) - 1
	merged := make(object, len(original))
	for k, v := range original {
		merged[k] = v
	}
	merged["output"] = append(append([]any{}, output[:index]...), replacement...)
	if err := b.translateResponse(merged); err != nil {
		return nil, -1, err
	}
	return merged, index, nil
}

// Progressive usage reports are snapshots, not separate charges. Retain only
// their maxima as a fallback when a failed correction has no terminal usage.
func maxReportedUsage(first, second object) object {
	result := make(object)
	for key, v := range first {
		result[key] = v
	}
	for key, v := range second {
		if nested, ok := v.(object); ok {
			before, _ := result[key].(object)
			result[key] = maxReportedUsage(before, nested)
			continue
		}
		n, valid := new(big.Int).SetString(fmt.Sprint(v), 10)
		if !valid || n.Sign() < 0 {
			continue
		}
		old, exists := new(big.Int).SetString(fmt.Sprint(result[key]), 10)
		if !exists || n.Cmp(old) > 0 {
			result[key] = json.Number(n.String())
		}
	}
	return result
}
