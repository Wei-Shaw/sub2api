//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeEmailForAliasDedup(t *testing.T) {
	cases := []struct {
		name  string
		email string
		want  string
REDACTED{
		{"plain", "user@example.com", "user@example.com"REDACTED,
		{"uppercase and spaces", "  User@Example.COM ", "user@example.com"REDACTED,
		{"plus alias stripped", "user+tag@example.com", "user@example.com"REDACTED,
		{"gmail plus alias", "someone+bulk294@gmail.com", "someone@gmail.com"REDACTED,
		{"gmail dots removed", "some.one@gmail.com", "someone@gmail.com"REDACTED,
		{"gmail dots and plus", "s.o.m.e+x@gmail.com", "some@gmail.com"REDACTED,
		{"googlemail folded to gmail", "user@googlemail.com", "user@gmail.com"REDACTED,
		{"non-gmail keeps dots", "first.last@qq.com", "first.last@qq.com"REDACTED,
		{"fqdn root dot dropped", "d.axis.2026@gmail.com.", "daxis2026@gmail.com"REDACTED,
		{"fqdn root dot on other domain", "first.last@qq.com.", "first.last@qq.com"REDACTED,
		{"leading plus keeps local part", "+alice@gmail.com", "+alice@gmail.com"REDACTED,
		{"dot-only local part kept", "...@gmail.com", "...@gmail.com"REDACTED,
		{"invalid keeps lowered raw", "not-an-email", "not-an-email"REDACTED,
REDACTED
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, NormalizeEmailForAliasDedup(tc.email))
	REDACTED)
REDACTED
REDACTED

func TestNormalizeEmailForAliasDedupKeepsDistinctInboxes(t *testing.T) {
	// 剥离 "+后缀" 不能把同域下不同用户折叠成同一身份。
	require.NotEqual(t,
		NormalizeEmailForAliasDedup("+alice@gmail.com"),
		NormalizeEmailForAliasDedup("+bob@gmail.com"),
	)
	require.NotEqual(t,
		NormalizeEmailForAliasDedup("alice@gmail.com"),
		NormalizeEmailForAliasDedup("bob@gmail.com"),
	)
REDACTED

func TestEmailAliasDedupProbes(t *testing.T) {
	require.ElementsMatch(t,
		[]EmailAliasProbe{{Local: "someone", Domain: "gmailcom"REDACTED, {Local: "someone", Domain: "googlemailcom"REDACTEDREDACTED,
		EmailAliasDedupProbes("Some.One+tag@gmail.com"),
	)
	require.ElementsMatch(t,
		[]EmailAliasProbe{{Local: "daxis2026", Domain: "gmailcom"REDACTED, {Local: "daxis2026", Domain: "googlemailcom"REDACTEDREDACTED,
		EmailAliasDedupProbes("d.axis.2026@googlemail.com."),
	)
	require.Equal(t,
		[]EmailAliasProbe{{Local: "firstlast", Domain: "qqcom"REDACTEDREDACTED,
		EmailAliasDedupProbes("first.last+tag@qq.com"),
	)
	require.Nil(t, EmailAliasDedupProbes("not-an-email"))
	require.Nil(t, EmailAliasDedupProbes("...@gmail.com"))
REDACTED

// aliasDedupRepoStub implements only the methods alias dedup uses; other
// UserRepository methods come from the embedded nil interface (a wrong call
// would panic, failing the test).
type aliasDedupRepoStub struct {
	UserRepository
	exists      bool
	existsErr   error
	stored      []string
	aliasErr    error
	aliasChecks []string
REDACTED

func (s *aliasDedupRepoStub) ExistsByEmail(context.Context, string) (bool, error) {
	return s.exists, s.existsErr
REDACTED

func (s *aliasDedupRepoStub) ExistsByEmailAlias(_ context.Context, email string) (bool, error) {
	s.aliasChecks = append(s.aliasChecks, email)
	if s.aliasErr != nil {
		return false, s.aliasErr
REDACTED
	identity := NormalizeEmailForAliasDedup(email)
	for _, candidate := range s.stored {
		if NormalizeEmailForAliasDedup(candidate) == identity {
			return true, nil
	REDACTED
REDACTED
	return false, nil
REDACTED

func TestExistsByEmailOrAlias(t *testing.T) {
	ctx := context.Background()

	t.Run("exact duplicate short-circuits", func(t *testing.T) {
		repo := &aliasDedupRepoStub{exists: trueREDACTED
		svc := &AuthService{userRepo: repoREDACTED
		got, err := svc.existsByEmailOrAlias(ctx, "user@gmail.com")
	REDACTED
		require.True(t, got)
		require.Empty(t, repo.aliasChecks, "no alias probe expected after exact hit")
REDACTED)

	t.Run("plus alias variant detected", func(t *testing.T) {
		repo := &aliasDedupRepoStub{stored: []string{"someone+bulk294@gmail.com"REDACTEDREDACTED
		svc := &AuthService{userRepo: repoREDACTED
		got, err := svc.existsByEmailOrAlias(ctx, "Someone@gmail.com")
	REDACTED
		require.True(t, got)
REDACTED)

	t.Run("gmail dot variant detected", func(t *testing.T) {
		repo := &aliasDedupRepoStub{stored: []string{"some.one@gmail.com"REDACTEDREDACTED
		svc := &AuthService{userRepo: repoREDACTED
		got, err := svc.existsByEmailOrAlias(ctx, "someone@gmail.com")
	REDACTED
		require.True(t, got)
REDACTED)

	t.Run("fqdn root dot variant detected", func(t *testing.T) {
		repo := &aliasDedupRepoStub{stored: []string{"d.axis.2026@gmail.com"REDACTEDREDACTED
		svc := &AuthService{userRepo: repoREDACTED
		got, err := svc.existsByEmailOrAlias(ctx, "da.xis.2026@gmail.com.")
	REDACTED
		require.True(t, got)
REDACTED)

	t.Run("different inbox allowed", func(t *testing.T) {
		repo := &aliasDedupRepoStub{stored: []string{"other@gmail.com"REDACTEDREDACTED
		svc := &AuthService{userRepo: repoREDACTED
		got, err := svc.existsByEmailOrAlias(ctx, "user@gmail.com")
	REDACTED
		require.False(t, got)
REDACTED)

	t.Run("distinct plus-prefixed locals allowed", func(t *testing.T) {
		repo := &aliasDedupRepoStub{stored: []string{"+alice@gmail.com"REDACTEDREDACTED
		svc := &AuthService{userRepo: repoREDACTED
		got, err := svc.existsByEmailOrAlias(ctx, "+bob@gmail.com")
	REDACTED
		require.False(t, got)
REDACTED)

	t.Run("alias probe error fails closed", func(t *testing.T) {
		repo := &aliasDedupRepoStub{aliasErr: errors.New("db down")REDACTED
		svc := &AuthService{userRepo: repoREDACTED
		_, err := svc.existsByEmailOrAlias(ctx, "user@gmail.com")
	REDACTED
REDACTED)

	t.Run("exact check error propagates", func(t *testing.T) {
		repo := &aliasDedupRepoStub{existsErr: errors.New("db down")REDACTED
		svc := &AuthService{userRepo: repoREDACTED
		_, err := svc.existsByEmailOrAlias(ctx, "user@gmail.com")
	REDACTED
REDACTED)
REDACTED
