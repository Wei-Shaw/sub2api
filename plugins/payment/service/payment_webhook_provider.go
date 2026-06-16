package service

import (
	"context"
	"fmt"
	"strings"

	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/plugins/payment/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
)

// GetWebhookProvider returns the provider that should verify a webhook.
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
func (s *PaymentService) GetWebhookProviders(ctx context.Context, providerKey, outTradeNo string) ([]payment.Provider, error) {
	if outTradeNo != "" {
		order, err := s.lookupOrderByTradeNo(ctx, outTradeNo)
		if err == nil && order != nil {
			return s.providersForOrder(ctx, order, providerKey)
		}
	}
	return s.providersForKeyOnly(ctx, providerKey)
}

func (s *PaymentService) lookupOrderByTradeNo(ctx context.Context, outTradeNo string) (*pluginent.PaymentOrder, error) {
	if s == nil || s.entClient == nil {
		return nil, nil
	}
	return s.entClient.PaymentOrder.Query().
		Where(paymentorder.OutTradeNo(outTradeNo)).Only(ctx)
}

func (s *PaymentService) providersForOrder(ctx context.Context, order *pluginent.PaymentOrder, providerKey string) ([]payment.Provider, error) {
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
	return s.providersForKeyOnly(ctx, providerKey)
}

func (s *PaymentService) providersForKeyOnly(ctx context.Context, providerKey string) ([]payment.Provider, error) {
	if strings.TrimSpace(providerKey) == payment.TypeWxpay {
		return s.getEnabledWebhookProvidersByKey(ctx, providerKey)
	}
	if !s.webhookRegistryFallbackAllowed(ctx, providerKey) {
		return nil, fmt.Errorf("webhook provider fallback is ambiguous for %s", providerKey)
	}
	s.EnsureProviders(ctx)
	if s.registry == nil {
		return nil, payment.ErrProviderNotFound
	}
	prov, err := s.registry.GetProviderByKey(providerKey)
	if err != nil {
		return nil, err
	}
	return []payment.Provider{prov}, nil
}

func (s *PaymentService) getPinnedOrderProvider(ctx context.Context, o *pluginent.PaymentOrder) (payment.Provider, error) {
	inst, err := s.getOrderProviderInstance(ctx, o)
	if err != nil {
		return nil, fmt.Errorf("load order provider instance: %w", err)
	}
	if inst == nil {
		return nil, fmt.Errorf("order %d provider instance is missing", o.ID)
	}
	return s.createProviderFromInstance(ctx, inst)
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
		).Count(ctx)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("payment webhook fallback instance count failed", "provider", providerKey, "error", err)
		}
		return false
	}
	return count <= 1
}

func psHasPinnedProviderInstance(order *pluginent.PaymentOrder) bool {
	if order == nil {
		return false
	}
	if psOrderProviderSnapshot(order) != nil {
		return true
	}
	return order.ProviderInstanceID != nil && strings.TrimSpace(*order.ProviderInstanceID) != ""
}

func (s *PaymentService) getEnabledWebhookProvidersByKey(ctx context.Context, providerKey string) ([]payment.Provider, error) {
	providerKey = strings.TrimSpace(providerKey)
	if s == nil || s.entClient == nil {
		return nil, payment.ErrProviderNotFound
	}
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(
			paymentproviderinstance.ProviderKeyEQ(providerKey),
			paymentproviderinstance.EnabledEQ(true),
		).
		Order(pluginent.Asc(paymentproviderinstance.FieldSortOrder)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query webhook provider instances: %w", err)
	}
	if len(instances) == 0 {
		return nil, payment.ErrProviderNotFound
	}
	providers := make([]payment.Provider, 0, len(instances))
	for _, inst := range instances {
		prov, provErr := s.createProviderFromInstance(ctx, inst)
		if provErr != nil {
			if s.logger != nil {
				s.logger.Warn("skip webhook provider instance",
					"provider", providerKey, "instanceID", inst.ID, "error", provErr)
			}
			continue
		}
		providers = append(providers, prov)
	}
	if len(providers) == 0 {
		return nil, payment.ErrProviderNotFound
	}
	return providers, nil
}
