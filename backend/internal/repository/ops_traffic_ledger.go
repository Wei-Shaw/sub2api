package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

const opsTrafficInsertChunkSize = 128

var _ service.OpsTrafficLedgerRepository = (*opsRepository)(nil)

// BatchInsertTrafficEvents is atomic across chunks. A retry reuses event IDs;
// even an ambiguous commit cannot count a request twice.
func (r *opsRepository) BatchInsertTrafficEvents(ctx context.Context, inputs []*service.OpsTrafficEvent) error {
	if len(inputs) == 0 {
		return nil
	}
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	// Validate/encode before opening a transaction. Only typed count/dimension
	// fields can enter the ledger, never arbitrary JSON from a client.
	args := make([][]any, 0, len(inputs))
	for _, input := range inputs {
		if input == nil || input.Traffic == nil || input.CreatedAt.IsZero() {
			return fmt.Errorf("invalid traffic event")
		}
		if _, err := uuid.Parse(input.EventID); err != nil {
			return fmt.Errorf("invalid traffic event ID: %w", err)
		}
		encoded, err := json.Marshal(input.Traffic)
		if err != nil {
			return fmt.Errorf("encode traffic event: %w", err)
		}
		args = append(args, []any{input.EventID, input.RequestID, input.GroupID, input.AccountID, nullableOpsTrafficPlatform(input.Platform), input.CreatedAt, string(encoded)})
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for start := 0; start < len(args); start += opsTrafficInsertChunkSize {
		end := min(start+opsTrafficInsertChunkSize, len(args))
		var query strings.Builder
		_, _ = query.WriteString("INSERT INTO ops_traffic_events (event_id, request_id, group_id, account_id, platform, created_at, traffic) VALUES ")
		values := make([]any, 0, (end-start)*7)
		for i, row := range args[start:end] {
			if i > 0 {
				_ = query.WriteByte(',')
			}
			p := i * 7
			fmt.Fprintf(&query, "($%d,$%d,$%d,$%d,$%d,$%d,$%d::jsonb)", p+1, p+2, p+3, p+4, p+5, p+6, p+7)
			values = append(values, row...)
		}
		_, _ = query.WriteString(" ON CONFLICT (event_id) DO NOTHING")
		if _, err = tx.ExecContext(ctx, query.String(), values...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func nullableOpsTrafficPlatform(platform string) any {
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform == "" {
		return nil
	}
	return platform
}
