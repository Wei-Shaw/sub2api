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
	contextResult     *OrganizationContext
	contextErr        error
	applicationResult *CompanyApplication
	applicationErr    error
	createdUser       *User
	createErr         error
	findUser          *User
	findContext       *OrganizationContext
	findErr           error
	findLoginName     string
	findAccountID     string
	resolved          *BillingContext
	resolveErr        error
	policyErr         error
	memberStatusErr   error
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
	return &IAMMember{UserID: 2, ExternalUserID: "201705485041478971", LoginName: user.LoginName, Principal: CanonicalIAMPrincipal(user.LoginName, "1719905235756637"), Status: MembershipStatusActive, MustChangePassword: user.MustChangePassword, PolicyNames: []string{}}, nil
}
func (s *organizationRepoStub) FindIAMByPrincipal(_ context.Context, loginName, accountID string) (*User, *OrganizationContext, error) {
	s.findLoginName = loginName
	s.findAccountID = accountID
	return s.findUser, s.findContext, s.findErr
}
func (s *organizationRepoStub) ResolveBillingContext(context.Context, int64) (*BillingContext, error) {
	return s.resolved, s.resolveErr
}
func (s *organizationRepoStub) SetPolicyAttachment(context.Context, int64, int64, string, bool, string) error {
	return s.policyErr
}
func (s *organizationRepoStub) SetIAMMemberStatus(context.Context, int64, int64, string) error {
	return s.memberStatusErr
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

func TestOrganizationServiceCreateIAMMemberHashesOwnerPasswordAndHonorsPasswordChangeChoice(t *testing.T) {
	repo := &organizationRepoStub{}
	service := NewOrganizationService(repo, &organizationUserRepoStub{}, companyTestConfig())
	initialPassword := "Owner-chosen-password-123"

	member, password, err := service.CreateIAMMember(context.Background(), 1, " Finance.Reader ", "recovery@example.com", initialPassword, false)
	require.NoError(t, err)
	require.Equal(t, "finance.reader", member.LoginName)
	require.Equal(t, "finance.reader@1719905235756637.opentk.ai", member.Principal)
	require.Equal(t, initialPassword, password)
	require.False(t, member.MustChangePassword)
	require.NotNil(t, repo.createdUser)
	require.NotEqual(t, password, repo.createdUser.PasswordHash)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(repo.createdUser.PasswordHash), []byte(password)))
	require.False(t, repo.createdUser.MustChangePassword)
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

	member, password, err := service.CreateIAMMember(context.Background(), 1, "member", "", "initial-password", true)
	require.ErrorIs(t, err, ErrIAMMemberLimit)
	require.Nil(t, member)
	require.Empty(t, password)
}

func TestOrganizationServiceCreateIAMMemberRejectsInvalidPasswordLength(t *testing.T) {
	repo := &organizationRepoStub{}
	service := NewOrganizationService(repo, &organizationUserRepoStub{}, companyTestConfig())

	member, password, err := service.CreateIAMMember(context.Background(), 1, "member", "", "short", true)

	require.ErrorIs(t, err, ErrIAMPassword)
	require.Nil(t, member)
	require.Empty(t, password)
	require.Nil(t, repo.createdUser)
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

	_, _, err = service.AuthenticateIAM(context.Background(), "reader@1719905235756637.opentk.ai", validPassword)

	require.NoError(t, err)
	require.Equal(t, "reader", repo.findLoginName)
	require.Equal(t, "1719905235756637", repo.findAccountID)
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
	}{{"malformed", "recovery@example.com", validPassword, &organizationRepoStub{}}, {"legacy principal", "reader@1719905235756637", validPassword, &organizationRepoStub{}}, {"unknown", "reader@1719905235756637.opentk.ai", validPassword, &organizationRepoStub{findErr: ErrIAMMemberNotFound}}, {"disabled membership", "reader@1719905235756637.opentk.ai", validPassword, &organizationRepoStub{findUser: &User{Status: StatusActive, PasswordHash: string(hash)}, findContext: &OrganizationContext{OrganizationStatus: OrganizationStatusActive, MembershipStatus: MembershipStatusDisabled}}}, {"disabled user", "reader@1719905235756637.opentk.ai", validPassword, &organizationRepoStub{findUser: &User{Status: "disabled", PasswordHash: string(hash)}, findContext: active}}, {"wrong password", "reader@1719905235756637.opentk.ai", "wrong", &organizationRepoStub{findUser: &User{Status: StatusActive, PasswordHash: string(hash)}, findContext: active}}} {
		t.Run(test.name, func(t *testing.T) {
			service := NewOrganizationService(test.repo, &organizationUserRepoStub{}, companyTestConfig())
			_, _, err := service.AuthenticateIAM(context.Background(), test.principal, test.password)
			require.ErrorIs(t, err, ErrInvalidCredentials)
		})
	}
}

func TestOrganizationServiceAuthenticateIAMRejectsWhenFeatureDisabled(t *testing.T) {
	service := NewOrganizationService(&organizationRepoStub{}, &organizationUserRepoStub{}, &config.Config{})

	user, organization, err := service.AuthenticateIAM(context.Background(), "reader@1719905235756637.opentk.ai", "password")

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
