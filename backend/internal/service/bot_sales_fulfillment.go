package service

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var botSalesDeviceCodePattern = regexp.MustCompile(`^DLG-[A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4}$`)

type BotSalesBuyer struct {
	ExternalUserID string `json:"external_user_id,omitempty"`
	Email          string `json:"email,omitempty"`
	TelegramID     string `json:"telegram_id,omitempty"`
}

type BotSalesDeliveryPolicy struct {
	IssueAPIKey string `json:"issue_api_key,omitempty"`
}

// UnmarshalJSON accepts both the current policy string and the legacy boolean
// form used by older bot-sales clients.
func (p *BotSalesDeliveryPolicy) UnmarshalJSON(data []byte) error {
	var raw struct {
		IssueAPIKey any `json:"issue_api_key"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch value := raw.IssueAPIKey.(type) {
	case bool:
		if value {
			p.IssueAPIKey = "always"
		} else {
			p.IssueAPIKey = "never"
		}
	case string:
		p.IssueAPIKey = strings.TrimSpace(value)
	}
	return nil
}

type BotSalesFulfillmentRequest struct {
	IdempotencyKey      string                 `json:"-"`
	ExternalOrderCode   string                 `json:"external_order_code" binding:"required"`
	ExternalOrderItemID string                 `json:"external_order_item_id" binding:"required"`
	Buyer               BotSalesBuyer          `json:"buyer"`
	BalanceAmount       float64                `json:"balance_amount" binding:"required"`
	BalancePackageCode  string                 `json:"balance_package_code,omitempty"`
	GroupID             *int64                 `json:"group_id,omitempty"`
	Quantity            int                    `json:"quantity,omitempty"`
	DeliveryPolicy      BotSalesDeliveryPolicy `json:"delivery_policy,omitempty"`
	IssueDeviceCode     bool                   `json:"issue_device_code"`
	DeviceCode          string                 `json:"device_code,omitempty"`
}

type BotSalesFulfillmentResponse struct {
	FulfillmentID       int64           `json:"fulfillment_id"`
	ExternalOrderCode   string          `json:"external_order_code"`
	ExternalOrderItemID string          `json:"external_order_item_id"`
	UserID              int64           `json:"user_id"`
	Email               string          `json:"email"`
	BalanceAdded        float64         `json:"balance_added"`
	Balance             float64         `json:"balance"`
	BalancePackageCode  string          `json:"balance_package_code,omitempty"`
	GroupID             int64           `json:"group_id,omitempty"`
	APIKey              *BotSalesAPIKey `json:"api_key,omitempty"`
	DeviceLoginCode     string          `json:"device_login_code,omitempty"`
	PaymentOrderID      int64           `json:"payment_order_id"`
	Replayed            bool            `json:"replayed,omitempty"`
}

type BotSalesAPIKey struct {
	ID      int64  `json:"id,omitempty"`
	Key     string `json:"key"`
	GroupID *int64 `json:"group_id,omitempty"`
}

func normalizeBotSalesFulfillmentRequest(input BotSalesFulfillmentRequest) (BotSalesFulfillmentRequest, error) {
	input.ExternalOrderCode = strings.TrimSpace(input.ExternalOrderCode)
	input.ExternalOrderItemID = strings.TrimSpace(input.ExternalOrderItemID)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.Buyer.ExternalUserID = strings.TrimSpace(input.Buyer.ExternalUserID)
	input.Buyer.Email = strings.ToLower(strings.TrimSpace(input.Buyer.Email))
	input.Buyer.TelegramID = strings.TrimSpace(input.Buyer.TelegramID)
	input.DeviceCode = strings.ToUpper(strings.TrimSpace(input.DeviceCode))
	input.BalancePackageCode = strings.TrimSpace(input.BalancePackageCode)
	if input.GroupID != nil && *input.GroupID <= 0 {
		return BotSalesFulfillmentRequest{}, infraerrors.BadRequest("BOT_SALES_GROUP_ID_INVALID", "group_id must be a positive integer")
	}
	policy := strings.ToLower(strings.TrimSpace(input.DeliveryPolicy.IssueAPIKey))
	if policy == "" {
		policy = "always"
	}
	if policy != "always" && policy != "if_missing" && policy != "never" {
		return BotSalesFulfillmentRequest{}, infraerrors.BadRequest("BOT_SALES_API_KEY_POLICY_INVALID", "delivery_policy.issue_api_key must be always, if_missing, or never")
	}
	input.DeliveryPolicy.IssueAPIKey = policy

	switch {
	case input.ExternalOrderCode == "":
		return BotSalesFulfillmentRequest{}, infraerrors.BadRequest("BOT_SALES_ORDER_CODE_REQUIRED", "external_order_code is required")
	case input.ExternalOrderItemID == "":
		return BotSalesFulfillmentRequest{}, infraerrors.BadRequest("BOT_SALES_ORDER_ITEM_REQUIRED", "external_order_item_id is required")
	case !math.IsNaN(input.BalanceAmount) && !math.IsInf(input.BalanceAmount, 0) && input.BalanceAmount > 0:
		// Valid amount.
	default:
		return BotSalesFulfillmentRequest{}, infraerrors.BadRequest("BOT_SALES_BALANCE_AMOUNT_INVALID", "balance_amount must be a finite positive number")
	}

	if input.DeviceCode != "" && !botSalesDeviceCodePattern.MatchString(input.DeviceCode) {
		return BotSalesFulfillmentRequest{}, infraerrors.BadRequest("BOT_SALES_DEVICE_CODE_INVALID", "device_code must match DLG-XXXX-XXXX-XXXX")
	}
	if input.IdempotencyKey == "" {
		return BotSalesFulfillmentRequest{}, infraerrors.BadRequest("BOT_SALES_IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key is required")
	}
	return input, nil
}

func botSalesFulfillmentFingerprint(input BotSalesFulfillmentRequest) (string, error) {
	payload := struct {
		ExternalOrderCode   string        `json:"external_order_code"`
		ExternalOrderItemID string        `json:"external_order_item_id"`
		Buyer               BotSalesBuyer `json:"buyer"`
		BalanceAmount       float64       `json:"balance_amount"`
		BalancePackageCode  string        `json:"balance_package_code,omitempty"`
		GroupID             *int64        `json:"group_id,omitempty"`
		IssueDeviceCode     bool          `json:"issue_device_code"`
		DeviceCode          string        `json:"device_code,omitempty"`
		Quantity            int           `json:"quantity,omitempty"`
		IssueAPIKey         string        `json:"issue_api_key"`
	}{
		ExternalOrderCode:   input.ExternalOrderCode,
		ExternalOrderItemID: input.ExternalOrderItemID,
		Buyer:               input.Buyer,
		BalanceAmount:       input.BalanceAmount,
		BalancePackageCode:  input.BalancePackageCode,
		GroupID:             input.GroupID,
		IssueDeviceCode:     input.IssueDeviceCode,
		DeviceCode:          input.DeviceCode,
		Quantity:            input.Quantity,
		IssueAPIKey:         input.DeliveryPolicy.IssueAPIKey,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// botSalesFulfillmentUserRepository is intentionally narrower than UserRepository.
// Keeping the integration on the existing balance primitives avoids changing the
// upstream provider/accounting paths.
type botSalesFulfillmentUserRepository interface {
	GetByID(context.Context, int64) (*User, error)
	GetByEmail(context.Context, string) (*User, error)
	Create(context.Context, *User) error
	AdjustBalance(context.Context, int64, float64) (BalanceChange, error)
}

type BotSalesFulfillmentService struct {
	client         *dbent.Client
	users          botSalesFulfillmentUserRepository
	devices        UserDeviceRepository
	claimService   *VClawClaimService
	paymentService *PaymentService
	apiKeyService  *APIKeyService
}

func NewBotSalesFulfillmentService(
	client *dbent.Client,
	users UserRepository,
	devices UserDeviceRepository,
	claimService *VClawClaimService,
	paymentService *PaymentService,
	apiKeyService *APIKeyService,
) *BotSalesFulfillmentService {
	return &BotSalesFulfillmentService{
		client:         client,
		users:          users,
		devices:        devices,
		claimService:   claimService,
		paymentService: paymentService,
		apiKeyService:  apiKeyService,
	}
}

// Fulfill is implemented below the HTTP handler so bot-sales and future admin
// tooling share exactly the same validation and accounting contract.
func (s *BotSalesFulfillmentService) Fulfill(ctx context.Context, input BotSalesFulfillmentRequest) (*BotSalesFulfillmentResponse, error) {
	input, err := normalizeBotSalesFulfillmentRequest(input)
	if err != nil {
		return nil, err
	}
	if s == nil || s.client == nil || s.users == nil || s.devices == nil {
		return nil, infraerrors.ServiceUnavailable("BOT_SALES_NOT_CONFIGURED", "bot-sales fulfillment is not configured")
	}
	return s.fulfillInTransaction(ctx, input)
}

type botSalesFulfillmentRecord struct {
	id                 int64
	status             string
	requestFingerprint string
	responseJSON       string
}

func (s *BotSalesFulfillmentService) fulfillInTransaction(ctx context.Context, input BotSalesFulfillmentRequest) (*BotSalesFulfillmentResponse, error) {
	fingerprint, err := botSalesFulfillmentFingerprint(input)
	if err != nil {
		return nil, infraerrors.InternalServer("BOT_SALES_FINGERPRINT_FAILED", "failed to fingerprint fulfillment request")
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("BOT_SALES_DATABASE_UNAVAILABLE", "failed to start fulfillment transaction")
	}
	rollback := func() {
		_ = tx.Rollback()
	}
	txCtx := dbent.NewTxContext(ctx, tx)

	record, inserted, err := claimBotSalesFulfillmentRecord(txCtx, tx.Client(), input, fingerprint)
	if err != nil {
		rollback()
		return nil, err
	}
	if !inserted {
		if record.requestFingerprint != fingerprint {
			rollback()
			return nil, infraerrors.Conflict("BOT_SALES_IDEMPOTENCY_CONFLICT", "fulfillment key is already bound to a different request")
		}
		if record.status == "succeeded" {
			var response BotSalesFulfillmentResponse
			if err := json.Unmarshal([]byte(record.responseJSON), &response); err != nil {
				rollback()
				return nil, infraerrors.InternalServer("BOT_SALES_RESPONSE_CORRUPT", "stored fulfillment response is invalid")
			}
			response.Replayed = true
			rollback()
			return &response, nil
		}
		rollback()
		return nil, infraerrors.Conflict("BOT_SALES_FULFILLMENT_IN_PROGRESS", "fulfillment is already being processed")
	}

	response, err := s.applyBotSalesFulfillment(txCtx, input)
	if err != nil {
		rollback()
		return nil, err
	}
	response.FulfillmentID = record.id
	responseBody, err := json.Marshal(response)
	if err != nil {
		rollback()
		return nil, infraerrors.InternalServer("BOT_SALES_RESPONSE_ENCODE_FAILED", "failed to encode fulfillment response")
	}

	// payment_order_id uses NULL when no order was created (graceful degradation)
	var paymentOrderID any
	if response.PaymentOrderID > 0 {
		paymentOrderID = response.PaymentOrderID
	}

	if _, err := tx.Client().ExecContext(txCtx, `
		UPDATE bot_sales_fulfillments
		SET status = 'succeeded', user_id = $1, payment_order_id = $2, response_json = $3, updated_at = NOW()
		WHERE id = $4 AND status = 'processing'
	`, response.UserID, paymentOrderID, string(responseBody), record.id); err != nil {
		rollback()
		return nil, infraerrors.ServiceUnavailable("BOT_SALES_FULFILLMENT_RECORD_FAILED", "failed to finalize fulfillment record")
	}
	if err := tx.Commit(); err != nil {
		return nil, infraerrors.ServiceUnavailable("BOT_SALES_DATABASE_COMMIT_FAILED", "failed to commit fulfillment")
	}
	return response, nil
}

func claimBotSalesFulfillmentRecord(ctx context.Context, client *dbent.Client, input BotSalesFulfillmentRequest, fingerprint string) (botSalesFulfillmentRecord, bool, error) {
	result, err := client.ExecContext(ctx, `
		INSERT INTO bot_sales_fulfillments
			(idempotency_key, external_order_code, external_order_item_id, request_fingerprint, status, balance_amount)
		VALUES ($1, $2, $3, $4, 'processing', $5)
		ON CONFLICT DO NOTHING
	`, input.IdempotencyKey, input.ExternalOrderCode, input.ExternalOrderItemID, fingerprint, input.BalanceAmount)
	if err != nil {
		return botSalesFulfillmentRecord{}, false, infraerrors.ServiceUnavailable("BOT_SALES_FULFILLMENT_RECORD_FAILED", "failed to reserve fulfillment record")
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return botSalesFulfillmentRecord{}, false, infraerrors.ServiceUnavailable("BOT_SALES_FULFILLMENT_RECORD_FAILED", "failed to inspect fulfillment reservation")
	}
	record, err := readBotSalesFulfillmentRecord(ctx, client, input)
	if err != nil {
		return botSalesFulfillmentRecord{}, false, err
	}
	return record, inserted == 1, nil
}

func readBotSalesFulfillmentRecord(ctx context.Context, client *dbent.Client, input BotSalesFulfillmentRequest) (botSalesFulfillmentRecord, error) {
	rows, err := client.QueryContext(ctx, `
		SELECT id, status, request_fingerprint, COALESCE(response_json, '')
		FROM bot_sales_fulfillments
		WHERE idempotency_key = $1
		   OR (external_order_code = $2 AND external_order_item_id = $3)
		ORDER BY CASE WHEN idempotency_key = $1 THEN 0 ELSE 1 END, id
		LIMIT 1
		FOR UPDATE
	`, input.IdempotencyKey, input.ExternalOrderCode, input.ExternalOrderItemID)
	if err != nil {
		return botSalesFulfillmentRecord{}, infraerrors.ServiceUnavailable("BOT_SALES_FULFILLMENT_RECORD_FAILED", "failed to read fulfillment record")
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return botSalesFulfillmentRecord{}, infraerrors.ServiceUnavailable("BOT_SALES_FULFILLMENT_RECORD_FAILED", "failed to read fulfillment record")
		}
		return botSalesFulfillmentRecord{}, infraerrors.InternalServer("BOT_SALES_FULFILLMENT_RECORD_MISSING", "fulfillment reservation disappeared")
	}
	var record botSalesFulfillmentRecord
	if err := rows.Scan(&record.id, &record.status, &record.requestFingerprint, &record.responseJSON); err != nil {
		return botSalesFulfillmentRecord{}, infraerrors.ServiceUnavailable("BOT_SALES_FULFILLMENT_RECORD_FAILED", "failed to decode fulfillment record")
	}
	return record, nil
}

func (s *BotSalesFulfillmentService) applyBotSalesFulfillment(ctx context.Context, input BotSalesFulfillmentRequest) (*BotSalesFulfillmentResponse, error) {
	var (
		user       *User
		deviceCode = input.DeviceCode
	)

	switch {
	case deviceCode != "":
		device, err := s.devices.GetByDeviceCode(ctx, deviceCode)
		if err != nil {
			if stderrors.Is(err, ErrUserDeviceNotFound) {
				return nil, infraerrors.NotFound("BOT_SALES_DEVICE_NOT_FOUND", "device_code does not identify a device")
			}
			return nil, infraerrors.ServiceUnavailable("BOT_SALES_DEVICE_LOOKUP_FAILED", "failed to resolve device_code")
		}
		if device == nil || !device.IsActive() {
			return nil, ErrDeviceRevoked
		}
		user, err = s.users.GetByID(ctx, device.UserID)
		if err != nil {
			return nil, infraerrors.NotFound("BOT_SALES_USER_NOT_FOUND", "device owner was not found")
		}
	case input.Buyer.Email != "":
		var err error
		user, err = s.users.GetByEmail(ctx, input.Buyer.Email)
		if err != nil {
			if dbent.IsNotFound(err) || stderrors.Is(err, ErrUserNotFound) {
				return nil, infraerrors.NotFound("BOT_SALES_USER_NOT_FOUND", "buyer email was not found")
			}
			return nil, infraerrors.ServiceUnavailable("BOT_SALES_USER_LOOKUP_FAILED", "failed to resolve buyer email")
		}
	case !input.IssueDeviceCode:
		return nil, infraerrors.BadRequest("BOT_SALES_DEVICE_REQUIRED", "issue_device_code is required when buyer has no existing account")
	default:
		if s.claimService == nil {
			return nil, infraerrors.ServiceUnavailable("BOT_SALES_CLAIM_NOT_CONFIGURED", "device claim service is not configured")
		}
		deviceHash, err := randomHexString(32)
		if err != nil {
			return nil, infraerrors.ServiceUnavailable("BOT_SALES_DEVICE_HASH_FAILED", "failed to generate device identity")
		}
		claim, err := s.claimService.Claim(ctx, VClawClaimRequest{Device: VClawDeviceInput{
			DeviceHash:         deviceHash,
			FingerprintVersion: 1,
			Platform:           "bot-sales",
			Arch:               "server",
		}})
		if err != nil {
			return nil, err
		}
		user, err = s.users.GetByID(ctx, claim.UserID)
		if err != nil {
			return nil, infraerrors.ServiceUnavailable("BOT_SALES_USER_LOOKUP_FAILED", "failed to load claimed user")
		}
		deviceCode = claim.DeviceLoginCode
	}
	if user == nil || user.ID <= 0 {
		return nil, infraerrors.ServiceUnavailable("BOT_SALES_USER_LOOKUP_FAILED", "buyer user is unavailable")
	}

	apiKey, err := s.ensureBotSalesAPIKey(ctx, user, input)
	if err != nil {
		return nil, err
	}

	if input.IssueDeviceCode && input.DeviceCode == "" && deviceCode == "" {
		code, err := generateDeviceLoginCode()
		if err != nil {
			return nil, infraerrors.ServiceUnavailable("BOT_SALES_DEVICE_CODE_FAILED", "failed to generate device login code")
		}
		deviceHash, err := randomHexString(32)
		if err != nil {
			return nil, infraerrors.ServiceUnavailable("BOT_SALES_DEVICE_HASH_FAILED", "failed to generate device identity")
		}
		now := time.Now().UTC()
		if err := s.devices.Create(ctx, &UserDevice{
			UserID:             user.ID,
			DeviceCode:         &code,
			DeviceHash:         deviceHash,
			FingerprintVersion: 1,
			Platform:           "bot-sales",
			Arch:               "server",
			Status:             UserDeviceStatusActive,
			FirstClaimedAt:     now,
			LastClaimedAt:      &now,
		}); err != nil {
			return nil, infraerrors.ServiceUnavailable("BOT_SALES_DEVICE_CREATE_FAILED", "failed to create device login code")
		}
		deviceCode = code
	}

	// Create PaymentOrder for canonical order management integration
	paymentOrder, err := s.createBotSalesPaymentOrder(ctx, user, input)
	if err != nil {
		return nil, err
	}

	// AdjustBalance via canonical accounting path
	change, err := s.users.AdjustBalance(ctx, user.ID, input.BalanceAmount)
	if err != nil {
		return nil, err
	}

	// Update total_recharged for consistency
	if _, err := s.clientFromContext(ctx).ExecContext(ctx, `
		UPDATE users SET total_recharged = total_recharged + $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, input.BalanceAmount, user.ID); err != nil {
		return nil, infraerrors.ServiceUnavailable("BOT_SALES_BALANCE_AUDIT_FAILED", "failed to update recharge total")
	}

	// Mark PaymentOrder as completed and write audit log
	if err := s.markBotSalesPaymentOrderCompleted(ctx, paymentOrder, input, change); err != nil {
		return nil, err
	}

	return &BotSalesFulfillmentResponse{
		ExternalOrderCode:   input.ExternalOrderCode,
		ExternalOrderItemID: input.ExternalOrderItemID,
		UserID:              user.ID,
		Email:               user.Email,
		BalanceAdded:        input.BalanceAmount,
		Balance:             change.New,
		DeviceLoginCode:     deviceCode,
		BalancePackageCode:  input.BalancePackageCode,
		GroupID:             botSalesValueOrZero(input.GroupID),
		APIKey:              apiKey,
		PaymentOrderID: func() int64 {
			if paymentOrder != nil {
				return paymentOrder.ID
			}
			return 0
		}(),
	}, nil
}

func botSalesValueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func (s *BotSalesFulfillmentService) ensureBotSalesAPIKey(ctx context.Context, user *User, input BotSalesFulfillmentRequest) (*BotSalesAPIKey, error) {
	if input.DeliveryPolicy.IssueAPIKey == "never" {
		return nil, nil
	}
	if input.GroupID == nil {
		return nil, infraerrors.BadRequest("BOT_SALES_GROUP_ID_REQUIRED", "group_id is required when issuing an API key")
	}
	if s.apiKeyService == nil {
		return nil, infraerrors.ServiceUnavailable("BOT_SALES_API_KEY_NOT_CONFIGURED", "API key service is not configured")
	}

	// The fulfillment transaction is already open. On PostgreSQL, serialize
	// key provisioning for the same user/group pair so retries from separate
	// orders cannot create duplicate keys. Repository operations below use the
	// transaction client from context, so a failed fulfillment rolls the key
	// creation back together with the balance mutation.
	if s.client.Driver().Dialect() == dialect.Postgres {
		lockName := fmt.Sprintf("bot-sales-api-key:%d:%d", user.ID, *input.GroupID)
		if _, err := s.clientFromContext(ctx).ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockName); err != nil {
			return nil, infraerrors.ServiceUnavailable("BOT_SALES_API_KEY_LOCK_FAILED", "failed to serialize API key provisioning")
		}
	}

	keys, _, err := s.apiKeyService.List(ctx, user.ID, pagination.PaginationParams{Page: 1, PageSize: 1}, APIKeyListFilters{
		Status:  StatusAPIKeyActive,
		GroupID: input.GroupID,
	})
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("BOT_SALES_API_KEY_LOOKUP_FAILED", "failed to find an API key for the purchased group")
	}
	if len(keys) > 0 {
		key := keys[0]
		if key.GroupID == nil || *key.GroupID != *input.GroupID {
			return nil, infraerrors.InternalServer("BOT_SALES_API_KEY_GROUP_MISMATCH", "existing API key is not associated with the purchased group")
		}
		return &BotSalesAPIKey{ID: key.ID, Key: key.Key, GroupID: key.GroupID}, nil
	}

	key, err := s.apiKeyService.Create(ctx, user.ID, CreateAPIKeyRequest{
		Name:    fmt.Sprintf("Bot-sales group %d", *input.GroupID),
		GroupID: input.GroupID,
	})
	if err != nil {
		return nil, infraerrors.ServiceUnavailable("BOT_SALES_API_KEY_CREATE_FAILED", "failed to create an API key for the purchased group")
	}
	if key == nil || key.GroupID == nil || *key.GroupID != *input.GroupID || key.Key == "" {
		return nil, infraerrors.InternalServer("BOT_SALES_API_KEY_GROUP_MISMATCH", "created API key is not associated with the purchased group")
	}
	return &BotSalesAPIKey{ID: key.ID, Key: key.Key, GroupID: key.GroupID}, nil
}

func (s *BotSalesFulfillmentService) createBotSalesPaymentOrder(ctx context.Context, user *User, input BotSalesFulfillmentRequest) (*dbent.PaymentOrder, error) {
	if s.paymentService == nil {
		// Graceful degradation: skip PaymentOrder creation when payment service unavailable
		// This allows existing tests and minimal deployments to continue working
		return nil, nil
	}

	client := s.clientFromContext(ctx)
	now := time.Now()
	expiresAt := now.Add(24 * time.Hour)

	outTradeNo := fmt.Sprintf("BS-%s-%s", input.ExternalOrderCode, input.ExternalOrderItemID)
	if len(outTradeNo) > 64 {
		outTradeNo = outTradeNo[:64]
	}

	providerSnapshot := map[string]any{
		"schema_version":         2,
		"provider_key":           "bot-sales",
		"external_order_code":    input.ExternalOrderCode,
		"external_order_item_id": input.ExternalOrderItemID,
		"currency":               payment.DefaultPaymentCurrency,
	}

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetNillableUserNotes(psNilIfEmpty(user.Notes)).
		SetAmount(input.BalanceAmount).
		SetPayAmount(input.BalanceAmount).
		SetFeeRate(0).
		SetRechargeCode(fmt.Sprintf("BS-%d-%d", time.Now().UnixNano()%1000000, user.ID)).
		SetOutTradeNo(outTradeNo).
		SetPaymentType("bot-sales").
		SetPaymentTradeNo(input.IdempotencyKey).
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPaid).
		SetProviderKey("bot-sales").
		SetProviderSnapshot(providerSnapshot).
		SetExpiresAt(expiresAt).
		SetPaidAt(now).
		SetClientIP("bot-sales").
		SetSrcHost("bot-sales").
		Save(ctx)

	if err != nil {
		return nil, infraerrors.ServiceUnavailable("BOT_SALES_ORDER_CREATE_FAILED", fmt.Sprintf("failed to create payment order: %v", err))
	}

	return order, nil
}

func (s *BotSalesFulfillmentService) markBotSalesPaymentOrderCompleted(ctx context.Context, order *dbent.PaymentOrder, input BotSalesFulfillmentRequest, change BalanceChange) error {
	if s.paymentService == nil || order == nil {
		// Graceful degradation: skip when payment service unavailable or order wasn't created
		return nil
	}

	client := s.clientFromContext(ctx)
	now := time.Now()

	_, err := client.PaymentOrder.UpdateOneID(order.ID).
		SetStatus(OrderStatusCompleted).
		SetCompletedAt(now).
		Save(ctx)
	if err != nil {
		return infraerrors.ServiceUnavailable("BOT_SALES_ORDER_COMPLETE_FAILED", "failed to mark payment order completed")
	}

	// Write audit log via PaymentService
	s.paymentService.writeAuditLog(ctx, order.ID, "BOT_SALES_FULFILLMENT_SUCCESS", "bot-sales", map[string]any{
		"external_order_code":    input.ExternalOrderCode,
		"external_order_item_id": input.ExternalOrderItemID,
		"balance_amount":         input.BalanceAmount,
		"balance_before":         change.Old,
		"balance_after":          change.New,
		"idempotency_key":        input.IdempotencyKey,
	})

	return nil
}

func (s *BotSalesFulfillmentService) clientFromContext(ctx context.Context) *dbent.Client {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return tx.Client()
	}
	return s.client
}
