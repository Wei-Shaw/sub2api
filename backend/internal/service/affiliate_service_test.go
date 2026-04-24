//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type affiliateSettingRepoStub struct {
	value string
	err   error
REDACTED

func (s *affiliateSettingRepoStub) Get(context.Context, string) (*Setting, error) { return nil, s.err REDACTED
func (s *affiliateSettingRepoStub) GetValue(context.Context, string) (string, error) {
	if s.err != nil {
		return "", s.err
REDACTED
	return s.value, nil
REDACTED
func (s *affiliateSettingRepoStub) Set(context.Context, string, string) error { return s.err REDACTED
func (s *affiliateSettingRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	if s.err != nil {
		return nil, s.err
REDACTED
	return map[string]string{REDACTED, nil
REDACTED
func (s *affiliateSettingRepoStub) SetMultiple(context.Context, map[string]string) error {
	return s.err
REDACTED
func (s *affiliateSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	if s.err != nil {
		return nil, s.err
REDACTED
	return map[string]string{REDACTED, nil
REDACTED
func (s *affiliateSettingRepoStub) Delete(context.Context, string) error { return s.err REDACTED

func TestAffiliateRebateRatePercentSemantics(t *testing.T) {
	t.Parallel()

	svc := &AffiliateService{settingRepo: &affiliateSettingRepoStub{value: "1"REDACTEDREDACTED
	rate := svc.loadAffiliateRebateRatePercent(context.Background())
	require.Equal(t, 1.0, rate)

	svc.settingRepo = &affiliateSettingRepoStub{value: "0.2"REDACTED
	rate = svc.loadAffiliateRebateRatePercent(context.Background())
	require.Equal(t, 0.2, rate)
REDACTED

func TestMaskEmail(t *testing.T) {
	t.Parallel()
	require.Equal(t, "a***@g***.com", maskEmail("alice@gmail.com"))
	require.Equal(t, "x***@d***", maskEmail("x@domain"))
	require.Equal(t, "", maskEmail(""))
REDACTED
