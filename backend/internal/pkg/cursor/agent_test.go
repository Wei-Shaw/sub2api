package cursor

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEncodeAgentToolsGoldenFrame locks the McpToolDefinition wire shape:
// name=1, description=2, input_schema=3 (google.protobuf.Value struct),
// provider_identifier=4, tool_name=5, wrapped in McpTools{1}.
func TestEncodeAgentToolsGoldenFrame(t *testing.T) {
	encoded := EncodeAgentTools([]AgentTool{{
		Name:        "get_weather",
		Description: "Look up weather",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
	}})
	require.NotEmpty(t, encoded)

	// Outer container: field 1 (WireBytes) wrapping the definitions.
	container := NewProtobufReader(encoded)
	f, err := container.Next()
	require.NoError(t, err)
	require.Equal(t, uint32(1), f.Num)
	require.Equal(t, WireBytes, f.WireType)

	// Exactly one definition inside.
	defs := NewProtobufReader(f.Data)
	def, err := defs.Next()
	require.NoError(t, err)
	require.Equal(t, uint32(1), def.Num)

	fields := map[uint32]string{}
	var schema []byte
	pr := NewProtobufReader(def.Data)
	for {
		pf, err := pr.Next()
		if pf == nil || err != nil {
			break
		}
		switch pf.Num {
		case 1, 2, 4, 5:
			fields[pf.Num] = string(pf.Data)
		case 3:
			schema = append([]byte(nil), pf.Data...)
		}
	}
	require.Equal(t, "get_weather", fields[1])
	require.Equal(t, "Look up weather", fields[2])
	require.Equal(t, AgentToolProviderIdentifier, fields[4])
	require.Equal(t, "get_weather", fields[5])
	require.NotEmpty(t, schema)

	// Schema round-trips through the protobuf Value struct encoding.
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(DecodeProtoJSONValue(schema), &decoded))
	require.Equal(t, "object", decoded["type"])
	props, ok := decoded["properties"].(map[string]any)
	require.True(t, ok)
	city, ok := props["city"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "string", city["type"])
}

func TestEncodeAgentToolsEmpty(t *testing.T) {
	require.Empty(t, EncodeAgentTools(nil))
	require.Empty(t, EncodeAgentTools([]AgentTool{}))
}

// TestEncodeProtoJSONValueVariants covers every google.protobuf.Value kind.
func TestEncodeProtoJSONValueVariants(t *testing.T) {
	raw := json.RawMessage(`{"n":1.5,"s":"hi","b":true,"z":null,"a":[1,"x",false],"o":{"k":"v"}}`)
	encoded, err := EncodeProtoJSONValue(raw)
	require.NoError(t, err)

	decodedValue, ok := readProtoJSONValue(NewProtobufReader(encoded))
	require.True(t, ok)
	decoded, err := json.Marshal(decodedValue)
	require.NoError(t, err)

	var round map[string]any
	require.NoError(t, json.Unmarshal(decoded, &round))
	require.Equal(t, 1.5, round["n"])
	require.Equal(t, "hi", round["s"])
	require.Equal(t, true, round["b"])
	require.Nil(t, round["z"])
	list, ok := round["a"].([]any)
	require.True(t, ok)
	require.Len(t, list, 3)
	require.Equal(t, 1.0, list[0])
	inner, ok := round["o"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "v", inner["k"])
}

func TestEncodeProtoJSONValueInvalid(t *testing.T) {
	_, err := EncodeProtoJSONValue(json.RawMessage(`{not json`))
	require.Error(t, err)
}

func TestDecodeProtoJSONValueDegradesToString(t *testing.T) {
	// Garbage bytes decode to a JSON string rather than failing.
	out := DecodeProtoJSONValue([]byte{0xff, 0xfe})
	require.True(t, json.Valid([]byte(out)))
	require.NotEqual(t, `""`, string(out)) // bytes preserved, not silently emptied
}

// TestEncodeRequestContextReplyGoldenFrame verifies the reply envelope:
// AgentClientMessage{exec_client_message=2{..., request_context_result=10}}.
func TestEncodeRequestContextReplyGoldenFrame(t *testing.T) {
	encoded := EncodeRequestContextReply(7, "guid-1", []AgentTool{{
		Name:        "ping",
		Description: "reply pong",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}})

	client := NewProtobufReader(encoded)
	msg, err := client.Next()
	require.NoError(t, err)
	require.Equal(t, uint32(fieldAgentClientExecReply), msg.Num)

	exec := parseExecServerReplyIDs(msg.Data)
	require.Equal(t, uint64(7), exec.id)
	require.Equal(t, "guid-1", exec.guid)

	pr := NewProtobufReader(msg.Data)
	foundResult := false
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		if f.Num == fieldExecRequr && f.WireType == WireBytes {
			foundResult = true
			// RequestContextResult{success=1{request_context=1{tools=7}}}
			result := NewProtobufReader(f.Data)
			success, err := result.Next()
			require.NoError(t, err)
			require.Equal(t, uint32(1), success.Num)
			ctx := NewProtobufReader(success.Data)
			ctxMsg, err := ctx.Next()
			require.NoError(t, err)
			require.Equal(t, uint32(1), ctxMsg.Num)
			tools := NewProtobufReader(ctxMsg.Data)
			tool, err := tools.Next()
			require.NoError(t, err)
			require.Equal(t, uint32(fieldRequestContextTools), tool.Num)
			require.Contains(t, string(tool.Data), "ping")
		}
	}
	require.True(t, foundResult)
}

type execReplyIDs struct {
	id   uint64
	guid string
}

func parseExecServerReplyIDs(payload []byte) execReplyIDs {
	var out execReplyIDs
	pr := NewProtobufReader(payload)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		switch {
		case f.Num == fieldExecID && f.WireType == WireVarint:
			out.id = f.Varint
		case f.Num == fieldExecGUID && f.WireType == WireBytes:
			out.guid = string(f.Data)
		}
	}
	return out
}

func TestEncodeMcpHandoffReply(t *testing.T) {
	encoded := EncodeMcpHandoffReply(42, "")
	client := NewProtobufReader(encoded)
	msg, err := client.Next()
	require.NoError(t, err)
	require.Equal(t, uint32(fieldAgentClientExecReply), msg.Num)

	require.Equal(t, uint64(42), parseExecServerReplyIDs(msg.Data).id)

	pr := NewProtobufReader(msg.Data)
	found := false
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		if f.Num == fieldExecMcp && f.WireType == WireBytes {
			found = true
			// McpResult{success=1{content=1{text=1}}}
			result := NewProtobufReader(f.Data)
			success, err := result.Next()
			require.NoError(t, err)
			require.Equal(t, uint32(fieldMcpResultSuccess), success.Num)
			require.Contains(t, string(success.Data), MCPExternalHandoffMessage)
			require.False(t, strings.Contains(string(success.Data), `"is_error"`))
		}
	}
	require.True(t, found)
}

func TestEncodeMcpToolNotFoundReply(t *testing.T) {
	encoded := EncodeMcpToolNotFoundReply(9, "guid-9", "phantom_tool", []string{"declared_a", "declared_b"})
	client := NewProtobufReader(encoded)
	msg, err := client.Next()
	require.NoError(t, err)
	require.Equal(t, uint64(9), parseExecServerReplyIDs(msg.Data).id)

	pr := NewProtobufReader(msg.Data)
	found := false
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		if f.Num == fieldExecMcp && f.WireType == WireBytes {
			found = true
			result := NewProtobufReader(f.Data)
			notFound, err := result.Next()
			require.NoError(t, err)
			require.Equal(t, uint32(fieldMcpResultNotFound), notFound.Num)
			body := string(notFound.Data)
			require.Contains(t, body, "phantom_tool")
			require.Contains(t, body, "declared_a")
			require.Contains(t, body, "declared_b")
		}
	}
	require.True(t, found)
}

func TestEncodeInteractionRejectedReply(t *testing.T) {
	// Known query kinds produce a rejected response frame.
	for _, queryField := range []uint32{fieldInteractionQueryWebSearch, fieldInteractionQuerySwitchMo} {
		encoded := EncodeInteractionRejectedReply(3, queryField)
		require.NotEmpty(t, encoded)
		client := NewProtobufReader(encoded)
		msg, err := client.Next()
		require.NoError(t, err)
		require.Equal(t, uint32(fieldAgentClientInteractionResp), msg.Num)
		require.Contains(t, string(msg.Data), "not supported by this gateway")
	}
	// Unknown query kinds are ignored (no reply).
	require.Empty(t, EncodeInteractionRejectedReply(3, 99))
}

func TestParseExecServerFrameKinds(t *testing.T) {
	// MCP call frame: id=1, mcp_args=11.
	var mcpArgs ProtobufWriter
	mcpArgs.String(fieldMcpArgsName, "get_weather")
	mcpArgs.String(fieldMcpArgsToolCallID, "call-1")
	var requestCtx ProtobufWriter
	requestCtx.Varint(2, 0)
	var other ProtobufWriter
	other.Varint(2, 0)

	var mcpPayload ProtobufWriter
	mcpPayload.Varint(fieldExecID, 5)
	mcpPayload.Bytes(fieldExecMcp, mcpArgs.Result())

	var ctxPayload ProtobufWriter
	ctxPayload.Varint(fieldExecID, 6)
	ctxPayload.Bytes(fieldExecRequr, requestCtx.Result())

	var otherPayload ProtobufWriter
	otherPayload.Varint(fieldExecID, 7)
	otherPayload.Bytes(2, other.Result()) // shell_args

	mcpFrame := parseExecServerFrame(mcpPayload.Result())
	require.Equal(t, execKindMcpCall, mcpFrame.Kind)
	require.Equal(t, uint64(5), mcpFrame.ID)
	require.Equal(t, "get_weather", mcpFrame.Tool.Name)
	require.Equal(t, "call-1", mcpFrame.Tool.ID)
	require.True(t, mcpFrame.Tool.IsLast)

	ctxFrame := parseExecServerFrame(ctxPayload.Result())
	require.Equal(t, execKindRequestContext, ctxFrame.Kind)
	require.Equal(t, uint64(6), ctxFrame.ID)

	otherFrame := parseExecServerFrame(otherPayload.Result())
	require.Equal(t, execKindOther, otherFrame.Kind)
	require.Equal(t, uint64(7), otherFrame.ID)

	require.Equal(t, execKindNone, parseExecServerFrame(nil).Kind)
}

func TestParseMcpArgsMergesArgsMap(t *testing.T) {
	city, err := EncodeProtoJSONValue(json.RawMessage(`"Paris"`))
	require.NoError(t, err)
	unit, err := EncodeProtoJSONValue(json.RawMessage(`"celsius"`))
	require.NoError(t, err)

	var entry1, entry2 ProtobufWriter
	entry1.String(1, "city")
	entry1.Bytes(2, city)
	entry2.String(1, "unit")
	entry2.Bytes(2, unit)

	var args ProtobufWriter
	args.String(fieldMcpArgsName, "get_weather")
	args.Bytes(fieldMcpArgsArgs, entry1.Result())
	args.Bytes(fieldMcpArgsArgs, entry2.Result())
	args.String(fieldMcpArgsToolCallID, "call-77")
	args.String(fieldMcpArgsProvider, AgentToolProviderIdentifier)

	event := parseMcpArgs(args.Result())
	require.Equal(t, "get_weather", event.Name)
	require.Equal(t, "call-77", event.ID)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(event.RawArgs), &parsed))
	require.Equal(t, "Paris", parsed["city"])
	require.Equal(t, "celsius", parsed["unit"])
}

// TestEncodeAgentTurnsGoldenFrame locks the replay shape:
// ConversationTurn{agent_conversation_turn=1{user_message=1, steps=2}} with
// assistant/tool/thinking steps and paired McpToolCall results.
func TestEncodeAgentTurnsGoldenFrame(t *testing.T) {
	turns := EncodeAgentTurns([]AgentTurn{{
		UserText: "what is the weather",
		Steps: []AgentTurnStep{
			{AssistantText: "let me check"},
			{ToolCall: &AgentToolCallRecord{
				CallID:   "call-1",
				Name:     "get_weather",
				ArgsJSON: json.RawMessage(`{"city":"Paris"}`),
				Result:   &AgentToolResultRecord{ContentText: "sunny"},
			}},
			{AssistantText: "it is sunny"},
		},
	}})
	require.Len(t, turns, 1)

	turn := NewProtobufReader(turns[0])
	agentField, err := turn.Next()
	require.NoError(t, err)
	require.Equal(t, uint32(fieldTurnAgent), agentField.Num)

	agent := NewProtobufReader(agentField.Data)
	var userSeen, assistantSeen, toolSeen int
	var thinkingSeen int
	var toolStep []byte
	for {
		f, err := agent.Next()
		if f == nil || err != nil {
			break
		}
		switch {
		case f.Num == fieldAgentTurnUser && f.WireType == WireBytes:
			userSeen++
			require.Contains(t, string(f.Data), "what is the weather")
		case f.Num == fieldAgentTurnStep && f.WireType == WireBytes:
			step := NewProtobufReader(f.Data)
			sf, err := step.Next()
			require.NoError(t, err)
			switch sf.Num {
			case fieldStepAssistant:
				assistantSeen++
			case fieldStepToolCall:
				toolSeen++
				toolStep = append([]byte(nil), sf.Data...)
			case fieldStepThinking:
				thinkingSeen++
			}
		}
	}
	require.Equal(t, 1, userSeen)
	require.Equal(t, 2, assistantSeen)
	require.Equal(t, 1, toolSeen)
	require.Equal(t, 0, thinkingSeen)

	// Tool step payload is the ToolCall message itself:
	// {mcp_tool_call=15{args=1, result=2}, tool_call_id=57}.
	call := NewProtobufReader(toolStep)
	var mcpSeen, idSeen bool
	for {
		f, err := call.Next()
		if f == nil || err != nil {
			break
		}
		switch {
		case f.Num == fieldToolCallMcp && f.WireType == WireBytes:
			mcpSeen = true
			require.Contains(t, string(f.Data), "get_weather")
			require.Contains(t, string(f.Data), "call-1")
			require.Contains(t, string(f.Data), "sunny")
		case f.Num == fieldToolCallCallIDE && f.WireType == WireBytes:
			idSeen = true
			require.Equal(t, "call-1", string(f.Data))
		}
	}
	require.True(t, mcpSeen)
	require.True(t, idSeen)
}

func TestEncodeAgentTurnsEmptyTurnDropped(t *testing.T) {
	require.Empty(t, EncodeAgentTurns(nil))
	require.Empty(t, EncodeAgentTurns([]AgentTurn{{}}))
}

func TestSplitToolArgsJSON(t *testing.T) {
	out := splitToolArgsJSON(json.RawMessage(`{"a":1,"b":"x"}`))
	require.Len(t, out, 2)
	require.JSONEq(t, "1", string(out["a"]))
	require.JSONEq(t, `"x"`, string(out["b"]))

	// Non-object JSON collapses under a single "args" key.
	out = splitToolArgsJSON(json.RawMessage(`[1,2]`))
	require.Len(t, out, 1)
	require.JSONEq(t, "[1,2]", string(out["args"]))

	require.Empty(t, splitToolArgsJSON(nil))
}

func TestBuildAgentRunMessageAgentMode(t *testing.T) {
	payload, _, _ := buildAgentRunMessage(AgentRunRequest{
		Model: "claude-opus-5-high",
		Messages: []ChatMessage{
			{Role: "system", Content: "be terse"},
			{Role: "user", Content: "weather?"},
		},
		Tools: []AgentTool{{
			Name:        "get_weather",
			Description: "weather lookup",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
		Turns: []AgentTurn{{
			UserText: "hi",
			Steps:    []AgentTurnStep{{AssistantText: "hello"}},
		}},
	})

	runMsg := GetNested(payload, fieldAgentClientRunRequest)
	require.NotNil(t, runMsg)

	state := GetNested(runMsg, fieldRunConversationState)
	require.NotNil(t, state)
	require.Equal(t, uint64(AgentModeAgent), getVarint(state, fieldConvStateMode))

	// Replayed turns ride on the conversation state.
	turnCount := 0
	pr := NewProtobufReader(state)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		if f.Num == fieldConvStateTurns && f.WireType == WireBytes {
			turnCount++
			require.Contains(t, string(f.Data), "hi")
		}
	}
	require.Equal(t, 1, turnCount)

	// Tool table declared on the run request.
	tools := GetNested(runMsg, fieldRunMcpTools)
	require.NotNil(t, tools)
	require.Contains(t, string(tools), "get_weather")
	require.Contains(t, string(tools), AgentToolProviderIdentifier)

	// System prompt hoisted to the custom system field.
	require.Contains(t, string(runMsg), "be terse")
	// Action user message present.
	require.Contains(t, string(runMsg), "weather?")
}

func TestBuildAgentRunMessageAskModeUnchanged(t *testing.T) {
	payload, _, _ := buildAgentRunMessage(AgentRunRequest{
		Model: "grok-4.6",
		Messages: []ChatMessage{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "hey"},
			{Role: "assistant", Content: "hi there"},
			{Role: "user", Content: "bye"},
		},
	})

	runMsg := GetNested(payload, fieldAgentClientRunRequest)
	state := GetNested(runMsg, fieldRunConversationState)
	require.Equal(t, uint64(AgentModeAsk), getVarint(state, fieldConvStateMode))

	// No tool table and no replayed turns on the Ask path. The run request
	// still carries an empty mcp_tools field (present since the Ask path
	// shipped), so assert emptiness rather than absence.
	tools := GetNested(runMsg, fieldRunMcpTools)
	require.Empty(t, tools)
	turns := 0
	pr := NewProtobufReader(state)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		if f.Num == fieldConvStateTurns {
			turns++
		}
	}
	require.Zero(t, turns)
}

// TestConsumeAssistantStreamSurfacesToolCalls proves exec-encoded MCP tool
// calls flow through the stream parser as tool_call events.
func TestConsumeAssistantStreamSurfacesToolCalls(t *testing.T) {
	var mcpArgs ProtobufWriter
	mcpArgs.String(fieldMcpArgsName, "get_weather")
	mcpArgs.String(fieldMcpArgsToolCallID, "call-9")
	var exec ProtobufWriter
	exec.Varint(fieldExecID, 3)
	exec.Bytes(fieldExecMcp, mcpArgs.Result())
	var server ProtobufWriter
	server.Bytes(fieldAgentServerExec, exec.Result())
	frame, err := EncodeFrame(server.Result(), false)
	require.NoError(t, err)

	var ended ProtobufWriter
	ended.Varint(fieldTurnEndedOutputTokens, 4)
	var endUpdate ProtobufWriter
	endUpdate.Bytes(fieldInteractionTurnEnded, ended.Result())
	var endServer ProtobufWriter
	endServer.Bytes(fieldAgentServerInteraction, endUpdate.Result())
	endFrame, err := EncodeFrame(endServer.Result(), false)
	require.NoError(t, err)

	var events []StreamEvent
	usage, connectErr := ConsumeAssistantStream(bytes.NewReader(append(frame, endFrame...)), func(ev StreamEvent) error {
		events = append(events, ev)
		return nil
	})
	require.Empty(t, connectErr)
	require.Equal(t, 4, usage.OutputTokens)
	require.Len(t, events, 1)
	require.Equal(t, "tool_call", events[0].Type)
	require.NotNil(t, events[0].ToolCall)
	require.Equal(t, "get_weather", events[0].ToolCall.Name)
	require.Equal(t, "call-9", events[0].ToolCall.ID)
}
