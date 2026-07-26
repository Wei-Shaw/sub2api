//go:build integration

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/accountid"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func createOrganizationRoot(t *testing.T, client *dbent.Client, balance float64, role string) *service.User {
	t.Helper()
	id, err := accountid.GenerateRoot()
	require.NoError(t, err)
	if role == "" {
		role = service.RoleUser
	}
	created, err := client.User.Create().
		SetEmail(fmt.Sprintf("company-%s@example.com", uuid.NewString())).
		SetAccountID(id).
		SetExternalUserID(id).
		SetIdentityType(service.IdentityTypeRoot).
		SetPasswordHash("integration-hash").
		SetRole(role).
		SetStatus(service.StatusActive).
		SetBalance(balance).
		Save(context.Background())
	require.NoError(t, err)
	return &service.User{ID: created.ID, Email: created.Email, AccountID: id, ExternalUserID: id, IdentityType: service.IdentityTypeRoot, Role: role, Status: service.StatusActive, Balance: balance}
}

func isolateOrganizationIntegrationTest(t *testing.T) {
	t.Helper()
	cleanup := func() {
		ctx := context.Background()
		_, err := integrationDB.ExecContext(ctx, `
			TRUNCATE organization_name_change_requests,member_policy_attachments,organization_memberships,
			organization_financial_ledger,organization_audit_events,company_upgrade_applications,organizations
			RESTART IDENTITY CASCADE`)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, `DELETE FROM notification_outbox WHERE event LIKE 'company.%'`)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, `DELETE FROM users WHERE email LIKE 'company-%@example.com'`)
		require.NoError(t, err)
	}
	cleanup()
	t.Cleanup(cleanup)
}

func createActiveOrganization(t *testing.T, owner *service.User, limit int) int64 {
	t.Helper()
	ctx := context.Background()
	var organizationID int64
	err := integrationDB.QueryRowContext(ctx, `INSERT INTO organizations(account_id,owner_user_id,name,normalized_name,status,member_limit,effective_at) VALUES($1,$2,$3,$4,'active',$5,NOW()) RETURNING id`, owner.AccountID, owner.ID, "Company "+owner.AccountID, "company "+owner.AccountID, limit).Scan(&organizationID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `INSERT INTO organization_memberships(organization_id,user_id,role,status) VALUES($1,$2,'owner','active')`, organizationID, owner.ID)
	require.NoError(t, err)
	return organizationID
}

func TestOrganizationConstraints_OneOwnerAndOneActiveAttachment(t *testing.T) {
	isolateOrganizationIntegrationTest(t)
	ctx := context.Background()
	owner := createOrganizationRoot(t, integrationEntClient, 100, service.RoleUser)
	second := createOrganizationRoot(t, integrationEntClient, 0, service.RoleUser)
	organizationID := createActiveOrganization(t, owner, 20)

	_, err := integrationDB.ExecContext(ctx, `INSERT INTO organization_memberships(organization_id,user_id,role,status) VALUES($1,$2,'owner','active')`, organizationID, second.ID)
	require.Error(t, err)

	memberID := createIAMMemberForOrganizationTest(t, owner.ID, "constraint-member")
	var membershipID, policyID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT id FROM organization_memberships WHERE user_id=$1`, memberID).Scan(&membershipID))
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT id FROM managed_policies WHERE policy_key=$1`, service.PolicyCompanyFinanceReadOnly).Scan(&policyID))
	_, err = integrationDB.ExecContext(ctx, `INSERT INTO member_policy_attachments(organization_id,membership_id,policy_id,policy_version,attached_by_user_id) VALUES($1,$2,$3,1,$4)`, organizationID, membershipID, policyID, owner.ID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `INSERT INTO member_policy_attachments(organization_id,membership_id,policy_id,policy_version,attached_by_user_id) VALUES($1,$2,$3,1,$4)`, organizationID, membershipID, policyID, owner.ID)
	require.Error(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE member_policy_attachments SET detached_at=NOW() WHERE membership_id=$1 AND policy_id=$2 AND detached_at IS NULL`, membershipID, policyID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `INSERT INTO member_policy_attachments(organization_id,membership_id,policy_id,policy_version,attached_by_user_id) VALUES($1,$2,$3,1,$4)`, organizationID, membershipID, policyID, owner.ID)
	require.NoError(t, err)
}

func TestCompanyUpgradeReservationDecisionsAndFrozenMismatch(t *testing.T) {
	isolateOrganizationIntegrationTest(t)
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "company-application-audit")
	repo := NewOrganizationRepository(integrationDB)
	admin := createOrganizationRoot(t, integrationEntClient, 0, service.RoleAdmin)

	withdrawRoot := createOrganizationRoot(t, integrationEntClient, 100, service.RoleUser)
	key := uuid.NewString()
	application, err := repo.SubmitApplication(ctx, withdrawRoot.ID, "Withdraw Company", "withdraw company", key, "20.00000000", "USD")
	require.NoError(t, err)
	replayed, err := repo.SubmitApplication(ctx, withdrawRoot.ID, "Withdraw Company", "withdraw company", key, "20.00000000", "USD")
	require.NoError(t, err)
	require.Equal(t, application.ID, replayed.ID)
	assertUserBalances(t, withdrawRoot.ID, "80.00000000", "20.00000000")

	otherRoot := createOrganizationRoot(t, integrationEntClient, 100, service.RoleUser)
	otherApplication, err := repo.SubmitApplication(ctx, otherRoot.ID, "Other Company", "other company", key, "20.00000000", "USD")
	require.NoError(t, err)
	require.NotEqual(t, application.ID, otherApplication.ID)
	require.Equal(t, otherRoot.ID, otherApplication.ApplicantUserID)
	assertUserBalances(t, otherRoot.ID, "80.00000000", "20.00000000")

	_, err = repo.WithdrawApplication(ctx, withdrawRoot.ID, application.ID)
	require.NoError(t, err)
	assertUserBalances(t, withdrawRoot.ID, "100.00000000", "0.00000000")

	approveRoot := createOrganizationRoot(t, integrationEntClient, 100, service.RoleUser)
	approved, err := repo.SubmitApplication(ctx, approveRoot.ID, "Approved Company", "approved company", uuid.NewString(), "20.00000000", "USD")
	require.NoError(t, err)
	approved, err = repo.DecideApplication(ctx, admin.ID, approved.ID, true, "", 20)
	require.NoError(t, err)
	require.Equal(t, "approved", approved.Status)
	require.NotNil(t, approved.OrganizationID)
	assertUserBalances(t, approveRoot.ID, "80.00000000", "0.00000000")
	var owners int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT count(*) FROM organization_memberships WHERE organization_id=$1 AND role='owner'`, *approved.OrganizationID).Scan(&owners))
	require.Equal(t, 1, owners)

	brokenRoot := createOrganizationRoot(t, integrationEntClient, 100, service.RoleUser)
	broken, err := repo.SubmitApplication(ctx, brokenRoot.ID, "Broken Frozen", "broken frozen", uuid.NewString(), "20.00000000", "USD")
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE users SET frozen_balance=0 WHERE id=$1`, brokenRoot.ID)
	require.NoError(t, err)
	_, err = repo.DecideApplication(ctx, admin.ID, broken.ID, false, "not approved", 20)
	require.ErrorIs(t, err, service.ErrInsufficientBalance)
	current, err := repo.GetApplicationForUser(ctx, brokenRoot.ID)
	require.NoError(t, err)
	require.Equal(t, "pending", current.Status)
	var correlatedAuditCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT count(*) FROM organization_audit_events WHERE correlation_id=$1 AND action IN ('company.application.submit','company.application.withdraw','company.application.review')`, "company-application-audit").Scan(&correlatedAuditCount))
	require.Equal(t, 6, correlatedAuditCount)
}

func TestCompanyUpgradeAcceptance_EnqueuesReviewAndOutcomeNotifications(t *testing.T) {
	isolateOrganizationIntegrationTest(t)
	ctx := context.Background()
	repo := NewOrganizationRepository(integrationDB)
	admin := createOrganizationRoot(t, integrationEntClient, 0, service.RoleAdmin)

	adminRows, err := integrationDB.QueryContext(ctx, `SELECT email FROM users WHERE role='admin' AND status='active' AND deleted_at IS NULL AND email IS NOT NULL ORDER BY id`)
	require.NoError(t, err)
	adminRecipients := make(map[string]int)
	for adminRows.Next() {
		var email string
		require.NoError(t, adminRows.Scan(&email))
		adminRecipients[email] = 3
	}
	require.NoError(t, adminRows.Err())
	require.NoError(t, adminRows.Close())
	require.Contains(t, adminRecipients, admin.Email)

	withdrawApplicant := createOrganizationRoot(t, integrationEntClient, 100, service.RoleUser)
	withdrawApplication, err := repo.SubmitApplication(ctx, withdrawApplicant.ID, "Withdraw Notification Company", "withdraw notification company", uuid.NewString(), "20.00000000", "USD")
	require.NoError(t, err)
	_, err = repo.WithdrawApplication(ctx, withdrawApplicant.ID, withdrawApplication.ID)
	require.NoError(t, err)

	approveApplicant := createOrganizationRoot(t, integrationEntClient, 100, service.RoleUser)
	approveApplication, err := repo.SubmitApplication(ctx, approveApplicant.ID, "Approve Notification Company", "approve notification company", uuid.NewString(), "20.00000000", "USD")
	require.NoError(t, err)
	_, err = repo.DecideApplication(ctx, admin.ID, approveApplication.ID, true, "", 20)
	require.NoError(t, err)

	rejectApplicant := createOrganizationRoot(t, integrationEntClient, 100, service.RoleUser)
	rejectApplication, err := repo.SubmitApplication(ctx, rejectApplicant.ID, "Reject Notification Company", "reject notification company", uuid.NewString(), "20.00000000", "USD")
	require.NoError(t, err)
	_, err = repo.DecideApplication(ctx, admin.ID, rejectApplication.ID, false, "not approved", 20)
	require.NoError(t, err)

	notificationRows, err := integrationDB.QueryContext(ctx, `
		SELECT event, recipient
		FROM notification_outbox
		WHERE event IN ($1, $2, $3, $4)
		ORDER BY id`,
		service.NotificationEmailEventCompanyUpgradeSubmitted,
		service.NotificationEmailEventCompanyUpgradeWithdrawn,
		service.NotificationEmailEventCompanyUpgradeApproved,
		service.NotificationEmailEventCompanyUpgradeRejected,
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, notificationRows.Close()) }()

	submittedRecipients := make(map[string]int)
	terminalRecipients := make(map[string]string)
	var notificationCount int
	for notificationRows.Next() {
		var event, recipient string
		require.NoError(t, notificationRows.Scan(&event, &recipient))
		notificationCount++
		if event == service.NotificationEmailEventCompanyUpgradeSubmitted {
			submittedRecipients[recipient]++
			continue
		}
		terminalRecipients[event] = recipient
	}
	require.NoError(t, notificationRows.Err())
	require.Equal(t, 3*len(adminRecipients)+3, notificationCount)
	require.Equal(t, adminRecipients, submittedRecipients)
	require.Equal(t, map[string]string{
		service.NotificationEmailEventCompanyUpgradeWithdrawn: withdrawApplicant.Email,
		service.NotificationEmailEventCompanyUpgradeApproved:  approveApplicant.Email,
		service.NotificationEmailEventCompanyUpgradeRejected:  rejectApplicant.Email,
	}, terminalRecipients)
}

func TestCompanyUpgradeSelfReviewAllowed(t *testing.T) {
	isolateOrganizationIntegrationTest(t)
	ctx := context.Background()
	repo := NewOrganizationRepository(integrationDB)
	applicant := createOrganizationRoot(t, integrationEntClient, 100, service.RoleAdmin)
	application, err := repo.SubmitApplication(ctx, applicant.ID, "Self Review", "self review", uuid.NewString(), "20.00000000", "USD")
	require.NoError(t, err)
	approved, err := repo.DecideApplication(ctx, applicant.ID, application.ID, true, "", 20)
	require.NoError(t, err)
	require.Equal(t, "approved", approved.Status)
	require.NotNil(t, approved.ReviewerUserID)
	require.Equal(t, applicant.ID, *approved.ReviewerUserID)
	assertUserBalances(t, applicant.ID, "80.00000000", "0.00000000")
}

func TestCompanyNameSelfReviewAllowed(t *testing.T) {
	isolateOrganizationIntegrationTest(t)
	ctx := context.Background()
	repo := NewOrganizationRepository(integrationDB)
	owner := createOrganizationRoot(t, integrationEntClient, 100, service.RoleAdmin)
	createActiveOrganization(t, owner, 20)

	require.NoError(t, repo.RequestNameChange(ctx, owner.ID, "Self Reviewed Name", "self reviewed name"))
	requests, total, err := repo.ListNameChangeRequests(ctx, "pending", 1, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.NoError(t, repo.DecideNameChange(ctx, owner.ID, requests[0].ID, true, ""))

	organization, err := repo.GetContextForUser(ctx, owner.ID)
	require.NoError(t, err)
	require.Equal(t, "Self Reviewed Name", organization.CompanyName)
}

func TestCompanyUpgradeRejectAndConcurrentDecisionAreSettledOnce(t *testing.T) {
	isolateOrganizationIntegrationTest(t)
	ctx := context.Background()
	repo := NewOrganizationRepository(integrationDB)
	firstAdmin := createOrganizationRoot(t, integrationEntClient, 0, service.RoleAdmin)
	secondAdmin := createOrganizationRoot(t, integrationEntClient, 0, service.RoleAdmin)

	rejectedRoot := createOrganizationRoot(t, integrationEntClient, 100, service.RoleUser)
	rejected, err := repo.SubmitApplication(ctx, rejectedRoot.ID, "Rejected Company", "rejected company", uuid.NewString(), "20.00000000", "USD")
	require.NoError(t, err)
	rejected, err = repo.DecideApplication(ctx, firstAdmin.ID, rejected.ID, false, "insufficient information", 20)
	require.NoError(t, err)
	require.Equal(t, "rejected", rejected.Status)
	require.Equal(t, "insufficient information", rejected.ReviewReason)
	assertUserBalances(t, rejectedRoot.ID, "100.00000000", "0.00000000")
	_, err = repo.DecideApplication(ctx, secondAdmin.ID, rejected.ID, true, "", 20)
	require.ErrorIs(t, err, service.ErrApplicationTerminal)

	racingRoot := createOrganizationRoot(t, integrationEntClient, 100, service.RoleUser)
	tracing, err := repo.SubmitApplication(ctx, racingRoot.ID, "Racing Company", "racing company", uuid.NewString(), "20.00000000", "USD")
	require.NoError(t, err)
	errCh := make(chan error, 2)
	go func() {
		_, decideErr := repo.DecideApplication(ctx, firstAdmin.ID, tracing.ID, true, "", 20)
		errCh <- decideErr
	}()
	go func() {
		_, decideErr := repo.DecideApplication(ctx, secondAdmin.ID, tracing.ID, false, "not selected", 20)
		errCh <- decideErr
	}()
	firstErr, secondErr := <-errCh, <-errCh
	require.True(t, (firstErr == nil) != (secondErr == nil), "exactly one concurrent decision must commit")
	terminalErr := firstErr
	if terminalErr == nil {
		terminalErr = secondErr
	}
	require.ErrorIs(t, terminalErr, service.ErrApplicationTerminal)

	var terminalLedgerCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT count(*) FROM organization_financial_ledger WHERE application_id=$1 AND kind IN ('upgrade_capture','upgrade_release')`, tracing.ID).Scan(&terminalLedgerCount))
	require.Equal(t, 1, terminalLedgerCount)
	current, err := repo.GetApplicationForUser(ctx, racingRoot.ID)
	require.NoError(t, err)
	require.Contains(t, []string{"approved", "rejected"}, current.Status)
	assertUserBalances(t, racingRoot.ID, map[string]string{"approved": "80.00000000", "rejected": "100.00000000"}[current.Status], "0.00000000")
}

func TestCompanyNameReviewAndOrganizationSuspensionLifecycle(t *testing.T) {
	isolateOrganizationIntegrationTest(t)
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "company-lifecycle-audit")
	repo := NewOrganizationRepository(integrationDB)
	owner := createOrganizationRoot(t, integrationEntClient, 100, service.RoleUser)
	organizationID := createActiveOrganization(t, owner, 20)
	memberID := createIAMMemberForOrganizationTest(t, owner.ID, "lifecycle-member")
	admin := createOrganizationRoot(t, integrationEntClient, 0, service.RoleAdmin)

	require.NoError(t, repo.RequestNameChange(ctx, owner.ID, "Approved New Name", "approved new name"))
	requests, total, err := repo.ListNameChangeRequests(ctx, "pending", 1, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.NoError(t, repo.DecideNameChange(ctx, admin.ID, requests[0].ID, true, ""))
	organization, err := repo.GetContextForUser(ctx, owner.ID)
	require.NoError(t, err)
	require.Equal(t, "Approved New Name", organization.CompanyName)

	require.NoError(t, repo.RequestNameChange(ctx, owner.ID, "Rejected New Name", "rejected new name"))
	requests, _, err = repo.ListNameChangeRequests(ctx, "pending", 1, 20)
	require.NoError(t, err)
	require.NoError(t, repo.DecideNameChange(ctx, admin.ID, requests[0].ID, false, "keep current name"))
	organization, err = repo.GetContextForUser(ctx, owner.ID)
	require.NoError(t, err)
	require.Equal(t, "Approved New Name", organization.CompanyName)

	require.NoError(t, repo.SetOrganizationStatus(ctx, admin.ID, organizationID, service.OrganizationStatusSuspended))
	_, err = repo.ResolveBillingContext(ctx, memberID)
	require.ErrorIs(t, err, service.ErrOrganizationPermission)
	memberContext, err := repo.GetContextForUser(ctx, memberID)
	require.NoError(t, err)
	require.False(t, memberContext.Active())

	require.NoError(t, repo.SetOrganizationStatus(ctx, admin.ID, organizationID, service.OrganizationStatusActive))
	resolved, err := repo.ResolveBillingContext(ctx, memberID)
	require.NoError(t, err)
	require.Equal(t, memberID, resolved.PayerUserID)

	var auditCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT count(*) FROM organization_audit_events WHERE organization_id=$1 AND correlation_id=$2 AND action IN ('company.name.review','organization.status')`, organizationID, "company-lifecycle-audit").Scan(&auditCount))
	require.Equal(t, 4, auditCount)
}

func createIAMMemberForOrganizationTest(t *testing.T, ownerID int64, login string) int64 {
	t.Helper()
	repo := NewOrganizationRepository(integrationDB)
	member, err := repo.CreateIAMMember(context.Background(), ownerID, &service.User{LoginName: login, PasswordHash: "hash", RecoveryEmail: ""}, 20)
	require.NoError(t, err)
	return member.UserID
}

func TestIAMMemberLimitConcurrentArchiveAndLoginReuse(t *testing.T) {
	isolateOrganizationIntegrationTest(t)
	ctx := context.Background()
	repo := NewOrganizationRepository(integrationDB)
	owner := createOrganizationRoot(t, integrationEntClient, 100, service.RoleUser)
	createActiveOrganization(t, owner, 20)
	memberIDs := make([]int64, 0, 20)
	for index := 0; index < 19; index++ {
		memberIDs = append(memberIDs, createIAMMemberForOrganizationTest(t, owner.ID, fmt.Sprintf("member-%02d", index)))
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, login := range []string{"twentieth-a", "twentieth-b"} {
		wg.Add(1)
		go func(login string) {
			defer wg.Done()
			_, err := repo.CreateIAMMember(ctx, owner.ID, &service.User{LoginName: login, PasswordHash: "hash"}, 20)
			errs <- err
		}(login)
	}
	wg.Wait()
	close(errs)
	successes, limitFailures := 0, 0
	for err := range errs {
		if err == nil {
			successes++
		} else if errors.Is(err, service.ErrIAMMemberLimit) {
			limitFailures++
		} else {
			require.NoError(t, err)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, limitFailures)

	require.NoError(t, repo.SetIAMMemberStatus(ctx, owner.ID, memberIDs[0], service.MembershipStatusArchived))
	replacement, err := repo.CreateIAMMember(ctx, owner.ID, &service.User{LoginName: "member-00", PasswordHash: "hash"}, 20)
	require.NoError(t, err)
	require.NotEqual(t, memberIDs[0], replacement.UserID)
	members, limit, err := repo.ListIAMMembers(ctx, owner.ID)
	require.NoError(t, err)
	require.Equal(t, 20, limit)
	counted := 0
	for _, member := range members {
		if member.Status != service.MembershipStatusArchived {
			counted++
		}
	}
	require.Equal(t, 20, counted)
}

func TestOrganizationAllocationPayerSelectionAndFinanceRedaction(t *testing.T) {
	isolateOrganizationIntegrationTest(t)
	ctx := context.Background()
	repo := NewOrganizationRepository(integrationDB)
	owner := createOrganizationRoot(t, integrationEntClient, 100, service.RoleUser)
	createActiveOrganization(t, owner, 20)
	memberID := createIAMMemberForOrganizationTest(t, owner.ID, "billing-member")

	transferKey := uuid.NewString()
	require.NoError(t, repo.TransferBalance(ctx, owner.ID, memberID, "25.00000000", transferKey, false))
	require.NoError(t, repo.TransferBalance(ctx, owner.ID, memberID, "25.00000000", transferKey, false))
	require.Error(t, repo.TransferBalance(ctx, owner.ID, memberID, "24.00000000", transferKey, false))
	assertUserBalances(t, owner.ID, "75.00000000", "0.00000000")
	assertUserBalances(t, memberID, "25.00000000", "0.00000000")
	assertCombinedAvailableBalance(t, owner.ID, memberID, "100.00000000")

	otherOwner := createOrganizationRoot(t, integrationEntClient, 100, service.RoleUser)
	createActiveOrganization(t, otherOwner, 20)
	otherMemberID := createIAMMemberForOrganizationTest(t, otherOwner.ID, "other-billing-member")
	require.NoError(t, repo.TransferBalance(ctx, otherOwner.ID, otherMemberID, "25.00000000", transferKey, false))
	require.ErrorIs(t, repo.SetPolicyAttachment(ctx, owner.ID, otherMemberID, service.PolicyCompanySharedBalance, true, uuid.NewString()), service.ErrIAMMemberNotFound)
	assertUserBalances(t, otherOwner.ID, "75.00000000", "0.00000000")
	assertUserBalances(t, otherMemberID, "25.00000000", "0.00000000")
	resolved, err := repo.ResolveBillingContext(ctx, memberID)
	require.NoError(t, err)
	require.Equal(t, memberID, resolved.PayerUserID)
	require.Equal(t, "allocated", resolved.BalanceSource)

	require.NoError(t, repo.SetPolicyAttachment(ctx, owner.ID, memberID, service.PolicyCompanySharedBalance, true, uuid.NewString()))
	resolved, err = repo.ResolveBillingContext(ctx, memberID)
	require.NoError(t, err)
	require.Equal(t, owner.ID, resolved.PayerUserID)
	require.Equal(t, "shared", resolved.BalanceSource)
	redacted, err := repo.FinanceSummary(ctx, memberID)
	require.NoError(t, err)
	require.Nil(t, redacted.Available)
	require.Nil(t, redacted.Total)

	require.NoError(t, repo.SetPolicyAttachment(ctx, owner.ID, memberID, service.PolicyCompanyFinanceReadOnly, true, uuid.NewString()))
	visible, err := repo.FinanceSummary(ctx, memberID)
	require.NoError(t, err)
	require.NotNil(t, visible.Available)
	require.Equal(t, "75.00000000", *visible.Available)

	require.NoError(t, repo.SetPolicyAttachment(ctx, owner.ID, memberID, service.PolicyCompanySharedBalance, false, uuid.NewString()))
	resolved, err = repo.ResolveBillingContext(ctx, memberID)
	require.NoError(t, err)
	require.Equal(t, memberID, resolved.PayerUserID)
	require.Equal(t, "allocated", resolved.BalanceSource)
	require.NoError(t, repo.TransferBalance(ctx, owner.ID, memberID, "10.00000000", uuid.NewString(), true))
	assertUserBalances(t, owner.ID, "85.00000000", "0.00000000")
	assertUserBalances(t, memberID, "15.00000000", "0.00000000")
	assertCombinedAvailableBalance(t, owner.ID, memberID, "100.00000000")
}

func TestOrganizationDomainAuditCoverageAndCorrelation(t *testing.T) {
	isolateOrganizationIntegrationTest(t)
	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "company-audit-correlation")
	repo := NewOrganizationRepository(integrationDB)
	owner := createOrganizationRoot(t, integrationEntClient, 100, service.RoleUser)
	organizationID := createActiveOrganization(t, owner, 20)
	memberID := createIAMMemberForOrganizationTest(t, owner.ID, "audit-member")
	otherOwner := createOrganizationRoot(t, integrationEntClient, 100, service.RoleUser)
	createActiveOrganization(t, otherOwner, 20)
	foreignMemberID := createIAMMemberForOrganizationTest(t, otherOwner.ID, "foreign-audit-member")

	require.NoError(t, repo.SetIAMMemberStatus(ctx, owner.ID, memberID, service.MembershipStatusDisabled))
	require.NoError(t, repo.SetIAMMemberStatus(ctx, owner.ID, memberID, service.MembershipStatusActive))
	require.NoError(t, repo.UpdateIAMPassword(ctx, owner.ID, memberID, "owner-reset-hash", true))
	require.NoError(t, repo.UpdateIAMPassword(ctx, memberID, memberID, "member-change-hash", false))
	require.NoError(t, repo.SetPolicyAttachment(ctx, owner.ID, memberID, service.PolicyCompanyFinanceReadOnly, true, ""))
	require.NoError(t, repo.SetPolicyAttachment(ctx, owner.ID, memberID, service.PolicyCompanyFinanceReadOnly, false, ""))
	require.NoError(t, repo.TransferBalance(ctx, owner.ID, memberID, "5.00000000", uuid.NewString(), false))
	require.NoError(t, repo.TransferBalance(ctx, owner.ID, memberID, "2.00000000", uuid.NewString(), true))

	require.ErrorIs(t, repo.SetIAMMemberStatus(ctx, owner.ID, foreignMemberID, service.MembershipStatusDisabled), service.ErrIAMMemberNotFound)
	require.ErrorIs(t, repo.UpdateIAMPassword(ctx, owner.ID, foreignMemberID, "denied-hash", true), service.ErrIAMMemberNotFound)
	require.ErrorIs(t, repo.SetPolicyAttachment(ctx, owner.ID, foreignMemberID, service.PolicyCompanySharedBalance, true, ""), service.ErrIAMMemberNotFound)
	require.ErrorIs(t, repo.TransferBalance(ctx, owner.ID, foreignMemberID, "1.00000000", uuid.NewString(), false), service.ErrIAMMemberNotFound)

	expected := map[string]int{
		"iam.member.status:success":             2,
		"iam.member.status:denied":              1,
		"iam.member.password.reset:success":     1,
		"iam.member.password.reset:denied":      1,
		"iam.member.password.change:success":    1,
		"iam.policy.change:success":             2,
		"iam.policy.change:denied":              1,
		"organization.balance.allocate:success": 1,
		"organization.balance.reclaim:success":  1,
		"organization.balance.transfer:denied":  1,
	}
	rows, err := integrationDB.QueryContext(ctx, `
		SELECT action || ':' || result, count(*)
		FROM organization_audit_events
		WHERE organization_id=$1 AND correlation_id=$2
		GROUP BY action,result`, organizationID, "company-audit-correlation")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	actual := make(map[string]int)
	for rows.Next() {
		var key string
		var count int
		require.NoError(t, rows.Scan(&key, &count))
		actual[key] = count
	}
	require.NoError(t, rows.Err())
	for key, count := range expected {
		require.Equalf(t, count, actual[key], "audit event %s", key)
	}
}

func TestOrganizationReconciliationDetectsViolations(t *testing.T) {
	isolateOrganizationIntegrationTest(t)
	ctx := context.Background()
	repo := NewOrganizationRepository(integrationDB)
	owner := createOrganizationRoot(t, integrationEntClient, 100, service.RoleUser)
	organizationID := createActiveOrganization(t, owner, 20)
	firstMemberID := createIAMMemberForOrganizationTest(t, owner.ID, "reconcile-first")
	_ = createIAMMemberForOrganizationTest(t, owner.ID, "reconcile-second")

	_, err := integrationDB.ExecContext(ctx, `
		INSERT INTO company_upgrade_applications(applicant_user_id,requested_name,normalized_name,status,fee_amount,fee_currency,idempotency_key)
		VALUES($1,'Reconcile Company','reconcile company','pending',20,'USD',$2)`, owner.ID, uuid.NewString())
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE users SET frozen_balance=0 WHERE id=$1`, owner.ID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE organizations SET member_limit=1 WHERE id=$1`, organizationID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `DELETE FROM organization_memberships WHERE organization_id=$1 AND role='owner'`, organizationID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `
		INSERT INTO organization_financial_ledger(idempotency_key,kind,organization_id,actor_user_id,source_user_id,destination_user_id,amount,currency,source_balance_after,destination_balance_after)
		VALUES($1,'allocate',$2,$3,$3,$3,1,'USD',99,99)`, uuid.NewString(), organizationID, owner.ID)
	require.NoError(t, err)

	account := mustCreateAccount(t, integrationEntClient, &service.Account{Name: "reconciliation-account"})
	apiKey := mustCreateApiKey(t, integrationEntClient, &service.APIKey{UserID: firstMemberID})
	requestID := "organization-reconciliation-" + uuid.NewString()
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_logs WHERE request_id=$1`, requestID)
	})
	_, err = integrationDB.ExecContext(ctx, `
		INSERT INTO usage_logs(user_id,organization_id,api_key_id,account_id,request_id,model,input_tokens,output_tokens,total_cost,actual_cost,created_at)
		VALUES($1,$2,$3,$4,$5,'gpt-reconciliation',1,1,0,0,NOW())`, firstMemberID, organizationID, apiKey.ID, account.ID, requestID)
	require.NoError(t, err)

	checks, err := repo.Reconcile(ctx)
	require.NoError(t, err)
	for _, key := range []string{
		"pending_reservation_mismatch",
		"pending_frozen_shortfall",
		"owner_cardinality_violation",
		"member_limit_violation",
		"transfer_conservation_violation",
		"missing_usage_payer_snapshots",
	} {
		require.Greaterf(t, checks[key], int64(0), "reconciliation check %s", key)
	}
	for _, key := range []string{"upgrade_settlement_mismatch", "missing_async_payer_snapshots", "missing_batch_payer_snapshots", "oldest_review_queue_age_seconds"} {
		_, ok := checks[key]
		require.Truef(t, ok, "reconciliation result includes %s", key)
	}
}

func assertUserBalances(t *testing.T, userID int64, available, frozen string) {
	t.Helper()
	var actualAvailable, actualFrozen string
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), `SELECT balance::text,frozen_balance::text FROM users WHERE id=$1`, userID).Scan(&actualAvailable, &actualFrozen))
	require.Equal(t, available, actualAvailable)
	require.Equal(t, frozen, actualFrozen)
}

func assertCombinedAvailableBalance(t *testing.T, firstUserID, secondUserID int64, expected string) {
	t.Helper()
	var actual string
	require.NoError(t, integrationDB.QueryRowContext(context.Background(), `SELECT SUM(balance)::text FROM users WHERE id IN ($1,$2)`, firstUserID, secondUserID).Scan(&actual))
	require.Equal(t, expected, actual)
}

func TestOrganizationUsageIndexSupportsScopedPlan(t *testing.T) {
	isolateOrganizationIntegrationTest(t)
	ctx := context.Background()
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, func() error {
		_, execErr := tx.ExecContext(ctx, `SET LOCAL enable_seqscan=off`)
		return execErr
	}())

	plan := explainText(t, tx, `EXPLAIN (FORMAT TEXT) SELECT id FROM usage_logs WHERE organization_id=$1 AND created_at >= $2 ORDER BY created_at DESC LIMIT 20`, int64(-1), time.Now().Add(-time.Hour))
	require.Contains(t, plan, "idx_usage_logs_organization_created_at")

	// Production may partition usage_logs by created_at. Verify that the same
	// organization/time index propagates to leaves and that an organization
	// query prunes an out-of-range partition.
	_, err = tx.ExecContext(ctx, `
		CREATE TEMP TABLE organization_usage_partition_probe (
			id bigint, organization_id bigint, created_at timestamptz NOT NULL
		) PARTITION BY RANGE (created_at);
		CREATE TEMP TABLE organization_usage_partition_probe_old PARTITION OF organization_usage_partition_probe
			FOR VALUES FROM ('2025-01-01') TO ('2026-01-01');
		CREATE TEMP TABLE organization_usage_partition_probe_new PARTITION OF organization_usage_partition_probe
			FOR VALUES FROM ('2026-01-01') TO ('2027-01-01');
		CREATE INDEX organization_usage_partition_probe_scope_idx
			ON organization_usage_partition_probe(organization_id, created_at)
			WHERE organization_id IS NOT NULL;
		INSERT INTO organization_usage_partition_probe VALUES
			(1, 11, '2025-06-01'), (2, 11, '2026-06-01');
	`)
	require.NoError(t, err)
	partitionPlan := explainText(t, tx, `EXPLAIN (FORMAT TEXT) SELECT id FROM organization_usage_partition_probe WHERE organization_id=$1 AND created_at >= $2 AND created_at < $3`, int64(11), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	require.Contains(t, partitionPlan, "organization_usage_partition_probe_new")
	require.NotContains(t, partitionPlan, "organization_usage_partition_probe_old")
	require.Contains(t, partitionPlan, "organization_id")
}

func explainText(t *testing.T, tx *sql.Tx, query string, args ...any) string {
	t.Helper()
	rows, err := tx.QueryContext(context.Background(), query, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var lines []string
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		lines = append(lines, line)
	}
	require.NoError(t, rows.Err())
	return strings.Join(lines, "\n")
}

func TestOrganizationUsageFiltersCannotCrossOrganizationAndHistoricalNullsRemainReadable(t *testing.T) {
	isolateOrganizationIntegrationTest(t)
	ctx := context.Background()
	repo := NewOrganizationRepository(integrationDB)
	firstOwner := createOrganizationRoot(t, integrationEntClient, 100, service.RoleUser)
	firstOrganizationID := createActiveOrganization(t, firstOwner, 20)
	firstMemberID := createIAMMemberForOrganizationTest(t, firstOwner.ID, "usage-first")
	secondOwner := createOrganizationRoot(t, integrationEntClient, 100, service.RoleUser)
	secondOrganizationID := createActiveOrganization(t, secondOwner, 20)
	secondMemberID := createIAMMemberForOrganizationTest(t, secondOwner.ID, "usage-second")
	account := mustCreateAccount(t, integrationEntClient, &service.Account{Name: "organization-usage-account"})
	firstKey := mustCreateApiKey(t, integrationEntClient, &service.APIKey{UserID: firstMemberID})
	secondKey := mustCreateApiKey(t, integrationEntClient, &service.APIKey{UserID: secondMemberID})
	prefix := uuid.NewString()
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM usage_logs WHERE request_id LIKE $1`, prefix+"%")
	})

	insertUsage := func(userID, organizationID, payerID, apiKeyID int64, suffix string) int64 {
		var id int64
		err := integrationDB.QueryRowContext(ctx, `
			INSERT INTO usage_logs(user_id,organization_id,payer_user_id,balance_source,authz_generation,api_key_id,account_id,request_id,model,input_tokens,output_tokens,total_cost,actual_cost,billing_status,created_at)
			VALUES($1,$2,$3,'allocated',1,$4,$5,$6,'gpt-company-test',10,5,1,1,'charged',NOW()) RETURNING id`,
			userID, organizationID, payerID, apiKeyID, account.ID, prefix+suffix).Scan(&id)
		require.NoError(t, err)
		return id
	}
	insertUsage(firstMemberID, firstOrganizationID, firstMemberID, firstKey.ID, "-first")
	insertUsage(secondMemberID, secondOrganizationID, secondMemberID, secondKey.ID, "-second")

	rows, total, err := repo.ListUsage(ctx, firstOwner.ID, service.OrganizationUsageFilter{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, rows, 1)
	require.Equal(t, firstMemberID, rows[0].MemberUserID)

	foreignFilter := service.OrganizationUsageFilter{MemberID: &secondMemberID, Page: 1, PageSize: 20}
	rows, total, err = repo.ListUsage(ctx, firstOwner.ID, foreignFilter)
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, rows)
	stats, err := repo.UsageStats(ctx, firstOwner.ID, foreignFilter)
	require.NoError(t, err)
	require.Zero(t, stats.Requests)
	trend, err := repo.UsageTrend(ctx, firstOwner.ID, foreignFilter)
	require.NoError(t, err)
	require.Empty(t, trend)

	var historicalID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
		INSERT INTO usage_logs(user_id,api_key_id,account_id,request_id,model,input_tokens,output_tokens,total_cost,actual_cost,created_at)
		VALUES($1,$2,$3,$4,'gpt-historical-test',1,1,0,0,NOW()) RETURNING id`,
		firstMemberID, firstKey.ID, account.ID, prefix+"-historical").Scan(&historicalID))
	historical, err := NewUsageLogRepository(integrationEntClient, integrationDB).GetByID(ctx, historicalID)
	require.NoError(t, err)
	require.Nil(t, historical.OrganizationID)
	require.Nil(t, historical.PayerUserID)
}
