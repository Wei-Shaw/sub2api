package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	pluginent "github.com/Wei-Shaw/sub2api/plugins/payment/ent"
	paymentpkg "github.com/Wei-Shaw/sub2api/plugins/payment/internal/payment"
)

// paymentOrderProviderSnapshot captures the provider/instance details
// recorded on an order at creation time. The snapshot is the source of
// truth when verifying webhooks for an order whose provider has since
// been disabled or rotated, ensuring the in-flight payment still
// reconciles against the original merchant identity.
type paymentOrderProviderSnapshot struct {
	SchemaVersion      int
	ProviderInstanceID string
	ProviderKey        string
	PaymentMode        string
	MerchantAppID      string
	MerchantID         string
	Currency           string
}

// psOrderProviderSnapshot decodes the provider_snapshot JSON column on
// an order. Returns nil when the column is absent OR when every field
// in the snapshot is empty (treating an "empty object" as "no snapshot
// recorded").
func psOrderProviderSnapshot(order *pluginent.PaymentOrder) *paymentOrderProviderSnapshot {
	if order == nil || len(order.ProviderSnapshot) == 0 {
		return nil
	}
	snapshot := &paymentOrderProviderSnapshot{
		SchemaVersion:      psSnapshotIntValue(order.ProviderSnapshot["schema_version"]),
		ProviderInstanceID: psSnapshotStringValue(order.ProviderSnapshot["provider_instance_id"]),
		ProviderKey:        psSnapshotStringValue(order.ProviderSnapshot["provider_key"]),
		PaymentMode:        psSnapshotStringValue(order.ProviderSnapshot["payment_mode"]),
		MerchantAppID:      psSnapshotStringValue(order.ProviderSnapshot["merchant_app_id"]),
		MerchantID:         psSnapshotStringValue(order.ProviderSnapshot["merchant_id"]),
		Currency:           psSnapshotStringValue(order.ProviderSnapshot["currency"]),
	}
	if snapshot.SchemaVersion == 0 &&
		snapshot.ProviderInstanceID == "" &&
		snapshot.ProviderKey == "" &&
		snapshot.PaymentMode == "" &&
		snapshot.MerchantAppID == "" &&
		snapshot.MerchantID == "" &&
		snapshot.Currency == "" {
		return nil
	}
	return snapshot
}

func psSnapshotStringValue(value any) string {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func psSnapshotIntValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return n
		}
	}
	return 0
}

// resolveSnapshotOrderProviderInstance loads the *PaymentProviderInstance
// recorded in the order's snapshot. Returns an explicit error when the
// snapshot's instance id no longer matches a row in the database (the
// instance was deleted or the snapshot is corrupt).
func (s *PaymentService) resolveSnapshotOrderProviderInstance(ctx context.Context, order *pluginent.PaymentOrder, snapshot *paymentOrderProviderSnapshot) (*pluginent.PaymentProviderInstance, error) {
	if s == nil || s.entClient == nil || order == nil || snapshot == nil {
		return nil, nil
	}
	snapshotInstanceID := strings.TrimSpace(snapshot.ProviderInstanceID)
	columnInstanceID := strings.TrimSpace(psStringValue(order.ProviderInstanceID))
	if snapshotInstanceID == "" {
		snapshotInstanceID = columnInstanceID
	}
	if snapshotInstanceID == "" {
		return nil, fmt.Errorf("order %d provider snapshot is missing provider_instance_id", order.ID)
	}
	if columnInstanceID != "" && snapshot.ProviderInstanceID != "" &&
		!strings.EqualFold(columnInstanceID, snapshot.ProviderInstanceID) {
		return nil, fmt.Errorf("order %d provider snapshot instance mismatch: snapshot=%s order=%s",
			order.ID, snapshot.ProviderInstanceID, columnInstanceID)
	}
	instID, err := strconv.Atoi(snapshotInstanceID)
	if err != nil {
		return nil, fmt.Errorf("order %d provider snapshot instance id is invalid: %s",
			order.ID, snapshotInstanceID)
	}
	inst, err := s.entClient.PaymentProviderInstance.Get(ctx, instID)
	if err != nil {
		if pluginent.IsNotFound(err) {
			return nil, fmt.Errorf("order %d provider snapshot instance %s is missing", order.ID, snapshotInstanceID)
		}
		return nil, err
	}
	if snapshot.ProviderKey != "" && !strings.EqualFold(strings.TrimSpace(inst.ProviderKey), snapshot.ProviderKey) {
		return nil, fmt.Errorf("order %d provider snapshot key mismatch: snapshot=%s instance=%s",
			order.ID, snapshot.ProviderKey, inst.ProviderKey)
	}
	return inst, nil
}

// expectedNotificationProviderKeyForOrder picks the provider key the
// webhook handler should match against. Snapshot wins over the order's
// provider_key column so a provider rotation does not strand in-flight
// notifications.
func expectedNotificationProviderKeyForOrder(registry *paymentpkg.Registry, order *pluginent.PaymentOrder, instanceProviderKey string) string {
	if order == nil {
		return strings.TrimSpace(instanceProviderKey)
	}
	orderProviderKey := psStringValue(order.ProviderKey)
	if snapshot := psOrderProviderSnapshot(order); snapshot != nil && snapshot.ProviderKey != "" {
		orderProviderKey = snapshot.ProviderKey
	}
	return expectedNotificationProviderKey(registry, order.PaymentType, orderProviderKey, instanceProviderKey)
}

// expectedNotificationProviderKey returns the provider-key the webhook
// notification should be checked against. Order's recorded provider key
// (if any) wins; otherwise we fall back to the instance's current key.
// The registry parameter is reserved for future provider-key remapping
// (e.g. legacy → new) and is currently unused.
func expectedNotificationProviderKey(_ *paymentpkg.Registry, _ string, orderProviderKey, instanceProviderKey string) string {
	if k := strings.TrimSpace(orderProviderKey); k != "" {
		return k
	}
	return strings.TrimSpace(instanceProviderKey)
}

// validateProviderSnapshotMetadata ensures the merchant identity
// (appid / mchid / pid / currency / trade_state) reported in a webhook
// matches the snapshot recorded on the order. Returns a non-nil error
// when any required field disagrees.
func validateProviderSnapshotMetadata(order *pluginent.PaymentOrder, providerKey string, metadata map[string]string) error {
	if order == nil || len(metadata) == 0 {
		return nil
	}
	snapshot := psOrderProviderSnapshot(order)
	if snapshot == nil {
		return nil
	}
	switch strings.TrimSpace(providerKey) {
	case paymentpkg.TypeWxpay:
		return validateWxpaySnapshot(snapshot, metadata)
	case paymentpkg.TypeAlipay:
		return validateAlipaySnapshot(snapshot, metadata)
	case paymentpkg.TypeEasyPay:
		return validateEasyPaySnapshot(snapshot, metadata)
	}
	return nil
}

func validateWxpaySnapshot(snapshot *paymentOrderProviderSnapshot, metadata map[string]string) error {
	if expected := strings.TrimSpace(snapshot.MerchantAppID); expected != "" {
		actual := strings.TrimSpace(metadata["appid"])
		if actual == "" {
			return fmt.Errorf("wxpay notification missing appid")
		}
		if !strings.EqualFold(expected, actual) {
			return fmt.Errorf("wxpay appid mismatch: expected %s, got %s", expected, actual)
		}
	}
	if expected := strings.TrimSpace(snapshot.MerchantID); expected != "" {
		actual := strings.TrimSpace(metadata["mchid"])
		if actual == "" {
			return fmt.Errorf("wxpay notification missing mchid")
		}
		if !strings.EqualFold(expected, actual) {
			return fmt.Errorf("wxpay mchid mismatch: expected %s, got %s", expected, actual)
		}
	}
	if expected := strings.TrimSpace(snapshot.Currency); expected != "" {
		actual := strings.ToUpper(strings.TrimSpace(metadata["currency"]))
		if actual == "" {
			return fmt.Errorf("wxpay notification missing currency")
		}
		if !strings.EqualFold(expected, actual) {
			return fmt.Errorf("wxpay currency mismatch: expected %s, got %s", expected, actual)
		}
	}
	if actual := strings.TrimSpace(metadata["trade_state"]); actual != "" && !strings.EqualFold(actual, "SUCCESS") {
		return fmt.Errorf("wxpay trade_state mismatch: expected SUCCESS, got %s", actual)
	}
	return nil
}

func validateAlipaySnapshot(snapshot *paymentOrderProviderSnapshot, metadata map[string]string) error {
	if expected := strings.TrimSpace(snapshot.MerchantAppID); expected != "" {
		actual := strings.TrimSpace(metadata["app_id"])
		if actual == "" {
			return fmt.Errorf("alipay app_id missing")
		}
		if !strings.EqualFold(expected, actual) {
			return fmt.Errorf("alipay app_id mismatch: expected %s, got %s", expected, actual)
		}
	}
	return nil
}

func validateEasyPaySnapshot(snapshot *paymentOrderProviderSnapshot, metadata map[string]string) error {
	if expected := strings.TrimSpace(snapshot.MerchantID); expected != "" {
		actual := strings.TrimSpace(metadata["pid"])
		if actual == "" {
			return fmt.Errorf("easypay pid missing")
		}
		if !strings.EqualFold(expected, actual) {
			return fmt.Errorf("easypay pid mismatch: expected %s, got %s", expected, actual)
		}
	}
	return nil
}

// providerMerchantIdentityMetadata returns the static merchant
// identity fields the provider exposes, when the provider implements
// MerchantIdentityProvider. Used to populate provider_snapshot on
// order creation.
func providerMerchantIdentityMetadata(prov paymentpkg.Provider) map[string]string {
	if prov == nil {
		return nil
	}
	reporter, ok := prov.(paymentpkg.MerchantIdentityProvider)
	if !ok {
		return nil
	}
	return reporter.MerchantIdentityMetadata()
}
