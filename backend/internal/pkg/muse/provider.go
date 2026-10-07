package muse

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

type Capabilities struct {
	Models        []string `json:"models"`
	Streaming     bool     `json:"streaming"`
	Instructions  bool     `json:"instructions"`
	Tools         bool     `json:"tools"`
	Vision        bool     `json:"vision"`
	Reasoning     bool     `json:"reasoning"`
	Controls      bool     `json:"controls"`
	Continuations bool     `json:"continuations"`
}

type Session struct {
	AccountID        int64
	OwnerUserID      int64
	AccountUpdatedAt time.Time
	Document         map[string]any
	ProxyURL         string
}

type Observation struct {
	ProxyUpdatedAt *time.Time `json:"proxy_updated_at,omitempty"`
	// This must be observed from authenticated entitlement/model access, never inferred from a plan name.
	InferenceAllowed bool              `json:"inference_allowed"`
	Identity         Identity          `json:"identity"`
	Capabilities     Capabilities      `json:"capabilities"`
	Usage            *UsageObservation `json:"usage,omitempty"`
	SessionExpiresAt *time.Time        `json:"session_expires_at,omitempty"`
}

type UsageObservation struct {
	Plan        string        `json:"plan,omitempty"`
	Entitlement string        `json:"entitlement,omitempty"`
	ObservedAt  time.Time     `json:"observed_at"`
	Windows     []UsageWindow `json:"windows,omitempty"`
}
type UsageWindow struct {
	Name     string     `json:"name"`
	Used     *float64   `json:"used,omitempty"`
	Limit    *float64   `json:"limit,omitempty"`
	ResetsAt *time.Time `json:"resets_at,omitempty"`
}

type Request struct {
	OperationID      string
	ProviderParentID string
	Workspace        *Workspace
	Session          Session
	Input            *apicompat.ResponsesRequest
}

// Event is the provider-independent normalized event contract, not Meta frames.
type Event struct {
	OperationID    string
	Sequence       int64
	ProviderTurnID string
	Data           apicompat.ResponsesStreamEvent
}

type Result struct {
	ProviderTurnID string
	Response       *apicompat.ResponsesResponse
}

// Provider's implementation is the only component needing the live app wire
// contract. NativeProvider supports the qualified standard-VM text scope;
// DisabledProvider remains the fallback when the transport port is unavailable.
type Provider interface {
	Qualified() bool
	Verify(context.Context, Session) (*Observation, error)
	Renew(context.Context, Session) (map[string]any, error)
	Execute(context.Context, Request, func(Event) error) (*Result, error)
	Cancel(context.Context, Session, string) (bool, error)
	Reconcile(context.Context, Session, string) (*Result, error)
}

type DisabledProvider struct{}

func (DisabledProvider) Qualified() bool { return false }
func (DisabledProvider) Verify(context.Context, Session) (*Observation, error) {
	return nil, ErrTransportUnqualified
}
func (DisabledProvider) Renew(context.Context, Session) (map[string]any, error) {
	return nil, ErrTransportUnqualified
}
func (DisabledProvider) Execute(context.Context, Request, func(Event) error) (*Result, error) {
	return nil, ErrTransportUnqualified
}
func (DisabledProvider) Cancel(context.Context, Session, string) (bool, error) {
	return false, ErrTransportUnqualified
}
func (DisabledProvider) Reconcile(context.Context, Session, string) (*Result, error) {
	return nil, ErrTransportUnqualified
}

var ErrRejected = errors.New("muse rejected the turn before acceptance")

var ErrRemoteFailed = errors.New("muse remote task failed")

var ErrCapability = errors.New("muse account does not support requested capability")

func (c Capabilities) Validate(req *apicompat.ResponsesRequest) error {
	if req == nil {
		return ErrInvalid
	}
	found := false
	for _, model := range c.Models {
		if model == req.Model {
			found = true
			break
		}
	}
	if !found {
		return ErrCapability
	}
	if (req.Stream && !c.Streaming) || (req.Instructions != "" && !c.Instructions) || ((len(req.Tools) > 0 || len(req.ToolChoice) > 0 || req.ParallelToolCalls != nil) && !c.Tools) ||
		(req.Reasoning != nil && !c.Reasoning) || (req.PreviousResponseID != "" && !c.Continuations) ||
		((req.MaxOutputTokens != nil || req.Temperature != nil || req.TopP != nil || req.Text != nil || req.Store != nil || len(req.Include) > 0 || req.ServiceTier != "" || req.PromptCacheKey != "" || len(req.PromptCacheOptions) > 0) && !c.Controls) {
		return ErrCapability
	}

	if req.MaxOutputTokens != nil && *req.MaxOutputTokens <= 0 {
		return ErrInvalid
	}
	var text string
	if json.Unmarshal(req.Input, &text) == nil {
		if strings.TrimSpace(text) == "" {
			return ErrInvalid
		}
		return nil
	}
	var items []map[string]json.RawMessage
	if json.Unmarshal(req.Input, &items) != nil || len(items) == 0 {
		return ErrInvalid
	}
	for _, item := range items {
		var role, kind string
		_ = json.Unmarshal(item["role"], &role)
		_ = json.Unmarshal(item["type"], &kind)
		switch kind {
		case "", "message":
			if role != "user" && role != "assistant" && role != "system" && role != "developer" {
				return ErrInvalid
			}
			if (role == "system" || role == "developer") && !c.Instructions {
				return ErrCapability
			}
			if json.Unmarshal(item["content"], &text) == nil {
				continue
			}
			var parts []map[string]json.RawMessage
			if json.Unmarshal(item["content"], &parts) != nil || len(parts) == 0 {
				return ErrInvalid
			}
			for _, part := range parts {
				var typ string
				_ = json.Unmarshal(part["type"], &typ)
				switch typ {
				case "input_text", "output_text":
					if json.Unmarshal(part["text"], &text) != nil {
						return ErrInvalid
					}
				case "input_image", "input_file":
					if !c.Vision {
						return ErrCapability
					}
				default:
					return ErrCapability
				}
			}
		case "function_call", "function_call_output", "custom_tool_call", "custom_tool_call_output":
			if !c.Tools {
				return ErrCapability
			}
		case "reasoning":
			if !c.Reasoning {
				return ErrCapability
			}
		default:
			return ErrCapability
		}
	}
	return nil
}
