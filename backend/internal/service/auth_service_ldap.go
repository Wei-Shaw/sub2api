package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/go-ldap/ldap/v3"
	"golang.org/x/crypto/bcrypt"
)

const ldapSyntheticEmailDomain = "ldap.local"

// LDAPUserRepository is an optional extension implemented by userRepository.
type LDAPUserRepository interface {
	GetByLDAPUID(ctx context.Context, ldapUID string) (*User, error)
	GetLDAPProfileByUserID(ctx context.Context, userID int64) (*LDAPUserProfile, error)
	UpsertLDAPProfile(ctx context.Context, profile *LDAPUserProfile) error
	ListActiveLDAPSyncTargets(ctx context.Context) ([]LDAPSyncTarget, error)
	DisableUser(ctx context.Context, userID int64) error
}

// LDAPSyncResult summarizes a sync run.
type LDAPSyncResult struct {
	Checked  int `json:"checked"`
	Disabled int `json:"disabled"`
	Updated  int `json:"updated"`
}

// LDAPProvider implements ExternalAuthProvider for LDAP/AD integration.
type LDAPProvider struct {
	userRepo          UserRepository
	ldapUserRepo      LDAPUserRepository
	settingService    *SettingService
	cfg               *config.Config
	refreshTokenCache RefreshTokenCache

	syncMu sync.Mutex
	stopCh chan struct{}
}

// NewLDAPProvider creates a new LDAP provider.
func NewLDAPProvider(
	userRepo UserRepository,
	ldapUserRepo LDAPUserRepository,
	settingService *SettingService,
	cfg *config.Config,
	refreshTokenCache RefreshTokenCache,
) *LDAPProvider {
	return &LDAPProvider{
		userRepo:          userRepo,
		ldapUserRepo:      ldapUserRepo,
		settingService:    settingService,
		cfg:               cfg,
		refreshTokenCache: refreshTokenCache,
		stopCh:            make(chan struct{}),
	}
}

// Ensure interface compliance
var _ ExternalAuthProvider = (*LDAPProvider)(nil)

func (p *LDAPProvider) Start() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := p.runLDAPSyncIfDue(context.Background()); err != nil {
					log.Printf("[LDAP] periodic sync failed: %v", err)
				}
			case <-p.stopCh:
				return
			}
		}
	}()
}

func (p *LDAPProvider) Stop() {
	close(p.stopCh)
}

func (p *LDAPProvider) Login(ctx context.Context, identifier, password string) (*User, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" || strings.TrimSpace(password) == "" {
		return nil, ErrInvalidCredentials
	}

	cfg, err := p.settingService.GetLDAPConfig(ctx)
	if err != nil {
		return nil, ErrServiceUnavailable
	}
	if !cfg.Enabled {
		return nil, ErrInvalidCredentials
	}

	identity, err := p.authenticateLDAPUser(ctx, cfg, identifier, password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrUserNotActive) {
			return nil, err
		}
		if code := infraerrors.Code(err); code >= 400 && code < 500 {
			return nil, err
		}
		return nil, ErrServiceUnavailable
	}

	user, err := p.upsertLDAPUser(ctx, identity, cfg)
	if err != nil {
		return nil, err
	}
	if !user.IsActive() {
		return nil, ErrUserNotActive
	}

	go func() {
		if syncErr := p.runLDAPSyncIfDue(context.Background()); syncErr != nil {
			log.Printf("[LDAP] login-triggered sync failed: %v", syncErr)
		}
	}()

	return user, nil
}

func (p *LDAPProvider) authenticateLDAPUser(ctx context.Context, cfg *LDAPConfig, identifier, password string) (*LDAPIdentity, error) {
	conn, err := p.openLDAPConnection(cfg)
	if err != nil {
		log.Printf("[LDAP] connection failed: %v", err)
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	entry, err := p.searchLDAPUserForLogin(ctx, conn, cfg, identifier)
	if err != nil {
		log.Printf("[LDAP] user search failed identifier=%s: %v", identifier, err)
		if errors.Is(err, ErrUserNotFound) || errors.Is(err, ErrInvalidCredentials) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	identity := p.entryToLDAPIdentity(cfg, entry, identifier)
	if identity.Disabled {
		log.Printf("[LDAP] user is disabled in LDAP: %s", identity.UID)
		return nil, ErrUserNotActive
	}
	if !isLDAPUserAllowed(identity.GroupDNs, cfg.AllowedGroupDNs) {
		log.Printf("[LDAP] user %s (groups: %v) is not in allowed groups: %v", identity.UID, identity.GroupDNs, cfg.AllowedGroupDNs)
		return nil, infraerrors.Forbidden("LDAP_GROUP_NOT_ALLOWED", "ldap user is not in allowed groups")
	}

	if err := conn.Bind(entry.DN, password); err != nil {
		log.Printf("[LDAP] user bind failed dn=%s: %v", entry.DN, err)
		if isLDAPInvalidCredentials(err) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	return identity, nil
}

func (p *LDAPProvider) openLDAPConnection(cfg *LDAPConfig) (*ldap.Conn, error) {
	if cfg == nil || strings.TrimSpace(cfg.Host) == "" || cfg.Port <= 0 {
		return nil, infraerrors.BadRequest("LDAP_CONFIG_INVALID", "ldap host/port is not configured")
	}

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	tlsConfig := &tlsConfigWithServerName{
		InsecureSkipVerify: cfg.InsecureSkipVerify,
		ServerName:         cfg.Host,
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	var (
		conn *ldap.Conn
		err  error
	)
	if cfg.UseTLS {
		conn, err = ldap.DialURL("ldaps://"+addr, ldap.DialWithDialer(dialer), ldap.DialWithTLSConfig(tlsConfig.ToTLSConfig()))
	} else {
		conn, err = ldap.DialURL("ldap://"+addr, ldap.DialWithDialer(dialer))
	}
	if err != nil {
		return nil, err
	}

	if cfg.StartTLS {
		if err := conn.StartTLS(tlsConfig.ToTLSConfig()); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}

	if strings.TrimSpace(cfg.BindDN) != "" {
		if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
			log.Printf("[LDAP] manager bind failed: bind_dn=%s, err=%v", cfg.BindDN, err)
			_ = conn.Close()
			if isLDAPInvalidCredentials(err) {
				return nil, ErrInvalidCredentials
			}
			return nil, err
		}
	}
	return conn, nil
}

func (p *LDAPProvider) searchLDAPUserForLogin(_ context.Context, conn *ldap.Conn, cfg *LDAPConfig, identifier string) (*ldap.Entry, error) {
	filter := buildLDAPUserFilter(cfg, identifier)
	log.Printf("[LDAP] searching user with filter: %s", filter)
	attrs := uniqueLDAPAttrs([]string{
		cfg.UIDAttr,
		cfg.LoginAttr,
		cfg.EmailAttr,
		cfg.DisplayNameAttr,
		cfg.DepartmentAttr,
		cfg.GroupAttr,
		"userAccountControl",
	})

	req := ldap.NewSearchRequest(
		cfg.UserBaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		2,
		10,
		false,
		filter,
		attrs,
		nil,
	)
	resp, err := conn.Search(req)
	if err != nil {
		return nil, err
	}
	if len(resp.Entries) == 0 {
		return nil, ErrUserNotFound
	}
	if len(resp.Entries) > 1 {
		return nil, ErrInvalidCredentials
	}
	return resp.Entries[0], nil
}

func (p *LDAPProvider) entryToLDAPIdentity(cfg *LDAPConfig, entry *ldap.Entry, fallbackIdentifier string) *LDAPIdentity {
	uid := firstLDAPAttr(entry, cfg.UIDAttr)
	if uid == "" {
		uid = entry.DN
	}
	username := firstLDAPAttr(entry, cfg.LoginAttr)
	if username == "" {
		username = fallbackIdentifier
	}
	email := firstLDAPAttr(entry, cfg.EmailAttr)
	displayName := firstLDAPAttr(entry, cfg.DisplayNameAttr)
	department := firstLDAPAttr(entry, cfg.DepartmentAttr)
	groups := allLDAPAttrs(entry, cfg.GroupAttr)
	if displayName == "" {
		displayName = username
	}
	return &LDAPIdentity{
		UID:         uid,
		Username:    username,
		Email:       email,
		DisplayName: displayName,
		Department:  department,
		GroupDNs:    groups,
		Disabled:    ldapEntryIsDisabled(entry),
	}
}

func (p *LDAPProvider) upsertLDAPUser(ctx context.Context, identity *LDAPIdentity, cfg *LDAPConfig) (*User, error) {
	if identity == nil {
		return nil, ErrInvalidCredentials
	}

	var (
		user *User
		err  error
	)
	if strings.TrimSpace(identity.UID) != "" {
		user, err = p.ldapUserRepo.GetByLDAPUID(ctx, identity.UID)
		if err != nil && !errors.Is(err, ErrUserNotFound) {
			return nil, ErrServiceUnavailable
		}
	}
	if user == nil && strings.TrimSpace(identity.Email) != "" {
		user, err = p.userRepo.GetByEmail(ctx, identity.Email)
		if err != nil && !errors.Is(err, ErrUserNotFound) {
			return nil, ErrServiceUnavailable
		}
	}

	selectedMapping := pickLDAPMapping(identity.GroupDNs, cfg.GroupMappings)
	if user == nil {
		buf := make([]byte, 16)
		_, _ = rand.Read(buf)
		randomPassword := hex.EncodeToString(buf)
		
		hashedBytes, hashErr := bcrypt.GenerateFromPassword([]byte(randomPassword), bcrypt.DefaultCost)
		if hashErr != nil {
			return nil, ErrServiceUnavailable
		}
		hashedPassword := string(hashedBytes)

		balance := p.cfg.Default.UserBalance
		concurrency := p.cfg.Default.UserConcurrency
		if p.settingService != nil {
			balance = p.settingService.GetDefaultBalance(ctx)
			concurrency = p.settingService.GetDefaultConcurrency(ctx)
		}
		if selectedMapping != nil {
			if selectedMapping.Balance > 0 {
				balance = selectedMapping.Balance
			}
			if selectedMapping.Concurrency > 0 {
				concurrency = selectedMapping.Concurrency
			}
		}

		user = &User{
			Email:        ensureLDAPEmail(identity),
			Username:     identity.DisplayName,
			PasswordHash: hashedPassword,
			Role:         RoleUser,
			Balance:      balance,
			Concurrency:  concurrency,
			Status:       StatusActive,
			AuthSource:   "ldap",
		}
		if err := p.userRepo.Create(ctx, user); err != nil {
			if errors.Is(err, ErrEmailExists) {
				user, err = p.userRepo.GetByEmail(ctx, ensureLDAPEmail(identity))
				if err != nil {
					return nil, ErrServiceUnavailable
				}
			} else {
				return nil, ErrServiceUnavailable
			}
		}
	} else {
		if user.IsAdmin() && normalizeUserAuthSource(user.AuthSource) != "ldap" {
			return nil, infraerrors.Forbidden("LDAP_ADMIN_FORBIDDEN", "admin account must remain local")
		}

		user.AuthSource = "ldap"
		user.Status = StatusActive
		if strings.TrimSpace(identity.DisplayName) != "" {
			user.Username = identity.DisplayName
		}
		if strings.TrimSpace(identity.Email) != "" {
			if strings.EqualFold(identity.Email, user.Email) {
				user.Email = identity.Email
			} else {
				exists, existsErr := p.userRepo.ExistsByEmail(ctx, identity.Email)
				if existsErr == nil && !exists {
					user.Email = identity.Email
				}
			}
		}
		if !user.IsAdmin() {
			user.Role = RoleUser
			if selectedMapping != nil {
				if selectedMapping.Concurrency > 0 && user.Concurrency != selectedMapping.Concurrency {
					user.Concurrency = selectedMapping.Concurrency
				}
				if selectedMapping.Balance > 0 && user.Balance != selectedMapping.Balance {
					user.Balance = selectedMapping.Balance
				}
			}
		}

		if err := p.userRepo.Update(ctx, user); err != nil {
			return nil, ErrServiceUnavailable
		}
	}

	if err := p.ldapUserRepo.UpsertLDAPProfile(ctx, &LDAPUserProfile{
		UserID:       user.ID,
		LDAPUID:      identity.UID,
		LDAPUsername: identity.Username,
		LDAPEmail:    identity.Email,
		DisplayName:  identity.DisplayName,
		Department:   identity.Department,
		GroupsHash:   hashLDAPGroups(identity.GroupDNs),
		Active:       true,
		LastSyncedAt: time.Now().UTC(),
	}); err != nil {
		return nil, ErrServiceUnavailable
	}

	freshUser, err := p.userRepo.GetByID(ctx, user.ID)
	if err != nil {
		return nil, ErrServiceUnavailable
	}
	return freshUser, nil
}

func (p *LDAPProvider) runLDAPSyncIfDue(ctx context.Context) error {
	cfg, err := p.settingService.GetLDAPConfig(ctx)
	if err != nil {
		return err
	}
	if !cfg.Enabled || !cfg.SyncEnabled {
		return nil
	}
	lastSyncAt := p.settingService.GetLDAPLastSyncAt(ctx)
	if !lastSyncAt.IsZero() && time.Since(lastSyncAt) < time.Duration(cfg.SyncIntervalMins)*time.Minute {
		return nil
	}
	_, err = p.SyncNow(ctx)
	return err
}

func (p *LDAPProvider) SyncNow(ctx context.Context) (*LDAPSyncResult, error) {
	p.syncMu.Lock()
	defer p.syncMu.Unlock()

	cfg, err := p.settingService.GetLDAPConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return &LDAPSyncResult{}, nil
	}

	conn, err := p.openLDAPConnection(cfg)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	targets, err := p.ldapUserRepo.ListActiveLDAPSyncTargets(ctx)
	if err != nil {
		return nil, err
	}

	result := &LDAPSyncResult{}
	for _, target := range targets {
		result.Checked++
		identity, lookupErr := p.lookupLDAPIdentityForSync(ctx, conn, cfg, target)
		if lookupErr != nil || identity == nil || identity.Disabled || !isLDAPUserAllowed(identity.GroupDNs, cfg.AllowedGroupDNs) {
			if disableErr := p.ldapUserRepo.DisableUser(ctx, target.UserID); disableErr != nil {
				log.Printf("[LDAP] disable user failed user_id=%d err=%v", target.UserID, disableErr)
				continue
			}
			if p.refreshTokenCache != nil {
				_ = p.refreshTokenCache.DeleteUserRefreshTokens(ctx, target.UserID)
			}
			result.Disabled++
			continue
		}

		user, getErr := p.userRepo.GetByID(ctx, target.UserID)
		if getErr != nil {
			continue
		}
		selectedMapping := pickLDAPMapping(identity.GroupDNs, cfg.GroupMappings)

		changed := false
		if strings.TrimSpace(identity.DisplayName) != "" && user.Username != identity.DisplayName {
			user.Username = identity.DisplayName
			changed = true
		}
		if strings.TrimSpace(identity.Email) != "" {
			if strings.EqualFold(user.Email, identity.Email) {
				if user.Email != identity.Email {
					user.Email = identity.Email
					changed = true
				}
			} else {
				exists, existsErr := p.userRepo.ExistsByEmail(ctx, identity.Email)
				if existsErr == nil && !exists {
					user.Email = identity.Email
					changed = true
				}
			}
		}
		if !user.IsAdmin() && user.Role != RoleUser {
			user.Role = RoleUser
			changed = true
		}
		if !user.IsAdmin() && selectedMapping != nil {
			if selectedMapping.Concurrency > 0 && user.Concurrency != selectedMapping.Concurrency {
				user.Concurrency = selectedMapping.Concurrency
				changed = true
			}
			if selectedMapping.Balance > 0 && user.Balance != selectedMapping.Balance {
				user.Balance = selectedMapping.Balance
				changed = true
			}
		}
		if user.Status != StatusActive {
			user.Status = StatusActive
			changed = true
		}
		if normalizeUserAuthSource(user.AuthSource) != "ldap" {
			user.AuthSource = "ldap"
			changed = true
		}
		if changed {
			if updateErr := p.userRepo.Update(ctx, user); updateErr == nil {
				result.Updated++
			}
		}
		_ = p.ldapUserRepo.UpsertLDAPProfile(ctx, &LDAPUserProfile{
			UserID:       target.UserID,
			LDAPUID:      identity.UID,
			LDAPUsername: identity.Username,
			LDAPEmail:    identity.Email,
			DisplayName:  identity.DisplayName,
			Department:   identity.Department,
			GroupsHash:   hashLDAPGroups(identity.GroupDNs),
			Active:       true,
			LastSyncedAt: time.Now().UTC(),
		})
	}

	_ = p.settingService.SetLDAPLastSyncAt(ctx, time.Now().UTC())
	return result, nil
}

func (p *LDAPProvider) TestConnection(ctx context.Context) error {
	cfg, err := p.settingService.GetLDAPConfig(ctx)
	if err != nil {
		return err
	}
	conn, err := p.openLDAPConnection(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	return nil
}

func (p *LDAPProvider) lookupLDAPIdentityForSync(_ context.Context, conn *ldap.Conn, cfg *LDAPConfig, target LDAPSyncTarget) (*LDAPIdentity, error) {
	filters := make([]string, 0, 2)
	if strings.TrimSpace(target.LDAPUID) != "" {
		filters = append(filters, fmt.Sprintf("(%s=%s)", cfg.UIDAttr, ldap.EscapeFilter(target.LDAPUID)))
	}
	if strings.TrimSpace(target.LDAPUsername) != "" {
		filters = append(filters, fmt.Sprintf("(%s=%s)", cfg.LoginAttr, ldap.EscapeFilter(target.LDAPUsername)))
	}
	if len(filters) == 0 {
		return nil, ErrUserNotFound
	}

	attrs := uniqueLDAPAttrs([]string{
		cfg.UIDAttr,
		cfg.LoginAttr,
		cfg.EmailAttr,
		cfg.DisplayNameAttr,
		cfg.DepartmentAttr,
		cfg.GroupAttr,
		"userAccountControl",
	})

	for _, f := range filters {
		req := ldap.NewSearchRequest(
			cfg.UserBaseDN,
			ldap.ScopeWholeSubtree,
			ldap.NeverDerefAliases,
			1,
			10,
			false,
			f,
			attrs,
			nil,
		)
		resp, err := conn.Search(req)
		if err != nil {
			return nil, err
		}
		if len(resp.Entries) == 0 {
			continue
		}
		return p.entryToLDAPIdentity(cfg, resp.Entries[0], target.LDAPUsername), nil
	}
	return nil, ErrUserNotFound
}

func ensureLDAPEmail(identity *LDAPIdentity) string {
	if identity != nil {
		email := strings.TrimSpace(identity.Email)
		if email != "" {
			return email
		}
		login := strings.TrimSpace(identity.Username)
		if login == "" {
			login = strings.TrimSpace(identity.UID)
		}
		if login != "" {
			login = strings.ReplaceAll(strings.ToLower(login), " ", ".")
			login = strings.ReplaceAll(login, "@", "_")
			return login + "@" + ldapSyntheticEmailDomain
		}
	}
	return fmt.Sprintf("user_%d@%s", time.Now().UnixNano(), ldapSyntheticEmailDomain)
}

func pickLDAPMapping(groupDNs []string, mappings []LDAPGroupMapping) *LDAPGroupMapping {
	if len(groupDNs) == 0 || len(mappings) == 0 {
		return nil
	}
	groupSet := make(map[string]struct{}, len(groupDNs))
	for _, dn := range groupDNs {
		groupSet[strings.ToLower(strings.TrimSpace(dn))] = struct{}{}
	}

	candidates := make([]LDAPGroupMapping, 0, len(mappings))
	for _, m := range mappings {
		dn := strings.ToLower(strings.TrimSpace(m.LDAPGroupDN))
		if dn == "" {
			continue
		}
		if _, ok := groupSet[dn]; ok {
			candidates = append(candidates, m)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority == candidates[j].Priority {
			return candidates[i].LDAPGroupDN < candidates[j].LDAPGroupDN
		}
		return candidates[i].Priority > candidates[j].Priority
	})
	return &candidates[0]
}

func hashLDAPGroups(groupDNs []string) string {
	if len(groupDNs) == 0 {
		return ""
	}
	normalized := make([]string, 0, len(groupDNs))
	for _, dn := range groupDNs {
		dn = strings.ToLower(strings.TrimSpace(dn))
		if dn == "" {
			continue
		}
		normalized = append(normalized, dn)
	}
	sort.Strings(normalized)
	sum := sha256.Sum256([]byte(strings.Join(normalized, "|")))
	return hex.EncodeToString(sum[:])
}

func isLDAPUserAllowed(userGroups, allowedGroupDNs []string) bool {
	if len(allowedGroupDNs) == 0 {
		return true
	}
	groupSet := make(map[string]struct{}, len(userGroups))
	for _, g := range userGroups {
		groupSet[strings.ToLower(strings.TrimSpace(g))] = struct{}{}
	}
	for _, allowed := range allowedGroupDNs {
		if _, ok := groupSet[strings.ToLower(strings.TrimSpace(allowed))]; ok {
			return true
		}
	}
	return false
}

func buildLDAPUserFilter(cfg *LDAPConfig, identifier string) string {
	escapedValue := ldap.EscapeFilter(strings.TrimSpace(identifier))
	filter := strings.TrimSpace(cfg.UserFilter)
	if filter == "" {
		filter = "({login_attr}={login})"
	}
	filter = strings.ReplaceAll(filter, "{login_attr}", cfg.LoginAttr)
	filter = strings.ReplaceAll(filter, "{login}", escapedValue)
	if strings.Contains(filter, "%s") {
		filter = fmt.Sprintf(filter, escapedValue)
	}
	return filter
}

func uniqueLDAPAttrs(attrs []string) []string {
	out := make([]string, 0, len(attrs))
	seen := make(map[string]struct{}, len(attrs))
	for _, attr := range attrs {
		attr = strings.TrimSpace(attr)
		if attr == "" {
			continue
		}
		if _, ok := seen[attr]; ok {
			continue
		}
		seen[attr] = struct{}{}
		out = append(out, attr)
	}
	return out
}

func firstLDAPAttr(entry *ldap.Entry, attr string) string {
	if entry == nil || strings.TrimSpace(attr) == "" {
		return ""
	}
	value := strings.TrimSpace(entry.GetAttributeValue(attr))
	if value != "" {
		return value
	}
	raw := entry.GetRawAttributeValue(attr)
	if len(raw) > 0 {
		return hex.EncodeToString(raw)
	}
	return ""
}

func allLDAPAttrs(entry *ldap.Entry, attr string) []string {
	if entry == nil || strings.TrimSpace(attr) == "" {
		return nil
	}
	values := entry.GetAttributeValues(attr)
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}

func ldapEntryIsDisabled(entry *ldap.Entry) bool {
	if entry == nil {
		return false
	}
	raw := strings.TrimSpace(entry.GetAttributeValue("userAccountControl"))
	if raw == "" {
		return false
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return false
	}
	return (v & 0x2) == 0x2
}

func isLDAPInvalidCredentials(err error) bool {
	var ldapErr *ldap.Error
	if errors.As(err, &ldapErr) {
		return ldapErr.ResultCode == ldap.LDAPResultInvalidCredentials
	}
	return false
}

func normalizeUserAuthSource(source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		return "local"
	}
	return source
}

type tlsConfigWithServerName struct {
	InsecureSkipVerify bool
	ServerName         string
}

func (c *tlsConfigWithServerName) ToTLSConfig() *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: c.InsecureSkipVerify,
		ServerName:         c.ServerName,
		MinVersion:         tls.VersionTLS12,
	}
}
