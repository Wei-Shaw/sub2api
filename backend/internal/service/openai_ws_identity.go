package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"
	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	coderws "github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/net/http/httpguts"
	"google.golang.org/grpc"
)

// ErrOpenAIWSIdentityRewrite is a local plugin failure, not a provider/account signal.
var ErrOpenAIWSIdentityRewrite = errors.New("websocket identity rewrite failed")

const (
	wsIdentityRPCTimeout    = time.Second
	wsIdentityMaxValueBytes = 1024
	wsIdentityMaxRPCBytes   = 64 << 10
)

var wsIdentityHeaderKeys = []string{
	"user-agent", "originator", "session-id", "session_id", "thread-id", "thread_id",
	"conversation_id", "x-codex-window-id", "turn-id", "turn_id", "x-codex-parent-thread-id",
}

var wsIdentityBodyKeys = []string{
	"session_id", "thread_id", "turn_id", "window_id", "x-codex-window-id",
	"parent_thread_id", "x-codex-parent-thread-id", "parent_turn_id", "root_turn_id",
}

func pluginDeclaresWSIdentity(manifest PluginManifest) bool {
	for _, c := range manifest.Capabilities {
		if c.ID == PluginCapabilityOpenAIOAuthWSIdentity && c.Platform == PlatformOpenAI && c.AccountType == AccountTypeOAuth {
			return true
		}
	}
	return false
}

type wsIdentityStableKey struct{ kind, original string }

type openAIWSIdentitySession struct {
	runtime        *pluginRuntime
	client         pluginv1.WSIdentityPluginClient
	accountID      int64
	identitySource *Account
	apiKeyID       int64
	id             string
	epoch          uint64
	pinned         map[wsIdentityStableKey]string
}

func newOpenAIWSIdentitySession(cfg *config.Config, route *pluginRoute, accountID int64, identitySource *Account, apiKeyID int64) *openAIWSIdentitySession {
	if cfg == nil || !cfg.Plugins.AllowsOpenAIWSIdentity(accountID, apiKeyID) || route == nil || route.runtime == nil || codexAccountIdentityNamespace(identitySource) == "" {
		return nil
	}
	client, epoch := route.runtime.nativeWSIdentity(cfg.Plugins.OpenAIWSIdentityCompanionConfigured())
	if client == nil {
		return nil
	}
	return &openAIWSIdentitySession{runtime: route.runtime, client: client, accountID: accountID, identitySource: identitySource, apiKeyID: apiKeyID, id: uuid.NewString(), epoch: epoch}
}

func validWSIdentityValue(value string) bool {
	return len(value) > 0 && len(value) <= wsIdentityMaxValueBytes && utf8.ValidString(value) &&
		strings.TrimSpace(value) == value && !strings.ContainsRune(value, '\t') && httpguts.ValidHeaderFieldValue(value)
}

func projectWSIdentityObject(raw string) (map[string]string, error) {
	if !gjson.Valid(raw) || !gjson.Parse(raw).IsObject() {
		return nil, nil
	}
	if _, duplicate := requestmodel.ConflictingField("application/json", []byte(raw)); duplicate {
		return nil, newWSIdentityError("ambiguous identity metadata")
	}
	fields := make(map[string]string)
	for _, key := range wsIdentityBodyKeys {
		if v := gjson.Get(raw, key); v.Type == gjson.String && validWSIdentityValue(v.Str) {
			fields[key] = v.Str
		}
	}
	return fields, nil
}

func projectWSIdentityHeaders(headers http.Header) (*pluginv1.WSIdentityMetadata, error) {
	if headers == nil {
		return nil, nil
	}
	fields := make(map[string]string)
	for _, key := range wsIdentityHeaderKeys {
		if values := headers.Values(key); len(values) == 1 && validWSIdentityValue(values[0]) {
			fields[key] = values[0]
		}
	}
	turn, err := projectWSIdentityObject(headers.Get(openAIWSTurnMetadataHeader))
	return &pluginv1.WSIdentityMetadata{Fields: fields, TurnMetadata: turn}, err
}

func projectWSIdentityFrame(body []byte) (*pluginv1.WSIdentityMetadata, error) {
	object := gjson.GetBytes(body, "client_metadata")
	if !object.IsObject() {
		return nil, nil
	}
	fields, err := projectWSIdentityObject(object.Raw)
	if err != nil {
		return nil, err
	}
	turn, err := projectWSIdentityObject(object.Get(openAIWSTurnMetadataHeader).String())
	if len(fields) == 0 && len(turn) == 0 {
		return nil, err
	}
	return &pluginv1.WSIdentityMetadata{Fields: fields, TurnMetadata: turn}, err
}

// The core identity pass scopes ordinary IDs; scope lineage fields in the RPC
// projection too, without otherwise altering the native frame or handshake.
func (s *openAIWSIdentitySession) scopeLineage(metadata *pluginv1.WSIdentityMetadata) {
	for _, fields := range []map[string]string{metadata.GetFields(), metadata.GetTurnMetadata()} {
		for key, value := range fields {
			switch key {
			case "parent_thread_id", "x-codex-parent-thread-id":
				fields[key] = scopeCodexAccountIdentityValue(s.identitySource, s.apiKeyID, "thread", value)
			case "parent_turn_id", "root_turn_id":
				fields[key] = scopeCodexAccountIdentityValue(s.identitySource, s.apiKeyID, "turn", value)
			}
		}
	}
}

func validateWSIdentityPatch(input, patch *pluginv1.WSIdentityMetadata) error {
	for key, value := range patch.GetFields() {
		if _, present := input.GetFields()[key]; !present || !validWSIdentityValue(value) {
			return newWSIdentityError("invalid identity field replacement")
		}
	}
	for key, value := range patch.GetTurnMetadata() {
		if _, present := input.GetTurnMetadata()[key]; !present || !validWSIdentityValue(value) {
			return newWSIdentityError("invalid nested identity field replacement")
		}
	}
	return nil
}

func wsIdentityStableKind(key string) string {
	switch key {
	case "session_id", "session-id":
		return "session"
	case "thread_id", "thread-id":
		return "thread"
	case "conversation_id":
		return "conversation"
	default:
		return ""
	}
}

// Keep aliases of an already-mapped stable ID consistent even if a later patch omits them.
// Turn/window metadata is deliberately not pinned to the handshake.
func (s *openAIWSIdentitySession) pinScope(first bool, handshake, frame, handshakePatch, framePatch *pluginv1.WSIdentityMetadata) error {
	for _, pair := range [][2]map[string]string{
		{handshake.GetFields(), handshakePatch.Fields}, {handshake.GetTurnMetadata(), handshakePatch.TurnMetadata},
		{frame.GetFields(), framePatch.Fields}, {frame.GetTurnMetadata(), framePatch.TurnMetadata},
	} {
		for field, replacement := range pair[1] {
			if kind := wsIdentityStableKind(field); kind != "" {
				key := wsIdentityStableKey{kind, pair[0][field]}
				if pinned, ok := s.pinned[key]; ok && pinned != replacement {
					return newWSIdentityError("identity scope changed during the connection")
				}
				if first {
					s.pinned[key] = replacement
				}
			}
		}
	}
	for _, pair := range [][2]map[string]string{
		{handshake.GetFields(), handshakePatch.Fields}, {handshake.GetTurnMetadata(), handshakePatch.TurnMetadata},
		{frame.GetFields(), framePatch.Fields}, {frame.GetTurnMetadata(), framePatch.TurnMetadata},
	} {
		for field, original := range pair[0] {
			kind := wsIdentityStableKind(field)
			if kind == "" {
				continue
			}
			key := wsIdentityStableKey{kind, original}
			if pinned, ok := s.pinned[key]; ok {
				if pinned != original {
					pair[1][field] = pinned
				}
			} else if first {
				s.pinned[key] = original
			} else {
				for pinned := range s.pinned {
					if pinned.kind == kind {
						return newWSIdentityError("identity scope changed during the connection")
					}
				}
			}
		}
	}
	return nil
}

func applyWSIdentityObjectPatch(raw string, patch map[string]string) (string, error) {
	for key, value := range patch {
		var err error
		raw, err = sjson.Set(raw, key, value)
		if err != nil {
			return "", newWSIdentityError("could not apply identity patch")
		}
	}
	return raw, nil
}

func applyWSIdentityFrame(body []byte, input, patch *pluginv1.WSIdentityMetadata) ([]byte, error) {
	if len(patch.Fields) == 0 && len(patch.TurnMetadata) == 0 {
		return body, nil
	}
	object := gjson.GetBytes(body, "client_metadata")
	metadata, err := applyWSIdentityObjectPatch(object.Raw, patch.Fields)
	if err != nil {
		return nil, err
	}
	if len(patch.TurnMetadata) > 0 {
		turn, err := applyWSIdentityObjectPatch(object.Get(openAIWSTurnMetadataHeader).String(), patch.TurnMetadata)
		if err != nil {
			return nil, err
		}
		metadata, err = sjson.Set(metadata, openAIWSTurnMetadataHeader, turn)
		if err != nil {
			return nil, newWSIdentityError("could not apply turn identity patch")
		}
	}
	next, err := sjson.SetRawBytes(body, "client_metadata", []byte(metadata))
	if err != nil {
		return nil, newWSIdentityError("could not apply frame identity patch")
	}
	// Independent cache keys are not an identity input to the plugin.
	if session := patch.Fields["session_id"]; session != "" && gjson.GetBytes(body, "prompt_cache_key").String() == input.GetFields()["session_id"] {
		next, err = sjson.SetBytes(next, "prompt_cache_key", session)
		if err != nil {
			return nil, newWSIdentityError("could not apply session-derived cache key")
		}
	}
	return next, nil
}

func newWSIdentityError(reason string) error {
	return NewOpenAIWSClientCloseError(coderws.StatusInternalError, ErrOpenAIWSIdentityRewrite.Error(), fmt.Errorf("%w: %s", ErrOpenAIWSIdentityRewrite, reason))
}

func (s *openAIWSIdentitySession) rewrite(ctx context.Context, headers http.Header, body []byte) ([]byte, error) {
	if s == nil {
		return body, nil
	}
	frame, err := projectWSIdentityFrame(body)
	if err != nil {
		return nil, err
	}
	first := headers != nil
	if !first && frame == nil {
		return body, nil
	}
	handshake, err := projectWSIdentityHeaders(headers)
	if err != nil {
		return nil, err
	}
	if s.epoch%2 != 0 || s.runtime.wsConfigEpoch.Load() != s.epoch || !s.runtime.beginRequest() {
		return nil, newWSIdentityError("plugin is stopping or its configuration changed")
	}
	defer s.runtime.finishRequest()
	rpcCtx, cancel := context.WithTimeout(ctx, wsIdentityRPCTimeout)
	defer cancel()
	s.scopeLineage(handshake)
	s.scopeLineage(frame)
	result, err := s.client.RewriteWSIdentity(rpcCtx, &pluginv1.RewriteWSIdentityRequest{
		AccountId: s.accountID, ConnectionId: s.id, Handshake: handshake, ResponseCreate: frame,
	}, grpc.MaxCallSendMsgSize(wsIdentityMaxRPCBytes), grpc.MaxCallRecvMsgSize(wsIdentityMaxRPCBytes))
	if err != nil || result == nil || s.runtime.wsConfigEpoch.Load() != s.epoch {
		return nil, newWSIdentityError("plugin did not complete the identity rewrite")
	}
	if err := validateWSIdentityPatch(handshake, result.Handshake); err != nil {
		return nil, err
	}
	if err := validateWSIdentityPatch(frame, result.ResponseCreate); err != nil {
		return nil, err
	}
	if first {
		s.pinned = make(map[wsIdentityStableKey]string)
	}
	copyPatch := func(patch *pluginv1.WSIdentityMetadata) *pluginv1.WSIdentityMetadata {
		out := &pluginv1.WSIdentityMetadata{Fields: make(map[string]string), TurnMetadata: make(map[string]string)}
		for key, value := range patch.GetFields() {
			out.Fields[key] = value
		}
		for key, value := range patch.GetTurnMetadata() {
			out.TurnMetadata[key] = value
		}
		return out
	}
	handshakePatch, framePatch := copyPatch(result.Handshake), copyPatch(result.ResponseCreate)
	if err := s.pinScope(first, handshake, frame, handshakePatch, framePatch); err != nil {
		return nil, err
	}
	next, err := applyWSIdentityFrame(body, frame, framePatch)
	if err != nil {
		return nil, err
	}
	if len(handshakePatch.TurnMetadata) > 0 {
		turn, err := applyWSIdentityObjectPatch(headers.Get(openAIWSTurnMetadataHeader), handshakePatch.TurnMetadata)
		if err != nil {
			return nil, err
		}
		headers.Set(openAIWSTurnMetadataHeader, turn)
	}
	for key, value := range handshakePatch.Fields {
		headers.Set(key, value)
	}
	return next, nil
}
