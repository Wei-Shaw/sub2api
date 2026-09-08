# AI Relay Codex Diff Compatibility Design

## Goal

Make the AI Relay OpenCode `/v1/responses` bridge preserve Codex-native file modification tool calls so that Codex Desktop/IDE can render file diffs and Codex CLI can execute and summarize the same changes.

## Scope

The change applies only to the OpenCode platform's Responses-to-Chat-Completions bridge in `plugin-service/plugins/ai-relay/backend`. Direct Chat Completions forwarding and other relay platforms keep their current behavior.

## Current Behavior

The bridge currently converts every Responses tool into a Chat Completions function and converts every upstream tool call back into a Responses `function_call`. Codex freeform tools such as `apply_patch` use the `custom` tool type and require a `custom_tool_call` lifecycle. Losing that type prevents Codex clients from recognizing a native file change, so their normal diff presentation is not produced.

Chat Completions cannot represent a Responses freeform tool directly. The bridge therefore needs a reversible wrapper that keeps the original tool name and freeform input intact.

## Design

### Request Context

Responses request conversion will return both the Chat Completions request body and a request-scoped tool bridge context. The context records:

- Original Codex custom tool names.
- The same function name used upstream.
- The original Responses tool type needed on the return path.

### Request Conversion

For a Responses `custom` tool:

1. Preserve its original name in the bridge context.
2. Expose a same-name Chat Completions function.
3. Replace its freeform grammar with a reversible JSON schema containing one required string property named `input`.
4. Preserve ordinary Responses `function` tools using the current conversion.

Historical `custom_tool_call` input is wrapped as `{"input":"<freeform text>"}` when converted to a Chat assistant tool call. The input history converter will accept both `function_call_output` and `custom_tool_call_output`, converting either into the corresponding Chat Completions tool message.

### Response Conversion

Both streaming and non-streaming response converters receive the request-scoped context.

When an upstream tool call matches a mapped custom tool:

- Return a Responses item with `type: custom_tool_call`.
- Preserve the original name such as `apply_patch`.
- Extract the `input` property from Chat function arguments and place its raw value in the Responses `input` field.

For streaming responses, emit the Codex custom-tool lifecycle:

1. `response.output_item.added`
2. `response.custom_tool_call_input.delta`
3. `response.custom_tool_call_input.done`
4. `response.output_item.done`
5. Include the completed `custom_tool_call` in `response.completed.response.output`

Ordinary function calls continue to use the existing `response.function_call_arguments.*` lifecycle.

### Freeform Input Wrapper

The upstream Chat function schema is:

```json
{
  "type": "object",
  "properties": {
    "input": {
      "type": "string",
      "description": "The raw input for this tool, passed through verbatim."
    }
  },
  "required": ["input"]
}
```

When the upstream returns `{"input":"*** Begin Patch..."}`, the bridge extracts the string verbatim. If arguments are invalid JSON or do not contain a string `input`, it uses the raw arguments as the custom-tool input so the Codex client reports the real tool parsing error instead of losing the call.

## Error Handling

- Malformed tool definitions are skipped using the bridge's existing validation behavior.
- Invalid custom-tool response arguments are preserved as raw custom-tool input.
- Custom tools are identified from the current request rather than from a hard-coded name list, so tools other than `apply_patch` round-trip through the same wrapper.
- No relay request fails solely because an upstream model emitted an unmappable tool call.

## Tests

Add focused tests in `opencode_responses_test.go` for:

- `custom apply_patch` request conversion to a same-name function with the freeform wrapper schema.
- Non-streaming `apply_patch` function response conversion to a completed `custom_tool_call` with verbatim patch input.
- Streaming custom-tool event ordering and completed output.
- Multiline patch input preservation.
- Malformed wrapper fallback behavior.
- `custom_tool_call_output` continuation conversion.
- Existing ordinary function request and response behavior.

Run the focused AI Relay backend tests first, followed by all `plugin-service` tests.

## Non-Goals

- Generating UI diffs inside the relay service.
- Reading repository files on the relay server.
- Changing Codex Desktop, IDE, or CLI code.
- Refactoring the backend `apicompat` package into a shared Go module.
