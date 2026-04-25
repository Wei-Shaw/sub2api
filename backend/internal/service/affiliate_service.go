package service

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

var (
	ErrAffiliateProfileNotFound = infraerrors.NotFound("AFFILIATE_PROFILE_NOT_FOUND", "affiliate profile not found")
	ErrAffiliateCodeInvalid     = infraerrors.BadRequest("AFFILIATE_CODE_INVALID", "invalid affiliate code")
	ErrAffiliateAlreadyBound    = infraerrors.Conflict("AFFILIATE_ALREADY_BOUND", "affiliate inviter already bound")
	ErrAffiliateQuotaEmpty      = infraerrors.BadRequest("AFFILIATE_QUOTA_EMPTY", "no affiliate quota available to transfer")
)

const (
	affiliateInviteesLimit = 100
	// affiliateCodeFormatLength must stay in sync with repository.affiliateCodeLength.
	affiliateCodeFormatLength = 12
)

// affiliateCodeValidChar is a 256-entry lookup table mirroring the charset used
// by the repository's generateAffiliateCode (A-Z minus I/O, digits 2-9).
var affiliateCodeValidChar = func() [256]bool {
	var tbl [256]bool
	for _, c := range []byte("ABCDEFGHJKLMNPQRSTUVWXYZ23456789") {
		tbl[c] = true
REDACTED
	return tbl
REDACTED()

func isValidAffiliateCodeFormat(code string) bool {
	if len(code) != affiliateCodeFormatLength {
		return false
REDACTED
	for i := 0; i < len(code); i++ {
		if !affiliateCodeValidChar[code[i]] {
			return false
	REDACTED
REDACTED
	return true
REDACTED

type AffiliateSummary struct {
	UserID          int64     `json:"user_id"`
	AffCode         string    `json:"aff_code"`
	InviterID       *int64    `json:"inviter_id,omitempty"`
	AffCount        int       `json:"aff_count"`
	AffQuota        float64   `json:"aff_quota"`
	AffHistoryQuota float64   `json:"aff_history_quota"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
REDACTED

type AffiliateInvitee struct {
	UserID    int64      `json:"user_id"`
	Email     string     `json:"email"`
	Username  string     `json:"username"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
REDACTED

type AffiliateDetail struct {
	UserID          int64              `json:"user_id"`
	AffCode         string             `json:"aff_code"`
	InviterID       *int64             `json:"inviter_id,omitempty"`
	AffCount        int                `json:"aff_count"`
	AffQuota        float64            `json:"aff_quota"`
	AffHistoryQuota float64            `json:"aff_history_quota"`
	Invitees        []AffiliateInvitee `json:"invitees"`
REDACTED

type AffiliateRepository interface {
	EnsureUserAffiliate(ctx context.Context, userID int64) (*AffiliateSummary, error)
	GetAffiliateByCode(ctx context.Context, code string) (*AffiliateSummary, error)
	BindInviter(ctx context.Context, userID, inviterID int64) (bool, error)
	AccrueQuota(ctx context.Context, inviterID, inviteeUserID int64, amount float64) (bool, error)
	TransferQuotaToBalance(ctx context.Context, userID int64) (float64, float64, error)
	ListInvitees(ctx context.Context, inviterID int64, limit int) ([]AffiliateInvitee, error)
REDACTED

type AffiliateService struct {
	repo                 AffiliateRepository
	settingRepo          SettingRepository
	authCacheInvalidator APIKeyAuthCacheInvalidator
	billingCacheService  *BillingCacheService
REDACTED

func NewAffiliateService(repo AffiliateRepository, settingRepo SettingRepository, authCacheInvalidator APIKeyAuthCacheInvalidator, billingCacheService *BillingCacheService) *AffiliateService {
	return &AffiliateService{
		repo:                 repo,
		settingRepo:          settingRepo,
		authCacheInvalidator: authCacheInvalidator,
		billingCacheService:  billingCacheService,
REDACTED
REDACTED

func (s *AffiliateService) EnsureUserAffiliate(ctx context.Context, userID int64) (*AffiliateSummary, error) {
	if userID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_USER", "invalid user")
REDACTED
	if s == nil || s.repo == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
REDACTED
	return s.repo.EnsureUserAffiliate(ctx, userID)
REDACTED

func (s *AffiliateService) GetAffiliateDetail(ctx context.Context, userID int64) (*AffiliateDetail, error) {
	summary, err := s.EnsureUserAffiliate(ctx, userID)
	if err != nil {
		return nil, err
REDACTED
	invitees, err := s.listInvitees(ctx, userID)
	if err != nil {
		return nil, err
REDACTED
	return &AffiliateDetail{
		UserID:          summary.UserID,
		AffCode:         summary.AffCode,
		InviterID:       summary.InviterID,
		AffCount:        summary.AffCount,
		AffQuota:        summary.AffQuota,
		AffHistoryQuota: summary.AffHistoryQuota,
		Invitees:        invitees,
REDACTED, nil
REDACTED

func (s *AffiliateService) BindInviterByCode(ctx context.Context, userID int64, rawCode string) error {
	code := strings.ToUpper(strings.TrimSpace(rawCode))
	if code == "" {
		return nil
REDACTED
	if !isValidAffiliateCodeFormat(code) {
		return ErrAffiliateCodeInvalid
REDACTED
	if s == nil || s.repo == nil {
		return infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
REDACTED

	selfSummary, err := s.repo.EnsureUserAffiliate(ctx, userID)
	if err != nil {
		return err
REDACTED
	if selfSummary.InviterID != nil {
		return nil
REDACTED

	inviterSummary, err := s.repo.GetAffiliateByCode(ctx, code)
	if err != nil {
		if errors.Is(err, ErrAffiliateProfileNotFound) {
			return ErrAffiliateCodeInvalid
	REDACTED
		return err
REDACTED
	if inviterSummary == nil || inviterSummary.UserID <= 0 || inviterSummary.UserID == userID {
		return ErrAffiliateCodeInvalid
REDACTED

	bound, err := s.repo.BindInviter(ctx, userID, inviterSummary.UserID)
	if err != nil {
		return err
REDACTED
	if !bound {
		return ErrAffiliateAlreadyBound
REDACTED
	return nil
REDACTED

func (s *AffiliateService) AccrueInviteRebate(ctx context.Context, inviteeUserID int64, baseRechargeAmount float64) (float64, error) {
	if s == nil || s.repo == nil {
		return 0, nil
REDACTED
	if inviteeUserID <= 0 || baseRechargeAmount <= 0 || math.IsNaN(baseRechargeAmount) || math.IsInf(baseRechargeAmount, 0) {
		return 0, nil
REDACTED

	inviteeSummary, err := s.repo.EnsureUserAffiliate(ctx, inviteeUserID)
	if err != nil {
		return 0, err
REDACTED
	if inviteeSummary.InviterID == nil || *inviteeSummary.InviterID <= 0 {
		return 0, nil
REDACTED

	rebateRatePercent := s.loadAffiliateRebateRatePercent(ctx)
	rebate := roundTo(baseRechargeAmount*(rebateRatePercent/100), 8)
	if rebate <= 0 {
		return 0, nil
REDACTED

	if _, err := s.repo.EnsureUserAffiliate(ctx, *inviteeSummary.InviterID); err != nil {
		return 0, err
REDACTED

	applied, err := s.repo.AccrueQuota(ctx, *inviteeSummary.InviterID, inviteeUserID, rebate)
	if err != nil {
		return 0, err
REDACTED
	if !applied {
		return 0, nil
REDACTED
	return rebate, nil
REDACTED

func (s *AffiliateService) TransferAffiliateQuota(ctx context.Context, userID int64) (float64, float64, error) {
	if s == nil || s.repo == nil {
		return 0, 0, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
REDACTED

	transferred, balance, err := s.repo.TransferQuotaToBalance(ctx, userID)
	if err != nil {
		return 0, 0, err
REDACTED
	if transferred > 0 {
		s.invalidateAffiliateCaches(ctx, userID)
REDACTED
	return transferred, balance, nil
REDACTED

func (s *AffiliateService) listInvitees(ctx context.Context, inviterID int64) ([]AffiliateInvitee, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.ServiceUnavailable("SERVICE_UNAVAILABLE", "affiliate service unavailable")
REDACTED
	invitees, err := s.repo.ListInvitees(ctx, inviterID, affiliateInviteesLimit)
	if err != nil {
		return nil, err
REDACTED
	for i := range invitees {
		invitees[i].Email = maskEmail(invitees[i].Email)
REDACTED
	return invitees, nil
REDACTED

func (s *AffiliateService) loadAffiliateRebateRatePercent(ctx context.Context) float64 {
	if s == nil || s.settingRepo == nil {
		return AffiliateRebateRateDefault
REDACTED

	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAffiliateRebateRate)
	if err != nil {
		return AffiliateRebateRateDefault
REDACTED

	rate, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return AffiliateRebateRateDefault
REDACTED
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		return AffiliateRebateRateDefault
REDACTED
	if rate < AffiliateRebateRateMin {
		return AffiliateRebateRateMin
REDACTED
	if rate > AffiliateRebateRateMax {
		return AffiliateRebateRateMax
REDACTED
	return rate
REDACTED

func roundTo(v float64, scale int) float64 {
	factor := math.Pow10(scale)
	return math.Round(v*factor) / factor
REDACTED

func maskEmail(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return ""
REDACTED
	at := strings.Index(email, "@")
	if at <= 0 || at >= len(email)-1 {
		return "***"
REDACTED

	local := email[:at]
	domain := email[at+1:]
	dot := strings.LastIndex(domain, ".")

	maskedLocal := maskSegment(local)
	if dot <= 0 || dot >= len(domain)-1 {
		return maskedLocal + "@" + maskSegment(domain)
REDACTED

	domainName := domain[:dot]
	tld := domain[dot:]
	return maskedLocal + "@" + maskSegment(domainName) + tld
REDACTED

func maskSegment(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return "***"
REDACTED
	if len(r) == 1 {
		return string(r[0]) + "***"
REDACTED
	return string(r[0]) + "***"
REDACTED

func (s *AffiliateService) invalidateAffiliateCaches(ctx context.Context, userID int64) {
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
REDACTED
	if s.billingCacheService != nil {
		if err := s.billingCacheService.InvalidateUserBalance(ctx, userID); err != nil {
			logger.LegacyPrintf("service.affiliate", "[Affiliate] Failed to invalidate billing cache for user %d: %v", userID, err)
	REDACTED
REDACTED
REDACTED
