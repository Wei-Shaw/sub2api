package payment

import (
	"context"
	"encoding/json"
	"fmt"
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
	StrategyRoundRobin  Strategy = "round_robin"
	StrategyLeastAmount Strategy = "least_amount"
)

// InstanceLimits holds per-channel limits for a provider instance (JSON).
type InstanceLimits map[string]struct {
	DailyLimit float64 `json:"dailyLimit,omitempty"`
	SingleMin  float64 `json:"singleMin,omitempty"`
	SingleMax  float64 `json:"singleMax,omitempty"`
}

// LoadBalancer selects a provider instance for a given payment type.
type LoadBalancer interface {
	GetInstanceConfig(ctx context.Context, instanceID int64) (map[string]string, error)
	SelectInstance(ctx context.Context, providerKey string, paymentType PaymentType) (*InstanceSelection, error)
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
func (lb *DefaultLoadBalancer) SelectInstance(ctx context.Context, providerKey string, paymentType PaymentType) (*InstanceSelection, error) {
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

	// Filter by supported types
	var candidates []*dbent.PaymentProviderInstance
	for _, inst := range instances {
		if inst.SupportedTypes == "" || containsType(inst.SupportedTypes, paymentType) {
			candidates = append(candidates, inst)
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no enabled instance for provider %s type %s", providerKey, paymentType)
	}

	// Round-robin selection (default strategy)
	idx := lb.counter.Add(1) % uint64(len(candidates))
	selected := candidates[idx]

	// Decrypt config
	config, err := lb.decryptConfig(selected.Config)
	if err != nil {
		return nil, fmt.Errorf("decrypt instance %d config: %w", selected.ID, err)
	}

	return &InstanceSelection{
		InstanceID: fmt.Sprintf("%d", selected.ID),
		Config:     config,
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
			paymentorder.StatusIn("COMPLETED", "PAID", "RECHARGING"),
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

// containsType checks if a comma-separated list contains the given type.
func containsType(supportedTypes string, target PaymentType) bool {
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
