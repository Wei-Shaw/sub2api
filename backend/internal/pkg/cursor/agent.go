package cursor

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// newReplayMessageID mints UserMessage.message_id for replayed turns.
func newReplayMessageID() string { return uuid.New().String() }

// agent.v1 AgentService/Run tool-calling protocol: caller-declared MCP tools,
// inline exec replies (request context, MCP tool call handoffs), and
// conversation replay turns. Field numbers mirror Cursor 3.16's agent.proto.

const (
	// AgentToolProviderIdentifier marks tools executed by the API caller, not
	// by an in-process agent. Cursor only routes McpArgs frames for tools
	// declared with this provider.
	AgentToolProviderIdentifier = "pi-agent"

	// MCPExternalHandoffMessage is the placeholder tool result acknowledging a
	// handoff: it instructs the model to stop and wait for the real result,
	// which the API caller delivers on the next request.
	MCPExternalHandoffMessage = "Tool call received and handed off to the external client for execution. " +
		"Do not retry or call it again; end the turn. The result will be provided in the next request."
)

// agent.v1 McpToolDefinition / McpTools.
const (
	fieldToolDefName              = 1
	fieldToolDefDescription       = 2
	fieldToolDefInputSchema       = 3
	fieldToolDefProviderIdentifer = 4
	fieldToolDefToolName          = 5
	fieldMcpToolsDefinitions      = 1
)

// AgentClientMessage oneof (run_request=1, kv=3, heartbeat=7 live in nal.go).
const (
	fieldAgentClientExecReply       = 2
	fieldAgentClientInteractionResp = 6
)

// ExecServerMessage / ExecClientMessage.
const (
	fieldExecID     = 1
	fieldExecGUID   = 15
	fieldExecRequr  = 10 // RequestContextArgs / RequestContextResult
	fieldExecMcp    = 11 // McpArgs / McpResult
	fieldExecMcpSta = 36 // McpStateExecArgs / McpStateExecResult (tool discovery)
)

// McpArgs / McpResult / McpToolCall.
const (
	fieldMcpArgsName        = 1
	fieldMcpArgsArgs        = 2 // map<string, bytes>: argument name -> JSON value
	fieldMcpArgsToolCallID  = 3
	fieldMcpArgsProvider    = 4
	fieldMcpArgsToolName    = 5
	fieldMcpResultSuccess   = 1
	fieldMcpResultError     = 2
	fieldMcpResultNotFound  = 5
	fieldMcpSuccessContent  = 1
	fieldMcpSuccessIsError  = 2
	fieldMcpContentText     = 1
	fieldMcpTextContentText = 1
	fieldMcpNotFoundName    = 1
	fieldMcpNotFoundAvail   = 2
	fieldMcpCallArgs        = 1
	fieldMcpCallResult      = 2
)

// ToolCall envelope (ConversationStep.tool_call).
const (
	fieldToolCallMcp     = 15
	fieldToolCallCallIDE = 57
)

// ConversationStep / AgentConversationTurn / ConversationTurn.
const (
	fieldStepAssistant = 1
	fieldStepToolCall  = 2
	fieldStepThinking  = 3
	fieldTurnAgent     = 1
	fieldAgentTurnUser = 1
	fieldAgentTurnStep = 2
)

// ConversationStateStructure.
const (
	fieldConvStateRootPrompts = 1 // repeated bytes: root-prompt blob ids
	fieldConvStateTurns       = 8
	fieldConvStateModeOld     = 10 // nal.go fieldConvStateMode
)

// RequestContext / RequestContextResult / RequestContextSuccess.
const (
	fieldRequestContextResult     = 1
	fieldRequestContextTools      = 7
	fieldRequestCtxResultSuccess  = 1
	fieldRequestCtxSuccessPayload = 1
)

// InteractionUpdate tool-call deliveries (ToolCallStartedUpdate /
// ToolCallCompletedUpdate wrap the same ToolCall message).
const (
	fieldInteractionToolCallStarted   = 2
	fieldInteractionToolCallCompleted = 3
	fieldToolCallUpdateCallID         = 1
	fieldToolCallUpdateToolCall       = 2
)

// McpStateExecResult fields.
const (
	fieldMcpStateServers       = 1
	fieldMcpStateServerName    = 1
	fieldMcpStateServerIdent   = 2
	fieldMcpStateServerTools   = 5
	fieldMcpStateServerStatus  = 7
	fieldMcpStateResultSuccess = 1

	// mcpStateServerConnected marks the pi-agent server as live; without it
	// the model attempts an MCP authentication flow before calling tools.
	mcpStateServerConnected = "connected"
)

// InteractionQuery / InteractionResponse oneof fields.
const (
	fieldInteractionQueryWebSearch = 2
	fieldInteractionQuerySwitchMo  = 4
	fieldInteractionQueryExaSrch   = 5
	fieldInteractionQueryExaFetch  = 6
	fieldInteractionQueryWebFetch  = 9
	fieldInteractionRespID         = 1
	fieldInteractionRespWebSearch  = 2
	fieldInteractionRespSwitchMode = 4
	fieldInteractionRespExaSrch    = 5
	fieldInteractionRespExaFetch   = 6
	fieldInteractionRespWebFetch   = 9
	fieldRequestResponseApproved   = 1
	fieldRequestResponseRejected   = 2
	fieldRequestResponseReason     = 1
)

// AgentTool is one caller-executed tool advertised to Cursor as an MCP tool.
type AgentTool struct {
	Name        string
	Description string
	// InputSchema is the JSON Schema object describing the parameters.
	InputSchema json.RawMessage
}

// EncodeAgentTools serializes the caller's tool table as McpTools bytes for
// AgentRunRequest.mcp_tools.
func EncodeAgentTools(tools []AgentTool) []byte {
	if len(tools) == 0 {
		return nil
	}
	var list ProtobufWriter
	for _, tool := range tools {
		schema := tool.InputSchema
		if len(schema) == 0 {
			schema = json.RawMessage(`{}`)
		}
		schemaValue, err := EncodeProtoJSONValue(schema)
		if err != nil {
			// A schema that does not parse as JSON cannot be projected; send an
			// empty struct so the tool stays callable rather than dropping it.
			schemaValue, _ = EncodeProtoJSONValue(json.RawMessage(`{}`))
		}
		var def ProtobufWriter
		def.String(fieldToolDefName, tool.Name)
		def.String(fieldToolDefDescription, tool.Description)
		def.Bytes(fieldToolDefInputSchema, schemaValue)
		def.String(fieldToolDefProviderIdentifer, AgentToolProviderIdentifier)
		def.String(fieldToolDefToolName, tool.Name)
		list.Bytes(fieldMcpToolsDefinitions, def.Result())
	}
	// McpTools is itself the repeated field: the message bytes are the
	// concatenated definitions, with no extra wrapper. A wrapper layer here
	// decodes server-side as one corrupted definition and the model never
	// receives the tool table (observed live: schema-blind argument names and
	// an MCP-authentication detour).
	return list.Result()
}

// EncodeAgentToolsDeclaration wraps EncodeAgentTools for RequestContext.tools.
func encodeRequestContextTools(tools []AgentTool) []byte {
	encoded := EncodeAgentTools(tools)
	if len(encoded) == 0 {
		return nil
	}
	// EncodeAgentTools returns McpTools{definitions}; RequestContext wants the
	// bare repeated McpToolDefinition, so unwrap the outer message.
	var ctx ProtobufWriter
	r := NewProtobufReader(encoded)
	for {
		f, err := r.Next()
		if f == nil || err != nil {
			break
		}
		if f.Num == fieldMcpToolsDefinitions && f.WireType == WireBytes {
			ctx.Bytes(fieldRequestContextTools, f.Data)
		}
	}
	return ctx.Result()
}

// --- Inline exec replies -------------------------------------------------

// encodeExecReply wraps a payload in ExecClientMessage{field} inside
// AgentClientMessage{exec_client_message}, echoing the server's exec ids.
func encodeExecReply(id uint64, execGUID string, payloadField uint32, payload []byte) []byte {
	var reply ProtobufWriter
	reply.Varint(fieldExecID, int(id))
	if execGUID != "" {
		reply.String(fieldExecGUID, execGUID)
	}
	reply.Bytes(payloadField, payload)
	var client ProtobufWriter
	client.Bytes(fieldAgentClientExecReply, reply.Result())
	return client.Result()
}

// EncodeRequestContextReply answers ExecServerMessage.request_context_args
// with the declared tool table. A nil/empty answer strands the server-side
// turn, so callers must always reply.
func EncodeRequestContextReply(id uint64, execGUID string, tools []AgentTool) []byte {
	var success ProtobufWriter
	success.Bytes(fieldRequestCtxSuccessPayload, encodeRequestContextTools(tools))
	var result ProtobufWriter
	result.Bytes(fieldRequestContextResult, success.Result())
	return encodeExecReply(id, execGUID, fieldExecRequr, result.Result())
}

// EncodeMcpHandoffReply acknowledges an MCP tool call that the API caller will
// execute on its next request. The placeholder tells the model to end the turn
// instead of waiting for an inline result.
func EncodeMcpHandoffReply(id uint64, execGUID string) []byte {
	var text ProtobufWriter
	text.String(fieldMcpTextContentText, MCPExternalHandoffMessage)
	var content ProtobufWriter
	content.Bytes(fieldMcpContentText, text.Result())
	var success ProtobufWriter
	success.Bytes(fieldMcpSuccessContent, content.Result())
	var result ProtobufWriter
	result.Bytes(fieldMcpResultSuccess, success.Result())
	return encodeExecReply(id, execGUID, fieldExecMcp, result.Result())
}

// EncodeMcpToolNotFoundReply rejects a tool call for a tool the client never
// declared. Handing such calls off would promise a result that never arrives.
func EncodeMcpToolNotFoundReply(id uint64, execGUID, toolName string, declared []string) []byte {
	var notFound ProtobufWriter
	notFound.String(fieldMcpNotFoundName, toolName)
	for _, name := range declared {
		notFound.String(fieldMcpNotFoundAvail, name)
	}
	var result ProtobufWriter
	result.Bytes(fieldMcpResultNotFound, notFound.Result())
	return encodeExecReply(id, execGUID, fieldExecMcp, result.Result())
}

// EncodeInteractionQueryReply answers an InteractionQuery. Hosted web
// search / fetch / exa queries are approved (Cursor executes them
// server-side, mirroring the CLI client's behavior — rejecting them stalls
// the turn); mode switches are declined. Returns nil for query kinds without
// a known response shape, leaving the frame unanswered like the CLI does.
func EncodeInteractionQueryReply(id uint64, queryField uint32) []byte {
	var responseField uint32
	approve := false
	switch queryField {
	case fieldInteractionQueryWebSearch:
		responseField, approve = fieldInteractionRespWebSearch, true
	case fieldInteractionQueryExaSrch:
		responseField, approve = fieldInteractionRespExaSrch, true
	case fieldInteractionQueryExaFetch:
		responseField, approve = fieldInteractionRespExaFetch, true
	case fieldInteractionQueryWebFetch:
		responseField, approve = fieldInteractionRespWebFetch, true
	case fieldInteractionQuerySwitchMo:
		responseField = fieldInteractionRespSwitchMode
	default:
		// Unknown query kinds (the backend ships new ones faster than any
		// reverse-engineered proto): answer with an approved-shaped result
		// under the same field number rather than leaving the turn stalled —
		// live testing showed unanswered queries hang the stream. An empty
		// approved result mirrors the hosted-search responses; if the shape is
		// wrong the server errors visibly instead of deadlocking.
		responseField = queryField
		approve = true
	}
	var result ProtobufWriter
	if approve {
		result.Bytes(fieldRequestResponseApproved, nil)
	} else {
		result.String(fieldRequestResponseReason, "not supported by this gateway")
	}
	var response ProtobufWriter
	response.Bytes(responseField, result.Result())
	var client ProtobufWriter
	client.Varint(fieldInteractionRespID, int(id))
	client.Bytes(responseField, response.Result())
	return encodeAgentClientMessage(fieldAgentClientInteractionResp, client.Result())
}

func encodeAgentClientMessage(field uint32, payload []byte) []byte {
	var client ProtobufWriter
	client.Bytes(field, payload)
	return client.Result()
}

// --- Conversation replay turns ------------------------------------------

// AgentToolResultRecord is the caller-delivered outcome of a previous turn's
// tool call.
type AgentToolResultRecord struct {
	ContentText string
	IsError     bool
}

// AgentToolCallRecord is one assistant tool call from the replayed history.
// Result is nil for calls that are still pending (which the gateway never
// replays: a pending call means the previous turn had no result yet).
type AgentToolCallRecord struct {
	CallID     string
	Name       string
	ArgsJSON   json.RawMessage
	Result     *AgentToolResultRecord
	ServerKind bool // true when the call originated from Cursor's exec protocol
}

// AgentTurnStep is one ConversationStep inside a replayed turn.
type AgentTurnStep struct {
	AssistantText string
	ThinkingText  string
	ToolCall      *AgentToolCallRecord
}

// AgentTurn is one replayed user turn: the user message plus everything the
// assistant did in response.
type AgentTurn struct {
	UserText string
	Steps    []AgentTurnStep
}

func encodeToolCallStep(record AgentToolCallRecord) []byte {
	var mcpCall ProtobufWriter
	mcpCall.Bytes(fieldMcpCallArgs, encodeReplayMcpArgs(record))
	if record.Result != nil {
		mcpCall.Bytes(fieldMcpCallResult, encodeMcpResultFromRecord(*record.Result))
	}
	var toolCall ProtobufWriter
	toolCall.Bytes(fieldToolCallMcp, mcpCall.Result())
	if record.CallID != "" {
		toolCall.String(fieldToolCallCallIDE, record.CallID)
	}
	var step ProtobufWriter
	step.Bytes(fieldStepToolCall, toolCall.Result())
	return step.Result()
}

func encodeReplayMcpArgs(record AgentToolCallRecord) []byte {
	var args ProtobufWriter
	args.String(fieldMcpArgsName, record.Name)
	for key, value := range splitToolArgsJSON(record.ArgsJSON) {
		var entry ProtobufWriter
		entry.String(1, key)
		if encoded, err := EncodeProtoJSONValue(value); err == nil {
			entry.Bytes(2, encoded)
		} else {
			entry.Bytes(2, value)
		}
		args.Bytes(fieldMcpArgsArgs, entry.Result())
	}
	if record.CallID != "" {
		args.String(fieldMcpArgsToolCallID, record.CallID)
	}
	args.String(fieldMcpArgsProvider, AgentToolProviderIdentifier)
	args.String(fieldMcpArgsToolName, record.Name)
	return args.Result()
}

func encodeMcpResultFromRecord(result AgentToolResultRecord) []byte {
	var text ProtobufWriter
	text.String(fieldMcpTextContentText, result.ContentText)
	var content ProtobufWriter
	content.Bytes(fieldMcpContentText, text.Result())
	var success ProtobufWriter
	success.Bytes(fieldMcpSuccessContent, content.Result())
	if result.IsError {
		success.Varint(fieldMcpSuccessIsError, 1)
	}
	var out ProtobufWriter
	out.Bytes(fieldMcpResultSuccess, success.Result())
	return out.Result()
}

// splitToolArgsJSON breaks a JSON object into top-level key/value pairs for
// the McpArgs args map. Non-object JSON yields a single "args" entry.
func splitToolArgsJSON(raw json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage)
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return out
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		out["args"] = raw
		return out
	}
	for key, value := range obj {
		out[key] = value
	}
	return out
}

// --- Inbound exec frame parsing -----------------------------------------

type execKind int

const (
	execKindNone execKind = iota
	execKindRequestContext
	execKindMcpCall
	execKindMcpState
	execKindOther
)

type execServerFrame struct {
	ID     uint64
	ExecID string
	Kind   execKind
	// Tool is set when Kind == execKindMcpCall.
	Tool ToolCallEvent
	// ServerIDs are the identifiers an mcp_state query was scoped to.
	ServerIDs []string
}

// parseExecServerFrame decodes the pieces of ExecServerMessage the gateway
// must answer or surface.
func parseExecServerFrame(payload []byte) execServerFrame {
	frame := execServerFrame{Kind: execKindNone}
	pr := NewProtobufReader(payload)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		switch {
		case f.Num == fieldExecID && f.WireType == WireVarint:
			frame.ID = f.Varint
		case f.Num == fieldExecGUID && f.WireType == WireBytes:
			frame.ExecID = string(f.Data)
		case f.Num == fieldExecRequr && f.WireType == WireBytes:
			frame.Kind = execKindRequestContext
		case f.Num == fieldExecMcp && f.WireType == WireBytes:
			frame.Kind = execKindMcpCall
			frame.Tool = parseMcpArgs(f.Data)
		case f.Num == fieldExecMcpSta && f.WireType == WireBytes:
			frame.Kind = execKindMcpState
			frame.ServerIDs = parseMcpStateServerIDs(f.Data)
		default:
			if f.WireType == WireBytes || f.WireType == WireVarint {
				if frame.Kind == execKindNone {
					frame.Kind = execKindOther
				}
			}
		}
	}
	return frame
}

// parseMcpArgs decodes McpArgs into a tool call event. The args map entries
// are google.protobuf.Value encodings of each top-level argument.
func parseMcpArgs(payload []byte) ToolCallEvent {
	event := ToolCallEvent{IsLast: true}
	argsJSON := make(map[string]json.RawMessage)
	pr := NewProtobufReader(payload)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		switch {
		case f.Num == fieldMcpArgsName && f.WireType == WireBytes:
			event.Name = string(f.Data)
		case f.Num == fieldMcpArgsArgs && f.WireType == WireBytes:
			key, value := parseProtoMapEntry(f.Data)
			if key == "" {
				continue
			}
			argsJSON[key] = DecodeProtoJSONValue(value)
		case f.Num == fieldMcpArgsToolCallID && f.WireType == WireBytes:
			event.ID = string(f.Data)
		case f.Num == fieldMcpArgsToolName && f.WireType == WireBytes:
			event.Name = string(f.Data)
		}
	}
	if len(argsJSON) > 0 {
		merged, err := json.Marshal(argsJSON)
		if err == nil {
			event.RawArgs = string(merged)
		}
	}
	return event
}

func parseProtoMapEntry(payload []byte) (string, []byte) {
	var key string
	var value []byte
	pr := NewProtobufReader(payload)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		switch {
		case f.Num == 1 && f.WireType == WireBytes:
			key = string(f.Data)
		case f.Num == 2 && f.WireType == WireBytes:
			value = append([]byte(nil), f.Data...)
		}
	}
	return key, value
}

// --- google.protobuf.Value codec ----------------------------------------

// proto Value oneof fields.
const (
	protoValueNull   = 1
	protoValueNumber = 2
	protoValueString = 3
	protoValueBool   = 4
	protoValueStruct = 5
	protoValueList   = 6
)

// EncodeProtoJSONValue renders raw JSON bytes as google.protobuf.Value wire
// bytes, matching the encoding Cursor's McpToolDefinition.input_schema and
// McpArgs.args expect.
func EncodeProtoJSONValue(raw json.RawMessage) ([]byte, error) {
	var decoded any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("cursor: invalid tool schema JSON: %w", err)
	}
	var w ProtobufWriter
	writeProtoJSONValue(&w, decoded)
	return w.Result(), nil
}

func writeProtoJSONValue(w *ProtobufWriter, value any) {
	switch v := value.(type) {
	case nil:
		w.Varint(protoValueNull, 0)
	case bool:
		if v {
			w.Varint(protoValueBool, 1)
		} else {
			w.Varint(protoValueBool, 0)
		}
	case json.Number:
		if f, err := v.Float64(); err == nil {
			w.appendTag(protoValueNumber, WireFixed64)
			w.appendDouble(f)
		}
	case float64:
		w.appendTag(protoValueNumber, WireFixed64)
		w.appendDouble(v)
	case string:
		w.String(protoValueString, v)
	case []any:
		var list ProtobufWriter
		for _, item := range v {
			var itemWriter ProtobufWriter
			writeProtoJSONValue(&itemWriter, item)
			list.Bytes(protoValueList, itemWriter.Result())
		}
		w.Bytes(protoValueList, list.Result())
	case map[string]any:
		var structWriter ProtobufWriter
		for key, item := range v {
			var entry ProtobufWriter
			entry.String(1, key)
			var itemWriter ProtobufWriter
			writeProtoJSONValue(&itemWriter, item)
			entry.Bytes(2, itemWriter.Result())
			structWriter.Bytes(protoValueStruct, entry.Result())
		}
		w.Bytes(protoValueStruct, structWriter.Result())
	default:
		w.Varint(protoValueNull, 0)
	}
}

// DecodeProtoJSONValue converts google.protobuf.Value wire bytes back to raw
// JSON. Input with no decodable Value field degrades to a JSON string of the
// raw bytes so argument data is never silently dropped.
func DecodeProtoJSONValue(data []byte) json.RawMessage {
	value, ok := readProtoJSONValue(NewProtobufReader(data))
	if !ok {
		return marshalJSONStringOrEmpty(string(data))
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return marshalJSONStringOrEmpty(string(data))
	}
	return encoded
}

func marshalJSONStringOrEmpty(s string) json.RawMessage {
	encoded, err := json.Marshal(s)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return encoded
}

func readProtoJSONValue(pr *ProtobufReader) (any, bool) {
	var value any
	decoded := false
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			return value, decoded
		}
		switch {
		case f.Num == protoValueNull && f.WireType == WireVarint:
			value, decoded = nil, true
		case f.Num == protoValueNumber && f.WireType == WireFixed64:
			value, decoded = f.Double, true
		case f.Num == protoValueString && f.WireType == WireBytes:
			value, decoded = string(f.Data), true
		case f.Num == protoValueBool && f.WireType == WireVarint:
			value, decoded = f.Varint != 0, true
		case f.Num == protoValueStruct && f.WireType == WireBytes:
			obj := make(map[string]any)
			inner := NewProtobufReader(f.Data)
			for {
				entry, err := inner.Next()
				if entry == nil || err != nil {
					break
				}
				if entry.Num != protoValueStruct || entry.WireType != WireBytes {
					continue
				}
				key, raw := parseProtoMapEntry(entry.Data)
				if key == "" {
					continue
				}
				if inner, ok := readProtoJSONValue(NewProtobufReader(raw)); ok {
					obj[key] = inner
				}
			}
			value, decoded = obj, true
		case f.Num == protoValueList && f.WireType == WireBytes:
			var list []any
			inner := NewProtobufReader(f.Data)
			for {
				item, err := inner.Next()
				if item == nil || err != nil {
					break
				}
				if item.Num != protoValueList || item.WireType != WireBytes {
					continue
				}
				if decodedItem, ok := readProtoJSONValue(NewProtobufReader(item.Data)); ok {
					list = append(list, decodedItem)
				}
			}
			value, decoded = list, true
		}
	}
}

// ExecClientControlMessage / ExecClientThrow.
const (
	fieldAgentClientExecControl = 5
	fieldExecControlThrow       = 2
	fieldExecThrowID            = 1
	fieldExecThrowError         = 2
	fieldExecThrowErrorCode     = 4
)

// EncodeExecThrowReply errors out an exec frame the gateway cannot serve at
// all. Replying with a throw releases the server-side turn instead of leaving
// it stranded on a frame no one will answer.
func EncodeExecThrowReply(id uint64, message, code string) []byte {
	var throw ProtobufWriter
	throw.Varint(fieldExecThrowID, int(id))
	throw.String(fieldExecThrowError, message)
	if code != "" {
		throw.String(fieldExecThrowErrorCode, code)
	}
	var control ProtobufWriter
	control.Bytes(fieldExecControlThrow, throw.Result())
	return encodeAgentClientMessage(fieldAgentClientExecControl, control.Result())
}

// findAgentServerExec extracts AgentServerMessage.exec_server_message (field 2).
func findAgentServerExec(payload []byte) []byte {
	return GetNested(payload, fieldAgentServerExec)
}

// findAgentServerQuery extracts AgentServerMessage.interaction_query (field 7).
func findAgentServerQuery(payload []byte) (uint64, uint32, bool) {
	raw := GetNested(payload, fieldAgentServerQuery)
	if raw == nil {
		return 0, 0, false
	}
	var (
		id      uint64
		queryID uint32
	)
	pr := NewProtobufReader(raw)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		switch {
		case f.Num == fieldInteractionRespID && f.WireType == WireVarint:
			id = f.Varint
		case f.Num >= fieldInteractionQueryWebSearch && f.WireType == WireBytes:
			queryID = f.Num
		}
	}
	return id, queryID, true
}

// parseMcpStateServerIDs extracts McpStateExecArgs.server_identifiers.
func parseMcpStateServerIDs(payload []byte) []string {
	var ids []string
	pr := NewProtobufReader(payload)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		if f.Num == 1 && f.WireType == WireBytes && len(f.Data) > 0 {
			ids = append(ids, string(f.Data))
		}
	}
	return ids
}

// EncodeMcpStateReply answers ExecServerMessage.mcp_state_exec_args — the
// agent's tool discovery probe. The declared tool table is listed under the
// pi-agent server; when the probe is scoped to other servers the honest
// answer is an empty catalog.
func EncodeMcpStateReply(id uint64, execGUID string, tools []AgentTool, requestedServerIDs []string) []byte {
	var servers []byte
	if len(requestedServerIDs) == 0 || containsFoldString(requestedServerIDs, AgentToolProviderIdentifier) {
		var list ProtobufWriter
		for _, tool := range tools {
			schema, err := EncodeProtoJSONValue(orEmptySchema(tool.InputSchema))
			if err != nil {
				schema, _ = EncodeProtoJSONValue(orEmptySchema(nil))
			}
			var def ProtobufWriter
			def.String(fieldToolDefName, tool.Name)
			def.String(fieldToolDefDescription, tool.Description)
			def.Bytes(fieldToolDefInputSchema, schema)
			def.String(fieldToolDefProviderIdentifer, AgentToolProviderIdentifier)
			def.String(fieldToolDefToolName, tool.Name)
			list.Bytes(fieldMcpToolsDefinitions, def.Result())
		}
		var server ProtobufWriter
		server.String(fieldMcpStateServerName, AgentToolProviderIdentifier)
		server.String(fieldMcpStateServerIdent, AgentToolProviderIdentifier)
		server.Bytes(fieldMcpStateServerTools, list.Result())
		server.String(fieldMcpStateServerStatus, mcpStateServerConnected)
		var serversMsg ProtobufWriter
		serversMsg.Bytes(fieldMcpStateServers, server.Result())
		servers = serversMsg.Result()
	}
	var success ProtobufWriter
	success.Bytes(fieldMcpStateServers, servers)
	var result ProtobufWriter
	result.Bytes(fieldMcpStateResultSuccess, success.Result())
	return encodeExecReply(id, execGUID, fieldExecMcpSta, result.Result())
}

func orEmptySchema(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

func containsFoldString(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}

// parseToolCallUpdate decodes InteractionUpdate.tool_call_started /
// tool_call_completed into a tool call event when the wrapped ToolCall is an
// MCP call. Live streams deliver caller-executed tool calls this way.
func parseToolCallUpdate(data []byte) *ToolCallEvent {
	callID := GetString(data, fieldToolCallUpdateCallID)
	toolCall := GetNested(data, fieldToolCallUpdateToolCall)
	if toolCall == nil {
		return nil
	}
	mcpCall := GetNested(toolCall, fieldToolCallMcp)
	if mcpCall == nil {
		// Native (shell/read/...) calls are not caller-executable; skip.
		return nil
	}
	args := GetNested(mcpCall, fieldMcpCallArgs)
	if args == nil {
		return nil
	}
	event := parseMcpArgs(args)
	if callID != "" && event.ID == "" {
		event.ID = callID
	}
	return &event
}
