package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/balancepackage"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
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
)

// SubscriptionTypeNone is accepted for non-subscription groups used by balance packages.
const SubscriptionTypeNone = "none"

type BotSalesFulfillmentBuyer struct {
	ExternalUserID string `json:"external_user_id"`
	Email          string `json:"email"`
	Username       string `json:"username"`
	DisplayName    string `json:"display_name"`
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
	ExternalOrderID    string                        `json:"external_order_id"`
	ExternalPaymentID  string                        `json:"external_payment_id"`
	Operation          string                        `json:"operation"`
	EntitlementKind    string                        `json:"entitlement_kind"`
	PlanID             int64                         `json:"plan_id"`
	BalancePackageCode string                        `json:"balance_package_code"`
	DeviceCode         string                        `json:"device_code"`
	Buyer              BotSalesFulfillmentBuyer      `json:"buyer"`
	Affiliate          *BotSalesFulfillmentAffiliate `json:"affiliate"`
	DeliveryPolicy     BotSalesDeliveryPolicy        `json:"delivery_policy"`
	RawPayload         map[string]any                `json:"-"`
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
	subscriptionService *SubscriptionService
	apiKeyService       *APIKeyService
	userDeviceRepo      UserDeviceRepository
}

func NewBotSalesFulfillmentService(
	entClient *dbent.Client,
	userService *UserService,
	subscriptionService *SubscriptionService,
	apiKeyService *APIKeyService,
	userDeviceRepo UserDeviceRepository,
) *BotSalesFulfillmentService {
	return &BotSalesFulfillmentService{
		entClient:           entClient,
		userRepo:            userServiceUserRepo(userService),
		userService:         userService,
		subscriptionService: subscriptionService,
		apiKeyService:       apiKeyService,
		userDeviceRepo:      userDeviceRepo,
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
	if shouldIssueBotSalesAPIKey(req.DeliveryPolicy) && s.apiKeyService != nil {
		apiKey, err := s.apiKeyService.Create(ctx, apiKeyUserID, CreateAPIKeyRequest{
			Name:    fmt.Sprintf("bot-sales-%s", req.ExternalOrderID),
			GroupID: &resp.Entitlement.GroupID,
		})
		if err != nil {
			return nil, err
		}
		resp.Delivery.APIKey = &BotSalesDeliveryAPIKey{ID: apiKey.ID, Key: apiKey.Key, GroupID: apiKey.GroupID}
	}

	return resp, nil
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

	targetBuyer := buyer
	var deviceCode string
	if operation == BotSalesFulfillmentOperationTopup {
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
	if err := s.userService.UpdateBalance(ctx, targetBuyer.ID, pkg.AmountLedger); err != nil {
		return err
	}
	if operation == BotSalesFulfillmentOperationNew {
		issuedDeviceCode, err := s.issueBotSalesDeviceCode(ctx, targetBuyer, req)
		if err != nil {
			return err
		}
		deviceCode = issuedDeviceCode
	}
	groupID := int64(0)
	if pkg.GroupID != nil {
		groupID = *pkg.GroupID
	}

	resp.Operation = operation
	resp.EntitlementKind = BotSalesEntitlementBalance
	resp.Buyer = BotSalesFulfillmentBuyerResult{UserID: targetBuyer.ID, ExternalUserID: req.Buyer.ExternalUserID, Email: targetBuyer.Email}
	resp.Entitlement = BotSalesFulfillmentEntitlement{
		Kind:               BotSalesEntitlementBalance,
		BalancePackageCode: &code,
		GroupID:            groupID,
		BalanceCredited:    pkg.AmountLedger,
	}
	resp.Balance = BotSalesFulfillmentBalance{GroupID: groupID, AmountLedger: pkg.AmountLedger, ActualCredits: pkg.ActualCredits}
	resp.Delivery.DeviceCode = deviceCode
	resp.DeviceCode = deviceCode
	return nil
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
	if email == "" {
		email = botSalesSyntheticEmail(buyer.ExternalUserID)
	}
	existing, err := s.userRepo.GetByEmail(ctx, email)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrUserNotFound) {
		return nil, err
	}

	username := strings.TrimSpace(buyer.Username)
	if username == "" {
		username = strings.TrimSpace(buyer.DisplayName)
	}
	if username == "" {
		username = email
	}
	created := &User{
		Email:        email,
		Username:     username,
		PasswordHash: fmt.Sprintf("bot-sales:%d", time.Now().UnixNano()),
		Role:         RoleUser,
		Status:       StatusActive,
		Concurrency:  1,
	}
	if err := s.userRepo.Create(ctx, created); err != nil {
		return nil, err
	}
	return created, nil
}

func userServiceUserRepo(userService *UserService) UserRepository {
	if userService == nil {
		return nil
	}
	return userService.userRepo
}

func shouldIssueBotSalesAPIKey(policy BotSalesDeliveryPolicy) bool {
	switch strings.TrimSpace(policy.IssueAPIKey) {
	case "", BotSalesIssueAPIKeyAlways, BotSalesIssueAPIKeyIfMissing:
		return true
	case BotSalesIssueAPIKeyNever:
		return false
	default:
		return false
	}
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
