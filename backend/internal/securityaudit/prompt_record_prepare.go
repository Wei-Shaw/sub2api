package securityaudit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var errPromptRecordInvalidJSON = errors.New("prompt record request JSON is invalid")

type preparedPromptRecord struct {
	StoredBody         []byte
	StoredSnapshot     PromptSnapshot
	OriginalPromptHash string
}

type PromptRecordKey struct {
	ID int64
}

type promptRecordCorrelation struct {
	once      sync.Once
	mu        sync.Mutex
	ready     chan struct{}
	key       PromptRecordKey
	persisted bool
	onReady   func(PromptRecordKey, bool)
}

func newPromptRecordCorrelation() *promptRecordCorrelation {
	return &promptRecordCorrelation{ready: make(chan struct{})}
}

func newPromptRecordingRequestPair(req Request) (Request, Request) {
	correlation := newPromptRecordCorrelation()
	req.recordingCorrelation = correlation
	responseReference := req
	responseReference.Body = nil
	responseReference.Headers = nil
	return req, responseReference
}

func (c *promptRecordCorrelation) complete(key PromptRecordKey, persisted bool) {
	if c == nil {
		return
	}
	c.once.Do(func() {
		c.mu.Lock()
		c.key = key
		c.persisted = persisted
		close(c.ready)
		callback := c.onReady
		c.onReady = nil
		c.mu.Unlock()
		if callback != nil {
			callback(key, persisted)
		}
	})
}

func (c *promptRecordCorrelation) whenReady(callback func(PromptRecordKey, bool)) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	select {
	case <-c.ready:
		key, persisted := c.key, c.persisted
		c.mu.Unlock()
		callback(key, persisted)
		return true
	default:
	}
	if c.onReady != nil {
		c.mu.Unlock()
		return false
	}
	c.onReady = callback
	c.mu.Unlock()
	return true
}

func (c *promptRecordCorrelation) result() (PromptRecordKey, bool) {
	if c == nil {
		return PromptRecordKey{}, false
	}
	select {
	case <-c.ready:
		return c.key, c.persisted
	default:
		return PromptRecordKey{}, false
	}
}

func (c *promptRecordCorrelation) wait(timeout time.Duration) (PromptRecordKey, bool) {
	if c == nil {
		return PromptRecordKey{}, false
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-c.ready:
		return c.key, c.persisted
	case <-timer.C:
		return PromptRecordKey{}, false
	}
}

func preparePromptRecord(req Request) (preparedPromptRecord, error) {
	if req.recordingSkipPrompt && req.recordingIdentity != "" {
		snapshot := promptRecordMetadata(req)
		snapshot.PromptHash = req.recordingIdentity
		return preparedPromptRecord{StoredSnapshot: snapshot, OriginalPromptHash: req.recordingIdentity}, nil
	}
	document, err := decodePromptDocument(req.Body)
	if err != nil {
		return preparedPromptRecord{}, errPromptRecordInvalidJSON
	}
	originalSnapshot := promptRecordMetadataDocument(req, document, req.Body, false)
	prepared := preparedPromptRecord{OriginalPromptHash: originalSnapshot.PromptHash}
	if req.recordingSkipPrompt {
		prepared.StoredSnapshot = originalSnapshot
		prepared.StoredSnapshot.FullPrompt = ""
		prepared.StoredSnapshot.PromptLength = 0
		prepared.StoredSnapshot.MessageCount = 0
		return prepared, nil
	}

	storedDocument := sanitizePromptRecordDocument(req.Protocol, document, promptRecordFilterOptions{
		Enabled: req.recordingFilterPreset, AgentPreset: req.recordingFilterAgent, Skills: req.recordingFilterSkills,
	})
	prepared.StoredBody, err = json.Marshal(storedDocument)
	if err != nil {
		return preparedPromptRecord{}, err
	}
	prepared.StoredSnapshot = promptRecordSnapshotDocument(req, storedDocument, prepared.StoredBody)
	return prepared, nil
}

func promptRecordSnapshotDocument(req Request, document any, fallbackBody []byte) PromptSnapshot {
	return promptRecordMetadataDocument(req, document, fallbackBody, true)
}

// Recording does not use guard previews or scan payloads. Keep its identity
// compatible with auditing without running redaction or materializing scan text.
func promptRecordMetadataDocument(req Request, document any, fallbackBody []byte, includeText bool) PromptSnapshot {
	snapshot := promptRecordMetadata(req)
	segments := normalizeSegmentsLatestUserFirst(extractProtocolSegments(req.Protocol, document))
	if len(segments) == 0 {
		sum := sha256.Sum256(fallbackBody)
		snapshot.PromptHash = hex.EncodeToString(sum[:])
		return snapshot
	}
	digest := sha256.New()
	var retained strings.Builder
	remaining := DefaultFullPromptMaxRunes
	truncated := false
	for index, segment := range segments {
		if index > 0 {
			_, _ = digest.Write([]byte("\n\n"))
			if includeText {
				snapshot.PromptLength += 2
			}
		}
		_, _ = digest.Write([]byte(segment))
		if !includeText {
			continue
		}
		snapshot.PromptLength += utf8.RuneCountInString(segment)
		if index > 0 && remaining > 0 {
			for range 2 {
				if remaining == 0 {
					truncated = true
					break
				}
				retained.WriteByte('\n')
				remaining--
			}
		}
		for _, r := range segment {
			if r == 0 {
				continue
			}
			if remaining == 0 {
				truncated = true
				break
			}
			retained.WriteRune(r)
			remaining--
		}
	}
	snapshot.PromptHash = hex.EncodeToString(digest.Sum(nil))
	if includeText {
		snapshot.MessageCount = len(segments)
		if truncated {
			retained.WriteRune('…')
		}
		snapshot.FullPrompt = retained.String()
	}
	return snapshot
}

func promptRecordMetadata(req Request) PromptSnapshot {
	return PromptSnapshot{
		RequestID: req.RequestID, UserID: req.UserID, UsernameSnapshot: req.Username,
		UserEmailSnapshot: req.UserEmail, APIKeyID: req.APIKeyID, APIKeyNameSnapshot: req.APIKeyName,
		GroupID: cloneInt64Ptr(req.GroupID), GroupName: req.GroupName, Provider: req.Provider,
		Endpoint: req.Endpoint, Protocol: req.Protocol, Model: req.Model, Stage: ifEmpty(req.Stage, "http"),
	}
}

func promptRecordFallbackSnapshot(req Request, body []byte) PromptSnapshot {
	sum := sha256.Sum256(body)
	return PromptSnapshot{
		RequestID: req.RequestID, UserID: req.UserID, UsernameSnapshot: req.Username,
		UserEmailSnapshot: req.UserEmail, APIKeyID: req.APIKeyID, APIKeyNameSnapshot: req.APIKeyName,
		GroupID: cloneInt64Ptr(req.GroupID), GroupName: req.GroupName, Provider: req.Provider,
		Endpoint: req.Endpoint, Protocol: req.Protocol, Model: req.Model,
		Stage: ifEmpty(req.Stage, "http"), PromptHash: hex.EncodeToString(sum[:]),
	}
}
