package codexapp

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

const (
	EventAgentMessageDelta = "item/agentMessage/delta"
	EventTurnCompleted     = "turn/completed"
)

const (
	defaultStartTimeout     = 10 * time.Second
	defaultInterruptTimeout = 3 * time.Second
	defaultMaxMessageBytes  = 8 << 20
)

type ClientInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version,omitempty"`
}

type Config struct {
	Command          []string
	Env              []string
	ClientInfo       ClientInfo
	StartTimeout     time.Duration
	InterruptTimeout time.Duration
	MaxMessageBytes  int
	Stderr           io.Writer
}

type InputItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type TurnRequest struct {
	ThreadID  string
	Model     string
	Input     []InputItem
	Ephemeral bool
}

type Event struct {
	Type     string
	ThreadID string
	TurnID   string
	Delta    string
	Status   string
}

type TurnResult struct {
	ThreadID string
	TurnID   string
	Status   string
}

type EventSink func(Event) error

type RPCError struct {
	Method  string
	Code    int
	Message string
	Data    json.RawMessage
}

func (e *RPCError) Error() string {
	if e == nil {
		return "codex app-server RPC error"
	}
	return fmt.Sprintf("codex app-server %s failed (%d): %s", e.Method, e.Code, e.Message)
}

type rpcErrorPayload struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type rpcResponse struct {
	Result json.RawMessage
	Error  *rpcErrorPayload
}

type rpcNotification struct {
	Method string
	Params json.RawMessage
}
