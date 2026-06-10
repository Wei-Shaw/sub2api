package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	AdminComplianceVersion        = "v2026.06.10"
	AdminComplianceDocumentPathZH = "docs/legal/admin-compliance.zh.md"
	AdminComplianceDocumentPathEN = "docs/legal/admin-compliance.en.md"
	AdminComplianceDocumentURLZH  = "https://github.com/Wei-Shaw/sub2api/blob/main/docs/legal/admin-compliance.zh.md"
	AdminComplianceDocumentURLEN  = "https://github.com/Wei-Shaw/sub2api/blob/main/docs/legal/admin-compliance.en.md"
	AdminComplianceAckPhraseZH    = "我已阅读、理解并同意 Sub2API 部署与运营合规承诺"
	AdminComplianceAckPhraseEN    = "I have read, understood, and agree to the Sub2API Deployment and Operation Compliance Commitment"

	settingKeyAdminComplianceAcknowledgement = "admin_compliance_acknowledgement"
)

var (
	ErrAdminComplianceAcknowledgementRequired = infraerrors.New(
		http.StatusLocked,
		"ADMIN_COMPLIANCE_ACK_REQUIRED",
		"administrator compliance acknowledgement is required",
	)
	ErrAdminComplianceInvalidPhrase = infraerrors.BadRequest(
		"ADMIN_COMPLIANCE_INVALID_PHRASE",
		"confirmation phrase does not match",
	)
)

type AdminComplianceAcknowledgement struct {
	Version     string    `json:"version"`
	DocumentZH  string    `json:"document_zh"`
	DocumentEN  string    `json:"document_en"`
	AdminUserID int64     `json:"admin_user_id"`
	IPAddress   string    `json:"ip_address,omitempty"`
	UserAgent   string    `json:"user_agent,omitempty"`
	AcceptedAt  time.Time `json:"accepted_at"`
REDACTED

type AdminComplianceStatus struct {
	Required        bool                            `json:"required"`
	Version         string                          `json:"version"`
	DocumentPathZH  string                          `json:"document_path_zh"`
	DocumentPathEN  string                          `json:"document_path_en"`
	DocumentURLZH   string                          `json:"document_url_zh"`
	DocumentURLEN   string                          `json:"document_url_en"`
	AckPhraseZH     string                          `json:"ack_phrase_zh"`
	AckPhraseEN     string                          `json:"ack_phrase_en"`
	Acknowledgement *AdminComplianceAcknowledgement `json:"acknowledgement,omitempty"`
REDACTED

type AdminComplianceAcceptInput struct {
	AdminUserID int64
	Phrase      string
	Language    string
	IPAddress   string
	UserAgent   string
REDACTED

func normalizeAdminComplianceLanguage(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if strings.HasPrefix(raw, "zh") {
		return "zh"
REDACTED
	return "en"
REDACTED

func expectedAdminCompliancePhrase(language string) string {
	if normalizeAdminComplianceLanguage(language) == "zh" {
		return AdminComplianceAckPhraseZH
REDACTED
	return AdminComplianceAckPhraseEN
REDACTED

func adminComplianceAcknowledgementKey(adminUserID int64) string {
	if adminUserID <= 0 {
		return settingKeyAdminComplianceAcknowledgement
REDACTED
	return settingKeyAdminComplianceAcknowledgement + ":" + strconv.FormatInt(adminUserID, 10)
REDACTED

func (s *SettingService) GetAdminComplianceStatus(ctx context.Context, adminUserID int64) (*AdminComplianceStatus, error) {
	status := &AdminComplianceStatus{
		Required:       true,
		Version:        AdminComplianceVersion,
		DocumentPathZH: AdminComplianceDocumentPathZH,
		DocumentPathEN: AdminComplianceDocumentPathEN,
		DocumentURLZH:  AdminComplianceDocumentURLZH,
		DocumentURLEN:  AdminComplianceDocumentURLEN,
		AckPhraseZH:    AdminComplianceAckPhraseZH,
		AckPhraseEN:    AdminComplianceAckPhraseEN,
REDACTED
	if s == nil || s.settingRepo == nil {
		return status, nil
REDACTED

	raw, err := s.settingRepo.GetValue(ctx, adminComplianceAcknowledgementKey(adminUserID))
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return status, nil
	REDACTED
		return nil, fmt.Errorf("get admin compliance acknowledgement: %w", err)
REDACTED

	var ack AdminComplianceAcknowledgement
	if err := json.Unmarshal([]byte(raw), &ack); err != nil {
		return status, nil
REDACTED
	if ack.Version == AdminComplianceVersion {
		status.Required = false
		status.Acknowledgement = &ack
REDACTED
	return status, nil
REDACTED

func (s *SettingService) IsAdminComplianceAcknowledged(ctx context.Context, adminUserID int64) (bool, error) {
	status, err := s.GetAdminComplianceStatus(ctx, adminUserID)
	if err != nil {
		return false, err
REDACTED
	return status != nil && !status.Required, nil
REDACTED

func (s *SettingService) AcceptAdminCompliance(ctx context.Context, input AdminComplianceAcceptInput) (*AdminComplianceStatus, error) {
	if s == nil || s.settingRepo == nil {
		return nil, infraerrors.InternalServer("SETTING_SERVICE_UNAVAILABLE", "setting service is unavailable")
REDACTED
	phrase := strings.TrimSpace(input.Phrase)
	if phrase != expectedAdminCompliancePhrase(input.Language) {
		return nil, ErrAdminComplianceInvalidPhrase
REDACTED

	ack := AdminComplianceAcknowledgement{
		Version:     AdminComplianceVersion,
		DocumentZH:  AdminComplianceDocumentPathZH,
		DocumentEN:  AdminComplianceDocumentPathEN,
		AdminUserID: input.AdminUserID,
		IPAddress:   strings.TrimSpace(input.IPAddress),
		UserAgent:   strings.TrimSpace(input.UserAgent),
		AcceptedAt:  time.Now().UTC(),
REDACTED
	payload, err := json.Marshal(ack)
	if err != nil {
		return nil, fmt.Errorf("marshal admin compliance acknowledgement: %w", err)
REDACTED
	if err := s.settingRepo.Set(ctx, adminComplianceAcknowledgementKey(input.AdminUserID), string(payload)); err != nil {
		return nil, fmt.Errorf("save admin compliance acknowledgement: %w", err)
REDACTED

	return s.GetAdminComplianceStatus(ctx, input.AdminUserID)
REDACTED
