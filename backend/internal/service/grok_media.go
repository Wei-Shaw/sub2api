package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

const (
	// The authoritative owner outlives the longest supported generation and a
	// multi-day quiesce/restart recovery. Every authorized status/content read
	// renews this full window on the same immutable account.
	grokMediaVideoRequestOwnerRecoveryWindow = 7 * 24 * time.Hour
	grokMediaVideoTerminalRetention          = 7 * 24 * time.Hour
	grokMediaExpiredCleanupBatchSize         = 100
	grokMediaVideoCreateIdempotencyTTL       = 24 * time.Hour
	grokMediaVideoIdempotencyKeyMaxLen       = 255
	grokMediaImageCreateIdempotencyTTL       = 24 * time.Hour
	grokMediaImageIdempotencyKeyMaxLen       = 255
)

var (
	ErrGrokMediaVideoRequestOwnerNotFound    = errors.New("grok video request owner binding not found")
	ErrGrokMediaVideoRequestOwnerUnavailable = errors.New("grok video request owner account unavailable")
	ErrGrokMediaVideoRequestOwnerConflict    = errors.New("grok video request owner binding conflict")
	ErrGrokMediaVideoIdempotencyKeyRequired  = errors.New("grok video Idempotency-Key is required")
	ErrGrokMediaVideoIdempotencyKeyInvalid   = errors.New("grok video Idempotency-Key is invalid")
	ErrGrokMediaVideoIdempotencyConflict     = errors.New("grok video Idempotency-Key reused with different payload")
	ErrGrokMediaVideoIdempotencyUnavailable  = errors.New("grok video idempotency store unavailable")
	ErrGrokMediaImageIdempotencyKeyInvalid   = errors.New("grok image Idempotency-Key is invalid")
	ErrGrokMediaImageIdempotencyConflict     = errors.New("grok image Idempotency-Key reused with different payload")
	ErrGrokMediaImageIdempotencyUnavailable  = errors.New("grok image idempotency store unavailable")
)

type GrokMediaVideoRequestOwner struct {
	RequestID  string
	UserID     int64
	APIKeyID   int64
	GroupID    int64
	AccountID  int64
	ExpiresAt  time.Time
	TerminalAt *time.Time
}

type GrokMediaVideoCreateRecord struct {
	UserID                 int64
	APIKeyID               int64
	GroupID                int64
	Endpoint               GrokMediaEndpoint
	IdempotencyKeyHash     string
	RequestHash            string
	UpstreamIdempotencyKey string
	AccountID              int64
	RequestID              string
	ResponseStatus         int
	ResponseContentType    string
	ResponseBody           []byte
	ExpiresAt              time.Time
}

type GrokMediaImageCreateRecord struct {
	UserID                 int64
	APIKeyID               int64
	GroupID                int64
	Endpoint               GrokMediaEndpoint
	IdempotencyKeyHash     string
	RequestHash            string
	UpstreamIdempotencyKey string
	AccountID              int64
	ResponseStatus         int
	ResponseContentType    string
	ResponseBody           []byte
	ExpiresAt              time.Time
}

func (r *GrokMediaImageCreateRecord) Completed() bool {
	return r != nil && r.AccountID > 0 && r.ResponseStatus >= 200 && r.ResponseStatus < 300 && r.ResponseBody != nil
}

func (r *GrokMediaVideoCreateRecord) Completed() bool {
	return r != nil && strings.TrimSpace(r.RequestID) != "" && r.ResponseStatus >= 200 && r.ResponseStatus < 300 && r.ResponseBody != nil
}

// GrokMediaVideoRequestOwnerRepository is the durable source of truth for the
// account that created an asynchronous Grok video request. Redis remains only
// a derived acceleration cache and must never override this record.
type GrokMediaVideoRequestOwnerRepository interface {
	Bind(ctx context.Context, owner GrokMediaVideoRequestOwner) error
	Resolve(ctx context.Context, requestID string, userID, apiKeyID, groupID int64, refreshUntil time.Time) (*GrokMediaVideoRequestOwner, error)
	MarkTerminal(ctx context.Context, requestID string, userID, apiKeyID, groupID int64, terminalAt, retainUntil time.Time) error
	DeleteExpired(ctx context.Context, before time.Time, limit int) (int64, error)
	DeleteExpiredVideoCreates(ctx context.Context, before time.Time, limit int) (int64, error)
	ClaimVideoCreate(ctx context.Context, record GrokMediaVideoCreateRecord) (*GrokMediaVideoCreateRecord, error)
	BindVideoCreateAccount(ctx context.Context, record GrokMediaVideoCreateRecord, accountID int64) (int64, error)
	ReleaseVideoCreateAccount(ctx context.Context, record GrokMediaVideoCreateRecord, accountID int64) (bool, error)
	CompleteVideoCreate(ctx context.Context, record GrokMediaVideoCreateRecord, owner GrokMediaVideoRequestOwner) error
}

// GrokMediaImageCreateRepository persists only synchronous image-create
// intents and replay responses. It must never write video request owners.
type GrokMediaImageCreateRepository interface {
	ClaimImageCreate(ctx context.Context, record GrokMediaImageCreateRecord) (*GrokMediaImageCreateRecord, error)
	BindImageCreateAccount(ctx context.Context, record GrokMediaImageCreateRecord, accountID int64) (int64, error)
	ReleaseImageCreateAccount(ctx context.Context, record GrokMediaImageCreateRecord, accountID int64) (bool, error)
	CompleteImageCreate(ctx context.Context, record GrokMediaImageCreateRecord) error
	DeleteExpired(ctx context.Context, before time.Time, limit int) (int64, error)
}

type GrokMediaExpiredCleanupStats struct {
	OwnerDeleted       int64
	VideoCreateDeleted int64
	ImageCreateDeleted int64
	Duration           time.Duration
}

const (
	grokMediaBufferedResponseContextKey    = "grok_media_buffered_response"
	grokMediaUpstreamIdempotencyContextKey = "grok_media_upstream_idempotency_key"
)

type grokMediaBufferedResponse struct {
	statusCode int
	header     http.Header
	body       []byte
}

type GrokMediaEndpoint string

const (
	GrokMediaEndpointImagesGenerations GrokMediaEndpoint = "images_generations"
	GrokMediaEndpointImagesEdits       GrokMediaEndpoint = "images_edits"
	GrokMediaEndpointVideosGenerations GrokMediaEndpoint = "videos_generations"
	GrokMediaEndpointVideosEdits       GrokMediaEndpoint = "videos_edits"
	GrokMediaEndpointVideosExtensions  GrokMediaEndpoint = "videos_extensions"
	GrokMediaEndpointVideoStatus       GrokMediaEndpoint = "video_status"
	GrokMediaEndpointVideoContent      GrokMediaEndpoint = "video_content"
)

func (e GrokMediaEndpoint) RequiresRequestBody() bool {
	return !e.IsVideoLookupRequest()
}

func (e GrokMediaEndpoint) IsVideoLookupRequest() bool {
	return e == GrokMediaEndpointVideoStatus || e == GrokMediaEndpointVideoContent
}

func (e GrokMediaEndpoint) IsVideoGenerationRequest() bool {
	switch e {
	case GrokMediaEndpointVideosGenerations, GrokMediaEndpointVideosEdits, GrokMediaEndpointVideosExtensions:
		return true
	default:
		return false
	}
}

func (e GrokMediaEndpoint) IsImageGenerationRequest() bool {
	return e == GrokMediaEndpointImagesGenerations || e == GrokMediaEndpointImagesEdits
}

func (e GrokMediaEndpoint) IsGenerationRequest() bool {
	switch e {
	case GrokMediaEndpointImagesGenerations, GrokMediaEndpointImagesEdits, GrokMediaEndpointVideosGenerations, GrokMediaEndpointVideosEdits, GrokMediaEndpointVideosExtensions:
		return true
	default:
		return false
	}
}

type GrokMediaRequestInfo struct {
	Model           string
	Prompt          string
	N               int
	Size            string
	SizeTier        string
	Resolution      string
	DurationSeconds int
	InputImageURLs  []string
	MaskImageURL    string
	Uploads         []OpenAIImagesUpload
	MaskUpload      *OpenAIImagesUpload
}

func (r GrokMediaRequestInfo) ModerationBody() []byte {
	payload := map[string]any{}
	if prompt := strings.TrimSpace(r.Prompt); prompt != "" {
		payload["prompt"] = prompt
	}

	images := make([]map[string]string, 0, len(r.InputImageURLs)+len(r.Uploads)+1)
	for _, imageURL := range r.InputImageURLs {
		if imageURL = strings.TrimSpace(imageURL); imageURL != "" {
			images = append(images, map[string]string{"image_url": imageURL})
		}
	}
	for _, upload := range r.Uploads {
		if dataURL := upload.ModerationDataURL(); dataURL != "" {
			images = append(images, map[string]string{"image_url": dataURL})
		}
	}
	if maskURL := strings.TrimSpace(r.MaskImageURL); maskURL != "" {
		images = append(images, map[string]string{"image_url": maskURL})
	}
	if r.MaskUpload != nil {
		if dataURL := r.MaskUpload.ModerationDataURL(); dataURL != "" {
			images = append(images, map[string]string{"image_url": dataURL})
		}
	}
	if len(images) > 0 {
		payload["images"] = images
	}
	if len(payload) == 0 {
		return nil
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return body
}

func (e GrokMediaEndpoint) httpMethod() string {
	if e.IsVideoLookupRequest() {
		return http.MethodGet
	}
	return http.MethodPost
}

func ExtractGrokMediaModel(contentType string, body []byte) string {
	return ParseGrokMediaRequest(contentType, body).Model
}

func ParseGrokMediaRequest(contentType string, body []byte) GrokMediaRequestInfo {
	info := GrokMediaRequestInfo{N: 1}
	if gjson.ValidBytes(body) {
		parseGrokMediaJSONRequest(body, &info)
	} else {
		parseGrokMediaMultipartRequest(contentType, body, &info)
	}
	info.Model = strings.TrimSpace(info.Model)
	info.Prompt = strings.TrimSpace(info.Prompt)
	info.Size = strings.TrimSpace(info.Size)
	info.SizeTier = NormalizeImageBillingTierOrDefault(info.Size)
	info.Resolution = NormalizeVideoBillingResolutionOrDefault(info.Resolution)
	info.DurationSeconds = NormalizeVideoBillingDurationSecondsOrDefault(info.DurationSeconds)
	if info.N <= 0 {
		info.N = 1
	}
	return info
}

func parseGrokMediaJSONRequest(body []byte, info *GrokMediaRequestInfo) {
	if info == nil {
		return
	}
	info.Model = strings.TrimSpace(gjson.GetBytes(body, "model").String())
	info.Prompt = strings.TrimSpace(gjson.GetBytes(body, "prompt").String())
	info.Size = strings.TrimSpace(gjson.GetBytes(body, "size").String())
	info.Resolution = strings.TrimSpace(gjson.GetBytes(body, "resolution").String())
	if duration := gjson.GetBytes(body, "duration"); duration.Exists() && duration.Type == gjson.Number {
		info.DurationSeconds = int(duration.Int())
	}
	if n := gjson.GetBytes(body, "n"); n.Exists() && n.Type == gjson.Number {
		info.N = int(n.Int())
	}
	appendJSONImageURLs := func(value gjson.Result) {
		if !value.Exists() {
			return
		}
		switch {
		case value.IsArray():
			for _, item := range value.Array() {
				if imageURL := grokMediaJSONImageURL(item); imageURL != "" {
					info.InputImageURLs = append(info.InputImageURLs, imageURL)
					continue
				}
				if item.Type == gjson.String {
					imageURL := strings.TrimSpace(item.String())
					if imageURL == "" {
						continue
					}
					info.InputImageURLs = append(info.InputImageURLs, imageURL)
				}
			}
		default:
			if imageURL := grokMediaJSONImageURL(value); imageURL != "" {
				info.InputImageURLs = append(info.InputImageURLs, imageURL)
				return
			}
			if value.Type == gjson.String {
				imageURL := strings.TrimSpace(value.String())
				if imageURL == "" {
					return
				}
				info.InputImageURLs = append(info.InputImageURLs, imageURL)
			}
		}
	}
	appendJSONImageURLs(gjson.GetBytes(body, "image"))
	appendJSONImageURLs(gjson.GetBytes(body, "images"))
	appendJSONImageURLs(gjson.GetBytes(body, "reference_images"))
	info.MaskImageURL = grokMediaJSONImageURL(gjson.GetBytes(body, "mask"))
}

func grokMediaJSONImageURL(value gjson.Result) string {
	if imageURL := strings.TrimSpace(value.Get("url").String()); imageURL != "" {
		return imageURL
	}
	return strings.TrimSpace(value.Get("image_url").String())
}

func parseGrokMediaMultipartRequest(contentType string, body []byte, info *GrokMediaRequestInfo) {
	if info == nil {
		return
	}
	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return
		}
		if err != nil {
			return
		}
		name := strings.TrimSpace(part.FormName())
		if name == "" {
			_ = part.Close()
			continue
		}
		data, err := io.ReadAll(io.LimitReader(part, openAIImageMaxUploadPartSize))
		_ = part.Close()
		if err != nil {
			return
		}
		fileName := strings.TrimSpace(part.FileName())
		partContentType := strings.TrimSpace(part.Header.Get("Content-Type"))
		if fileName != "" {
			upload := OpenAIImagesUpload{
				FieldName:   name,
				FileName:    fileName,
				ContentType: partContentType,
				Data:        data,
			}
			if name == "mask" {
				info.MaskUpload = &upload
				continue
			}
			if name == "image" || strings.HasPrefix(name, "image[") {
				info.Uploads = append(info.Uploads, upload)
			}
			continue
		}

		value := strings.TrimSpace(string(data))
		switch name {
		case "model":
			info.Model = value
		case "prompt":
			info.Prompt = value
		case "size":
			info.Size = value
		case "resolution":
			info.Resolution = value
		case "duration":
			if duration, err := strconv.Atoi(value); err == nil {
				info.DurationSeconds = duration
			}
		case "n":
			if n, err := strconv.Atoi(value); err == nil {
				info.N = n
			}
		case "image", "image_url":
			if value != "" {
				info.InputImageURLs = append(info.InputImageURLs, value)
			}
		case "mask", "mask_image_url":
			info.MaskImageURL = value
		}
	}
}

func grokMediaVideoRequestOwnerCacheKey(requestID string, userID, apiKeyID, groupID int64) string {
	sessionHash := GrokMediaVideoRequestSessionHash(requestID, userID, apiKeyID)
	if sessionHash == "" {
		return ""
	}
	// This is deliberately not an OpenAI sticky-session key.
	return fmt.Sprintf("grok-video-owner:v2:%d:%s", groupID, sessionHash)
}

func GrokMediaVideoRequestSessionHash(requestID string, userID, apiKeyID int64) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || userID <= 0 || apiKeyID <= 0 {
		return ""
	}
	ownerSeed := fmt.Sprintf("%d:%d:%s", userID, apiKeyID, requestID)
	return "grok-video:" + DeriveSessionHashFromSeed(ownerSeed)
}

func CanonicalGrokMediaVideoRequestHash(endpoint GrokMediaEndpoint, contentType string, body []byte) (string, error) {
	if !endpoint.IsVideoGenerationRequest() {
		return "", fmt.Errorf("grok video create endpoint is invalid")
	}
	mediaType := strings.ToLower(strings.TrimSpace(contentType))
	if parsed, _, err := mime.ParseMediaType(mediaType); err == nil {
		mediaType = strings.ToLower(parsed)
	}
	if mediaType == "" {
		mediaType = "application/json"
	}

	canonicalBody := bytes.TrimSpace(body)
	if strings.HasSuffix(mediaType, "/json") || strings.HasSuffix(mediaType, "+json") {
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return "", fmt.Errorf("decode grok video request for canonical hash: %w", err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			if err == nil {
				return "", fmt.Errorf("decode grok video request for canonical hash: trailing JSON value")
			}
			return "", fmt.Errorf("decode grok video request for canonical hash: %w", err)
		}
		var err error
		canonicalBody, err = json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("encode canonical grok video request: %w", err)
		}
	}

	sum := sha256.Sum256(bytes.Join([][]byte{
		[]byte(endpoint),
		[]byte(mediaType),
		canonicalBody,
	}, []byte{'\n'}))
	return fmt.Sprintf("%x", sum[:]), nil
}

type grokMediaMultipartHashValue struct {
	ContentType string `json:"content_type,omitempty"`
	BodySHA256  string `json:"body_sha256"`
}

type grokMediaMultipartHashField struct {
	Name   string                        `json:"name"`
	Values []grokMediaMultipartHashValue `json:"values"`
}

// CanonicalGrokMediaImageRequestHash normalizes JSON object ordering and
// multipart boundaries and cross-field part ordering while hashing uploaded
// bytes rather than storing them. Client-side temporary filenames are not
// semantic input and are deliberately excluded. Repeated values for the same
// field remain in arrival order, preserving multi-image ordering.
func CanonicalGrokMediaImageRequestHash(endpoint GrokMediaEndpoint, contentType string, body []byte) (string, error) {
	if !endpoint.IsImageGenerationRequest() {
		return "", fmt.Errorf("grok image create endpoint is invalid")
	}
	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil {
		if strings.TrimSpace(contentType) != "" {
			return "", fmt.Errorf("parse grok image content type: %w", err)
		}
		mediaType = "application/json"
		params = map[string]string{}
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	var canonicalBody []byte
	switch {
	case strings.HasSuffix(mediaType, "/json") || strings.HasSuffix(mediaType, "+json"):
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return "", fmt.Errorf("decode grok image request for canonical hash: %w", err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("decode grok image request for canonical hash: trailing JSON value")
		}
		canonicalBody, err = json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("encode canonical grok image request: %w", err)
		}
	case mediaType == "multipart/form-data":
		boundary := strings.TrimSpace(params["boundary"])
		if boundary == "" {
			return "", fmt.Errorf("grok image multipart boundary is required")
		}
		reader := multipart.NewReader(bytes.NewReader(body), boundary)
		valuesByField := make(map[string][]grokMediaMultipartHashValue)
		for {
			part, partErr := reader.NextPart()
			if errors.Is(partErr, io.EOF) {
				break
			}
			if partErr != nil {
				return "", fmt.Errorf("read grok image multipart request: %w", partErr)
			}
			partBody, readErr := io.ReadAll(part)
			_ = part.Close()
			if readErr != nil {
				return "", fmt.Errorf("read grok image multipart part: %w", readErr)
			}
			partSum := sha256.Sum256(partBody)
			fieldName := part.FormName()
			if fieldName == "" {
				return "", fmt.Errorf("grok image multipart part name is required")
			}
			valuesByField[fieldName] = append(valuesByField[fieldName], grokMediaMultipartHashValue{
				ContentType: strings.ToLower(strings.TrimSpace(part.Header.Get("Content-Type"))),
				BodySHA256:  fmt.Sprintf("%x", partSum[:]),
			})
		}
		fieldNames := make([]string, 0, len(valuesByField))
		for fieldName := range valuesByField {
			fieldNames = append(fieldNames, fieldName)
		}
		sort.Strings(fieldNames)
		fields := make([]grokMediaMultipartHashField, 0, len(fieldNames))
		for _, fieldName := range fieldNames {
			fields = append(fields, grokMediaMultipartHashField{Name: fieldName, Values: valuesByField[fieldName]})
		}
		canonicalBody, err = json.Marshal(fields)
		if err != nil {
			return "", fmt.Errorf("encode canonical grok image multipart request: %w", err)
		}
	default:
		canonicalBody = bytes.TrimSpace(body)
	}
	sum := sha256.Sum256(bytes.Join([][]byte{[]byte(endpoint), []byte(mediaType), canonicalBody}, []byte{'\n'}))
	return fmt.Sprintf("%x", sum[:]), nil
}

func validGrokMediaIdempotencyKey(key string, maxLen int) bool {
	if key == "" || len(key) > maxLen {
		return false
	}
	for _, char := range key {
		if char < 0x21 || char > 0x7e {
			return false
		}
	}
	return true
}

// ClaimGrokMediaImageCreate is intentionally optional for legacy callers that
// omit Idempotency-Key. Once a key is present, store unavailability fails
// closed because forwarding without the durable account binding is unsafe.
func (s *OpenAIGatewayService) ClaimGrokMediaImageCreate(
	ctx context.Context,
	groupID *int64,
	endpoint GrokMediaEndpoint,
	idempotencyKey, contentType string,
	body []byte,
	userID, apiKeyID int64,
) (*GrokMediaImageCreateRecord, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return nil, nil
	}
	if !validGrokMediaIdempotencyKey(idempotencyKey, grokMediaImageIdempotencyKeyMaxLen) {
		return nil, ErrGrokMediaImageIdempotencyKeyInvalid
	}
	if s == nil || s.grokMediaImageCreateRepo == nil {
		return nil, ErrGrokMediaImageIdempotencyUnavailable
	}
	requestHash, err := CanonicalGrokMediaImageRequestHash(endpoint, contentType, body)
	if err != nil {
		return nil, err
	}
	keySum := sha256.Sum256([]byte(idempotencyKey))
	keyHash := fmt.Sprintf("%x", keySum[:])
	group := derefGroupID(groupID)
	upstreamSum := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%d:%s:%s", userID, apiKeyID, group, endpoint, keyHash)))
	record := GrokMediaImageCreateRecord{
		UserID:                 userID,
		APIKeyID:               apiKeyID,
		GroupID:                group,
		Endpoint:               endpoint,
		IdempotencyKeyHash:     keyHash,
		RequestHash:            requestHash,
		UpstreamIdempotencyKey: "sub2api-grok-image-" + fmt.Sprintf("%x", upstreamSum[:]),
		ExpiresAt:              time.Now().Add(grokMediaImageCreateIdempotencyTTL),
	}
	claimed, err := s.grokMediaImageCreateRepo.ClaimImageCreate(ctx, record)
	if err != nil {
		if errors.Is(err, ErrGrokMediaImageIdempotencyConflict) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", ErrGrokMediaImageIdempotencyUnavailable, err)
	}
	if claimed == nil || claimed.RequestHash != requestHash || claimed.UpstreamIdempotencyKey != record.UpstreamIdempotencyKey {
		return nil, ErrGrokMediaImageIdempotencyConflict
	}
	return claimed, nil
}

func (s *OpenAIGatewayService) BindGrokMediaImageCreateAccount(ctx context.Context, record *GrokMediaImageCreateRecord, accountID int64) (int64, error) {
	if s == nil || s.grokMediaImageCreateRepo == nil || record == nil || accountID <= 0 {
		return 0, ErrGrokMediaImageIdempotencyUnavailable
	}
	boundID, err := s.grokMediaImageCreateRepo.BindImageCreateAccount(ctx, *record, accountID)
	if err != nil || boundID <= 0 {
		if err == nil {
			err = ErrGrokMediaImageIdempotencyUnavailable
		}
		return 0, fmt.Errorf("bind grok image create account: %w", err)
	}
	record.AccountID = boundID
	return boundID, nil
}

func (s *OpenAIGatewayService) ReleaseGrokMediaImageCreateAccount(ctx context.Context, record *GrokMediaImageCreateRecord, accountID int64) error {
	if s == nil || s.grokMediaImageCreateRepo == nil || record == nil || accountID <= 0 {
		return ErrGrokMediaImageIdempotencyUnavailable
	}
	released, err := s.grokMediaImageCreateRepo.ReleaseImageCreateAccount(ctx, *record, accountID)
	if err != nil {
		return fmt.Errorf("release rejected grok image create account: %w", err)
	}
	if !released {
		return ErrGrokMediaImageIdempotencyConflict
	}
	record.AccountID = 0
	return nil
}

func (s *OpenAIGatewayService) CompleteGrokMediaImageCreate(ctx context.Context, record *GrokMediaImageCreateRecord, accountID int64, statusCode int, contentType string, body []byte) error {
	if s == nil || s.grokMediaImageCreateRepo == nil || record == nil || accountID <= 0 || statusCode < 200 || statusCode >= 300 || body == nil {
		return ErrGrokMediaImageIdempotencyUnavailable
	}
	completion := *record
	completion.AccountID = accountID
	completion.ResponseStatus = statusCode
	completion.ResponseContentType = strings.TrimSpace(contentType)
	completion.ResponseBody = append([]byte(nil), body...)
	if err := s.grokMediaImageCreateRepo.CompleteImageCreate(ctx, completion); err != nil {
		return fmt.Errorf("complete grok image create idempotency: %w", err)
	}
	*record = completion
	return nil
}

func WriteGrokMediaImageCreateReplay(c *gin.Context, record *GrokMediaImageCreateRecord) error {
	if c == nil || c.Writer == nil || c.Writer.Written() || record == nil || !record.Completed() {
		return fmt.Errorf("grok image idempotent replay is incomplete")
	}
	contentType := strings.TrimSpace(record.ResponseContentType)
	if contentType == "" {
		contentType = "application/json"
	}
	c.Header("Idempotent-Replayed", "true")
	c.Data(record.ResponseStatus, contentType, append([]byte(nil), record.ResponseBody...))
	return nil
}

func (s *OpenAIGatewayService) ClaimGrokMediaVideoCreate(
	ctx context.Context,
	groupID *int64,
	endpoint GrokMediaEndpoint,
	idempotencyKey, contentType string,
	body []byte,
	userID, apiKeyID int64,
) (*GrokMediaVideoCreateRecord, error) {
	if s == nil || s.grokMediaVideoOwnerRepo == nil {
		return nil, ErrGrokMediaVideoIdempotencyUnavailable
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return nil, ErrGrokMediaVideoIdempotencyKeyRequired
	}
	if len(idempotencyKey) > grokMediaVideoIdempotencyKeyMaxLen {
		return nil, ErrGrokMediaVideoIdempotencyKeyInvalid
	}
	for _, char := range idempotencyKey {
		if char < 0x21 || char > 0x7e {
			return nil, ErrGrokMediaVideoIdempotencyKeyInvalid
		}
	}
	requestHash, err := CanonicalGrokMediaVideoRequestHash(endpoint, contentType, body)
	if err != nil {
		return nil, err
	}
	keySum := sha256.Sum256([]byte(idempotencyKey))
	keyHash := fmt.Sprintf("%x", keySum[:])
	group := derefGroupID(groupID)
	upstreamSum := sha256.Sum256([]byte(fmt.Sprintf(
		"%d:%d:%d:%s:%s", userID, apiKeyID, group, endpoint, keyHash,
	)))
	record := GrokMediaVideoCreateRecord{
		UserID:                 userID,
		APIKeyID:               apiKeyID,
		GroupID:                group,
		Endpoint:               endpoint,
		IdempotencyKeyHash:     keyHash,
		RequestHash:            requestHash,
		UpstreamIdempotencyKey: "sub2api-grok-video-" + fmt.Sprintf("%x", upstreamSum[:]),
		ExpiresAt:              time.Now().Add(grokMediaVideoCreateIdempotencyTTL),
	}
	claimed, err := s.grokMediaVideoOwnerRepo.ClaimVideoCreate(ctx, record)
	if err != nil {
		if errors.Is(err, ErrGrokMediaVideoIdempotencyConflict) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", ErrGrokMediaVideoIdempotencyUnavailable, err)
	}
	if claimed == nil || claimed.RequestHash != requestHash || claimed.UpstreamIdempotencyKey != record.UpstreamIdempotencyKey {
		return nil, ErrGrokMediaVideoIdempotencyConflict
	}
	return claimed, nil
}

func (s *OpenAIGatewayService) BindGrokMediaVideoCreateAccount(
	ctx context.Context,
	record *GrokMediaVideoCreateRecord,
	accountID int64,
) (int64, error) {
	if s == nil || s.grokMediaVideoOwnerRepo == nil || record == nil || accountID <= 0 {
		return 0, ErrGrokMediaVideoIdempotencyUnavailable
	}
	boundID, err := s.grokMediaVideoOwnerRepo.BindVideoCreateAccount(ctx, *record, accountID)
	if err != nil || boundID <= 0 {
		if err == nil {
			err = ErrGrokMediaVideoIdempotencyUnavailable
		}
		return 0, fmt.Errorf("bind grok video create account: %w", err)
	}
	record.AccountID = boundID
	return boundID, nil
}

func (s *OpenAIGatewayService) ReleaseGrokMediaVideoCreateAccount(
	ctx context.Context,
	record *GrokMediaVideoCreateRecord,
	accountID int64,
) error {
	if s == nil || s.grokMediaVideoOwnerRepo == nil || record == nil || accountID <= 0 {
		return ErrGrokMediaVideoIdempotencyUnavailable
	}
	released, err := s.grokMediaVideoOwnerRepo.ReleaseVideoCreateAccount(ctx, *record, accountID)
	if err != nil {
		return fmt.Errorf("release rejected grok video create account: %w", err)
	}
	if !released {
		return ErrGrokMediaVideoIdempotencyConflict
	}
	record.AccountID = 0
	return nil
}

func (s *OpenAIGatewayService) CompleteGrokMediaVideoCreate(
	ctx context.Context,
	record *GrokMediaVideoCreateRecord,
	requestID string,
	accountID int64,
	statusCode int,
	contentType string,
	body []byte,
) error {
	if s == nil || s.grokMediaVideoOwnerRepo == nil || record == nil || accountID <= 0 || strings.TrimSpace(requestID) == "" || statusCode < 200 || statusCode >= 300 || body == nil {
		return ErrGrokMediaVideoIdempotencyUnavailable
	}
	completion := *record
	completion.AccountID = accountID
	completion.RequestID = strings.TrimSpace(requestID)
	completion.ResponseStatus = statusCode
	completion.ResponseContentType = strings.TrimSpace(contentType)
	completion.ResponseBody = append([]byte(nil), body...)
	owner := GrokMediaVideoRequestOwner{
		RequestID: completion.RequestID,
		UserID:    completion.UserID,
		APIKeyID:  completion.APIKeyID,
		GroupID:   completion.GroupID,
		AccountID: accountID,
		ExpiresAt: time.Now().Add(grokMediaVideoRequestOwnerRecoveryWindow),
	}
	if err := s.grokMediaVideoOwnerRepo.CompleteVideoCreate(ctx, completion, owner); err != nil {
		return fmt.Errorf("complete grok video create idempotency: %w", err)
	}
	*record = completion
	if s.cache != nil {
		cacheKey := grokMediaVideoRequestOwnerCacheKey(owner.RequestID, owner.UserID, owner.APIKeyID, owner.GroupID)
		if cacheKey != "" {
			_ = s.cache.SetSessionAccountID(ctx, owner.GroupID, cacheKey, accountID, grokMediaVideoRequestOwnerRecoveryWindow)
		}
	}
	return nil
}

func SetGrokMediaUpstreamIdempotencyKey(c *gin.Context, key string) {
	if c != nil {
		c.Set(grokMediaUpstreamIdempotencyContextKey, strings.TrimSpace(key))
	}
}

func WriteGrokMediaVideoCreateReplay(c *gin.Context, record *GrokMediaVideoCreateRecord) error {
	if c == nil || c.Writer == nil || c.Writer.Written() || record == nil || !record.Completed() {
		return fmt.Errorf("grok media idempotent replay is incomplete")
	}
	contentType := strings.TrimSpace(record.ResponseContentType)
	if contentType == "" {
		contentType = "application/json"
	}
	c.Header("Idempotent-Replayed", "true")
	c.Data(record.ResponseStatus, contentType, append([]byte(nil), record.ResponseBody...))
	return nil
}

func (s *OpenAIGatewayService) BindGrokMediaVideoRequestAccount(
	ctx context.Context,
	groupID *int64,
	requestID string,
	userID, apiKeyID, accountID int64,
) error {
	if s == nil || s.grokMediaVideoOwnerRepo == nil {
		return fmt.Errorf("grok video request owner repository is unavailable")
	}
	requestID = strings.TrimSpace(requestID)
	group := derefGroupID(groupID)
	cacheKey := grokMediaVideoRequestOwnerCacheKey(requestID, userID, apiKeyID, group)
	if cacheKey == "" || accountID <= 0 {
		return fmt.Errorf("grok video request binding is invalid")
	}
	owner := GrokMediaVideoRequestOwner{
		RequestID: requestID,
		UserID:    userID,
		APIKeyID:  apiKeyID,
		GroupID:   group,
		AccountID: accountID,
		ExpiresAt: time.Now().Add(grokMediaVideoRequestOwnerRecoveryWindow),
	}
	if err := s.grokMediaVideoOwnerRepo.Bind(ctx, owner); err != nil {
		return fmt.Errorf("persist grok video request owner binding: %w", err)
	}
	if s.cache != nil {
		// The database commit above is authoritative. Cache failure is therefore
		// best-effort and cannot turn a persisted paid task into a failed create.
		_ = s.cache.SetSessionAccountID(ctx, group, cacheKey, accountID, grokMediaVideoRequestOwnerRecoveryWindow)
	}
	return nil
}

func (s *OpenAIGatewayService) ResolveGrokMediaVideoRequestAccount(
	ctx context.Context,
	groupID *int64,
	requestID string,
	userID, apiKeyID int64,
) (int64, error) {
	if s == nil || s.grokMediaVideoOwnerRepo == nil {
		return 0, fmt.Errorf("grok video request owner repository is unavailable")
	}
	requestID = strings.TrimSpace(requestID)
	group := derefGroupID(groupID)
	cacheKey := grokMediaVideoRequestOwnerCacheKey(requestID, userID, apiKeyID, group)
	if cacheKey == "" {
		return 0, fmt.Errorf("grok video request binding is invalid")
	}
	owner, err := s.grokMediaVideoOwnerRepo.Resolve(
		ctx, requestID, userID, apiKeyID, group,
		time.Now().Add(grokMediaVideoRequestOwnerRecoveryWindow),
	)
	if err != nil {
		if errors.Is(err, ErrGrokMediaVideoRequestOwnerNotFound) {
			return 0, ErrGrokMediaVideoRequestOwnerNotFound
		}
		return 0, fmt.Errorf("resolve grok video request owner binding: %w", err)
	}
	if owner == nil || owner.AccountID <= 0 {
		return 0, ErrGrokMediaVideoRequestOwnerNotFound
	}
	if s.cache != nil {
		remaining := time.Until(owner.ExpiresAt)
		if remaining > 0 {
			_ = s.cache.SetSessionAccountID(ctx, group, cacheKey, owner.AccountID, remaining)
		}
	}
	return owner.AccountID, nil
}

// MarkGrokMediaVideoRequestTerminal starts the bounded terminal-retention
// lifecycle without changing the immutable owner. Cleanup is opportunistic and
// deletes only rows whose recovery window has already elapsed.
func (s *OpenAIGatewayService) MarkGrokMediaVideoRequestTerminal(
	ctx context.Context,
	groupID *int64,
	requestID string,
	userID, apiKeyID int64,
) error {
	if s == nil || s.grokMediaVideoOwnerRepo == nil {
		return fmt.Errorf("grok video request owner repository is unavailable")
	}
	requestID = strings.TrimSpace(requestID)
	group := derefGroupID(groupID)
	if grokMediaVideoRequestOwnerCacheKey(requestID, userID, apiKeyID, group) == "" {
		return fmt.Errorf("grok video terminal owner binding is invalid")
	}
	now := time.Now().UTC()
	if err := s.grokMediaVideoOwnerRepo.MarkTerminal(
		ctx, requestID, userID, apiKeyID, group, now,
		now.Add(grokMediaVideoTerminalRetention),
	); err != nil {
		return fmt.Errorf("mark grok video request owner terminal: %w", err)
	}
	s.CleanupGrokMediaExpiredRecords(ctx)
	return nil
}

// CleanupGrokMediaExpiredRecords performs one bounded, opportunistic batch for
// each durable Grok media record type. Repository failures are intentionally
// reduced to fixed classification fields so response payloads and SQL values
// can never be reflected into logs or client responses.
func (s *OpenAIGatewayService) CleanupGrokMediaExpiredRecords(ctx context.Context) GrokMediaExpiredCleanupStats {
	startedAt := time.Now()
	now := startedAt.UTC()
	stats := GrokMediaExpiredCleanupStats{}
	log := logger.FromContext(ctx)
	cleanup := func(recordType string, deleteBatch func() (int64, error)) int64 {
		deleted, err := deleteBatch()
		if err != nil {
			log.Warn("grok_media.expired_cleanup_failed",
				zap.String("record_type", recordType),
				zap.String("failure", "repository_error"),
			)
			return 0
		}
		return deleted
	}
	if s != nil && s.grokMediaVideoOwnerRepo != nil {
		stats.OwnerDeleted = cleanup("video_owner", func() (int64, error) {
			return s.grokMediaVideoOwnerRepo.DeleteExpired(ctx, now, grokMediaExpiredCleanupBatchSize)
		})
		stats.VideoCreateDeleted = cleanup("video_create", func() (int64, error) {
			return s.grokMediaVideoOwnerRepo.DeleteExpiredVideoCreates(ctx, now, grokMediaExpiredCleanupBatchSize)
		})
	}
	if s != nil && s.grokMediaImageCreateRepo != nil {
		stats.ImageCreateDeleted = cleanup("image_create", func() (int64, error) {
			return s.grokMediaImageCreateRepo.DeleteExpired(ctx, now, grokMediaExpiredCleanupBatchSize)
		})
	}
	stats.Duration = time.Since(startedAt)
	log.Info("grok_media.expired_cleanup_completed",
		zap.Int64("owner_deleted", stats.OwnerDeleted),
		zap.Int64("video_create_deleted", stats.VideoCreateDeleted),
		zap.Int64("image_create_deleted", stats.ImageCreateDeleted),
		zap.Duration("duration", stats.Duration),
	)
	return stats
}

// SelectGrokMediaVideoRequestOwner strictly acquires the account that created
// an asynchronous video request. It bypasses the general scheduler so health
// scores, TTFT, error rate, and spare capacity cannot move a lookup.
func (s *OpenAIGatewayService) SelectGrokMediaVideoRequestOwner(
	ctx context.Context,
	groupID *int64,
	accountID int64,
) (*AccountSelectionResult, error) {
	if s == nil || s.accountRepo == nil || accountID <= 0 {
		return nil, ErrGrokMediaVideoRequestOwnerUnavailable
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return nil, fmt.Errorf("%w: account %d", ErrGrokMediaVideoRequestOwnerUnavailable, accountID)
	}
	if !s.openAIAccountMatchesSchedulingGroup(account, groupID) ||
		normalizeOpenAICompatiblePlatform(account.Platform) != PlatformGrok ||
		!account.IsSchedulable() {
		return nil, fmt.Errorf("%w: account %d", ErrGrokMediaVideoRequestOwnerUnavailable, accountID)
	}

	maxConcurrency := account.SchedulingSlotConcurrency()
	result, err := s.tryAcquireAccountSlot(ctx, account.ID, maxConcurrency)
	if err != nil {
		return nil, fmt.Errorf("%w: acquire account %d: %v", ErrGrokMediaVideoRequestOwnerUnavailable, accountID, err)
	}
	if result != nil && result.Acquired {
		return &AccountSelectionResult{
			Account:     account,
			Acquired:    true,
			ReleaseFunc: result.ReleaseFunc,
		}, nil
	}

	cfg := s.schedulingConfig()
	return &AccountSelectionResult{
		Account:  account,
		Acquired: false,
		WaitPlan: &AccountWaitPlan{
			AccountID:      account.ID,
			MaxConcurrency: maxConcurrency,
			Timeout:        cfg.StickySessionWaitTimeout,
			MaxWaiting:     cfg.StickySessionMaxWaiting,
		},
	}, nil
}

func (s *OpenAIGatewayService) ForwardGrokMedia(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	endpoint GrokMediaEndpoint,
	requestID string,
	body []byte,
	contentType string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	if account == nil {
		return nil, fmt.Errorf("grok account is required")
	}
	if account.Platform != PlatformGrok {
		return nil, fmt.Errorf("account platform %s is not supported for grok media", account.Platform)
	}

	token, _, err := s.getRequestCredential(ctx, c, account)
	if err != nil {
		return nil, err
	}
	if endpoint == GrokMediaEndpointVideoContent {
		return s.forwardGrokMediaVideoContent(ctx, c, account, token, requestID, startTime)
	}
	targetURL, err := buildGrokMediaURL(account, s.cfg, endpoint, requestID)
	if err != nil {
		return nil, err
	}

	body, contentType, err = prepareGrokMediaForwardBody(endpoint, body, contentType)
	if err != nil {
		return nil, err
	}
	body, contentType, err = normalizeGrokMediaForwardBody(endpoint, body, contentType)
	if err != nil {
		return nil, err
	}
	requestInfo := ParseGrokMediaRequest(contentType, body)
	upstreamModel := requestInfo.Model
	if endpoint.RequiresRequestBody() && gjson.ValidBytes(body) {
		if mappedModel := strings.TrimSpace(account.GetMappedModel(requestInfo.Model)); mappedModel != "" {
			upstreamModel = mappedModel
		}
		if upstreamModel != requestInfo.Model {
			body, err = sjson.SetBytes(body, "model", upstreamModel)
			if err != nil {
				return nil, fmt.Errorf("rewrite grok media account mapped model: %w", err)
			}
		}
	}
	body, contentType, err = sanitizeGrokMediaForwardBody(endpoint, body, contentType)
	if err != nil {
		return nil, err
	}

	var bodyReader io.Reader
	if endpoint.RequiresRequestBody() {
		bodyReader = bytes.NewReader(body)
	}
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, endpoint.httpMethod(), targetURL, bodyReader)
	if err != nil {
		return nil, err
	}
	upstreamReq.Header.Set("Authorization", "Bearer "+token)
	upstreamReq.Header.Set("Accept", "application/json")
	if account.IsGrokOAuth() && isGrokCLIProxyTarget(targetURL) {
		applyGrokCLIHeaders(upstreamReq.Header)
	}
	if endpoint.RequiresRequestBody() {
		contentType = strings.TrimSpace(contentType)
		if contentType == "" {
			contentType = "application/json"
		}
		upstreamReq.Header.Set("Content-Type", contentType)
	}
	// 账号级请求头覆写最后应用，配置值优先于内置默认头。
	account.ApplyHeaderOverrides(upstreamReq.Header)
	if endpoint.IsGenerationRequest() && c != nil {
		if value, ok := c.Get(grokMediaUpstreamIdempotencyContextKey); ok {
			if idempotencyKey, ok := value.(string); ok && strings.TrimSpace(idempotencyKey) != "" {
				// The durable, scope-derived key must win over account header
				// overrides. Replaying it against the persisted account closes
				// the upstream-accepted/local-response-lost crash gap.
				upstreamReq.Header.Set("Idempotency-Key", strings.TrimSpace(idempotencyKey))
			}
		}
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = resp.Body.Close() }()

	requestIDHeader := firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id"))
	requestModel := requestInfo.Model
	if resp.StatusCode >= 400 {
		return s.handleGrokMediaErrorResponse(ctx, resp, c, account, requestIDHeader, requestModel)
	}

	s.updateGrokUsageFromResponse(ctx, account, resp.Header, resp.StatusCode)
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	if endpoint == GrokMediaEndpointImagesGenerations || endpoint == GrokMediaEndpointImagesEdits {
		if countOpenAIResponseImageOutputsFromJSONBytes(respBody) <= 0 {
			setOpsUpstreamError(c, http.StatusBadGateway, "xAI upstream returned no image output", truncateString(string(respBody), 512))
			return nil, &UpstreamFailoverError{
				StatusCode:      http.StatusBadGateway,
				ResponseBody:    respBody,
				ResponseHeaders: resp.Header.Clone(),
			}
		}
	}
	grokMediaVideoTerminal := endpoint == GrokMediaEndpointVideoStatus && grokMediaVideoStatusTerminal(respBody)
	if endpoint == GrokMediaEndpointVideoStatus {
		respBody = rewriteGrokMediaVideoContentURLs(
			respBody,
			requestID,
			grokMediaContentProxyURL(c, requestID),
		)
	}
	usage := grokMediaUsageFromResponse(endpoint, requestInfo, respBody)
	if endpoint.IsVideoGenerationRequest() {
		if strings.TrimSpace(usage.ResponseID) == "" {
			setOpsUpstreamError(c, http.StatusBadGateway, "xAI upstream returned no video request id", truncateString(string(respBody), 512))
			return nil, &UpstreamFailoverError{
				StatusCode:      http.StatusBadGateway,
				ResponseBody:    respBody,
				ResponseHeaders: resp.Header.Clone(),
			}
		}
	}
	if endpoint.IsVideoGenerationRequest() || (endpoint.IsImageGenerationRequest() && hasGrokMediaUpstreamIdempotencyKey(c)) {
		bufferGrokMediaResponse(c, resp, respBody, s.responseHeaderFilter)
	} else {
		writeGrokMediaResponse(c, resp, respBody, s.responseHeaderFilter)
	}
	return &OpenAIForwardResult{
		RequestID:              requestIDHeader,
		ResponseID:             usage.ResponseID,
		Usage:                  usage.Usage,
		Model:                  requestModel,
		BillingModel:           requestModel,
		UpstreamModel:          upstreamModel,
		ResponseHeaders:        resp.Header.Clone(),
		Duration:               time.Since(startTime),
		ImageCount:             usage.ImageCount,
		ImageSize:              usage.ImageSize,
		ImageInputSize:         usage.ImageInputSize,
		ImageOutputSizes:       usage.ImageOutputSizes,
		VideoCount:             usage.VideoCount,
		VideoResolution:        usage.VideoResolution,
		GrokMediaVideoTerminal: grokMediaVideoTerminal,
		VideoDurationSeconds:   usage.VideoDurationSeconds,
	}, nil
}

func hasGrokMediaUpstreamIdempotencyKey(c *gin.Context) bool {
	if c == nil {
		return false
	}
	value, ok := c.Get(grokMediaUpstreamIdempotencyContextKey)
	if !ok {
		return false
	}
	key, ok := value.(string)
	return ok && strings.TrimSpace(key) != ""
}

func grokMediaVideoStatusTerminal(body []byte) bool {
	switch strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "status").String())) {
	case "completed", "succeeded", "success", "done", "failed", "error", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

func (s *OpenAIGatewayService) forwardGrokMediaVideoContent(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token, requestID string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	statusURL, err := buildGrokMediaURL(account, s.cfg, GrokMediaEndpointVideoStatus, requestID)
	if err != nil {
		return nil, err
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	statusReq, err := http.NewRequestWithContext(
		WithHTTPUpstreamRedirectsDisabled(upstreamCtx),
		http.MethodGet,
		statusURL,
		nil,
	)
	if err != nil {
		return nil, err
	}
	statusReq.Header.Set("Authorization", "Bearer "+token)
	statusReq.Header.Set("Accept", "application/json")
	if account.IsGrokOAuth() && isGrokCLIProxyTarget(statusURL) {
		applyGrokCLIHeaders(statusReq.Header)
	}
	account.ApplyHeaderOverrides(statusReq.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	statusResp, err := s.httpUpstream.Do(statusReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	statusRequestID := firstNonEmpty(statusResp.Header.Get("x-request-id"), statusResp.Header.Get("xai-request-id"))
	if statusResp.StatusCode >= 300 {
		defer func() { _ = statusResp.Body.Close() }()
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		if statusResp.StatusCode < 400 {
			return nil, fmt.Errorf("grok media status redirect is not allowed")
		}
		return s.handleGrokMediaErrorResponse(ctx, statusResp, c, account, statusRequestID, "")
	}
	statusBody, err := ReadUpstreamResponseBody(statusResp.Body, s.cfg, c, openAITooLargeError)
	_ = statusResp.Body.Close()
	if err != nil {
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		return nil, err
	}

	contentURL, err := grokMediaSignedVideoContentURL(statusBody, requestID)
	if err != nil {
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		return nil, err
	}
	signedContent := contentURL != ""
	if !signedContent {
		contentURL, err = buildGrokMediaURL(account, s.cfg, GrokMediaEndpointVideoContent, requestID)
		if err != nil {
			SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
			return nil, err
		}
	}

	contentReq, err := http.NewRequestWithContext(
		WithHTTPUpstreamRedirectsDisabled(upstreamCtx),
		http.MethodGet,
		contentURL,
		nil,
	)
	if err != nil {
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		return nil, err
	}
	contentReq.Header.Set("Accept", "*/*")
	if c != nil {
		if rangeHeader := strings.TrimSpace(c.GetHeader("Range")); rangeHeader != "" {
			contentReq.Header.Set("Range", rangeHeader)
		}
	}
	if !signedContent {
		contentReq.Header.Set("Authorization", "Bearer "+token)
		if account.IsGrokOAuth() && isGrokCLIProxyTarget(contentURL) {
			applyGrokCLIHeaders(contentReq.Header)
		}
		account.ApplyHeaderOverrides(contentReq.Header)
	}

	contentResp, err := s.httpUpstream.Do(contentReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = contentResp.Body.Close() }()
	contentRequestID := firstNonEmpty(contentResp.Header.Get("x-request-id"), contentResp.Header.Get("xai-request-id"), statusRequestID)
	if contentResp.StatusCode >= 300 && contentResp.StatusCode < 400 {
		return nil, fmt.Errorf("grok media signed content redirect is not allowed")
	}
	if contentResp.StatusCode >= 400 && contentResp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		return s.handleGrokMediaErrorResponse(ctx, contentResp, c, account, contentRequestID, "")
	}

	s.updateGrokUsageFromResponse(ctx, account, contentResp.Header, contentResp.StatusCode)
	if err := writeGrokMediaContentResponse(c, contentResp); err != nil {
		return nil, err
	}
	return &OpenAIForwardResult{
		RequestID:              contentRequestID,
		ResponseHeaders:        contentResp.Header.Clone(),
		Duration:               time.Since(startTime),
		GrokMediaVideoTerminal: true,
	}, nil
}

func grokMediaSignedVideoContentURL(body []byte, requestID string) (string, error) {
	rawURL := strings.TrimSpace(gjson.GetBytes(body, "video.url").String())
	if rawURL == "" {
		return "", nil
	}
	// An upstream Sub2API rewrites protected content URLs to its own proxy
	// endpoint. Treat that as an authenticated relay path, not as a signed URL;
	// the caller will rebuild it against the configured account base URL and
	// attach the upstream API key.
	if isGrokMediaVideoContentURL(rawURL, requestID) {
		return "", nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") ||
		!strings.EqualFold(parsed.Hostname(), "vidgen.x.ai") ||
		(parsed.Port() != "" && parsed.Port() != "443") || parsed.User != nil {
		return "", fmt.Errorf("grok media status returned an unsupported video content URL")
	}
	return parsed.String(), nil
}

func isGrokCLIProxyTarget(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	return err == nil && strings.EqualFold(parsed.Hostname(), "cli-chat-proxy.grok.com")
}

func prepareGrokMediaForwardBody(endpoint GrokMediaEndpoint, body []byte, contentType string) ([]byte, string, error) {
	if endpoint != GrokMediaEndpointImagesEdits || gjson.ValidBytes(body) {
		return body, contentType, nil
	}
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return body, contentType, nil
	}

	info := ParseGrokMediaRequest(contentType, body)
	payload := make(map[string]any)
	if info.Model != "" {
		payload["model"] = info.Model
	}
	if info.Prompt != "" {
		payload["prompt"] = info.Prompt
	}
	if info.N > 1 {
		payload["n"] = info.N
	}
	if info.Size != "" {
		payload["size"] = info.Size
	}

	images := make([]map[string]string, 0, len(info.InputImageURLs)+len(info.Uploads))
	for _, imageURL := range info.InputImageURLs {
		if imageURL = strings.TrimSpace(imageURL); imageURL != "" {
			images = append(images, map[string]string{"url": imageURL})
		}
	}
	for _, upload := range info.Uploads {
		dataURL, err := openAIImageUploadToDataURL(upload)
		if err != nil {
			return nil, "", err
		}
		images = append(images, map[string]string{"url": dataURL})
	}
	if len(images) > 0 {
		payload["image"] = images[0]
		if len(images) > 1 {
			payload["images"] = images
		}
	}

	maskImageURL := strings.TrimSpace(info.MaskImageURL)
	if info.MaskUpload != nil {
		dataURL, err := openAIImageUploadToDataURL(*info.MaskUpload)
		if err != nil {
			return nil, "", err
		}
		maskImageURL = dataURL
	}
	if maskImageURL != "" {
		payload["mask"] = map[string]string{"url": maskImageURL}
	}

	out, err := marshalOpenAIUpstreamJSON(payload)
	if err != nil {
		return nil, "", err
	}
	return out, "application/json", nil
}

func normalizeGrokMediaForwardBody(endpoint GrokMediaEndpoint, body []byte, contentType string) ([]byte, string, error) {
	if !endpoint.RequiresRequestBody() || !gjson.ValidBytes(body) {
		return body, contentType, nil
	}
	var imageFields []string
	switch endpoint {
	case GrokMediaEndpointImagesEdits:
		imageFields = []string{"image", "images", "mask"}
	case GrokMediaEndpointVideosGenerations:
		imageFields = []string{"image", "images", "reference_images"}
	}
	var err error
	body, err = canonicalizeGrokMediaImageURLFields(body, imageFields...)
	if err != nil {
		return nil, "", err
	}
	info := ParseGrokMediaRequest(contentType, body)
	upstreamModel := NormalizeGrokMediaModelForEndpoint(endpoint, info.Model, info.HasInputImage())
	if upstreamModel == "" || upstreamModel == info.Model {
		return body, contentType, nil
	}
	out, err := sjson.SetBytes(body, "model", upstreamModel)
	if err != nil {
		return nil, "", fmt.Errorf("rewrite grok media model: %w", err)
	}
	return out, contentType, nil
}

func canonicalizeGrokMediaImageURLFields(body []byte, fields ...string) ([]byte, error) {
	out := body
	for _, field := range fields {
		value := gjson.GetBytes(out, field)
		if !value.Exists() {
			continue
		}
		if value.IsArray() {
			for index := range value.Array() {
				var err error
				out, err = canonicalizeGrokMediaImageURLObject(out, fmt.Sprintf("%s.%d", field, index))
				if err != nil {
					return nil, err
				}
			}
			continue
		}
		var err error
		out, err = canonicalizeGrokMediaImageURLObject(out, field)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func canonicalizeGrokMediaImageURLObject(body []byte, path string) ([]byte, error) {
	legacyPath := path + ".image_url"
	legacy := gjson.GetBytes(body, legacyPath)
	if !legacy.Exists() {
		return body, nil
	}

	out := body
	if strings.TrimSpace(gjson.GetBytes(out, path+".url").String()) == "" {
		var err error
		out, err = sjson.SetBytes(out, path+".url", legacy.Value())
		if err != nil {
			return nil, fmt.Errorf("normalize grok media image url: %w", err)
		}
	}
	out, err := sjson.DeleteBytes(out, legacyPath)
	if err != nil {
		return nil, fmt.Errorf("remove legacy grok media image url: %w", err)
	}
	return out, nil
}

func sanitizeGrokMediaForwardBody(endpoint GrokMediaEndpoint, body []byte, contentType string) ([]byte, string, error) {
	if !endpoint.RequiresRequestBody() || !gjson.ValidBytes(body) {
		return body, contentType, nil
	}
	switch endpoint {
	case GrokMediaEndpointImagesGenerations, GrokMediaEndpointImagesEdits:
		if !gjson.GetBytes(body, "size").Exists() {
			return body, contentType, nil
		}
		out, err := sjson.DeleteBytes(body, "size")
		if err != nil {
			return nil, "", fmt.Errorf("sanitize grok media size: %w", err)
		}
		return out, contentType, nil
	default:
		return body, contentType, nil
	}
}

func (r GrokMediaRequestInfo) HasInputImage() bool {
	return len(r.InputImageURLs) > 0 || len(r.Uploads) > 0
}

// NormalizeGrokMediaModelForEndpoint resolves the built-in upstream model alias
// for a media endpoint before account-level model mapping and scheduling.
func NormalizeGrokMediaModelForEndpoint(endpoint GrokMediaEndpoint, model string, hasInputImage bool) string {
	model = strings.TrimSpace(model)
	switch endpoint {
	case GrokMediaEndpointImagesGenerations, GrokMediaEndpointImagesEdits:
		if model == "grok-imagine" {
			return "grok-imagine-image-quality"
		}
	case GrokMediaEndpointVideosGenerations:
		if model == "grok-imagine-video-1.5" && !hasInputImage {
			return "grok-imagine-video"
		}
	}
	return model
}

type grokMediaUsageMetadata struct {
	ResponseID           string
	Usage                OpenAIUsage
	ImageCount           int
	ImageSize            string
	ImageInputSize       string
	ImageOutputSizes     []string
	VideoCount           int
	VideoResolution      string
	VideoDurationSeconds int
}

func grokMediaUsageFromResponse(endpoint GrokMediaEndpoint, requestInfo GrokMediaRequestInfo, responseBody []byte) grokMediaUsageMetadata {
	usage, _ := extractOpenAIUsageFromJSONBytes(responseBody)
	meta := grokMediaUsageMetadata{Usage: usage}
	switch endpoint {
	case GrokMediaEndpointImagesGenerations, GrokMediaEndpointImagesEdits:
		meta.ImageCount = countOpenAIResponseImageOutputsFromJSONBytes(responseBody)
		meta.ImageSize = requestInfo.SizeTier
		meta.ImageInputSize = requestInfo.Size
		meta.ImageOutputSizes = collectOpenAIResponseImageOutputSizesFromJSONBytes(responseBody)
	case GrokMediaEndpointVideosGenerations, GrokMediaEndpointVideosEdits, GrokMediaEndpointVideosExtensions:
		meta.ResponseID = extractGrokMediaVideoRequestID(responseBody)
		meta.VideoCount = 1
		meta.VideoResolution = requestInfo.Resolution
		meta.VideoDurationSeconds = requestInfo.DurationSeconds
		// Keep the legacy media-unit counter populated for existing usage displays.
		meta.ImageCount = 1
	}
	return meta
}

func extractGrokMediaVideoRequestID(body []byte) string {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return ""
	}
	for _, path := range []string{"request_id", "id", "data.request_id", "data.id", "video.request_id", "video.id"} {
		if id := strings.TrimSpace(gjson.GetBytes(body, path).String()); id != "" {
			return id
		}
	}
	return ""
}

func (s *OpenAIGatewayService) handleGrokMediaErrorResponse(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	requestIDHeader string,
	requestedModel string,
) (*OpenAIForwardResult, error) {
	body := s.readUpstreamErrorBody(resp)
	// Reconcile readiness before configurable passthrough branches can return;
	// otherwise a Grok 429 can remain schedulable.
	s.handleGrokAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, body)
	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	if upstreamMsg == "" {
		upstreamMsg = fmt.Sprintf("xAI upstream returned status %d", resp.StatusCode)
	}

	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
	if isGrokContentPolicyRejection(resp.StatusCode, body) {
		clientMsg := grokContentPolicyClientMessage(body)
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  requestIDHeader,
			Kind:               "http_error",
			Message:            clientMsg,
			Detail:             upstreamDetail,
		})
		MarkResponseCommitted(c)
		writeGrokMediaErrorResponse(c, http.StatusForbidden, "invalid_request_error", clientMsg)
		return nil, fmt.Errorf("grok content policy rejection: %s", clientMsg)
	}

	if status, errType, errMsg, matched := applyErrorPassthroughRule(
		c,
		account.Platform,
		resp.StatusCode,
		body,
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed",
	); matched {
		MarkResponseCommitted(c)
		writeGrokMediaErrorResponse(c, status, errType, errMsg)
		return nil, fmt.Errorf("upstream error: %d (passthrough rule matched) message=%s", resp.StatusCode, upstreamMsg)
	}

	if !account.ShouldHandleErrorCode(resp.StatusCode) {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  requestIDHeader,
			Kind:               "http_error",
			Message:            upstreamMsg,
			Detail:             upstreamDetail,
		})
		MarkResponseCommitted(c)
		writeGrokMediaErrorResponse(c, http.StatusInternalServerError, "upstream_error", "Upstream gateway error")
		return nil, fmt.Errorf("upstream error: %d (not in custom error codes) message=%s", resp.StatusCode, upstreamMsg)
	}

	kind := "http_error"
	if s.shouldFailoverGrokUpstreamError(resp.StatusCode, body) {
		kind = "failover"
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  requestIDHeader,
		Kind:               kind,
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
	})
	if kind == "failover" {
		return nil, &UpstreamFailoverError{
			StatusCode:             resp.StatusCode,
			ResponseBody:           body,
			ResponseHeaders:        resp.Header.Clone(),
			RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
		}
	}

	MarkResponseCommitted(c)
	writeGrokMediaErrorResponse(c, resp.StatusCode, grokMediaErrorType(resp.StatusCode), upstreamMsg)
	return nil, fmt.Errorf("upstream error: %d %s", resp.StatusCode, upstreamMsg)
}

func grokMediaErrorType(statusCode int) string {
	switch statusCode {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	default:
		return "upstream_error"
	}
}

func writeGrokMediaErrorResponse(c *gin.Context, statusCode int, errType, message string) {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return
	}
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    strings.TrimSpace(errType),
			"message": strings.TrimSpace(message),
		},
	})
}

func writeGrokMediaResponse(c *gin.Context, resp *http.Response, body []byte, filter *responseheaders.CompiledHeaderFilter) {
	if c == nil || resp == nil {
		return
	}
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, filter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, body)
}

func bufferGrokMediaResponse(c *gin.Context, resp *http.Response, body []byte, filter *responseheaders.CompiledHeaderFilter) {
	if c == nil || resp == nil {
		return
	}
	header := make(http.Header)
	writeOpenAIPassthroughResponseHeaders(header, resp.Header, filter)
	c.Set(grokMediaBufferedResponseContextKey, &grokMediaBufferedResponse{
		statusCode: resp.StatusCode,
		header:     header,
		body:       append([]byte(nil), body...),
	})
}

// CommitBufferedGrokMediaResponse writes a successful asynchronous-video
// create response only after its durable owner binding has committed.
func CommitBufferedGrokMediaResponse(c *gin.Context) error {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return fmt.Errorf("grok media response cannot be committed")
	}
	value, ok := c.Get(grokMediaBufferedResponseContextKey)
	buffered, ok := value.(*grokMediaBufferedResponse)
	if !ok || buffered == nil {
		return fmt.Errorf("grok media buffered response is missing")
	}
	for name, values := range buffered.header {
		for _, value := range values {
			c.Writer.Header().Add(name, value)
		}
	}
	contentType := strings.TrimSpace(buffered.header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	c.Set(grokMediaBufferedResponseContextKey, nil)
	c.Data(buffered.statusCode, contentType, buffered.body)
	return nil
}

func BufferedGrokMediaResponse(c *gin.Context) (int, string, []byte, error) {
	if c == nil {
		return 0, "", nil, fmt.Errorf("grok media buffered response is missing")
	}
	value, ok := c.Get(grokMediaBufferedResponseContextKey)
	buffered, ok := value.(*grokMediaBufferedResponse)
	if !ok || buffered == nil {
		return 0, "", nil, fmt.Errorf("grok media buffered response is missing")
	}
	contentType := strings.TrimSpace(buffered.header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
	}
	return buffered.statusCode, contentType, append([]byte(nil), buffered.body...), nil
}

func writeGrokMediaContentResponse(c *gin.Context, resp *http.Response) error {
	if c == nil || resp == nil || resp.Body == nil {
		return fmt.Errorf("grok media content response is incomplete")
	}

	for _, name := range []string{
		"Content-Type",
		"Content-Length",
		"Content-Range",
		"Accept-Ranges",
		"Content-Disposition",
	} {
		if value := strings.TrimSpace(resp.Header.Get(name)); value != "" {
			c.Header(name, value)
		}
	}
	if strings.TrimSpace(c.Writer.Header().Get("Content-Length")) == "" && resp.ContentLength >= 0 {
		c.Header("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	}
	if strings.TrimSpace(c.Writer.Header().Get("Content-Type")) == "" {
		c.Header("Content-Type", "application/octet-stream")
	}
	c.Status(resp.StatusCode)
	MarkResponseCommitted(c)
	_, err := io.Copy(c.Writer, resp.Body)
	return err
}

func rewriteGrokMediaVideoContentURLs(body []byte, requestID, proxyURL string) []byte {
	if len(body) == 0 || strings.TrimSpace(requestID) == "" || strings.TrimSpace(proxyURL) == "" || !gjson.ValidBytes(body) {
		return body
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return body
	}
	changed := rewriteGrokMediaKnownVideoURL(&value, proxyURL)
	if rewriteGrokMediaVideoContentURLValue(&value, requestID, proxyURL) {
		changed = true
	}
	if !changed {
		return body
	}
	rewritten, err := json.Marshal(value)
	if err != nil {
		return body
	}
	return rewritten
}

func rewriteGrokMediaKnownVideoURL(value *any, proxyURL string) bool {
	if value == nil {
		return false
	}
	root, ok := (*value).(map[string]any)
	if !ok {
		return false
	}
	video, ok := root["video"].(map[string]any)
	if !ok {
		return false
	}
	rawURL, ok := video["url"].(string)
	if !ok || strings.TrimSpace(rawURL) == "" {
		return false
	}
	video["url"] = proxyURL
	return true
}

func rewriteGrokMediaVideoContentURLValue(value *any, requestID, proxyURL string) bool {
	if value == nil {
		return false
	}
	switch typed := (*value).(type) {
	case map[string]any:
		changed := false
		for key, child := range typed {
			childValue := child
			if rewriteGrokMediaVideoContentURLValue(&childValue, requestID, proxyURL) {
				typed[key] = childValue
				changed = true
			}
		}
		return changed
	case []any:
		changed := false
		for index, child := range typed {
			childValue := child
			if rewriteGrokMediaVideoContentURLValue(&childValue, requestID, proxyURL) {
				typed[index] = childValue
				changed = true
			}
		}
		return changed
	case string:
		if isGrokMediaVideoContentURL(typed, requestID) {
			*value = proxyURL
			return true
		}
	}
	return false
}

func isGrokMediaVideoContentURL(rawURL, requestID string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Path == "" {
		return false
	}
	segments := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(segments) < 3 {
		return false
	}
	requestID = strings.Trim(requestID, "/")
	decodedID, err := url.PathUnescape(segments[len(segments)-2])
	if err != nil {
		return false
	}
	return segments[len(segments)-3] == "videos" &&
		decodedID == requestID &&
		segments[len(segments)-1] == "content"
}

func grokMediaContentProxyURL(c *gin.Context, requestID string) string {
	if c == nil || c.Request == nil || c.Request.URL == nil || strings.TrimSpace(requestID) == "" {
		return ""
	}
	pathPrefix := ""
	if strings.HasPrefix(c.Request.URL.Path, "/v1/") {
		pathPrefix = "/v1"
	}
	return pathPrefix + "/videos/" + url.PathEscape(strings.Trim(requestID, "/")) + "/content"
}
