package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
)

// Strategy represents a load balancing strategy for provider instance selection.
type Strategy string

const (
	StrategyRoundRobin  Strategy = "round-robin"
	StrategyLeastAmount Strategy = "least-amount"
)

// ChannelLimits holds limits for a single payment channel within a provider instance.
type ChannelLimits struct {
	DailyLimit float64 `json:"dailyLimit,omitempty"`
	SingleMin  float64 `json:"singleMin,omitempty"`
	SingleMax  float64 `json:"singleMax,omitempty"`
}

// InstanceLimits holds per-channel limits for a provider instance (JSON).
type InstanceLimits map[string]ChannelLimits

// LoadBalancer selects a provider instance for a given payment type.
type LoadBalancer interface {
	GetInstanceConfig(ctx context.Context, instanceID int64) (map[string]string, error)
	SelectInstance(ctx context.Context, providerKey string, paymentType PaymentType, strategy Strategy, orderAmount float64) (*InstanceSelection, error)
}

// DefaultLoadBalancer implements LoadBalancer using database queries.
type DefaultLoadBalancer struct {
	db            *dbent.Client
	encryptionKey []byte
	counter       atomic.Uint64
}

// NewDefaultLoadBalancer creates a new load balancer.
func NewDefaultLoadBalancer(db *dbent.Client, encryptionKey []byte) *DefaultLoadBalancer {
	return &DefaultLoadBalancer{db: db, encryptionKey: encryptionKey}
}

// SelectInstance picks an enabled instance for the given provider key and payment type.
func (lb *DefaultLoadBalancer) SelectInstance(ctx context.Context, providerKey string, paymentType PaymentType, strategy Strategy, orderAmount float64) (*InstanceSelection, error) {
	candidates, err := lb.getCandidates(ctx, providerKey, paymentType, orderAmount)
	if err != nil {
		return nil, err
	}

	selected, err := lb.pickByStrategy(ctx, candidates, strategy)
	if err != nil {
		return nil, err
	}

	return lb.buildSelection(selected)
}

func (lb *DefaultLoadBalancer) getCandidates(ctx context.Context, providerKey string, paymentType PaymentType, orderAmount float64) ([]*dbent.PaymentProviderInstance, error) {
	instances, err := lb.db.PaymentProviderInstance.Query().
		Where(
			paymentproviderinstance.ProviderKey(providerKey),
			paymentproviderinstance.Enabled(true),
		).
		Order(dbent.Asc(paymentproviderinstance.FieldSortOrder)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query provider instances: %w", err)
	}

	// Filter by supported types.
	// When paymentType equals providerKey (e.g. "stripe"), all instances of that
	// provider are candidates — the sub-type filtering is handled internally.
	var candidates []*dbent.PaymentProviderInstance
	for _, inst := range instances {
		if paymentType == providerKey || InstanceSupportsType(inst.SupportedTypes, paymentType) {
			candidates = append(candidates, inst)
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no enabled instance for provider %s type %s", providerKey, paymentType)
	}

	// Exclude instances that cannot accommodate this order (daily limit or single amount range).
	filtered := lb.filterByLimits(ctx, candidates, paymentType, orderAmount)
	if len(filtered) > 0 {
		return filtered, nil
	}
	// All instances exhausted — fall back to full candidate list so the order
	// can still be attempted (the provider itself may reject it).
	slog.Warn("all instances exceeded limits, using full candidate list",
		"provider", providerKey, "payment_type", paymentType,
		"order_amount", orderAmount, "count", len(candidates))
	return candidates, nil
}

// filterByLimits removes instances that cannot accommodate the order:
//   - daily remaining capacity < orderAmount
//   - orderAmount outside single-transaction min/max range
func (lb *DefaultLoadBalancer) filterByLimits(ctx context.Context, candidates []*dbent.PaymentProviderInstance, paymentType PaymentType, orderAmount float64) []*dbent.PaymentProviderInstance {
	var filtered []*dbent.PaymentProviderInstance
	for _, inst := range candidates {
		cl := getInstanceChannelLimits(inst, paymentType)

		// Check single-transaction range.
		if cl.SingleMin > 0 && orderAmount < cl.SingleMin {
			slog.Info("order below instance single min, skipping",
				"instance_id", inst.ID, "order", orderAmount, "min", cl.SingleMin)
			continue
		}
		if cl.SingleMax > 0 && orderAmount > cl.SingleMax {
			slog.Info("order above instance single max, skipping",
				"instance_id", inst.ID, "order", orderAmount, "max", cl.SingleMax)
			continue
		}

		// Check daily remaining capacity.
		if cl.DailyLimit > 0 {
			used, err := lb.GetInstanceDailyAmount(ctx, fmt.Sprintf("%d", inst.ID))
			if err != nil {
				slog.Warn("failed to query daily amount, keeping instance",
					"instance_id", inst.ID, "error", err)
				filtered = append(filtered, inst)
				continue
			}
			if used+orderAmount > cl.DailyLimit {
				slog.Info("instance daily remaining insufficient, skipping",
					"instance_id", inst.ID, "used", used,
					"order", orderAmount, "limit", cl.DailyLimit)
				continue
			}
		}

		filtered = append(filtered, inst)
	}
	return filtered
}

// getInstanceChannelLimits returns the channel limits for a specific payment type.
func getInstanceChannelLimits(inst *dbent.PaymentProviderInstance, paymentType PaymentType) ChannelLimits {
	if inst.Limits == "" {
		return ChannelLimits{}
	}
	var limits InstanceLimits
	if err := json.Unmarshal([]byte(inst.Limits), &limits); err != nil {
		return ChannelLimits{}
	}
	// For Stripe, limits are stored under the provider key "stripe".
	lookupKey := paymentType
	if inst.ProviderKey == "stripe" {
		lookupKey = "stripe"
	}
	if cl, ok := limits[lookupKey]; ok {
		return cl
	}
	return ChannelLimits{}
}

func (lb *DefaultLoadBalancer) pickByStrategy(ctx context.Context, candidates []*dbent.PaymentProviderInstance, strategy Strategy) (*dbent.PaymentProviderInstance, error) {
	if strategy == StrategyLeastAmount && len(candidates) > 1 {
		return lb.pickLeastAmount(ctx, candidates)
	}
	// Default: round-robin
	idx := lb.counter.Add(1) % uint64(len(candidates))
	return candidates[idx], nil
}

func (lb *DefaultLoadBalancer) pickLeastAmount(ctx context.Context, candidates []*dbent.PaymentProviderInstance) (*dbent.PaymentProviderInstance, error) {
	var selected *dbent.PaymentProviderInstance
	minAmount := -1.0
	for _, c := range candidates {
		amount, err := lb.GetInstanceDailyAmount(ctx, fmt.Sprintf("%d", c.ID))
		if err != nil {
			slog.Warn("failed to get instance daily amount, falling back", "instance_id", c.ID, "error", err)
			amount = 0
		}
		if minAmount < 0 || amount < minAmount {
			minAmount = amount
			selected = c
		}
	}
	return selected, nil
}

func (lb *DefaultLoadBalancer) buildSelection(selected *dbent.PaymentProviderInstance) (*InstanceSelection, error) {
	config, err := lb.decryptConfig(selected.Config)
	if err != nil {
		return nil, fmt.Errorf("decrypt instance %d config: %w", selected.ID, err)
	}

	// Inject payment_mode into config so providers can read it uniformly
	if selected.PaymentMode != "" {
		config["paymentMode"] = selected.PaymentMode
	}

	return &InstanceSelection{
		InstanceID:     fmt.Sprintf("%d", selected.ID),
		Config:         config,
		SupportedTypes: selected.SupportedTypes,
		PaymentMode:    selected.PaymentMode,
	}, nil
}

func (lb *DefaultLoadBalancer) decryptConfig(encrypted string) (map[string]string, error) {
	plaintext, err := Decrypt(encrypted, lb.encryptionKey)
	if err != nil {
		return nil, err
	}
	var config map[string]string
	if err := json.Unmarshal([]byte(plaintext), &config); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return config, nil
}

// GetInstanceDailyAmount returns the total completed order amount for an instance today.
func (lb *DefaultLoadBalancer) GetInstanceDailyAmount(ctx context.Context, instanceID string) (float64, error) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	var result []struct {
		Sum float64 `json:"sum"`
	}
	err := lb.db.PaymentOrder.Query().
		Where(
			paymentorder.ProviderInstanceID(instanceID),
			paymentorder.StatusIn(OrderStatusCompleted, OrderStatusPaid, OrderStatusRecharging),
			paymentorder.PaidAtGTE(todayStart),
		).
		Aggregate(dbent.Sum(paymentorder.FieldPayAmount)).
		Scan(ctx, &result)
	if err != nil {
		return 0, fmt.Errorf("query daily amount: %w", err)
	}
	if len(result) > 0 {
		return result[0].Sum, nil
	}
	return 0, nil
}

// InstanceSupportsType checks if the given supported types string includes the target type.
// An empty supportedTypes string means all types are supported.
func InstanceSupportsType(supportedTypes string, target PaymentType) bool {
	if supportedTypes == "" {
		return true
	}
	for _, t := range strings.Split(supportedTypes, ",") {
		if strings.TrimSpace(t) == target {
			return true
		}
	}
	return false
}

// GetInstanceConfig decrypts and returns the configuration for a provider instance by ID.
func (lb *DefaultLoadBalancer) GetInstanceConfig(ctx context.Context, instanceID int64) (map[string]string, error) {
	inst, err := lb.db.PaymentProviderInstance.Get(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("get instance %d: %w", instanceID, err)
	}
	return lb.decryptConfig(inst.Config)
}
