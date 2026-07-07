package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/payment"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestPcParseFloat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		defaultVal float64
		expected   float64
REDACTED{
		{"empty string returns default", "", 1.0, 1.0REDACTED,
		{"valid float", "3.14", 0, 3.14REDACTED,
		{"valid integer as float", "42", 0, 42.0REDACTED,
		{"invalid string returns default", "notanumber", 9.99, 9.99REDACTED,
		{"zero value", "0", 5.0, 0REDACTED,
		{"negative value", "-10.5", 0, -10.5REDACTED,
		{"very large value", "99999999.99", 0, 99999999.99REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := pcParseFloat(tt.input, tt.defaultVal)
			if got != tt.expected {
				t.Fatalf("pcParseFloat(%q, %v) = %v, want %v", tt.input, tt.defaultVal, got, tt.expected)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestPcParseInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		defaultVal int
		expected   int
REDACTED{
		{"empty string returns default", "", 30, 30REDACTED,
		{"valid int", "10", 0, 10REDACTED,
		{"invalid string returns default", "abc", 5, 5REDACTED,
		{"float string returns default", "3.14", 0, 0REDACTED,
		{"zero value", "0", 99, 0REDACTED,
		{"negative value", "-1", 0, -1REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := pcParseInt(tt.input, tt.defaultVal)
			if got != tt.expected {
				t.Fatalf("pcParseInt(%q, %v) = %v, want %v", tt.input, tt.defaultVal, got, tt.expected)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestParsePaymentConfig(t *testing.T) {
	t.Parallel()

	svc := &PaymentConfigService{REDACTED

	t.Run("empty vals uses defaults", func(t *testing.T) {
		t.Parallel()
		cfg := svc.parsePaymentConfig(map[string]string{REDACTED)
		if cfg.Enabled {
			t.Fatal("expected Enabled=false by default")
	REDACTED
		if cfg.MinAmount != 1 {
			t.Fatalf("expected MinAmount=1, got %v", cfg.MinAmount)
	REDACTED
		if cfg.MaxAmount != 0 {
			t.Fatalf("expected MaxAmount=0 (no limit), got %v", cfg.MaxAmount)
	REDACTED
		if cfg.OrderTimeoutMin != 30 {
			t.Fatalf("expected OrderTimeoutMin=30, got %v", cfg.OrderTimeoutMin)
	REDACTED
		if cfg.MaxPendingOrders != 3 {
			t.Fatalf("expected MaxPendingOrders=3, got %v", cfg.MaxPendingOrders)
	REDACTED
		if cfg.LoadBalanceStrategy != payment.DefaultLoadBalanceStrategy {
			t.Fatalf("expected LoadBalanceStrategy=%s, got %q", payment.DefaultLoadBalanceStrategy, cfg.LoadBalanceStrategy)
	REDACTED
		if len(cfg.EnabledTypes) != 0 {
			t.Fatalf("expected empty EnabledTypes, got %v", cfg.EnabledTypes)
	REDACTED
REDACTED)

	t.Run("all values populated", func(t *testing.T) {
		t.Parallel()
		vals := map[string]string{
			SettingPaymentEnabled:      "true",
			SettingMinRechargeAmount:   "5.00",
			SettingMaxRechargeAmount:   "1000.00",
			SettingDailyRechargeLimit:  "5000.00",
			SettingOrderTimeoutMinutes: "15",
			SettingMaxPendingOrders:    "5",
			SettingEnabledPaymentTypes: "alipay,wxpay,stripe",
			SettingBalancePayDisabled:  "true",
			SettingLoadBalanceStrategy: "least_amount",
			SettingProductNamePrefix:   "PRE",
			SettingProductNameSuffix:   "SUF",
	REDACTED
		cfg := svc.parsePaymentConfig(vals)

		if !cfg.Enabled {
			t.Fatal("expected Enabled=true")
	REDACTED
		if cfg.MinAmount != 5 {
			t.Fatalf("MinAmount = %v, want 5", cfg.MinAmount)
	REDACTED
		if cfg.MaxAmount != 1000 {
			t.Fatalf("MaxAmount = %v, want 1000", cfg.MaxAmount)
	REDACTED
		if cfg.DailyLimit != 5000 {
			t.Fatalf("DailyLimit = %v, want 5000", cfg.DailyLimit)
	REDACTED
		if cfg.OrderTimeoutMin != 15 {
			t.Fatalf("OrderTimeoutMin = %v, want 15", cfg.OrderTimeoutMin)
	REDACTED
		if cfg.MaxPendingOrders != 5 {
			t.Fatalf("MaxPendingOrders = %v, want 5", cfg.MaxPendingOrders)
	REDACTED
		if len(cfg.EnabledTypes) != 3 {
			t.Fatalf("EnabledTypes len = %d, want 3", len(cfg.EnabledTypes))
	REDACTED
		if cfg.EnabledTypes[0] != "alipay" || cfg.EnabledTypes[1] != "wxpay" || cfg.EnabledTypes[2] != "stripe" {
			t.Fatalf("EnabledTypes = %v, want [alipay wxpay stripe]", cfg.EnabledTypes)
	REDACTED
		if !cfg.BalanceDisabled {
			t.Fatal("expected BalanceDisabled=true")
	REDACTED
		if cfg.LoadBalanceStrategy != "least_amount" {
			t.Fatalf("LoadBalanceStrategy = %q, want %q", cfg.LoadBalanceStrategy, "least_amount")
	REDACTED
		if cfg.ProductNamePrefix != "PRE" {
			t.Fatalf("ProductNamePrefix = %q, want %q", cfg.ProductNamePrefix, "PRE")
	REDACTED
		if cfg.ProductNameSuffix != "SUF" {
			t.Fatalf("ProductNameSuffix = %q, want %q", cfg.ProductNameSuffix, "SUF")
	REDACTED
REDACTED)

	t.Run("enabled types with spaces are trimmed", func(t *testing.T) {
		t.Parallel()
		vals := map[string]string{
			SettingEnabledPaymentTypes: " alipay , wxpay ",
	REDACTED
		cfg := svc.parsePaymentConfig(vals)
		if len(cfg.EnabledTypes) != 2 {
			t.Fatalf("EnabledTypes len = %d, want 2", len(cfg.EnabledTypes))
	REDACTED
		if cfg.EnabledTypes[0] != "alipay" || cfg.EnabledTypes[1] != "wxpay" {
			t.Fatalf("EnabledTypes = %v, want [alipay wxpay]", cfg.EnabledTypes)
	REDACTED
REDACTED)

	t.Run("enabled types are normalized to visible methods and deduplicated", func(t *testing.T) {
		t.Parallel()
		vals := map[string]string{
			SettingEnabledPaymentTypes: "alipay_direct, alipay, wxpay_direct, wxpay",
	REDACTED
		cfg := svc.parsePaymentConfig(vals)
		if len(cfg.EnabledTypes) != 2 {
			t.Fatalf("EnabledTypes len = %d, want 2", len(cfg.EnabledTypes))
	REDACTED
		if cfg.EnabledTypes[0] != "alipay" || cfg.EnabledTypes[1] != "wxpay" {
			t.Fatalf("EnabledTypes = %v, want [alipay wxpay]", cfg.EnabledTypes)
	REDACTED
REDACTED)

	t.Run("custom enabled types are preserved", func(t *testing.T) {
		t.Parallel()
		vals := map[string]string{
			SettingEnabledPaymentTypes: "alipay,ldc,usdt_trc20",
	REDACTED
		cfg := svc.parsePaymentConfig(vals)
		want := []string{"alipay", "ldc", "usdt_trc20"REDACTED
		if len(cfg.EnabledTypes) != len(want) {
			t.Fatalf("EnabledTypes len = %d, want %d (%v)", len(cfg.EnabledTypes), len(want), cfg.EnabledTypes)
	REDACTED
		for i := range want {
			if cfg.EnabledTypes[i] != want[i] {
				t.Fatalf("EnabledTypes[%d] = %q, want %q (full=%v)", i, cfg.EnabledTypes[i], want[i], cfg.EnabledTypes)
		REDACTED
	REDACTED
REDACTED)

	t.Run("empty enabled types string", func(t *testing.T) {
		t.Parallel()
		vals := map[string]string{
			SettingEnabledPaymentTypes: "",
	REDACTED
		cfg := svc.parsePaymentConfig(vals)
		if len(cfg.EnabledTypes) != 0 {
			t.Fatalf("expected empty EnabledTypes for empty string, got %v", cfg.EnabledTypes)
	REDACTED
REDACTED)
REDACTED

func TestGetBasePaymentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    string
		expected string
REDACTED{
		{payment.TypeEasyPay, payment.TypeEasyPayREDACTED,
		{payment.TypeStripe, payment.TypeStripeREDACTED,
		{payment.TypeCard, payment.TypeStripeREDACTED,
		{payment.TypeLink, payment.TypeStripeREDACTED,
		{payment.TypeAlipay, payment.TypeAlipayREDACTED,
		{payment.TypeAlipayDirect, payment.TypeAlipayREDACTED,
		{payment.TypeWxpay, payment.TypeWxpayREDACTED,
		{payment.TypeWxpayDirect, payment.TypeWxpayREDACTED,
		{"unknown", "unknown"REDACTED,
		{"", ""REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := payment.GetBasePaymentType(tt.input)
			if got != tt.expected {
				t.Fatalf("GetBasePaymentType(%q) = %q, want %q", tt.input, got, tt.expected)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestApplyVisibleMethodRoutingToEnabledTypes(t *testing.T) {
	t.Parallel()

	base := []string{"alipay", "wxpay", "stripe"REDACTED
	vals := map[string]string{
		SettingPaymentVisibleMethodAlipayEnabled: "true",
		SettingPaymentVisibleMethodAlipaySource:  VisibleMethodSourceOfficialAlipay,
		SettingPaymentVisibleMethodWxpayEnabled:  "true",
		SettingPaymentVisibleMethodWxpaySource:   VisibleMethodSourceOfficialWechat,
REDACTED
	available := map[string]bool{
		VisibleMethodSourceOfficialAlipay: true,
		VisibleMethodSourceOfficialWechat: false,
REDACTED

	got := applyVisibleMethodRoutingToEnabledTypes(base, vals, available)
	want := []string{"alipay", "stripe"REDACTED
	if len(got) != len(want) {
		t.Fatalf("applyVisibleMethodRoutingToEnabledTypes len = %d, want %d (%v)", len(got), len(want), got)
REDACTED
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("applyVisibleMethodRoutingToEnabledTypes[%d] = %q, want %q (full=%v)", i, got[i], want[i], got)
	REDACTED
REDACTED
REDACTED

func TestApplyVisibleMethodRoutingAddsConfiguredVisibleMethod(t *testing.T) {
	t.Parallel()

	base := []string{"stripe"REDACTED
	vals := map[string]string{
		SettingPaymentVisibleMethodAlipayEnabled: "true",
		SettingPaymentVisibleMethodAlipaySource:  VisibleMethodSourceEasyPayAlipay,
REDACTED
	available := map[string]bool{
		VisibleMethodSourceEasyPayAlipay: true,
REDACTED

	got := applyVisibleMethodRoutingToEnabledTypes(base, vals, available)
	want := []string{"stripe", "alipay"REDACTED
	if len(got) != len(want) {
		t.Fatalf("applyVisibleMethodRoutingToEnabledTypes len = %d, want %d (%v)", len(got), len(want), got)
REDACTED
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("applyVisibleMethodRoutingToEnabledTypes[%d] = %q, want %q (full=%v)", i, got[i], want[i], got)
	REDACTED
REDACTED
REDACTED

func TestBuildVisibleMethodSourceAvailability(t *testing.T) {
	t.Parallel()

	instances := []*dbent.PaymentProviderInstance{
		{ProviderKey: payment.TypeAlipay, SupportedTypes: "alipay"REDACTED,
		{ProviderKey: payment.TypeEasyPay, SupportedTypes: "wxpay_direct, alipay"REDACTED,
		{ProviderKey: payment.TypeWxpay, SupportedTypes: "wxpay_direct"REDACTED,
REDACTED

	got := buildVisibleMethodSourceAvailability(instances)
	if !got[VisibleMethodSourceOfficialAlipay] {
		t.Fatalf("expected %q to be available", VisibleMethodSourceOfficialAlipay)
REDACTED
	if !got[VisibleMethodSourceEasyPayAlipay] {
		t.Fatalf("expected %q to be available", VisibleMethodSourceEasyPayAlipay)
REDACTED
	if !got[VisibleMethodSourceOfficialWechat] {
		t.Fatalf("expected %q to be available", VisibleMethodSourceOfficialWechat)
REDACTED
	if !got[VisibleMethodSourceEasyPayWechat] {
		t.Fatalf("expected %q to be available", VisibleMethodSourceEasyPayWechat)
REDACTED
REDACTED

func TestGetPaymentConfigKeepsStoredEnabledTypes(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	_, err := client.PaymentProviderInstance.Create().
		SetProviderKey(payment.TypeEasyPay).
		SetName("EasyPay Alipay").
		SetConfig("{REDACTED").
		SetSupportedTypes("alipay").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create easypay instance: %v", err)
REDACTED

	svc := &PaymentConfigService{
		entClient: client,
		settingRepo: &paymentConfigSettingRepoStub{
			values: map[string]string{
				SettingEnabledPaymentTypes: "alipay,wxpay,stripe",
		REDACTED,
	REDACTED,
REDACTED

	cfg, err := svc.GetPaymentConfig(ctx)
	if err != nil {
		t.Fatalf("GetPaymentConfig returned error: %v", err)
REDACTED

	want := []string{payment.TypeAlipay, payment.TypeWxpay, payment.TypeStripeREDACTED
	if len(cfg.EnabledTypes) != len(want) {
		t.Fatalf("EnabledTypes len = %d, want %d (%v)", len(cfg.EnabledTypes), len(want), cfg.EnabledTypes)
REDACTED
	for i := range want {
		if cfg.EnabledTypes[i] != want[i] {
			t.Fatalf("EnabledTypes[%d] = %q, want %q (full=%v)", i, cfg.EnabledTypes[i], want[i], cfg.EnabledTypes)
	REDACTED
REDACTED
REDACTED

func newPaymentConfigServiceTestClient(t *testing.T) *dbent.Client {
REDACTED

	dbName := fmt.Sprintf(
		"file:%s?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()),
	)
	db, err := sql.Open("sqlite", dbName)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
REDACTED
	t.Cleanup(func() { _ = db.Close() REDACTED)

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
REDACTED

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() REDACTED)
	return client
REDACTED

type paymentConfigSettingRepoStub struct {
	values  map[string]string
	updates map[string]string
REDACTED

func (s *paymentConfigSettingRepoStub) Get(context.Context, string) (*Setting, error) {
	return nil, nil
REDACTED
func (s *paymentConfigSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
REDACTED
func (s *paymentConfigSettingRepoStub) Set(context.Context, string, string) error { return nil REDACTED
func (s *paymentConfigSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = s.values[key]
REDACTED
	return out, nil
REDACTED
func (s *paymentConfigSettingRepoStub) SetMultiple(_ context.Context, values map[string]string) error {
	s.updates = make(map[string]string, len(values))
	for key, value := range values {
		s.updates[key] = value
		if s.values == nil {
			s.values = map[string]string{REDACTED
	REDACTED
		s.values[key] = value
REDACTED
	return nil
REDACTED
func (s *paymentConfigSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return s.values, nil
REDACTED
func (s *paymentConfigSettingRepoStub) Delete(context.Context, string) error { return nil REDACTED

func TestUpdatePaymentConfig_PersistsVisibleMethodRouting(t *testing.T) {
	repo := &paymentConfigSettingRepoStub{values: map[string]string{REDACTEDREDACTED
	svc := &PaymentConfigService{settingRepo: repoREDACTED

	alipayEnabled := true
	wxpayEnabled := false
	err := svc.UpdatePaymentConfig(context.Background(), UpdatePaymentConfigRequest{
		VisibleMethodAlipayEnabled: &alipayEnabled,
		VisibleMethodAlipaySource:  paymentConfigStrPtr(VisibleMethodSourceEasyPayAlipay),
		VisibleMethodWxpayEnabled:  &wxpayEnabled,
		VisibleMethodWxpaySource:   paymentConfigStrPtr(VisibleMethodSourceOfficialWechat),
REDACTED)
	if err != nil {
		t.Fatalf("UpdatePaymentConfig returned error: %v", err)
REDACTED

	if repo.values[SettingPaymentVisibleMethodAlipayEnabled] != "true" {
		t.Fatalf("alipay enabled = %q, want true", repo.values[SettingPaymentVisibleMethodAlipayEnabled])
REDACTED
	if repo.values[SettingPaymentVisibleMethodAlipaySource] != VisibleMethodSourceEasyPayAlipay {
		t.Fatalf("alipay source = %q, want %q", repo.values[SettingPaymentVisibleMethodAlipaySource], VisibleMethodSourceEasyPayAlipay)
REDACTED
	if repo.values[SettingPaymentVisibleMethodWxpayEnabled] != "false" {
		t.Fatalf("wxpay enabled = %q, want false", repo.values[SettingPaymentVisibleMethodWxpayEnabled])
REDACTED
	if repo.values[SettingPaymentVisibleMethodWxpaySource] != VisibleMethodSourceOfficialWechat {
		t.Fatalf("wxpay source = %q, want %q", repo.values[SettingPaymentVisibleMethodWxpaySource], VisibleMethodSourceOfficialWechat)
REDACTED
REDACTED

func paymentConfigStrPtr(value string) *string {
	return &value
REDACTED
