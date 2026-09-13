package cursor

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/connectrpc"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
)

func TestEncodeRunRequestIncludesModelAndUserText(t *testing.T) {
	body := EncodeRunRequest("claude-4.6-opus-high", "conv-1", "hello", "be brief", []AgentTool{{
		Name:        "lookup",
		Description: "look something up",
		SchemaJSON:  `{"type":"object"}`,
	}})
	require.NotEmpty(t, body)
	require.Contains(t, string(body), "claude-4.6-opus-high")
	require.Contains(t, string(body), "hello")
	require.Contains(t, string(body), "lookup")
}

func TestDecodeServerMessageTextThinkingUsageAndDone(t *testing.T) {
	text := connectrpc.AppendMessage(nil, 1, connectrpc.AppendString(nil, 1, "hello"))
	thinking := connectrpc.AppendMessage(nil, 4, connectrpc.AppendString(nil, 1, "hmm"))
	usage := connectrpc.AppendMessage(nil, 8, connectrpc.AppendVarint(nil, 1, 7))
	done := connectrpc.AppendMessage(nil, 14, connectrpc.AppendVarint(nil, 1, 1))
	update := append(append(append(text, thinking...), usage...), done...)
	payload := connectrpc.AppendMessage(nil, 1, update)

	events, controls := DecodeServerMessage(payload)
	require.Empty(t, controls)
	require.Equal(t, []AgentEvent{
		{Kind: "text", Text: "hello"},
		{Kind: "thinking", Text: "hmm"},
		{Kind: "usage", Tokens: 7},
		{Kind: "done", Done: true},
	}, events)
}

func TestDecodeServerMessageQueryAndExecControls(t *testing.T) {
	query := connectrpc.AppendVarint(nil, 1, 42)
	query = connectrpc.AppendMessage(query, 2, connectrpc.AppendString(nil, 1, "https://example.com"))
	exec := connectrpc.AppendVarint(nil, 1, 9)
	payload := append(connectrpc.AppendMessage(nil, 7, query), connectrpc.AppendMessage(nil, 2, exec)...)

	events, controls := DecodeServerMessage(payload)
	require.Empty(t, events)
	require.Equal(t, []ServerControl{
		{Kind: "query", QueryID: 42, QueryField: protowire.Number(2)},
		{Kind: "exec", ExecID: 9},
	}, controls)
	require.True(t, NetworkQuery(2))
	require.False(t, NetworkQuery(1))
}

func TestEncodeInteractionAndExecReplies(t *testing.T) {
	approve := EncodeInteractionResponse(3, 2, true, "")
	reject := EncodeInteractionResponse(3, 7, false, "interactive queries are not supported by this gateway")
	exec := EncodeExecReject(4, "native tools are not executed by this gateway")
	require.NotEmpty(t, approve)
	require.NotEmpty(t, reject)
	require.NotEmpty(t, exec)
	require.Contains(t, string(reject), "interactive queries")
	require.Contains(t, string(exec), "native tools")
}
