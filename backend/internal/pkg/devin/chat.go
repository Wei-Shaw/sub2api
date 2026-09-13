package devin

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/connectrpc"
	"google.golang.org/protobuf/encoding/protowire"
)

const (
	ChatPath        = "/exa.api_server_pb.ApiServerService/GetChatMessage"
	AssignModelPath = "/exa.api_server_pb.ApiServerService/AssignModel"
	AuthPath        = "/exa.auth_pb.AuthService/GetUserJwt"
	SourceUser      = 1
	SourceSystem    = 2
	SourceTool      = 4
	RequestCascade  = 5
	PlannerDefault  = 1
)

var DefaultModelIDs = []string{"swe-1-6", "swe-1-6-fast"}

type ChatMessage struct {
	ID        string
	Source    int
	Prompt    string
	Thinking  string
	ToolID    string
	ToolError bool
}

type ChatEvent struct {
	Kind     string
	Text     string
	CallID   string
	ToolName string
	ArgsJSON string
	Input    int
	Output   int
	CacheR   int
	CacheW   int
	Stop     int
	ModelUID string
}

func EncodeMetadata(apiKey, userJWT string) []byte {
	osName := runtime.GOOS
	if osName != "darwin" && osName != "windows" {
		osName = "linux"
	}
	buf := connectrpc.AppendString(nil, 1, "devin-cli")
	buf = connectrpc.AppendString(buf, 2, "3000.6.2")
	buf = connectrpc.AppendString(buf, 3, NormalizeSessionToken(apiKey))
	buf = connectrpc.AppendString(buf, 4, "en")
	buf = connectrpc.AppendString(buf, 5, osName)
	buf = connectrpc.AppendString(buf, 7, "3000.6.2")
	buf = connectrpc.AppendString(buf, 12, "chisel")
	buf = connectrpc.AppendString(buf, 21, userJWT)
	buf = connectrpc.AppendString(buf, 28, "chisel")
	return buf
}

func EncodeGetUserJwt(apiKey string) []byte {
	return connectrpc.AppendMessage(nil, 1, EncodeMetadata(apiKey, ""))
}

func EncodeAssignModel(apiKey, routerUID, cascadeID, prompt string) []byte {
	buf := connectrpc.AppendMessage(nil, 1, EncodeMetadata(apiKey, ""))
	buf = connectrpc.AppendString(buf, 2, routerUID)
	buf = connectrpc.AppendString(buf, 3, cascadeID)
	if prompt != "" {
		user := connectrpc.AppendVarint(nil, 2, SourceUser)
		user = connectrpc.AppendString(user, 3, prompt)
		buf = connectrpc.AppendMessage(buf, 5, user)
	}
	return buf
}

func EncodeChatRequest(apiKey, userJWT, modelUID, assignmentJWT, cascadeID, systemPrompt string, messages []ChatMessage, tools [][]byte) []byte {
	buf := connectrpc.AppendMessage(nil, 1, EncodeMetadata(apiKey, userJWT))
	buf = connectrpc.AppendString(buf, 2, systemPrompt)
	for _, msg := range messages {
		item := connectrpc.AppendString(nil, 1, msg.ID)
		item = connectrpc.AppendVarint(item, 2, uint64(msg.Source))
		item = connectrpc.AppendString(item, 3, msg.Prompt)
		item = connectrpc.AppendString(item, 7, msg.ToolID)
		item = connectrpc.AppendBool(item, 9, msg.ToolError)
		item = connectrpc.AppendString(item, 11, msg.Thinking)
		buf = connectrpc.AppendMessage(buf, 3, item)
	}
	buf = connectrpc.AppendVarint(buf, 7, RequestCascade)
	for _, tool := range tools {
		buf = connectrpc.AppendMessage(buf, 10, tool)
	}
	buf = connectrpc.AppendMessage(buf, 12, connectrpc.AppendString(nil, 1, "auto"))
	buf = connectrpc.AppendMessage(buf, 13, connectrpc.AppendVarint(nil, 1, 1))
	buf = connectrpc.AppendString(buf, 16, cascadeID)
	buf = connectrpc.AppendVarint(buf, 20, PlannerDefault)
	buf = connectrpc.AppendString(buf, 21, modelUID)
	buf = connectrpc.AppendString(buf, 26, assignmentJWT)
	return buf
}

func EncodeTool(name, description, schema string) []byte {
	buf := connectrpc.AppendString(nil, 1, name)
	buf = connectrpc.AppendString(buf, 2, description)
	return connectrpc.AppendString(buf, 3, schema)
}

func DecodeUserJwt(payload []byte) (jwt, customBase string) {
	for len(payload) > 0 {
		num, typ, value, rest, ok := connectrpc.Consume(payload)
		if !ok {
			return jwt, customBase
		}
		payload = rest
		if typ != protowire.BytesType {
			continue
		}
		switch num {
		case 1:
			jwt = connectrpc.ConsumeString(value)
		case 2:
			customBase = strings.TrimRight(connectrpc.ConsumeString(value), "/")
		}
	}
	return jwt, customBase
}

func DecodeAssignment(payload []byte) (modelUID, assignmentJWT string) {
	for len(payload) > 0 {
		num, typ, value, rest, ok := connectrpc.Consume(payload)
		if !ok {
			return modelUID, assignmentJWT
		}
		payload = rest
		if num != 1 || typ != protowire.BytesType {
			continue
		}
		inner := value
		for len(inner) > 0 {
			inNum, inTyp, inVal, inRest, inOK := connectrpc.Consume(inner)
			if !inOK {
				break
			}
			inner = inRest
			if inTyp != protowire.BytesType {
				continue
			}
			switch inNum {
			case 1:
				assignmentJWT = connectrpc.ConsumeString(inVal)
			case 2:
				modelUID = connectrpc.ConsumeString(inVal)
			}
		}
	}
	return modelUID, assignmentJWT
}

func DecodeChatEvent(payload []byte) ChatEvent {
	var ev ChatEvent
	for len(payload) > 0 {
		num, typ, value, rest, ok := connectrpc.Consume(payload)
		if !ok {
			return ev
		}
		payload = rest
		switch {
		case num == 3 && typ == protowire.BytesType:
			ev.Kind = "text"
			ev.Text = connectrpc.ConsumeString(value)
		case num == 5 && typ == protowire.VarintType:
			ev.Stop = int(connectrpc.ConsumeVarint(value))
		case num == 6 && typ == protowire.BytesType:
			ev.Kind = "tool"
			ev.CallID = firstString(value, 1)
			ev.ToolName = firstString(value, 2)
			ev.ArgsJSON = firstString(value, 3)
		case num == 7 && typ == protowire.BytesType:
			ev.Input = int(firstVarint(value, 2))
			ev.Output = int(firstVarint(value, 3))
			ev.CacheW = int(firstVarint(value, 4))
			ev.CacheR = int(firstVarint(value, 5))
		case num == 9 && typ == protowire.BytesType:
			ev.Kind = "thinking"
			ev.Text = connectrpc.ConsumeString(value)
		case num == 23 && typ == protowire.BytesType:
			ev.ModelUID = connectrpc.ConsumeString(value)
		}
	}
	return ev
}

func PostUnary(ctx context.Context, client *http.Client, baseURL, path string, body []byte) ([]byte, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = ChatBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/proto")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("Accept", "*/*")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, MaxRead))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("devin %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	if frame, _, err := connectrpc.Decode(raw); err == nil && len(frame.Payload) > 0 {
		return frame.Payload, nil
	}
	return raw, nil
}

func PostStream(ctx context.Context, client *http.Client, baseURL, path string, body []byte) (*http.Response, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = ChatBaseURL
	}
	framed, err := connectrpc.EncodeGzip(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+path, bytes.NewReader(framed))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/connect+proto")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("Connect-Content-Encoding", "gzip")
	req.Header.Set("Connect-Accept-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", "connect-go/1.18.1")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return nil, fmt.Errorf("devin %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	return resp, nil
}

const MaxRead = 8 << 20

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

func NewHTTPClient(proxyURL string) (*http.Client, error) {
	return NewStreamClient(proxyURL)
}

func NewStreamClient(proxyURL string) (*http.Client, error) {
	client, err := NewClient(proxyURL)
	if err != nil {
		return nil, err
	}
	stream := *client.httpClient
	stream.Timeout = 0
	return &stream, nil
}

func ValidateSession(ctx context.Context, client *http.Client, sessionToken string) (string, error) {
	payload, err := PostUnary(ctx, client, ChatBaseURL, AuthPath, EncodeGetUserJwt(sessionToken))
	if err != nil {
		return "", err
	}
	jwt, _ := DecodeUserJwt(payload)
	if jwt == "" {
		return "", fmt.Errorf("devin GetUserJwt returned an empty token")
	}
	return jwt, nil
}
