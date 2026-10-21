package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/authidentity"
	"github.com/Wei-Shaw/sub2api/ent/user"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/robfig/cron/v3"
)

const dingTalkDailySyncSchedule = "23 3 * * *"

func ProvideDingTalkOrganizationHandler(db *sql.DB, settings *service.SettingService, users *service.UserService, auth *AuthHandler, apiKeys *service.APIKeyService) *DingTalkOrganizationHandler {
	h := NewDingTalkOrganizationHandler(db, settings, users, auth, apiKeys)
	h.StartDailySync()
	return h
}

// StartDailySync uses the server timezone (Asia/Shanghai by default). Each app's
// existing database lease also prevents overlap with manual or other-node syncs.
func (h *DingTalkOrganizationHandler) StartDailySync() {
	ctx, cancel := context.WithCancel(context.Background())
	h.dailyCancel = cancel
	h.dailySync = cron.New(cron.WithLocation(timezone.Location()), cron.WithChain(cron.Recover(cron.DefaultLogger), cron.SkipIfStillRunning(cron.DefaultLogger)))
	if _, err := h.dailySync.AddFunc(dingTalkDailySyncSchedule, func() { h.startDailyOrganizationSync(ctx) }); err != nil {
		cancel()
		slog.Error("dingtalk: cannot schedule daily organization sync", "error", err)
		return
	}
	h.dailySync.Start()
}

func (h *DingTalkOrganizationHandler) StopDailySync() {
	if h == nil {
		return
	}
	if h.dailyCancel != nil {
		h.dailyCancel()
	}
	if h.dailySync != nil {
		<-h.dailySync.Stop().Done()
	}
	if h.organization != nil {
		h.organization.Stop()
	}
}

func (h *DingTalkOrganizationHandler) startDailyOrganizationSync(ctx context.Context) {
	apps, err := h.settings.GetDingTalkApps(ctx)
	if err != nil {
		slog.Error("dingtalk: daily sync cannot load applications", "error", err)
		return
	}
	ids := []string{"default"}
	for _, app := range apps {
		if app.Enabled {
			ids = append(ids, app.ID)
		}
	}
	for _, app := range ids {
		if ctx.Err() != nil {
			return
		}
		cfg, err := h.settings.GetDingTalkOAuthConfigForApp(ctx, app)
		if err != nil {
			// An unconfigured/disabled default app is normal in multi-app deployments.
			if app != "default" || infraerrors.Reason(err) != "OAUTH_DISABLED" {
				slog.Warn("dingtalk: daily sync application unavailable", "app", app, "error", err)
			}
			continue
		}
		_, err = h.organization.StartSync(ctx, app, h.auth.dingTalkClient(cfg).ReadOrganization)
		if err != nil {
			slog.Error("dingtalk: daily organization sync failed to start", "app", app, "error", err)
		}
	}
}

// Missing entries in a snapshot may reflect directory visibility changes. Confirm
// absence through the staff API before disabling credentials, and collect the full
// result first so a temporary upstream failure never applies a partial revocation.
func (h *DingTalkOrganizationHandler) reconcileDepartedMembers(ctx context.Context, app string, members []service.DingTalkDirectoryMember) error {
	cfg, err := h.settings.GetDingTalkOAuthConfigForApp(ctx, app)
	if err != nil {
		return err
	}
	client := h.auth.entClient()
	identities, err := client.AuthIdentity.Query().Where(authidentity.ProviderTypeEQ("dingtalk"), authidentity.ProviderKeyEQ(service.DingTalkProviderKey(app))).All(ctx)
	if err != nil {
		return err
	}
	current := make(map[string]bool, len(members))
	for _, member := range members {
		if member.UnionID != "" {
			current[member.UnionID] = true
		}
	}
	departed := make(map[int64]bool)
	for _, identity := range identities {
		if current[identity.ProviderSubject] {
			continue
		}
		_, err := h.auth.dingTalkClient(cfg).GetCurrentStaff(ctx, identity.ProviderSubject)
		if err == nil {
			continue
		}
		if !isDingTalkMembershipLost(err) {
			return fmt.Errorf("verify DingTalk membership: %w", err)
		}
		departed[identity.UserID] = true
	}
	if len(departed) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(departed))
	for id := range departed {
		ids = append(ids, id)
	}
	return h.disableDepartedUsers(ctx, ids)
}

var errDingTalkAccountDeparted = infraerrors.Forbidden("DINGTALK_ACCOUNT_DEPARTED", "用户已离职或不在钉钉企业通讯录中，账户访问权限已禁用")
var errDingTalkMembershipUnavailable = infraerrors.ServiceUnavailable("DINGTALK_MEMBERSHIP_UNAVAILABLE", "暂时无法确认钉钉在职状态，请稍后重试")

// validateUsageMembership is called only for API-key balance queries. Ordinary
// inference requests keep using local/database/cache authentication.
func (h *DingTalkOrganizationHandler) validateUsageMembership(ctx context.Context, userID int64) error {
	identities, err := h.auth.entClient().AuthIdentity.Query().Where(authidentity.UserIDEQ(userID), authidentity.ProviderTypeEQ("dingtalk")).All(ctx)
	if err != nil {
		return errDingTalkMembershipUnavailable.WithCause(err)
	}
	var unavailable error
	for _, identity := range identities {
		app := strings.TrimPrefix(identity.ProviderKey, "dingtalk:")
		if identity.ProviderKey == "dingtalk" {
			app = "default"
		}
		cfg, err := h.settings.GetDingTalkOAuthConfigForApp(ctx, app)
		if err != nil {
			unavailable = err
			continue
		}
		_, err = h.auth.dingTalkClient(cfg).GetCurrentStaff(ctx, identity.ProviderSubject)
		if err == nil {
			continue
		}
		if !isDingTalkMembershipLost(err) {
			unavailable = err
			continue
		}
		// Once absence is confirmed, finish revocation even if the client disconnects.
		revokeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		disableErr := h.disableDepartedUsers(revokeCtx, []int64{userID})
		cancel()
		if disableErr != nil {
			return errDingTalkMembershipUnavailable.WithCause(disableErr)
		}
		return errDingTalkAccountDeparted
	}
	if unavailable != nil {
		return errDingTalkMembershipUnavailable.WithCause(unavailable)
	}
	return nil
}

func (h *DingTalkOrganizationHandler) disableDepartedUsers(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := h.auth.entClient().Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.User.Update().Where(user.IDIn(ids...), user.DeletedAtIsNil()).SetStatus(service.StatusDisabled).Save(ctx); err != nil {
		return err
	}
	count, err := tx.APIKey.Update().Where(apikey.UserIDIn(ids...), apikey.DeletedAtIsNil(), apikey.StatusNEQ(service.StatusAPIKeyDisabled)).SetStatus(service.StatusAPIKeyDisabled).Save(ctx)
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	for _, id := range ids {
		h.apiKeys.InvalidateAuthCacheByUserID(ctx, id)
		// JWT and refresh authentication also read the disabled user status, so
		// failure to remove refresh sessions cannot restore account access.
		if err := h.auth.authService.RevokeAllUserSessions(ctx, id); err != nil {
			slog.Error("dingtalk: failed to remove disabled user's refresh sessions", "user_id", id, "error", err)
		}
	}
	slog.Info("dingtalk: disabled departed employees' accounts and API keys", "users", len(ids), "keys", count)
	return nil
}

func isDingTalkMembershipLost(err error) bool {
	var upstream *DingTalkAPIError
	if !errors.As(err, &upstream) {
		return false
	}
	return upstream.Code == "60121" || upstream.Code == "60111" || upstream.Code == "MEMBERSHIP_REQUIRED"
}
