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
REDACTED

func (m *mockProvider) Name() string                  { return m.name REDACTED
func (m *mockProvider) ProviderKey() string           { return m.key REDACTED
func (m *mockProvider) SupportedTypes() []PaymentType { return m.supportedTypes REDACTED
func (m *mockProvider) CreatePayment(_ context.Context, _ CreatePaymentRequest) (*CreatePaymentResponse, error) {
	return nil, nil
REDACTED
func (m *mockProvider) QueryOrder(_ context.Context, _ string) (*QueryOrderResponse, error) {
	return nil, nil
REDACTED
func (m *mockProvider) VerifyNotification(_ context.Context, _ string, _ map[string]string) (*PaymentNotification, error) {
	return nil, nil
REDACTED
func (m *mockProvider) Refund(_ context.Context, _ RefundRequest) (*RefundResponse, error) {
	return nil, nil
REDACTED

func TestRegistryRegisterAndGetProvider(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	p := &mockProvider{
		name:           "TestPay",
		key:            "testpay",
		supportedTypes: []PaymentType{TypeAlipay, TypeWxpayREDACTED,
REDACTED
	r.Register(p)

	got, err := r.GetProvider(TypeAlipay)
	if err != nil {
		t.Fatalf("GetProvider(alipay) error: %v", err)
REDACTED
	if got.ProviderKey() != "testpay" {
		t.Fatalf("GetProvider(alipay) key = %q, want %q", got.ProviderKey(), "testpay")
REDACTED

	got2, err := r.GetProvider(TypeWxpay)
	if err != nil {
		t.Fatalf("GetProvider(wxpay) error: %v", err)
REDACTED
	if got2.ProviderKey() != "testpay" {
		t.Fatalf("GetProvider(wxpay) key = %q, want %q", got2.ProviderKey(), "testpay")
REDACTED
REDACTED

func TestRegistryGetProviderNotFound(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	_, err := r.GetProvider("nonexistent")
	if err == nil {
		t.Fatal("GetProvider for unregistered type should return error")
REDACTED
REDACTED

func TestRegistryGetProviderByKey(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	p := &mockProvider{
		name:           "EasyPay",
		key:            "easypay",
		supportedTypes: []PaymentType{TypeAlipayREDACTED,
REDACTED
	r.Register(p)

	got, err := r.GetProviderByKey("easypay")
	if err != nil {
		t.Fatalf("GetProviderByKey error: %v", err)
REDACTED
	if got.Name() != "EasyPay" {
		t.Fatalf("GetProviderByKey name = %q, want %q", got.Name(), "EasyPay")
REDACTED
REDACTED

func TestRegistryGetProviderByKeyNotFound(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	_, err := r.GetProviderByKey("nonexistent")
	if err == nil {
		t.Fatal("GetProviderByKey for unknown key should return error")
REDACTED
REDACTED

func TestRegistryGetProviderKeyUnknownType(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	key := r.GetProviderKey("unknown_type")
	if key != "" {
		t.Fatalf("GetProviderKey for unknown type should return empty, got %q", key)
REDACTED
REDACTED

func TestRegistryGetProviderKeyKnownType(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	p := &mockProvider{
		name:           "Stripe",
		key:            "stripe",
		supportedTypes: []PaymentType{TypeStripeREDACTED,
REDACTED
	r.Register(p)

	key := r.GetProviderKey(TypeStripe)
	if key != "stripe" {
		t.Fatalf("GetProviderKey(stripe) = %q, want %q", key, "stripe")
REDACTED
REDACTED

func TestRegistrySupportedTypes(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	p1 := &mockProvider{
		name:           "EasyPay",
		key:            "easypay",
		supportedTypes: []PaymentType{TypeAlipay, TypeWxpayREDACTED,
REDACTED
	p2 := &mockProvider{
		name:           "Stripe",
		key:            "stripe",
		supportedTypes: []PaymentType{TypeStripeREDACTED,
REDACTED
	r.Register(p1)
	r.Register(p2)

	types := r.SupportedTypes()
	if len(types) != 3 {
		t.Fatalf("SupportedTypes() len = %d, want 3", len(types))
REDACTED

	typeSet := make(map[PaymentType]bool)
	for _, tp := range types {
		typeSet[tp] = true
REDACTED
	for _, expected := range []PaymentType{TypeAlipay, TypeWxpay, TypeStripeREDACTED {
		if !typeSet[expected] {
			t.Fatalf("SupportedTypes() missing %q", expected)
	REDACTED
REDACTED
REDACTED

func TestRegistrySupportedTypesEmpty(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	types := r.SupportedTypes()
	if len(types) != 0 {
		t.Fatalf("SupportedTypes() on empty registry should be empty, got %d", len(types))
REDACTED
REDACTED

func TestRegistryOverwriteExisting(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	p1 := &mockProvider{
		name:           "OldPay",
		key:            "old",
		supportedTypes: []PaymentType{TypeAlipayREDACTED,
REDACTED
	p2 := &mockProvider{
		name:           "NewPay",
		key:            "new",
		supportedTypes: []PaymentType{TypeAlipayREDACTED,
REDACTED
	r.Register(p1)
	r.Register(p2)

	got, err := r.GetProvider(TypeAlipay)
	if err != nil {
		t.Fatalf("GetProvider error: %v", err)
REDACTED
	if got.Name() != "NewPay" {
		t.Fatalf("expected overwritten provider, got %q", got.Name())
REDACTED
REDACTED

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
				supportedTypes: []PaymentType{PaymentType(fmt.Sprintf("type-%d", idx))REDACTED,
		REDACTED
			r.Register(p)
	REDACTED(i)
REDACTED

	// Concurrent readers
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_ = r.SupportedTypes()
			_, _ = r.GetProvider("some-type")
			_ = r.GetProviderKey("some-type")
	REDACTED()
REDACTED

	wg.Wait()

	types := r.SupportedTypes()
	if len(types) != goroutines {
		t.Fatalf("after concurrent registration, expected %d types, got %d", goroutines, len(types))
REDACTED
REDACTED
