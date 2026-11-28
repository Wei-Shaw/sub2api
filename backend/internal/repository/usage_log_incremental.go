package repository

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type usageCursorScanner struct {
	rows          *sql.Rows
	transactionID string
}

func (s *usageCursorScanner) Scan(dest ...any) error {
	return s.rows.Scan(append(dest, &s.transactionID)...)
}

func (r *usageLogRepository) ListIncremental(ctx context.Context, cursor service.UsageCursor, limit int) (*service.IncrementalUsagePage, error) {
	// xmin is the oldest active transaction in this statement's snapshot. A lower
	// transaction cannot insert another row after this snapshot, so advancing the
	// cursor cannot skip an insert that commits out of order or arrives late.
	query := "SELECT " + usageLogSelectColumns + `, ingestion_txid::text
 FROM usage_logs
 WHERE (ingestion_txid, id) > ($1::xid8, $2)
   AND ingestion_txid < pg_snapshot_xmin(pg_current_snapshot())
 ORDER BY ingestion_txid, id LIMIT $3`
	rows, err := r.sql.QueryContext(ctx, query, strconv.FormatUint(cursor.TransactionID, 10), cursor.ID, limit+1)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	page := &service.IncrementalUsagePage{Records: make([]service.UsageLog, 0, limit), NextCursor: cursor}
	for rows.Next() {
		if len(page.Records) == limit {
			page.HasMore = true
			break
		}
		scanner := &usageCursorScanner{rows: rows}
		log, err := scanUsageLog(scanner)
		if err != nil {
			return nil, err
		}
		txid, err := strconv.ParseUint(scanner.transactionID, 10, 64)
		if err != nil {
			return nil, err
		}
		page.Records = append(page.Records, *log)
		page.NextCursor = service.UsageCursor{TransactionID: txid, ID: log.ID}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return page, nil
}
