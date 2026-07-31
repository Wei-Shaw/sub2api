package provider

import (
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// CreateProvider creates a Provider from a provider key, instance ID and decrypted config.
func CreateProvider(providerKey string, instanceID string, config map[string]string) (payment.Provider, error) {
	switch providerKey {
	case payment.TypeEasyPay:
		easyPay, err := NewEasyPay(instanceID, config)
		if err != nil {
			return nil, err
		}
		switch EasyPayCompatibilityMode(config) {
		case EasyPayCompatibilityStandard:
			return easyPay, nil
		case EasyPayCompatibilityA5:
			return NewA5EasyPay(easyPay), nil
		default:
			return nil, fmt.Errorf("invalid easypay compatibilityMode: %s", strings.TrimSpace(config[EasyPayCompatibilityModeKey]))
		}
	case payment.TypeAlipay:
		return NewAlipay(instanceID, config)
	case payment.TypeWxpay:
		return NewWxpay(instanceID, config)
	case payment.TypeStripe:
		return NewStripe(instanceID, config)
	case payment.TypeAirwallex:
		return NewAirwallex(instanceID, config)
	default:
		return nil, fmt.Errorf("unknown provider key: %s", providerKey)
	}
}
