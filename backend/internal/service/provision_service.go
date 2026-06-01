package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrProvisionPlanNotFound    = infraerrors.NotFound("PROVISION_PLAN_NOT_FOUND", "provision plan not found")
	ErrProvisionPlanExists      = infraerrors.Conflict("PROVISION_PLAN_EXISTS", "provision plan already exists")
	ErrProvisionPlanDisabled    = infraerrors.BadRequest("PROVISION_PLAN_DISABLED", "provision plan is disabled")
	ErrProvisionPlanInvalid     = infraerrors.BadRequest("PROVISION_PLAN_INVALID", "provision plan is invalid")
	ErrProvisionOrderNotFound   = infraerrors.NotFound("PROVISION_ORDER_NOT_FOUND", "provision order not found")
	ErrProvisionOrderExists     = infraerrors.Conflict("PROVISION_ORDER_EXISTS", "provision order already exists")
	ErrProvisionOrderProcessing = infraerrors.Conflict("PROVISION_ORDER_PROCESSING", "provision order is still processing")
	ErrProvisionOrderIncomplete = infraerrors.Conflict("PROVISION_ORDER_INCOMPLETE", "provision order is incomplete")
)

const (
	ProvisionOrderStatusProcessing = "processing"
	ProvisionOrderStatusCompleted  = "completed"
)

type ProvisionRepository interface {
	ListPlans(ctx context.Context) ([]ProvisionPlan, error)
	GetPlanByID(ctx context.Context, id int64) (*ProvisionPlan, error)
	GetPlanByCode(ctx context.Context, code string) (*ProvisionPlan, error)
	CreatePlan(ctx context.Context, input ProvisionPlanInput) (*ProvisionPlan, error)
	UpdatePlan(ctx context.Context, id int64, input ProvisionPlanInput) (*ProvisionPlan, error)
	DeletePlan(ctx context.Context, id int64) error
	GetResultByOrderID(ctx context.Context, orderID string) (*ProvisionResult, error)
	CreateOrderProcessing(ctx context.Context, order *ProvisionOrder) (*ProvisionOrder, error)
	CompleteOrder(ctx context.Context, orderID string, userID, apiKeyID int64, snapshot ProvisionSnapshot) error
}

type ProvisionService struct {
	repo          ProvisionRepository
	userRepo      UserRepository
	groupRepo     GroupRepository
	apiKeyService *APIKeyService
	entClient     *dbent.Client
}

func NewProvisionService(repo ProvisionRepository, userRepo UserRepository, groupRepo GroupRepository, apiKeyService *APIKeyService, entClient *dbent.Client) *ProvisionService {
	return &ProvisionService{
		repo:          repo,
		userRepo:      userRepo,
		groupRepo:     groupRepo,
		apiKeyService: apiKeyService,
		entClient:     entClient,
	}
}

func (s *ProvisionService) ListPlans(ctx context.Context) ([]ProvisionPlan, error) {
	return s.repo.ListPlans(ctx)
}

func (s *ProvisionService) CreatePlan(ctx context.Context, input ProvisionPlanInput) (*ProvisionPlan, error) {
	normalized, err := s.normalizePlanInput(ctx, input)
	if err != nil {
		return nil, err
	}
	return s.repo.CreatePlan(ctx, normalized)
}

func (s *ProvisionService) UpdatePlan(ctx context.Context, id int64, input ProvisionPlanInput) (*ProvisionPlan, error) {
	if id <= 0 {
		return nil, infraerrors.BadRequest("PROVISION_PLAN_ID_INVALID", "invalid provision plan id")
	}
	normalized, err := s.normalizePlanInput(ctx, input)
	if err != nil {
		return nil, err
	}
	return s.repo.UpdatePlan(ctx, id, normalized)
}

func (s *ProvisionService) DeletePlan(ctx context.Context, id int64) error {
	if id <= 0 {
		return infraerrors.BadRequest("PROVISION_PLAN_ID_INVALID", "invalid provision plan id")
	}
	return s.repo.DeletePlan(ctx, id)
}

func (s *ProvisionService) ProvisionAPIKey(ctx context.Context, input ProvisionAPIKeyInput) (*ProvisionResult, error) {
	orderID := strings.TrimSpace(input.OrderID)
	planCode := normalizeProvisionCode(input.PlanCode)
	if orderID == "" {
		return nil, infraerrors.BadRequest("PROVISION_ORDER_ID_REQUIRED", "order_id is required")
	}
	if len(orderID) > 128 {
		return nil, infraerrors.BadRequest("PROVISION_ORDER_ID_TOO_LONG", "order_id must be at most 128 characters")
	}
	if planCode == "" {
		return nil, infraerrors.BadRequest("PROVISION_PLAN_CODE_REQUIRED", "plan_code is required")
	}

	if existing, err := s.repo.GetResultByOrderID(ctx, orderID); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrProvisionOrderNotFound) {
		return nil, err
	}

	plan, err := s.repo.GetPlanByCode(ctx, planCode)
	if err != nil {
		return nil, err
	}
	if !plan.Enabled {
		return nil, ErrProvisionPlanDisabled
	}
	group, err := s.validateProvisionGroup(ctx, plan.GroupID)
	if err != nil {
		return nil, err
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin provision transaction: %w", err)
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	defer func() { _ = tx.Rollback() }()

	snapshot := snapshotProvisionPlan(*plan, group)
	order := &ProvisionOrder{
		OrderID:       orderID,
		PlanID:        &plan.ID,
		PlanCode:      plan.Code,
		PlanSnapshot:  snapshot,
		Status:        ProvisionOrderStatusProcessing,
		CustomerLabel: strings.TrimSpace(input.CustomerLabel),
	}
	if _, err := s.repo.CreateOrderProcessing(txCtx, order); err != nil {
		if errors.Is(err, ErrProvisionOrderExists) {
			if existing, getErr := s.repo.GetResultByOrderID(ctx, orderID); getErr == nil {
				return existing, nil
			}
		}
		return nil, err
	}

	password, err := generateProvisionPassword()
	if err != nil {
		return nil, err
	}
	user := &User{
		Email:         provisionEmailForOrder(orderID),
		Username:      strings.TrimSpace(input.CustomerLabel),
		Notes:         provisionUserNotes(orderID, plan.Code, input.CustomerLabel),
		Role:          RoleUser,
		Balance:       plan.Balance,
		Concurrency:   plan.Concurrency,
		RPMLimit:      plan.RPMLimit,
		Status:        StatusActive,
		AllowedGroups: []int64{plan.GroupID},
	}
	if err := user.SetPassword(password); err != nil {
		return nil, err
	}
	if err := s.userRepo.Create(txCtx, user); err != nil {
		return nil, fmt.Errorf("create provision user: %w", err)
	}

	keyName := orderID
	if keyName == "" {
		keyName = plan.Code
	}
	apiKey, err := s.apiKeyService.Create(txCtx, user.ID, CreateAPIKeyRequest{
		Name:          truncateProvisionName(keyName, 100),
		GroupID:       &plan.GroupID,
		Quota:         plan.Quota,
		ExpiresInDays: plan.ExpiresInDays,
		RateLimit5h:   plan.RateLimit5h,
		RateLimit1d:   plan.RateLimit1d,
		RateLimit7d:   plan.RateLimit7d,
	})
	if err != nil {
		return nil, fmt.Errorf("create provision api key: %w", err)
	}

	if err := s.repo.CompleteOrder(txCtx, orderID, user.ID, apiKey.ID, snapshot); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit provision transaction: %w", err)
	}

	return &ProvisionResult{
		OrderID:        orderID,
		APIKey:         apiKey.Key,
		KeyID:          apiKey.ID,
		UserID:         user.ID,
		PlanCode:       plan.Code,
		GroupID:        plan.GroupID,
		Balance:        plan.Balance,
		Quota:          plan.Quota,
		RateMultiplier: group.RateMultiplier,
	}, nil
}

func (s *ProvisionService) normalizePlanInput(ctx context.Context, input ProvisionPlanInput) (ProvisionPlanInput, error) {
	input.Code = normalizeProvisionCode(input.Code)
	input.Name = strings.TrimSpace(input.Name)
	if input.Code == "" {
		return input, infraerrors.BadRequest("PROVISION_PLAN_CODE_REQUIRED", "code is required")
	}
	if len(input.Code) > 64 {
		return input, infraerrors.BadRequest("PROVISION_PLAN_CODE_TOO_LONG", "code must be at most 64 characters")
	}
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`).MatchString(input.Code) {
		return input, infraerrors.BadRequest("PROVISION_PLAN_CODE_INVALID", "code may only contain lowercase letters, numbers, underscores, and hyphens")
	}
	if input.Name == "" {
		return input, infraerrors.BadRequest("PROVISION_PLAN_NAME_REQUIRED", "name is required")
	}
	if input.GroupID <= 0 {
		return input, infraerrors.BadRequest("PROVISION_PLAN_GROUP_REQUIRED", "group_id is required")
	}
	if input.Balance <= 0 {
		return input, infraerrors.BadRequest("PROVISION_PLAN_BALANCE_INVALID", "balance must be greater than zero")
	}
	if input.Quota < 0 || input.RateLimit5h < 0 || input.RateLimit1d < 0 || input.RateLimit7d < 0 {
		return input, infraerrors.BadRequest("PROVISION_PLAN_LIMIT_INVALID", "quota and rate limits must be greater than or equal to zero")
	}
	if input.ExpiresInDays != nil && *input.ExpiresInDays <= 0 {
		return input, infraerrors.BadRequest("PROVISION_PLAN_EXPIRY_INVALID", "expires_in_days must be greater than zero")
	}
	if input.Concurrency <= 0 {
		input.Concurrency = 5
	}
	if input.RPMLimit < 0 {
		return input, infraerrors.BadRequest("PROVISION_PLAN_RPM_INVALID", "rpm_limit must be greater than or equal to zero")
	}
	if _, err := s.validateProvisionGroup(ctx, input.GroupID); err != nil {
		return input, err
	}
	return input, nil
}

func (s *ProvisionService) validateProvisionGroup(ctx context.Context, groupID int64) (*Group, error) {
	group, err := s.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("get provision group: %w", err)
	}
	if !group.IsActive() {
		return nil, infraerrors.BadRequest("PROVISION_GROUP_INACTIVE", "provision group must be active")
	}
	if group.IsSubscriptionType() {
		return nil, infraerrors.BadRequest("PROVISION_GROUP_SUBSCRIPTION_UNSUPPORTED", "provision group must be a standard group")
	}
	if !group.IsExclusive {
		return nil, infraerrors.BadRequest("PROVISION_GROUP_NOT_EXCLUSIVE", "provision group must be exclusive")
	}
	return group, nil
}

func snapshotProvisionPlan(plan ProvisionPlan, group *Group) ProvisionSnapshot {
	return ProvisionSnapshot{
		PlanID:         plan.ID,
		PlanCode:       plan.Code,
		PlanName:       plan.Name,
		GroupID:        plan.GroupID,
		GroupName:      group.Name,
		Platform:       group.Platform,
		RateMultiplier: group.RateMultiplier,
		Balance:        plan.Balance,
		Quota:          plan.Quota,
		ExpiresInDays:  plan.ExpiresInDays,
		RateLimit5h:    plan.RateLimit5h,
		RateLimit1d:    plan.RateLimit1d,
		RateLimit7d:    plan.RateLimit7d,
		Concurrency:    plan.Concurrency,
		RPMLimit:       plan.RPMLimit,
	}
}

func normalizeProvisionCode(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}

func provisionEmailForOrder(orderID string) string {
	safe := strings.ToLower(strings.TrimSpace(orderID))
	var b strings.Builder
	for _, r := range safe {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	local := strings.Trim(b.String(), "-_.")
	if local == "" {
		local = "order"
	}
	sum := sha256.Sum256([]byte(orderID))
	suffix := hex.EncodeToString(sum[:])[:10]
	maxLocal := 64 - len("order_") - len(suffix) - 1
	if len(local) > maxLocal {
		local = local[:maxLocal]
	}
	return fmt.Sprintf("order_%s_%s@provision.local", local, suffix)
}

func provisionUserNotes(orderID, planCode, customerLabel string) string {
	notes := map[string]string{
		"source":    "provision",
		"order_id":  orderID,
		"plan_code": planCode,
	}
	if strings.TrimSpace(customerLabel) != "" {
		notes["customer_label"] = strings.TrimSpace(customerLabel)
	}
	raw, _ := json.Marshal(notes)
	return string(raw)
}

func truncateProvisionName(name string, max int) string {
	name = strings.TrimSpace(name)
	if len(name) <= max {
		return name
	}
	return name[:max]
}

func generateProvisionPassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate provision password: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
