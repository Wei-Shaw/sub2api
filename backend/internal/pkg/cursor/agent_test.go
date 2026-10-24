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

	// McpTools is the repeated field itself: top-level reads yield each
	// McpToolDefinition directly (field 1, WireBytes), with no wrapper.
	container := NewProtobufReader(encoded)
	f, err := container.Next()
	require.NoError(t, err)
	require.Equal(t, uint32(1), f.Num)
	require.Equal(t, WireBytes, f.WireType)
	def := f

	// No further top-level entries beyond the single definition.
	rest, err := container.Next()
	require.NoError(t, err)
	require.Nil(t, rest)

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
	encoded := EncodeRequestContextReply(7, "guid-1", "be terse", []AgentTool{{
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
			// The system prompt rides as a global user rule alongside tools.
			require.Contains(t, string(ctxMsg.Data), "be terse")
			require.Contains(t, string(ctxMsg.Data), "/sub2api/system-prompt/0.mdc")
			fields := map[uint32]bool{}
			cr := NewProtobufReader(ctxMsg.Data)
			for {
				cf, err := cr.Next()
				if cf == nil || err != nil {
					break
				}
				if cf.WireType == WireBytes {
					fields[cf.Num] = true
				}
			}
			require.True(t, fields[fieldRequestContextRules])
			require.True(t, fields[fieldRequestContextTools])
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

func TestEncodeInteractionQueryReply(t *testing.T) {
	// Hosted search/fetch queries are approved with an empty approved result.
	for _, queryField := range []uint32{
		fieldInteractionQueryWebSearch,
		fieldInteractionQueryExaSrch,
		fieldInteractionQueryExaFetch,
		fieldInteractionQueryWebFetch,
	} {
		encoded := EncodeInteractionQueryReply(3, queryField)
		require.NotEmpty(t, encoded)
		client := NewProtobufReader(encoded)
		msg, err := client.Next()
		require.NoError(t, err)
		require.Equal(t, uint32(fieldAgentClientInteractionResp), msg.Num)
		// Approved result: oneof field with an empty approved message (field 1).
		require.Contains(t, string(msg.Data), string([]byte{byte(fieldRequestResponseApproved)<<3 | 2, 0}))
		require.NotContains(t, string(msg.Data), "not supported")
	}
	// Mode switches are declined.
	encoded := EncodeInteractionQueryReply(3, fieldInteractionQuerySwitchMo)
	require.NotNil(t, encoded)
	client := NewProtobufReader(encoded)
	msg, err := client.Next()
	require.NoError(t, err)
	require.Contains(t, string(msg.Data), "not supported by this gateway")
	// Unknown query kinds get an approved-shaped reply (same field number) so
	// the turn never stalls on an unanswered query.
	unknown := EncodeInteractionQueryReply(3, 11)
	require.NotEmpty(t, unknown)
	unknownMsg, err := NewProtobufReader(unknown).Next()
	require.NoError(t, err)
	require.Equal(t, uint32(fieldAgentClientInteractionResp), unknownMsg.Num)
	require.Contains(t, string(unknownMsg.Data), string([]byte{11<<3 | 2}))
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
func TestEncodeAgentTurnBlobsReferencesKVStoredTurns(t *testing.T) {
	blobs := &RootPromptBlobs{ByIDs: make(map[string][]byte)}
	turnIDs := EncodeAgentTurnBlobs(blobs, []AgentTurn{{
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
	require.Len(t, turnIDs, 1)

	// Every declared id must be served by the blob set.
	turn := blobs.ByIDs[string(turnIDs[0])]
	require.NotNil(t, turn, "turn blob must exist")
	require.Contains(t, blobs.ByIDs, string(turnIDs[0]))

	// The turn blob decodes as ConversationTurnStructure{agent_conversation_turn=1}.
	agentField := GetNested(turn, fieldTurnAgent)
	require.NotNil(t, agentField)

	// Agent turn references a user-message blob and step blobs by id.
	userID := GetNested(agentField, fieldAgentTurnUser)
	require.NotNil(t, userID)
	require.Len(t, userID, 32, "user message referenced by SHA-256 blob id")
	userMsg := blobs.ByIDs[string(userID)]
	require.NotNil(t, userMsg)
	require.Contains(t, string(userMsg), "what is the weather")

	steps := 0
	pr := NewProtobufReader(agentField)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		if f.Num == fieldAgentTurnStep && f.WireType == WireBytes {
			steps++
			require.Len(t, f.Data, 32, "step referenced by blob id")
			step := blobs.ByIDs[string(f.Data)]
			require.NotNil(t, step)
		}
	}
	require.Equal(t, 3, steps)

	// The tool-call step blob pairs the call with its result.
	for _, data := range blobs.ByIDs {
		if bytes.Contains(data, []byte("get_weather")) && bytes.Contains(data, []byte("sunny")) {
			return
		}
	}
	t.Fatal("no blob pairs the tool call with its result")
}

func TestNormalizeToolCallID(t *testing.T) {
	require.Equal(t, "call-abc_123", NormalizeToolCallID("call-abc\n123"))
	require.Equal(t, "call_x", NormalizeToolCallID("call x"))
	require.Len(t, NormalizeToolCallID(strings.Repeat("a", 100)), 64)
	require.Equal(t, "call-1", NormalizeToolCallID("call-1"))
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
	payload, _, _, _ := buildAgentRunMessage(AgentRunRequest{
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

	// Replayed turns ride on the conversation state as 32-byte blob ids.
	turnCount := 0
	pr := NewProtobufReader(state)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		if f.Num == fieldConvStateTurns && f.WireType == WireBytes {
			turnCount++
			require.Len(t, f.Data, 32, "turn referenced by blob id, not inline")
		}
	}
	require.Equal(t, 1, turnCount)

	// Root-prompt blob ids are also declared (field 1).
	rootPromptIDs := 0
	pr = NewProtobufReader(state)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		if f.Num == fieldConvStateRootPrompts && f.WireType == WireBytes {
			rootPromptIDs++
			require.Len(t, f.Data, 32)
		}
	}
	require.Positive(t, rootPromptIDs)

	// Tool table declared on the run request.
	tools := GetNested(runMsg, fieldRunMcpTools)
	require.NotNil(t, tools)
	require.Contains(t, string(tools), "get_weather")
	require.Contains(t, string(tools), AgentToolProviderIdentifier)

	// The system prompt never rides the (server-rejected) custom_system field;
	// it lives in the root-prompt blobs on the conversation state.
	require.Contains(t, string(runMsg), "/sub2api/system-prompt/0.mdc")
	// Action user message present.
	require.Contains(t, string(runMsg), "weather?")
}

func TestBuildAgentRunMessageAskModeUnchanged(t *testing.T) {
	payload, _, _, _ := buildAgentRunMessage(AgentRunRequest{
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
	rootPromptIDs := 0
	pr := NewProtobufReader(state)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		if f.Num == fieldConvStateTurns {
			turns++
		}
		if f.Num == fieldConvStateRootPrompts {
			rootPromptIDs++
		}
	}
	require.Zero(t, turns)
	// The system prompt rides as a root-prompt blob even in Ask mode.
	require.Equal(t, 1, rootPromptIDs)
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

func TestBuildAgentRunMessageEncodesImages(t *testing.T) {
	payload, _, _, _ := buildAgentRunMessage(AgentRunRequest{
		Model: "grok-4.6",
		Messages: []ChatMessage{
			{Role: "user", Content: "what is in this picture?"},
		},
		Images: []AgentImage{
			{Mime: "image/png", Data: []byte{0x89, 0x50, 0x4E, 0x47}},
		},
	})

	runMsg := GetNested(payload, fieldAgentClientRunRequest)
	require.NotNil(t, runMsg)
	action := GetNested(runMsg, fieldRunAction)
	require.NotNil(t, action)
	userAction := GetNested(action, fieldActionUserMessage)
	require.NotNil(t, userAction)
	userMsg := GetNested(userAction, fieldUserMsgActionMessage)
	require.NotNil(t, userMsg)

	// selected_context (field 3) wraps the image list with raw data payloads.
	ctx := GetNested(userMsg, fieldUserMsgSelectedContext)
	require.NotNil(t, ctx, "selected_context must be set when images present")
	body := string(ctx)
	require.Contains(t, body, "image/png")
	require.Contains(t, body, string([]byte{0x89, 0x50, 0x4E, 0x47}))

	// Without images the field is absent.
	plain, _, _, _ := buildAgentRunMessage(AgentRunRequest{
		Model:    "grok-4.6",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	})
	runPlain := GetNested(plain, fieldAgentClientRunRequest)
	actionPlain := GetNested(runPlain, fieldRunAction)
	userActionPlain := GetNested(actionPlain, fieldActionUserMessage)
	userMsgPlain := GetNested(userActionPlain, fieldUserMsgActionMessage)
	require.Nil(t, GetNested(userMsgPlain, fieldUserMsgSelectedContext))
}

func TestTrimAgentTurnsDropsOldestUnderBudget(t *testing.T) {
	big := strings.Repeat("x", 1000)
	turns := []AgentTurn{
		{UserText: big, Steps: []AgentTurnStep{{AssistantText: big}}},
		{UserText: big, Steps: []AgentTurnStep{{AssistantText: big}}},
		{UserText: big, Steps: []AgentTurnStep{{AssistantText: big}}},
	}
	// Budget fits exactly one big turn: the two oldest drop, the newest stays.
	trimmed := TrimAgentTurns(turns, 2100)
	require.Len(t, trimmed, 1)
	require.Equal(t, turns[2].UserText, trimmed[0].UserText, "most recent turn survives")

	// A single oversized turn is never dropped.
	solo := TrimAgentTurns(turns[:1], 10)
	require.Len(t, solo, 1)

	// Tool pairing inside kept turns is untouched.
	withTool := []AgentTurn{
		{UserText: big, Steps: []AgentTurnStep{{ToolCall: &AgentToolCallRecord{
			CallID: "c1", Name: "t", ArgsJSON: json.RawMessage(`{}`),
			Result: &AgentToolResultRecord{ContentText: big},
		}}}},
		{UserText: "latest"},
	}
	trimmedTool := TrimAgentTurns(withTool, 100)
	require.Len(t, trimmedTool, 1)
	require.Equal(t, "latest", trimmedTool[0].UserText)
}
