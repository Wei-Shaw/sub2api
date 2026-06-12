package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/balancepackage"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/ent/userdevice"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	BotSalesFulfillmentOperationNew   = "new"
	BotSalesFulfillmentOperationRenew = "renew"
	BotSalesFulfillmentOperationTopup = "topup"

	BotSalesIssueAPIKeyAlways    = "always"
	BotSalesIssueAPIKeyIfMissing = "if_missing"
	BotSalesIssueAPIKeyNever     = "never"

	BotSalesEntitlementSubscription = "subscription"
	BotSalesEntitlementBalance      = "balance"

	BotSalesPaymentProvider = "bot_sales"
	botSalesPaymentCurrency = "VND"
	botSalesMaxQuantity     = 1000
)

// SubscriptionTypeNone is accepted for non-subscription groups used by balance packages.
const SubscriptionTypeNone = "none"

type BotSalesFulfillmentBuyer struct {
	ExternalUserID string `json:"external_user_id"`
	Email          string `json:"email"`
	Username       string `json:"username"`
	DisplayName    string `json:"display_name"`
	Provider       string `json:"provider"`
	ProviderUserID string `json:"provider_user_id"`
	TelegramID     string `json:"telegram_id"`
}

type BotSalesFulfillmentAffiliate struct {
	AffCode string `json:"aff_code"`
}

type BotSalesDeliveryPolicy struct {
	IssueAPIKey     string `json:"issue_api_key"`
	IssueDeviceCode bool   `json:"issue_device_code"`
}

func (p *BotSalesDeliveryPolicy) UnmarshalJSON(data []byte) error {
	var raw struct {
		IssueAPIKey     any  `json:"issue_api_key"`
		IssueDeviceCode bool `json:"issue_device_code"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch v := raw.IssueAPIKey.(type) {
	case bool:
		if v {
			p.IssueAPIKey = BotSalesIssueAPIKeyAlways
		} else {
			p.IssueAPIKey = BotSalesIssueAPIKeyNever
		}
	case string:
		p.IssueAPIKey = strings.TrimSpace(v)
	}
	p.IssueDeviceCode = raw.IssueDeviceCode
	return nil
}

type BotSalesTokenFulfillmentRequest struct {
	ExternalOrderID      string                        `json:"external_order_id"`
	ExternalOrderItemID  string                        `json:"external_order_item_id"`
	ExternalPaymentID    string                        `json:"external_payment_id"`
	Operation            string                        `json:"operation"`
	EntitlementKind      string                        `json:"entitlement_kind"`
	PlanID               int64                         `json:"plan_id"`
	BalancePackageCode   string                        `json:"balance_package_code"`
	Quantity             int                           `json:"quantity"`
	PaymentAmount        float64                       `json:"payment_amount"`
	PaymentCurrency      string                        `json:"payment_currency"`
	PaymentProvider      string                        `json:"payment_provider"`
	PaymentProviderTxnID string                        `json:"payment_provider_txn_id"`
	PaidAt               *time.Time                    `json:"paid_at"`
	DeviceCode           string                        `json:"device_code"`
	Buyer                BotSalesFulfillmentBuyer      `json:"buyer"`
	Affiliate            *BotSalesFulfillmentAffiliate `json:"affiliate"`
	DeliveryPolicy       BotSalesDeliveryPolicy        `json:"delivery_policy"`
	RawPayload           map[string]any                `json:"-"`
}

type BotSalesTokenFulfillmentResponse struct {
	ExternalOrderID string                          `json:"external_order_id"`
	Operation       string                          `json:"operation"`
	EntitlementKind string                          `json:"entitlement_kind"`
	Entitlement     BotSalesFulfillmentEntitlement  `json:"entitlement"`
	Subscription    BotSalesFulfillmentSubscription `json:"subscription,omitempty"`
	Balance         BotSalesFulfillmentBalance      `json:"balance,omitempty"`
	Buyer           BotSalesFulfillmentBuyerResult  `json:"buyer"`
	Delivery        BotSalesFulfillmentDelivery     `json:"delivery"`
	DeviceCode      string                          `json:"device_code,omitempty"`
	paymentReplayed bool
}

type BotSalesFulfillmentEntitlement struct {
	Kind               string  `json:"kind"`
	PlanID             *int64  `json:"plan_id,omitempty"`
	BalancePackageCode *string `json:"balance_package_code,omitempty"`
	GroupID            int64   `json:"group_id"`
	BalanceCredited    float64 `json:"balance_credited,omitempty"`
}

type BotSalesFulfillmentBuyerResult struct {
	UserID         int64  `json:"user_id"`
	ExternalUserID string `json:"external_user_id"`
	Email          string `json:"email"`
}

type BotSalesFulfillmentDelivery struct {
	APIKey     *BotSalesDeliveryAPIKey `json:"api_key,omitempty"`
	DeviceCode string                  `json:"device_code,omitempty"`
}

type BotSalesDeliveryAPIKey struct {
	ID      int64  `json:"id,omitempty"`
	Key     string `json:"key"`
	GroupID *int64 `json:"group_id,omitempty"`
}

type BotSalesFulfillmentSubscription struct {
	ID      int64 `json:"id,omitempty"`
	GroupID int64 `json:"group_id"`
}

type BotSalesFulfillmentBalance struct {
	GroupID       int64   `json:"group_id"`
	AmountLedger  float64 `json:"amount_ledger"`
	ActualCredits int64   `json:"actual_credits"`
}

type BotSalesFulfillmentService struct {
	entClient           *dbent.Client
	userRepo            UserRepository
	userService         *UserService
	settingService      *SettingService
	subscriptionService *SubscriptionService
	apiKeyService       *APIKeyService
	userDeviceRepo      UserDeviceRepository
	paymentService      *PaymentService
}

func NewBotSalesFulfillmentService(
	entClient *dbent.Client,
	userService *UserService,
	settingService *SettingService,
	subscriptionService *SubscriptionService,
	apiKeyService *APIKeyService,
	userDeviceRepo UserDeviceRepository,
	paymentService *PaymentService,
) *BotSalesFulfillmentService {
	return &BotSalesFulfillmentService{
		entClient:           entClient,
		userRepo:            userServiceUserRepo(userService),
		userService:         userService,
		settingService:      settingService,
		subscriptionService: subscriptionService,
		apiKeyService:       apiKeyService,
		userDeviceRepo:      userDeviceRepo,
		paymentService:      paymentService,
	}
}

func (s *BotSalesFulfillmentService) Fulfill(ctx context.Context, req BotSalesTokenFulfillmentRequest) (*BotSalesTokenFulfillmentResponse, error) {
	if s == nil || s.entClient == nil || s.userRepo == nil || s.userService == nil {
		return nil, infraerrors.ServiceUnavailable("BOT_SALES_FULFILLMENT_UNAVAILABLE", "bot-sales fulfillment service is not available")
	}
	if _, ok := req.RawPayload["target_group_id"]; ok {
		return nil, infraerrors.BadRequest("UNSUPPORTED_FIELD", "target_group_id is not accepted; send plan_id or balance_package_code")
	}
	if _, ok := req.RawPayload["targetGroupId"]; ok {
		return nil, infraerrors.BadRequest("UNSUPPORTED_FIELD", "targetGroupId is not accepted; send plan_id or balance_package_code")
	}
	if strings.TrimSpace(req.ExternalOrderID) == "" {
		return nil, infraerrors.BadRequest("BOT_SALES_EXTERNAL_ORDER_REQUIRED", "external_order_id is required")
	}
	if strings.TrimSpace(req.Buyer.ExternalUserID) == "" {
		return nil, infraerrors.BadRequest("BOT_SALES_BUYER_REQUIRED", "buyer.external_user_id is required")
	}

	buyer, err := s.findOrCreateBuyer(ctx, req.Buyer)
	if err != nil {
		return nil, err
	}

	resp := &BotSalesTokenFulfillmentResponse{
		ExternalOrderID: req.ExternalOrderID,
		Operation:       req.Operation,
		EntitlementKind: req.EntitlementKind,
		Buyer: BotSalesFulfillmentBuyerResult{
			UserID:         buyer.ID,
			ExternalUserID: req.Buyer.ExternalUserID,
			Email:          buyer.Email,
		},
	}

	switch req.EntitlementKind {
	case BotSalesEntitlementSubscription:
		if err := s.fulfillSubscription(ctx, buyer, req, resp); err != nil {
			return nil, err
		}
	case BotSalesEntitlementBalance:
		if err := s.fulfillBalance(ctx, buyer, req, resp); err != nil {
			return nil, err
		}
	default:
		return nil, infraerrors.BadRequest("BOT_SALES_ENTITLEMENT_KIND_INVALID", "entitlement_kind must be subscription or balance")
	}

	apiKeyUserID := resp.Buyer.UserID
	if apiKeyUserID <= 0 {
		apiKeyUserID = buyer.ID
	}
	if !resp.paymentReplayed && s.apiKeyService != nil {
		apiKey, err := s.issueBotSalesAPIKeyForPolicy(ctx, apiKeyUserID, resp.Entitlement.GroupID, req)
		if err != nil {
			return nil, err
		}
		if apiKey != nil {
			resp.Delivery.APIKey = &BotSalesDeliveryAPIKey{ID: apiKey.ID, Key: apiKey.Key, GroupID: apiKey.GroupID}
		}
	}

	return resp, nil
}

func (s *BotSalesFulfillmentService) issueBotSalesAPIKeyForPolicy(ctx context.Context, userID int64, targetGroupID int64, req BotSalesTokenFulfillmentRequest) (*APIKey, error) {
	policy := strings.TrimSpace(req.DeliveryPolicy.IssueAPIKey)
	switch policy {
	case "", BotSalesIssueAPIKeyAlways:
		return s.createBotSalesAPIKey(ctx, userID, targetGroupID, req.ExternalOrderID)
	case BotSalesIssueAPIKeyIfMissing:
		apiKey, err := s.findReusableBotSalesAPIKey(ctx, userID)
		if err != nil {
			return nil, err
		}
		if apiKey != nil {
			return s.rebindBotSalesAPIKeyIfNeeded(ctx, apiKey, userID, targetGroupID)
		}
		return s.createBotSalesAPIKey(ctx, userID, targetGroupID, req.ExternalOrderID)
	case BotSalesIssueAPIKeyNever:
		return nil, nil
	default:
		return nil, nil
	}
}

func (s *BotSalesFulfillmentService) createBotSalesAPIKey(ctx context.Context, userID int64, targetGroupID int64, externalOrderID string) (*APIKey, error) {
	if s == nil || s.apiKeyService == nil {
		return nil, nil
	}
	return s.apiKeyService.Create(ctx, userID, CreateAPIKeyRequest{
		Name:    fmt.Sprintf("bot-sales-%s", externalOrderID),
		GroupID: &targetGroupID,
	})
}

func (s *BotSalesFulfillmentService) findReusableBotSalesAPIKey(ctx context.Context, userID int64) (*APIKey, error) {
	if s == nil || s.apiKeyService == nil || userID <= 0 {
		return nil, nil
	}
	keys, _, err := s.apiKeyService.List(ctx, userID, pagination.PaginationParams{Page: 1, PageSize: 1000, SortBy: "created_at", SortOrder: pagination.SortOrderAsc}, APIKeyListFilters{Status: StatusAPIKeyActive})
	if err != nil {
		return nil, err
	}
	for i := range keys {
		candidate := &keys[i]
		if isReusableBotSalesAPIKey(candidate) {
			return candidate, nil
		}
	}
	return nil, nil
}

func (s *BotSalesFulfillmentService) rebindBotSalesAPIKeyIfNeeded(ctx context.Context, apiKey *APIKey, userID int64, targetGroupID int64) (*APIKey, error) {
	if apiKey == nil || targetGroupID <= 0 || apiKey.GroupID != nil && *apiKey.GroupID == targetGroupID {
		return apiKey, nil
	}
	return s.rebindBotSalesAPIKeyGroup(ctx, apiKey, userID, targetGroupID)
}

func (s *BotSalesFulfillmentService) rebindBotSalesAPIKeyGroup(ctx context.Context, apiKey *APIKey, userID int64, targetGroupID int64) (*APIKey, error) {
	if s == nil || s.apiKeyService == nil || apiKey == nil {
		return apiKey, nil
	}
	user, err := s.apiKeyService.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	group, err := s.apiKeyService.groupRepo.GetByID(ctx, targetGroupID)
	if err != nil {
		return nil, fmt.Errorf("get group: %w", err)
	}
	if !s.apiKeyService.canUserBindGroup(ctx, user, group) {
		return nil, ErrGroupNotAllowed
	}
	apiKey.GroupID = &targetGroupID
	if err := s.apiKeyService.apiKeyRepo.Update(ctx, apiKey); err != nil {
		return nil, fmt.Errorf("update api key: %w", err)
	}
	s.apiKeyService.InvalidateAuthCacheByKey(ctx, apiKey.Key)
	s.apiKeyService.compileAPIKeyIPRules(apiKey)
	return apiKey, nil
}

func isReusableBotSalesAPIKey(apiKey *APIKey) bool {
	if apiKey == nil || !apiKey.IsActive() {
		return false
	}
	return !apiKey.IsExpired() && !apiKey.IsQuotaExhausted()
}

func (s *BotSalesFulfillmentService) fulfillSubscription(ctx context.Context, buyer *User, req BotSalesTokenFulfillmentRequest, resp *BotSalesTokenFulfillmentResponse) error {
	if req.PlanID <= 0 {
		return infraerrors.BadRequest("BOT_SALES_PLAN_REQUIRED", "plan_id is required for subscription fulfillment")
	}
	if s.subscriptionService == nil {
		return infraerrors.ServiceUnavailable("SUBSCRIPTION_SERVICE_UNAVAILABLE", "subscription service is not available")
	}

	plan, err := s.entClient.SubscriptionPlan.Query().Where(subscriptionplan.IDEQ(req.PlanID)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return infraerrors.NotFound("BOT_SALES_PLAN_NOT_FOUND", "subscription plan not found")
		}
		return err
	}
	if plan.GroupID <= 0 {
		return infraerrors.BadRequest("BOT_SALES_PLAN_GROUP_REQUIRED", "subscription plan has no group")
	}

	validityDays := plan.ValidityDays
	if validityDays <= 0 {
		validityDays = 30
	}
	operation := req.Operation
	if operation == "" {
		operation = BotSalesFulfillmentOperationNew
	}
	if operation != BotSalesFulfillmentOperationNew && operation != BotSalesFulfillmentOperationRenew {
		return infraerrors.BadRequest("BOT_SALES_OPERATION_INVALID", "subscription operation must be new or renew")
	}

	sub, err := s.subscriptionService.AssignSubscription(ctx, &AssignSubscriptionInput{
		UserID:       buyer.ID,
		GroupID:      plan.GroupID,
		ValidityDays: validityDays,
		Notes:        botSalesNotes(req),
	})
	if err != nil {
		return err
	}

	planID := plan.ID
	resp.Operation = operation
	resp.EntitlementKind = BotSalesEntitlementSubscription
	resp.Entitlement = BotSalesFulfillmentEntitlement{
		Kind:    BotSalesEntitlementSubscription,
		PlanID:  &planID,
		GroupID: plan.GroupID,
	}
	resp.Subscription = BotSalesFulfillmentSubscription{ID: sub.ID, GroupID: sub.GroupID}
	return nil
}

func (s *BotSalesFulfillmentService) fulfillBalance(ctx context.Context, buyer *User, req BotSalesTokenFulfillmentRequest, resp *BotSalesTokenFulfillmentResponse) error {
	code := strings.TrimSpace(req.BalancePackageCode)
	if code == "" {
		return infraerrors.BadRequest("BOT_SALES_BALANCE_PACKAGE_REQUIRED", "balance_package_code is required for balance fulfillment")
	}
	operation := req.Operation
	if operation == "" {
		operation = BotSalesFulfillmentOperationNew
	}
	if operation != BotSalesFulfillmentOperationNew && operation != BotSalesFulfillmentOperationTopup {
		return infraerrors.BadRequest("BOT_SALES_OPERATION_INVALID", "balance operation must be new or topup")
	}
	req.Operation = operation

	pkg, err := s.entClient.BalancePackage.Query().Where(balancepackage.CodeEQ(code)).Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return infraerrors.NotFound("BOT_SALES_BALANCE_PACKAGE_NOT_FOUND", "balance package not found")
		}
		return err
	}
	if pkg.AmountLedger <= 0 {
		return infraerrors.BadRequest("BOT_SALES_BALANCE_PACKAGE_INVALID", "balance package amount must be positive")
	}
	if err := validateBotSalesBalanceQuantity(req, pkg); err != nil {
		return err
	}

	targetBuyer := buyer
	var deviceCode string
	if operation == BotSalesFulfillmentOperationTopup && strings.TrimSpace(req.DeviceCode) != "" {
		device, err := s.resolveBotSalesDevice(ctx, req.DeviceCode)
		if err != nil {
			return err
		}
		owner, err := s.userRepo.GetByID(ctx, device.UserID)
		if err != nil {
			return err
		}
		targetBuyer = owner
		if device.DeviceCode != nil {
			deviceCode = *device.DeviceCode
		}
	}
	paymentOrder, paymentReplayed, err := s.recordBotSalesBalancePayment(ctx, targetBuyer, req, pkg)
	if err != nil {
		return err
	}
	resp.paymentReplayed = paymentReplayed
	if operation == BotSalesFulfillmentOperationNew && !paymentReplayed {
		issuedDeviceCode, err := s.ensureBotSalesDeviceCode(ctx, targetBuyer, req)
		if err != nil {
			return err
		}
		deviceCode = issuedDeviceCode
	}
	groupID := int64(0)
	if pkg.GroupID != nil {
		groupID = *pkg.GroupID
	}

	balanceCredited := botSalesBalanceLedgerAmount(req, pkg)
	actualCredits := botSalesBalanceActualCredits(req, pkg)
	if paymentReplayed && paymentOrder != nil {
		balanceCredited = paymentOrder.Amount
		if paymentOrder.ActualCredits != nil {
			actualCredits = *paymentOrder.ActualCredits
		}
	}

	resp.Operation = operation
	resp.EntitlementKind = BotSalesEntitlementBalance
	resp.Buyer = BotSalesFulfillmentBuyerResult{UserID: targetBuyer.ID, ExternalUserID: req.Buyer.ExternalUserID, Email: targetBuyer.Email}
	resp.Entitlement = BotSalesFulfillmentEntitlement{
		Kind:               BotSalesEntitlementBalance,
		BalancePackageCode: &code,
		GroupID:            groupID,
		BalanceCredited:    balanceCredited,
	}
	resp.Balance = BotSalesFulfillmentBalance{GroupID: groupID, AmountLedger: balanceCredited, ActualCredits: actualCredits}
	resp.Delivery.DeviceCode = deviceCode
	resp.DeviceCode = deviceCode
	return nil
}

func (s *BotSalesFulfillmentService) recordBotSalesBalancePayment(ctx context.Context, buyer *User, req BotSalesTokenFulfillmentRequest, pkg *dbent.BalancePackage) (*dbent.PaymentOrder, bool, error) {
	if s.entClient == nil {
		return nil, false, infraerrors.ServiceUnavailable("BOT_SALES_PAYMENT_ACCOUNTING_UNAVAILABLE", "payment accounting service is not available")
	}
	if s.paymentService == nil {
		return nil, false, infraerrors.ServiceUnavailable("BOT_SALES_PAYMENT_ACCOUNTING_UNAVAILABLE", "payment accounting service is not available")
	}
	currency := normalizeCurrencyCode(req.PaymentCurrency, botSalesPaymentCurrency)
	if currency != botSalesPaymentCurrency {
		return nil, false, infraerrors.BadRequest("BOT_SALES_PAYMENT_CURRENCY_INVALID", "payment_currency must be VND for bot-sales balance fulfillment")
	}
	paymentAmount, paymentAmountSource, err := botSalesPaymentAmount(req, pkg, currency)
	if err != nil {
		return nil, false, err
	}
	paymentAmount = roundPaymentAmountForCollection(paymentAmount, currency)

	outTradeNo := botSalesOutTradeNo(req)
	externalPaymentID := botSalesExternalPaymentID(req, outTradeNo)
	existing, err := s.entClient.PaymentOrder.Query().Where(paymentorder.OutTradeNoEQ(outTradeNo)).Only(ctx)
	if err == nil {
		if err := validateBotSalesExistingPaymentOrder(existing, buyer, req, pkg, paymentAmount, currency); err != nil {
			return nil, false, err
		}
		if existing.Status == OrderStatusCompleted {
			return existing, true, nil
		}
		if existing.Status == OrderStatusRecharging {
			return nil, false, infraerrors.Conflict("BOT_SALES_FULFILLMENT_IN_PROGRESS", "bot-sales balance fulfillment is already in progress")
		}
		if existing.Status != OrderStatusPaid {
			_, err = s.entClient.PaymentOrder.UpdateOneID(existing.ID).
				SetStatus(OrderStatusPaid).
				SetPaidAt(botSalesPaidAt(req)).
				SetPaymentTradeNo(externalPaymentID).
				Save(ctx)
			if err != nil {
				return nil, false, fmt.Errorf("mark bot-sales order paid: %w", err)
			}
		}
		if err := s.fulfillBotSalesBalanceOrder(ctx, existing.ID); err != nil {
			return nil, false, err
		}
		updated, getErr := s.entClient.PaymentOrder.Get(ctx, existing.ID)
		if getErr != nil {
			return existing, false, nil
		}
		if updated.Status != OrderStatusCompleted {
			return nil, false, infraerrors.Conflict("BOT_SALES_FULFILLMENT_IN_PROGRESS", "bot-sales balance fulfillment is already in progress")
		}
		return updated, false, nil
	}
	if !dbent.IsNotFound(err) {
		return nil, false, err
	}

	paidAt := botSalesPaidAt(req)
	ledgerCurrency := defaultLedgerCurrency
	groupID := int64(0)
	if pkg.GroupID != nil {
		groupID = *pkg.GroupID
	}
	rechargeCode := botSalesRechargeCode(outTradeNo)
	providerSnapshot := botSalesProviderSnapshot(req, pkg, outTradeNo, paymentAmount, paymentAmountSource, currency, externalPaymentID)
	amount := roundLedgerAmountForCredit(botSalesBalanceLedgerAmount(req, pkg), ledgerCurrency)
	paymentAmount = roundPaymentAmountForCollection(paymentAmount, currency)
	created, err := s.entClient.PaymentOrder.Create().
		SetUserID(buyer.ID).
		SetUserEmail(buyer.Email).
		SetUserName(buyer.Username).
		SetAmount(amount).
		SetPayAmount(paymentAmount).
		SetPaymentAmount(paymentAmount).
		SetPaymentCurrency(currency).
		SetLedgerAmount(amount).
		SetLedgerCurrency(ledgerCurrency).
		SetFeeRate(0).
		SetPaymentType(BotSalesPaymentProvider).
		SetProviderKey(BotSalesPaymentProvider).
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPaid).
		SetOutTradeNo(outTradeNo).
		SetPaymentTradeNo(externalPaymentID).
		SetRechargeCode(rechargeCode).
		SetActualCredits(botSalesBalanceActualCredits(req, pkg)).
		SetProviderSnapshot(providerSnapshot).
		SetClientIP("bot-sales").
		SetSrcHost("bot-sales").
		SetPaidAt(paidAt).
		SetCreatedAt(paidAt).
		SetUpdatedAt(time.Now()).
		SetExpiresAt(paidAt.Add(24 * time.Hour)).
		SetNillableBalanceGroupID(int64PtrOrNil(groupID)).
		Save(ctx)
	if err != nil {
		var constraintErr *dbent.ConstraintError
		if errors.As(err, &constraintErr) {
			existing, getErr := s.entClient.PaymentOrder.Query().Where(paymentorder.OutTradeNoEQ(outTradeNo)).Only(ctx)
			if getErr == nil {
				if err := validateBotSalesExistingPaymentOrder(existing, buyer, req, pkg, paymentAmount, currency); err != nil {
					return nil, false, err
				}
				if existing.Status == OrderStatusCompleted {
					return existing, true, nil
				}
				if existing.Status == OrderStatusRecharging {
					return nil, false, infraerrors.Conflict("BOT_SALES_FULFILLMENT_IN_PROGRESS", "bot-sales balance fulfillment is already in progress")
				}
				if fulfillErr := s.fulfillBotSalesBalanceOrder(ctx, existing.ID); fulfillErr != nil {
					return nil, false, fulfillErr
				}
				updated, getErr := s.entClient.PaymentOrder.Get(ctx, existing.ID)
				if getErr != nil {
					return existing, false, nil
				}
				if updated.Status != OrderStatusCompleted {
					return nil, false, infraerrors.Conflict("BOT_SALES_FULFILLMENT_IN_PROGRESS", "bot-sales balance fulfillment is already in progress")
				}
				return updated, false, nil
			}
		}
		return nil, false, err
	}
	s.writeBotSalesPaymentAudit(ctx, created.ID, "ORDER_CREATED", map[string]any{
		"source":              "bot-sales",
		"externalOrderID":     req.ExternalOrderID,
		"externalPaymentID":   externalPaymentID,
		"paymentAmount":       paymentAmount,
		"paymentAmountSource": paymentAmountSource,
		"paymentCurrency":     currency,
		"balancePackageCode":  pkg.Code,
	})
	s.writeBotSalesPaymentAudit(ctx, created.ID, "ORDER_PAID", map[string]any{
		"source":            "bot-sales",
		"externalPaymentID": externalPaymentID,
		"paidAt":            paidAt.Format(time.RFC3339),
	})

	if err := s.fulfillBotSalesBalanceOrder(ctx, created.ID); err != nil {
		return nil, false, err
	}
	updated, err := s.entClient.PaymentOrder.Get(ctx, created.ID)
	if err != nil {
		return created, false, nil
	}
	if updated.Status != OrderStatusCompleted {
		return nil, false, infraerrors.Conflict("BOT_SALES_FULFILLMENT_IN_PROGRESS", "bot-sales balance fulfillment is already in progress")
	}
	return updated, false, nil
}

func (s *BotSalesFulfillmentService) fulfillBotSalesBalanceOrder(ctx context.Context, orderID int64) error {
	if s.paymentService == nil {
		return infraerrors.ServiceUnavailable("BOT_SALES_PAYMENT_ACCOUNTING_UNAVAILABLE", "payment accounting service is not available")
	}
	return s.paymentService.ExecuteBalanceFulfillment(ctx, orderID)
}

func (s *BotSalesFulfillmentService) writeBotSalesPaymentAudit(ctx context.Context, orderID int64, action string, metadata map[string]any) {
	if s == nil || s.entClient == nil || orderID <= 0 {
		return
	}
	_, _ = s.entClient.PaymentAuditLog.Create().
		SetOrderID(fmt.Sprintf("%d", orderID)).
		SetAction(action).
		SetOperator("bot-sales").
		SetDetail(botSalesAuditDetail(metadata)).
		Save(ctx)
}

func botSalesAuditDetail(metadata map[string]any) string {
	if len(metadata) == 0 {
		return ""
	}
	b, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Sprintf("%v", metadata)
	}
	return string(b)
}

func botSalesOutTradeNo(req BotSalesTokenFulfillmentRequest) string {
	parts := []string{
		"bot-sales",
		strings.TrimSpace(req.ExternalOrderID),
		strings.TrimSpace(req.ExternalOrderItemID),
	}
	if strings.TrimSpace(req.ExternalOrderItemID) == "" {
		parts = append(parts, strings.TrimSpace(req.Operation), strings.TrimSpace(req.BalancePackageCode))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, ":")))
	return "bs_" + hex.EncodeToString(sum[:])[:40]
}

func botSalesRechargeCode(outTradeNo string) string {
	sum := sha256.Sum256([]byte("redeem:" + outTradeNo))
	return strings.ToUpper(hex.EncodeToString(sum[:])[:24])
}

func botSalesPaidAt(req BotSalesTokenFulfillmentRequest) time.Time {
	if req.PaidAt != nil && !req.PaidAt.IsZero() {
		return req.PaidAt.UTC()
	}
	return time.Now().UTC()
}

func botSalesExternalPaymentID(req BotSalesTokenFulfillmentRequest, outTradeNo string) string {
	if externalPaymentID := strings.TrimSpace(req.ExternalPaymentID); externalPaymentID != "" {
		return externalPaymentID
	}
	return outTradeNo
}

func botSalesQuantity(req BotSalesTokenFulfillmentRequest) int {
	if req.Quantity > 0 {
		return req.Quantity
	}
	return 1
}

func botSalesPaymentAmount(req BotSalesTokenFulfillmentRequest, pkg *dbent.BalancePackage, currency string) (float64, string, error) {
	if req.PaymentAmount > 0 && !math.IsNaN(req.PaymentAmount) && !math.IsInf(req.PaymentAmount, 0) {
		return req.PaymentAmount, "payload", nil
	}
	if botSalesPaymentAmountProvided(req) {
		if req.PaymentAmount < 0 || math.IsNaN(req.PaymentAmount) || math.IsInf(req.PaymentAmount, 0) {
			return 0, "", infraerrors.BadRequest("BOT_SALES_PAYMENT_AMOUNT_INVALID", "payment_amount must be non-negative")
		}
		return req.PaymentAmount, "payload", nil
	}
	unitPrice, ok := resolveCurrencyOverride(pkg.CurrencyOverrides, currency)
	if !ok {
		return 0, "", infraerrors.BadRequest("BOT_SALES_PACKAGE_VND_PRICE_REQUIRED", "payment_amount is required unless balance package has a VND currency override")
	}
	quantity := botSalesQuantity(req)
	return unitPrice * float64(quantity), "balance_package.currency_overrides", nil
}

func botSalesPaymentAmountProvided(req BotSalesTokenFulfillmentRequest) bool {
	if req.RawPayload == nil {
		return false
	}
	if _, ok := req.RawPayload["payment_amount"]; ok {
		return true
	}
	_, ok := req.RawPayload["paymentAmount"]
	return ok
}

func validateBotSalesBalanceQuantity(req BotSalesTokenFulfillmentRequest, pkg *dbent.BalancePackage) error {
	if req.Quantity < 0 {
		return infraerrors.BadRequest("BOT_SALES_QUANTITY_INVALID", "quantity must be a positive integer")
	}
	if _, provided := req.RawPayload["quantity"]; provided && req.Quantity <= 0 {
		return infraerrors.BadRequest("BOT_SALES_QUANTITY_INVALID", "quantity must be a positive integer")
	}
	quantity := botSalesQuantity(req)
	if quantity <= 0 {
		return infraerrors.BadRequest("BOT_SALES_QUANTITY_INVALID", "quantity must be a positive integer")
	}
	if quantity > botSalesMaxQuantity {
		return infraerrors.BadRequest("BOT_SALES_QUANTITY_INVALID", "quantity exceeds the maximum allowed")
	}
	ledgerAmount := pkg.AmountLedger * float64(quantity)
	if ledgerAmount <= 0 || math.IsNaN(ledgerAmount) || math.IsInf(ledgerAmount, 0) {
		return infraerrors.BadRequest("BOT_SALES_QUANTITY_INVALID", "quantity produces an invalid balance amount")
	}
	const maxInt64 = int64(9223372036854775807)
	if pkg.ActualCredits > 0 && pkg.ActualCredits > maxInt64/int64(quantity) {
		return infraerrors.BadRequest("BOT_SALES_QUANTITY_INVALID", "quantity produces an invalid actual credits amount")
	}
	return nil
}

func validateBotSalesExistingPaymentOrder(existing *dbent.PaymentOrder, buyer *User, req BotSalesTokenFulfillmentRequest, pkg *dbent.BalancePackage, paymentAmount float64, currency string) error {
	if existing == nil || buyer == nil || pkg == nil {
		return infraerrors.Conflict("BOT_SALES_FULFILLMENT_CONFLICT", "bot-sales fulfillment conflicts with an existing payment order")
	}
	if existing.UserID != buyer.ID {
		return infraerrors.Conflict("BOT_SALES_FULFILLMENT_CONFLICT", "bot-sales fulfillment conflicts with the existing payment buyer")
	}
	if normalizeCurrencyCode(existing.PaymentCurrency, "") != currency {
		return infraerrors.Conflict("BOT_SALES_FULFILLMENT_CONFLICT", "bot-sales fulfillment conflicts with the existing payment currency")
	}
	if !currencyAmountMatches(existing.PaymentAmount, paymentAmount, currency) {
		return infraerrors.Conflict("BOT_SALES_FULFILLMENT_CONFLICT", "bot-sales fulfillment conflicts with the existing payment amount")
	}
	expectedLedgerAmount := roundLedgerAmountForCredit(botSalesBalanceLedgerAmount(req, pkg), defaultLedgerCurrency)
	if !currencyAmountMatches(existing.Amount, expectedLedgerAmount, defaultLedgerCurrency) || !currencyAmountMatches(existing.LedgerAmount, expectedLedgerAmount, defaultLedgerCurrency) {
		return infraerrors.Conflict("BOT_SALES_FULFILLMENT_CONFLICT", "bot-sales fulfillment conflicts with the existing ledger amount")
	}
	expectedActualCredits := botSalesBalanceActualCredits(req, pkg)
	if existing.ActualCredits != nil && *existing.ActualCredits != expectedActualCredits {
		return infraerrors.Conflict("BOT_SALES_FULFILLMENT_CONFLICT", "bot-sales fulfillment conflicts with the existing actual credits")
	}
	if snapshotPackage := botSalesSnapshotString(existing.ProviderSnapshot, "balance_package_code"); snapshotPackage != "" && snapshotPackage != pkg.Code {
		return infraerrors.Conflict("BOT_SALES_FULFILLMENT_CONFLICT", "bot-sales fulfillment conflicts with the existing balance package")
	}
	if snapshotOperation := botSalesSnapshotString(existing.ProviderSnapshot, "operation"); snapshotOperation != "" && snapshotOperation != req.Operation {
		return infraerrors.Conflict("BOT_SALES_FULFILLMENT_CONFLICT", "bot-sales fulfillment conflicts with the existing operation")
	}
	if snapshotQuantity, ok := botSalesSnapshotInt(existing.ProviderSnapshot, "quantity"); ok && snapshotQuantity != botSalesQuantity(req) {
		return infraerrors.Conflict("BOT_SALES_FULFILLMENT_CONFLICT", "bot-sales fulfillment conflicts with the existing quantity")
	}
	return nil
}

func botSalesSnapshotString(snapshot map[string]any, key string) string {
	if len(snapshot) == 0 {
		return ""
	}
	if s, ok := snapshot[key].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func botSalesSnapshotInt(snapshot map[string]any, key string) (int, bool) {
	if len(snapshot) == 0 {
		return 0, false
	}
	switch value := snapshot[key].(type) {
	case int:
		return value, true
	case int64:
		return int(value), true
	case float64:
		return int(value), true
	case json.Number:
		v, err := value.Int64()
		if err == nil {
			return int(v), true
		}
	}
	return 0, false
}

func botSalesBalanceLedgerAmount(req BotSalesTokenFulfillmentRequest, pkg *dbent.BalancePackage) float64 {
	if pkg == nil {
		return 0
	}
	return pkg.AmountLedger * float64(botSalesQuantity(req))
}

func botSalesBalanceActualCredits(req BotSalesTokenFulfillmentRequest, pkg *dbent.BalancePackage) int64 {
	if pkg == nil || pkg.ActualCredits <= 0 {
		return 0
	}
	return pkg.ActualCredits * int64(botSalesQuantity(req))
}

func botSalesProviderSnapshot(req BotSalesTokenFulfillmentRequest, pkg *dbent.BalancePackage, outTradeNo string, paymentAmount float64, paymentAmountSource string, currency string, externalPaymentID string) map[string]any {
	provider := strings.TrimSpace(req.PaymentProvider)
	if provider == "" {
		provider = BotSalesPaymentProvider
	}
	return map[string]any{
		"schema_version":          1,
		"source":                  "bot-sales",
		"provider_key":            provider,
		"out_trade_no":            outTradeNo,
		"external_order_id":       req.ExternalOrderID,
		"external_order_item_id":  req.ExternalOrderItemID,
		"external_payment_id":     externalPaymentID,
		"payment_provider_txn_id": strings.TrimSpace(req.PaymentProviderTxnID),
		"payment_amount":          paymentAmount,
		"payment_amount_source":   paymentAmountSource,
		"payment_currency":        currency,
		"balance_package_code":    pkg.Code,
		"quantity":                botSalesQuantity(req),
		"operation":               req.Operation,
	}
}

func int64PtrOrNil(v int64) *int64 {
	if v <= 0 {
		return nil
	}
	return &v
}

func (s *BotSalesFulfillmentService) ensureBotSalesDeviceCode(ctx context.Context, buyer *User, req BotSalesTokenFulfillmentRequest) (string, error) {
	if s.userDeviceRepo == nil || s.entClient == nil {
		return "", infraerrors.ServiceUnavailable("BOT_SALES_DEVICE_ISSUE_UNAVAILABLE", "device issue service is not available")
	}
	deviceHash := botSalesDeviceHash(req.ExternalOrderID, buyer.ID)
	existing, err := s.entClient.UserDevice.Query().Where(userdevice.DeviceHashEQ(deviceHash)).Only(ctx)
	if err == nil && existing.DeviceCode != nil {
		return *existing.DeviceCode, nil
	}
	if err != nil && !dbent.IsNotFound(err) {
		return "", err
	}
	return s.issueBotSalesDeviceCode(ctx, buyer, req)
}

func (s *BotSalesFulfillmentService) resolveBotSalesDevice(ctx context.Context, rawCode string) (*UserDevice, error) {
	if s.userDeviceRepo == nil {
		return nil, infraerrors.ServiceUnavailable("BOT_SALES_DEVICE_LOOKUP_UNAVAILABLE", "device lookup service is not available")
	}
	code := NormalizeRedeemCode(rawCode)
	if code == "" || !strings.HasPrefix(code, "DLG-") {
		return nil, infraerrors.BadRequest("BOT_SALES_DEVICE_CODE_REQUIRED", "device_code is required for balance topup")
	}
	device, err := s.userDeviceRepo.GetByDeviceCode(ctx, code)
	if err != nil {
		if errors.Is(err, ErrUserDeviceNotFound) {
			return nil, infraerrors.NotFound("BOT_SALES_DEVICE_NOT_FOUND", "device_code was not found")
		}
		return nil, err
	}
	if device == nil || device.UserID <= 0 {
		return nil, infraerrors.NotFound("BOT_SALES_DEVICE_NOT_FOUND", "device_code was not found")
	}
	return device, nil
}

func (s *BotSalesFulfillmentService) issueBotSalesDeviceCode(ctx context.Context, buyer *User, req BotSalesTokenFulfillmentRequest) (string, error) {
	if s.userDeviceRepo == nil {
		return "", infraerrors.ServiceUnavailable("BOT_SALES_DEVICE_ISSUE_UNAVAILABLE", "device issue service is not available")
	}
	claimedAt := time.Now().UTC()
	deviceHash := botSalesDeviceHash(req.ExternalOrderID, buyer.ID)
	for attempt := 0; attempt < 5; attempt++ {
		code, err := GenerateRedeemCodeForType(RedeemTypeDeviceLogin)
		if err != nil {
			return "", err
		}
		device := &UserDevice{
			UserID:             buyer.ID,
			DeviceCode:         &code,
			DeviceHash:         deviceHash,
			FingerprintVersion: 1,
			Platform:           "bot-sales",
			Arch:               "api",
			Status:             UserDeviceStatusActive,
			FirstClaimedAt:     claimedAt,
			LastClaimedAt:      &claimedAt,
		}
		if err := s.userDeviceRepo.Create(ctx, device); err != nil {
			var constraintErr *dbent.ConstraintError
			if errors.As(err, &constraintErr) && strings.Contains(strings.ToLower(err.Error()), "device_code") {
				continue
			}
			return "", err
		}
		return code, nil
	}
	return "", infraerrors.ServiceUnavailable("BOT_SALES_DEVICE_CODE_COLLISION", "could not issue a unique device code")
}

func (s *BotSalesFulfillmentService) findOrCreateBuyer(ctx context.Context, buyer BotSalesFulfillmentBuyer) (*User, error) {
	email := strings.TrimSpace(buyer.Email)
	lookupEmails := []string{email}
	if email == "" {
		lookupEmails = botSalesBuyerLookupEmails(buyer)
		email = lookupEmails[0]
	}
	for _, lookupEmail := range lookupEmails {
		if lookupEmail == "" {
			continue
		}
		existing, err := s.userRepo.GetByEmail(ctx, lookupEmail)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, ErrUserNotFound) {
			return nil, err
		}
	}

	username := strings.TrimSpace(buyer.Username)
	if username == "" {
		username = strings.TrimSpace(buyer.DisplayName)
	}
	if username == "" {
		username = email
	}
	defaultConcurrency, defaultRPMLimit := s.botSalesBuyerDefaults(ctx)
	created := &User{
		Email:        email,
		Username:     username,
		PasswordHash: fmt.Sprintf("bot-sales:%d", time.Now().UnixNano()),
		Role:         RoleUser,
		Status:       StatusActive,
		Concurrency:  defaultConcurrency,
		RPMLimit:     defaultRPMLimit,
	}
	if err := s.userRepo.Create(ctx, created); err != nil {
		return nil, err
	}
	return created, nil
}

func (s *BotSalesFulfillmentService) botSalesBuyerDefaults(ctx context.Context) (int, int) {
	defaultConcurrency := dbuser.DefaultConcurrency
	defaultRPMLimit := dbuser.DefaultRpmLimit
	if s != nil && s.settingService != nil {
		defaultConcurrency = s.settingService.GetDefaultConcurrency(ctx)
		defaultRPMLimit = s.settingService.GetDefaultUserRPMLimit(ctx)
	}
	return defaultConcurrency, defaultRPMLimit
}

func userServiceUserRepo(userService *UserService) UserRepository {
	if userService == nil {
		return nil
	}
	return userService.userRepo
}

func botSalesBuyerLookupEmails(buyer BotSalesFulfillmentBuyer) []string {
	candidates := []string{buyer.ExternalUserID}
	provider := strings.TrimSpace(strings.ToLower(buyer.Provider))
	providerUserID := strings.TrimSpace(buyer.ProviderUserID)
	if parsedProvider, parsedUserID := botSalesParseChannelUserID(buyer.ExternalUserID); parsedProvider != "" && parsedUserID != "" {
		if provider == "" {
			provider = parsedProvider
		}
		if providerUserID == "" {
			providerUserID = parsedUserID
		}
	}
	telegramID := strings.TrimSpace(buyer.TelegramID)
	if providerUserID == "" && provider == "telegram" {
		providerUserID = telegramID
	}
	if provider != "" && providerUserID != "" {
		candidates = append(candidates, provider+":"+providerUserID)
	}
	if telegramID != "" {
		candidates = append(candidates, "telegram:"+telegramID, "tg:"+telegramID)
	}

	emails := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		email := botSalesSyntheticEmail(candidate)
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		emails = append(emails, email)
	}
	if len(emails) == 0 {
		return []string{botSalesSyntheticEmail(buyer.ExternalUserID)}
	}
	return emails
}

func botSalesParseChannelUserID(externalUserID string) (string, string) {
	value := strings.TrimSpace(externalUserID)
	if !strings.HasPrefix(value, "channel:") {
		return "", ""
	}
	parts := strings.SplitN(strings.TrimPrefix(value, "channel:"), ":user:", 2)
	if len(parts) != 2 {
		return "", ""
	}
	provider := strings.TrimSpace(strings.ToLower(parts[0]))
	providerUserID := strings.TrimSpace(parts[1])
	if provider == "" || providerUserID == "" {
		return "", ""
	}
	return provider, providerUserID
}

func botSalesSyntheticEmail(externalUserID string) string {
	local := strings.TrimSpace(strings.ToLower(externalUserID))
	local = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, local)
	local = strings.Trim(local, "-")
	if local == "" {
		local = "buyer"
	}
	return local + "@bot-sales.local"
}

func botSalesDeviceHash(externalOrderID string, userID int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("bot-sales:%s:%d", strings.TrimSpace(externalOrderID), userID)))
	return hex.EncodeToString(sum[:])
}

func botSalesNotes(req BotSalesTokenFulfillmentRequest) string {
	parts := []string{"bot-sales", "external_order_id=" + req.ExternalOrderID}
	if req.ExternalPaymentID != "" {
		parts = append(parts, "external_payment_id="+req.ExternalPaymentID)
	}
	if req.Affiliate != nil && strings.TrimSpace(req.Affiliate.AffCode) != "" {
		parts = append(parts, "aff_code="+strings.TrimSpace(req.Affiliate.AffCode))
	}
	return strings.Join(parts, ";")
}
