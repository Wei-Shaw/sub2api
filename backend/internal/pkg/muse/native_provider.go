package muse

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/google/uuid"
)

// NativeProvider supports a single text request in a fresh side chat on a
// standard VM. It never replays a submission, switches the account's global
// model, approves an action, or enables confidential/delegated trust modes.
type NativeProvider struct {
	do   SessionDo
	dial func(context.Context, Session, string) (NoiseSocket, func(), error)
}

func NewNativeProvider(do SessionDo) *NativeProvider {
	return &NativeProvider{do: do, dial: dialNativeNoise}
}

func (p *NativeProvider) Qualified() bool { return p != nil && p.do != nil && p.dial != nil }

// SubmissionProvider supplies a durable probe reference before any remote
// side effect. The reference is not a claim of remote acceptance.
type SubmissionProvider interface {
	SubmissionID(Request) (string, error)
}

type RequestValidator interface {
	ValidateRequest(*apicompat.ResponsesRequest) error
}

func (p *NativeProvider) ValidateRequest(input *apicompat.ResponsesRequest) error {
	_, err := nativeInput(input)
	return err
}

type nativeReference struct{ vm, principalHash, sideChat string }

func principalHash(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
func (r nativeReference) String() string {
	return "muse_noise_v1:" + r.vm + ":" + r.principalHash + ":" + r.sideChat
}
func parseNativeReference(value string) (*nativeReference, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 4 || parts[0] != "muse_noise_v1" || len(parts[2]) != 64 {
		return nil, ErrInvalid
	}
	for _, id := range []string{parts[1], parts[3]} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed.String() != id {
			return nil, ErrInvalid
		}
	}
	if _, err := hex.DecodeString(parts[2]); err != nil {
		return nil, ErrInvalid
	}
	return &nativeReference{vm: parts[1], principalHash: parts[2], sideChat: parts[3]}, nil
}
func (p *NativeProvider) SubmissionID(request Request) (string, error) {
	if !validID(request.OperationID, 96) || request.Workspace == nil || request.Workspace.Identity.Validate() != nil || request.ProviderParentID != "" {
		return "", ErrInvalid
	}
	w := request.Workspace.Identity
	if w.AccountID != request.Session.AccountID || w.OwnerUserID != request.Session.OwnerUserID || !w.AccountUpdatedAt.Equal(request.Session.AccountUpdatedAt) {
		return "", ErrGeneration
	}
	if id, err := uuid.Parse(w.WorkspaceID); err != nil || id.String() != w.WorkspaceID {
		return "", ErrInvalid
	}
	sideChat := uuid.NewSHA1(uuid.NameSpaceOID, []byte("sub2api-muse:"+w.PrincipalID+":"+w.WorkspaceID+":"+request.OperationID)).String()
	return (nativeReference{vm: w.WorkspaceID, principalHash: principalHash(w.PrincipalID), sideChat: sideChat}).String(), nil
}

func nativeCatalog(body []byte) ([]string, error) {
	payload, err := NoiseRPCPayload(body)
	if err != nil {
		return nil, err
	}
	var catalog struct {
		Models []struct {
			ID string `json:"id"`
		} `json:"available_models"`
	}
	if json.Unmarshal(payload, &catalog) != nil || len(catalog.Models) == 0 || len(catalog.Models) > 256 {
		return nil, ErrCapability
	}
	models := make([]string, 0, len(catalog.Models))
	seen := map[string]bool{}
	for _, model := range catalog.Models {
		if !validID(model.ID, 200) {
			return nil, ErrCapability
		}
		if !seen[model.ID] {
			models = append(models, model.ID)
			seen[model.ID] = true
		}
	}
	return models, nil
}

func (p *NativeProvider) Verify(ctx context.Context, session Session) (*Observation, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	conn, boot, err := p.connect(ctx, session)
	if err != nil {
		return nil, err
	}
	defer conn.close()
	body, err := conn.request(ctx, "GET", "/model", nil)
	if err != nil {
		return nil, err
	}
	models, err := nativeCatalog(body)
	if err != nil {
		return nil, err
	}
	// This is authenticated model access, not inference based on a plan name.
	// Plan, allowance, and token usage remain unknown until observed natively.
	return &Observation{InferenceAllowed: true, Identity: Identity{PrincipalID: boot.viewer, WorkspaceID: boot.target.VMID}, Capabilities: Capabilities{Models: models, Streaming: true}, SessionExpiresAt: boot.expires}, nil
}

func (p *NativeProvider) Renew(ctx context.Context, session Session) (map[string]any, error) {
	boot, err := p.bootstrap(ctx, session)
	if err != nil {
		return nil, err
	}
	return boot.document, nil
}

func nativeInput(input *apicompat.ResponsesRequest) (string, error) {
	if input == nil || !validID(input.Model, 200) {
		return "", ErrInvalid
	}
	if err := (Capabilities{Models: []string{input.Model}, Streaming: true}).Validate(input); err != nil {
		return "", err
	}
	var text string
	if json.Unmarshal(input.Input, &text) != nil {
		decode := func(body []byte, value any) error {
			decoder := json.NewDecoder(strings.NewReader(string(body)))
			decoder.DisallowUnknownFields()
			return decoder.Decode(value)
		}
		var items []struct {
			Type string          `json:"type"`
			Role string          `json:"role"`
			Body json.RawMessage `json:"content"`
		}
		if decode(input.Input, &items) != nil || len(items) != 1 || items[0].Role != "user" || (items[0].Type != "" && items[0].Type != "message") {
			return "", ErrCapability
		}
		if json.Unmarshal(items[0].Body, &text) != nil {
			var parts []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if decode(items[0].Body, &parts) != nil || len(parts) == 0 {
				return "", ErrInvalid
			}
			for _, part := range parts {
				if part.Type != "input_text" && part.Type != "output_text" {
					return "", ErrCapability
				}
				text += part.Text
			}
		}
	}
	if strings.TrimSpace(text) == "" || len(text) > 1<<20 {
		return "", ErrInvalid
	}
	return text, nil
}

func (p *NativeProvider) ownedConnection(ctx context.Context, session Session, reference *nativeReference) (*nativeConnection, error) {
	conn, boot, err := p.connect(ctx, session)
	if err != nil {
		return nil, err
	}
	if boot.target.VMID != reference.vm || principalHash(boot.viewer) != reference.principalHash {
		conn.close()
		return nil, ErrGeneration
	}
	return conn, nil
}

func (p *NativeProvider) Execute(ctx context.Context, request Request, emit func(Event) error) (*Result, error) {
	text, err := nativeInput(request.Input)
	if err != nil {
		return nil, errors.Join(ErrRejected, err)
	}
	ref, err := p.SubmissionID(request)
	if err != nil {
		return nil, errors.Join(ErrRejected, err)
	}
	reference, _ := parseNativeReference(ref)
	conn, err := p.ownedConnection(ctx, request.Session, reference)
	if err != nil {
		return nil, errors.Join(ErrRejected, err)
	}
	defer conn.close()
	// Check current model access again before submission. A stale catalog cannot
	// silently redirect a requested model to auto or alter the VM's global model.
	body, err := conn.request(ctx, "GET", "/model", nil)
	if err != nil {
		return nil, errors.Join(ErrRejected, err)
	}
	models, err := nativeCatalog(body)
	if err != nil {
		return nil, errors.Join(ErrRejected, err)
	}
	allowed := false
	for _, model := range models {
		allowed = allowed || model == request.Input.Model
	}
	if !allowed {
		return nil, errors.Join(ErrRejected, ErrCapability)
	}
	body, err = conn.request(ctx, "POST", "/chat/stream", map[string]any{"session_id": reference.sideChat, "node_id": uuid.NewString(), "message": text, "model": request.Input.Model, "capabilities": []string{"chat_cancel", "delta_stream"}})
	if err != nil {
		return nil, err // Sending succeeded or may have succeeded: never retry.
	}
	ack, err := ParseChatAcknowledgement(body, reference.sideChat)
	if err != nil {
		return nil, err
	}
	state := newNativeTurn(reference.sideChat, ack.MessageID, request.Input.Model, ref, request.OperationID, emit)
	return p.readTurn(ctx, conn, state)
}

func (p *NativeProvider) readTurn(ctx context.Context, conn *nativeConnection, state *nativeTurn) (*Result, error) {
	id, err := conn.send(ctx, "POST", "/chat/subscribe", map[string]any{"session_id": state.sessionID, "after_stream_seq": 0, "after_chat_event_seq": 0, "capabilities": []string{"chat_cancel", "delta_stream"}})
	if err != nil {
		return nil, err
	}
	var decoder NoiseSubscriptionDecoder
	for {
		event, err := conn.read(ctx)
		if err != nil {
			return nil, err
		}
		if event.StreamID != id || event.Kind == "reset" || (event.Kind == "response" && event.Status != 200) {
			return nil, ErrNoiseProtocol
		}
		err = decoder.Feed(event.Body, event.EndBody, state.record)
		if err != nil && err != io.EOF {
			return nil, err
		}
		if state.result != nil {
			return state.result, nil
		}
		if event.EndBody || err == io.EOF {
			return nil, ErrOwnerReview
		}
	}
}

func (p *NativeProvider) Reconcile(ctx context.Context, session Session, ref string) (*Result, error) {
	reference, err := parseNativeReference(ref)
	if err != nil {
		return nil, err
	}
	conn, err := p.ownedConnection(ctx, session, reference)
	if err != nil {
		return nil, err
	}
	defer conn.close()
	return p.readTurn(ctx, conn, newNativeTurn(reference.sideChat, "", "", ref, "", nil))
}
