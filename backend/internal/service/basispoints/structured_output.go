package basispoints

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const structuredSchemaURL = "https://basispoints.invalid/structured-output.json"

var structuredFormatName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// BPS rejects native text.format/response_format fields. Apply the requested
// format through instructions and validate the authoritative final answer before
// exposing any message text. This is not upstream constrained decoding.
type structuredOutput struct {
	format object
	schema *jsonschema.Schema
}

type localSchemaLoader struct{}

func (localSchemaLoader) Load(string) (any, error) {
	return nil, fmt.Errorf("external structured output schema references are not supported")
}

func prepareStructuredOutput(raw any) (*structuredOutput, error) {
	if raw == nil {
		return nil, nil
	}
	config, ok := raw.(object)
	if !ok {
		return nil, fmt.Errorf("basispoints text must be an object")
	}
	if config["format"] == nil {
		return nil, nil
	}
	format, ok := config["format"].(object)
	if !ok {
		return nil, fmt.Errorf("basispoints text.format must be an object")
	}
	kind := text(format["type"])
	if kind == "text" {
		return nil, nil
	}
	if kind != "json_object" && kind != "json_schema" {
		return nil, fmt.Errorf("basispoints text.format requires text, json_object or json_schema")
	}
	for key := range format {
		if key == "type" || (kind == "json_schema" && (key == "name" || key == "schema" || key == "strict" || key == "description")) {
			continue
		}
		return nil, fmt.Errorf("basispoints text.format contains an unsupported field")
	}
	result := &structuredOutput{format: format}
	if kind == "json_object" {
		return result, nil
	}
	if !structuredFormatName.MatchString(text(format["name"])) {
		return nil, fmt.Errorf("basispoints json_schema requires a name of 1-64 letters, digits, underscores or hyphens")
	}
	if value := format["strict"]; value != nil {
		if _, ok := value.(bool); !ok {
			return nil, fmt.Errorf("basispoints json_schema strict must be a boolean")
		}
	}
	if value := format["description"]; value != nil {
		if _, ok := value.(string); !ok {
			return nil, fmt.Errorf("basispoints json_schema description must be a string")
		}
	}
	schema, ok := format["schema"].(object)
	if !ok || schema == nil {
		return nil, fmt.Errorf("basispoints json_schema requires a schema object")
	}
	encoded, err := json.Marshal(schema)
	if err != nil || len(encoded) > 1<<20 {
		return nil, fmt.Errorf("basispoints structured output schema exceeds 1 MiB")
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	// Schemas are untrusted request data. Resolve in-document references only;
	// never fetch a URL or read a server file while compiling them.
	compiler.UseLoader(localSchemaLoader{})
	if err := compiler.AddResource(structuredSchemaURL, schema); err != nil {
		return nil, fmt.Errorf("basispoints structured output schema is invalid")
	}
	result.schema, err = compiler.Compile(structuredSchemaURL)
	if err != nil {
		return nil, fmt.Errorf("basispoints structured output schema is invalid or references an external resource")
	}
	return result, nil
}

func (s *structuredOutput) instructions() string {
	prompt := "The client requires a structured final answer. Your final assistant answer must be exactly one JSON value, with no Markdown fences or surrounding prose. " +
		"Tool calls and refusals remain separate protocol items; use the client tool transport as needed before the final answer. " +
		"The gateway validates the final JSON before returning it to the client."
	if s.schema != nil {
		encoded, _ := json.Marshal(s.format)
		prompt += " The final answer must satisfy the schema in this output format: \n" + string(encoded)
	}
	return prompt
}

func (s *structuredOutput) validate(response object) error {
	output, _ := response["output"].([]any)
	var answer strings.Builder
	hasTool, hasRefusal := false, false
	for _, raw := range output {
		item, _ := raw.(object)
		hasTool = hasTool || isTool(item)
		if text(item["type"]) != "message" {
			continue
		}
		content, _ := item["content"].([]any)
		for _, rawPart := range content {
			part, _ := rawPart.(object)
			switch text(part["type"]) {
			case "output_text":
				value, ok := part["text"].(string)
				if !ok || answer.Len()+len(value) > 16<<20 {
					return fmt.Errorf("basispoints structured output text is invalid or exceeds 16 MiB")
				}
				_, _ = answer.WriteString(value)
			case "refusal":
				hasRefusal = true
			default:
				return fmt.Errorf("basispoints structured output contains unsupported message content")
			}
		}
	}
	// A tool turn is not the final answer. Refusals are explicitly outside the
	// JSON contract, as in Responses; never turn them into fabricated JSON.
	if hasTool || (hasRefusal && answer.Len() == 0) {
		return nil
	}
	var instance any
	if err := decode([]byte(answer.String()), &instance); err != nil {
		return fmt.Errorf("basispoints structured output is not one valid JSON value")
	}
	if s.schema != nil && s.schema.Validate(instance) != nil {
		return fmt.Errorf("basispoints structured output does not satisfy the requested JSON schema")
	}
	return nil
}

func isStructuredMessageEvent(kind string, item object) bool {
	return strings.HasPrefix(kind, "response.output_text.") || strings.HasPrefix(kind, "response.refusal.") ||
		strings.HasPrefix(kind, "response.content_part.") ||
		(strings.HasPrefix(kind, "response.output_item.") && text(item["type"]) == "message")
}

// Reconstruct message events from the validated terminal items. Earlier deltas
// can disagree with the terminal response and must never escape validation.
func emitStructuredMessage(item object, index int, emit func(string, object) error) error {
	id := text(item["id"])
	added := make(object, len(item))
	for key, value := range item {
		added[key] = value
	}
	added["content"], added["status"] = []any{}, "in_progress"
	if err := emit("response.output_item.added", object{"output_index": index, "item": added}); err != nil {
		return err
	}
	content, _ := item["content"].([]any)
	for i, raw := range content {
		part, _ := raw.(object)
		field, prefix := "text", "response.output_text"
		if text(part["type"]) == "refusal" {
			field, prefix = "refusal", "response.refusal"
		}
		empty := make(object, len(part))
		for key, value := range part {
			empty[key] = value
		}
		empty[field] = ""
		events := []struct {
			kind    string
			payload object
		}{
			{"response.content_part.added", object{"part": empty}},
			{prefix + ".delta", object{"delta": part[field]}},
			{prefix + ".done", object{field: part[field]}},
			{"response.content_part.done", object{"part": part}},
		}
		for _, event := range events {
			event.payload["output_index"], event.payload["item_id"], event.payload["content_index"] = index, id, i
			if err := emit(event.kind, event.payload); err != nil {
				return err
			}
		}
	}
	return emit("response.output_item.done", object{"output_index": index, "item": item})
}
