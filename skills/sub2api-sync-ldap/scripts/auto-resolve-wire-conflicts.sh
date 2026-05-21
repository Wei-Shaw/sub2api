#!/usr/bin/env bash

set -euo pipefail

if [[ ! -f backend/internal/repository/wire.go || ! -f backend/internal/service/wire.go ]]; then
    echo "ERROR: run this script in sub2api repository root."
    exit 1
fi

has_unmerged_file() {
    [[ -n "$(git ls-files -u -- "$1")" ]]
}

resolve_repository_wire() {
    local file="backend/internal/repository/wire.go"
    perl -0pi -e 's{<<<<<<< HEAD\n\tNewSoraAccountRepository,[^\n]*\n\tNewScheduledTestPlanRepository,[^\n]*\n\tNewScheduledTestResultRepository,[^\n]*\n=======\n\tNewSoraAccountRepository,[^\n]*\n\tNewSoraGenerationRepository,[^\n]*\n>>>>>>> [^\n]+\n}{\tNewSoraAccountRepository, \/\/ Sora 账号扩展表仓储\n\tNewSoraGenerationRepository, \/\/ Sora 生成任务仓储\n\tNewScheduledTestPlanRepository,   \/\/ 定时测试计划仓储\n\tNewScheduledTestResultRepository, \/\/ 定时测试结果仓储\n}sg' "$file"
}

resolve_service_wire() {
    local file="backend/internal/service/wire.go"
    perl -0pi -e 's{<<<<<<< HEAD\n\tProvideScheduledTestService,\n\tProvideScheduledTestRunnerService,\n=======\n\tProviderSetExternalAuth,\n>>>>>>> [^\n]+\n}{\tProvideScheduledTestService,\n\tProvideScheduledTestRunnerService,\n\tProviderSetExternalAuth,\n}sg' "$file"
    perl -0pi -e 's{\n// ProvideOAuthRefreshAPI creates OAuthRefreshAPI with the default lock TTL\.\nfunc ProvideOAuthRefreshAPI\(accountRepo AccountRepository, tokenCache GeminiTokenCache\) \*OAuthRefreshAPI \{\n\treturn NewOAuthRefreshAPI\(accountRepo, tokenCache\)\n\}\n(?=.*// ProvideOAuthRefreshAPI creates OAuthRefreshAPI with the default distributed lock TTL\.)}{}sg' "$file"
}

resolve_public_settings_conflicts() {
    perl -0pi -e 's{<<<<<<< HEAD\n\tBackendModeEnabled\s+bool\s+`json:"backend_mode_enabled"`\n=======\n\tLDAPEnabled\s+bool\s+`json:"ldap_enabled"`\n>>>>>>> [^\n]+\n}{\tBackendModeEnabled               bool             `json:"backend_mode_enabled"`\n\tLDAPEnabled                      bool             `json:"ldap_enabled"`\n}sg' backend/internal/handler/dto/settings.go
    perl -0pi -e 's{<<<<<<< HEAD\n\t\tBackendModeEnabled:\s+settings\.BackendModeEnabled,\n=======\n\t\tLDAPEnabled:\s+settings\.LDAPEnabled,\n>>>>>>> [^\n]+\n}{\t\tBackendModeEnabled:               settings.BackendModeEnabled,\n\t\tLDAPEnabled:                      settings.LDAPEnabled,\n}sg' backend/internal/handler/setting_handler.go
    perl -0pi -e 's{<<<<<<< HEAD\n\tBackendModeEnabled\s+bool\n=======\n\tLDAPEnabled\s+bool\n>>>>>>> [^\n]+\n}{\tBackendModeEnabled  bool\n\tLDAPEnabled         bool\n}sg' backend/internal/service/settings_view.go
    perl -0pi -e 's{<<<<<<< HEAD\n\t\tSettingKeyBackendModeEnabled,\n=======\n\t\tSettingKeyLDAPEnabled,\n>>>>>>> [^\n]+\n}{\t\tSettingKeyBackendModeEnabled,\n\t\tSettingKeyLDAPEnabled,\n}sg' backend/internal/service/setting_service.go
    perl -0pi -e 's{<<<<<<< HEAD\n\t\tSettingKeyAffiliateEnabled,\n=======\n\t\tSettingKeyLDAPEnabled,\n>>>>>>> [^\n]+\n}{\t\tSettingKeyAffiliateEnabled,\n\t\tSettingKeyLDAPEnabled,\n}sg' backend/internal/service/setting_service.go
    perl -0pi -e 's{<<<<<<< HEAD\n\t\tBackendModeEnabled:\s+settings\[SettingKeyBackendModeEnabled\] == "true",\n=======\n\t\tLDAPEnabled:\s+ldapEnabled,\n>>>>>>> [^\n]+\n}{\t\tBackendModeEnabled:               settings[SettingKeyBackendModeEnabled] == "true",\n\t\tLDAPEnabled:                      ldapEnabled,\n}sg' backend/internal/service/setting_service.go
    perl -0pi -e 's{<<<<<<< HEAD\n\t\tBackendModeEnabled\s+bool\s+`json:"backend_mode_enabled"`\n=======\n\t\tLDAPEnabled\s+bool\s+`json:"ldap_enabled"`\n>>>>>>> [^\n]+\n}{\t\tBackendModeEnabled               bool            `json:"backend_mode_enabled"`\n\t\tLDAPEnabled                      bool            `json:"ldap_enabled"`\n}sg' backend/internal/service/setting_service.go
    perl -0pi -e 's{<<<<<<< HEAD\n\t\tBackendModeEnabled:\s+settings\.BackendModeEnabled,\n=======\n\t\tLDAPEnabled:\s+settings\.LDAPEnabled,\n>>>>>>> [^\n]+\n}{\t\tBackendModeEnabled:               settings.BackendModeEnabled,\n\t\tLDAPEnabled:                      settings.LDAPEnabled,\n}sg' backend/internal/service/setting_service.go
    perl -0pi -e 's{<<<<<<< HEAD\n\t\tBackendModeEnabled:\s+req\.BackendModeEnabled,\n=======\n\t\tLDAPEnabled:\s+req\.LDAPEnabled,\n\t\tLDAPHost:\s+req\.LDAPHost,\n\t\tLDAPPort:\s+req\.LDAPPort,\n\t\tLDAPUseTLS:\s+req\.LDAPUseTLS,\n\t\tLDAPStartTLS:\s+req\.LDAPStartTLS,\n\t\tLDAPInsecureSkipVerify:\s+req\.LDAPInsecureSkipVerify,\n\t\tLDAPBindDN:\s+req\.LDAPBindDN,\n\t\tLDAPBindPassword:\s+req\.LDAPBindPassword,\n\t\tLDAPUserBaseDN:\s+req\.LDAPUserBaseDN,\n\t\tLDAPUserFilter:\s+req\.LDAPUserFilter,\n\t\tLDAPLoginAttr:\s+req\.LDAPLoginAttr,\n\t\tLDAPUIDAttr:\s+req\.LDAPUIDAttr,\n\t\tLDAPEmailAttr:\s+req\.LDAPEmailAttr,\n\t\tLDAPDisplayNameAttr:\s+req\.LDAPDisplayNameAttr,\n\t\tLDAPDepartmentAttr:\s+req\.LDAPDepartmentAttr,\n\t\tLDAPGroupAttr:\s+req\.LDAPGroupAttr,\n\t\tLDAPAllowedGroupDNs:\s+req\.LDAPAllowedGroupDNs,\n\t\tLDAPGroupMappings:\s+fromDTOLDAPGroupMappings\(req\.LDAPGroupMappings\),\n\t\tLDAPSyncEnabled:\s+req\.LDAPSyncEnabled,\n\t\tLDAPSyncIntervalMinutes:\s+req\.LDAPSyncIntervalMinutes,\n>>>>>>> [^\n]+\n}{\t\tBackendModeEnabled:               req.BackendModeEnabled,\n\t\tLDAPEnabled:                      req.LDAPEnabled,\n\t\tLDAPHost:                         req.LDAPHost,\n\t\tLDAPPort:                         req.LDAPPort,\n\t\tLDAPUseTLS:                       req.LDAPUseTLS,\n\t\tLDAPStartTLS:                     req.LDAPStartTLS,\n\t\tLDAPInsecureSkipVerify:           req.LDAPInsecureSkipVerify,\n\t\tLDAPBindDN:                       req.LDAPBindDN,\n\t\tLDAPBindPassword:                 req.LDAPBindPassword,\n\t\tLDAPUserBaseDN:                   req.LDAPUserBaseDN,\n\t\tLDAPUserFilter:                   req.LDAPUserFilter,\n\t\tLDAPLoginAttr:                    req.LDAPLoginAttr,\n\t\tLDAPUIDAttr:                      req.LDAPUIDAttr,\n\t\tLDAPEmailAttr:                    req.LDAPEmailAttr,\n\t\tLDAPDisplayNameAttr:              req.LDAPDisplayNameAttr,\n\t\tLDAPDepartmentAttr:               req.LDAPDepartmentAttr,\n\t\tLDAPGroupAttr:                    req.LDAPGroupAttr,\n\t\tLDAPAllowedGroupDNs:              req.LDAPAllowedGroupDNs,\n\t\tLDAPGroupMappings:                fromDTOLDAPGroupMappings(req.LDAPGroupMappings),\n\t\tLDAPSyncEnabled:                  req.LDAPSyncEnabled,\n\t\tLDAPSyncIntervalMinutes:          req.LDAPSyncIntervalMinutes,\n}sg' backend/internal/handler/admin/setting_handler.go
    perl -0pi -e 's{<<<<<<< HEAD\n\t\t\t\t\t"backend_mode_enabled": false,\n\t\t\t\t\t"custom_menu_items": \[\]\n=======\n\t\t\t\t\t"custom_menu_items": \[\],\n\t\t\t\t\t"ldap_enabled": false,\n\t\t\t\t\t"ldap_host": "",\n\t\t\t\t\t"ldap_port": 389,\n\t\t\t\t\t"ldap_use_tls": false,\n\t\t\t\t\t"ldap_start_tls": false,\n\t\t\t\t\t"ldap_insecure_skip_verify": false,\n\t\t\t\t\t"ldap_bind_dn": "",\n\t\t\t\t\t"ldap_bind_password_configured": false,\n\t\t\t\t\t"ldap_user_base_dn": "",\n\t\t\t\t\t"ldap_user_filter": "\(\{login_attr\}=\{login\}\)",\n\t\t\t\t\t"ldap_login_attr": "mail",\n\t\t\t\t\t"ldap_display_name_attr": "displayName",\n\t\t\t\t\t"ldap_email_attr": "mail",\n\t\t\t\t\t"ldap_uid_attr": "uid",\n\t\t\t\t\t"ldap_department_attr": "department",\n\t\t\t\t\t"ldap_group_attr": "memberOf",\n\t\t\t\t\t"ldap_allowed_group_dns": \[\],\n\t\t\t\t\t"ldap_group_mappings": \[\],\n\t\t\t\t\t"ldap_sync_enabled": true,\n\t\t\t\t\t"ldap_sync_interval_minutes": 1440\n>>>>>>> [^\n]+\n}{\t\t\t\t\t"backend_mode_enabled": false,\n\t\t\t\t\t"custom_menu_items": [],\n\t\t\t\t\t"ldap_enabled": false,\n\t\t\t\t\t"ldap_host": "",\n\t\t\t\t\t"ldap_port": 389,\n\t\t\t\t\t"ldap_use_tls": false,\n\t\t\t\t\t"ldap_start_tls": false,\n\t\t\t\t\t"ldap_insecure_skip_verify": false,\n\t\t\t\t\t"ldap_bind_dn": "",\n\t\t\t\t\t"ldap_bind_password_configured": false,\n\t\t\t\t\t"ldap_user_base_dn": "",\n\t\t\t\t\t"ldap_user_filter": "({login_attr}={login})",\n\t\t\t\t\t"ldap_login_attr": "mail",\n\t\t\t\t\t"ldap_display_name_attr": "displayName",\n\t\t\t\t\t"ldap_email_attr": "mail",\n\t\t\t\t\t"ldap_uid_attr": "uid",\n\t\t\t\t\t"ldap_department_attr": "department",\n\t\t\t\t\t"ldap_group_attr": "memberOf",\n\t\t\t\t\t"ldap_allowed_group_dns": [],\n\t\t\t\t\t"ldap_group_mappings": [],\n\t\t\t\t\t"ldap_sync_enabled": true,\n\t\t\t\t\t"ldap_sync_interval_minutes": 1440\n}sg' backend/internal/server/api_contract_test.go
    perl -0pi -e 's{<<<<<<< HEAD\n\s+backend_mode_enabled: boolean\n=======\n\s+ldap_enabled: boolean\n>>>>>>> [^\n]+\n}{  backend_mode_enabled: boolean\n  ldap_enabled: boolean\n}sg' frontend/src/types/index.ts
    perl -0pi -e 's{<<<<<<< HEAD\n\s+backend_mode_enabled: false,\n=======\n\s+ldap_enabled: false,\n>>>>>>> [^\n]+\n}{        backend_mode_enabled: false,\n        ldap_enabled: false,\n}sg' frontend/src/stores/app.ts
}

resolve_auth_service_signature_drift() {
    # Upstream occasionally inserts new dependencies into AuthService. Keep the
    # newest upstream signature and preserve the LDAP external auth slot.
    perl -0pi -e 's{<<<<<<< HEAD\n(\s*\w+\s*:?=\s*)service\.NewAuthService\(([^,\n]+), ([^,\n]+), nil, nil, cfg, ([^\n]+)\)\n=======\n\1service\.NewAuthService\(\2, \3, nil, nil, nil, cfg, ([^\n]+)\)\n>>>>>>> [^\n]+\n}{\1service.NewAuthService(\2, \3, nil, nil, nil, cfg, \4, nil)}sg' \
        backend/cmd/jwtgen/main.go \
        backend/internal/server/middleware/admin_auth_test.go \
        backend/internal/server/middleware/jwt_auth_test.go

    perl -0pi -e 's@<<<<<<< HEAD\n// RegisterWithVerification[^\n]*\nfunc \(s \*AuthService\) RegisterWithVerification\(ctx context\.Context, email, password, verifyCode, promoCode, invitationCode, affiliateCode string\) \(string, \*User, error\) \{\n=======\n// RegisterWithVerification[^\n]*\nfunc \(s \*AuthService\) RegisterWithVerification\(ctx context\.Context, email, password, verifyCode, promoCode, invitationCode string\) \(string, \*User, error\) \{\n(\s*if s\.settingService != nil && s\.settingService\.IsLDAPEnabled\(ctx\) \{\n\s*return "", nil, infraerrors\.Forbidden\("LDAP_ONLY_MODE", "registration is disabled while LDAP mode is enabled"\)\n\s*\}\n)\n>>>>>>> [^\n]+\n@// RegisterWithVerification 用户注册（支持邮件验证、优惠码、邀请码和邀请返利码），返回token和用户。\nfunc (s *AuthService) RegisterWithVerification(ctx context.Context, email, password, verifyCode, promoCode, invitationCode, affiliateCode string) (string, *User, error) {\n\1@sg' \
        backend/internal/service/auth_service.go
}

resolve_frontend_account_modal_conflicts() {
    perl -0pi -e 's{\n<<<<<<< HEAD\n=======\nasync function loadTLSProfiles\(\) \{\n  try \{\n    const profiles = await adminAPI\.tlsFingerprintProfiles\.list\(\)\n    tlsFingerprintProfiles\.value = profiles\.map\(p => \(\{ id: p\.id, name: p\.name \}\)\)\n  \} catch \{\n    tlsFingerprintProfiles\.value = \[\]\n  \}\n\}\n\n>>>>>>> [^\n]+\n}{\n}sg' frontend/src/components/account/EditAccountModal.vue
}

resolve_compose_healthchecks() {
    local file
    for file in deploy/docker-compose.local.yml deploy/docker-compose.standalone.yml deploy/docker-compose.yml; do
        perl -0pi -e 's{<<<<<<< HEAD\n\s+test: \["CMD", "wget", "-q", "-T", "5", "-O", "/dev/null", "http://localhost:8080/health"\]\n=======\n\s+test: \["CMD-SHELL", "wget -q -T 5 -O /dev/null http://localhost:8080/health \|\| curl -fsS http://localhost:8080/health >/dev/null 2>&1"\]\n>>>>>>> [^\n]+\n}{      test: ["CMD-SHELL", "wget -q -T 5 -O /dev/null http://localhost:8080/health || curl -fsS http://localhost:8080/health >/dev/null 2>&1"]\n}sg' "$file"
    done
}

resolve_generated_conflicts() {
    if has_unmerged_file backend/cmd/server/VERSION; then
        git checkout --ours -- backend/cmd/server/VERSION >/dev/null
        git add backend/cmd/server/VERSION
    fi

    if has_unmerged_file backend/cmd/server/wire_gen.go; then
        # Keep the LDAP-side generated file long enough for generated-repair.sh to refresh it.
        git checkout --theirs -- backend/cmd/server/wire_gen.go >/dev/null
        git add backend/cmd/server/wire_gen.go
    fi
}

echo "Auto-resolve: known merge conflicts"
resolve_repository_wire
resolve_service_wire
resolve_public_settings_conflicts
resolve_auth_service_signature_drift
resolve_frontend_account_modal_conflicts
resolve_compose_healthchecks
resolve_generated_conflicts

if rg -n "^(<<<<<<<|=======|>>>>>>>)" \
    backend/cmd/jwtgen/main.go \
    backend/internal/repository/wire.go \
    backend/internal/service/wire.go \
    backend/internal/server/middleware/admin_auth_test.go \
    backend/internal/server/middleware/jwt_auth_test.go \
    backend/internal/handler/admin/setting_handler.go \
    backend/internal/handler/dto/settings.go \
    backend/internal/handler/setting_handler.go \
    backend/internal/server/api_contract_test.go \
    backend/internal/service/auth_service.go \
    backend/internal/service/setting_service.go \
    backend/internal/service/settings_view.go \
    deploy/docker-compose.local.yml \
    deploy/docker-compose.standalone.yml \
    deploy/docker-compose.yml \
    frontend/src/components/account/EditAccountModal.vue \
    frontend/src/stores/app.ts \
    frontend/src/types/index.ts >/dev/null 2>&1; then
    echo "WARN: conflict markers still present in known conflict files."
    exit 1
fi

git add \
    backend/cmd/jwtgen/main.go \
    backend/internal/repository/wire.go \
    backend/internal/service/wire.go \
    backend/internal/server/middleware/admin_auth_test.go \
    backend/internal/server/middleware/jwt_auth_test.go \
    backend/internal/handler/admin/setting_handler.go \
    backend/internal/handler/dto/settings.go \
    backend/internal/handler/setting_handler.go \
    backend/internal/server/api_contract_test.go \
    backend/internal/service/auth_service.go \
    backend/internal/service/setting_service.go \
    backend/internal/service/settings_view.go \
    deploy/docker-compose.local.yml \
    deploy/docker-compose.standalone.yml \
    deploy/docker-compose.yml \
    backend/cmd/server/VERSION \
    backend/cmd/server/wire_gen.go \
    frontend/src/components/account/EditAccountModal.vue \
    frontend/src/stores/app.ts \
    frontend/src/types/index.ts
echo "OK: resolved known wire/settings/compose conflicts."
