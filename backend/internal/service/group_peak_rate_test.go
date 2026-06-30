package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

func init() {
	// 测试固定全局时区为 UTC，确保判定可复现。
	_ = timezone.Init("UTC")
REDACTED

func newPeakGroup(enabled bool, start, end string, mult float64) *Group {
	return &Group{
		SubscriptionType:   "subscription",
		PeakRateEnabled:    enabled,
		PeakStart:          start,
		PeakEnd:            end,
		PeakRateMultiplier: mult,
REDACTED
REDACTED

func at(hour, min int) time.Time {
	return time.Date(2026, 6, 29, hour, min, 0, 0, time.UTC)
REDACTED

func TestPeakMultiplierAt_DisabledOrUnconfigured(t *testing.T) {
	cases := []struct {
		name string
		g    *Group
REDACTED{
		{"disabled", newPeakGroup(false, "14:00", "18:00", 3.0)REDACTED,
		{"empty start", newPeakGroup(true, "", "18:00", 3.0)REDACTED,
		{"empty end", newPeakGroup(true, "14:00", "", 3.0)REDACTED,
		{"invalid start>=end", newPeakGroup(true, "18:00", "14:00", 3.0)REDACTED,
		{"equal start==end", newPeakGroup(true, "14:00", "14:00", 3.0)REDACTED,
		{"malformed start", newPeakGroup(true, "99:99", "18:00", 3.0)REDACTED,
REDACTED
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.g.PeakMultiplierAt(at(15, 0)); got != 1.0 {
				t.Fatalf("expect 1.0, got %v", got)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestPeakMultiplierAt_NilReceiver(t *testing.T) {
	var g *Group
	if got := g.PeakMultiplierAt(at(15, 0)); got != 1.0 {
		t.Fatalf("expect 1.0, got %v", got)
REDACTED
REDACTED

func TestPeakMultiplierAt_Boundaries(t *testing.T) {
	g := newPeakGroup(true, "14:00", "18:00", 3.0)
	cases := []struct {
		t    time.Time
		want float64
REDACTED{
		{at(13, 59), 1.0REDACTED,
		{at(14, 0), 3.0REDACTED,
		{at(15, 30), 3.0REDACTED,
		{at(17, 59), 3.0REDACTED,
		{at(18, 0), 1.0REDACTED,
		{at(23, 0), 1.0REDACTED,
REDACTED
	for _, c := range cases {
		t.Run(c.t.Format("15:04"), func(t *testing.T) {
			if got := g.PeakMultiplierAt(c.t); got != c.want {
				t.Fatalf("at %s: expect %v, got %v", c.t.Format("15:04"), c.want, got)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestPeakMultiplierAt_RespectsTimezoneLocation(t *testing.T) {
	// 全局时区为 UTC。北京 15:00 = UTC 07:00，不在 [14:00,18:00)。
	nonUTC := time.Date(2026, 6, 29, 15, 0, 0, 0, mustLoad("Asia/Shanghai"))
	g := newPeakGroup(true, "14:00", "18:00", 3.0)
	if got := g.PeakMultiplierAt(nonUTC); got != 1.0 {
		t.Fatalf("expect 1.0 (converted to UTC 07:00), got %v", got)
REDACTED
REDACTED

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
REDACTED
	return loc
REDACTED

func TestValidatePeakRateConfig(t *testing.T) {
	cases := []struct {
		name    string
		subType string
		enabled bool
		start   string
		end     string
		mult    float64
		wantErr bool
REDACTED{
		{"disabled passes through", "subscription", false, "", "", 0, falseREDACTED,
		{"subscription enabled valid", "subscription", true, "14:00", "18:00", 3.0, falseREDACTED,
		{"standard enabled rejected", "standard", true, "14:00", "18:00", 3.0, trueREDACTED,
		{"empty type treated as standard", "", true, "14:00", "18:00", 3.0, trueREDACTED,
		{"standard disabled passes", "standard", false, "", "", 0, falseREDACTED,
		{"enabled empty start", "subscription", true, "", "18:00", 1.0, trueREDACTED,
		{"enabled empty end", "subscription", true, "14:00", "", 1.0, trueREDACTED,
		{"enabled malformed start", "subscription", true, "99:99", "18:00", 1.0, trueREDACTED,
		{"enabled malformed end", "subscription", true, "14:00", "25:00", 1.0, trueREDACTED,
		{"enabled equal start==end", "subscription", true, "14:00", "14:00", 1.0, trueREDACTED,
		{"enabled cross-day rejected", "subscription", true, "22:00", "02:00", 1.0, trueREDACTED,
		{"enabled negative multiplier", "subscription", true, "14:00", "18:00", -0.5, trueREDACTED,
		{"enabled zero multiplier allowed", "subscription", true, "14:00", "18:00", 0, falseREDACTED,
REDACTED
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidatePeakRateConfig(c.subType, c.enabled, c.start, c.end, c.mult)
			if c.wantErr && err == nil {
				t.Fatalf("expect error, got nil")
		REDACTED
			if !c.wantErr && err != nil {
				t.Fatalf("expect no error, got %v", err)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestPeakMultiplierAt_StandardTypeDegradesToOne(t *testing.T) {
	g := newPeakGroup(true, "14:00", "18:00", 3.0)
	g.SubscriptionType = "standard"
	if got := g.PeakMultiplierAt(at(15, 30)); got != 1.0 {
		t.Fatalf("standard group must degrade to 1.0, got %v", got)
REDACTED

	sub := newPeakGroup(true, "14:00", "18:00", 3.0)
	sub.SubscriptionType = "subscription"
	if got := sub.PeakMultiplierAt(at(15, 30)); got != 3.0 {
		t.Fatalf("subscription group peak multiplier: got %v, want 3.0", got)
REDACTED
REDACTED

// TestPeakMultiplier_GatewayBillingSequence 调用 gateway_service.recordUsageCore 与
// openai_gateway_service.RecordUsage 共用的 computePeakAwareMultipliers，验证计费叠加顺序：
// 图片倍率基于基础倍率算出且不受高峰影响，高峰因子只乘入文本倍率。
// 若有人调换叠加顺序或把高峰并入 imageMultiplier，此测试会失败。
func TestPeakMultiplier_GatewayBillingSequence(t *testing.T) {
	const baseMultiplier = 0.8
	apiKey := &APIKey{Group: newPeakGroup(true, "14:00", "18:00", 3.0)REDACTED
	approxEq := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 REDACTED

	t.Run("peak hour amplifies text only", func(t *testing.T) {
		now := at(15, 30) // 处于 [14:00, 18:00)
		textMultiplier, imageMultiplier := computePeakAwareMultipliers(apiKey, baseMultiplier, now)
		if !approxEq(imageMultiplier, baseMultiplier) {
			t.Fatalf("image multiplier must not be affected by peak: got %v, want %v", imageMultiplier, baseMultiplier)
	REDACTED
		if want := baseMultiplier * 3.0; !approxEq(textMultiplier, want) {
			t.Fatalf("text multiplier should include peak factor: got %v, want %v", textMultiplier, want)
	REDACTED
REDACTED)

	t.Run("off-peak leaves both multipliers at base", func(t *testing.T) {
		now := at(20, 0)
		textMultiplier, imageMultiplier := computePeakAwareMultipliers(apiKey, baseMultiplier, now)
		if !approxEq(imageMultiplier, baseMultiplier) {
			t.Fatalf("image multiplier: got %v, want %v", imageMultiplier, baseMultiplier)
	REDACTED
		if !approxEq(textMultiplier, baseMultiplier) {
			t.Fatalf("text multiplier should equal base off-peak: got %v, want %v", textMultiplier, baseMultiplier)
	REDACTED
REDACTED)

	t.Run("image independent mode decoupled from peak", func(t *testing.T) {
		indGroup := newPeakGroup(true, "14:00", "18:00", 3.0)
		indGroup.ImageRateIndependent = true
		indGroup.ImageRateMultiplier = 0.5
		indKey := &APIKey{Group: indGroupREDACTED
		now := at(15, 30)
		textMultiplier, imageMultiplier := computePeakAwareMultipliers(indKey, baseMultiplier, now)
		if !approxEq(imageMultiplier, 0.5) {
			t.Fatalf("independent image multiplier: got %v, want 0.5", imageMultiplier)
	REDACTED
		if want := baseMultiplier * 3.0; !approxEq(textMultiplier, want) {
			t.Fatalf("text multiplier should include peak factor: got %v, want %v", textMultiplier, want)
	REDACTED
REDACTED)

	t.Run("nil api key degrades to base multipliers", func(t *testing.T) {
		now := at(15, 30)
		textMultiplier, imageMultiplier := computePeakAwareMultipliers(nil, baseMultiplier, now)
		if !approxEq(textMultiplier, baseMultiplier) {
			t.Fatalf("nil group text multiplier: got %v, want %v", textMultiplier, baseMultiplier)
	REDACTED
		if !approxEq(imageMultiplier, baseMultiplier) {
			t.Fatalf("nil group image multiplier: got %v, want %v", imageMultiplier, baseMultiplier)
	REDACTED
REDACTED)
REDACTED

// TestPeakMultiplier_SnapshotRoundTrip 防回归：认证缓存快照（APIKeyAuthGroupSnapshot）
// 必须携带高峰倍率 4 字段，否则扣费路径拿到的 apiKey.Group 会缺字段、PeakMultiplierAt 恒降级为 1.0。
// 调用真实链路 snapshotFromAPIKey → snapshotToAPIKey，验证 peak 配置经快照往返后仍生效。
func TestPeakMultiplier_SnapshotRoundTrip(t *testing.T) {
	apiKey := &APIKey{
		User:  &User{ID: 1, Status: StatusActive, Role: RoleUserREDACTED,
		Group: newPeakGroup(true, "14:00", "18:00", 3.0),
REDACTED
	svc := &APIKeyService{REDACTED

	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	if snapshot == nil || snapshot.Group == nil {
		t.Fatalf("snapshot or snapshot.Group must not be nil")
REDACTED
	restored := svc.snapshotToAPIKey("k", snapshot)
	if restored.Group == nil {
		t.Fatalf("restored.Group must not be nil")
REDACTED

	if !restored.Group.PeakRateEnabled ||
		restored.Group.PeakStart != "14:00" ||
		restored.Group.PeakEnd != "18:00" ||
		restored.Group.PeakRateMultiplier != 3.0 {
		t.Fatalf("peak fields lost in snapshot round-trip: %+v", restored.Group)
REDACTED
	if got := restored.Group.PeakMultiplierAt(at(15, 30)); got != 3.0 {
		t.Fatalf("peak hour multiplier after round-trip: got %v, want 3.0", got)
REDACTED
	if got := restored.Group.PeakMultiplierAt(at(20, 0)); got != 1.0 {
		t.Fatalf("off-peak multiplier after round-trip: got %v, want 1.0", got)
REDACTED
REDACTED
