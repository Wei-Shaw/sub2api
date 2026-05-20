package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	NotificationEmailEventSubscriptionPurchaseSuccess = "subscription.purchase_success"
	NotificationEmailEventSubscriptionExpiryReminder  = "subscription.expiry_reminder"
	NotificationEmailEventBalanceLow                  = "balance.low"
	NotificationEmailEventBalanceRechargeSuccess      = "balance.recharge_success"

	notificationEmailTemplateKeyPrefix    = "notification_email_template:"
	notificationEmailPreferenceKeyPrefix  = "notification_email_preference:"
	notificationEmailDeliveryKeyPrefix    = "notification_email_delivery:"
	notificationEmailLocaleUserKeyPrefix  = "notification_email_locale:user:"
	notificationEmailLocaleEmailKeyPrefix = "notification_email_locale:email:"
	notificationEmailUnsubscribeSecretKey = "notification_email_unsubscribe_secret"
	notificationEmailDefaultLocale        = "en"
	notificationEmailLocaleChinese        = "zh"
	notificationEmailMaxSubjectLength     = 200
	notificationEmailMaxHTMLLength        = 30000
	notificationEmailUnsubscribeTTL       = 365 * 24 * time.Hour
)

var (
	notificationEmailPlaceholderPattern = regexp.MustCompile(`{{\s*([a-zA-Z][a-zA-Z0-9_]*)\s*REDACTEDREDACTED`)
	notificationEmailLocales            = []string{notificationEmailDefaultLocale, notificationEmailLocaleChineseREDACTED
	notificationEmailCommonPlaceholders = []string{"site_name", "recipient_name", "recipient_email"REDACTED
)

type NotificationEmailService struct {
	settingRepo  SettingRepository
	emailService *EmailService
REDACTED

type NotificationEmailEventInfo struct {
	Event        string   `json:"event"`
	Label        string   `json:"label"`
	Description  string   `json:"description"`
	Category     string   `json:"category"`
	Optional     bool     `json:"optional"`
	Placeholders []string `json:"placeholders"`
REDACTED

type NotificationEmailTemplate struct {
	Event        string     `json:"event"`
	Locale       string     `json:"locale"`
	Subject      string     `json:"subject"`
	HTML         string     `json:"html"`
	IsCustom     bool       `json:"is_custom"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	Placeholders []string   `json:"placeholders"`
REDACTED

type NotificationEmailPreview struct {
	Subject string `json:"subject"`
	HTML    string `json:"html"`
REDACTED

type NotificationEmailPreviewInput struct {
	Event     string            `json:"event"`
	Locale    string            `json:"locale"`
	Subject   string            `json:"subject"`
	HTML      string            `json:"html"`
	Variables map[string]string `json:"variables,omitempty"`
REDACTED

type NotificationEmailSendInput struct {
	Event          string
	Locale         string
	RecipientEmail string
	RecipientName  string
	UserID         int64
	SourceType     string
	SourceID       string
	ReminderKey    string
	Variables      map[string]string
REDACTED

type NotificationEmailUnsubscribeResult struct {
	Event string `json:"event"`
	Email string `json:"email"`
	Done  bool   `json:"done"`
REDACTED

type notificationEmailStoredTemplate struct {
	Subject   string    `json:"subject"`
	HTML      string    `json:"html"`
	UpdatedAt time.Time `json:"updated_at"`
REDACTED

type notificationEmailOfficialTemplate struct {
	Subject string
	HTML    string
REDACTED

type notificationEmailUnsubscribeClaims struct {
	Email string `json:"email"`
	Event string `json:"event"`
	Exp   int64  `json:"exp"`
REDACTED

func NewNotificationEmailService(settingRepo SettingRepository, emailService *EmailService) *NotificationEmailService {
	return &NotificationEmailService{settingRepo: settingRepo, emailService: emailServiceREDACTED
REDACTED

func (s *NotificationEmailService) ListEventInfos() []NotificationEmailEventInfo {
	infos := make([]NotificationEmailEventInfo, 0, len(notificationEmailEventDefinitions))
	for _, event := range notificationEmailEventOrder {
		info := notificationEmailEventDefinitions[event]
		info.Placeholders = append([]string(nil), info.Placeholders...)
		infos = append(infos, info)
REDACTED
	return infos
REDACTED

func (s *NotificationEmailService) SupportedLocales() []string {
	return append([]string(nil), notificationEmailLocales...)
REDACTED

func (s *NotificationEmailService) ListTemplates(ctx context.Context) ([]NotificationEmailTemplate, error) {
	items := make([]NotificationEmailTemplate, 0, len(notificationEmailEventOrder)*len(notificationEmailLocales))
	for _, event := range notificationEmailEventOrder {
		for _, locale := range notificationEmailLocales {
			tmpl, err := s.GetTemplate(ctx, event, locale)
			if err != nil {
				return nil, err
		REDACTED
			items = append(items, tmpl)
	REDACTED
REDACTED
	return items, nil
REDACTED

func (s *NotificationEmailService) GetTemplate(ctx context.Context, event, locale string) (NotificationEmailTemplate, error) {
	info, normalizedEvent, err := s.eventInfo(event)
	if err != nil {
		return NotificationEmailTemplate{REDACTED, err
REDACTED
	normalizedLocale := normalizeNotificationLocale(locale)
	official, ok := notificationEmailOfficialTemplates[normalizedEvent][normalizedLocale]
	if !ok {
		return NotificationEmailTemplate{REDACTED, fmt.Errorf("official template not found for %s/%s", normalizedEvent, normalizedLocale)
REDACTED

	tmpl := NotificationEmailTemplate{
		Event:        normalizedEvent,
		Locale:       normalizedLocale,
		Subject:      official.Subject,
		HTML:         official.HTML,
		Placeholders: append([]string(nil), info.Placeholders...),
REDACTED

	raw, err := s.settingRepo.GetValue(ctx, notificationEmailTemplateKey(normalizedEvent, normalizedLocale))
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return tmpl, nil
	REDACTED
		return NotificationEmailTemplate{REDACTED, err
REDACTED
	if strings.TrimSpace(raw) == "" {
		return tmpl, nil
REDACTED

	var stored notificationEmailStoredTemplate
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return NotificationEmailTemplate{REDACTED, fmt.Errorf("decode email template override: %w", err)
REDACTED
	if err := validateNotificationEmailTemplate(normalizedEvent, stored.Subject, stored.HTML); err != nil {
		return NotificationEmailTemplate{REDACTED, err
REDACTED
	tmpl.Subject = stored.Subject
	tmpl.HTML = stored.HTML
	tmpl.IsCustom = true
	updatedAt := stored.UpdatedAt
	tmpl.UpdatedAt = &updatedAt
	return tmpl, nil
REDACTED

func (s *NotificationEmailService) UpdateTemplate(ctx context.Context, event, locale, subject, htmlBody string) (NotificationEmailTemplate, error) {
	_, normalizedEvent, err := s.eventInfo(event)
	if err != nil {
		return NotificationEmailTemplate{REDACTED, err
REDACTED
	normalizedLocale := normalizeNotificationLocale(locale)
	if err := validateNotificationEmailTemplate(normalizedEvent, subject, htmlBody); err != nil {
		return NotificationEmailTemplate{REDACTED, err
REDACTED
	stored := notificationEmailStoredTemplate{
		Subject:   strings.TrimSpace(subject),
		HTML:      htmlBody,
		UpdatedAt: time.Now().UTC(),
REDACTED
	payload, err := json.Marshal(stored)
	if err != nil {
		return NotificationEmailTemplate{REDACTED, err
REDACTED
	if err := s.settingRepo.Set(ctx, notificationEmailTemplateKey(normalizedEvent, normalizedLocale), string(payload)); err != nil {
		return NotificationEmailTemplate{REDACTED, err
REDACTED
	return s.GetTemplate(ctx, normalizedEvent, normalizedLocale)
REDACTED

func (s *NotificationEmailService) RestoreOfficialTemplate(ctx context.Context, event, locale string) (NotificationEmailTemplate, error) {
	_, normalizedEvent, err := s.eventInfo(event)
	if err != nil {
		return NotificationEmailTemplate{REDACTED, err
REDACTED
	normalizedLocale := normalizeNotificationLocale(locale)
	if err := s.settingRepo.Delete(ctx, notificationEmailTemplateKey(normalizedEvent, normalizedLocale)); err != nil && !errors.Is(err, ErrSettingNotFound) {
		return NotificationEmailTemplate{REDACTED, err
REDACTED
	return s.GetTemplate(ctx, normalizedEvent, normalizedLocale)
REDACTED

func (s *NotificationEmailService) PreviewTemplate(ctx context.Context, input NotificationEmailPreviewInput) (NotificationEmailPreview, error) {
	_, normalizedEvent, err := s.eventInfo(input.Event)
	if err != nil {
		return NotificationEmailPreview{REDACTED, err
REDACTED
	normalizedLocale := normalizeNotificationLocale(input.Locale)
	subject := input.Subject
	htmlBody := input.HTML
	if strings.TrimSpace(subject) == "" || strings.TrimSpace(htmlBody) == "" {
		tmpl, err := s.GetTemplate(ctx, normalizedEvent, normalizedLocale)
		if err != nil {
			return NotificationEmailPreview{REDACTED, err
	REDACTED
		if strings.TrimSpace(subject) == "" {
			subject = tmpl.Subject
	REDACTED
		if strings.TrimSpace(htmlBody) == "" {
			htmlBody = tmpl.HTML
	REDACTED
REDACTED
	if err := validateNotificationEmailTemplate(normalizedEvent, subject, htmlBody); err != nil {
		return NotificationEmailPreview{REDACTED, err
REDACTED
	variables := s.sampleVariables(ctx, normalizedEvent, normalizedLocale)
	for key, value := range input.Variables {
		variables[key] = value
REDACTED
	return renderNotificationEmail(normalizedEvent, subject, htmlBody, variables)
REDACTED

func (s *NotificationEmailService) Send(ctx context.Context, input NotificationEmailSendInput) error {
	info, normalizedEvent, err := s.eventInfo(input.Event)
	if err != nil {
		return err
REDACTED
	recipient := strings.TrimSpace(input.RecipientEmail)
	if recipient == "" {
		return nil
REDACTED
	if info.Optional {
		unsubscribed, err := s.IsUnsubscribed(ctx, recipient, normalizedEvent)
		if err != nil {
			return err
	REDACTED
		if unsubscribed {
			slog.Info("notification email suppressed by unsubscribe preference", "event", normalizedEvent, "recipient_hash", notificationEmailHash(recipient))
			return nil
	REDACTED
REDACTED

	locale := normalizeNotificationLocale(input.Locale)
	if strings.TrimSpace(input.Locale) == "" {
		locale = s.ResolveRecipientLocale(ctx, input.UserID, recipient)
REDACTED
	tmpl, err := s.GetTemplate(ctx, normalizedEvent, locale)
	if err != nil {
		return err
REDACTED
	variables := s.runtimeVariables(ctx, normalizedEvent, locale, input)
	rendered, err := renderNotificationEmail(normalizedEvent, tmpl.Subject, tmpl.HTML, variables)
	if err != nil {
		return err
REDACTED

	deliveryKey := notificationEmailDeliveryKey(normalizedEvent, input.SourceType, input.SourceID, recipient, input.ReminderKey)
	if deliveryKey != "" {
		sent, err := s.deliveryExists(ctx, deliveryKey)
		if err != nil {
			return err
	REDACTED
		if sent {
			return nil
	REDACTED
REDACTED

	if s.emailService == nil {
		return errors.New("email service is not configured")
REDACTED
	if err := s.emailService.SendEmail(ctx, recipient, rendered.Subject, rendered.HTML); err != nil {
		return err
REDACTED
	if deliveryKey != "" {
		_ = s.settingRepo.Set(ctx, deliveryKey, time.Now().UTC().Format(time.RFC3339Nano))
REDACTED
	return nil
REDACTED

func (s *NotificationEmailService) RememberRecipientLocale(ctx context.Context, userID int64, email, acceptLanguage string) {
	locale := normalizeNotificationLocale(acceptLanguage)
	if strings.TrimSpace(acceptLanguage) == "" || s == nil || s.settingRepo == nil {
		return
REDACTED
	if userID > 0 {
		_ = s.settingRepo.Set(ctx, notificationEmailLocaleUserKeyPrefix+strconv.FormatInt(userID, 10), locale)
REDACTED
	if emailHash := notificationEmailHash(email); emailHash != "" {
		_ = s.settingRepo.Set(ctx, notificationEmailLocaleEmailKeyPrefix+emailHash, locale)
REDACTED
REDACTED

func (s *NotificationEmailService) ResolveRecipientLocale(ctx context.Context, userID int64, email string) string {
	if s == nil || s.settingRepo == nil {
		return notificationEmailDefaultLocale
REDACTED
	if userID > 0 {
		if locale, err := s.settingRepo.GetValue(ctx, notificationEmailLocaleUserKeyPrefix+strconv.FormatInt(userID, 10)); err == nil && strings.TrimSpace(locale) != "" {
			return normalizeNotificationLocale(locale)
	REDACTED
REDACTED
	if emailHash := notificationEmailHash(email); emailHash != "" {
		if locale, err := s.settingRepo.GetValue(ctx, notificationEmailLocaleEmailKeyPrefix+emailHash); err == nil && strings.TrimSpace(locale) != "" {
			return normalizeNotificationLocale(locale)
	REDACTED
REDACTED
	return notificationEmailDefaultLocale
REDACTED

func (s *NotificationEmailService) IsUnsubscribed(ctx context.Context, email, event string) (bool, error) {
	info, normalizedEvent, err := s.eventInfo(event)
	if err != nil {
		return false, err
REDACTED
	if !info.Optional {
		return false, nil
REDACTED
	value, err := s.settingRepo.GetValue(ctx, notificationEmailPreferenceKey(normalizedEvent, email))
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return false, nil
	REDACTED
		return false, err
REDACTED
	return strings.EqualFold(strings.TrimSpace(value), "unsubscribed"), nil
REDACTED

func (s *NotificationEmailService) Unsubscribe(ctx context.Context, token string) (NotificationEmailUnsubscribeResult, error) {
	claims, err := s.parseUnsubscribeToken(ctx, token)
	if err != nil {
		return NotificationEmailUnsubscribeResult{REDACTED, err
REDACTED
	info, normalizedEvent, err := s.eventInfo(claims.Event)
	if err != nil {
		return NotificationEmailUnsubscribeResult{REDACTED, err
REDACTED
	if !info.Optional {
		return NotificationEmailUnsubscribeResult{REDACTED, fmt.Errorf("%s is transactional and cannot be unsubscribed", normalizedEvent)
REDACTED
	if err := s.settingRepo.Set(ctx, notificationEmailPreferenceKey(normalizedEvent, claims.Email), "unsubscribed"); err != nil {
		return NotificationEmailUnsubscribeResult{REDACTED, err
REDACTED
	return NotificationEmailUnsubscribeResult{Event: normalizedEvent, Email: claims.Email, Done: trueREDACTED, nil
REDACTED

func (s *NotificationEmailService) eventInfo(event string) (NotificationEmailEventInfo, string, error) {
	normalized := strings.ToLower(strings.TrimSpace(event))
	info, ok := notificationEmailEventDefinitions[normalized]
	if !ok {
		return NotificationEmailEventInfo{REDACTED, "", fmt.Errorf("unsupported email template event: %s", event)
REDACTED
	return info, normalized, nil
REDACTED

func (s *NotificationEmailService) sampleVariables(ctx context.Context, event, locale string) map[string]string {
	info := notificationEmailEventDefinitions[event]
	variables := make(map[string]string, len(info.Placeholders))
	for key, value := range notificationEmailSampleVariables(locale) {
		variables[key] = value
REDACTED
	variables["site_name"] = s.siteName(ctx)
	if variables["unsubscribe_url"] == "" && info.Optional {
		variables["unsubscribe_url"] = "https://example.com/unsubscribe"
REDACTED
	return variables
REDACTED

func (s *NotificationEmailService) runtimeVariables(ctx context.Context, event, locale string, input NotificationEmailSendInput) map[string]string {
	variables := s.sampleVariables(ctx, event, locale)
	for key, value := range input.Variables {
		variables[key] = value
REDACTED
	variables["site_name"] = s.siteName(ctx)
	variables["recipient_email"] = input.RecipientEmail
	if strings.TrimSpace(input.RecipientName) != "" {
		variables["recipient_name"] = input.RecipientName
REDACTED
	if notificationEmailEventDefinitions[event].Optional {
		if unsubscribeURL, err := s.buildUnsubscribeURL(ctx, input.RecipientEmail, event); err == nil {
			variables["unsubscribe_url"] = unsubscribeURL
	REDACTED
REDACTED
	return variables
REDACTED

func (s *NotificationEmailService) siteName(ctx context.Context) string {
	if s == nil || s.settingRepo == nil {
		return defaultSiteName
REDACTED
	name, err := s.settingRepo.GetValue(ctx, SettingKeySiteName)
	if err != nil || strings.TrimSpace(name) == "" {
		return defaultSiteName
REDACTED
	return strings.TrimSpace(name)
REDACTED

func (s *NotificationEmailService) baseURL(ctx context.Context) string {
	if s == nil || s.settingRepo == nil {
		return ""
REDACTED
	for _, key := range []string{SettingKeyAPIBaseURL, SettingKeyFrontendURLREDACTED {
		value, err := s.settingRepo.GetValue(ctx, key)
		if err == nil && strings.TrimSpace(value) != "" {
			return strings.TrimRight(strings.TrimSpace(value), "/")
	REDACTED
REDACTED
	return ""
REDACTED

func (s *NotificationEmailService) buildUnsubscribeURL(ctx context.Context, email, event string) (string, error) {
	token, err := s.createUnsubscribeToken(ctx, email, event)
	if err != nil {
		return "", err
REDACTED
	path := "/api/v1/settings/email-unsubscribe?token=" + url.QueryEscape(token)
	baseURL := s.baseURL(ctx)
	if baseURL == "" {
		return path, nil
REDACTED
	return baseURL + path, nil
REDACTED

func (s *NotificationEmailService) createUnsubscribeToken(ctx context.Context, email, event string) (string, error) {
	secret, err := s.unsubscribeSecret(ctx)
	if err != nil {
		return "", err
REDACTED
	claims := notificationEmailUnsubscribeClaims{Email: strings.TrimSpace(email), Event: event, Exp: time.Now().Add(notificationEmailUnsubscribeTTL).Unix()REDACTED
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
REDACTED
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := signNotificationEmailToken(secret, encodedPayload)
	return encodedPayload + "." + signature, nil
REDACTED

func (s *NotificationEmailService) parseUnsubscribeToken(ctx context.Context, token string) (notificationEmailUnsubscribeClaims, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return notificationEmailUnsubscribeClaims{REDACTED, errors.New("invalid unsubscribe token")
REDACTED
	secret, err := s.unsubscribeSecret(ctx)
	if err != nil {
		return notificationEmailUnsubscribeClaims{REDACTED, err
REDACTED
	expected := signNotificationEmailToken(secret, parts[0])
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return notificationEmailUnsubscribeClaims{REDACTED, errors.New("invalid unsubscribe token signature")
REDACTED
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return notificationEmailUnsubscribeClaims{REDACTED, errors.New("invalid unsubscribe token payload")
REDACTED
	var claims notificationEmailUnsubscribeClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return notificationEmailUnsubscribeClaims{REDACTED, errors.New("invalid unsubscribe token payload")
REDACTED
	if strings.TrimSpace(claims.Email) == "" || strings.TrimSpace(claims.Event) == "" {
		return notificationEmailUnsubscribeClaims{REDACTED, errors.New("invalid unsubscribe token claims")
REDACTED
	if claims.Exp <= time.Now().Unix() {
		return notificationEmailUnsubscribeClaims{REDACTED, errors.New("unsubscribe token expired")
REDACTED
	return claims, nil
REDACTED

func (s *NotificationEmailService) unsubscribeSecret(ctx context.Context) (string, error) {
	secret, err := s.settingRepo.GetValue(ctx, notificationEmailUnsubscribeSecretKey)
	if err == nil && strings.TrimSpace(secret) != "" {
		return strings.TrimSpace(secret), nil
REDACTED
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return "", err
REDACTED
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
REDACTED
	secret = base64.RawURLEncoding.EncodeToString(buf)
	if err := s.settingRepo.Set(ctx, notificationEmailUnsubscribeSecretKey, secret); err != nil {
		return "", err
REDACTED
	return secret, nil
REDACTED

func (s *NotificationEmailService) deliveryExists(ctx context.Context, key string) (bool, error) {
	_, err := s.settingRepo.GetValue(ctx, key)
	if err == nil {
		return true, nil
REDACTED
	if errors.Is(err, ErrSettingNotFound) {
		return false, nil
REDACTED
	return false, err
REDACTED

func validateNotificationEmailTemplate(event, subject, htmlBody string) error {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return errors.New("email subject cannot be empty")
REDACTED
	if len([]rune(subject)) > notificationEmailMaxSubjectLength {
		return fmt.Errorf("email subject cannot exceed %d characters", notificationEmailMaxSubjectLength)
REDACTED
	if strings.TrimSpace(htmlBody) == "" {
		return errors.New("email html cannot be empty")
REDACTED
	if len([]byte(htmlBody)) > notificationEmailMaxHTMLLength {
		return fmt.Errorf("email html cannot exceed %d bytes", notificationEmailMaxHTMLLength)
REDACTED
	allowed := notificationEmailAllowedPlaceholderSet(event)
	for _, placeholder := range notificationEmailPlaceholdersIn(subject + "\n" + htmlBody) {
		if _, ok := allowed[placeholder]; !ok {
			return fmt.Errorf("unsupported placeholder {{%sREDACTEDREDACTED for event %s", placeholder, event)
	REDACTED
REDACTED
	return nil
REDACTED

func renderNotificationEmail(event, subject, htmlBody string, variables map[string]string) (NotificationEmailPreview, error) {
	if err := validateNotificationEmailTemplate(event, subject, htmlBody); err != nil {
		return NotificationEmailPreview{REDACTED, err
REDACTED
	renderedSubject, err := renderNotificationEmailString(event, subject, variables, false)
	if err != nil {
		return NotificationEmailPreview{REDACTED, err
REDACTED
	renderedHTML, err := renderNotificationEmailString(event, htmlBody, variables, true)
	if err != nil {
		return NotificationEmailPreview{REDACTED, err
REDACTED
	return NotificationEmailPreview{Subject: sanitizeEmailHeader(renderedSubject), HTML: renderedHTMLREDACTED, nil
REDACTED

func renderNotificationEmailString(event, raw string, variables map[string]string, escapeHTML bool) (string, error) {
	allowed := notificationEmailAllowedPlaceholderSet(event)
	var renderErr error
	rendered := notificationEmailPlaceholderPattern.ReplaceAllStringFunc(raw, func(match string) string {
		if renderErr != nil {
			return ""
	REDACTED
		parts := notificationEmailPlaceholderPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return ""
	REDACTED
		name := parts[1]
		if _, ok := allowed[name]; !ok {
			renderErr = fmt.Errorf("unsupported placeholder {{%sREDACTEDREDACTED for event %s", name, event)
			return ""
	REDACTED
		value := variables[name]
		if strings.HasSuffix(name, "_url") && !isSafeNotificationEmailURL(value) {
			value = ""
	REDACTED
		if escapeHTML {
			return html.EscapeString(value)
	REDACTED
		return sanitizeEmailHeader(value)
REDACTED)
	if renderErr != nil {
		return "", renderErr
REDACTED
	return rendered, nil
REDACTED

func notificationEmailAllowedPlaceholderSet(event string) map[string]struct{REDACTED {
	info := notificationEmailEventDefinitions[event]
	allowed := make(map[string]struct{REDACTED, len(info.Placeholders))
	for _, placeholder := range info.Placeholders {
		allowed[placeholder] = struct{REDACTED{REDACTED
REDACTED
	return allowed
REDACTED

func notificationEmailPlaceholdersIn(raw string) []string {
	matches := notificationEmailPlaceholderPattern.FindAllStringSubmatch(raw, -1)
	seen := make(map[string]struct{REDACTED, len(matches))
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) != 2 {
			continue
	REDACTED
		if _, exists := seen[match[1]]; exists {
			continue
	REDACTED
		seen[match[1]] = struct{REDACTED{REDACTED
		out = append(out, match[1])
REDACTED
	return out
REDACTED

func normalizeNotificationLocale(raw string) string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return notificationEmailDefaultLocale
REDACTED
	for _, part := range strings.Split(trimmed, ",") {
		tag := strings.TrimSpace(strings.Split(part, ";")[0])
		if strings.HasPrefix(tag, "zh") || tag == "cn" {
			return notificationEmailLocaleChinese
	REDACTED
		if strings.HasPrefix(tag, "en") {
			return notificationEmailDefaultLocale
	REDACTED
REDACTED
	return notificationEmailDefaultLocale
REDACTED

func notificationEmailTemplateKey(event, locale string) string {
	return notificationEmailTemplateKeyPrefix + event + ":" + locale
REDACTED

func notificationEmailPreferenceKey(event, email string) string {
	return notificationEmailPreferenceKeyPrefix + event + ":" + notificationEmailHash(email)
REDACTED

func notificationEmailDeliveryKey(event, sourceType, sourceID, recipient, reminderKey string) string {
	if strings.TrimSpace(sourceType) == "" || strings.TrimSpace(sourceID) == "" || strings.TrimSpace(recipient) == "" {
		return ""
REDACTED
	parts := []string{notificationEmailDeliveryKeyPrefix, event, ":", safeNotificationEmailKeyPart(sourceType), ":", safeNotificationEmailKeyPart(sourceID), ":", notificationEmailHash(recipient)REDACTED
	if strings.TrimSpace(reminderKey) != "" {
		parts = append(parts, ":", safeNotificationEmailKeyPart(reminderKey))
REDACTED
	return strings.Join(parts, "")
REDACTED

func notificationEmailHash(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return ""
REDACTED
	sum := sha256.Sum256([]byte(trimmed))
	return hex.EncodeToString(sum[:])
REDACTED

func safeNotificationEmailKeyPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			builder.WriteRune(r)
	REDACTED else {
			builder.WriteRune('_')
	REDACTED
REDACTED
	return builder.String()
REDACTED

func signNotificationEmailToken(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
REDACTED

func isSafeNotificationEmailURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return true
REDACTED
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return false
REDACTED
	if parsed.IsAbs() {
		scheme := strings.ToLower(parsed.Scheme)
		return scheme == "http" || scheme == "https" || scheme == "mailto"
REDACTED
	return strings.HasPrefix(trimmed, "/")
REDACTED

func notificationEmailSampleVariables(locale string) map[string]string {
	if normalizeNotificationLocale(locale) == notificationEmailLocaleChinese {
		return map[string]string{
			"site_name":          defaultSiteName,
			"recipient_name":     "张三",
			"recipient_email":    "user@example.com",
			"subscription_group": "Claude Pro",
			"subscription_days":  "30",
			"expiry_time":        "2026-06-18 12:00",
			"days_remaining":     "3",
			"current_balance":    "12.34",
			"threshold":          "20.00",
			"recharge_url":       "https://example.com/recharge",
			"recharge_amount":    "50.00",
			"order_id":           "1024",
			"unsubscribe_url":    "https://example.com/unsubscribe",
	REDACTED
REDACTED
	return map[string]string{
		"site_name":          defaultSiteName,
		"recipient_name":     "Alex",
		"recipient_email":    "user@example.com",
		"subscription_group": "Claude Pro",
		"subscription_days":  "30",
		"expiry_time":        "2026-06-18 12:00",
		"days_remaining":     "3",
		"current_balance":    "12.34",
		"threshold":          "20.00",
		"recharge_url":       "https://example.com/recharge",
		"recharge_amount":    "50.00",
		"order_id":           "1024",
		"unsubscribe_url":    "https://example.com/unsubscribe",
REDACTED
REDACTED

var notificationEmailEventOrder = []string{
	NotificationEmailEventSubscriptionPurchaseSuccess,
	NotificationEmailEventSubscriptionExpiryReminder,
	NotificationEmailEventBalanceLow,
	NotificationEmailEventBalanceRechargeSuccess,
REDACTED

var notificationEmailEventDefinitions = map[string]NotificationEmailEventInfo{
	NotificationEmailEventSubscriptionPurchaseSuccess: {
		Event:        NotificationEmailEventSubscriptionPurchaseSuccess,
		Label:        "Subscription purchase success",
		Description:  "Sent after a subscription purchase is fulfilled.",
		Category:     "subscription",
		Optional:     false,
		Placeholders: append(append([]string{REDACTED, notificationEmailCommonPlaceholders...), "subscription_group", "subscription_days", "expiry_time", "order_id"),
REDACTED,
	NotificationEmailEventSubscriptionExpiryReminder: {
		Event:        NotificationEmailEventSubscriptionExpiryReminder,
		Label:        "Subscription expiry reminder",
		Description:  "Optional reminder sent before an active subscription expires.",
		Category:     "subscription",
		Optional:     true,
		Placeholders: append(append([]string{REDACTED, notificationEmailCommonPlaceholders...), "subscription_group", "expiry_time", "days_remaining", "unsubscribe_url"),
REDACTED,
	NotificationEmailEventBalanceLow: {
		Event:        NotificationEmailEventBalanceLow,
		Label:        "Low balance alert",
		Description:  "Optional alert sent when balance crosses the configured low-balance threshold.",
		Category:     "billing",
		Optional:     true,
		Placeholders: append(append([]string{REDACTED, notificationEmailCommonPlaceholders...), "current_balance", "threshold", "recharge_url", "unsubscribe_url"),
REDACTED,
	NotificationEmailEventBalanceRechargeSuccess: {
		Event:        NotificationEmailEventBalanceRechargeSuccess,
		Label:        "Balance recharge success",
		Description:  "Sent after a balance recharge order is fulfilled.",
		Category:     "billing",
		Optional:     false,
		Placeholders: append(append([]string{REDACTED, notificationEmailCommonPlaceholders...), "recharge_amount", "current_balance", "order_id"),
REDACTED,
REDACTED

var notificationEmailOfficialTemplates = map[string]map[string]notificationEmailOfficialTemplate{
	NotificationEmailEventSubscriptionPurchaseSuccess: {
		notificationEmailDefaultLocale: {
			Subject: "[{{site_nameREDACTEDREDACTED] Subscription purchase successful",
			HTML: notificationEmailCard("#2563eb", "Subscription activated", `
<p>Hello {{recipient_nameREDACTEDREDACTED,</p>
<p>Your subscription for <strong>{{subscription_groupREDACTEDREDACTED</strong> has been activated for <strong>{{subscription_daysREDACTEDREDACTED</strong> days.</p>
<p>Expiry time: <strong>{{expiry_timeREDACTEDREDACTED</strong></p>
<p>Order ID: {{order_idREDACTEDREDACTED</p>`),
	REDACTED,
		notificationEmailLocaleChinese: {
			Subject: "[{{site_nameREDACTEDREDACTED] 订阅购买成功",
			HTML: notificationEmailCard("#2563eb", "订阅已开通", `
<p>{{recipient_nameREDACTEDREDACTED，您好：</p>
<p>您的 <strong>{{subscription_groupREDACTEDREDACTED</strong> 订阅已成功开通，有效期 <strong>{{subscription_daysREDACTEDREDACTED</strong> 天。</p>
<p>到期时间：<strong>{{expiry_timeREDACTEDREDACTED</strong></p>
<p>订单号：{{order_idREDACTEDREDACTED</p>`),
	REDACTED,
REDACTED,
	NotificationEmailEventSubscriptionExpiryReminder: {
		notificationEmailDefaultLocale: {
			Subject: "[{{site_nameREDACTEDREDACTED] Subscription expires in {{days_remainingREDACTEDREDACTED day(s)",
			HTML: notificationEmailCard("#f97316", "Subscription expiry reminder", `
<p>Hello {{recipient_nameREDACTEDREDACTED,</p>
<p>Your <strong>{{subscription_groupREDACTEDREDACTED</strong> subscription will expire in <strong>{{days_remainingREDACTEDREDACTED</strong> day(s).</p>
<p>Expiry time: <strong>{{expiry_timeREDACTEDREDACTED</strong></p>
<p class="muted"><a href="{{unsubscribe_urlREDACTEDREDACTED">Unsubscribe from optional subscription reminders</a></p>`),
	REDACTED,
		notificationEmailLocaleChinese: {
			Subject: "[{{site_nameREDACTEDREDACTED] 订阅将在 {{days_remainingREDACTEDREDACTED 天后到期",
			HTML: notificationEmailCard("#f97316", "订阅到期提醒", `
<p>{{recipient_nameREDACTEDREDACTED，您好：</p>
<p>您的 <strong>{{subscription_groupREDACTEDREDACTED</strong> 订阅将在 <strong>{{days_remainingREDACTEDREDACTED</strong> 天后到期。</p>
<p>到期时间：<strong>{{expiry_timeREDACTEDREDACTED</strong></p>
<p class="muted"><a href="{{unsubscribe_urlREDACTEDREDACTED">退订此类订阅提醒</a></p>`),
	REDACTED,
REDACTED,
	NotificationEmailEventBalanceLow: {
		notificationEmailDefaultLocale: {
			Subject: "[{{site_nameREDACTEDREDACTED] Low balance alert",
			HTML: notificationEmailCard("#d97706", "Low balance alert", `
<p>Hello {{recipient_nameREDACTEDREDACTED,</p>
<p>Your current balance is <strong>${{current_balanceREDACTEDREDACTED</strong>, below the configured alert threshold of <strong>${{thresholdREDACTEDREDACTED</strong>.</p>
<p>Please recharge in time to avoid service interruption.</p>
<p><a class="button" href="{{recharge_urlREDACTEDREDACTED">Recharge now</a></p>
<p class="muted"><a href="{{unsubscribe_urlREDACTEDREDACTED">Unsubscribe from optional balance alerts</a></p>`),
	REDACTED,
		notificationEmailLocaleChinese: {
			Subject: "[{{site_nameREDACTEDREDACTED] 余额不足提醒",
			HTML: notificationEmailCard("#d97706", "余额不足提醒", `
<p>{{recipient_nameREDACTEDREDACTED，您好：</p>
<p>您当前余额为 <strong>${{current_balanceREDACTEDREDACTED</strong>，已低于提醒阈值 <strong>${{thresholdREDACTEDREDACTED</strong>。</p>
<p>请及时充值以免服务中断。</p>
<p><a class="button" href="{{recharge_urlREDACTEDREDACTED">立即充值</a></p>
<p class="muted"><a href="{{unsubscribe_urlREDACTEDREDACTED">退订此类余额提醒</a></p>`),
	REDACTED,
REDACTED,
	NotificationEmailEventBalanceRechargeSuccess: {
		notificationEmailDefaultLocale: {
			Subject: "[{{site_nameREDACTEDREDACTED] Balance recharge successful",
			HTML: notificationEmailCard("#16a34a", "Recharge successful", `
<p>Hello {{recipient_nameREDACTEDREDACTED,</p>
<p>Your balance recharge of <strong>${{recharge_amountREDACTEDREDACTED</strong> has been completed.</p>
<p>Current balance: <strong>${{current_balanceREDACTEDREDACTED</strong></p>
<p>Order ID: {{order_idREDACTEDREDACTED</p>`),
	REDACTED,
		notificationEmailLocaleChinese: {
			Subject: "[{{site_nameREDACTEDREDACTED] 余额充值成功",
			HTML: notificationEmailCard("#16a34a", "余额充值成功", `
<p>{{recipient_nameREDACTEDREDACTED，您好：</p>
<p>您的余额充值 <strong>${{recharge_amountREDACTEDREDACTED</strong> 已完成。</p>
<p>当前余额：<strong>${{current_balanceREDACTEDREDACTED</strong></p>
<p>订单号：{{order_idREDACTEDREDACTED</p>`),
	REDACTED,
REDACTED,
REDACTED

func notificationEmailCard(accent, title, content string) string {
	return `<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <style>
    body { margin: 0; padding: 24px; background: #f4f4f5; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; color: #18181b; REDACTED
    .container { max-width: 640px; margin: 0 auto; background: #ffffff; border-radius: 12px; overflow: hidden; box-shadow: 0 8px 30px rgba(15, 23, 42, 0.10); REDACTED
    .header { background: ` + accent + `; color: #ffffff; padding: 28px 32px; REDACTED
    .header h1 { margin: 0; font-size: 24px; line-height: 1.25; REDACTED
    .content { padding: 32px; font-size: 15px; line-height: 1.7; REDACTED
    .button { display: inline-block; margin-top: 12px; padding: 11px 18px; border-radius: 8px; background: ` + accent + `; color: #ffffff; text-decoration: none; font-weight: 600; REDACTED
    .muted { color: #71717a; font-size: 13px; REDACTED
    .footer { padding: 18px 32px; background: #fafafa; color: #a1a1aa; font-size: 12px; REDACTED
  </style>
</head>
<body>
  <div class="container">
    <div class="header"><h1>` + title + `</h1></div>
    <div class="content">` + content + `</div>
    <div class="footer">This email was sent by {{site_nameREDACTEDREDACTED. Please do not reply directly.</div>
  </div>
</body>
</html>`
REDACTED
