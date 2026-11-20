package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/tidwall/gjson"
)

// DeepseekFile is the gateway's upload record. AccountID and Format are internal.
type DeepseekFile struct {
	ID        string
	AccountID int64 `json:"account_id,string"`
	Format    string
	Filename  string
	Bytes     int64
	CreatedAt time.Time
	Purpose   string
	ExpiresAt int64
	MIMEType  string
}

type DeepseekFileStore interface {
	SaveDeepseekFile(ctx context.Context, userID int64, file DeepseekFile) error
	DeepseekFileAccounts(ctx context.Context, userID int64, ids []string) (map[string]int64, error)
	ListDeepseekFiles(ctx context.Context, userID int64) ([]DeepseekFile, error)
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
	eachFileField(gjson.ParseBytes(body), field, func(messages gjson.Result) {
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

// DeepseekFileAccount returns zero for a request without references. Missing
// and foreign records have the same response, including in mixed requests.
func (s *OpenAIGatewayService) DeepseekFileAccount(ctx context.Context, userID int64, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	for _, id := range ids {
		if userID <= 0 || !isValidDeepseekFileID(id) {
			return 0, errDeepseekFileNotFound
		}
	}
	store, err := s.deepseekFileStore()
	if err != nil {
		return 0, err
	}
	accounts, err := store.DeepseekFileAccounts(ctx, userID, ids)
	if err != nil {
		return 0, errDeepseekFileStore.WithCause(err)
	}
	owner := accounts[ids[0]]
	for _, id := range ids {
		if accounts[id] <= 0 {
			return 0, errDeepseekFileNotFound
		}
	}
	for _, id := range ids {
		if accounts[id] != owner {
			return 0, infraerrors.BadRequest("invalid_request_error", "Referenced files belong to different upstream accounts")
		}
	}
	return owner, nil
}

type deepseekFileAccountKey struct{}

// WithDeepseekFileReferences pins all scheduling attempts to the uploading
// account after composite routing has resolved the request platform.
func (s *OpenAIGatewayService) WithDeepseekFileReferences(ctx context.Context, platform, protocol string, userID int64, body []byte) (context.Context, error) {
	if platform != PlatformDeepseek {
		return ctx, nil
	}
	id, err := s.DeepseekFileAccount(ctx, userID, deepseekFileIDs(body, protocol))
	if err != nil {
		if errors.Is(err, errDeepseekFileNotFound) {
			return ctx, invalidFileRequest("Referenced file not found")
		}
		return ctx, err
	}
	return withDeepseekFileAccount(ctx, id), nil
}

func withDeepseekFileAccount(ctx context.Context, accountID int64) context.Context {
	if accountID == 0 {
		return ctx
	}
	return context.WithValue(ctx, deepseekFileAccountKey{}, accountID)
}
