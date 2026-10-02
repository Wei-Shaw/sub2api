package provider

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// CreateProvider creates a Provider from a provider key, instance ID and decrypted config.
func CreateProvider(providerKey string, instanceID string, config map[string]string) (payment.Provider, error) {
	switch providerKey {
	case payment.TypeNowPayments:
		return NewNowPayments(instanceID, config)
	case payment.TypeGPMPay:
		return NewGPMPay(instanceID, config)
	default:
		return nil, fmt.Errorf("unknown provider key: %s", providerKey)
	}
}
