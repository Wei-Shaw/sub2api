package codexapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Provider struct {
	proc                     *process
	interruptTimeoutDuration time.Duration
	runMu                    sync.Mutex
}

func Start(ctx context.Context, cfg Config) (*Provider, error) {
	proc, err := startProcess(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &Provider{
		proc:                     proc,
		interruptTimeoutDuration: interruptTimeout(cfg),
	}, nil
}

func (p *Provider) Close(ctx context.Context) error {
	if p == nil {
		return nil
	}
	return p.proc.close(ctx)
}

func (p *Provider) RunTurn(ctx context.Context, request TurnRequest, sink EventSink) (TurnResult, error) {
	if p == nil || p.proc == nil {
		return TurnResult{}, errors.New("codex app-server provider is not started")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if len(request.Input) == 0 {
		return TurnResult{}, errors.New("codex app-server turn input is required")
	}

	p.runMu.Lock()
	defer p.runMu.Unlock()

	threadID := request.ThreadID
	if threadID == "" {
		var threadResult struct {
			Thread struct {
				ID string `json:"id"`
			} `json:"thread"`
		}
		if err := p.proc.rpc.Call(ctx, "thread/start", map[string]any{
			"model":     request.Model,
			"ephemeral": request.Ephemeral,
		}, &threadResult); err != nil {
			return TurnResult{}, err
		}
		threadID = threadResult.Thread.ID
		if threadID == "" {
			return TurnResult{}, errors.New("codex app-server thread/start returned an empty thread id")
		}
	}

	var turnResult struct {
		Turn struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"turn"`
	}
	if err := p.proc.rpc.Call(ctx, "turn/start", map[string]any{
		"threadId": threadID,
		"input":    request.Input,
	}, &turnResult); err != nil {
		return TurnResult{}, err
	}
	turnID := turnResult.Turn.ID
	if turnID == "" {
		return TurnResult{}, errors.New("codex app-server turn/start returned an empty turn id")
	}

	for {
		select {
		case notification, ok := <-p.proc.rpc.Notifications():
			if !ok {
				return TurnResult{}, p.proc.terminalError()
			}
			event, matched, err := decodeTurnEvent(notification, threadID, turnID)
			if err != nil {
				return TurnResult{}, err
			}
			if !matched {
				continue
			}
			if sink != nil {
				if err := sink(event); err != nil {
					p.interrupt(threadID, turnID)
					return TurnResult{}, fmt.Errorf("emit codex app-server event: %w", err)
				}
			}
			if event.Type == EventTurnCompleted {
				return TurnResult{ThreadID: threadID, TurnID: turnID, Status: event.Status}, nil
			}
		case <-ctx.Done():
			p.interrupt(threadID, turnID)
			return TurnResult{}, ctx.Err()
		case <-p.proc.rpc.Done():
			return TurnResult{}, p.proc.terminalError()
		}
	}
}

func (p *Provider) interrupt(threadID, turnID string) {
	ctx, cancel := context.WithTimeout(context.Background(), p.interruptTimeoutDuration)
	defer cancel()
	_ = p.proc.rpc.Call(ctx, "turn/interrupt", map[string]any{
		"threadId": threadID,
		"turnId":   turnID,
	}, nil)
}

func decodeTurnEvent(notification rpcNotification, threadID, turnID string) (Event, bool, error) {
	switch notification.Method {
	case EventAgentMessageDelta:
		var params struct {
			ThreadID string `json:"threadId"`
			TurnID   string `json:"turnId"`
			Delta    string `json:"delta"`
		}
		if err := json.Unmarshal(notification.Params, &params); err != nil {
			return Event{}, false, fmt.Errorf("decode %s notification: %w", notification.Method, err)
		}
		if params.ThreadID != threadID || params.TurnID != turnID {
			return Event{}, false, nil
		}
		return Event{Type: notification.Method, ThreadID: params.ThreadID, TurnID: params.TurnID, Delta: params.Delta}, true, nil
	case EventTurnCompleted:
		var params struct {
			ThreadID string `json:"threadId"`
			Turn     struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"turn"`
		}
		if err := json.Unmarshal(notification.Params, &params); err != nil {
			return Event{}, false, fmt.Errorf("decode %s notification: %w", notification.Method, err)
		}
		if params.ThreadID != threadID || params.Turn.ID != turnID {
			return Event{}, false, nil
		}
		return Event{Type: notification.Method, ThreadID: params.ThreadID, TurnID: params.Turn.ID, Status: params.Turn.Status}, true, nil
	default:
		return Event{}, false, nil
	}
}
