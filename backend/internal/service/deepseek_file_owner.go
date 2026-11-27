package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// DeepseekFile is the gateway's upload record. AccountID, Origin and Format stay internal.
type DeepseekFile struct {
	ID        string
	AccountID int64 `json:"account_id,string"`
	Format    string
	Origin    string
	Filename  string
	Bytes     int64
	CreatedAt time.Time
	Purpose   string
	ExpiresAt int64
	MIMEType  string
}

type DeepseekFileStore interface {
	SaveDeepseekFile(ctx context.Context, userID int64, file DeepseekFile) error
	LookupDeepseekFiles(ctx context.Context, userID int64, ids []string) (map[string]DeepseekFile, error)
	ListDeepseekFiles(ctx context.Context, userID int64, page DeepseekFilePage) ([]DeepseekFile, error)
	DeleteDeepseekFile(ctx context.Context, userID, accountID int64, id string) error
}

var (
	errDeepseekFileStore    = infraerrors.ServiceUnavailable("api_error", "File ownership store is temporarily unavailable")
	errDeepseekFileNotFound = infraerrors.NotFound("not_found_error", "File not found")
)

func (s *OpenAIGatewayService) deepseekFileStore() (DeepseekFileStore, error) {
	if s != nil {
		if store, ok := s.cache.(DeepseekFileStore); ok {
			return store, nil
		}
	}
	return nil, errDeepseekFileStore
}

// Inspect every matching field, including repeated or escaped JSON keys. Upstream
// decoders can otherwise interpret a different occurrence from the gateway.
func eachFileField(object gjson.Result, name string, visit func(gjson.Result)) {
	object.ForEach(func(key, value gjson.Result) bool {
		if strings.EqualFold(key.String(), name) {
			visit(value)
		}
		return true
	})
}

func eachFilePart(array gjson.Result, visit func(gjson.Result)) {
	if array.IsArray() {
		array.ForEach(func(_, item gjson.Result) bool {
			visit(item)
			return true
		})
	}
}

func deepseekFileBlockType(block gjson.Result, types ...string) bool {
	matches := false
	block.ForEach(func(key, value gjson.Result) bool {
		matches = strings.EqualFold(key.String(), "type") && slices.Contains(types, value.String())
		return !matches
	})
	return matches
}

func deepseekFileIDs(body []byte, protocol string) []string {
	var ids []string
	seen := make(map[string]bool)
	add := func(v gjson.Result) {
		id := v.String()
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	readID := func(v gjson.Result) { eachFileField(v, "file_id", add) }
	visitPart := func(block gjson.Result) {
		switch protocol {
		case APIProtocolAnthropic:
			if deepseekFileBlockType(block, "image", "document") {
				eachFileField(block, "source", func(source gjson.Result) {
					if deepseekFileBlockType(source, "file") {
						readID(source)
					}
				})
			}
		case APIProtocolChatCompletions:
			if deepseekFileBlockType(block, "file") {
				readID(block)
				eachFileField(block, "file", readID)
			}
		case APIProtocolResponses:
			if deepseekFileBlockType(block, "input_file", "input_image") {
				readID(block)
			}
		}
	}
	content := func(parts gjson.Result) {
		eachFilePart(parts, func(block gjson.Result) {
			visitPart(block)
			if protocol == APIProtocolAnthropic && deepseekFileBlockType(block, "tool_result") {
				eachFileField(block, "content", func(nested gjson.Result) { eachFilePart(nested, visitPart) })
			}
		})
	}
	field := "messages"
	if protocol == APIProtocolResponses {
		field = "input"
	}
	eachFileField(parseRawJSONView(body), field, func(messages gjson.Result) {
		eachFilePart(messages, func(message gjson.Result) {
			eachFileField(message, "content", content)
			if protocol == APIProtocolResponses {
				visitPart(message)
				eachFileField(message, "output", content)
			}
		})
	})
	return ids
}

type deepseekFileBinding struct {
	accountID int64
	origin    string
}

func lookupDeepseekFileBinding(ctx context.Context, cache GatewayCache, userID int64, ids []string) (deepseekFileBinding, error) {
	var owner deepseekFileBinding
	for _, id := range ids {
		if userID <= 0 || !isValidDeepseekFileID(id) {
			return owner, errDeepseekFileNotFound
		}
	}
	store, ok := cache.(DeepseekFileStore)
	if !ok {
		return owner, errDeepseekFileStore
	}
	files, err := store.LookupDeepseekFiles(ctx, userID, ids)
	if err != nil {
		return owner, errDeepseekFileStore.WithCause(err)
	}
	for _, id := range ids {
		f := files[id]
		if f.AccountID <= 0 || f.Origin == "" {
			return deepseekFileBinding{}, errDeepseekFileNotFound
		}
	}
	for _, id := range ids {
		f := files[id]
		binding := deepseekFileBinding{f.AccountID, f.Origin}
		if owner.accountID != 0 && owner != binding {
			return deepseekFileBinding{}, invalidFileRequest("Referenced files belong to different upstreams")
		}
		owner = binding
	}
	return owner, nil
}

type deepseekFileAccountKey struct{}

type deepseekReferences struct {
	userID  int64
	ids     []string
	binding deepseekFileBinding
}
type deepseekReferencesKey struct{}

type deepseekDestinationError struct{ error }

func writeDeepseekReferenceError(c *gin.Context, err error) error {
	e := infraerrors.FromError(err)
	write := writeResponsesError
	switch path := c.Request.URL.Path; {
	case strings.Contains(path, "/messages"):
		write = writeAnthropicError
	case strings.Contains(path, "/chat/completions"):
		write = writeChatCompletionsError
	}
	write(c, int(e.Code), e.Reason, e.Message)
	MarkResponseCommitted(c)
	return fmt.Errorf("%w: %w", errInvalidGatewayRequest, err)
}

// Ordinary Anthropic API-key accounts can be configured against DeepSeek too.
// Inspect the final Messages body at request construction, including retries.
func (s *GatewayService) verifyAnthropicDeepseekFiles(ctx context.Context, c *gin.Context, account *Account, dest *url.URL, body []byte) error {
	if !isDeepseekOfficialHost(dest.String()) {
		return nil
	}
	ids := deepseekFileIDs(body, APIProtocolAnthropic)
	if len(ids) == 0 {
		return nil
	}
	user, _ := ctx.Value(ctxkey.UserID).(int64)
	ref := deepseekReferences{userID: user, ids: ids}
	if err := verifyDeepseekFileDestination(context.WithValue(ctx, deepseekReferencesKey{}, ref), s.cache, account, dest); err != nil {
		return writeDeepseekReferenceError(c, err)
	}
	return nil
}

func hasTopLevelJSONField(body []byte, name string) bool {
	found := false
	parseRawJSONView(body).ForEach(func(key, _ gjson.Result) bool {
		found = strings.EqualFold(key.String(), name)
		return !found
	})
	return found
}

func chatUsesResponsesShape(body []byte) bool {
	return !hasTopLevelJSONField(body, "messages") && hasTopLevelJSONField(body, "input")
}

// Resolve the same history shape that forwarding will consume. Native input
// takes precedence over an accompanying legacy Messages history.
func (s *OpenAIGatewayService) PrepareDeepseekFileIngress(ctx context.Context, platform, protocol string, userID int64, body []byte) (context.Context, []byte, error) {
	if protocol == APIProtocolResponses && hasTopLevelJSONField(body, "messages") {
		if len(deepseekFileIDs(body, APIProtocolChatCompletions)) > 0 {
			normalized, _, err := normalizeOpenAIResponsesLegacyIngressWithConfig(s.cfg, body)
			if err != nil {
				return ctx, body, invalidFileRequest("Invalid legacy Responses file input")
			}
			body = normalized
		}
	}
	if protocol == APIProtocolChatCompletions && chatUsesResponsesShape(body) {
		protocol = APIProtocolResponses
	}
	ids := deepseekFileIDs(body, protocol)
	if len(ids) == 0 {
		return ctx, body, nil
	}
	ref := deepseekReferences{userID: userID, ids: ids}
	if platform == PlatformDeepseek {
		var err error
		ref.binding, err = lookupDeepseekFileBinding(ctx, s.cache, userID, ids)
		if err != nil {
			return ctx, body, deepseekReferenceError(err)
		}
		ctx = withDeepseekFileAccount(ctx, ref.binding.accountID)
	}
	return context.WithValue(ctx, deepseekReferencesKey{}, ref), body, nil
}

func deepseekReferenceError(err error) error {
	if errors.Is(err, errDeepseekFileNotFound) {
		return invalidFileRequest("Referenced file not found")
	}
	return err
}

// Verify the final URL on each attempt. An OpenAI-platform mapping can also
// serve the official DeepSeek endpoint.
func withDeepseekWireFiles(req *http.Request, account *Account, protocol string, body []byte) *http.Request {
	if account.Platform != PlatformDeepseek && !isDeepseekOfficialHost(req.URL.String()) {
		return req
	}
	ids := deepseekFileIDs(body, protocol)
	if len(ids) == 0 {
		return req
	}
	ctx := req.Context()
	ref, ok := ctx.Value(deepseekReferencesKey{}).(deepseekReferences)
	if !ok {
		ref.userID, _ = ctx.Value(ctxkey.UserID).(int64)
	}
	ref.ids = ids
	return req.WithContext(context.WithValue(ctx, deepseekReferencesKey{}, ref))
}

func verifyDeepseekFileDestination(ctx context.Context, cache GatewayCache, account *Account, dest *url.URL) error {
	ref, ok := ctx.Value(deepseekReferencesKey{}).(deepseekReferences)
	if !ok || (account.Platform != PlatformDeepseek && !isDeepseekOfficialHost(dest.String())) {
		return nil
	}
	owner, err := lookupDeepseekFileBinding(ctx, cache, ref.userID, ref.ids)
	if err != nil {
		return deepseekReferenceError(err)
	}
	if owner.accountID != account.ID || owner.origin != deepseekStorageOrigin(dest) {
		return invalidFileRequest("Referenced file not found")
	}
	return nil
}

func withDeepseekFileAccount(ctx context.Context, accountID int64) context.Context {
	if accountID == 0 {
		return ctx
	}
	return context.WithValue(ctx, deepseekFileAccountKey{}, accountID)
}
