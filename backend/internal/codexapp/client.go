package codexapp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

type rpcClient struct {
	stdin         io.WriteCloser
	maxMessageLen int

	nextID  atomic.Int64
	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[int64]chan rpcResponse
	err     error

	notifications chan rpcNotification
	done          chan struct{}
	finishOnce    sync.Once
}

func newRPCClient(stdout io.Reader, stdin io.WriteCloser, maxMessageLen int) *rpcClient {
	if maxMessageLen <= 0 {
		maxMessageLen = defaultMaxMessageBytes
	}
	c := &rpcClient{
		stdin:         stdin,
		maxMessageLen: maxMessageLen,
		pending:       make(map[int64]chan rpcResponse),
		notifications: make(chan rpcNotification, 128),
		done:          make(chan struct{}),
	}
	go c.readLoop(stdout)
	return c
}

func (c *rpcClient) Call(ctx context.Context, method string, params, result any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	id := c.nextID.Add(1)
	responseCh := make(chan rpcResponse, 1)

	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return err
	}
	c.pending[id] = responseCh
	c.mu.Unlock()

	request := struct {
		ID     int64  `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params,omitempty"`
	}{ID: id, Method: method, Params: params}
	if err := c.writeJSON(request); err != nil {
		c.removePending(id)
		return fmt.Errorf("write codex app-server %s request: %w", method, err)
	}

	select {
	case response := <-responseCh:
		if response.Error != nil {
			return &RPCError{
				Method:  method,
				Code:    response.Error.Code,
				Message: response.Error.Message,
				Data:    response.Error.Data,
			}
		}
		if result == nil || len(response.Result) == 0 || string(response.Result) == "null" {
			return nil
		}
		if err := json.Unmarshal(response.Result, result); err != nil {
			return fmt.Errorf("decode codex app-server %s response: %w", method, err)
		}
		return nil
	case <-ctx.Done():
		c.removePending(id)
		return ctx.Err()
	case <-c.done:
		c.removePending(id)
		return c.terminalError()
	}
}

func (c *rpcClient) Notify(method string, params any) error {
	request := struct {
		Method string `json:"method"`
		Params any    `json:"params,omitempty"`
	}{Method: method, Params: params}
	if err := c.writeJSON(request); err != nil {
		return fmt.Errorf("write codex app-server %s notification: %w", method, err)
	}
	return nil
}

func (c *rpcClient) Notifications() <-chan rpcNotification {
	return c.notifications
}

func (c *rpcClient) Done() <-chan struct{} {
	return c.done
}

func (c *rpcClient) CloseInput() error {
	if c.stdin == nil {
		return nil
	}
	return c.stdin.Close()
}

func (c *rpcClient) readLoop(stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), c.maxMessageLen)
	for scanner.Scan() {
		var envelope struct {
			ID     json.RawMessage  `json:"id"`
			Method string           `json:"method"`
			Params json.RawMessage  `json:"params"`
			Result json.RawMessage  `json:"result"`
			Error  *rpcErrorPayload `json:"error"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &envelope); err != nil {
			c.finish(fmt.Errorf("decode codex app-server message: %w", err))
			return
		}

		if len(envelope.ID) > 0 && string(envelope.ID) != "null" {
			var id int64
			if err := json.Unmarshal(envelope.ID, &id); err != nil {
				c.finish(fmt.Errorf("decode codex app-server response id: %w", err))
				return
			}
			c.deliverResponse(id, rpcResponse{Result: envelope.Result, Error: envelope.Error})
			continue
		}
		if envelope.Method == "" {
			c.finish(errors.New("codex app-server message has neither id nor method"))
			return
		}

		select {
		case c.notifications <- rpcNotification{Method: envelope.Method, Params: envelope.Params}:
		case <-c.done:
			return
		}
	}
	if err := scanner.Err(); err != nil {
		c.finish(fmt.Errorf("read codex app-server stdout: %w", err))
		return
	}
	c.finish(io.EOF)
}

func (c *rpcClient) writeJSON(value any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.mu.Lock()
	err := c.err
	c.mu.Unlock()
	if err != nil {
		return err
	}
	return json.NewEncoder(c.stdin).Encode(value)
}

func (c *rpcClient) deliverResponse(id int64, response rpcResponse) {
	c.mu.Lock()
	ch := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()
	if ch != nil {
		ch <- response
	}
}

func (c *rpcClient) removePending(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *rpcClient) finish(err error) {
	c.finishOnce.Do(func() {
		if err == nil {
			err = io.EOF
		}
		c.mu.Lock()
		c.err = err
		c.pending = make(map[int64]chan rpcResponse)
		c.mu.Unlock()
		close(c.done)
		close(c.notifications)
	})
}

func (c *rpcClient) terminalError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil {
		return io.EOF
	}
	return c.err
}
