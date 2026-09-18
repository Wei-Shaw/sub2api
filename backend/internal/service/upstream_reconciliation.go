package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	billingMaxConnections = 100
	billingMaxBindings    = 100
	billingMaxBills       = 10000
	billingMaxAllocations = 100000
	billingSyncTimeout    = 150 * time.Second
)

var (
	ErrBillingNotFound    = infraerrors.NotFound("UPSTREAM_BILLING_NOT_FOUND", "upstream billing connection not found")
	ErrBillingBusy        = infraerrors.Conflict("UPSTREAM_BILLING_BUSY", "this upstream billing connection is being synchronized or edited; try again later")
	ErrBillingStorage     = infraerrors.InternalServer("UPSTREAM_BILLING_STORAGE", "upstream billing storage operation failed")
	ErrBillingEncryption  = infraerrors.BadRequest("UPSTREAM_BILLING_ENCRYPTION", "a fixed TOTP_ENCRYPTION_KEY must be configured before saving or using billing credentials")
	ErrBillingCredentials = infraerrors.BadRequest("UPSTREAM_BILLING_CREDENTIALS", "billing credentials could not be decrypted; restore the original encryption key or replace all credentials")
	ErrBillingSync        = infraerrors.BadRequest("UPSTREAM_BILLING_SYNC", "provider billing synchronization failed; check the read-only billing permissions, provider settings and network connection")
)

type BillingBinding struct {
	ResourceID string `json:"resource_id"`
	AccountID  int64  `json:"account_id"`
}

type BillingConnection struct {
	ID                 int64             `json:"id"`
	Name               string            `json:"name"`
	Provider           string            `json:"provider"`
	Settings           map[string]string `json:"settings"`
	Enabled            bool              `json:"enabled"`
	SyncIntervalHours  int               `json:"sync_interval_hours"`
	SyncLookbackMonths int               `json:"sync_lookback_months"`
	AllocationMode     string            `json:"allocation_mode"`
	LastSyncedAt       *time.Time        `json:"last_synced_at"`
	NextSyncAt         time.Time         `json:"next_sync_at"`
	LastError          string            `json:"last_error"`
	HasCredentials     bool              `json:"has_credentials"`
	Bindings           []BillingBinding  `json:"bindings"`
}

type BillingAccountOption struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

type SaveBillingConnectionInput struct {
	Name               string            `json:"name"`
	Provider           string            `json:"provider"`
	Settings           map[string]string `json:"settings"`
	Secrets            map[string]string `json:"secrets"`
	Enabled            bool              `json:"enabled"`
	SyncIntervalHours  int               `json:"sync_interval_hours"`
	SyncLookbackMonths *int              `json:"sync_lookback_months"`
	AllocationMode     string            `json:"allocation_mode"`
	Bindings           []BillingBinding  `json:"bindings"`
}

type BillingReportItem struct {
	ConnectionID       int64     `json:"connection_id,omitempty"`
	ConnectionName     string    `json:"connection_name,omitempty"`
	Provider           string    `json:"provider,omitempty"`
	ResourceID         string    `json:"resource_id,omitempty"`
	AccountID          int64     `json:"account_id,omitempty"`
	AccountName        string    `json:"account_name,omitempty"`
	APIKeyID           int64     `json:"api_key_id"`
	APIKeyName         string    `json:"api_key_name"`
	UserID             int64     `json:"user_id,omitempty"`
	Requests           int64     `json:"requests"`
	Tokens             int64     `json:"tokens"`
	LocalCost          string    `json:"local_cost"`
	AllocatedCost      string    `json:"allocated_cost"`
	Currency           string    `json:"currency"`
	Method             string    `json:"method"`
	PeriodStart        time.Time `json:"period_start"`
	PeriodEnd          time.Time `json:"period_end"`
	OfficialTokens     *int64    `json:"official_tokens,omitempty"`
	LocalMatchedTokens *int64    `json:"local_matched_tokens,omitempty"`
	TokenStatus        string    `json:"token_status"`
}

type BillingReportBill struct {
	SourceID           int64              `json:"source_id"`
	HasRawSource       bool               `json:"has_raw_source"`
	ID                 string             `json:"id"`
	ConnectionID       int64              `json:"connection_id"`
	ResourceID         string             `json:"resource_id"`
	Description        string             `json:"description"`
	Amount             string             `json:"amount"`
	AllocatedCost      string             `json:"allocated_cost"`
	UnmatchedCost      string             `json:"unmatched_cost"`
	SourceAmount       string             `json:"source_amount"`
	Currency           string             `json:"currency"`
	PeriodStart        time.Time          `json:"period_start"`
	PeriodEnd          time.Time          `json:"period_end"`
	Usage              *ProviderBillUsage `json:"usage,omitempty"`
	UsageStatus        string             `json:"usage_status"`
	LocalMatchedTokens *int64             `json:"local_matched_tokens,omitempty"`
	TokenDifference    *int64             `json:"token_difference,omitempty"`
	TokenStatus        string             `json:"token_status"`
}

type BillingReportTotal struct {
	Currency      string `json:"currency"`
	OfficialCost  string `json:"official_cost,omitempty"`
	AllocatedCost string `json:"allocated_cost"`
	UnmatchedCost string `json:"unmatched_cost,omitempty"`
}

type BillingReport struct {
	Month   string               `json:"month"`
	Items   []BillingReportItem  `json:"items"`
	Bills   []BillingReportBill  `json:"bills,omitempty"`
	Totals  []BillingReportTotal `json:"totals"`
	IsAdmin bool                 `json:"is_admin"`
}

// UpstreamReconciliationService keeps official invoices separate from the
// gateway's original charging ledger. All allocations are estimates, never
// claims that the provider reported an individual downstream key's cost.
type UpstreamReconciliationService struct {
	db            *sql.DB
	encryptor     SecretEncryptor
	persistentKey bool
	ctx           context.Context
	cancel        context.CancelFunc
	startOnce     sync.Once
	stopOnce      sync.Once
	wg            sync.WaitGroup
	fetch         func(context.Context, BillingProviderConfig, time.Time, time.Time) ([]ProviderBill, error)
	now           func() time.Time
}

func NewUpstreamReconciliationService(db *sql.DB, encryptor SecretEncryptor, cfg *config.Config) *UpstreamReconciliationService {
	ctx, cancel := context.WithCancel(context.Background())
	return &UpstreamReconciliationService{db: db, encryptor: encryptor,
		persistentKey: cfg != nil && cfg.Totp.EncryptionKeyConfigured,
		ctx:           ctx, cancel: cancel, fetch: FetchProviderBills, now: time.Now}
}

func (s *UpstreamReconciliationService) Start() {
	s.startOnce.Do(func() {
		if s.db == nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.runDue()
			ticker := time.NewTicker(time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-s.ctx.Done():
					return
				case <-ticker.C:
					s.runDue()
				}
			}
		}()
	})
}

func (s *UpstreamReconciliationService) Stop() {
	s.stopOnce.Do(s.cancel)
	s.wg.Wait()
}

func (s *UpstreamReconciliationService) runDue() {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM upstream_billing_connections WHERE enabled AND next_sync_at <= NOW() ORDER BY next_sync_at LIMIT $1`, billingMaxConnections)
	if err != nil {
		cancel()
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	rowErr := rows.Err()
	_ = rows.Close()
	cancel()
	if err != nil || rowErr != nil {
		return
	}
	for _, id := range ids {
		if s.ctx.Err() != nil {
			return
		}
		// Configured months are fetched under the same cross-process lock. Late
		// adjustments replace the active revision, never append duplicate costs.
		_ = s.syncConnection(s.ctx, id, "", true)
	}
}

type billingQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

const billingConnectionColumns = `id,name,provider,settings,enabled,sync_interval_hours,last_synced_at,next_sync_at,last_error,(encrypted_secrets <> ''),sync_lookback_months,allocation_mode`

func scanBillingConnection(scanner interface{ Scan(...any) error }) (BillingConnection, error) {
	var c BillingConnection
	var settings []byte
	var last sql.NullTime
	err := scanner.Scan(&c.ID, &c.Name, &c.Provider, &settings, &c.Enabled, &c.SyncIntervalHours, &last, &c.NextSyncAt, &c.LastError, &c.HasCredentials, &c.SyncLookbackMonths, &c.AllocationMode)
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(settings, &c.Settings); err != nil {
		return c, err
	}
	if c.Settings == nil {
		c.Settings = map[string]string{}
	}
	if last.Valid {
		c.LastSyncedAt = &last.Time
	}
	c.Bindings = []BillingBinding{}
	return c, nil
}

func loadBillingConnection(ctx context.Context, q billingQueryer, id int64) (*BillingConnection, error) {
	c, err := scanBillingConnection(q.QueryRowContext(ctx, `SELECT `+billingConnectionColumns+` FROM upstream_billing_connections WHERE id=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBillingNotFound
	}
	if err != nil {
		return nil, ErrBillingStorage
	}
	rows, err := q.QueryContext(ctx, `SELECT resource_id,account_id FROM upstream_billing_bindings WHERE connection_id=$1 ORDER BY account_id`, id)
	if err != nil {
		return nil, ErrBillingStorage
	}
	defer rows.Close()
	for rows.Next() {
		var b BillingBinding
		if err := rows.Scan(&b.ResourceID, &b.AccountID); err != nil {
			return nil, ErrBillingStorage
		}
		c.Bindings = append(c.Bindings, b)
	}
	if rows.Err() != nil {
		return nil, ErrBillingStorage
	}
	return &c, nil
}

func (s *UpstreamReconciliationService) Connections(ctx context.Context) ([]BillingConnection, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+billingConnectionColumns+` FROM upstream_billing_connections ORDER BY id LIMIT $1`, billingMaxConnections+1)
	if err != nil {
		return nil, ErrBillingStorage
	}
	list := []BillingConnection{}
	for rows.Next() {
		c, err := scanBillingConnection(rows)
		if err != nil {
			rows.Close()
			return nil, ErrBillingStorage
		}
		list = append(list, c)
	}
	rowErr := rows.Err()
	rows.Close()
	if rowErr != nil {
		return nil, ErrBillingStorage
	}
	if len(list) > billingMaxConnections {
		return nil, billingInvalid("too many billing connections")
	}
	if len(list) == 0 {
		return list, nil
	}
	bindings, err := s.db.QueryContext(ctx, `SELECT connection_id,resource_id,account_id FROM upstream_billing_bindings ORDER BY connection_id,account_id`)
	if err != nil {
		return nil, ErrBillingStorage
	}
	defer bindings.Close()
	byID := make(map[int64]int, len(list))
	for i := range list {
		byID[list[i].ID] = i
	}
	for bindings.Next() {
		var id int64
		var b BillingBinding
		if err := bindings.Scan(&id, &b.ResourceID, &b.AccountID); err != nil {
			return nil, ErrBillingStorage
		}
		if index, ok := byID[id]; ok {
			list[index].Bindings = append(list[index].Bindings, b)
		}
	}
	if bindings.Err() != nil {
		return nil, ErrBillingStorage
	}
	return list, nil
}

func (s *UpstreamReconciliationService) Accounts(ctx context.Context) ([]BillingAccountOption, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,platform FROM accounts WHERE type='apikey' AND deleted_at IS NULL ORDER BY name,id LIMIT 10001`)
	if err != nil {
		return nil, ErrBillingStorage
	}
	defer rows.Close()
	result := []BillingAccountOption{}
	for rows.Next() {
		var a BillingAccountOption
		if err := rows.Scan(&a.ID, &a.Name, &a.Platform); err != nil {
			return nil, ErrBillingStorage
		}
		result = append(result, a)
	}
	if rows.Err() != nil {
		return nil, ErrBillingStorage
	}
	if len(result) > 10000 {
		return nil, billingInvalid("too many upstream accounts to list")
	}
	return result, nil
}

func billingInvalid(message string) error {
	return infraerrors.BadRequest("UPSTREAM_BILLING_INVALID", message)
}

func billingLockName(id int64) string { return fmt.Sprintf("sub2api:upstream-billing:%d", id) }

func (s *UpstreamReconciliationService) encryptionReady() bool {
	return s.persistentKey && s.encryptor != nil
}

func (s *UpstreamReconciliationService) SaveConnection(ctx context.Context, id int64, ownerID int64, input SaveBillingConnectionInput) (*BillingConnection, error) {
	if !s.encryptionReady() {
		return nil, ErrBillingEncryption
	}
	if id < 0 || ownerID <= 0 {
		return nil, billingInvalid("invalid billing connection owner or identifier")
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 120 {
		return nil, billingInvalid("connection name must contain 1–120 bytes")
	}
	if input.SyncIntervalHours < 1 || input.SyncIntervalHours > 168 {
		return nil, billingInvalid("sync interval must be between 1 and 168 hours")
	}
	lookback, allocationMode, err := resolveBillingPolicy(input, nil)
	if err != nil {
		return nil, err
	}
	if len(input.Bindings) > billingMaxBindings {
		return nil, billingInvalid("a connection supports at most 100 upstream account bindings")
	}
	if len(input.Settings) > 20 || len(input.Secrets) > 10 {
		return nil, billingInvalid("too many provider settings")
	}
	for k, v := range input.Settings {
		if len(k) > 80 || len(v) > 2048 {
			return nil, billingInvalid("provider setting exceeds the length limit")
		}
	}
	for k, v := range input.Secrets {
		if len(k) > 80 || len(v) > 8192 {
			return nil, billingInvalid("provider credential exceeds the length limit")
		}
	}
	if input.Settings == nil {
		input.Settings = map[string]string{}
	}
	if input.Secrets == nil {
		input.Secrets = map[string]string{}
	}
	seen := map[int64]bool{}
	for i := range input.Bindings {
		b := &input.Bindings[i]
		b.ResourceID = strings.TrimSpace(b.ResourceID)
		if input.Provider == "azure" {
			b.ResourceID = strings.ToLower(b.ResourceID)
		}
		if b.AccountID <= 0 || seen[b.AccountID] || b.ResourceID == "" || len(b.ResourceID) > 2048 || b.ResourceID == "*" {
			return nil, billingInvalid("bindings require unique upstream accounts and exact resource identifiers")
		}
		seen[b.AccountID] = true
	}
	sort.Slice(input.Bindings, func(i, j int) bool { return input.Bindings[i].AccountID < input.Bindings[j].AccountID })
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, ErrBillingStorage
	}
	defer tx.Rollback()
	var locked bool
	// Serialize configuration writes, including overlapping resource bindings
	// and the count cap. This lock does not block unrelated invoice fetches.
	if err = tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, "sub2api:upstream-billing:configuration").Scan(&locked); err != nil {
		return nil, ErrBillingStorage
	}
	if !locked {
		return nil, ErrBillingBusy
	}
	if err = tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, billingLockName(id)).Scan(&locked); err != nil {
		return nil, ErrBillingStorage
	}
	if !locked {
		return nil, ErrBillingBusy
	}
	if id > 0 {
		var provider, ciphertext string
		var previous BillingConnection
		var previousSettings []byte
		err = tx.QueryRowContext(ctx, `SELECT provider,encrypted_secrets,sync_lookback_months,allocation_mode,settings FROM upstream_billing_connections WHERE id=$1 FOR UPDATE`, id).Scan(&provider, &ciphertext, &previous.SyncLookbackMonths, &previous.AllocationMode, &previousSettings)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrBillingNotFound
		}
		if err != nil {
			return nil, ErrBillingStorage
		}
		if input.Provider != provider {
			return nil, billingInvalid("the provider of an existing connection cannot be changed")
		}
		if json.Unmarshal(previousSettings, &previous.Settings) != nil {
			return nil, ErrBillingStorage
		}
		if err = validateBillingScopeEdit(provider, previous.Settings, input.Settings); err != nil {
			return nil, err
		}
		lookback, allocationMode, err = resolveBillingPolicy(input, &previous)
		if err != nil {
			return nil, err
		}
		input.Secrets, err = s.resolveBillingSecrets(ciphertext, input)
		if err != nil {
			return nil, err
		}
	} else {
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_billing_connections`).Scan(&count); err != nil {
			return nil, ErrBillingStorage
		}
		if count >= billingMaxConnections {
			return nil, billingInvalid("at most 100 upstream billing connections are supported")
		}
	}
	providerConfig := BillingProviderConfig{Provider: input.Provider, Settings: input.Settings, Secrets: input.Secrets}
	if err := ValidateBillingProviderConfig(providerConfig); err != nil {
		return nil, billingInvalid(err.Error())
	}
	scopeKey := billingScopeKey(providerConfig)
	var duplicateScope bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM upstream_billing_connections WHERE scope_key=$1 AND id<>$2)`, scopeKey, id).Scan(&duplicateScope); err != nil {
		return nil, ErrBillingStorage
	}
	if duplicateScope {
		return nil, infraerrors.Conflict("UPSTREAM_BILLING_SCOPE_CONFLICT", "this provider billing scope already has a connection; edit the existing connection to avoid double counting")
	}
	plaintext, err := json.Marshal(input.Secrets)
	if err != nil {
		return nil, billingInvalid("invalid provider credentials")
	}
	ciphertext, err := s.encryptor.Encrypt(string(plaintext))
	if err != nil {
		return nil, ErrBillingCredentials
	}
	settings, err := json.Marshal(input.Settings)
	if err != nil {
		return nil, billingInvalid("invalid provider settings")
	}
	for _, b := range input.Bindings {
		var valid bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE id=$1 AND type='apikey' AND deleted_at IS NULL)`, b.AccountID).Scan(&valid); err != nil {
			return nil, ErrBillingStorage
		}
		if !valid {
			return nil, billingInvalid("bindings can only reference existing API-key upstream accounts")
		}
		var occupied bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM upstream_billing_bindings WHERE account_id=$1 AND connection_id<>$2)`, b.AccountID, id).Scan(&occupied); err != nil {
			return nil, ErrBillingStorage
		}
		if occupied {
			return nil, infraerrors.Conflict("UPSTREAM_BILLING_BINDING_CONFLICT", "an upstream account is already bound to another billing connection")
		}
		if b.ResourceID != "__organization__" {
			query := `SELECT EXISTS(SELECT 1 FROM upstream_billing_bindings b JOIN upstream_billing_connections c ON c.id=b.connection_id WHERE b.resource_id=$1 AND c.provider=$2 AND c.id<>$3`
			args := []any{b.ResourceID, input.Provider, id}
			if billingHasAccountScope(input.Provider) {
				query += ` AND c.settings->>'account_id'=$4`
				args = append(args, input.Settings["account_id"])
			}
			if err = tx.QueryRowContext(ctx, query+`)`, args...).Scan(&occupied); err != nil {
				return nil, ErrBillingStorage
			}
			if occupied {
				return nil, infraerrors.Conflict("UPSTREAM_BILLING_RESOURCE_CONFLICT", "this provider resource is already mapped in another billing connection")
			}
		}
	}
	if id == 0 {
		err = tx.QueryRowContext(ctx, `INSERT INTO upstream_billing_connections(owner_id,name,provider,settings,encrypted_secrets,enabled,sync_interval_hours,scope_key,sync_lookback_months,allocation_mode) VALUES($1,$2,$3,$4::jsonb,$5,$6,$7,$8,$9,$10) RETURNING id`, ownerID, input.Name, input.Provider, string(settings), ciphertext, input.Enabled, input.SyncIntervalHours, scopeKey, lookback, allocationMode).Scan(&id)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE upstream_billing_connections SET name=$2,settings=$3::jsonb,encrypted_secrets=$4,enabled=$5,sync_interval_hours=$6,next_sync_at=NOW(),last_error='',updated_at=NOW(),scope_key=$7,sync_lookback_months=$8,allocation_mode=$9 WHERE id=$1`, id, input.Name, string(settings), ciphertext, input.Enabled, input.SyncIntervalHours, scopeKey, lookback, allocationMode)
	}
	if err != nil {
		return nil, ErrBillingStorage
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM upstream_billing_bindings WHERE connection_id=$1`, id); err != nil {
		return nil, ErrBillingStorage
	}
	for _, b := range input.Bindings {
		if _, err = tx.ExecContext(ctx, `INSERT INTO upstream_billing_bindings(connection_id,resource_id,account_id) VALUES($1,$2,$3)`, id, b.ResourceID, b.AccountID); err != nil {
			return nil, infraerrors.Conflict("UPSTREAM_BILLING_BINDING_CONFLICT", "could not save account bindings; an account may already be bound to another connection")
		}
	}
	result, err := loadBillingConnection(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, ErrBillingStorage
	}
	return result, nil
}

func mergeBillingSecrets(previous, incoming map[string]string) map[string]string {
	result := make(map[string]string, len(previous)+len(incoming))
	for k, v := range previous {
		result[k] = v
	}
	for k, v := range incoming {
		if v != "" {
			result[k] = v
		}
	}
	return result
}

func (s *UpstreamReconciliationService) resolveBillingSecrets(ciphertext string, input SaveBillingConnectionInput) (map[string]string, error) {
	// Full replacement is intentionally possible after recovery from a lost
	// encryption key. Partial/blank updates still require decrypting old values.
	if ValidateBillingProviderConfig(BillingProviderConfig{Provider: input.Provider, Settings: input.Settings, Secrets: input.Secrets}) == nil {
		return mergeBillingSecrets(nil, input.Secrets), nil
	}
	previous, err := s.decryptCredentials(ciphertext)
	if err != nil {
		return nil, err
	}
	return mergeBillingSecrets(previous, input.Secrets), nil
}

func billingScopeKey(cfg BillingProviderConfig) string {
	var identity string
	switch cfg.Provider {
	case "azure":
		identity = strings.ToLower(strings.TrimSpace(cfg.Settings["resource_id"]))
	case "tencent":
		identity = cfg.Secrets["secret_id"] + "\x00" + cfg.Settings["business_code"]
	case "anthropic":
		identity = cfg.Settings["workspace_id"]
		if identity == "" || identity == "__organization__" {
			identity = cfg.Secrets["admin_api_key"] + "\x00" + identity
		}
	case "aliyun", "volcengine":
		identity = cfg.Settings["account_id"] + "\x00" + cfg.Settings["product_code"]
	}
	sum := sha256.Sum256([]byte(cfg.Provider + "\x00" + identity))
	return hex.EncodeToString(sum[:])
}

func (s *UpstreamReconciliationService) decryptCredentials(ciphertext string) (map[string]string, error) {
	if !s.encryptionReady() {
		return nil, ErrBillingEncryption
	}
	plaintext, err := s.encryptor.Decrypt(ciphertext)
	if err != nil {
		return nil, ErrBillingCredentials
	}
	var secrets map[string]string
	if err = json.Unmarshal([]byte(plaintext), &secrets); err != nil || len(secrets) == 0 {
		return nil, ErrBillingCredentials
	}
	return secrets, nil
}

func billingHasAccountScope(provider string) bool {
	return provider == "aliyun" || provider == "volcengine"
}

func billingLocation(provider string) *time.Location {
	if provider == "tencent" || billingHasAccountScope(provider) {
		return time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return time.UTC
}

func billingMonth(month string, now time.Time, location *time.Location) (time.Time, time.Time, error) {
	if len(month) != 7 {
		return time.Time{}, time.Time{}, billingInvalid("month must be YYYY-MM")
	}
	start, err := time.ParseInLocation("2006-01", month, location)
	if err != nil || start.Format("2006-01") != month || start.Year() < 2000 || start.After(now) {
		return time.Time{}, time.Time{}, billingInvalid("month must be a valid non-future YYYY-MM since 2000")
	}
	return start, start.AddDate(0, 1, 0), nil
}

func (s *UpstreamReconciliationService) Sync(ctx context.Context, id int64, month string) error {
	return s.syncConnection(ctx, id, month, false)
}

func (s *UpstreamReconciliationService) syncConnection(parent context.Context, id int64, month string, scheduled bool) error {
	if !s.encryptionReady() {
		return ErrBillingEncryption
	}
	if id <= 0 {
		return billingInvalid("invalid billing connection identifier")
	}
	budget := billingSyncTimeout
	if scheduled {
		budget *= 6 // One bounded budget per configured month, at most six.
	}
	ctx, cancel := context.WithTimeout(parent, budget)
	defer cancel()
	stopCancel := context.AfterFunc(s.ctx, cancel)
	defer stopCancel()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return ErrBillingStorage
	}
	defer conn.Close()
	var locked bool
	if err = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,0))`, billingLockName(id)).Scan(&locked); err != nil {
		return ErrBillingStorage
	}
	if !locked {
		return ErrBillingBusy
	}
	defer func() {
		unlockCtx, unlockCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer unlockCancel()
		var unlocked bool
		if unlockErr := conn.QueryRowContext(unlockCtx, `SELECT pg_advisory_unlock(hashtextextended($1,0))`, billingLockName(id)).Scan(&unlocked); unlockErr != nil || !unlocked {
			// Never return a session that still owns an advisory lock to the pool.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	c, err := loadBillingConnection(ctx, conn, id)
	if err != nil {
		return err
	}
	if scheduled && (!c.Enabled || c.NextSyncAt.After(s.now())) {
		return nil
	}
	location := billingLocation(c.Provider)
	months := []string{month}
	if scheduled {
		months = billingScheduledMonths(s.now(), location, c.SyncLookbackMonths)
	}
	for _, m := range months {
		if _, _, err = billingMonth(m, s.now(), location); err != nil {
			return err
		}
	}
	var ciphertext string
	if err = conn.QueryRowContext(ctx, `SELECT encrypted_secrets FROM upstream_billing_connections WHERE id=$1`, id).Scan(&ciphertext); err != nil {
		return ErrBillingStorage
	}
	secrets, err := s.decryptCredentials(ciphertext)
	if err != nil {
		s.markBillingSync(conn, id, err)
		return err
	}
	cfg := BillingProviderConfig{Provider: c.Provider, Settings: c.Settings, Secrets: secrets}
	if err = ValidateBillingProviderConfig(cfg); err != nil {
		s.markBillingSync(conn, id, ErrBillingSync)
		return ErrBillingSync
	}
	for _, m := range months {
		start, end, _ := billingMonth(m, s.now(), location)
		monthCtx, monthCancel := context.WithTimeout(ctx, billingSyncTimeout)
		bills, fetchErr := s.fetch(monthCtx, cfg, start, end)
		if fetchErr != nil {
			monthCancel()
			s.markBillingSync(conn, id, ErrBillingSync)
			return ErrBillingSync
		}
		err = s.persistBills(monthCtx, conn, c, m, start, end, bills)
		monthCancel()
		if err != nil {
			s.markBillingSync(conn, id, err)
			return err
		}
	}
	return s.markBillingSync(conn, id, nil)
}

func (s *UpstreamReconciliationService) markBillingSync(conn *sql.Conn, id int64, syncErr error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var err error
	if syncErr == nil {
		_, err = conn.ExecContext(ctx, `UPDATE upstream_billing_connections SET last_synced_at=NOW(),next_sync_at=NOW()+sync_interval_hours*INTERVAL '1 hour',last_error='',updated_at=NOW() WHERE id=$1`, id)
	} else {
		// Store a fixed safe summary, not an HTTP response body or SQL error.
		_, err = conn.ExecContext(ctx, `UPDATE upstream_billing_connections SET next_sync_at=NOW()+INTERVAL '1 hour',last_error=$2,updated_at=NOW() WHERE id=$1`, id, infraerrors.Message(syncErr))
	}
	if err != nil {
		return ErrBillingStorage
	}
	return nil
}

type billingUsageWeight struct {
	AccountID     int64
	AccountName   string
	APIKeyID      int64
	APIKeyName    string
	UserID        int64
	Requests      int64
	Tokens        int64
	LocalCost     string
	MatchedTokens *int64
}

type billingAllocation struct {
	billingUsageWeight
	AllocatedCost  string
	Method         string
	OfficialTokens *int64
	TokenStatus    string
}

type billingBillSnapshot struct {
	Bill         ProviderBill
	SourceAmount string
	Allocations  []billingAllocation
	Evidence     billingUsageEvidence
}

func parseBillingAmount(value string) (decimal.Decimal, error) {
	if len(value) == 0 || len(value) > 80 {
		return decimal.Zero, billingInvalid("provider returned an invalid monetary amount")
	}
	n, err := decimal.NewFromString(value)
	if err != nil || n.Exponent() < -18 || n.Exponent() > 18 || n.Abs().GreaterThan(decimal.New(1, 25)) {
		return decimal.Zero, billingInvalid("provider monetary amount exceeds supported precision")
	}
	return n, nil
}

func normalizeBillingBills(bills []ProviderBill, start, end time.Time) ([]ProviderBill, error) {
	if len(bills) > billingMaxBills {
		return nil, billingInvalid("provider returned too many invoice records; narrow the resource scope")
	}
	result := append([]ProviderBill(nil), bills...)
	seen := make(map[string]bool, len(result))
	rawBytes := 0
	for i := range result {
		b := &result[i]
		if err := normalizeBillingEvidence(b); err != nil {
			return nil, err
		}
		rawBytes += len(b.RawSource)
		if rawBytes > 32<<20 {
			return nil, billingInvalid("provider original invoice data exceeds 32 MiB; narrow the resource scope")
		}
		if b.ID == "" || len(b.ID) > 256 || seen[b.ID] || len(b.ResourceID) > 2048 || len(b.Description) > 2048 {
			return nil, billingInvalid("provider returned invalid or duplicate invoice identifiers")
		}
		seen[b.ID] = true
		if !b.PeriodStart.Before(b.PeriodEnd) || b.PeriodStart.Before(start) || b.PeriodEnd.After(end) {
			return nil, billingInvalid("provider invoice period is outside the requested billing month")
		}
		b.Currency = strings.ToUpper(strings.TrimSpace(b.Currency))
		if len(b.Currency) != 3 {
			return nil, billingInvalid("provider invoice currency is invalid")
		}
		for _, ch := range b.Currency {
			if ch < 'A' || ch > 'Z' {
				return nil, billingInvalid("provider invoice currency is invalid")
			}
		}
		n, err := parseBillingAmount(b.Amount)
		if err != nil {
			return nil, err
		}
		b.Amount = n.String()
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// allocateBillingAmount works in integral 1e-8 currency units. Truncation toward
// zero followed by largest-remainder distribution conserves the rounded bill
// for debits and credits without assigning a negative rounding tail to a debit.
func allocateBillingAmount(amount string, usage []billingUsageWeight) ([]billingAllocation, error) {
	n, err := parseBillingAmount(amount)
	if err != nil {
		return nil, err
	}
	n = n.Round(8)
	weights := make([]decimal.Decimal, len(usage))
	total := decimal.Zero
	method := "local_cost_weighted"
	for i, u := range usage {
		w, err := parseBillingAmount(u.LocalCost)
		if err != nil {
			return nil, err
		}
		if w.IsPositive() {
			weights[i] = w
			total = total.Add(w)
		}
	}
	if total.IsZero() {
		method = "token_weighted"
		for i, u := range usage {
			weights[i] = decimal.Zero
			if u.Tokens > 0 {
				weights[i] = decimal.NewFromInt(u.Tokens)
				total = total.Add(weights[i])
			}
		}
	}
	if total.IsZero() {
		return []billingAllocation{{billingUsageWeight: billingUsageWeight{LocalCost: "0"}, AllocatedCost: n.StringFixed(8), Method: "unmatched"}}, nil
	}
	units := n.Abs().Shift(8)
	parts := make([]decimal.Decimal, len(usage))
	fractions := make([]decimal.Decimal, len(usage))
	allocated := decimal.Zero
	indices := make([]int, 0, len(usage))
	for i, w := range weights {
		if w.IsZero() {
			continue
		}
		// DivRound with extra guard digits leaves at most one unit per row to
		// distribute; inputs and output remain decimal, never binary floats.
		exact := units.Mul(w).DivRound(total, 28)
		parts[i] = exact.Truncate(0)
		fractions[i] = exact.Sub(parts[i])
		allocated = allocated.Add(parts[i])
		indices = append(indices, i)
	}
	sort.SliceStable(indices, func(i, j int) bool { return fractions[indices[i]].GreaterThan(fractions[indices[j]]) })
	remaining := units.Sub(allocated)
	if remaining.IsNegative() || remaining.GreaterThan(decimal.NewFromInt(int64(len(indices)))) {
		return nil, billingInvalid("could not conserve invoice allocation precision")
	}
	for i := int64(0); i < remaining.IntPart(); i++ {
		parts[indices[i]] = parts[indices[i]].Add(decimal.NewFromInt(1))
	}
	result := make([]billingAllocation, 0, len(usage))
	for i, u := range usage {
		part := parts[i].Shift(-8)
		if n.IsNegative() {
			part = part.Neg()
		}
		result = append(result, billingAllocation{billingUsageWeight: u, AllocatedCost: part.StringFixed(8), Method: method})
	}
	return result, nil
}

func billingPeriodKey(resource string, start, end time.Time) string {
	return fmt.Sprintf("%s\x00%s\x00%s", resource, start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano))
}

func billingUsageIdentity(u billingUsageWeight) string {
	return fmt.Sprintf("%d:%d:%d", u.AccountID, u.APIKeyID, u.UserID)
}

// Usage logs are append-only but can be pruned. Retain the last complete input
// snapshot if fewer requests remain on a subsequent invoice correction. This
// keeps monthly reconciliation useful after the usage-retention horizon.
func preserveBillingUsage(current, previous []billingUsageWeight, accountIDs []int64) []billingUsageWeight {
	allowed := map[int64]bool{}
	for _, id := range accountIDs {
		allowed[id] = true
	}
	result := map[string]billingUsageWeight{}
	for _, u := range current {
		if allowed[u.AccountID] {
			result[billingUsageIdentity(u)] = u
		}
	}
	for _, u := range previous {
		if !allowed[u.AccountID] {
			continue
		}
		key := billingUsageIdentity(u)
		if now, ok := result[key]; !ok || now.Requests < u.Requests {
			result[key] = u
		}
	}
	list := make([]billingUsageWeight, 0, len(result))
	for _, u := range result {
		list = append(list, u)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].AccountID != list[j].AccountID {
			return list[i].AccountID < list[j].AccountID
		}
		if list[i].APIKeyID != list[j].APIKeyID {
			return list[i].APIKeyID < list[j].APIKeyID
		}
		return list[i].UserID < list[j].UserID
	})
	return list
}

func loadPriorBillingUsage(ctx context.Context, tx *sql.Tx, connectionID int64, month string) (map[string][]billingUsageWeight, error) {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT ON (b.resource_id,b.period_start,b.period_end,a.account_id,a.api_key_id,a.user_id)
	 b.resource_id,b.period_start,b.period_end,a.account_id,a.account_name,a.api_key_id,a.api_key_name,a.user_id,a.requests,a.tokens,a.local_cost::text
	 FROM upstream_billing_months m JOIN upstream_billing_bills b ON b.run_id=m.active_run_id
	 JOIN upstream_billing_allocations a ON a.bill_id=b.id
	 WHERE m.connection_id=$1 AND m.month=$2 AND a.method<>'unmatched' AND a.local_matched_tokens IS NULL
	 ORDER BY b.resource_id,b.period_start,b.period_end,a.account_id,a.api_key_id,a.user_id,a.requests DESC LIMIT $3`, connectionID, month, billingMaxAllocations+1)
	if err != nil {
		return nil, ErrBillingStorage
	}
	defer rows.Close()
	result := map[string][]billingUsageWeight{}
	count := 0
	for rows.Next() {
		var resource string
		var start, end time.Time
		var u billingUsageWeight
		if err = rows.Scan(&resource, &start, &end, &u.AccountID, &u.AccountName, &u.APIKeyID, &u.APIKeyName, &u.UserID, &u.Requests, &u.Tokens, &u.LocalCost); err != nil {
			return nil, ErrBillingStorage
		}
		count++
		if count > billingMaxAllocations {
			return nil, billingInvalid("saved reconciliation exceeds the allocation limit")
		}
		key := billingPeriodKey(resource, start, end)
		result[key] = append(result[key], u)
	}
	if rows.Err() != nil {
		return nil, ErrBillingStorage
	}
	return result, nil
}

func loadBillingUsage(ctx context.Context, tx *sql.Tx, accountIDs []int64, start, end time.Time) ([]billingUsageWeight, error) {
	if len(accountIDs) == 0 {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT u.account_id,COALESCE(a.name,''),u.api_key_id,COALESCE(k.name,''),u.user_id,
	 COUNT(*),COALESCE(SUM(u.input_tokens::bigint+u.output_tokens::bigint+u.cache_creation_tokens::bigint+u.cache_read_tokens::bigint),0),COALESCE(SUM(u.total_cost),0)::text
	 FROM usage_logs u LEFT JOIN accounts a ON a.id=u.account_id LEFT JOIN api_keys k ON k.id=u.api_key_id
	 WHERE u.account_id=ANY($1) AND u.created_at >= $2 AND u.created_at < $3
	 GROUP BY u.account_id,a.name,u.api_key_id,k.name,u.user_id
	 ORDER BY u.account_id,u.api_key_id,u.user_id LIMIT $4`, pq.Array(accountIDs), start, end, billingMaxAllocations+1)
	if err != nil {
		return nil, ErrBillingStorage
	}
	defer rows.Close()
	result := []billingUsageWeight{}
	for rows.Next() {
		var u billingUsageWeight
		if err = rows.Scan(&u.AccountID, &u.AccountName, &u.APIKeyID, &u.APIKeyName, &u.UserID, &u.Requests, &u.Tokens, &u.LocalCost); err != nil {
			return nil, ErrBillingStorage
		}
		result = append(result, u)
		if len(result) > billingMaxAllocations {
			return nil, billingInvalid("usage exceeds the allocation limit; narrow the resource scope")
		}
	}
	if rows.Err() != nil {
		return nil, ErrBillingStorage
	}
	return result, nil
}

func billingSnapshotFingerprint(bindings []BillingBinding, snapshots []billingBillSnapshot, modes ...string) (string, error) {
	// Callers sort bindings, bills and allocations deterministically. The hash
	// covers usage weights too, so newly-arriving local usage creates a revision.
	data, err := json.Marshal(struct {
		Bindings       []BillingBinding
		Bills          []billingBillSnapshot
		AllocationMode string
	}{bindings, snapshots, billingSnapshotMode(modes...)})
	if err != nil {
		return "", ErrBillingStorage
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func (s *UpstreamReconciliationService) persistBills(ctx context.Context, conn *sql.Conn, c *BillingConnection, month string, start, end time.Time, fetched []ProviderBill) error {
	mode := billingSnapshotMode(c.AllocationMode)
	bills, err := normalizeBillingBills(fetched, start, end)
	if err != nil {
		return err
	}
	// Read committed is intentional: the overlap check takes a fresh snapshot
	// after acquiring the provider-wide lock, so a preceding importer is visible.
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return ErrBillingStorage
	}
	defer tx.Rollback()
	if err = checkBillingImportOverlap(ctx, tx, c, bills); err != nil {
		return err
	}
	if mode != "disabled" {
		if err = checkBillingEvidenceRegression(ctx, tx, c.ID, month, bills); err != nil {
			return err
		}
	}
	prior, err := loadPriorBillingUsage(ctx, tx, c.ID, month)
	if err != nil {
		return err
	}
	resources := map[string][]int64{}
	for _, binding := range c.Bindings {
		resources[binding.ResourceID] = append(resources[binding.ResourceID], binding.AccountID)
	}
	usageCache := map[string][]billingUsageWeight{}
	comparableCache := map[string][]billingUsageWeight{}
	snapshots := make([]billingBillSnapshot, 0, len(bills))
	allocationCount := 0
	for _, b := range bills {
		evidence, comparable := billingEvidenceFor(b)
		var allocations []billingAllocation
		if mode == "disabled" {
			comparable = false
			evidence.TokenStatus = "not_checked"
			allocations, err = allocateBillingAmount(b.Amount, nil)
			if err != nil {
				return err
			}
		}
		if comparable {
			key := billingOfficialUsageKey(b)
			usage, ok := comparableCache[key]
			if !ok {
				usage, err = loadComparableBillingUsage(ctx, tx, resources[b.ResourceID], b)
				if err != nil {
					return err
				}
				previous, priorErr := loadPriorComparableBillingUsage(ctx, tx, c.ID, month, b)
				if priorErr != nil {
					return priorErr
				}
				usage = preserveBillingUsage(usage, previous, resources[b.ResourceID])
				comparableCache[key] = usage
			}
			var local int64
			allocations, local, err = allocateOfficialBillingAmount(b.Amount, b.Usage.Tokens, usage)
			if err != nil {
				return err
			}
			difference := b.Usage.Tokens - local
			evidence.LocalMatchedTokens = &local
			evidence.TokenDifference = &difference
			if local > b.Usage.Tokens {
				evidence.TokenStatus = "local_exceeds_official"
			} else if local == 0 {
				evidence.TokenStatus = "no_local_usage"
			}
		}
		if allocations == nil && mode == "local_weighted" && evidence.TokenStatus != "local_exceeds_official" {
			key := billingPeriodKey(b.ResourceID, b.PeriodStart, b.PeriodEnd)
			usage, ok := usageCache[key]
			if !ok {
				usage, err = loadBillingUsage(ctx, tx, resources[b.ResourceID], b.PeriodStart, b.PeriodEnd)
				if err != nil {
					return err
				}
				usage = preserveBillingUsage(usage, prior[key], resources[b.ResourceID])
				usageCache[key] = usage
			}
			allocations, err = allocateBillingAmount(b.Amount, usage)
			if err != nil {
				return err
			}
		}
		if allocations == nil {
			// An explicit strict policy never distributes unverified cloud costs.
			allocations, err = allocateBillingAmount(b.Amount, nil)
			if err != nil {
				return err
			}
		}
		for i := range allocations {
			allocations[i].TokenStatus = evidence.TokenStatus
			if comparable {
				official := b.Usage.Tokens
				allocations[i].OfficialTokens = &official
			}
		}
		allocationCount += len(allocations)
		if allocationCount > billingMaxAllocations {
			return billingInvalid("invoice exceeds the allocation limit; narrow the resource scope")
		}
		sourceAmount := b.Amount
		amount, _ := parseBillingAmount(b.Amount)
		b.Amount = amount.Round(8).StringFixed(8)
		snapshots = append(snapshots, billingBillSnapshot{Bill: b, SourceAmount: sourceAmount, Allocations: allocations, Evidence: evidence})
	}
	fingerprint, err := billingSnapshotFingerprint(c.Bindings, snapshots, mode)
	if err != nil {
		return err
	}
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT r.fingerprint FROM upstream_billing_months m JOIN upstream_billing_runs r ON r.id=m.active_run_id WHERE m.connection_id=$1 AND m.month=$2`, c.ID, month).Scan(&existing)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ErrBillingStorage
	}
	if existing == fingerprint {
		if tx.Commit() != nil {
			return ErrBillingStorage
		}
		return nil
	}
	bindings, _ := json.Marshal(c.Bindings)
	var runID int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO upstream_billing_runs(connection_id,month,fingerprint,bindings_snapshot,allocation_mode) VALUES($1,$2,$3,$4::jsonb,$5) RETURNING id`, c.ID, month, fingerprint, string(bindings), mode).Scan(&runID); err != nil {
		return ErrBillingStorage
	}
	for _, snapshot := range snapshots {
		b := snapshot.Bill
		evidenceJSON, jsonErr := json.Marshal(snapshot.Evidence)
		if jsonErr != nil {
			return ErrBillingStorage
		}
		var billID int64
		if err = tx.QueryRowContext(ctx, `INSERT INTO upstream_billing_bills(run_id,provider_bill_id,resource_id,description,source_amount,amount,currency,period_start,period_end,raw_source,usage_evidence) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11::jsonb) RETURNING id`, runID, b.ID, b.ResourceID, b.Description, snapshot.SourceAmount, b.Amount, b.Currency, b.PeriodStart, b.PeriodEnd, string(b.RawSource), string(evidenceJSON)).Scan(&billID); err != nil {
			return ErrBillingStorage
		}
		for _, a := range snapshot.Allocations {
			if _, err = tx.ExecContext(ctx, `INSERT INTO upstream_billing_allocations(bill_id,account_id,account_name,api_key_id,api_key_name,user_id,requests,tokens,local_cost,allocated_cost,method,official_tokens,local_matched_tokens,token_status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, billID, a.AccountID, a.AccountName, a.APIKeyID, a.APIKeyName, a.UserID, a.Requests, a.Tokens, a.LocalCost, a.AllocatedCost, a.Method, a.OfficialTokens, a.MatchedTokens, a.TokenStatus); err != nil {
				return ErrBillingStorage
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO upstream_billing_months(connection_id,month,active_run_id) VALUES($1,$2,$3) ON CONFLICT(connection_id,month) DO UPDATE SET active_run_id=EXCLUDED.active_run_id`, c.ID, month, runID); err != nil {
		return ErrBillingStorage
	}
	if err = tx.Commit(); err != nil {
		return ErrBillingStorage
	}
	return nil
}

func (s *UpstreamReconciliationService) Report(ctx context.Context, month string, userID int64, isAdmin bool) (*BillingReport, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// A report spans providers in their own billing-month timezones. The label
	// alone is shared; allocation windows always retain provider timestamps.
	if _, _, err := billingMonth(month, s.now(), billingLocation("tencent")); err != nil {
		return nil, err
	}
	if !isAdmin && userID <= 0 {
		return nil, infraerrors.Forbidden("UPSTREAM_BILLING_FORBIDDEN", "authentication is required")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, ErrBillingStorage
	}
	defer tx.Rollback()
	report := &BillingReport{Month: month, Items: []BillingReportItem{}, Totals: []BillingReportTotal{}, IsAdmin: isAdmin}
	query := `SELECT c.id,c.name,c.provider,b.resource_id,a.account_id,a.account_name,a.api_key_id,a.api_key_name,a.user_id,MAX(a.requests),MAX(a.tokens),MAX(a.local_cost)::text,SUM(a.allocated_cost)::text,b.currency,a.method,b.period_start,b.period_end,MAX(a.official_tokens),MAX(a.local_matched_tokens),a.token_status
	 FROM upstream_billing_months m JOIN upstream_billing_connections c ON c.id=m.connection_id
	 JOIN upstream_billing_bills b ON b.run_id=m.active_run_id JOIN upstream_billing_allocations a ON a.bill_id=b.id
	 WHERE m.month=$1`
	args := []any{month}
	if !isAdmin {
		query += ` AND a.user_id=$2 AND a.method<>'unmatched'`
		args = append(args, userID)
	}
	// Multiple invoice line items may share one resource/time window. Their
	// allocations add, but their repeated usage-weight snapshots must not add.
	query += fmt.Sprintf(` GROUP BY c.id,c.name,c.provider,b.resource_id,a.account_id,a.account_name,a.api_key_id,a.api_key_name,a.user_id,b.currency,a.method,b.period_start,b.period_end,a.token_status,CASE WHEN a.official_tokens IS NOT NULL THEN b.usage_evidence->'usage' ELSE NULL END ORDER BY b.period_start,c.id,b.resource_id,a.account_id,a.api_key_id LIMIT %d`, billingMaxAllocations+1)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, ErrBillingStorage
	}
	for rows.Next() {
		var item BillingReportItem
		if err = rows.Scan(&item.ConnectionID, &item.ConnectionName, &item.Provider, &item.ResourceID, &item.AccountID, &item.AccountName, &item.APIKeyID, &item.APIKeyName, &item.UserID, &item.Requests, &item.Tokens, &item.LocalCost, &item.AllocatedCost, &item.Currency, &item.Method, &item.PeriodStart, &item.PeriodEnd, &item.OfficialTokens, &item.LocalMatchedTokens, &item.TokenStatus); err != nil {
			rows.Close()
			return nil, ErrBillingStorage
		}
		if !isAdmin {
			if item.UserID != userID {
				rows.Close()
				return nil, ErrBillingStorage
			}
			redactBillingItem(&item)
		}
		report.Items = append(report.Items, item)
		if len(report.Items) > billingMaxAllocations {
			rows.Close()
			return nil, billingInvalid("report exceeds the result limit; reduce configured invoice scope")
		}
	}
	rowErr := rows.Err()
	rows.Close()
	if rowErr != nil {
		return nil, ErrBillingStorage
	}
	if isAdmin {
		billRows, err := tx.QueryContext(ctx, `SELECT b.provider_bill_id,m.connection_id,b.resource_id,b.description,b.amount::text,b.source_amount,b.currency,b.period_start,b.period_end,b.id,(b.raw_source<>'{}'::jsonb),b.usage_evidence,totals.allocated::text,totals.unmatched::text FROM upstream_billing_months m JOIN upstream_billing_bills b ON b.run_id=m.active_run_id LEFT JOIN LATERAL (SELECT COALESCE(SUM(allocated_cost) FILTER (WHERE method<>'unmatched'),0) AS allocated,COALESCE(SUM(allocated_cost) FILTER (WHERE method='unmatched'),0) AS unmatched FROM upstream_billing_allocations WHERE bill_id=b.id) totals ON TRUE WHERE m.month=$1 ORDER BY m.connection_id,b.provider_bill_id LIMIT $2`, month, billingMaxBills+1)
		if err != nil {
			return nil, ErrBillingStorage
		}
		report.Bills = []BillingReportBill{}
		for billRows.Next() {
			var b BillingReportBill
			var evidenceJSON []byte
			if err = billRows.Scan(&b.ID, &b.ConnectionID, &b.ResourceID, &b.Description, &b.Amount, &b.SourceAmount, &b.Currency, &b.PeriodStart, &b.PeriodEnd, &b.SourceID, &b.HasRawSource, &evidenceJSON, &b.AllocatedCost, &b.UnmatchedCost); err != nil {
				billRows.Close()
				return nil, ErrBillingStorage
			}
			var evidence billingUsageEvidence
			if json.Unmarshal(evidenceJSON, &evidence) != nil {
				billRows.Close()
				return nil, ErrBillingStorage
			}
			b.Usage = evidence.Usage
			b.UsageStatus = evidence.UsageStatus
			b.TokenStatus = evidence.TokenStatus
			b.LocalMatchedTokens = evidence.LocalMatchedTokens
			b.TokenDifference = evidence.TokenDifference
			report.Bills = append(report.Bills, b)
			if len(report.Bills) > billingMaxBills {
				billRows.Close()
				return nil, billingInvalid("report exceeds the invoice result limit")
			}
		}
		rowErr = billRows.Err()
		billRows.Close()
		if rowErr != nil {
			return nil, ErrBillingStorage
		}
	}
	report.Totals, err = billingReportTotals(report.Items, report.Bills, isAdmin)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, ErrBillingStorage
	}
	return report, nil
}

func redactBillingItem(item *BillingReportItem) {
	item.ConnectionID = 0
	item.ConnectionName = ""
	item.Provider = ""
	item.ResourceID = ""
	item.AccountID = 0
	item.AccountName = ""
	item.UserID = 0
	item.OfficialTokens = nil
}

func billingReportTotals(items []BillingReportItem, bills []BillingReportBill, isAdmin bool) ([]BillingReportTotal, error) {
	type totals struct{ official, allocated, unmatched decimal.Decimal }
	byCurrency := map[string]totals{}
	for _, item := range items {
		n, err := parseBillingAmount(item.AllocatedCost)
		if err != nil {
			return nil, ErrBillingStorage
		}
		t := byCurrency[item.Currency]
		if item.Method == "unmatched" {
			t.unmatched = t.unmatched.Add(n)
		} else {
			t.allocated = t.allocated.Add(n)
		}
		byCurrency[item.Currency] = t
	}
	if isAdmin {
		for _, bill := range bills {
			n, err := parseBillingAmount(bill.Amount)
			if err != nil {
				return nil, ErrBillingStorage
			}
			t := byCurrency[bill.Currency]
			t.official = t.official.Add(n)
			byCurrency[bill.Currency] = t
		}
	}
	currencies := make([]string, 0, len(byCurrency))
	for currency := range byCurrency {
		currencies = append(currencies, currency)
	}
	sort.Strings(currencies)
	result := make([]BillingReportTotal, 0, len(currencies))
	for _, currency := range currencies {
		t := byCurrency[currency]
		row := BillingReportTotal{Currency: currency, AllocatedCost: t.allocated.StringFixed(8)}
		if isAdmin {
			row.OfficialCost = t.official.StringFixed(8)
			row.UnmatchedCost = t.unmatched.StringFixed(8)
		}
		result = append(result, row)
	}
	return result, nil
}
