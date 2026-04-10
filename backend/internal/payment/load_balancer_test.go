//go:build unit

package payment

import (
	"encoding/json"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

func TestInstanceSupportsType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		supportedTypes string
		target         PaymentType
		expected       bool
REDACTED{
		{
			name:           "exact match single type",
			supportedTypes: "alipay",
			target:         "alipay",
			expected:       true,
	REDACTED,
		{
			name:           "no match single type",
			supportedTypes: "wxpay",
			target:         "alipay",
			expected:       false,
	REDACTED,
		{
			name:           "match in comma-separated list",
			supportedTypes: "alipay,wxpay,stripe",
			target:         "wxpay",
			expected:       true,
	REDACTED,
		{
			name:           "first in comma-separated list",
			supportedTypes: "alipay,wxpay",
			target:         "alipay",
			expected:       true,
	REDACTED,
		{
			name:           "last in comma-separated list",
			supportedTypes: "alipay,wxpay,stripe",
			target:         "stripe",
			expected:       true,
	REDACTED,
		{
			name:           "no match in comma-separated list",
			supportedTypes: "alipay,wxpay",
			target:         "stripe",
			expected:       false,
	REDACTED,
		{
			name:           "empty target",
			supportedTypes: "alipay,wxpay",
			target:         "",
			expected:       false,
	REDACTED,
		{
			name:           "types with spaces are trimmed",
			supportedTypes: " alipay , wxpay ",
			target:         "alipay",
			expected:       true,
	REDACTED,
		{
			name:           "partial match should not succeed",
			supportedTypes: "alipay_direct",
			target:         "alipay",
			expected:       false,
	REDACTED,
		{
			name:           "empty supported types means all supported",
			supportedTypes: "",
			target:         "alipay",
			expected:       true,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := InstanceSupportsType(tt.supportedTypes, tt.target)
			if got != tt.expected {
				t.Fatalf("InstanceSupportsType(%q, %q) = %v, want %v", tt.supportedTypes, tt.target, got, tt.expected)
		REDACTED
	REDACTED)
REDACTED
REDACTED

// ---------------------------------------------------------------------------
// Helper to build test PaymentProviderInstance values
// ---------------------------------------------------------------------------

func testInstance(id int64, providerKey, limits string) *dbent.PaymentProviderInstance {
	return &dbent.PaymentProviderInstance{
		ID:          id,
		ProviderKey: providerKey,
		Limits:      limits,
		Enabled:     true,
REDACTED
REDACTED

// makeLimitsJSON builds a limits JSON string for a single payment type.
func makeLimitsJSON(paymentType string, cl ChannelLimits) string {
	m := map[string]ChannelLimits{paymentType: clREDACTED
	b, _ := json.Marshal(m)
	return string(b)
REDACTED

// ---------------------------------------------------------------------------
// filterByLimits
// ---------------------------------------------------------------------------

func TestFilterByLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		candidates  []instanceCandidate
		paymentType PaymentType
		orderAmount float64
		wantIDs     []int64 // expected surviving instance IDs
REDACTED{
		{
			name: "order below SingleMin is filtered out",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{SingleMin: 10REDACTED)), dailyUsed: 0REDACTED,
		REDACTED,
			paymentType: "alipay",
			orderAmount: 5,
			wantIDs:     nil,
	REDACTED,
		{
			name: "order at exact SingleMin boundary passes",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{SingleMin: 10REDACTED)), dailyUsed: 0REDACTED,
		REDACTED,
			paymentType: "alipay",
			orderAmount: 10,
			wantIDs:     []int64{1REDACTED,
	REDACTED,
		{
			name: "order above SingleMax is filtered out",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{SingleMax: 100REDACTED)), dailyUsed: 0REDACTED,
		REDACTED,
			paymentType: "alipay",
			orderAmount: 150,
			wantIDs:     nil,
	REDACTED,
		{
			name: "order at exact SingleMax boundary passes",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{SingleMax: 100REDACTED)), dailyUsed: 0REDACTED,
		REDACTED,
			paymentType: "alipay",
			orderAmount: 100,
			wantIDs:     []int64{1REDACTED,
	REDACTED,
		{
			name: "daily used + orderAmount exceeding dailyLimit is filtered out",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{DailyLimit: 500REDACTED)), dailyUsed: 480REDACTED,
		REDACTED,
			paymentType: "alipay",
			orderAmount: 30,
			wantIDs:     nil, // 480+30=510 > 500
	REDACTED,
		{
			name: "daily used + orderAmount equal to dailyLimit passes (strict greater-than)",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{DailyLimit: 500REDACTED)), dailyUsed: 480REDACTED,
		REDACTED,
			paymentType: "alipay",
			orderAmount: 20,
			wantIDs:     []int64{1REDACTED, // 480+20=500, 500 > 500 is false → passes
	REDACTED,
		{
			name: "daily used + orderAmount below dailyLimit passes",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{DailyLimit: 500REDACTED)), dailyUsed: 400REDACTED,
		REDACTED,
			paymentType: "alipay",
			orderAmount: 50,
			wantIDs:     []int64{1REDACTED,
	REDACTED,
		{
			name: "no limits configured passes through",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", ""), dailyUsed: 99999REDACTED,
		REDACTED,
			paymentType: "alipay",
			orderAmount: 100,
			wantIDs:     []int64{1REDACTED,
	REDACTED,
		{
			name: "multiple candidates with partial filtering",
			candidates: []instanceCandidate{
				// singleMax=50, order=80 → filtered out
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{SingleMax: 50REDACTED)), dailyUsed: 0REDACTED,
				// no limits → passes
				{inst: testInstance(2, "easypay", ""), dailyUsed: 0REDACTED,
				// singleMin=100, order=80 → filtered out
				{inst: testInstance(3, "easypay", makeLimitsJSON("alipay", ChannelLimits{SingleMin: 100REDACTED)), dailyUsed: 0REDACTED,
				// daily limit ok → passes (500+80=580 < 1000)
				{inst: testInstance(4, "easypay", makeLimitsJSON("alipay", ChannelLimits{DailyLimit: 1000REDACTED)), dailyUsed: 500REDACTED,
		REDACTED,
			paymentType: "alipay",
			orderAmount: 80,
			wantIDs:     []int64{2, 4REDACTED,
	REDACTED,
		{
			name: "zero SingleMin and SingleMax means no single-transaction limit",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{SingleMin: 0, SingleMax: 0, DailyLimit: 0REDACTED)), dailyUsed: 0REDACTED,
		REDACTED,
			paymentType: "alipay",
			orderAmount: 99999,
			wantIDs:     []int64{1REDACTED,
	REDACTED,
		{
			name: "all limits combined - order passes all checks",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{SingleMin: 10, SingleMax: 200, DailyLimit: 1000REDACTED)), dailyUsed: 500REDACTED,
		REDACTED,
			paymentType: "alipay",
			orderAmount: 50,
			wantIDs:     []int64{1REDACTED,
	REDACTED,
		{
			name: "all limits combined - order fails SingleMin",
			candidates: []instanceCandidate{
				{inst: testInstance(1, "easypay", makeLimitsJSON("alipay", ChannelLimits{SingleMin: 10, SingleMax: 200, DailyLimit: 1000REDACTED)), dailyUsed: 500REDACTED,
		REDACTED,
			paymentType: "alipay",
			orderAmount: 5,
			wantIDs:     nil,
	REDACTED,
		{
			name: "empty candidates returns empty",
			candidates:  nil,
			paymentType: "alipay",
			orderAmount: 10,
			wantIDs:     nil,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filterByLimits(tt.candidates, tt.paymentType, tt.orderAmount)
			gotIDs := make([]int64, len(got))
			for i, c := range got {
				gotIDs[i] = c.inst.ID
		REDACTED
			if !int64SliceEqual(gotIDs, tt.wantIDs) {
				t.Fatalf("filterByLimits() returned IDs %v, want %v", gotIDs, tt.wantIDs)
		REDACTED
	REDACTED)
REDACTED
REDACTED

// ---------------------------------------------------------------------------
// pickLeastAmount
// ---------------------------------------------------------------------------

func TestPickLeastAmount(t *testing.T) {
	t.Parallel()

	t.Run("picks candidate with lowest dailyUsed", func(t *testing.T) {
		t.Parallel()
		candidates := []instanceCandidate{
			{inst: testInstance(1, "easypay", ""), dailyUsed: 300REDACTED,
			{inst: testInstance(2, "easypay", ""), dailyUsed: 100REDACTED,
			{inst: testInstance(3, "easypay", ""), dailyUsed: 200REDACTED,
	REDACTED
		got := pickLeastAmount(candidates)
		if got.inst.ID != 2 {
			t.Fatalf("pickLeastAmount() picked instance %d, want 2", got.inst.ID)
	REDACTED
REDACTED)

	t.Run("with equal dailyUsed picks the first one", func(t *testing.T) {
		t.Parallel()
		candidates := []instanceCandidate{
			{inst: testInstance(1, "easypay", ""), dailyUsed: 100REDACTED,
			{inst: testInstance(2, "easypay", ""), dailyUsed: 100REDACTED,
			{inst: testInstance(3, "easypay", ""), dailyUsed: 200REDACTED,
	REDACTED
		got := pickLeastAmount(candidates)
		if got.inst.ID != 1 {
			t.Fatalf("pickLeastAmount() picked instance %d, want 1 (first with lowest)", got.inst.ID)
	REDACTED
REDACTED)

	t.Run("single candidate returns that candidate", func(t *testing.T) {
		t.Parallel()
		candidates := []instanceCandidate{
			{inst: testInstance(42, "easypay", ""), dailyUsed: 999REDACTED,
	REDACTED
		got := pickLeastAmount(candidates)
		if got.inst.ID != 42 {
			t.Fatalf("pickLeastAmount() picked instance %d, want 42", got.inst.ID)
	REDACTED
REDACTED)

	t.Run("zero usage among non-zero picks zero", func(t *testing.T) {
		t.Parallel()
		candidates := []instanceCandidate{
			{inst: testInstance(1, "easypay", ""), dailyUsed: 500REDACTED,
			{inst: testInstance(2, "easypay", ""), dailyUsed: 0REDACTED,
			{inst: testInstance(3, "easypay", ""), dailyUsed: 300REDACTED,
	REDACTED
		got := pickLeastAmount(candidates)
		if got.inst.ID != 2 {
			t.Fatalf("pickLeastAmount() picked instance %d, want 2", got.inst.ID)
	REDACTED
REDACTED)
REDACTED

// ---------------------------------------------------------------------------
// getInstanceChannelLimits
// ---------------------------------------------------------------------------

func TestGetInstanceChannelLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		inst        *dbent.PaymentProviderInstance
		paymentType PaymentType
		want        ChannelLimits
REDACTED{
		{
			name:        "empty limits string returns zero ChannelLimits",
			inst:        testInstance(1, "easypay", ""),
			paymentType: "alipay",
			want:        ChannelLimits{REDACTED,
	REDACTED,
		{
			name:        "invalid JSON returns zero ChannelLimits",
			inst:        testInstance(1, "easypay", "not-json{"),
			paymentType: "alipay",
			want:        ChannelLimits{REDACTED,
	REDACTED,
		{
			name: "valid JSON with matching payment type",
			inst: testInstance(1, "easypay",
				`{"alipay":{"singleMin":5,"singleMax":200,"dailyLimit":1000REDACTEDREDACTED`),
			paymentType: "alipay",
			want:        ChannelLimits{SingleMin: 5, SingleMax: 200, DailyLimit: 1000REDACTED,
	REDACTED,
		{
			name: "payment type not in limits returns zero ChannelLimits",
			inst: testInstance(1, "easypay",
				`{"alipay":{"singleMin":5,"singleMax":200REDACTEDREDACTED`),
			paymentType: "wxpay",
			want:        ChannelLimits{REDACTED,
	REDACTED,
		{
			name: "stripe provider uses stripe lookup key regardless of payment type",
			inst: testInstance(1, "stripe",
				`{"stripe":{"singleMin":10,"singleMax":500,"dailyLimit":5000REDACTEDREDACTED`),
			paymentType: "alipay",
			want:        ChannelLimits{SingleMin: 10, SingleMax: 500, DailyLimit: 5000REDACTED,
	REDACTED,
		{
			name: "stripe provider ignores payment type key even if present",
			inst: testInstance(1, "stripe",
				`{"stripe":{"singleMin":10,"singleMax":500REDACTED,"alipay":{"singleMin":1,"singleMax":100REDACTEDREDACTED`),
			paymentType: "alipay",
			want:        ChannelLimits{SingleMin: 10, SingleMax: 500REDACTED,
	REDACTED,
		{
			name: "non-stripe provider uses payment type as lookup key",
			inst: testInstance(1, "easypay",
				`{"alipay":{"singleMin":5REDACTED,"wxpay":{"singleMin":10REDACTEDREDACTED`),
			paymentType: "wxpay",
			want:        ChannelLimits{SingleMin: 10REDACTED,
	REDACTED,
		{
			name: "valid JSON with partial limits (only dailyLimit)",
			inst: testInstance(1, "easypay",
				`{"alipay":{"dailyLimit":800REDACTEDREDACTED`),
			paymentType: "alipay",
			want:        ChannelLimits{DailyLimit: 800REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := getInstanceChannelLimits(tt.inst, tt.paymentType)
			if got != tt.want {
				t.Fatalf("getInstanceChannelLimits() = %+v, want %+v", got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

// ---------------------------------------------------------------------------
// startOfDay
// ---------------------------------------------------------------------------

func TestStartOfDay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   time.Time
		want time.Time
REDACTED{
		{
			name: "midday returns midnight of same day",
			in:   time.Date(2025, 6, 15, 14, 30, 45, 123456789, time.UTC),
			want: time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
	REDACTED,
		{
			name: "midnight returns same time",
			in:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			want: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	REDACTED,
		{
			name: "last second of day returns midnight of same day",
			in:   time.Date(2025, 12, 31, 23, 59, 59, 999999999, time.UTC),
			want: time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
	REDACTED,
		{
			name: "preserves timezone location",
			in:   time.Date(2025, 3, 10, 15, 0, 0, 0, time.FixedZone("CST", 8*3600)),
			want: time.Date(2025, 3, 10, 0, 0, 0, 0, time.FixedZone("CST", 8*3600)),
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := startOfDay(tt.in)
			if !got.Equal(tt.want) {
				t.Fatalf("startOfDay(%v) = %v, want %v", tt.in, got, tt.want)
		REDACTED
			// Also verify location is preserved.
			if got.Location().String() != tt.want.Location().String() {
				t.Fatalf("startOfDay() location = %v, want %v", got.Location(), tt.want.Location())
		REDACTED
	REDACTED)
REDACTED
REDACTED

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// int64SliceEqual compares two int64 slices for equality.
// Both nil and empty slices are treated as equal.
func int64SliceEqual(a, b []int64) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
REDACTED
	if len(a) != len(b) {
		return false
REDACTED
	for i := range a {
		if a[i] != b[i] {
			return false
	REDACTED
REDACTED
	return true
REDACTED
