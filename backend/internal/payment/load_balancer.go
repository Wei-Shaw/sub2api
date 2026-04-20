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
REDACTED

// InstanceLimits holds per-channel limits for a provider instance (JSON).
type InstanceLimits map[string]ChannelLimits

// LoadBalancer selects a provider instance for a given payment type.
type LoadBalancer interface {
	GetInstanceConfig(ctx context.Context, instanceID int64) (map[string]string, error)
	SelectInstance(ctx context.Context, providerKey string, paymentType PaymentType, strategy Strategy, orderAmount float64) (*InstanceSelection, error)
REDACTED

// DefaultLoadBalancer implements LoadBalancer using database queries.
type DefaultLoadBalancer struct {
	db            *dbent.Client
	encryptionKey []byte
	counter       atomic.Uint64
REDACTED

type contextKey string

const wxpayJSAPIAppIDContextKey contextKey = "payment.wxpay.jsapi_app_id"

// NewDefaultLoadBalancer creates a new load balancer.
func NewDefaultLoadBalancer(db *dbent.Client, encryptionKey []byte) *DefaultLoadBalancer {
	return &DefaultLoadBalancer{db: db, encryptionKey: encryptionKeyREDACTED
REDACTED

func WithWxpayJSAPIAppID(ctx context.Context, appID string) context.Context {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return ctx
REDACTED
	return context.WithValue(ctx, wxpayJSAPIAppIDContextKey, appID)
REDACTED

func wxpayJSAPIAppIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
REDACTED
	appID, _ := ctx.Value(wxpayJSAPIAppIDContextKey).(string)
	return strings.TrimSpace(appID)
REDACTED

// instanceCandidate pairs an instance with its pre-fetched daily usage.
type instanceCandidate struct {
	inst      *dbent.PaymentProviderInstance
	dailyUsed float64 // includes PENDING orders
REDACTED

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
REDACTED

	// Step 2: batch-fetch daily usage for all candidates.
	candidates := lb.attachDailyUsage(ctx, instances)

	// Step 3: filter by limits.
	available := filterByLimits(candidates, paymentType, orderAmount)
	if len(available) == 0 {
		slog.Warn("all instances exceeded limits, using full candidate list",
			"provider", providerKey, "payment_type", paymentType,
			"order_amount", orderAmount, "count", len(candidates))
		available = candidates
REDACTED

	// Step 4: pick by strategy.
	selected := lb.pickByStrategy(available, strategy)
	return lb.buildSelection(selected.inst)
REDACTED

// queryEnabledInstances returns enabled instances that support paymentType.
// When providerKey is non-empty, only instances with that provider key are considered.
// When providerKey is empty, instances across all providers are considered,
// enabling cross-provider load balancing (e.g. EasyPay + Alipay direct for "alipay").
func (lb *DefaultLoadBalancer) queryEnabledInstances(
	ctx context.Context,
	providerKey string,
	paymentType PaymentType,
) ([]*dbent.PaymentProviderInstance, error) {
	query := lb.db.PaymentProviderInstance.Query().
		Where(paymentproviderinstance.Enabled(true))
	if providerKey != "" {
		query = query.Where(paymentproviderinstance.ProviderKey(providerKey))
REDACTED
	instances, err := query.
		Order(dbent.Asc(paymentproviderinstance.FieldSortOrder)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query provider instances: %w", err)
REDACTED

	var matched []*dbent.PaymentProviderInstance
	expectedWxpayJSAPIAppID := wxpayJSAPIAppIDFromContext(ctx)
	for _, inst := range instances {
		// Stripe: match by provider_key because supported_types lists sub-types (card,link,alipay,wxpay),
		// not "stripe" itself. The checkout page aggregates all sub-types under "stripe".
		if paymentType == TypeStripe {
			if inst.ProviderKey == TypeStripe {
				matched = append(matched, inst)
		REDACTED
	REDACTED else if InstanceSupportsType(inst.SupportedTypes, paymentType) {
			if expectedWxpayJSAPIAppID != "" && normalizeVisibleMethodSupportType(paymentType) == TypeWxpay && inst.ProviderKey == TypeWxpay {
				config, cfgErr := lb.decryptConfig(inst.Config)
				if cfgErr != nil {
					slog.Warn("skip wxpay instance with unreadable config during jsapi filtering", "instance_id", inst.ID, "error", cfgErr)
					continue
			REDACTED
				if resolveWxpayJSAPIAppID(config) != expectedWxpayJSAPIAppID {
					continue
			REDACTED
		REDACTED
			matched = append(matched, inst)
	REDACTED
REDACTED
	if len(matched) == 0 {
		return nil, fmt.Errorf("no enabled instance for payment type %s", paymentType)
REDACTED
	return matched, nil
REDACTED

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
REDACTED

	// Batch query: sum pay_amount grouped by provider_instance_id.
	type row struct {
		InstanceID string  `json:"provider_instance_id"`
		Sum        float64 `json:"sum"`
REDACTED
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
REDACTED

	usageMap := make(map[string]float64, len(rows))
	for _, r := range rows {
		usageMap[r.InstanceID] = r.Sum
REDACTED

	candidates := make([]instanceCandidate, len(instances))
	for i, inst := range instances {
		candidates[i] = instanceCandidate{
			inst:      inst,
			dailyUsed: usageMap[fmt.Sprintf("%d", inst.ID)],
	REDACTED
REDACTED
	return candidates
REDACTED

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
	REDACTED
		if cl.SingleMax > 0 && orderAmount > cl.SingleMax {
			slog.Info("order above instance single max, skipping",
				"instance_id", c.inst.ID, "order", orderAmount, "max", cl.SingleMax)
			continue
	REDACTED
		if cl.DailyLimit > 0 && c.dailyUsed+orderAmount > cl.DailyLimit {
			slog.Info("instance daily remaining insufficient, skipping",
				"instance_id", c.inst.ID, "used", c.dailyUsed,
				"order", orderAmount, "limit", cl.DailyLimit)
			continue
	REDACTED

		result = append(result, c)
REDACTED
	return result
REDACTED

// getInstanceChannelLimits returns the channel limits for a specific payment type.
func getInstanceChannelLimits(inst *dbent.PaymentProviderInstance, paymentType PaymentType) ChannelLimits {
	if inst.Limits == "" {
		return ChannelLimits{REDACTED
REDACTED
	var limits InstanceLimits
	if err := json.Unmarshal([]byte(inst.Limits), &limits); err != nil {
		return ChannelLimits{REDACTED
REDACTED
	// For Stripe, limits are stored under the provider key "stripe".
	lookupKey := paymentType
	if inst.ProviderKey == "stripe" {
		lookupKey = "stripe"
REDACTED
	if cl, ok := limits[lookupKey]; ok {
		return cl
REDACTED
	if aliasKey := legacyVisibleMethodAlias(lookupKey); aliasKey != "" {
		if cl, ok := limits[aliasKey]; ok {
			return cl
	REDACTED
REDACTED
	return ChannelLimits{REDACTED
REDACTED

// pickByStrategy selects one instance from the available candidates.
func (lb *DefaultLoadBalancer) pickByStrategy(candidates []instanceCandidate, strategy Strategy) instanceCandidate {
	if strategy == StrategyLeastAmount && len(candidates) > 1 {
		return pickLeastAmount(candidates)
REDACTED
	// Default: round-robin.
	idx := lb.counter.Add(1) % uint64(len(candidates))
	return candidates[idx]
REDACTED

// pickLeastAmount selects the instance with the lowest daily usage.
// No extra DB queries — usage was pre-fetched in attachDailyUsage.
func pickLeastAmount(candidates []instanceCandidate) instanceCandidate {
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.dailyUsed < best.dailyUsed {
			best = c
	REDACTED
REDACTED
	return best
REDACTED

func (lb *DefaultLoadBalancer) buildSelection(selected *dbent.PaymentProviderInstance) (*InstanceSelection, error) {
	config, err := lb.decryptConfig(selected.Config)
	if err != nil {
		return nil, fmt.Errorf("decrypt instance %d config: %w", selected.ID, err)
REDACTED

	if selected.PaymentMode != "" {
		config["paymentMode"] = selected.PaymentMode
REDACTED

	return &InstanceSelection{
		InstanceID:     fmt.Sprintf("%d", selected.ID),
		ProviderKey:    selected.ProviderKey,
		Config:         config,
		SupportedTypes: selected.SupportedTypes,
		PaymentMode:    selected.PaymentMode,
REDACTED, nil
REDACTED

func (lb *DefaultLoadBalancer) decryptConfig(encrypted string) (map[string]string, error) {
	plaintext, err := Decrypt(encrypted, lb.encryptionKey)
	if err != nil {
		return nil, err
REDACTED
	var config map[string]string
	if err := json.Unmarshal([]byte(plaintext), &config); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
REDACTED
	return config, nil
REDACTED

// GetInstanceDailyAmount returns the total completed order amount for an instance today.
func (lb *DefaultLoadBalancer) GetInstanceDailyAmount(ctx context.Context, instanceID string) (float64, error) {
	todayStart := startOfDay(time.Now())

	var result []struct {
		Sum float64 `json:"sum"`
REDACTED
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
REDACTED
	if len(result) > 0 {
		return result[0].Sum, nil
REDACTED
	return 0, nil
REDACTED

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
REDACTED

// InstanceSupportsType checks if the given supported types string includes the target type.
// An empty supportedTypes string means all types are supported.
func InstanceSupportsType(supportedTypes string, target PaymentType) bool {
	if supportedTypes == "" {
		return true
REDACTED
	normalizedTarget := normalizeVisibleMethodSupportType(target)
	for _, t := range strings.Split(supportedTypes, ",") {
		supported := strings.TrimSpace(t)
		if supported == target || normalizeVisibleMethodSupportType(supported) == normalizedTarget {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func normalizeVisibleMethodSupportType(paymentType PaymentType) PaymentType {
	switch strings.TrimSpace(paymentType) {
	case TypeAlipay, TypeAlipayDirect:
		return TypeAlipay
	case TypeWxpay, TypeWxpayDirect:
		return TypeWxpay
	default:
		return strings.TrimSpace(paymentType)
REDACTED
REDACTED

func legacyVisibleMethodAlias(paymentType PaymentType) PaymentType {
	switch normalizeVisibleMethodSupportType(paymentType) {
	case TypeAlipay:
		return TypeAlipayDirect
	case TypeWxpay:
		return TypeWxpayDirect
	default:
		return ""
REDACTED
REDACTED

func resolveWxpayJSAPIAppID(config map[string]string) string {
	if appID := strings.TrimSpace(config["mpAppId"]); appID != "" {
		return appID
REDACTED
	return strings.TrimSpace(config["appId"])
REDACTED

// GetInstanceConfig decrypts and returns the configuration for a provider instance by ID.
func (lb *DefaultLoadBalancer) GetInstanceConfig(ctx context.Context, instanceID int64) (map[string]string, error) {
	inst, err := lb.db.PaymentProviderInstance.Get(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("get instance %d: %w", instanceID, err)
REDACTED
	return lb.decryptConfig(inst.Config)
REDACTED
