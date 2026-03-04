package service

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCollectSelectionFailureStats(t *testing.T) {
	svc := &GatewayService{REDACTED
	model := "sora2-landscape-10s"
	resetAt := time.Now().Add(2 * time.Minute).Format(time.RFC3339)

	accounts := []Account{
		// excluded
		{
			ID:          1,
			Platform:    PlatformSora,
			Status:      StatusActive,
			Schedulable: true,
	REDACTED,
		// unschedulable
		{
			ID:          2,
			Platform:    PlatformSora,
			Status:      StatusActive,
			Schedulable: false,
	REDACTED,
		// platform filtered
		{
			ID:          3,
			Platform:    PlatformOpenAI,
			Status:      StatusActive,
			Schedulable: true,
	REDACTED,
		// model unsupported
		{
			ID:          4,
			Platform:    PlatformSora,
			Status:      StatusActive,
			Schedulable: true,
	REDACTED
				"model_mapping": map[string]any{
					"gpt-image": "gpt-image",
			REDACTED,
		REDACTED,
	REDACTED,
		// model rate limited
		{
			ID:          5,
			Platform:    PlatformSora,
			Status:      StatusActive,
			Schedulable: true,
			Extra: map[string]any{
				"model_rate_limits": map[string]any{
					model: map[string]any{
						"rate_limit_reset_at": resetAt,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
		// eligible
		{
			ID:          6,
			Platform:    PlatformSora,
			Status:      StatusActive,
			Schedulable: true,
	REDACTED,
REDACTED

	excluded := map[int64]struct{REDACTED{1: {REDACTEDREDACTED
	stats := svc.collectSelectionFailureStats(context.Background(), accounts, model, PlatformSora, excluded, false)

	if stats.Total != 6 {
		t.Fatalf("total=%d want=6", stats.Total)
REDACTED
	if stats.Excluded != 1 {
		t.Fatalf("excluded=%d want=1", stats.Excluded)
REDACTED
	if stats.Unschedulable != 1 {
		t.Fatalf("unschedulable=%d want=1", stats.Unschedulable)
REDACTED
	if stats.PlatformFiltered != 1 {
		t.Fatalf("platform_filtered=%d want=1", stats.PlatformFiltered)
REDACTED
	if stats.ModelUnsupported != 1 {
		t.Fatalf("model_unsupported=%d want=1", stats.ModelUnsupported)
REDACTED
	if stats.ModelRateLimited != 1 {
		t.Fatalf("model_rate_limited=%d want=1", stats.ModelRateLimited)
REDACTED
	if stats.Eligible != 1 {
		t.Fatalf("eligible=%d want=1", stats.Eligible)
REDACTED
REDACTED

func TestDiagnoseSelectionFailure_SoraUnschedulableDetail(t *testing.T) {
	svc := &GatewayService{REDACTED
	acc := &Account{
		ID:          7,
		Platform:    PlatformSora,
		Status:      StatusActive,
		Schedulable: false,
REDACTED

	diagnosis := svc.diagnoseSelectionFailure(context.Background(), acc, "sora2-landscape-10s", PlatformSora, map[int64]struct{REDACTED{REDACTED, false)
	if diagnosis.Category != "unschedulable" {
		t.Fatalf("category=%s want=unschedulable", diagnosis.Category)
REDACTED
	if diagnosis.Detail != "schedulable=false" {
		t.Fatalf("detail=%s want=schedulable=false", diagnosis.Detail)
REDACTED
REDACTED

func TestDiagnoseSelectionFailure_SoraModelRateLimitedDetail(t *testing.T) {
	svc := &GatewayService{REDACTED
	model := "sora2-landscape-10s"
	resetAt := time.Now().Add(2 * time.Minute).UTC().Format(time.RFC3339)
	acc := &Account{
		ID:          8,
		Platform:    PlatformSora,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			"model_rate_limits": map[string]any{
				model: map[string]any{
					"rate_limit_reset_at": resetAt,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	diagnosis := svc.diagnoseSelectionFailure(context.Background(), acc, model, PlatformSora, map[int64]struct{REDACTED{REDACTED, false)
	if diagnosis.Category != "model_rate_limited" {
		t.Fatalf("category=%s want=model_rate_limited", diagnosis.Category)
REDACTED
	if !strings.Contains(diagnosis.Detail, "remaining=") {
		t.Fatalf("detail=%s want contains remaining=", diagnosis.Detail)
REDACTED
REDACTED
