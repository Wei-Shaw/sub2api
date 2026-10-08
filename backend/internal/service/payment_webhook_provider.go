package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// GetWebhookProvider returns the provider instance that should verify a webhook.
// It resolves the original provider instance from the order whenever possible and
// only falls back to a registry provider for legacy/single-instance scenarios.
func (s *PaymentService) GetWebhookProvider(ctx context.Context, providerKey, outTradeNo string) (payment.Provider, error) {
	providers, err := s.GetWebhookProviders(ctx, providerKey, outTradeNo)
	if err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		return nil, payment.ErrProviderNotFound
	}
	return providers[0], nil
}

// GetWebhookProviders returns provider candidates that can verify the webhook.
// Official WeChat Pay and configurable SePay content patterns may require
// multiple candidates because the callback body cannot always be bound to a
// merchant before provider-specific verification.
func (s *PaymentService) GetWebhookProviders(ctx context.Context, providerKey, outTradeNo string) ([]payment.Provider, error) {
	if outTradeNo != "" {
		order, err := s.entClient.PaymentOrder.Query().Where(paymentorder.OutTradeNo(outTradeNo)).Only(ctx)
		if err == nil {
			if psHasPinnedProviderInstance(order) {
				prov, err := s.getPinnedOrderProvider(ctx, order)
				if err != nil {
					return nil, err
				}
				return []payment.Provider{prov}, nil
			}
			inst, err := s.getOrderProviderInstance(ctx, order)
			if err != nil {
				return nil, fmt.Errorf("load order provider instance: %w", err)
			}
			if inst != nil {
				prov, err := s.createProviderFromInstance(ctx, inst)
				if err != nil {
					return nil, err
				}
				return []payment.Provider{prov}, nil
			}
			if strings.TrimSpace(providerKey) == payment.TypeSepay {
				prov, err := s.getUniqueEnabledWebhookProviderByKey(ctx, providerKey)
				if err != nil {
					return nil, err
				}
				return []payment.Provider{prov}, nil
			}
			if supportsMultiProviderWebhookLookup(providerKey) {
				return s.getEnabledWebhookProvidersByKey(ctx, providerKey)
			}
			if !s.webhookRegistryFallbackAllowed(ctx, providerKey) {
				return nil, fmt.Errorf("webhook provider fallback is ambiguous for %s", providerKey)
			}
			s.EnsureProviders(ctx)
			prov, err := s.registry.GetProviderByKey(providerKey)
			if err != nil {
				return nil, err
			}
			return []payment.Provider{prov}, nil
		}
	}

	if supportsMultiProviderWebhookLookup(providerKey) {
		return s.getEnabledWebhookProvidersByKey(ctx, providerKey)
	}

	if !s.webhookRegistryFallbackAllowed(ctx, providerKey) {
		return nil, fmt.Errorf("webhook provider fallback is ambiguous for %s", providerKey)
	}

	s.EnsureProviders(ctx)
	prov, err := s.registry.GetProviderByKey(providerKey)
	if err != nil {
		return nil, err
	}
	return []payment.Provider{prov}, nil
}

func (s *PaymentService) getPinnedOrderProvider(ctx context.Context, o *dbent.PaymentOrder) (payment.Provider, error) {
	inst, err := s.getOrderProviderInstance(ctx, o)
	if err != nil {
		return nil, fmt.Errorf("load order provider instance: %w", err)
	}
	if inst == nil {
		return nil, fmt.Errorf("order %d provider instance is missing", o.ID)
	}
	return s.createProviderFromInstance(ctx, inst)
}

func supportsMultiProviderWebhookLookup(providerKey string) bool {
	providerKey = strings.TrimSpace(providerKey)
	return providerKey == payment.TypeWxpay || providerKey == payment.TypeSepay
}

func (s *PaymentService) getUniqueEnabledWebhookProviderByKey(ctx context.Context, providerKey string) (payment.Provider, error) {
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(
			paymentproviderinstance.ProviderKeyEQ(strings.TrimSpace(providerKey)),
			paymentproviderinstance.EnabledEQ(true),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query webhook provider instances: %w", err)
	}
	if len(instances) != 1 {
		return nil, fmt.Errorf("webhook provider instance is ambiguous for order without a pinned provider instance: %s", providerKey)
	}
	return s.createProviderFromInstance(ctx, instances[0])
}

func (s *PaymentService) webhookRegistryFallbackAllowed(ctx context.Context, providerKey string) bool {
	providerKey = strings.TrimSpace(providerKey)
	if providerKey == "" || s == nil || s.entClient == nil {
		return false
	}

	count, err := s.entClient.PaymentProviderInstance.Query().
		Where(
			paymentproviderinstance.ProviderKeyEQ(providerKey),
			paymentproviderinstance.EnabledEQ(true),
		).
		Count(ctx)
	if err != nil {
		slog.Warn("payment webhook fallback instance count failed", "provider", providerKey, "error", err)
		return false
	}
	return count <= 1
}

func psHasPinnedProviderInstance(order *dbent.PaymentOrder) bool {
	return order != nil && (psOrderProviderSnapshot(order) != nil || (order.ProviderInstanceID != nil && strings.TrimSpace(*order.ProviderInstanceID) != ""))
}

func (s *PaymentService) getEnabledWebhookProvidersByKey(ctx context.Context, providerKey string) ([]payment.Provider, error) {
	providerKey = strings.TrimSpace(providerKey)
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(
			paymentproviderinstance.ProviderKeyEQ(providerKey),
			paymentproviderinstance.EnabledEQ(true),
		).
		Order(dbent.Asc(paymentproviderinstance.FieldSortOrder)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query webhook provider instances: %w", err)
	}
	if len(instances) == 0 {
		return nil, payment.ErrProviderNotFound
	}

	providers := make([]payment.Provider, 0, len(instances))
	seenSepayWebhookKeys := make(map[[sha256.Size]byte]struct{})
	for _, inst := range instances {
		if providerKey == payment.TypeSepay {
			cfg, cfgErr := s.loadBalancer.GetInstanceConfig(ctx, int64(inst.ID))
			if cfgErr != nil {
				slog.Warn("skip SePay webhook provider instance with unreadable config", "instanceID", inst.ID, "error", cfgErr)
				continue
			}
			webhookKey := strings.TrimSpace(providerConfigFieldValue(cfg, "webhookApiKey"))
			if webhookKey == "" {
				slog.Warn("skip SePay webhook provider instance without webhook authentication", "instanceID", inst.ID)
				continue
			}
			fingerprint := sha256.Sum256([]byte(webhookKey))
			if _, duplicate := seenSepayWebhookKeys[fingerprint]; duplicate {
				return nil, fmt.Errorf("ambiguous SePay webhook credentials across enabled provider instances")
			}
			seenSepayWebhookKeys[fingerprint] = struct{}{}
		}
		prov, provErr := s.createProviderFromInstance(ctx, inst)
		if provErr != nil {
			slog.Warn("skip webhook provider instance", "provider", providerKey, "instanceID", inst.ID, "error", provErr)
			continue
		}
		providers = append(providers, prov)
	}
	if len(providers) == 0 {
		return nil, payment.ErrProviderNotFound
	}
	return providers, nil
}
