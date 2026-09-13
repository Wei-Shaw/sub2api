package cursor

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/connectrpc"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/encoding/protowire"
)

const (
	RunPath             = "/agent.v1.AgentService/Run"
	GetUsableModelsPath = "/agent.v1.AgentService/GetUsableModels"
)

var DefaultModelIDs = []string{
	"claude-4.6-opus-high",
	"claude-4.6-sonnet-medium",
	"gpt-5.3",
}

type AgentEvent struct {
	Kind      string
	Text      string
	CallID    string
	ToolName  string
	Arguments string
	Tokens    int
	Done      bool
}

func NewHTTP2Client(proxyURL string) (*http.Client, error) {
	transport := &http2.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}
	_, parsed, err := proxyurl.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	if parsed != nil {
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		base := &http.Transport{
			DialContext:         dialer.DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
			ForceAttemptHTTP2:   true,
		}
		if err := proxyutil.ConfigureTransportProxy(base, parsed); err != nil {
			return nil, err
		}
		return &http.Client{Timeout: 0, Transport: base}, nil
	}
	return &http.Client{Timeout: 0, Transport: transport}, nil
}

type AgentTool struct {
	Name        string
	Description string
	SchemaJSON  string
}

func EncodeRunRequest(modelID, conversationID, userText, systemPrompt string, tools []AgentTool) []byte {
	userMessage := connectrpc.AppendString(nil, 1, userText)
	userAction := connectrpc.AppendMessage(nil, 1, userMessage)
	action := connectrpc.AppendMessage(nil, 1, userAction)
	requested := connectrpc.AppendString(nil, 1, modelID)
	run := connectrpc.AppendMessage(nil, 2, action)
	if len(tools) > 0 {
		var mcp []byte
		for _, tool := range tools {
			def := connectrpc.AppendString(nil, 1, tool.Name)
			def = connectrpc.AppendString(def, 2, tool.Description)
			def = connectrpc.AppendString(def, 5, tool.Name)
			def = connectrpc.AppendString(def, 6, tool.SchemaJSON)
			mcp = connectrpc.AppendMessage(mcp, 1, def)
		}
		run = connectrpc.AppendMessage(run, 4, mcp)
	}
	run = connectrpc.AppendString(run, 5, conversationID)
	run = connectrpc.AppendString(run, 8, systemPrompt)
	run = connectrpc.AppendMessage(run, 9, requested)
	return connectrpc.AppendMessage(nil, 1, run)
}

func EncodeInteractionResponse(queryID uint64, queryField protowire.Number, approve bool, reason string) []byte {
	resp := connectrpc.AppendVarint(nil, 1, queryID)
	inner := connectrpc.AppendMessage(nil, 1, nil)
	if !approve {
		rejected := connectrpc.AppendString(nil, 1, reason)
		inner = connectrpc.AppendMessage(nil, 2, rejected)
	}
	resp = connectrpc.AppendMessage(resp, queryField, inner)
	return connectrpc.AppendMessage(nil, 6, resp)
}

func EncodeExecReject(execID uint64, message string) []byte {
	throw := connectrpc.AppendVarint(nil, 1, execID)
	throw = connectrpc.AppendString(throw, 2, message)
	control := connectrpc.AppendMessage(nil, 2, throw)
	return connectrpc.AppendMessage(nil, 5, control)
}

type ServerControl struct {
	Kind       string
	QueryID    uint64
	QueryField protowire.Number
	ExecID     uint64
}

func DecodeServerMessage(payload []byte) (events []AgentEvent, controls []ServerControl) {
	walk(payload, func(num protowire.Number, value []byte) {
		switch num {
		case 1:
			events = append(events, decodeInteractionUpdate(value)...)
		case 2:
			controls = append(controls, ServerControl{Kind: "exec", ExecID: firstVarint(value, 1)})
		case 7:
			queryField := firstNestedField(value)
			controls = append(controls, ServerControl{
				Kind:       "query",
				QueryID:    firstVarint(value, 1),
				QueryField: queryField,
			})
		}
	})
	return events, controls
}

func NetworkQuery(field protowire.Number) bool {
	switch field {
	case 2, 5, 6, 9:
		return true
	default:
		return false
	}
}

func DecodeServerEvents(payload []byte) []AgentEvent {
	events, _ := DecodeServerMessage(payload)
	return events
}

func decodeInteractionUpdate(value []byte) []AgentEvent {
	var events []AgentEvent
	walk(value, func(updateNum protowire.Number, update []byte) {
		switch updateNum {
		case 1:
			events = append(events, AgentEvent{Kind: "text", Text: firstString(update, 1)})
		case 4:
			events = append(events, AgentEvent{Kind: "thinking", Text: firstString(update, 1)})
		case 8:
			events = append(events, AgentEvent{Kind: "usage", Tokens: int(firstVarint(update, 1))})
		case 14:
			events = append(events, AgentEvent{Kind: "done", Done: true})
		case 2:
			events = append(events, AgentEvent{
				Kind:     "tool_start",
				CallID:   firstString(update, 1),
				ToolName: extractToolName(nestedBytes(update, 2)),
			})
		case 7, 15:
			events = append(events, AgentEvent{
				Kind:      "tool_delta",
				CallID:    firstString(update, 1),
				Arguments: firstString(update, 3),
			})
		case 3:
			events = append(events, AgentEvent{
				Kind:     "tool_end",
				CallID:   firstString(update, 1),
				ToolName: extractToolName(nestedBytes(update, 2)),
			})
		}
	})
	return events
}

func DecodeUsableModels(payload []byte) []string {
	var ids []string
	walk(payload, func(num protowire.Number, value []byte) {
		if num != 1 {
			return
		}
		if id := firstString(value, 1); id != "" {
			ids = append(ids, id)
		}
	})
	return ids
}

func PostConnect(ctx context.Context, client *http.Client, baseURL, path, accessToken, clientVersion string, body []byte) (*http.Response, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = APIBaseURL
	}
	if strings.TrimSpace(clientVersion) == "" {
		clientVersion = DefaultClientVersion
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/connect+proto")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	req.Header.Set("x-ghost-mode", "true")
	req.Header.Set("x-cursor-client-type", "cli")
	req.Header.Set("x-cursor-client-version", clientVersion)
	req.Header.Set("x-request-id", fmt.Sprintf("%d", time.Now().UnixNano()))
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return nil, fmt.Errorf("cursor %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	return resp, nil
}

func walk(buf []byte, fn func(num protowire.Number, value []byte)) {
	for len(buf) > 0 {
		num, typ, value, rest, ok := connectrpc.Consume(buf)
		if !ok {
			return
		}
		buf = rest
		if typ == protowire.BytesType {
			fn(num, value)
		}
	}
}

func firstString(buf []byte, field protowire.Number) string {
	for len(buf) > 0 {
		num, typ, value, rest, ok := connectrpc.Consume(buf)
		if !ok {
			return ""
		}
		buf = rest
		if num == field && typ == protowire.BytesType {
			return connectrpc.ConsumeString(value)
		}
	}
	return ""
}

func firstVarint(buf []byte, field protowire.Number) uint64 {
	for len(buf) > 0 {
		num, typ, value, rest, ok := connectrpc.Consume(buf)
		if !ok {
			return 0
		}
		buf = rest
		if num == field && typ == protowire.VarintType {
			return connectrpc.ConsumeVarint(value)
		}
	}
	return 0
}

func nestedBytes(buf []byte, field protowire.Number) []byte {
	for len(buf) > 0 {
		num, typ, value, rest, ok := connectrpc.Consume(buf)
		if !ok {
			return nil
		}
		buf = rest
		if num == field && typ == protowire.BytesType {
			return value
		}
	}
	return nil
}

func firstNestedField(buf []byte) protowire.Number {
	for len(buf) > 0 {
		num, typ, _, rest, ok := connectrpc.Consume(buf)
		if !ok {
			return 0
		}
		buf = rest
		if num >= 2 && typ == protowire.BytesType {
			return num
		}
	}
	return 0
}

func extractToolName(toolCall []byte) string {
	if mcp := nestedBytes(toolCall, 15); len(mcp) > 0 {
		args := nestedBytes(mcp, 1)
		if name := firstString(args, 5); name != "" {
			return name
		}
		return firstString(args, 1)
	}
	return firstString(toolCall, 1)
}
