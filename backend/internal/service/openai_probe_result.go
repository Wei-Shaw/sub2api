package service

import (
	"context"
	"errors"
	"io"
	"time"
)

type openAIProbeVerdict uint8

const (
	openAIProbeVerdictUnknown openAIProbeVerdict = iota
	openAIProbeVerdictSupported
	openAIProbeVerdictUnsupported
)

var errOpenAIProbeBodyTooLarge = errors.New("probe response body exceeds limit")

const openAIProbePersistenceTimeout = 3 * time.Second

type openAIProbeExtraUpdater interface {
	UpdateExtra(ctx context.Context, id int64, updates map[string]any) error
}

// persistOpenAIProbeExtra keeps the small diagnostic write independent from
// client cancellation while retaining context values for tracing.
func persistOpenAIProbeExtra(callerCtx context.Context, repo openAIProbeExtraUpdater, accountID int64, updates map[string]any) error {
	if repo == nil || len(updates) == 0 {
		return nil
	}
	baseCtx := context.Background()
	if callerCtx != nil {
		baseCtx = context.WithoutCancel(callerCtx)
	}
	persistCtx, cancel := context.WithTimeout(baseCtx, openAIProbePersistenceTimeout)
	defer cancel()
	return repo.UpdateExtra(persistCtx, accountID, updates)
}

// readOpenAIProbeBody reads one extra byte so a truncated transcript can
// never be mistaken for a complete protocol exchange.
func readOpenAIProbeBody(body io.Reader, maxBytes int64) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return data, err
	}
	if int64(len(data)) > maxBytes {
		return data[:maxBytes], errOpenAIProbeBodyTooLarge
	}
	return data, nil
}
