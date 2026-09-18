package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/lib/pq"
)

const (
	AdminBillingDefaultPageSize = 10
	AdminBillingMaxPageSize     = 20
	AdminBillingMaxPageBytes    = 2 << 20
	adminBillingCursorMaxBytes  = 8192
	adminBillingQueryTimeout    = 20 * time.Second
)

type AdminBillingFilter struct {
	Month        string `json:"month"`
	ConnectionID int64  `json:"connection_id,omitempty"`
	Provider     string `json:"provider,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
}

type AdminBillingListInput struct {
	AdminBillingFilter
	Limit  int
	Cursor string
}

// AdminBillingRecord contains only immutable successful invoice data and safe
// connection labels. Provider credentials/settings are intentionally absent.
type AdminBillingRecord struct {
	ID                 int64              `json:"id"`
	RunID              int64              `json:"run_id"`
	ProviderBillID     string             `json:"provider_bill_id"`
	ConnectionID       int64              `json:"connection_id"`
	ConnectionName     string             `json:"connection_name"`
	Provider           string             `json:"provider"`
	ResourceID         string             `json:"resource_id"`
	Description        string             `json:"description"`
	Amount             string             `json:"amount"`
	SourceAmount       string             `json:"source_amount"`
	Currency           string             `json:"currency"`
	PeriodStart        time.Time          `json:"period_start"`
	PeriodEnd          time.Time          `json:"period_end"`
	SyncedAt           time.Time          `json:"synced_at"`
	AllocationMode     string             `json:"allocation_mode"`
	RawSource          json.RawMessage    `json:"raw_source"`
	Usage              *ProviderBillUsage `json:"usage,omitempty"`
	UsageStatus        string             `json:"usage_status"`
	TokenStatus        string             `json:"token_status"`
	LocalMatchedTokens *int64             `json:"local_matched_tokens,omitempty"`
	TokenDifference    *int64             `json:"token_difference,omitempty"`
}

type AdminBillingList struct {
	Month      string               `json:"month"`
	SnapshotAt time.Time            `json:"snapshot_at"`
	Items      []AdminBillingRecord `json:"items"`
	Limit      int                  `json:"limit"`
	HasMore    bool                 `json:"has_more"`
	NextCursor string               `json:"next_cursor,omitempty"`
}

type AdminBillingProviderCapability struct {
	Provider        string `json:"provider"`
	Name            string `json:"name"`
	HistoricalBills bool   `json:"historical_bills"`
	RawBills        bool   `json:"raw_bills"`
	UsageEvidence   string `json:"usage_evidence"`
	Timezone        string `json:"timezone,omitempty"`
}

type AdminBillingCapabilities struct {
	Version         int                              `json:"version"`
	Authentication  []string                         `json:"authentication"`
	RequiredFilters []string                         `json:"required_filters"`
	OptionalFilters []string                         `json:"optional_filters"`
	DefaultPageSize int                              `json:"default_page_size"`
	MaxPageSize     int                              `json:"max_page_size"`
	MaxPageBytes    int                              `json:"max_page_bytes"`
	MaxRawRowBytes  int                              `json:"max_raw_row_bytes"`
	Providers       []AdminBillingProviderCapability `json:"providers"`
}

func GetAdminBillingCapabilities() AdminBillingCapabilities {
	return AdminBillingCapabilities{
		Version: 1, Authentication: []string{"x-api-key: administrator API key", "Authorization: Bearer administrator JWT"},
		RequiredFilters: []string{"month"}, OptionalFilters: []string{"connection_id", "provider", "resource_id"},
		DefaultPageSize: AdminBillingDefaultPageSize, MaxPageSize: AdminBillingMaxPageSize, MaxPageBytes: AdminBillingMaxPageBytes, MaxRawRowBytes: 65536,
		Providers: []AdminBillingProviderCapability{
			{Provider: "azure", Name: "Azure OpenAI", HistoricalBills: true, RawBills: true, UsageEvidence: "unsupported", Timezone: "UTC"},
			{Provider: "tencent", Name: "Tencent Hunyuan", HistoricalBills: true, RawBills: true, UsageEvidence: "unsupported", Timezone: "Asia/Shanghai"},
			{Provider: "anthropic", Name: "Anthropic", HistoricalBills: true, RawBills: true, UsageEvidence: "conditional_output", Timezone: "UTC"},
			{Provider: "aliyun", Name: "Alibaba Cloud Bailian (Qwen)", HistoricalBills: true, RawBills: true, UsageEvidence: "unsupported", Timezone: "Asia/Shanghai"},
			{Provider: "volcengine", Name: "Volcengine Ark", HistoricalBills: true, RawBills: true, UsageEvidence: "unsupported", Timezone: "Asia/Shanghai"},
			{Provider: "deepseek", Name: "DeepSeek", HistoricalBills: false, RawBills: false, UsageEvidence: "balance_only"},
		},
	}
}

type adminBillingCursor struct {
	Version    int       `json:"v"`
	FilterHash string    `json:"filter"`
	SnapshotAt time.Time `json:"at"`
	RunIDs     []int64   `json:"runs"`
	AfterID    int64     `json:"after"`
}

func validateAdminBillingFilter(filter AdminBillingFilter, now time.Time) (AdminBillingFilter, error) {
	if _, _, err := billingMonth(filter.Month, now, billingLocation("tencent")); err != nil {
		return filter, err
	}
	if filter.ConnectionID < 0 {
		return filter, billingInvalid("connection_id must be a positive integer when supplied")
	}
	switch filter.Provider {
	case "", "azure", "tencent", "anthropic", "aliyun", "volcengine":
	default:
		return filter, billingInvalid("unsupported historical billing provider")
	}
	if len(filter.ResourceID) > 2048 || strings.ContainsAny(filter.ResourceID, "\x00\r\n") {
		return filter, billingInvalid("invalid resource_id filter")
	}
	if filter.Provider == "azure" {
		filter.ResourceID = strings.ToLower(filter.ResourceID)
	}
	return filter, nil
}

func adminBillingFilterHash(filter AdminBillingFilter) string {
	data, _ := json.Marshal(filter)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func encodeAdminBillingCursor(cursor adminBillingCursor) (string, error) {
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", ErrBillingStorage
	}
	encoded := base64.RawURLEncoding.EncodeToString(data)
	if len(encoded) > adminBillingCursorMaxBytes {
		return "", billingInvalid("billing snapshot exceeds cursor capacity")
	}
	return encoded, nil
}

// A cursor is not an authentication token. Every request goes through admin
// authentication; every referenced revision is checked against the supplied
// month and filters. No encoded revision ID can widen that database predicate.
func decodeAdminBillingCursor(value string, filter AdminBillingFilter, now time.Time) (adminBillingCursor, error) {
	var cursor adminBillingCursor
	invalid := func() (adminBillingCursor, error) {
		return cursor, billingInvalid("invalid billing cursor or cursor does not match the requested filters")
	}
	if len(value) > adminBillingCursorMaxBytes {
		return invalid()
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || !json.Valid(data) {
		return invalid()
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cursor) != nil || cursor.Version != 1 || cursor.FilterHash != adminBillingFilterHash(filter) || cursor.AfterID < 0 || cursor.SnapshotAt.IsZero() || cursor.SnapshotAt.After(now.Add(5*time.Minute)) || len(cursor.RunIDs) == 0 || len(cursor.RunIDs) > billingMaxConnections {
		return invalid()
	}
	previous := int64(0)
	for _, id := range cursor.RunIDs {
		if id <= previous {
			return invalid()
		}
		previous = id
	}
	return cursor, nil
}

const adminBillingColumns = `b.id,r.id,b.provider_bill_id,c.id,c.name,c.provider,b.resource_id,b.description,b.amount::text,b.source_amount,b.currency,b.period_start,b.period_end,r.created_at,r.allocation_mode,b.raw_source,b.usage_evidence`

const adminBillingJoins = ` FROM upstream_billing_bills b JOIN upstream_billing_runs r ON r.id=b.run_id JOIN upstream_billing_connections c ON c.id=r.connection_id `

func scanAdminBillingRecord(scanner interface{ Scan(...any) error }) (AdminBillingRecord, error) {
	var record AdminBillingRecord
	var raw, evidenceData []byte
	err := scanner.Scan(&record.ID, &record.RunID, &record.ProviderBillID, &record.ConnectionID, &record.ConnectionName, &record.Provider, &record.ResourceID, &record.Description, &record.Amount, &record.SourceAmount, &record.Currency, &record.PeriodStart, &record.PeriodEnd, &record.SyncedAt, &record.AllocationMode, &raw, &evidenceData)
	if err != nil {
		return record, err
	}
	if len(raw) > 131072 || len(evidenceData) > 8192 {
		return record, ErrBillingStorage
	}
	var compact bytes.Buffer
	if json.Compact(&compact, raw) != nil || compact.Len() > 65536 {
		return record, ErrBillingStorage
	}
	record.RawSource = json.RawMessage(compact.Bytes())
	var evidence billingUsageEvidence
	if json.Unmarshal(evidenceData, &evidence) != nil {
		return record, ErrBillingStorage
	}
	record.Usage = evidence.Usage
	record.UsageStatus = evidence.UsageStatus
	record.TokenStatus = evidence.TokenStatus
	record.LocalMatchedTokens = evidence.LocalMatchedTokens
	record.TokenDifference = evidence.TokenDifference
	if record.UsageStatus == "" {
		record.UsageStatus = "unsupported"
	}
	if record.TokenStatus == "" {
		record.TokenStatus = "unsupported"
	}
	return record, nil
}

func loadAdminBillingRuns(ctx context.Context, tx *sql.Tx, filter AdminBillingFilter) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT m.active_run_id FROM upstream_billing_months m JOIN upstream_billing_connections c ON c.id=m.connection_id
	 WHERE m.month=$1 AND ($2::bigint=0 OR c.id=$2) AND ($3::text='' OR c.provider=$3)
	 AND ($4::text='' OR EXISTS(SELECT 1 FROM upstream_billing_bills b WHERE b.run_id=m.active_run_id AND b.resource_id=$4))
	 ORDER BY m.active_run_id LIMIT $5`, filter.Month, filter.ConnectionID, filter.Provider, filter.ResourceID, billingMaxConnections+1)
	if err != nil {
		return nil, ErrBillingStorage
	}
	defer rows.Close()
	runIDs := []int64{}
	for rows.Next() {
		var id int64
		if rows.Scan(&id) != nil {
			return nil, ErrBillingStorage
		}
		runIDs = append(runIDs, id)
	}
	if rows.Err() != nil {
		return nil, ErrBillingStorage
	}
	if len(runIDs) > billingMaxConnections {
		return nil, billingInvalid("billing snapshot exceeds the connection limit")
	}
	return runIDs, nil
}

func validateAdminBillingRuns(ctx context.Context, tx *sql.Tx, filter AdminBillingFilter, runIDs []int64) error {
	var count, connections int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COUNT(DISTINCT r.connection_id) FROM upstream_billing_runs r JOIN upstream_billing_connections c ON c.id=r.connection_id
	 WHERE r.id=ANY($1) AND r.month=$2 AND ($3::bigint=0 OR c.id=$3) AND ($4::text='' OR c.provider=$4)
	 AND ($5::text='' OR EXISTS(SELECT 1 FROM upstream_billing_bills b WHERE b.run_id=r.id AND b.resource_id=$5))`, pq.Array(runIDs), filter.Month, filter.ConnectionID, filter.Provider, filter.ResourceID).Scan(&count, &connections)
	if err != nil {
		return ErrBillingStorage
	}
	if count != len(runIDs) || connections != len(runIDs) {
		return billingInvalid("billing cursor references an unavailable or mismatched successful snapshot")
	}
	return nil
}

// AdminBills freezes the set of successful monthly revisions on the first
// page. Later syncs can switch active_run_id without affecting subsequent pages.
func (s *UpstreamReconciliationService) AdminBills(parent context.Context, input AdminBillingListInput) (*AdminBillingList, error) {
	filter, err := validateAdminBillingFilter(input.AdminBillingFilter, s.now())
	if err != nil {
		return nil, err
	}
	limit := input.Limit
	if limit == 0 {
		limit = AdminBillingDefaultPageSize
	}
	if limit < 1 || limit > AdminBillingMaxPageSize {
		return nil, billingInvalid("limit must be between 1 and 20")
	}
	cursor := adminBillingCursor{Version: 1, FilterHash: adminBillingFilterHash(filter), SnapshotAt: s.now().UTC()}
	if input.Cursor != "" {
		cursor, err = decodeAdminBillingCursor(input.Cursor, filter, s.now())
		if err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(parent, adminBillingQueryTimeout)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, ErrBillingStorage
	}
	defer tx.Rollback()
	if input.Cursor == "" {
		cursor.RunIDs, err = loadAdminBillingRuns(ctx, tx, filter)
		if err != nil {
			return nil, err
		}
	} else if err = validateAdminBillingRuns(ctx, tx, filter, cursor.RunIDs); err != nil {
		return nil, err
	}
	result := &AdminBillingList{Month: filter.Month, SnapshotAt: cursor.SnapshotAt, Items: []AdminBillingRecord{}, Limit: limit}
	if len(cursor.RunIDs) == 0 {
		if tx.Commit() != nil {
			return nil, ErrBillingStorage
		}
		return result, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+adminBillingColumns+adminBillingJoins+`
	 WHERE r.id=ANY($1) AND r.month=$2 AND b.id>$3 AND ($4::bigint=0 OR c.id=$4) AND ($5::text='' OR c.provider=$5) AND ($6::text='' OR b.resource_id=$6)
	 ORDER BY b.id LIMIT $7`, pq.Array(cursor.RunIDs), filter.Month, cursor.AfterID, filter.ConnectionID, filter.Provider, filter.ResourceID, limit+1)
	if err != nil {
		return nil, ErrBillingStorage
	}
	defer rows.Close()
	// Leave room for the response wrapper, snapshot metadata and bounded cursor.
	remaining := AdminBillingMaxPageBytes - 16384
	for rows.Next() {
		record, scanErr := scanAdminBillingRecord(rows)
		if scanErr != nil {
			return nil, ErrBillingStorage
		}
		encoded, encodeErr := json.Marshal(record)
		if encodeErr != nil {
			return nil, ErrBillingStorage
		}
		if len(result.Items) >= limit || len(encoded)+1 > remaining {
			result.HasMore = true
			break
		}
		remaining -= len(encoded) + 1
		result.Items = append(result.Items, record)
		cursor.AfterID = record.ID
	}
	if rows.Err() != nil {
		return nil, ErrBillingStorage
	}
	if err = rows.Close(); err != nil {
		return nil, ErrBillingStorage
	}
	if result.HasMore {
		if len(result.Items) == 0 {
			return nil, billingInvalid("a provider source row exceeds the response size limit")
		}
		result.NextCursor, err = encodeAdminBillingCursor(cursor)
		if err != nil {
			return nil, err
		}
	}
	if tx.Commit() != nil {
		return nil, ErrBillingStorage
	}
	return result, nil
}

func (s *UpstreamReconciliationService) AdminBill(parent context.Context, id int64, input AdminBillingFilter) (*AdminBillingRecord, error) {
	if id <= 0 {
		return nil, billingInvalid("invalid bill identifier")
	}
	filter, err := validateAdminBillingFilter(input, s.now())
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, adminBillingQueryTimeout)
	defer cancel()
	record, err := scanAdminBillingRecord(s.db.QueryRowContext(ctx, `SELECT `+adminBillingColumns+adminBillingJoins+`
	 WHERE b.id=$1 AND r.month=$2 AND ($3::bigint=0 OR c.id=$3) AND ($4::text='' OR c.provider=$4) AND ($5::text='' OR b.resource_id=$5)`, id, filter.Month, filter.ConnectionID, filter.Provider, filter.ResourceID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBillingNotFound
	}
	if err != nil {
		return nil, ErrBillingStorage
	}
	return &record, nil
}
