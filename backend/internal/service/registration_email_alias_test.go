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
		{"invalid keeps lowered raw", "not-an-email", "not-an-email"REDACTED,
REDACTED
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, NormalizeEmailForAliasDedup(tc.email))
	REDACTED)
REDACTED
REDACTED

func TestAliasDedupCandidateDomains(t *testing.T) {
	require.ElementsMatch(t, []string{"gmail.com", "googlemail.com"REDACTED, aliasDedupCandidateDomains("user@gmail.com"))
	require.ElementsMatch(t, []string{"gmail.com", "googlemail.com"REDACTED, aliasDedupCandidateDomains("user@googlemail.com"))
	require.Equal(t, []string{"qq.com"REDACTED, aliasDedupCandidateDomains("user@qq.com"))
	require.Nil(t, aliasDedupCandidateDomains("not-an-email"))
REDACTED

// aliasDedupRepoStub implements only the methods alias dedup uses; other
// UserRepository methods come from the embedded nil interface (a wrong call
// would panic, failing the test).
type aliasDedupRepoStub struct {
	UserRepository
	exists    bool
	existsErr error
	emails    []string
	listErr   error
	scanned   [][]string
REDACTED

func (s *aliasDedupRepoStub) ExistsByEmail(context.Context, string) (bool, error) {
	return s.exists, s.existsErr
REDACTED

func (s *aliasDedupRepoStub) ListEmailsByDomains(_ context.Context, domains []string) ([]string, error) {
	s.scanned = append(s.scanned, domains)
	return s.emails, s.listErr
REDACTED

// exactOnlyRepoStub only supports the exact check (no alias-lookup capability).
type exactOnlyRepoStub struct {
	UserRepository
	exists bool
REDACTED

func (s *exactOnlyRepoStub) ExistsByEmail(context.Context, string) (bool, error) {
	return s.exists, nil
REDACTED

func TestExistsByEmailOrAlias(t *testing.T) {
	ctx := context.Background()

	t.Run("exact duplicate short-circuits", func(t *testing.T) {
		repo := &aliasDedupRepoStub{exists: trueREDACTED
		svc := &AuthService{userRepo: repoREDACTED
		got, err := svc.existsByEmailOrAlias(ctx, "user@gmail.com")
	REDACTED
		require.True(t, got)
		require.Empty(t, repo.scanned, "no alias scan expected after exact hit")
REDACTED)

	t.Run("plus alias variant detected", func(t *testing.T) {
		repo := &aliasDedupRepoStub{emails: []string{"someone+bulk294@gmail.com"REDACTEDREDACTED
		svc := &AuthService{userRepo: repoREDACTED
		got, err := svc.existsByEmailOrAlias(ctx, "Someone@gmail.com")
	REDACTED
		require.True(t, got)
REDACTED)

	t.Run("gmail dot variant detected", func(t *testing.T) {
		repo := &aliasDedupRepoStub{emails: []string{"some.one@gmail.com"REDACTEDREDACTED
		svc := &AuthService{userRepo: repoREDACTED
		got, err := svc.existsByEmailOrAlias(ctx, "someone@gmail.com")
	REDACTED
		require.True(t, got)
REDACTED)

	t.Run("gmail scans both gmail-family domains", func(t *testing.T) {
		repo := &aliasDedupRepoStub{REDACTED
		svc := &AuthService{userRepo: repoREDACTED
		_, err := svc.existsByEmailOrAlias(ctx, "user@googlemail.com")
	REDACTED
		require.Len(t, repo.scanned, 1)
		require.ElementsMatch(t, []string{"gmail.com", "googlemail.com"REDACTED, repo.scanned[0])
REDACTED)

	t.Run("different inbox allowed", func(t *testing.T) {
		repo := &aliasDedupRepoStub{emails: []string{"other@gmail.com"REDACTEDREDACTED
		svc := &AuthService{userRepo: repoREDACTED
		got, err := svc.existsByEmailOrAlias(ctx, "user@gmail.com")
	REDACTED
		require.False(t, got)
REDACTED)

	t.Run("list error fails closed", func(t *testing.T) {
		repo := &aliasDedupRepoStub{listErr: errors.New("db down")REDACTED
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

	t.Run("repo without capability falls back to exact check", func(t *testing.T) {
		svc := &AuthService{userRepo: &exactOnlyRepoStub{exists: falseREDACTEDREDACTED
		got, err := svc.existsByEmailOrAlias(ctx, "user@gmail.com")
	REDACTED
		require.False(t, got)
REDACTED)
REDACTED
