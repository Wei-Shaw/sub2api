package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// UsageCursor orders ingestion transactions, then rows within each transaction.
// Transaction IDs are kept as decimal strings to avoid JSON number precision loss.
type UsageCursor struct {
	TransactionID uint64
	ID            int64
}

func ParseUsageCursor(raw string) (UsageCursor, error) {
	if raw == "" {
		return UsageCursor{}, nil
	}
	parts := strings.Split(raw, ":")
	if len(parts) != 3 || parts[0] != "v1" {
		return UsageCursor{}, fmt.Errorf("invalid usage cursor")
	}
	txid, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return UsageCursor{}, fmt.Errorf("invalid usage cursor")
	}
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || id < 0 {
		return UsageCursor{}, fmt.Errorf("invalid usage cursor")
	}
	return UsageCursor{TransactionID: txid, ID: id}, nil
}

func (c UsageCursor) String() string { return fmt.Sprintf("v1:%d:%d", c.TransactionID, c.ID) }

type IncrementalUsagePage struct {
	Records    []UsageLog
	NextCursor UsageCursor
	HasMore    bool
}

// IncrementalUsageRepository is the optional ingestion-ordered export capability.
// Keeping it separate allows statistics-only repositories to implement UsageLogRepository.
type IncrementalUsageRepository interface {
	ListIncremental(context.Context, UsageCursor, int) (*IncrementalUsagePage, error)
}

func (s *UsageService) ListIncremental(ctx context.Context, cursor UsageCursor, limit int) (*IncrementalUsagePage, error) {
	if limit < 1 || limit > 1000 {
		return nil, infraerrors.BadRequest("USAGE_LIMIT_INVALID", "limit must be between 1 and 1000")
	}
	repo, ok := s.usageRepo.(IncrementalUsageRepository)
	if !ok {
		return nil, infraerrors.InternalServer("USAGE_EXPORT_UNAVAILABLE", "incremental usage export is unavailable")
	}
	return repo.ListIncremental(ctx, cursor, limit)
}
