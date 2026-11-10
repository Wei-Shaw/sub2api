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

// Healthy reports whether the child process is still available for another
// turn. Pool users discard a worker after the app-server process exits.
func (p *Provider) Healthy() bool {
	if p == nil || p.proc == nil || p.proc.rpc == nil {
		return false
	}
	select {
	case <-p.proc.rpc.Done():
		return false
	default:
		return true
	}
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

	var usage TokenUsage
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
			if event.Type == EventThreadTokenUsage {
				usage = usage.add(event.Usage)
			}
			if event.Type == EventTurnCompleted {
				return TurnResult{ThreadID: threadID, TurnID: turnID, Status: event.Status, Usage: usage}, nil
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
	case EventThreadTokenUsage:
		var params struct {
			ThreadID   string `json:"threadId"`
			TurnID     string `json:"turnId"`
			TokenUsage struct {
				Last struct {
					InputTokens           int `json:"inputTokens"`
					CachedInputTokens     int `json:"cachedInputTokens"`
					OutputTokens          int `json:"outputTokens"`
					ReasoningOutputTokens int `json:"reasoningOutputTokens"`
					TotalTokens           int `json:"totalTokens"`
				} `json:"last"`
			} `json:"tokenUsage"`
		}
		if err := json.Unmarshal(notification.Params, &params); err != nil {
			return Event{}, false, fmt.Errorf("decode %s notification: %w", notification.Method, err)
		}
		if params.ThreadID != threadID || params.TurnID != turnID {
			return Event{}, false, nil
		}
		return Event{
			Type: notification.Method, ThreadID: params.ThreadID, TurnID: params.TurnID,
			Usage: TokenUsage{
				InputTokens:           params.TokenUsage.Last.InputTokens,
				CachedInputTokens:     params.TokenUsage.Last.CachedInputTokens,
				OutputTokens:          params.TokenUsage.Last.OutputTokens,
				ReasoningOutputTokens: params.TokenUsage.Last.ReasoningOutputTokens,
				TotalTokens:           params.TokenUsage.Last.TotalTokens,
			},
		}, true, nil
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
