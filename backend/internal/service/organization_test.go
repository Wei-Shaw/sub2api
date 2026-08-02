//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type organizationRepoStub struct {
	OrganizationRepository
	contextResult              *OrganizationContext
	contextErr                 error
	applicationResult          *CompanyApplication
	applicationErr             error
	createdUser                *User
	createErr                  error
	findUser                   *User
	findContext                *OrganizationContext
	findErr                    error
	findLoginName              string
	findCompanyID              string
	resolved                   *BillingContext
	resolveErr                 error
	requiredAmount             float64
	policyErr                  error
	memberStatusErr            error
	adminOrgSub                *OrganizationSubscription
	adminOrgSubErr             error
	adminActorID               int64
	adminOrganization          int64
	adminGroupID               int64
	adminValidity              int
	adminNotes                 string
	spendMemberIDs             []int64
	spendDaily                 *string
	spendMonthly               *string
	spendAlert                 bool
	spendThreshold             float64
	spendRecipients            []string
	spendCheckCalls            int
	spendCheckSource           string
	spendCheckAmount           float64
	spendCheckErr              error
	organizationBalance        float64
	organizationBalanceErr     error
	organizationBalanceDebits  []float64
	organizationBalanceCredits []float64
}

func (s *organizationRepoStub) GetContextForUser(context.Context, int64) (*OrganizationContext, error) {
	return s.contextResult, s.contextErr
}
func (s *organizationRepoStub) GetApplicationForUser(context.Context, int64) (*CompanyApplication, error) {
	return s.applicationResult, s.applicationErr
}
func (s *organizationRepoStub) CreateIAMMember(_ context.Context, _ int64, user *User, _ int) (*IAMMember, error) {
	s.createdUser = user
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &IAMMember{UserID: 2, ExternalUserID: "201705485041478971", Username: user.Username, LoginName: user.LoginName, Principal: CanonicalIAMPrincipal(user.LoginName, "c123456789012345"), Status: MembershipStatusActive, MustChangePassword: user.MustChangePassword, PolicyNames: []string{}}, nil
}
func (s *organizationRepoStub) FindIAMByPrincipal(_ context.Context, loginName, companyID string) (*User, *OrganizationContext, error) {
	s.findLoginName = loginName
	s.findCompanyID = companyID
	return s.findUser, s.findContext, s.findErr
}
func (s *organizationRepoStub) ResolveBillingContext(_ context.Context, _ int64, requiredAmount float64) (*BillingContext, error) {
	s.requiredAmount = requiredAmount
	return s.resolved, s.resolveErr
}
func (s *organizationRepoStub) SetPolicyAttachment(context.Context, int64, int64, string, bool, string) error {
	return s.policyErr
}
func (s *organizationRepoStub) SetIAMMemberStatus(context.Context, int64, int64, string) error {
	return s.memberStatusErr
}
func (s *organizationRepoStub) AdminCreateOrganizationSubscription(_ context.Context, actorID, organizationID, groupID int64, validityDays int, notes string) (*OrganizationSubscription, error) {
	s.adminActorID = actorID
	s.adminOrganization = organizationID
	s.adminGroupID = groupID
	s.adminValidity = validityDays
	s.adminNotes = notes
	return s.adminOrgSub, s.adminOrgSubErr
}
func (s *organizationRepoStub) ListSpendLimitRules(context.Context, int64) ([]OrganizationSpendLimitRule, error) {
	return []OrganizationSpendLimitRule{}, nil
}
func (s *organizationRepoStub) UpsertSpendLimitRules(_ context.Context, _ int64, memberIDs []int64, daily, monthly *string, alert bool, threshold float64, recipients []string) ([]OrganizationSpendLimitRule, error) {
	s.spendMemberIDs = append([]int64(nil), memberIDs...)
	s.spendDaily = daily
	s.spendMonthly = monthly
	s.spendAlert = alert
	s.spendThreshold = threshold
	s.spendRecipients = append([]string(nil), recipients...)
	return []OrganizationSpendLimitRule{}, nil
}
func (s *organizationRepoStub) DeleteSpendLimitRule(context.Context, int64, *int64) error {
	return nil
}
func (s *organizationRepoStub) ListSpendLimitUsage(context.Context, int64) ([]OrganizationSpendUsage, error) {
	return []OrganizationSpendUsage{}, nil
}
func (s *organizationRepoStub) CheckOrganizationSpendLimit(_ context.Context, _ int64, source string, amount float64) error {
	s.spendCheckCalls++
	s.spendCheckSource = source
	s.spendCheckAmount = amount
	return s.spendCheckErr
}
func (s *organizationRepoStub) RecordSpendLimitAlert(context.Context, int64, string) error {
	return nil
}
func (s *organizationRepoStub) GetOrganizationBalance(context.Context, int64) (float64, error) {
	return s.organizationBalance, s.organizationBalanceErr
}
func (s *organizationRepoStub) DeductOrganizationBalance(_ context.Context, _ int64, amount float64) (float64, error) {
	if s.organizationBalanceErr != nil {
		return 0, s.organizationBalanceErr
	}
	s.organizationBalanceDebits = append(s.organizationBalanceDebits, amount)
	s.organizationBalance -= amount
	return s.organizationBalance, nil
}
func (s *organizationRepoStub) CreditOrganizationBalance(_ context.Context, _ int64, amount float64) (float64, error) {
	if s.organizationBalanceErr != nil {
		return 0, s.organizationBalanceErr
	}
	s.organizationBalanceCredits = append(s.organizationBalanceCredits, amount)
	s.organizationBalance += amount
	return s.organizationBalance, nil
}

func TestBillingContextCompanySourceRequiresOrganizationID(t *testing.T) {
	billing := &BillingContext{PayerUserID: 99, BalanceSource: BalanceSourceCompany}
	require.True(t, billing.UsesCompanyBalance())

	resolver := NewBillingContextResolver(&organizationRepoStub{})
	_, err := resolver.DeductOrganizationBalance(context.Background(), billing, 1)
	require.ErrorIs(t, err, ErrCompanyNotFound)
}

type organizationAuthCacheInvalidatorStub struct{ userIDs []int64 }

func (s *organizationAuthCacheInvalidatorStub) InvalidateAuthCacheByKey(context.Context, string) {}
func (s *organizationAuthCacheInvalidatorStub) InvalidateAuthCacheByGroupID(context.Context, int64) {
}
func (s *organizationAuthCacheInvalidatorStub) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	s.userIDs = append(s.userIDs, userID)
}

type organizationUserRepoStub struct {
	UserRepository
	user *User
	err  error
}

func (s *organizationUserRepoStub) GetByID(context.Context, int64) (*User, error) {
	return s.user, s.err
}

func companyTestConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Company.IAMEnabled = true
	cfg.Company.ApplicationsEnabled = true
	cfg.Company.DefaultMemberLimit = 20
	cfg.Company.UpgradeFee = 20
	cfg.Company.UpgradeCurrency = "USD"
	return cfg
}

type organizationFeatureReaderStub struct {
	applications bool
	iam          bool
}

func (s organizationFeatureReaderStub) IsCompanyApplicationsEnabled(context.Context) bool {
	return s.applications
}

func (s organizationFeatureReaderStub) IsCompanyIAMEnabled(context.Context) bool {
	return s.iam
}

func TestOrganizationServiceUsesRuntimeIAMFeatureSetting(t *testing.T) {
	repo := &organizationRepoStub{}
	svc := NewOrganizationService(repo, &organizationUserRepoStub{}, &config.Config{})
	svc.SetCompanyFeatureReader(organizationFeatureReaderStub{iam: true})

	member, _, err := svc.CreateIAMMember(context.Background(), 1, "member", "", "", "initial-password", true)

	require.NoError(t, err)
	require.NotNil(t, member)
}

func TestOrganizationServiceCreateIAMMemberHashesOwnerPasswordAndHonorsPasswordChangeChoice(t *testing.T) {
	repo := &organizationRepoStub{}
	service := NewOrganizationService(repo, &organizationUserRepoStub{}, companyTestConfig())
	initialPassword := "Owner-chosen-password-123"

	member, password, err := service.CreateIAMMember(context.Background(), 1, " Finance.Reader ", " Finance Reader ", "recovery@example.com", initialPassword, false)
	require.NoError(t, err)
	require.Equal(t, "finance.reader", member.LoginName)
	require.Equal(t, "Finance Reader", member.Username)
	require.Equal(t, "finance.reader@c123456789012345.opentk.ai", member.Principal)
	require.Equal(t, initialPassword, password)
	require.False(t, member.MustChangePassword)
	require.NotNil(t, repo.createdUser)
	require.NotEqual(t, password, repo.createdUser.PasswordHash)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(repo.createdUser.PasswordHash), []byte(password)))
	require.False(t, repo.createdUser.MustChangePassword)
	require.Equal(t, "Finance Reader", repo.createdUser.Username)
	require.Equal(t, RoleUser, repo.createdUser.Role, "organization ownership must not become a global admin role")
	require.Zero(t, repo.createdUser.Balance)

	encoded, err := json.Marshal(member)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), password)
	require.NotContains(t, string(encoded), "password_hash")
}

func TestOrganizationServiceCreateIAMMemberDoesNotReturnCredentialOnFailure(t *testing.T) {
	repo := &organizationRepoStub{createErr: ErrIAMMemberLimit}
	service := NewOrganizationService(repo, &organizationUserRepoStub{}, companyTestConfig())

	member, password, err := service.CreateIAMMember(context.Background(), 1, "member", "", "", "initial-password", true)
	require.ErrorIs(t, err, ErrIAMMemberLimit)
	require.Nil(t, member)
	require.Empty(t, password)
}

func TestOrganizationServiceCreateIAMMemberRejectsInvalidPasswordLength(t *testing.T) {
	repo := &organizationRepoStub{}
	service := NewOrganizationService(repo, &organizationUserRepoStub{}, companyTestConfig())

	member, password, err := service.CreateIAMMember(context.Background(), 1, "member", "", "", "short", true)

	require.ErrorIs(t, err, ErrIAMPassword)
	require.Nil(t, member)
	require.Empty(t, password)
	require.Nil(t, repo.createdUser)
}

func TestOrganizationServiceAdminCreateOrganizationSubscription(t *testing.T) {
	repo := &organizationRepoStub{adminOrgSub: &OrganizationSubscription{ID: 7, OrganizationID: 11, GroupID: 13}}
	service := NewOrganizationService(repo, &organizationUserRepoStub{}, companyTestConfig())

	subscription, err := service.AdminCreateOrganizationSubscription(context.Background(), 5, 11, 13, 30, " admin grant ")

	require.NoError(t, err)
	require.Equal(t, int64(7), subscription.ID)
	require.Equal(t, int64(5), repo.adminActorID)
	require.Equal(t, int64(11), repo.adminOrganization)
	require.Equal(t, int64(13), repo.adminGroupID)
	require.Equal(t, 30, repo.adminValidity)
	require.Equal(t, "admin grant", repo.adminNotes)
}

func TestOrganizationServiceAdminCreateOrganizationSubscriptionRejectsInvalidValidity(t *testing.T) {
	repo := &organizationRepoStub{}
	service := NewOrganizationService(repo, &organizationUserRepoStub{}, companyTestConfig())

	_, err := service.AdminCreateOrganizationSubscription(context.Background(), 5, 11, 13, 0, "")

	require.Error(t, err)
	require.Zero(t, repo.adminActorID)
}

func TestOrganizationServiceAuthenticateIAMParsesCanonicalPrincipal(t *testing.T) {
	validPassword := "correct-password"
	hash, err := bcrypt.GenerateFromPassword([]byte(validPassword), bcrypt.MinCost)
	require.NoError(t, err)
	repo := &organizationRepoStub{
		findUser:    &User{Status: StatusActive, PasswordHash: string(hash)},
		findContext: &OrganizationContext{OrganizationStatus: OrganizationStatusActive, MembershipStatus: MembershipStatusActive},
	}
	service := NewOrganizationService(repo, &organizationUserRepoStub{}, companyTestConfig())

	_, _, err = service.AuthenticateIAM(context.Background(), "reader@c123456789012345.opentk.ai", validPassword)

	require.NoError(t, err)
	require.Equal(t, "reader", repo.findLoginName)
	require.Equal(t, "c123456789012345", repo.findCompanyID)
}

func TestOrganizationServiceAuthenticateIAMUsesGenericFailures(t *testing.T) {
	validPassword := "correct-password"
	hash, err := bcrypt.GenerateFromPassword([]byte(validPassword), bcrypt.MinCost)
	require.NoError(t, err)
	active := &OrganizationContext{OrganizationStatus: OrganizationStatusActive, MembershipStatus: MembershipStatusActive}

	for _, test := range []struct {
		name      string
		principal string
		password  string
		repo      *organizationRepoStub
	}{{"malformed", "recovery@example.com", validPassword, &organizationRepoStub{}}, {"legacy numeric principal", "reader@1719905235756637.opentk.ai", validPassword, &organizationRepoStub{}}, {"unknown", "reader@c123456789012345.opentk.ai", validPassword, &organizationRepoStub{findErr: ErrIAMMemberNotFound}}, {"disabled membership", "reader@c123456789012345.opentk.ai", validPassword, &organizationRepoStub{findUser: &User{Status: StatusActive, PasswordHash: string(hash)}, findContext: &OrganizationContext{OrganizationStatus: OrganizationStatusActive, MembershipStatus: MembershipStatusDisabled}}}, {"disabled user", "reader@c123456789012345.opentk.ai", validPassword, &organizationRepoStub{findUser: &User{Status: "disabled", PasswordHash: string(hash)}, findContext: active}}, {"wrong password", "reader@c123456789012345.opentk.ai", "wrong", &organizationRepoStub{findUser: &User{Status: StatusActive, PasswordHash: string(hash)}, findContext: active}}} {
		t.Run(test.name, func(t *testing.T) {
			service := NewOrganizationService(test.repo, &organizationUserRepoStub{}, companyTestConfig())
			_, _, err := service.AuthenticateIAM(context.Background(), test.principal, test.password)
			require.ErrorIs(t, err, ErrInvalidCredentials)
		})
	}
}

func TestOrganizationServiceAuthenticateIAMRejectsWhenFeatureDisabled(t *testing.T) {
	service := NewOrganizationService(&organizationRepoStub{}, &organizationUserRepoStub{}, &config.Config{})

	user, organization, err := service.AuthenticateIAM(context.Background(), "reader@c123456789012345.opentk.ai", "password")

	require.ErrorIs(t, err, ErrIAMFeatureDisabled)
	require.Nil(t, user)
	require.Nil(t, organization)
}

func TestOrganizationServiceUpgradeEligibility(t *testing.T) {
	root := &User{ID: 1, IdentityType: IdentityTypeRoot, Status: StatusActive}
	pending := &CompanyApplication{ID: 8, Status: "pending"}
	service := NewOrganizationService(&organizationRepoStub{contextErr: ErrCompanyNotFound, applicationResult: pending}, &organizationUserRepoStub{user: root}, companyTestConfig())

	result, err := service.UpgradeEligibility(context.Background(), root.ID)
	require.NoError(t, err)
	require.False(t, result.Eligible)
	require.Equal(t, "application_pending", result.Reason)
	require.Equal(t, "20.00000000", result.FeeAmount)
	require.Equal(t, pending, result.Application)
}

func TestBillingContextResolverFailsClosedWithoutFallback(t *testing.T) {
	expected := errors.New("authorization lookup unavailable")
	resolver := NewBillingContextResolver(&organizationRepoStub{resolveErr: expected})

	resolved, err := resolver.Resolve(context.Background(), 12)
	require.Error(t, err)
	require.ErrorIs(t, err, expected)
	require.Nil(t, resolved)
}

func TestBillingContextResolverPassesRequiredAmount(t *testing.T) {
	repo := &organizationRepoStub{resolved: &BillingContext{ConsumerUserID: 12, PayerUserID: 12, BalanceSource: "allocated"}}
	resolver := NewBillingContextResolver(repo)

	resolved, err := resolver.ResolveForAmount(context.Background(), 12, 3.25)
	require.NoError(t, err)
	require.Equal(t, 3.25, repo.requiredAmount)
	require.Equal(t, int64(12), resolved.PayerUserID)
}

func TestOrganizationServiceNormalizesSpendLimitBatch(t *testing.T) {
	repo := &organizationRepoStub{}
	svc := NewOrganizationService(repo, &organizationUserRepoStub{}, companyTestConfig())
	daily := " 10.5 "
	monthly := "100"

	_, err := svc.UpsertSpendLimitRules(context.Background(), 1, []int64{22, 22, 23}, &daily, &monthly, true, 80, []string{" Ops@Example.com ", "ops@example.com", "finance@example.com"})

	require.NoError(t, err)
	require.Equal(t, []int64{22, 23}, repo.spendMemberIDs)
	require.Equal(t, "10.5000000000", *repo.spendDaily)
	require.Equal(t, "100.0000000000", *repo.spendMonthly)
	require.True(t, repo.spendAlert)
	require.Equal(t, 80.0, repo.spendThreshold)
	require.Equal(t, []string{"ops@example.com", "finance@example.com"}, repo.spendRecipients)
}

func TestOrganizationServiceRejectsInvalidSpendLimitConfiguration(t *testing.T) {
	svc := NewOrganizationService(&organizationRepoStub{}, &organizationUserRepoStub{}, companyTestConfig())

	_, err := svc.UpsertSpendLimitRules(context.Background(), 1, nil, nil, nil, false, 80, nil)
	require.ErrorIs(t, err, ErrSpendLimitInvalid)
	daily := "10"
	_, err = svc.UpsertSpendLimitRules(context.Background(), 1, nil, &daily, nil, true, 0, nil)
	require.ErrorIs(t, err, ErrSpendLimitThreshold)
	_, err = svc.UpsertSpendLimitRules(context.Background(), 1, nil, &daily, nil, true, 80, []string{"not-an-email"})
	require.Error(t, err)
}

func TestBillingContextResolverChecksOnlyCompanySponsoredSources(t *testing.T) {
	repo := &organizationRepoStub{resolved: &BillingContext{ConsumerUserID: 12, PayerUserID: 12, BalanceSource: "allocated"}}
	resolver := NewBillingContextResolver(repo)

	_, err := resolver.ResolveForAmount(context.Background(), 12, 3.25)
	require.NoError(t, err)
	require.Zero(t, repo.spendCheckCalls)

	organizationID := int64(8)
	repo.resolved = &BillingContext{ConsumerUserID: 12, OrganizationID: &organizationID, PayerUserID: 1, BalanceSource: BalanceSourceCompany}
	repo.spendCheckErr = ErrSpendLimitExceeded
	_, err = resolver.ResolveForAmount(context.Background(), 12, 4.5)
	require.ErrorIs(t, err, ErrSpendLimitExceeded)
	require.Equal(t, 1, repo.spendCheckCalls)
	require.Equal(t, BalanceSourceCompany, repo.spendCheckSource)
	require.Equal(t, 4.5, repo.spendCheckAmount)

	_, err = resolver.ResolveForAmount(context.Background(), 12, 0)
	require.ErrorIs(t, err, ErrSpendLimitExceeded)
	require.Equal(t, 2, repo.spendCheckCalls)
	require.Zero(t, repo.spendCheckAmount)
}

func TestOrganizationAuthorizationMutationsInvalidateUserCaches(t *testing.T) {
	repo := &organizationRepoStub{}
	invalidator := &organizationAuthCacheInvalidatorStub{}
	svc := NewOrganizationService(repo, &organizationUserRepoStub{}, companyTestConfig())
	svc.SetAuthCacheInvalidator(invalidator)

	require.NoError(t, svc.SetPolicyAttachment(context.Background(), 1, 22, PolicyCompanySharedBalance, true, "request-1"))
	require.NoError(t, svc.SetIAMMemberStatus(context.Background(), 1, 23, MembershipStatusDisabled))
	require.Equal(t, []int64{22, 23}, invalidator.userIDs)
}

func TestAPIKeyOrganizationAuthorizationFailsClosedAcrossInstances(t *testing.T) {
	repo := &organizationRepoStub{contextResult: &OrganizationContext{
		OrganizationStatus: OrganizationStatusActive,
		MembershipStatus:   MembershipStatusActive,
		AuthzGeneration:    2,
	}}
	issuedSnapshot := &User{ID: 42, IdentityType: IdentityTypeIAM, Status: StatusActive, AuthzGeneration: 1}

	first := &APIKeyService{organizationRepo: repo}
	second := &APIKeyService{organizationRepo: repo}
	require.ErrorIs(t, first.ValidateOrganizationAccess(context.Background(), issuedSnapshot), ErrOrganizationPermission)
	require.ErrorIs(t, second.ValidateOrganizationAccess(context.Background(), issuedSnapshot), ErrOrganizationPermission)
	require.Equal(t, uint64(1), first.AuthCacheInvalidationSubscriberHealth().DatabaseFallbacks)
	require.Equal(t, uint64(1), second.AuthCacheInvalidationSubscriberHealth().DatabaseFallbacks)
}
