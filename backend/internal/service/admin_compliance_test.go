package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type adminComplianceRepoStub struct {
	values map[string]string
REDACTED

func (r *adminComplianceRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	if value, ok := r.values[key]; ok {
		return &Setting{Key: key, Value: valueREDACTED, nil
REDACTED
	return nil, ErrSettingNotFound
REDACTED

func (r *adminComplianceRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	setting, err := r.Get(ctx, key)
	if err != nil {
		return "", err
REDACTED
	return setting.Value, nil
REDACTED

func (r *adminComplianceRepoStub) Set(ctx context.Context, key, value string) error {
	if r.values == nil {
		r.values = map[string]string{REDACTED
REDACTED
	r.values[key] = value
	return nil
REDACTED

func (r *adminComplianceRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	return map[string]string{REDACTED, nil
REDACTED

func (r *adminComplianceRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	return nil
REDACTED

func (r *adminComplianceRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	return map[string]string{REDACTED, nil
REDACTED

func (r *adminComplianceRepoStub) Delete(ctx context.Context, key string) error {
	delete(r.values, key)
	return nil
REDACTED

func TestAdminComplianceStatusRequiresAckWhenMissing(t *testing.T) {
	svc := NewSettingService(&adminComplianceRepoStub{REDACTED, &config.Config{REDACTED)

	status, err := svc.GetAdminComplianceStatus(context.Background(), 1)
REDACTED
	require.True(t, status.Required)
	require.Equal(t, AdminComplianceVersion, status.Version)
	require.Equal(t, AdminComplianceAckPhraseZH, status.AckPhraseZH)
	require.Equal(t, AdminComplianceDocumentPathZH, status.DocumentPathZH)
REDACTED

func TestAcceptAdminComplianceRejectsWrongPhrase(t *testing.T) {
	svc := NewSettingService(&adminComplianceRepoStub{REDACTED, &config.Config{REDACTED)

	_, err := svc.AcceptAdminCompliance(context.Background(), AdminComplianceAcceptInput{
		AdminUserID: 1,
		Language:    "zh",
		Phrase:      "我同意",
REDACTED)
REDACTED
	require.True(t, errors.Is(err, ErrAdminComplianceInvalidPhrase))
REDACTED

func TestAcceptAdminCompliancePersistsCurrentVersion(t *testing.T) {
	repo := &adminComplianceRepoStub{REDACTED
	svc := NewSettingService(repo, &config.Config{REDACTED)

	status, err := svc.AcceptAdminCompliance(context.Background(), AdminComplianceAcceptInput{
		AdminUserID: 42,
		Language:    "zh-CN",
		Phrase:      AdminComplianceAckPhraseZH,
		IPAddress:   "203.0.113.10",
		UserAgent:   "test-agent",
REDACTED)
REDACTED
	require.False(t, status.Required)
	require.NotNil(t, status.Acknowledgement)
	require.Equal(t, int64(42), status.Acknowledgement.AdminUserID)
	require.Equal(t, "203.0.113.10", status.Acknowledgement.IPAddress)

	var stored AdminComplianceAcknowledgement
	require.NoError(t, json.Unmarshal([]byte(repo.values[adminComplianceAcknowledgementKey(42)]), &stored))
	require.Equal(t, AdminComplianceVersion, stored.Version)
	require.Equal(t, AdminComplianceDocumentPathZH, stored.DocumentZH)
REDACTED

func TestAdminComplianceStatusRequiresAckOnOldVersion(t *testing.T) {
	old, err := json.Marshal(AdminComplianceAcknowledgement{Version: "v2026.01.01"REDACTED)
REDACTED
	svc := NewSettingService(&adminComplianceRepoStub{
		values: map[string]string{adminComplianceAcknowledgementKey(1): string(old)REDACTED,
REDACTED, &config.Config{REDACTED)

	status, err := svc.GetAdminComplianceStatus(context.Background(), 1)
REDACTED
	require.True(t, status.Required)
	require.Nil(t, status.Acknowledgement)
REDACTED

func TestAdminComplianceStatusIsPerAdminUser(t *testing.T) {
	current, err := json.Marshal(AdminComplianceAcknowledgement{
		Version:     AdminComplianceVersion,
		AdminUserID: 1,
REDACTED)
REDACTED
	svc := NewSettingService(&adminComplianceRepoStub{
		values: map[string]string{adminComplianceAcknowledgementKey(1): string(current)REDACTED,
REDACTED, &config.Config{REDACTED)

	statusForUserOne, err := svc.GetAdminComplianceStatus(context.Background(), 1)
REDACTED
	require.False(t, statusForUserOne.Required)

	statusForUserTwo, err := svc.GetAdminComplianceStatus(context.Background(), 2)
REDACTED
	require.True(t, statusForUserTwo.Required)
REDACTED
