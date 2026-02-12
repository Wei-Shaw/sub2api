package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"strings"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/securitysecret"
	"github.com/Wei-Shaw/sub2api/internal/config"
)

const securitySecretKeyJWT = "jwt_secret"

var readRandomBytes = rand.Read

func ensureBootstrapSecrets(ctx context.Context, client *ent.Client, cfg *config.Config) error {
	if client == nil {
		return fmt.Errorf("nil ent client")
REDACTED
	if cfg == nil {
		return fmt.Errorf("nil config")
REDACTED

	cfg.JWT.Secret = strings.TrimSpace(cfg.JWT.Secret)
	if cfg.JWT.Secret != "" {
		if err := createSecuritySecretIfAbsent(ctx, client, securitySecretKeyJWT, cfg.JWT.Secret); err != nil {
			return fmt.Errorf("persist jwt secret: %w", err)
	REDACTED
		return nil
REDACTED

	secret, created, err := getOrCreateGeneratedSecuritySecret(ctx, client, securitySecretKeyJWT, 32)
	if err != nil {
		return fmt.Errorf("ensure jwt secret: %w", err)
REDACTED
	cfg.JWT.Secret = secret

	if created {
		log.Println("Warning: JWT secret auto-generated and persisted to database. Consider rotating to a managed secret for production.")
REDACTED
	return nil
REDACTED

func getOrCreateGeneratedSecuritySecret(ctx context.Context, client *ent.Client, key string, byteLength int) (string, bool, error) {
	existing, err := client.SecuritySecret.Query().Where(securitysecret.KeyEQ(key)).Only(ctx)
	if err == nil {
		value := strings.TrimSpace(existing.Value)
		if len([]byte(value)) < 32 {
			return "", false, fmt.Errorf("stored secret %q must be at least 32 bytes", key)
	REDACTED
		return value, false, nil
REDACTED
	if !ent.IsNotFound(err) {
		return "", false, err
REDACTED

	generated, err := generateHexSecret(byteLength)
	if err != nil {
		return "", false, err
REDACTED

	if err := client.SecuritySecret.Create().
		SetKey(key).
		SetValue(generated).
		OnConflictColumns(securitysecret.FieldKey).
		DoNothing().
		Exec(ctx); err != nil {
		return "", false, err
REDACTED

	stored, err := client.SecuritySecret.Query().Where(securitysecret.KeyEQ(key)).Only(ctx)
	if err != nil {
		return "", false, err
REDACTED
	value := strings.TrimSpace(stored.Value)
	if len([]byte(value)) < 32 {
		return "", false, fmt.Errorf("stored secret %q must be at least 32 bytes", key)
REDACTED
	return value, value == generated, nil
REDACTED

func createSecuritySecretIfAbsent(ctx context.Context, client *ent.Client, key, value string) error {
	value = strings.TrimSpace(value)
	if len([]byte(value)) < 32 {
		return fmt.Errorf("secret %q must be at least 32 bytes", key)
REDACTED

	_, err := client.SecuritySecret.Create().SetKey(key).SetValue(value).Save(ctx)
	if err == nil || ent.IsConstraintError(err) {
		return nil
REDACTED
	return err
REDACTED

func generateHexSecret(byteLength int) (string, error) {
	if byteLength <= 0 {
		byteLength = 32
REDACTED
	buf := make([]byte, byteLength)
	if _, err := readRandomBytes(buf); err != nil {
		return "", fmt.Errorf("generate random secret: %w", err)
REDACTED
	return hex.EncodeToString(buf), nil
REDACTED
