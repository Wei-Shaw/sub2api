package payment

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// mockProvider implements the Provider interface for testing.
type mockProvider struct {
	name           string
	key            string
	supportedTypes []PaymentType
}

func (m *mockProvider) Name() string                  { return m.name }
func (m *mockProvider) ProviderKey() string           { return m.key }
func (m *mockProvider) SupportedTypes() []PaymentType { return m.supportedTypes }
func (m *mockProvider) CreatePayment(_ context.Context, _ CreatePaymentRequest) (*CreatePaymentResponse, error) {
	return nil, nil
}
func (m *mockProvider) QueryOrder(_ context.Context, _ string) (*QueryOrderResponse, error) {
	return nil, nil
}
func (m *mockProvider) VerifyNotification(_ context.Context, _ string, _ map[string]string) (*PaymentNotification, error) {
	return nil, nil
}
func TestRegistryRegisterAndGetProvider(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	p := &mockProvider{
		name:           "TestPay",
		key:            "testpay",
		supportedTypes: []PaymentType{TypeSePay, TypeNowPayments},
	}
	r.Register(p)

	got, err := r.GetProvider(TypeSePay)
	if err != nil {
		t.Fatalf("GetProvider(sepay) error: %v", err)
	}
	if got.ProviderKey() != "testpay" {
		t.Fatalf("GetProvider(sepay) key = %q, want %q", got.ProviderKey(), "testpay")
	}

	got2, err := r.GetProvider(TypeNowPayments)
	if err != nil {
		t.Fatalf("GetProvider(nowpayments) error: %v", err)
	}
	if got2.ProviderKey() != "testpay" {
		t.Fatalf("GetProvider(nowpayments) key = %q, want %q", got2.ProviderKey(), "testpay")
	}
}

func TestRegistryGetProviderNotFound(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	_, err := r.GetProvider("nonexistent")
	if err == nil {
		t.Fatal("GetProvider for unregistered type should return error")
	}
}

func TestRegistryGetProviderByKey(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	p := &mockProvider{
		name:           "SePay",
		key:            "sepay",
		supportedTypes: []PaymentType{TypeSePay},
	}
	r.Register(p)

	got, err := r.GetProviderByKey("sepay")
	if err != nil {
		t.Fatalf("GetProviderByKey error: %v", err)
	}
	if got.Name() != "SePay" {
		t.Fatalf("GetProviderByKey name = %q, want %q", got.Name(), "SePay")
	}
}

func TestRegistryGetProviderByKeyNotFound(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	_, err := r.GetProviderByKey("nonexistent")
	if err == nil {
		t.Fatal("GetProviderByKey for unknown key should return error")
	}
}

func TestRegistryGetProviderKeyUnknownType(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	key := r.GetProviderKey("unknown_type")
	if key != "" {
		t.Fatalf("GetProviderKey for unknown type should return empty, got %q", key)
	}
}

func TestRegistryGetProviderKeyKnownType(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	p := &mockProvider{
		name:           "NOWPayments",
		key:            "nowpayments",
		supportedTypes: []PaymentType{TypeNowPayments},
	}
	r.Register(p)

	key := r.GetProviderKey(TypeNowPayments)
	if key != "nowpayments" {
		t.Fatalf("GetProviderKey(nowpayments) = %q, want %q", key, "nowpayments")
	}
}

func TestRegistrySupportedTypes(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	p1 := &mockProvider{
		name:           "SePay",
		key:            "sepay",
		supportedTypes: []PaymentType{TypeSePay},
	}
	p2 := &mockProvider{
		name:           "NOWPayments",
		key:            "nowpayments",
		supportedTypes: []PaymentType{TypeNowPayments},
	}
	r.Register(p1)
	r.Register(p2)

	types := r.SupportedTypes()
	if len(types) != 2 {
		t.Fatalf("SupportedTypes() len = %d, want 2", len(types))
	}

	typeSet := make(map[PaymentType]bool)
	for _, tp := range types {
		typeSet[tp] = true
	}
	for _, expected := range []PaymentType{TypeSePay, TypeNowPayments} {
		if !typeSet[expected] {
			t.Fatalf("SupportedTypes() missing %q", expected)
		}
	}
}

func TestRegistrySupportedTypesEmpty(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	types := r.SupportedTypes()
	if len(types) != 0 {
		t.Fatalf("SupportedTypes() on empty registry should be empty, got %d", len(types))
	}
}

func TestRegistryOverwriteExisting(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	p1 := &mockProvider{
		name:           "OldPay",
		key:            "old",
		supportedTypes: []PaymentType{TypeSePay},
	}
	p2 := &mockProvider{
		name:           "NewPay",
		key:            "new",
		supportedTypes: []PaymentType{TypeSePay},
	}
	r.Register(p1)
	r.Register(p2)

	got, err := r.GetProvider(TypeSePay)
	if err != nil {
		t.Fatalf("GetProvider error: %v", err)
	}
	if got.Name() != "NewPay" {
		t.Fatalf("expected overwritten provider, got %q", got.Name())
	}
}

func TestRegistryConcurrentAccess(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Concurrent writers
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			p := &mockProvider{
				name:           fmt.Sprintf("Provider-%d", idx),
				key:            fmt.Sprintf("key-%d", idx),
				supportedTypes: []PaymentType{PaymentType(fmt.Sprintf("type-%d", idx))},
			}
			r.Register(p)
		}(i)
	}

	// Concurrent readers
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_ = r.SupportedTypes()
			_, _ = r.GetProvider("some-type")
			_ = r.GetProviderKey("some-type")
		}()
	}

	wg.Wait()

	types := r.SupportedTypes()
	if len(types) != goroutines {
		t.Fatalf("after concurrent registration, expected %d types, got %d", goroutines, len(types))
	}
}
