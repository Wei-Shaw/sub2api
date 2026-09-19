package cursor

import (
	"os"
	"strings"

	"github.com/google/uuid"
)

// Official agent.v1 field numbers from Cursor 3.16.
const (
	fieldAgentClientRunRequest  = 1
	fieldAgentClientKV          = 3
	fieldAgentClientHeartbeat   = 7
	fieldAgentServerInteraction = 1
	fieldAgentServerExec        = 2
	fieldAgentServerQuery       = 7
	fieldAgentServerKV          = 4

	fieldRunConversationState = 1
	fieldRunAction            = 2
	fieldRunModelDetails      = 3
	fieldRunMcpTools          = 4
	fieldRunConversationID    = 5
	fieldRunRequestedModel    = 9
	fieldRunCustomSystem      = 8
	fieldRunID                = 25

	fieldConvStateMode = 10

	fieldActionUserMessage = 1

	fieldUserMsgActionMessage = 1
	fieldUserMsgActionContext = 2
	fieldUserMsgActionPrepend = 4

	fieldUserMsgText = 1
	fieldUserMsgID   = 2
	fieldUserMsgMode = 4

	fieldReqCtxEnv = 4

	fieldNALEnvOSVersion = 1
	fieldNALEnvShell     = 3
	fieldNALEnvTimezone  = 10

	fieldModelID = 1

	fieldInteractionTextDelta     = 1
	fieldInteractionThinkingDelta = 4
	fieldInteractionTokenDelta    = 8
	fieldInteractionHeartbeat     = 13
	fieldInteractionTurnEnded     = 14

	fieldTextDeltaText         = 1
	fieldTextDeltaServerNotice = 2
	fieldThinkingDeltaText     = 1
	fieldTokenDeltaTokens      = 1

	fieldTurnEndedInputTokens      = 1
	fieldTurnEndedOutputTokens     = 2
	fieldTurnEndedCacheReadTokens  = 3
	fieldTurnEndedCacheWriteTokens = 4
	fieldTurnEndedReasoningTokens  = 5

	fieldKVId          = 1
	fieldKVGetBlobArgs = 2
	fieldKVSetBlobArgs = 3
	fieldKVGetResult   = 2
	fieldKVSetResult   = 3

	fieldBlobID       = 1
	fieldBlobData     = 1 // GetBlobResult.blob_data; SetBlobArgs uses 2
	fieldSetBlobID    = 1
	fieldSetBlobData  = 2
	fieldBlobError    = 2
	fieldErrorMessage = 1

	// AgentMode ASK is the chat-without-tools path.
	AgentModeUnspecified = 0
	AgentModeAgent       = 1
	AgentModeAsk         = 2
)

// AgentRunRequest describes one AgentService/Run invocation.
type AgentRunRequest struct {
	// Model is the resolved AgentService/Run slug.
	Model string
	// Messages carries the Ask-mode input (flattened by splitAskMessages).
	// In Agent mode it should hold only the system prompt (optional) and the
	// final user message; earlier history belongs in Turns.
	Messages []ChatMessage
	// Tools enables Agent mode: when non-empty the run declares the tool table
	// (AgentRunRequest.mcp_tools), switches the conversation mode to Agent,
	// and streams exec frames are answered inline (see execResponder).
	Tools []AgentTool
	// Turns is the replayed conversation history for Agent mode, one entry per
	// prior user turn with the assistant's steps attached.
	Turns []AgentTurn
}

// BuildAgentClientMessage encodes agent.v1.AgentClientMessage{run_request} for
// a flattened Ask-mode conversation.
func BuildAgentClientMessage(messages []ChatMessage, model string) (payload []byte, conversationID, runID string) {
	return buildAgentRunMessage(AgentRunRequest{Model: model, Messages: messages})
}

// buildAgentRunMessage encodes the run request for both conversation modes:
// Ask (flattened messages, no tools) and Agent (tool table + replayed turns).
func buildAgentRunMessage(req AgentRunRequest) (payload []byte, conversationID, runID string) {
	if req.Model == "" {
		req.Model = "default"
	}
	conversationID = uuid.New().String()
	runID = uuid.New().String()

	agentMode := len(req.Tools) > 0
	mode := AgentModeAsk
	if agentMode {
		mode = AgentModeAgent
	}
	systemPrompt, userText, prior := splitAskMessages(req.Messages)

	var state ProtobufWriter
	state.Varint(fieldConvStateMode, mode)
	if agentMode {
		for _, turn := range EncodeAgentTurns(req.Turns) {
			state.Bytes(fieldConvStateTurns, turn)
		}
	}

	var userMsg ProtobufWriter
	userMsg.String(fieldUserMsgText, userText)
	userMsg.String(fieldUserMsgID, uuid.New().String())
	userMsg.Varint(fieldUserMsgMode, mode)

	var env ProtobufWriter
	if rel := osRelease(); rel != "" {
		env.String(fieldNALEnvOSVersion, rel)
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		env.String(fieldNALEnvShell, sh)
	}
	env.String(fieldNALEnvTimezone, clientTimezone())

	var reqCtx ProtobufWriter
	reqCtx.Bytes(fieldReqCtxEnv, env.Result())

	var userAction ProtobufWriter
	userAction.Bytes(fieldUserMsgActionMessage, userMsg.Result())
	userAction.Bytes(fieldUserMsgActionContext, reqCtx.Result())
	if !agentMode {
		for _, p := range prior {
			var pre ProtobufWriter
			pre.String(fieldUserMsgText, p)
			pre.String(fieldUserMsgID, uuid.New().String())
			pre.Varint(fieldUserMsgMode, AgentModeAsk)
			userAction.Bytes(fieldUserMsgActionPrepend, pre.Result())
		}
	}

	var action ProtobufWriter
	action.Bytes(fieldActionUserMessage, userAction.Result())

	var modelDetails ProtobufWriter
	modelDetails.String(fieldModelID, req.Model)

	var requested ProtobufWriter
	requested.String(fieldModelID, req.Model)

	var run ProtobufWriter
	run.Bytes(fieldRunConversationState, state.Result())
	run.Bytes(fieldRunAction, action.Result())
	run.Bytes(fieldRunModelDetails, modelDetails.Result())
	run.Bytes(fieldRunMcpTools, agentToolsField(req))
	run.String(fieldRunConversationID, conversationID)
	if systemPrompt != "" {
		run.String(fieldRunCustomSystem, systemPrompt)
	}
	run.Bytes(fieldRunRequestedModel, requested.Result())
	run.String(fieldRunID, runID)

	var client ProtobufWriter
	client.Bytes(fieldAgentClientRunRequest, run.Result())
	return client.Result(), conversationID, runID
}

// agentToolsField renders the caller tool table for AgentRunRequest.mcp_tools.
func agentToolsField(req AgentRunRequest) []byte {
	if len(req.Tools) == 0 {
		return nil
	}
	return EncodeAgentTools(req.Tools)
}

func encodeClientHeartbeat() []byte {
	var w ProtobufWriter
	w.Bytes(fieldAgentClientHeartbeat, nil)
	return w.Result()
}

func splitAskMessages(messages []ChatMessage) (systemPrompt, userText string, prior []string) {
	var systems []string
	var rest []ChatMessage
	for _, m := range messages {
		if strings.EqualFold(m.Role, "system") {
			if strings.TrimSpace(m.Content) != "" {
				systems = append(systems, m.Content)
			}
			continue
		}
		rest = append(rest, m)
	}
	systemPrompt = strings.Join(systems, "\n\n")
	if len(rest) == 0 {
		return systemPrompt, "", nil
	}
	if len(rest) == 1 && strings.EqualFold(rest[0].Role, "user") {
		return systemPrompt, rest[0].Content, nil
	}

	last := rest[len(rest)-1]
	if strings.EqualFold(last.Role, "user") {
		for _, m := range rest[:len(rest)-1] {
			prior = append(prior, rolePrefix(m.Role)+m.Content)
		}
		return systemPrompt, last.Content, prior
	}

	var b strings.Builder
	for i, m := range rest {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(rolePrefix(m.Role))
		b.WriteString(m.Content)
	}
	return systemPrompt, b.String(), nil
}

func rolePrefix(role string) string {
	switch strings.ToLower(role) {
	case "assistant":
		return "Assistant: "
	case "user":
		return "User: "
	default:
		return role + ": "
	}
}

type kvServerOp struct {
	id      uint64
	getBlob []byte
	setBlob []byte
	setData []byte
}

func parseAgentKV(data []byte) *kvServerOp {
	kv := GetNested(data, fieldAgentServerKV)
	if kv == nil {
		return nil
	}
	op := &kvServerOp{}
	pr := NewProtobufReader(kv)
	found := false
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		switch f.Num {
		case fieldKVId:
			op.id = f.Varint
			found = true
		case fieldKVGetBlobArgs:
			op.getBlob = GetNested(f.Data, fieldBlobID)
			if op.getBlob == nil {
				op.getBlob = f.Data
			}
			found = true
		case fieldKVSetBlobArgs:
			op.setBlob = GetNested(f.Data, fieldSetBlobID)
			op.setData = GetNested(f.Data, fieldSetBlobData)
			found = true
		}
	}
	if !found {
		return nil
	}
	return op
}

func encodeKVGetResult(id uint64, blob []byte, errMsg string) []byte {
	var result ProtobufWriter
	if errMsg != "" {
		var e ProtobufWriter
		e.String(fieldErrorMessage, errMsg)
		result.Bytes(fieldBlobError, e.Result())
	} else {
		result.Bytes(fieldBlobData, blob)
	}
	var kv ProtobufWriter
	kv.Varint(fieldKVId, int(id))
	kv.Bytes(fieldKVGetResult, result.Result())
	var client ProtobufWriter
	client.Bytes(fieldAgentClientKV, kv.Result())
	return client.Result()
}

func encodeKVSetResult(id uint64) []byte {
	var kv ProtobufWriter
	kv.Varint(fieldKVId, int(id))
	kv.Bytes(fieldKVSetResult, nil)
	var client ProtobufWriter
	client.Bytes(fieldAgentClientKV, kv.Result())
	return client.Result()
}
