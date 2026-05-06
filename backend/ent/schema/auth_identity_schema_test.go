package schema

import (
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/entc/load"
	"entgo.io/ent/schema/field"
	"github.com/stretchr/testify/require"
)

func TestAuthIdentityFoundationSchemas(t *testing.T) {
	spec, err := (&load.Config{Path: "."REDACTED).Load()
REDACTED

	schemas := map[string]*load.Schema{REDACTED
	for _, schema := range spec.Schemas {
		schemas[schema.Name] = schema
REDACTED

	authIdentity := requireSchema(t, schemas, "AuthIdentity")
	requireSchemaFields(t, authIdentity,
		"user_id",
		"provider_type",
		"provider_key",
		"provider_subject",
		"verified_at",
		"issuer",
		"metadata",
	)
	requireHasUniqueIndex(t, authIdentity, "provider_type", "provider_key", "provider_subject")

	authIdentityChannel := requireSchema(t, schemas, "AuthIdentityChannel")
	requireSchemaFields(t, authIdentityChannel,
		"identity_id",
		"provider_type",
		"provider_key",
		"channel",
		"channel_app_id",
		"channel_subject",
		"metadata",
	)
	requireHasUniqueIndex(t, authIdentityChannel, "provider_type", "provider_key", "channel", "channel_app_id", "channel_subject")

	pendingAuthSession := requireSchema(t, schemas, "PendingAuthSession")
	requireSchemaFields(t, pendingAuthSession,
		"intent",
		"provider_type",
		"provider_key",
		"provider_subject",
		"target_user_id",
		"redirect_to",
		"resolved_email",
		"registration_password_hash",
		"upstream_identity_claims",
		"local_flow_state",
		"browser_session_key",
		"completion_code_hash",
		"completion_code_expires_at",
		"email_verified_at",
		"password_verified_at",
		"totp_verified_at",
		"expires_at",
		"consumed_at",
	)

	adoptionDecision := requireSchema(t, schemas, "IdentityAdoptionDecision")
	requireSchemaFields(t, adoptionDecision,
		"pending_auth_session_id",
		"identity_id",
		"adopt_display_name",
		"adopt_avatar",
		"decided_at",
	)
	requireHasUniqueIndex(t, adoptionDecision, "pending_auth_session_id")

	userSchema := requireSchema(t, schemas, "User")
	requireSchemaFields(t, userSchema, "signup_source", "last_login_at", "last_active_at")
	signupSource := requireSchemaField(t, userSchema, "signup_source")
	require.Equal(t, field.TypeString, signupSource.Info.Type)
	require.True(t, signupSource.Default)
	require.Equal(t, "email", signupSource.DefaultValue)
	require.Equal(t, 1, signupSource.Validators)

	validator := requireStringFieldValidator(t, User{REDACTED.Fields(), "signup_source")
	for _, value := range []string{"email", "linuxdo", "wechat", "oidc", "github", "google"REDACTED {
		require.NoError(t, validator(value))
REDACTED
	require.Error(t, validator("unknown"))
REDACTED

func requireSchema(t *testing.T, schemas map[string]*load.Schema, name string) *load.Schema {
REDACTED

	schema, ok := schemas[name]
	require.True(t, ok, "schema %s should exist", name)
	return schema
REDACTED

func requireSchemaFields(t *testing.T, schema *load.Schema, names ...string) {
REDACTED

	fields := map[string]struct{REDACTED{REDACTED
	for _, field := range schema.Fields {
		fields[field.Name] = struct{REDACTED{REDACTED
REDACTED

	for _, name := range names {
		_, ok := fields[name]
		require.True(t, ok, "schema %s should include field %s", schema.Name, name)
REDACTED
REDACTED

func requireSchemaField(t *testing.T, schema *load.Schema, name string) *load.Field {
REDACTED

	for _, schemaField := range schema.Fields {
		if schemaField.Name == name {
			return schemaField
	REDACTED
REDACTED

	require.Failf(t, "missing schema field", "schema %s should include field %s", schema.Name, name)
	return nil
REDACTED

func requireStringFieldValidator(t *testing.T, fields []ent.Field, name string) func(string) error {
REDACTED

	for _, entField := range fields {
		descriptor := entField.Descriptor()
		if descriptor.Name != name {
			continue
	REDACTED
		require.NotEmpty(t, descriptor.Validators, "field %s should include a validator", name)
		validator, ok := descriptor.Validators[0].(func(string) error)
		require.True(t, ok, "field %s validator should be func(string) error", name)
		return validator
REDACTED

	require.Failf(t, "missing field validator", "schema should include field %s", name)
	return nil
REDACTED

func requireHasUniqueIndex(t *testing.T, schema *load.Schema, fields ...string) {
REDACTED

	for _, index := range schema.Indexes {
		if !index.Unique {
			continue
	REDACTED
		if len(index.Fields) != len(fields) {
			continue
	REDACTED
		match := true
		for i := range fields {
			if index.Fields[i] != fields[i] {
				match = false
				break
		REDACTED
	REDACTED
		if match {
			return
	REDACTED
REDACTED

	require.Failf(t, "missing unique index", "schema %s should include unique index on %v", schema.Name, fields)
REDACTED
