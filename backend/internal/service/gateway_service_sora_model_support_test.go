package service

import "testing"

func TestGatewayServiceIsModelSupportedByAccount_SoraNoMappingAllowsAll(t *testing.T) {
	svc := &GatewayService{REDACTED
	account := &Account{
		Platform:    PlatformSora,
REDACTEDREDACTED,
REDACTED

	if !svc.isModelSupportedByAccount(account, "sora2-landscape-10s") {
		t.Fatalf("expected sora model to be supported when model_mapping is empty")
REDACTED
REDACTED

func TestGatewayServiceIsModelSupportedByAccount_SoraLegacyNonSoraMappingDoesNotBlock(t *testing.T) {
	svc := &GatewayService{REDACTED
	account := &Account{
		Platform: PlatformSora,
REDACTED
			"model_mapping": map[string]any{
				"gpt-4o": "gpt-4o",
		REDACTED,
	REDACTED,
REDACTED

	if !svc.isModelSupportedByAccount(account, "sora2-landscape-10s") {
		t.Fatalf("expected sora model to be supported when mapping has no sora selectors")
REDACTED
REDACTED

func TestGatewayServiceIsModelSupportedByAccount_SoraFamilyAlias(t *testing.T) {
	svc := &GatewayService{REDACTED
	account := &Account{
		Platform: PlatformSora,
REDACTED
			"model_mapping": map[string]any{
				"sora2": "sora2",
		REDACTED,
	REDACTED,
REDACTED

	if !svc.isModelSupportedByAccount(account, "sora2-landscape-15s") {
		t.Fatalf("expected family selector sora2 to support sora2-landscape-15s")
REDACTED
REDACTED

func TestGatewayServiceIsModelSupportedByAccount_SoraUnderlyingModelAlias(t *testing.T) {
	svc := &GatewayService{REDACTED
	account := &Account{
		Platform: PlatformSora,
REDACTED
			"model_mapping": map[string]any{
				"sy_8": "sy_8",
		REDACTED,
	REDACTED,
REDACTED

	if !svc.isModelSupportedByAccount(account, "sora2-landscape-10s") {
		t.Fatalf("expected underlying model selector sy_8 to support sora2-landscape-10s")
REDACTED
REDACTED

func TestGatewayServiceIsModelSupportedByAccount_SoraExplicitImageSelectorBlocksVideo(t *testing.T) {
	svc := &GatewayService{REDACTED
	account := &Account{
		Platform: PlatformSora,
REDACTED
			"model_mapping": map[string]any{
				"gpt-image": "gpt-image",
		REDACTED,
	REDACTED,
REDACTED

	if svc.isModelSupportedByAccount(account, "sora2-landscape-10s") {
		t.Fatalf("expected video model to be blocked when mapping explicitly only allows gpt-image")
REDACTED
REDACTED
