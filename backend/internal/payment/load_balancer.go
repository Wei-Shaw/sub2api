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

// instanceCandidate pairs an instance with its pre-fetched daily usage.
type instanceCandidate struct {
	inst      *dbent.PaymentProviderInstance
	dailyUsed float64 // includes PENDING orders
}

// SelectInstance picks an enabled instance for the given provider key and payment type.
//
// Flow:
//  1. Query all enabled instances for providerKey, filter by supported paymentType
//  2. Batch-query daily usage (PENDING + PAID + COMPLETED + RECHARGING) for all candidates
//  3. Filter out instances where: single-min/max violated OR daily remaining < orderAmount
//  4. Pick from survivors using the configured strategy (round-robin / least-amount)
//  5. If all filtered out, fall back to full list (let the provider itself reject)
func (lb *DefaultLoadBalancer) SelectInstance(
	ctx context.Context,
	providerKey string,
	paymentType PaymentType,
	strategy Strategy,
	orderAmount float64,
) (*InstanceSelection, error) {
	// Step 1: query enabled instances matching payment type.
	instances, err := lb.queryEnabledInstances(ctx, providerKey, paymentType)
	if err != nil {
		return nil, err
	}

	// Step 2: batch-fetch daily usage for all candidates.
	candidates := lb.attachDailyUsage(ctx, instances)

	// Step 3: filter by limits.
	available := filterByLimits(candidates, paymentType, orderAmount)
	if len(available) == 0 {
		slog.Warn("all instances exceeded limits, using full candidate list",
			"provider", providerKey, "payment_type", paymentType,
			"order_amount", orderAmount, "count", len(candidates))
		available = candidates
	}

	// Step 4: pick by strategy.
	selected := lb.pickByStrategy(available, strategy)
	return lb.buildSelection(selected.inst)
}

// queryEnabledInstances returns enabled instances for providerKey that support paymentType.
func (lb *DefaultLoadBalancer) queryEnabledInstances(
	ctx context.Context,
	providerKey string,
	paymentType PaymentType,
) ([]*dbent.PaymentProviderInstance, error) {
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

	var matched []*dbent.PaymentProviderInstance
	for _, inst := range instances {
		if paymentType == providerKey || InstanceSupportsType(inst.SupportedTypes, paymentType) {
			matched = append(matched, inst)
		}
	}
	if len(matched) == 0 {
		return nil, fmt.Errorf("no enabled instance for provider %s type %s", providerKey, paymentType)
	}
	return matched, nil
}

// attachDailyUsage queries daily usage for each instance in a single pass.
// Usage includes PENDING orders to avoid over-committing capacity.
func (lb *DefaultLoadBalancer) attachDailyUsage(
	ctx context.Context,
	instances []*dbent.PaymentProviderInstance,
) []instanceCandidate {
	todayStart := startOfDay(time.Now())

	// Collect instance IDs.
	ids := make([]string, len(instances))
	for i, inst := range instances {
		ids[i] = fmt.Sprintf("%d", inst.ID)
	}

	// Batch query: sum pay_amount grouped by provider_instance_id.
	type row struct {
		InstanceID string  `json:"provider_instance_id"`
		Sum        float64 `json:"sum"`
	}
	var rows []row
	err := lb.db.PaymentOrder.Query().
		Where(
			paymentorder.ProviderInstanceIDIn(ids...),
			paymentorder.StatusIn(
				OrderStatusPending, OrderStatusPaid,
				OrderStatusCompleted, OrderStatusRecharging,
			),
			paymentorder.CreatedAtGTE(todayStart),
		).
		GroupBy(paymentorder.FieldProviderInstanceID).
		Aggregate(dbent.Sum(paymentorder.FieldPayAmount)).
		Scan(ctx, &rows)
	if err != nil {
		slog.Warn("batch daily usage query failed, treating all as zero", "error", err)
	}

	usageMap := make(map[string]float64, len(rows))
	for _, r := range rows {
		usageMap[r.InstanceID] = r.Sum
	}

	candidates := make([]instanceCandidate, len(instances))
	for i, inst := range instances {
		candidates[i] = instanceCandidate{
			inst:      inst,
			dailyUsed: usageMap[fmt.Sprintf("%d", inst.ID)],
		}
	}
	return candidates
}

// filterByLimits removes instances that cannot accommodate the order:
//   - orderAmount outside single-transaction [min, max]
//   - daily remaining capacity (limit - used) < orderAmount
func filterByLimits(candidates []instanceCandidate, paymentType PaymentType, orderAmount float64) []instanceCandidate {
	var result []instanceCandidate
	for _, c := range candidates {
		cl := getInstanceChannelLimits(c.inst, paymentType)

		if cl.SingleMin > 0 && orderAmount < cl.SingleMin {
			slog.Info("order below instance single min, skipping",
				"instance_id", c.inst.ID, "order", orderAmount, "min", cl.SingleMin)
			continue
		}
		if cl.SingleMax > 0 && orderAmount > cl.SingleMax {
			slog.Info("order above instance single max, skipping",
				"instance_id", c.inst.ID, "order", orderAmount, "max", cl.SingleMax)
			continue
		}
		if cl.DailyLimit > 0 && c.dailyUsed+orderAmount > cl.DailyLimit {
			slog.Info("instance daily remaining insufficient, skipping",
				"instance_id", c.inst.ID, "used", c.dailyUsed,
				"order", orderAmount, "limit", cl.DailyLimit)
			continue
		}

		result = append(result, c)
	}
	return result
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

// pickByStrategy selects one instance from the available candidates.
func (lb *DefaultLoadBalancer) pickByStrategy(candidates []instanceCandidate, strategy Strategy) instanceCandidate {
	if strategy == StrategyLeastAmount && len(candidates) > 1 {
		return pickLeastAmount(candidates)
	}
	// Default: round-robin.
	idx := lb.counter.Add(1) % uint64(len(candidates))
	return candidates[idx]
}

// pickLeastAmount selects the instance with the lowest daily usage.
// No extra DB queries — usage was pre-fetched in attachDailyUsage.
func pickLeastAmount(candidates []instanceCandidate) instanceCandidate {
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.dailyUsed < best.dailyUsed {
			best = c
		}
	}
	return best
}

func (lb *DefaultLoadBalancer) buildSelection(selected *dbent.PaymentProviderInstance) (*InstanceSelection, error) {
	config, err := lb.decryptConfig(selected.Config)
	if err != nil {
		return nil, fmt.Errorf("decrypt instance %d config: %w", selected.ID, err)
	}

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
	todayStart := startOfDay(time.Now())

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

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
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
