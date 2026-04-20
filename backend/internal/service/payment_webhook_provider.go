package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
)

// GetWebhookProvider returns the provider instance that should verify a webhook.
// It resolves the original provider instance from the order whenever possible and
// only falls back to a registry provider for legacy/single-instance scenarios.
func (s *PaymentService) GetWebhookProvider(ctx context.Context, providerKey, outTradeNo string) (payment.Provider, error) {
	if outTradeNo != "" {
		order, err := s.entClient.PaymentOrder.Query().Where(paymentorder.OutTradeNo(outTradeNo)).Only(ctx)
		if err == nil {
			if psHasPinnedProviderInstance(order) {
				return s.getPinnedOrderProvider(ctx, order)
		REDACTED
			if !s.webhookRegistryFallbackAllowed(ctx, providerKey) {
				return nil, fmt.Errorf("webhook provider fallback is ambiguous for %s", providerKey)
		REDACTED
			s.EnsureProviders(ctx)
			return s.registry.GetProviderByKey(providerKey)
	REDACTED
REDACTED

	if !s.webhookRegistryFallbackAllowed(ctx, providerKey) {
		return nil, fmt.Errorf("webhook provider fallback is ambiguous for %s", providerKey)
REDACTED

	s.EnsureProviders(ctx)
	return s.registry.GetProviderByKey(providerKey)
REDACTED

func (s *PaymentService) getPinnedOrderProvider(ctx context.Context, o *dbent.PaymentOrder) (payment.Provider, error) {
	inst, err := s.getOrderProviderInstance(ctx, o)
	if err != nil {
		return nil, fmt.Errorf("load order provider instance: %w", err)
REDACTED
	if inst == nil {
		return nil, fmt.Errorf("order %d provider instance is missing", o.ID)
REDACTED

	instID := strconv.FormatInt(int64(inst.ID), 10)
	cfg, err := s.loadBalancer.GetInstanceConfig(ctx, int64(inst.ID))
	if err != nil {
		return nil, fmt.Errorf("load provider instance config: %w", err)
REDACTED

	prov, err := provider.CreateProvider(inst.ProviderKey, instID, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pinned provider: %w", err)
REDACTED
	return prov, nil
REDACTED

func (s *PaymentService) webhookRegistryFallbackAllowed(ctx context.Context, providerKey string) bool {
	providerKey = strings.TrimSpace(providerKey)
	if providerKey == "" || s == nil || s.entClient == nil {
		return false
REDACTED

	count, err := s.entClient.PaymentProviderInstance.Query().
		Where(
			paymentproviderinstance.ProviderKeyEQ(providerKey),
			paymentproviderinstance.EnabledEQ(true),
		).
		Count(ctx)
	if err != nil {
		slog.Warn("payment webhook fallback instance count failed", "provider", providerKey, "error", err)
		return false
REDACTED
	return count <= 1
REDACTED

func psHasPinnedProviderInstance(order *dbent.PaymentOrder) bool {
	return order != nil && order.ProviderInstanceID != nil && strings.TrimSpace(*order.ProviderInstanceID) != ""
REDACTED
