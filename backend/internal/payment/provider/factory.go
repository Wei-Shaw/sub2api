package provider

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// CreateProvider creates a Provider from a provider key, instance ID and decrypted config.
func CreateProvider(providerKey string, instanceID string, config map[string]string) (payment.Provider, error) {
	switch providerKey {
	case "easypay":
		return NewEasyPay(instanceID, config)
	case "alipay":
		return NewAlipay(instanceID, config)
	case "wxpay":
		return NewWxpay(instanceID, config)
	case "stripe":
		return NewStripe(instanceID, config)
	default:
		return nil, fmt.Errorf("unknown provider key: %s", providerKey)
	}
}
