package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// --- Provider Instance CRUD ---

func (s *PaymentConfigService) ListProviderInstances(ctx context.Context) ([]*dbent.PaymentProviderInstance, error) {
	return s.entClient.PaymentProviderInstance.Query().Order(paymentproviderinstance.BySortOrder()).All(ctx)
}

// ProviderInstanceResponse is the API response for a provider instance.
type ProviderInstanceResponse struct {
	ID             int64             `json:"id"`
	ProviderKey    string            `json:"provider_key"`
	Name           string            `json:"name"`
	Config         map[string]string `json:"config"`
	SupportedTypes []string          `json:"supported_types"`
	Limits         string            `json:"limits"`
	Enabled        bool              `json:"enabled"`
	RefundEnabled  bool              `json:"refund_enabled"`
	SortOrder      int               `json:"sort_order"`
	PaymentMode    string            `json:"payment_mode"`
}

// ListProviderInstancesWithConfig returns provider instances with decrypted config.
func (s *PaymentConfigService) ListProviderInstancesWithConfig(ctx context.Context) ([]ProviderInstanceResponse, error) {
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Order(paymentproviderinstance.BySortOrder()).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]ProviderInstanceResponse, 0, len(instances))
	for _, inst := range instances {
		resp := ProviderInstanceResponse{
			ID: int64(inst.ID), ProviderKey: inst.ProviderKey, Name: inst.Name,
			SupportedTypes: splitTypes(inst.SupportedTypes), Limits: inst.Limits,
			Enabled: inst.Enabled, RefundEnabled: inst.RefundEnabled, SortOrder: inst.SortOrder,
			PaymentMode: inst.PaymentMode,
		}
		resp.Config, err = s.decryptAndMaskConfig(inst.Config)
		if err != nil {
			return nil, fmt.Errorf("decrypt config for instance %d: %w", inst.ID, err)
		}
		result = append(result, resp)
	}
	return result, nil
}

func (s *PaymentConfigService) decryptAndMaskConfig(encrypted string) (map[string]string, error) {
	return s.decryptConfig(encrypted)
}

// pendingOrderStatuses are order statuses considered "in progress".
var pendingOrderStatuses = []string{
	payment.OrderStatusPending,
	payment.OrderStatusPaid,
	payment.OrderStatusRecharging,
}

var sensitiveConfigPatterns = []string{"key", "pkey", "secret", "private", "password"}

func isSensitiveConfigField(fieldName string) bool {
	lower := strings.ToLower(fieldName)
	for _, p := range sensitiveConfigPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func (s *PaymentConfigService) countPendingOrders(ctx context.Context, providerInstanceID int64) (int, error) {
	return s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.ProviderInstanceIDEQ(strconv.FormatInt(providerInstanceID, 10)),
			paymentorder.StatusIn(pendingOrderStatuses...),
		).Count(ctx)
}

func (s *PaymentConfigService) countPendingOrdersByPlan(ctx context.Context, planID int64) (int, error) {
	return s.entClient.PaymentOrder.Query().
		Where(
			paymentorder.PlanIDEQ(planID),
			paymentorder.StatusIn(pendingOrderStatuses...),
		).Count(ctx)
}

var validProviderKeys = map[string]bool{
	payment.TypeEasyPay: true, payment.TypeAlipay: true, payment.TypeWxpay: true, payment.TypeStripe: true,
}

func (s *PaymentConfigService) CreateProviderInstance(ctx context.Context, req CreateProviderInstanceRequest) (*dbent.PaymentProviderInstance, error) {
	typesStr := joinTypes(req.SupportedTypes)
	if err := validateProviderRequest(req.ProviderKey, req.Name, typesStr); err != nil {
		return nil, err
	}
	enc, err := s.encryptConfig(req.Config)
	if err != nil {
		return nil, err
	}
	return s.entClient.PaymentProviderInstance.Create().
		SetProviderKey(req.ProviderKey).SetName(req.Name).SetConfig(enc).
		SetSupportedTypes(typesStr).SetEnabled(req.Enabled).SetPaymentMode(req.PaymentMode).
		SetSortOrder(req.SortOrder).SetLimits(req.Limits).SetRefundEnabled(req.RefundEnabled).
		Save(ctx)
}

func validateProviderRequest(providerKey, name, supportedTypes string) error {
	if strings.TrimSpace(name) == "" {
		return infraerrors.BadRequest("VALIDATION_ERROR", "provider name is required")
	}
	if !validProviderKeys[providerKey] {
		return infraerrors.BadRequest("VALIDATION_ERROR", fmt.Sprintf("invalid provider key: %s", providerKey))
	}
	// supported_types can be empty (provider accepts no payment types until configured)
	return nil
}

// UpdateProviderInstance updates a provider instance by ID (patch semantics).
// NOTE: This function exceeds 30 lines due to per-field nil-check patch update
// boilerplate and pending-order safety checks.
func (s *PaymentConfigService) UpdateProviderInstance(ctx context.Context, id int64, req UpdateProviderInstanceRequest) (*dbent.PaymentProviderInstance, error) {
	if req.Config != nil {
		hasSensitive := false
		for k := range req.Config {
			if isSensitiveConfigField(k) && req.Config[k] != "" {
				hasSensitive = true
				break
			}
		}
		if hasSensitive {
			count, err := s.countPendingOrders(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("check pending orders: %w", err)
			}
			if count > 0 {
				return nil, infraerrors.Conflict("PENDING_ORDERS", "instance has pending orders").
					WithMetadata(map[string]string{"count": strconv.Itoa(count)})
			}
		}
	}
	if req.Enabled != nil && !*req.Enabled {
		count, err := s.countPendingOrders(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("check pending orders: %w", err)
		}
		if count > 0 {
			return nil, infraerrors.Conflict("PENDING_ORDERS", "instance has pending orders").
				WithMetadata(map[string]string{"count": strconv.Itoa(count)})
		}
	}
	u := s.entClient.PaymentProviderInstance.UpdateOneID(id)
	if req.Name != nil {
		u.SetName(*req.Name)
	}
	if req.Config != nil {
		merged, err := s.mergeConfig(ctx, id, req.Config)
		if err != nil {
			return nil, err
		}
		enc, err := s.encryptConfig(merged)
		if err != nil {
			return nil, err
		}
		u.SetConfig(enc)
	}
	if req.SupportedTypes != nil {
		// Check pending orders before removing payment types
		count, err := s.countPendingOrders(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("check pending orders: %w", err)
		}
		if count > 0 {
			// Load current instance to compare types
			inst, err := s.entClient.PaymentProviderInstance.Get(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("load provider instance: %w", err)
			}
			oldTypes := strings.Split(inst.SupportedTypes, ",")
			newTypes := req.SupportedTypes
			for _, ot := range oldTypes {
				ot = strings.TrimSpace(ot)
				if ot == "" {
					continue
				}
				found := false
				for _, nt := range newTypes {
					if strings.TrimSpace(nt) == ot {
						found = true
						break
					}
				}
				if !found {
					return nil, infraerrors.Conflict("PENDING_ORDERS", "cannot remove payment types while instance has pending orders").
						WithMetadata(map[string]string{"count": strconv.Itoa(count)})
				}
			}
		}
		u.SetSupportedTypes(joinTypes(req.SupportedTypes))
	}
	if req.Enabled != nil {
		u.SetEnabled(*req.Enabled)
	}
	if req.SortOrder != nil {
		u.SetSortOrder(*req.SortOrder)
	}
	if req.Limits != nil {
		u.SetLimits(*req.Limits)
	}
	if req.RefundEnabled != nil {
		u.SetRefundEnabled(*req.RefundEnabled)
	}
	if req.PaymentMode != nil {
		u.SetPaymentMode(*req.PaymentMode)
	}
	return u.Save(ctx)
}

func (s *PaymentConfigService) mergeConfig(ctx context.Context, id int64, newConfig map[string]string) (map[string]string, error) {
	inst, err := s.entClient.PaymentProviderInstance.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load existing provider: %w", err)
	}
	existing, err := s.decryptConfig(inst.Config)
	if err != nil {
		return nil, fmt.Errorf("decrypt existing config for instance %d: %w", id, err)
	}
	if existing == nil {
		return newConfig, nil
	}
	for k, v := range newConfig {
		existing[k] = v
	}
	return existing, nil
}

func (s *PaymentConfigService) decryptConfig(encrypted string) (map[string]string, error) {
	if encrypted == "" {
		return nil, nil
	}
	decrypted, err := payment.Decrypt(encrypted, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt config: %w", err)
	}
	var raw map[string]string
	if err := json.Unmarshal([]byte(decrypted), &raw); err != nil {
		return nil, fmt.Errorf("unmarshal decrypted config: %w", err)
	}
	return raw, nil
}

func (s *PaymentConfigService) DeleteProviderInstance(ctx context.Context, id int64) error {
	count, err := s.countPendingOrders(ctx, id)
	if err != nil {
		return fmt.Errorf("check pending orders: %w", err)
	}
	if count > 0 {
		return infraerrors.Conflict("PENDING_ORDERS",
			fmt.Sprintf("this instance has %d in-progress orders and cannot be deleted — wait for orders to complete or disable the instance first", count))
	}
	return s.entClient.PaymentProviderInstance.DeleteOneID(id).Exec(ctx)
}

func (s *PaymentConfigService) encryptConfig(cfg map[string]string) (string, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}
	enc, err := payment.Encrypt(string(data), s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("encrypt config: %w", err)
	}
	return enc, nil
}
