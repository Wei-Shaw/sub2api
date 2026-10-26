package service

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	openAICompactionSummaryCiphertextPrefix = "s2cmp1.v1."
	openAICompactionSummaryFamilyPrefix     = "s2cmp"
	openAICompactionSummaryPurpose          = "openai.compaction_summary"
	openAICompactionSummaryVersion          = 1
	openAICompactionSummaryMaxSummaryBytes  = 256 * 1024
	openAICompactionSummaryMaxEnvelopeBytes = 2 * 1024 * 1024
	openAICompactionSummaryNonceBytes       = 12
)

var (
	ErrOpenAICompactionSummaryInvalid = errors.New("invalid Sub2API compaction summary payload")
	ErrOpenAICompactionSummaryKey     = errors.New("Sub2API compaction summary key unavailable")
)

type OpenAICompactionOwnerScope struct {
	UserID     int64
	GroupID    int64
	GroupBound bool
}

type openAICompactionSummaryEnvelope struct {
	Version    int    `json:"v"`
	Purpose    string `json:"purpose"`
	UserID     int64  `json:"user_id"`
	GroupID    int64  `json:"group_id,omitempty"`
	GroupBound bool   `json:"group_bound"`
	IssuedAt   int64  `json:"issued_at"`
	Summary    string `json:"summary"`
}

func NewOpenAICompactionOwnerScope(userID int64, groupID *int64) (OpenAICompactionOwnerScope, error) {
	if userID <= 0 {
		return OpenAICompactionOwnerScope{}, fmt.Errorf("%w: authenticated user is missing", ErrOpenAICompactionSummaryInvalid)
	}
	scope := OpenAICompactionOwnerScope{UserID: userID}
	if groupID != nil {
		if *groupID <= 0 {
			return OpenAICompactionOwnerScope{}, fmt.Errorf("%w: invalid group scope", ErrOpenAICompactionSummaryInvalid)
		}
		scope.GroupID = *groupID
		scope.GroupBound = true
	}
	return scope, nil
}

func IsOpenAICompactionSummaryCiphertext(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), openAICompactionSummaryCiphertextPrefix)
}

func looksLikeOpenAICompactionSummaryCiphertext(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), openAICompactionSummaryFamilyPrefix)
}

func SealOpenAICompactionSummary(key []byte, scope OpenAICompactionOwnerScope, summary string, issuedAt time.Time) (string, error) {
	aead, err := newOpenAICompactionSummaryAEAD(key)
	if err != nil {
		return "", err
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return "", fmt.Errorf("%w: summary is empty", ErrOpenAICompactionSummaryInvalid)
	}
	if len([]byte(summary)) > openAICompactionSummaryMaxSummaryBytes {
		return "", fmt.Errorf("%w: summary exceeds %d bytes", ErrOpenAICompactionSummaryInvalid, openAICompactionSummaryMaxSummaryBytes)
	}
	if scope.UserID <= 0 || (scope.GroupBound && scope.GroupID <= 0) {
		return "", fmt.Errorf("%w: owner scope is incomplete", ErrOpenAICompactionSummaryInvalid)
	}
	if issuedAt.IsZero() {
		issuedAt = time.Now().UTC()
	}
	envelope := openAICompactionSummaryEnvelope{
		Version:    openAICompactionSummaryVersion,
		Purpose:    openAICompactionSummaryPurpose,
		UserID:     scope.UserID,
		GroupID:    scope.GroupID,
		GroupBound: scope.GroupBound,
		IssuedAt:   issuedAt.UTC().Unix(),
		Summary:    summary,
	}
	plaintext, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("encode compaction summary envelope: %w", err)
	}
	if len(plaintext) > openAICompactionSummaryMaxEnvelopeBytes {
		return "", fmt.Errorf("%w: encoded summary envelope exceeds %d bytes", ErrOpenAICompactionSummaryInvalid, openAICompactionSummaryMaxEnvelopeBytes)
	}
	nonce := make([]byte, openAICompactionSummaryNonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate compaction summary nonce: %w", err)
	}
	sealed := aead.Seal(nil, nonce, plaintext, openAICompactionSummaryAAD(scope))
	wire := make([]byte, 0, len(nonce)+len(sealed))
	wire = append(wire, nonce...)
	wire = append(wire, sealed...)
	return openAICompactionSummaryCiphertextPrefix + base64.RawURLEncoding.EncodeToString(wire), nil
}

func OpenOpenAICompactionSummary(key []byte, scope OpenAICompactionOwnerScope, ciphertext string) (string, error) {
	ciphertext = strings.TrimSpace(ciphertext)
	if !IsOpenAICompactionSummaryCiphertext(ciphertext) {
		return "", fmt.Errorf("%w: unsupported version or key id", ErrOpenAICompactionSummaryInvalid)
	}
	encoded := strings.TrimPrefix(ciphertext, openAICompactionSummaryCiphertextPrefix)
	if encoded == "" || len(encoded) > (openAICompactionSummaryMaxEnvelopeBytes+64*1024)*4/3 {
		return "", fmt.Errorf("%w: ciphertext size is invalid", ErrOpenAICompactionSummaryInvalid)
	}
	wire, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(wire) <= openAICompactionSummaryNonceBytes {
		return "", fmt.Errorf("%w: ciphertext encoding is invalid", ErrOpenAICompactionSummaryInvalid)
	}
	aead, err := newOpenAICompactionSummaryAEAD(key)
	if err != nil {
		return "", err
	}
	nonce := wire[:openAICompactionSummaryNonceBytes]
	plaintext, err := aead.Open(nil, nonce, wire[openAICompactionSummaryNonceBytes:], openAICompactionSummaryAAD(scope))
	if err != nil {
		return "", fmt.Errorf("%w: authentication failed", ErrOpenAICompactionSummaryInvalid)
	}
	if len(plaintext) > openAICompactionSummaryMaxEnvelopeBytes {
		return "", fmt.Errorf("%w: decoded summary envelope exceeds %d bytes", ErrOpenAICompactionSummaryInvalid, openAICompactionSummaryMaxEnvelopeBytes)
	}
	var envelope openAICompactionSummaryEnvelope
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return "", fmt.Errorf("%w: envelope decoding failed", ErrOpenAICompactionSummaryInvalid)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("%w: trailing envelope data", ErrOpenAICompactionSummaryInvalid)
	}
	if envelope.Version != openAICompactionSummaryVersion || envelope.Purpose != openAICompactionSummaryPurpose ||
		envelope.UserID != scope.UserID || envelope.GroupBound != scope.GroupBound ||
		(scope.GroupBound && envelope.GroupID != scope.GroupID) || envelope.IssuedAt <= 0 {
		return "", fmt.Errorf("%w: owner or purpose mismatch", ErrOpenAICompactionSummaryInvalid)
	}
	summary := strings.TrimSpace(envelope.Summary)
	if summary == "" || len([]byte(summary)) > openAICompactionSummaryMaxSummaryBytes {
		return "", fmt.Errorf("%w: summary size is invalid", ErrOpenAICompactionSummaryInvalid)
	}
	return summary, nil
}

func newOpenAICompactionSummaryAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("%w: expected a 32-byte key", ErrOpenAICompactionSummaryKey)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("initialize compaction summary cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize compaction summary GCM: %w", err)
	}
	return aead, nil
}

func openAICompactionSummaryAAD(scope OpenAICompactionOwnerScope) []byte {
	return []byte(fmt.Sprintf(
		"%s|%s|%d|%d|%t",
		openAICompactionSummaryCiphertextPrefix,
		openAICompactionSummaryPurpose,
		scope.UserID,
		scope.GroupID,
		scope.GroupBound,
	))
}
