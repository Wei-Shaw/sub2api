<template>
  <div v-show="activeTab === 'security'" class="space-y-6">
    <!-- Admin API Key Settings -->
    <div class="card">
      <div
        class="card-header"
      >
        <h2 class="card-title">
          {{ t("admin.settings.adminApiKey.title") }}
        </h2>
        <p class="mt-1 text-sm text-fg-muted">
          {{ t("admin.settings.adminApiKey.description") }}
        </p>
      </div>
      <div class="card-body space-y-4">
        <!-- Security Warning -->
        <div
          class="border border-warning/40 bg-warning-weak px-4 py-3"
        >
          <div class="flex items-start">
            <Icon
              name="exclamationTriangle"
              size="md"
              class="mt-0.5 flex-shrink-0 text-warning"
            />
            <p class="ml-3 text-sm text-warning-strong">
              {{ t("admin.settings.adminApiKey.securityWarning") }}
            </p>
          </div>
        </div>

        <!-- Loading State -->
        <div
          v-if="adminApiKeyLoading"
          class="flex items-center gap-2 text-fg-muted"
        >
          <div
            class="h-4 w-4 animate-spin rounded-full border-b-2 border-accent"
          ></div>
          {{ t("common.loading") }}
        </div>

        <!-- No Key Configured -->
        <div
          v-else-if="!adminApiKeyExists"
          class="flex items-center justify-between"
        >
          <span class="text-fg-muted">
            {{ t("admin.settings.adminApiKey.notConfigured") }}
          </span>
          <button
            type="button"
            @click="createAdminApiKey"
            :disabled="adminApiKeyOperating"
            class="btn btn-primary btn-sm"
          >
            <svg
              v-if="adminApiKeyOperating"
              class="mr-1 h-4 w-4 animate-spin"
              fill="none"
              viewBox="0 0 24 24"
            >
              <circle
                class="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                stroke-width="4"
              ></circle>
              <path
                class="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
              ></path>
            </svg>
            {{
              adminApiKeyOperating
                ? t("admin.settings.adminApiKey.creating")
                : t("admin.settings.adminApiKey.create")
            }}
          </button>
        </div>

        <!-- Key Exists -->
        <div v-else class="space-y-4">
          <div class="flex items-center justify-between">
            <div>
              <label
                class="mb-1 block text-sm font-medium text-fg"
              >
                {{ t("admin.settings.adminApiKey.currentKey") }}
              </label>
              <code
                class="code px-2 py-1"
              >
                {{ adminApiKeyMasked }}
              </code>
            </div>
            <div class="flex gap-2">
              <button
                type="button"
                @click="regenerateAdminApiKey"
                :disabled="adminApiKeyOperating"
                class="btn btn-secondary btn-sm"
              >
                {{
                  adminApiKeyOperating
                    ? t("admin.settings.adminApiKey.regenerating")
                    : t("admin.settings.adminApiKey.regenerate")
                }}
              </button>
              <button
                type="button"
                @click="deleteAdminApiKey"
                :disabled="adminApiKeyOperating"
                class="btn btn-secondary btn-sm text-danger hover:text-danger-strong"
              >
                {{ t("admin.settings.adminApiKey.delete") }}
              </button>
            </div>
          </div>

          <!-- Newly Generated Key Display -->
          <div
            v-if="newAdminApiKey"
            class="stub space-y-3 px-4 py-3"
          >
            <p
              class="text-sm font-medium text-success-strong"
            >
              {{ t("admin.settings.adminApiKey.keyWarning") }}
            </p>
            <div class="flex items-center gap-2">
              <code
                class="code flex-1 select-all break-all px-3 py-2"
              >
                {{ newAdminApiKey }}
              </code>
              <button
                type="button"
                @click="copyNewKey"
                class="btn btn-primary btn-sm flex-shrink-0"
              >
                {{ t("admin.settings.adminApiKey.copyKey") }}
              </button>
            </div>
            <p class="text-xs text-success">
              {{ t("admin.settings.adminApiKey.usage") }}
            </p>
          </div>
        </div>
      </div>
    </div>
  </div>

  <div v-show="activeTab === 'security'" class="space-y-6">
    <!-- Registration Settings -->
    <div class="card">
      <div
        class="card-header"
      >
        <h2 class="card-title">
          {{ t("admin.settings.registration.title") }}
        </h2>
        <p class="mt-1 text-sm text-fg-muted">
          {{ t("admin.settings.registration.description") }}
        </p>
      </div>
      <div class="card-body space-y-5">
        <!-- Enable Registration -->
        <div class="flex items-center justify-between">
          <div>
            <label class="font-medium text-fg">{{
              t("admin.settings.registration.enableRegistration")
            }}</label>
            <p class="text-sm text-fg-muted">
              {{
                t("admin.settings.registration.enableRegistrationHint")
              }}
            </p>
          </div>
          <Toggle v-model="form.registration_enabled" />
        </div>

        <!-- Email Verification -->
        <div
          class="flex items-center justify-between border-t border-border pt-4"
        >
          <div>
            <label class="font-medium text-fg">{{
              t("admin.settings.registration.emailVerification")
            }}</label>
            <p class="text-sm text-fg-muted">
              {{ t("admin.settings.registration.emailVerificationHint") }}
            </p>
          </div>
          <Toggle v-model="form.email_verify_enabled" />
        </div>

        <!-- Email Suffix Whitelist -->
        <div class="border-t border-border pt-4">
          <label class="font-medium text-fg">{{
            t("admin.settings.registration.emailSuffixWhitelist")
          }}</label>
          <p class="mt-1 text-sm text-fg-muted">
            {{
              t("admin.settings.registration.emailSuffixWhitelistHint")
            }}
          </p>
          <div
            class="mt-3 rounded-sm border border-border-strong bg-surface p-2"
          >
            <div class="flex flex-wrap items-center gap-2">
              <span
                v-for="suffix in registrationEmailSuffixWhitelistTags"
                :key="suffix"
                class="inline-flex items-center gap-1 rounded-sm bg-surface-sunken px-2 py-1 text-xs font-mono text-fg"
              >
                <span>{{ suffix }}</span>
                <button
                  :aria-label="t('common.remove')"
                  :title="t('common.remove')"
                  type="button"
                  class="rounded-sm text-fg-muted hover:bg-accent-weak hover:text-fg"
                  @click="
                    removeRegistrationEmailSuffixWhitelistTag(suffix)
                  "
                >
                  <Icon
                    name="x"
                    size="xs"
                    class="h-3.5 w-3.5"
                    :stroke-width="2"
                  />
                </button>
              </span>

              <div
                class="flex min-w-[220px] flex-1 items-center gap-1 rounded-sm border border-transparent px-2 py-1 focus-within:border-accent"
              >
                <input
                  v-model="registrationEmailSuffixWhitelistDraft"
                  type="text"
                  class="w-full bg-transparent text-sm font-mono text-fg outline-none placeholder:text-fg-subtle"
                  :placeholder="
                    t(
                      'admin.settings.registration.emailSuffixWhitelistPlaceholder',
                    )
                  "
                  @input="
                    handleRegistrationEmailSuffixWhitelistDraftInput
                  "
                  @keydown="
                    handleRegistrationEmailSuffixWhitelistDraftKeydown
                  "
                  @blur="commitRegistrationEmailSuffixWhitelistDraft"
                  @paste="handleRegistrationEmailSuffixWhitelistPaste"
                />
              </div>
            </div>
          </div>
          <p class="mt-2 text-xs text-fg-muted">
            {{
              t(
                "admin.settings.registration.emailSuffixWhitelistInputHint",
              )
            }}
          </p>
        </div>

        <!-- Email Domain Quota -->
        <div
          class="flex items-center justify-between border-t border-border pt-4"
        >
          <div>
            <label class="font-medium text-fg">{{
              t("admin.settings.registration.emailDomainQuota")
            }}</label>
            <p class="text-sm text-fg-muted">
              {{ t("admin.settings.registration.emailDomainQuotaHint") }}
            </p>
          </div>
          <Toggle
            v-model="form.registration_email_domain_quota_enabled"
          />
        </div>

        <!-- Promo Code -->
        <div
          class="flex items-center justify-between border-t border-border pt-4"
        >
          <div>
            <label class="font-medium text-fg">{{
              t("admin.settings.registration.promoCode")
            }}</label>
            <p class="text-sm text-fg-muted">
              {{ t("admin.settings.registration.promoCodeHint") }}
            </p>
          </div>
          <Toggle v-model="form.promo_code_enabled" />
        </div>

        <!-- Invitation Code -->
        <div
          class="flex items-center justify-between border-t border-border pt-4"
        >
          <div>
            <label class="font-medium text-fg">{{
              t("admin.settings.registration.invitationCode")
            }}</label>
            <p class="text-sm text-fg-muted">
              {{ t("admin.settings.registration.invitationCodeHint") }}
            </p>
          </div>
          <Toggle v-model="form.invitation_code_enabled" />
        </div>
        <!-- Password Reset - Only show when email verification is enabled -->
        <div
          v-if="form.email_verify_enabled"
          class="flex items-center justify-between border-t border-border pt-4"
        >
          <div>
            <label class="font-medium text-fg">{{
              t("admin.settings.registration.passwordReset")
            }}</label>
            <p class="text-sm text-fg-muted">
              {{ t("admin.settings.registration.passwordResetHint") }}
            </p>
          </div>
          <Toggle v-model="form.password_reset_enabled" />
        </div>
        <!-- Frontend URL - Only show when password reset is enabled -->
        <div
          v-if="form.email_verify_enabled && form.password_reset_enabled"
          class="border-t border-border pt-4"
        >
          <label
            class="input-label"
          >
            {{ t("admin.settings.registration.frontendUrl") }}
          </label>
          <input
            v-model="form.frontend_url"
            type="url"
            class="input"
            :placeholder="
              t('admin.settings.registration.frontendUrlPlaceholder')
            "
          />
          <p class="input-hint">
            {{ t("admin.settings.registration.frontendUrlHint") }}
          </p>
        </div>

        <!-- TOTP 2FA -->
        <div
          class="flex items-center justify-between border-t border-border pt-4"
        >
          <div>
            <label class="font-medium text-fg">{{
              t("admin.settings.registration.totp")
            }}</label>
            <p class="text-sm text-fg-muted">
              {{ t("admin.settings.registration.totpHint") }}
            </p>
            <!-- Warning when encryption key not configured -->
            <p
              v-if="!form.totp_encryption_key_configured"
              class="mt-2 text-sm text-warning"
            >
              {{ t("admin.settings.registration.totpKeyNotConfigured") }}
            </p>
          </div>
          <Toggle
            v-model="form.totp_enabled"
            :disabled="!form.totp_encryption_key_configured"
          />
        </div>

        <!-- Passkey sign-in -->
        <div
          class="border-t border-border pt-4"
          data-testid="passkey-settings"
        >
          <div class="flex items-start justify-between gap-4">
            <div>
              <label class="font-medium text-fg">{{
                t("admin.settings.security.passkey")
              }}</label>
              <p class="text-sm text-fg-muted">
                {{ t("admin.settings.security.passkeyHint") }}
              </p>
            </div>
            <Toggle
              v-model="form.passkey_enabled"
              data-testid="passkey-toggle"
              :disabled="!form.passkey_configured"
            />
          </div>
          <div
            class="mt-3 border px-3 py-2 text-sm"
            :class="
              form.passkey_configured
                ? 'border-success/40 bg-success-weak text-success-strong'
                : 'border-warning/40 bg-warning-weak text-warning-strong'
            "
            data-testid="passkey-config-status"
          >
            <p class="font-medium">
              {{
                form.passkey_configured
                  ? t("admin.settings.security.passkeyConfigured")
                  : t("admin.settings.security.passkeyNotConfigured")
              }}
            </p>
            <p class="mt-1 break-all">
              {{ t("admin.settings.security.passkeyRPID") }}:
              {{
                form.passkey_rp_id ||
                t("admin.settings.security.passkeyValueNotConfigured")
              }}
            </p>
            <p class="mt-1 break-all">
              {{ t("admin.settings.security.passkeyOrigins") }}:
              {{
                form.passkey_rp_origins.length > 0
                  ? form.passkey_rp_origins.join(", ")
                  : t(
                      "admin.settings.security.passkeyValueNotConfigured",
                    )
              }}
            </p>
            <p v-if="!form.passkey_configured" class="mt-2">
              {{ t("admin.settings.security.passkeyDeploymentHint") }}
            </p>
          </div>
        </div>

        <!-- 敏感操作 step-up 2FA -->
        <div
          class="flex items-center justify-between border-t border-border pt-4"
        >
          <div>
            <label class="font-medium text-fg">{{
              t("admin.settings.security.stepUp")
            }}</label>
            <p class="text-sm text-fg-muted">
              {{ t("admin.settings.security.stepUpHint") }}
            </p>
          </div>
          <Toggle v-model="form.step_up_enabled" />
        </div>

        <!-- 会话 IP/UA 绑定 -->
        <div
          class="flex items-center justify-between border-t border-border pt-4"
        >
          <div>
            <label class="font-medium text-fg">{{
              t("admin.settings.security.sessionBinding")
            }}</label>
            <p class="text-sm text-fg-muted">
              {{ t("admin.settings.security.sessionBindingHint") }}
            </p>
          </div>
          <Toggle v-model="form.session_binding_enabled" />
        </div>

        <!-- 审计日志保留天数 -->
        <div
          class="flex items-center justify-between border-t border-border pt-4"
        >
          <div>
            <label class="font-medium text-fg">{{
              t("admin.settings.security.auditRetention")
            }}</label>
            <p class="text-sm text-fg-muted">
              {{ t("admin.settings.security.auditRetentionHint") }}
            </p>
          </div>
          <input
            v-model.number="form.audit_log_retention_days"
            type="number"
            min="0"
            class="input w-28 text-right"
          />
        </div>
      </div>
    </div>

    <!-- API Key IP ACL Settings -->
    <div class="card">
      <div
        class="card-header"
      >
        <h2 class="card-title">
          {{ t("admin.settings.apiKeyAcl.title") }}
        </h2>
        <p class="mt-1 text-sm text-fg-muted">
          {{ t("admin.settings.apiKeyAcl.description") }}
        </p>
      </div>
      <div class="card-body space-y-5">
        <div class="flex items-center justify-between gap-4">
          <div>
            <label class="font-medium text-fg">
              {{ t("admin.settings.apiKeyAcl.trustForwardedIp") }}
            </label>
            <p class="text-sm text-fg-muted">
              {{ t("admin.settings.apiKeyAcl.trustForwardedIpHint") }}
            </p>
          </div>
          <Toggle v-model="form.api_key_acl_trust_forwarded_ip" />
        </div>

        <div
          v-if="form.api_key_acl_trust_forwarded_ip"
          class="border-t border-border pt-4"
        >
          <label
            for="forwarded-client-ip-headers"
            class="font-medium text-fg"
          >
            {{ t("admin.settings.apiKeyAcl.forwardedClientIpHeaders") }}
          </label>
          <p class="mt-1 text-sm text-fg-muted">
            {{ t("admin.settings.apiKeyAcl.forwardedClientIpHeadersHint") }}
          </p>
          <div
            class="mt-3 rounded-sm border border-border-strong bg-surface p-2"
          >
            <div class="flex flex-wrap items-center gap-2">
              <span
                v-for="header in form.forwarded_client_ip_headers"
                :key="header"
                data-testid="forwarded-client-ip-header-tag"
                class="inline-flex items-center gap-1 rounded-sm bg-surface-sunken px-2 py-1 text-xs font-mono text-fg"
              >
                <span>{{ header }}</span>
                <button
                  type="button"
                  class="rounded-sm text-fg-muted hover:bg-accent-weak hover:text-fg"
                  :aria-label="t('admin.settings.apiKeyAcl.removeForwardedClientIpHeader', { header })"
                  @click="removeForwardedClientIpHeader(header)"
                >
                  <Icon
                    name="x"
                    size="xs"
                    class="h-3.5 w-3.5"
                    :stroke-width="2"
                  />
                </button>
              </span>
              <div
                class="flex min-w-[220px] flex-1 items-center gap-1 rounded-sm border border-transparent px-2 py-1 focus-within:border-accent"
              >
                <input
                  id="forwarded-client-ip-headers"
                  v-model="forwardedClientIpHeaderDraft"
                  data-testid="forwarded-client-ip-headers-input"
                  type="text"
                  class="w-full bg-transparent text-sm font-mono text-fg outline-none placeholder:text-fg-subtle"
                  :placeholder="t('admin.settings.apiKeyAcl.forwardedClientIpHeadersPlaceholder')"
                  @keydown="handleForwardedClientIpHeaderKeydown"
                  @blur="commitForwardedClientIpHeaderDraft"
                  @paste="handleForwardedClientIpHeaderPaste"
                />
              </div>
            </div>
          </div>
          <p class="mt-2 text-xs text-fg-muted">
            {{ t("admin.settings.apiKeyAcl.forwardedClientIpHeadersRiskHint") }}
          </p>
        </div>
      </div>
    </div>

    <!-- Panel API Rate Limit Settings -->
    <div class="card">
      <div
        class="card-header"
      >
        <div class="flex items-center gap-2">
          <Icon
            name="shield"
            size="md"
            class="text-accent"
          />
          <h2 class="card-title">
            {{ t("admin.settings.panelRateLimit.title") }}
          </h2>
        </div>
        <p class="mt-1 text-sm text-fg-muted">
          {{ t("admin.settings.panelRateLimit.description") }}
        </p>
      </div>
      <div class="card-body space-y-5">
        <div
          v-if="panelRateLimitLoading"
          class="flex items-center gap-2 text-fg-muted"
        >
          <div
            class="h-4 w-4 animate-spin rounded-full border-b-2 border-accent"
          ></div>
          {{ t("common.loading") }}
        </div>

        <template v-else>
          <!-- 计数维度说明：按账号计数，反代部署无误伤 -->
          <div
            class="border border-accent/40 bg-accent-weak px-4 py-3"
          >
            <div class="flex items-start">
              <Icon
                name="infoCircle"
                size="md"
                class="mt-0.5 flex-shrink-0 text-accent"
              />
              <p class="ml-3 text-sm text-accent-strong">
                {{ t("admin.settings.panelRateLimit.proxySafeNote") }}
              </p>
            </div>
          </div>

          <div class="flex items-center justify-between">
            <div>
              <label class="font-medium text-fg">{{
                t("admin.settings.panelRateLimit.enabled")
              }}</label>
              <p class="text-sm text-fg-muted">
                {{ t("admin.settings.panelRateLimit.enabledHint") }}
              </p>
            </div>
            <Toggle v-model="panelRateLimitForm.enabled" />
          </div>

          <div
            v-if="panelRateLimitForm.enabled"
            class="space-y-5 border-t border-border pt-4"
          >
            <div class="grid grid-cols-1 gap-6 sm:grid-cols-2">
              <div>
                <label
                  class="input-label"
                >
                  {{ t("admin.settings.panelRateLimit.userRpm") }}
                </label>
                <div class="flex items-center gap-2">
                  <input
                    v-model.number="panelRateLimitForm.user_rpm"
                    data-testid="panel-rate-limit-user-rpm"
                    type="number"
                    min="0"
                    max="100000"
                    class="input w-32"
                  />
                  <span class="text-sm text-fg-muted">
                    {{ t("admin.settings.panelRateLimit.perMinute") }}
                  </span>
                </div>
                <p class="input-hint">
                  {{ t("admin.settings.panelRateLimit.userRpmHint") }}
                </p>
              </div>

              <div>
                <label
                  class="input-label"
                >
                  {{ t("admin.settings.panelRateLimit.heavyRpm") }}
                </label>
                <div class="flex items-center gap-2">
                  <input
                    v-model.number="panelRateLimitForm.heavy_rpm"
                    type="number"
                    min="0"
                    max="100000"
                    class="input w-32"
                  />
                  <span class="text-sm text-fg-muted">
                    {{ t("admin.settings.panelRateLimit.perMinute") }}
                  </span>
                </div>
                <p class="input-hint">
                  {{ t("admin.settings.panelRateLimit.heavyRpmHint") }}
                </p>
              </div>

              <div>
                <label
                  class="input-label"
                >
                  {{ t("admin.settings.panelRateLimit.publicIpRpm") }}
                </label>
                <div class="flex items-center gap-2">
                  <input
                    v-model.number="panelRateLimitForm.public_ip_rpm"
                    type="number"
                    min="0"
                    max="100000"
                    class="input w-32"
                  />
                  <span class="text-sm text-fg-muted">
                    {{ t("admin.settings.panelRateLimit.perMinute") }}
                  </span>
                </div>
                <p class="input-hint">
                  {{ t("admin.settings.panelRateLimit.publicIpRpmHint") }}
                </p>
              </div>
            </div>

            <div
              class="flex items-center justify-between border-t border-border pt-4"
            >
              <div>
                <label class="font-medium text-fg">{{
                  t("admin.settings.panelRateLimit.exemptAdmin")
                }}</label>
                <p class="text-sm text-fg-muted">
                  {{ t("admin.settings.panelRateLimit.exemptAdminHint") }}
                </p>
              </div>
              <Toggle v-model="panelRateLimitForm.exempt_admin" />
            </div>
          </div>

          <div
            class="flex justify-end border-t border-border pt-4"
          >
            <button
              type="button"
              data-testid="panel-rate-limit-save"
              @click="savePanelRateLimitSettings"
              :disabled="panelRateLimitSaving"
              class="btn btn-primary btn-sm"
            >
              <svg
                v-if="panelRateLimitSaving"
                class="mr-1 h-4 w-4 animate-spin"
                fill="none"
                viewBox="0 0 24 24"
              >
                <circle
                  class="opacity-25"
                  cx="12"
                  cy="12"
                  r="10"
                  stroke="currentColor"
                  stroke-width="4"
                ></circle>
                <path
                  class="opacity-75"
                  fill="currentColor"
                  d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                ></path>
              </svg>
              {{
                panelRateLimitSaving
                  ? t("common.saving")
                  : t("common.save")
              }}
            </button>
          </div>
        </template>
      </div>
    </div>

    <!-- 人机验证 Settings -->
    <div class="card">
      <div
        class="card-header"
      >
        <h2 class="card-title">
          {{ t("admin.settings.captcha.title") }}
        </h2>
        <p class="mt-1 text-sm text-fg-muted">
          {{ t("admin.settings.captcha.description") }}
        </p>
      </div>
      <div class="card-body space-y-5">
        <!-- Enable Captcha -->
        <div class="flex items-center justify-between">
          <div>
            <label class="font-medium text-fg">{{
              t("admin.settings.captcha.enable")
            }}</label>
            <p class="text-sm text-fg-muted">
              {{ t("admin.settings.captcha.enableHint") }}
            </p>
          </div>
          <Toggle
            v-model="captchaMasterEnabled"
            data-testid="captcha-enabled-toggle"
          />
        </div>

        <!-- Provider fields - Only show when enabled -->
        <div
          v-if="captchaMasterEnabled"
          class="border-t border-border pt-4"
        >
          <!-- Provider Selector -->
          <div class="mb-6">
            <label
              class="input-label"
            >
              {{ t("admin.settings.captcha.provider") }}
            </label>
            <div
              class="grid grid-cols-3 divide-x divide-border-strong border border-border-strong"
            >
              <button
                type="button"
                data-testid="captcha-provider-turnstile"
                class="inline-flex min-h-[40px] items-center justify-center gap-2 px-3 py-2 text-sm font-semibold transition-colors"
                :class="
                  captchaProviderSelection === 'turnstile'
                    ? 'bg-accent text-white dark:text-surface-sunken'
                    : 'bg-surface text-fg-muted hover:bg-accent-weak hover:text-accent-strong'
                "
                @click="selectCaptchaProvider('turnstile')"
              >
                {{ t("admin.settings.captcha.providerTurnstile") }}
              </button>
              <button
                type="button"
                data-testid="captcha-provider-tencent"
                class="inline-flex min-h-[40px] items-center justify-center gap-2 px-3 py-2 text-sm font-semibold transition-colors"
                :class="
                  captchaProviderSelection === 'tencent'
                    ? 'bg-accent text-white dark:text-surface-sunken'
                    : 'bg-surface text-fg-muted hover:bg-accent-weak hover:text-accent-strong'
                "
                @click="selectCaptchaProvider('tencent')"
              >
                {{ t("admin.settings.captcha.providerTencent") }}
              </button>
              <button
                type="button"
                data-testid="captcha-provider-aliyun"
                class="inline-flex min-h-[40px] items-center justify-center gap-2 px-3 py-2 text-sm font-semibold transition-colors"
                :class="
                  captchaProviderSelection === 'aliyun'
                    ? 'bg-accent text-white dark:text-surface-sunken'
                    : 'bg-surface text-fg-muted hover:bg-accent-weak hover:text-accent-strong'
                "
                @click="selectCaptchaProvider('aliyun')"
              >
                {{ t("admin.settings.captcha.providerAliyun") }}
              </button>
            </div>
          </div>

          <!-- Cloudflare Turnstile fields -->
          <div
            v-if="captchaProviderSelection === 'turnstile'"
            class="grid grid-cols-1 gap-6"
          >
            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.turnstile.siteKey") }}
              </label>
              <input
                v-model="form.turnstile_site_key"
                type="text"
                class="input font-mono text-sm"
                placeholder="0x4AAAAAAA..."
              />
              <p class="input-hint">
                {{ t("admin.settings.turnstile.siteKeyHint") }}
                <a
                  href="https://dash.cloudflare.com/"
                  target="_blank"
                  class="text-accent hover:text-accent-strong"
                  >{{
                    t("admin.settings.turnstile.cloudflareDashboard")
                  }}</a
                >
              </p>
            </div>
            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.turnstile.secretKey") }}
              </label>
              <input
                v-model="form.turnstile_secret_key"
                type="password"
                class="input font-mono text-sm"
                placeholder="0x4AAAAAAA..."
              />
              <p class="input-hint">
                {{
                  form.turnstile_secret_key_configured
                    ? t(
                        "admin.settings.turnstile.secretKeyConfiguredHint",
                      )
                    : t("admin.settings.turnstile.secretKeyHint")
                }}
              </p>
            </div>
          </div>

          <!-- Tencent Captcha fields -->
          <div v-else-if="captchaProviderSelection === 'tencent'">
            <div class="mb-6 max-w-sm">
              <label class="input-label">
                {{ t("admin.settings.tencentCaptcha.region") }}
              </label>
              <div class="grid grid-cols-2 divide-x divide-border-strong border border-border-strong">
                <button
                  type="button"
                  data-testid="tencent-captcha-region-cn"
                  class="inline-flex min-h-[36px] items-center justify-center px-3 py-1.5 text-sm font-semibold transition-colors"
                  :class="
                    form.tencent_captcha_region !== 'intl'
                      ? 'bg-accent text-white dark:text-surface-sunken'
                      : 'bg-surface text-fg-muted hover:bg-accent-weak hover:text-accent-strong'
                  "
                  @click="form.tencent_captcha_region = 'cn'"
                >
                  {{ t("admin.settings.tencentCaptcha.regionCn") }}
                </button>
                <button
                  type="button"
                  data-testid="tencent-captcha-region-intl"
                  class="inline-flex min-h-[36px] items-center justify-center px-3 py-1.5 text-sm font-semibold transition-colors"
                  :class="
                    form.tencent_captcha_region === 'intl'
                      ? 'bg-accent text-white dark:text-surface-sunken'
                      : 'bg-surface text-fg-muted hover:bg-accent-weak hover:text-accent-strong'
                  "
                  @click="form.tencent_captcha_region = 'intl'"
                >
                  {{ t("admin.settings.tencentCaptcha.regionIntl") }}
                </button>
              </div>
              <p class="input-hint">
                {{ t("admin.settings.tencentCaptcha.regionHint") }}
              </p>
            </div>
            <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
              <div class="md:col-span-2">
                <h3 class="text-label font-bold text-fg">
                  {{ t("admin.settings.tencentCaptcha.appCredentialsTitle") }}
                </h3>
                <p class="mt-1 text-xs text-fg-muted">
                  {{ t("admin.settings.tencentCaptcha.appCredentialsHint") }}
                </p>
              </div>
              <div>
                <label class="input-label">
                  {{ t("admin.settings.tencentCaptcha.appId") }}
                </label>
                <input
                  v-model="form.tencent_captcha_app_id"
                  type="text"
                  inputmode="numeric"
                  class="input font-mono text-sm"
                  placeholder="123456789"
                />
              </div>
              <div>
                <label class="input-label">
                  {{ t("admin.settings.tencentCaptcha.appSecretKey") }}
                </label>
                <input
                  v-model="form.tencent_captcha_app_secret_key"
                  type="password"
                  autocomplete="new-password"
                  class="input font-mono text-sm"
                  :placeholder="t('admin.settings.tencentCaptcha.keepExisting')"
                />
                <p class="input-hint">
                  {{ form.tencent_captcha_app_secret_key_configured ? t("admin.settings.tencentCaptcha.configured") : t("admin.settings.tencentCaptcha.required") }}
                </p>
              </div>
              <div class="border-t border-border pt-5 md:col-span-2">
                <h3 class="text-label font-bold text-fg">
                  {{ t("admin.settings.tencentCaptcha.cloudCredentialsTitle") }}
                </h3>
                <p class="mt-1 text-xs text-fg-muted">
                  {{ t("admin.settings.tencentCaptcha.cloudCredentialsHint") }}
                </p>
              </div>
              <div>
                <label class="input-label">
                  {{ t("admin.settings.tencentCaptcha.cloudSecretId") }}
                </label>
                <input
                  v-model="form.tencent_captcha_cloud_secret_id"
                  type="password"
                  autocomplete="new-password"
                  class="input font-mono text-sm"
                  :placeholder="t('admin.settings.tencentCaptcha.keepExisting')"
                />
                <p class="input-hint">
                  {{ form.tencent_captcha_cloud_secret_id_configured ? t("admin.settings.tencentCaptcha.configured") : t("admin.settings.tencentCaptcha.required") }}
                </p>
              </div>
              <div>
                <label class="input-label">
                  {{ t("admin.settings.tencentCaptcha.cloudSecretKey") }}
                </label>
                <input
                  v-model="form.tencent_captcha_cloud_secret_key"
                  type="password"
                  autocomplete="new-password"
                  class="input font-mono text-sm"
                  :placeholder="t('admin.settings.tencentCaptcha.keepExisting')"
                />
                <p class="input-hint">
                  {{ form.tencent_captcha_cloud_secret_key_configured ? t("admin.settings.tencentCaptcha.configured") : t("admin.settings.tencentCaptcha.required") }}
                </p>
              </div>
            </div>
            <p class="mt-5 text-xs text-fg-muted">
              {{ t("admin.settings.tencentCaptcha.camPermissionHint") }}
            </p>
            <p class="mt-2 text-xs text-fg-muted">
              {{ t("admin.settings.tencentCaptcha.aidEncryptedHint") }}
            </p>
            <div class="mt-3 flex flex-wrap gap-x-4 gap-y-2 text-sm">
              <a
                :href="tencentCaptchaLinks.console"
                target="_blank"
                rel="noopener noreferrer"
                class="text-accent hover:text-accent-strong"
              >
                {{ t("admin.settings.tencentCaptcha.openCaptchaConsole") }}
              </a>
              <a
                :href="tencentCaptchaLinks.cloudKeys"
                target="_blank"
                rel="noopener noreferrer"
                class="text-accent hover:text-accent-strong"
              >
                {{ t("admin.settings.tencentCaptcha.createCloudKeys") }}
              </a>
              <a
                :href="tencentCaptchaLinks.webDocs"
                target="_blank"
                rel="noopener noreferrer"
                class="text-accent hover:text-accent-strong"
              >
                {{ t("admin.settings.tencentCaptcha.openWebDocs") }}
              </a>
            </div>
          </div>

          <!-- Aliyun Captcha 2.0 fields -->
          <div v-else class="grid grid-cols-1 gap-6">
            <div class="grid grid-cols-1 gap-6 sm:grid-cols-2">
              <div>
                <label
                  class="input-label"
                >
                  {{ t("admin.settings.aliyunCaptcha.region") }}
                </label>
                <div
                  class="grid grid-cols-2 divide-x divide-border-strong border border-border-strong"
                >
                  <button
                    type="button"
                    class="inline-flex min-h-[36px] items-center justify-center px-3 py-1.5 text-sm font-semibold transition-colors"
                    :class="
                      form.aliyun_captcha_region !== 'sgp'
                        ? 'bg-accent text-white dark:text-surface-sunken'
                        : 'bg-surface text-fg-muted hover:bg-accent-weak hover:text-accent-strong'
                    "
                    @click="form.aliyun_captcha_region = 'cn'"
                  >
                    {{ t("admin.settings.aliyunCaptcha.regionCn") }}
                  </button>
                  <button
                    type="button"
                    class="inline-flex min-h-[36px] items-center justify-center px-3 py-1.5 text-sm font-semibold transition-colors"
                    :class="
                      form.aliyun_captcha_region === 'sgp'
                        ? 'bg-accent text-white dark:text-surface-sunken'
                        : 'bg-surface text-fg-muted hover:bg-accent-weak hover:text-accent-strong'
                    "
                    @click="form.aliyun_captcha_region = 'sgp'"
                  >
                    {{ t("admin.settings.aliyunCaptcha.regionSgp") }}
                  </button>
                </div>
                <p class="input-hint">
                  {{ t("admin.settings.aliyunCaptcha.regionHint") }}
                </p>
              </div>
              <div>
                <label
                  class="input-label"
                >
                  {{ t("admin.settings.aliyunCaptcha.prefix") }}
                </label>
                <input
                  v-model="form.aliyun_captcha_prefix"
                  type="text"
                  class="input font-mono text-sm"
                  placeholder="14xxxxx"
                />
                <p class="input-hint">
                  {{ t("admin.settings.aliyunCaptcha.prefixHint") }}
                </p>
              </div>
            </div>
            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.aliyunCaptcha.sceneId") }}
              </label>
              <input
                v-model="form.aliyun_captcha_scene_id"
                type="text"
                class="input font-mono text-sm"
                placeholder="1cxxxxxx"
              />
              <p class="input-hint">
                {{ t("admin.settings.aliyunCaptcha.sceneIdHint") }}
              </p>
            </div>
            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.aliyunCaptcha.accessKeyId") }}
              </label>
              <input
                v-model="form.aliyun_captcha_access_key_id"
                type="text"
                class="input font-mono text-sm"
                placeholder="LTAI..."
              />
              <p class="input-hint">
                {{ t("admin.settings.aliyunCaptcha.accessKeyIdHint") }}
              </p>
            </div>
            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.aliyunCaptcha.accessKeySecret") }}
              </label>
              <input
                v-model="form.aliyun_captcha_access_key_secret"
                type="password"
                autocomplete="new-password"
                class="input font-mono text-sm"
                placeholder="••••••••"
              />
              <p class="input-hint">
                {{
                  form.aliyun_captcha_access_key_secret_configured
                    ? t(
                        "admin.settings.aliyunCaptcha.accessKeySecretConfiguredHint",
                      )
                    : t("admin.settings.aliyunCaptcha.accessKeySecretHint")
                }}
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- LinuxDo Connect OAuth 登录 -->
    <div class="card">
      <div
        class="card-header"
      >
        <h2 class="card-title">
          {{ t("admin.settings.linuxdo.title") }}
        </h2>
        <p class="mt-1 text-sm text-fg-muted">
          {{ t("admin.settings.linuxdo.description") }}
        </p>
      </div>
      <div class="card-body space-y-5">
        <div class="flex items-center justify-between">
          <div>
            <label class="font-medium text-fg">{{
              t("admin.settings.linuxdo.enable")
            }}</label>
            <p class="text-sm text-fg-muted">
              {{ t("admin.settings.linuxdo.enableHint") }}
            </p>
          </div>
          <Toggle v-model="form.linuxdo_connect_enabled" />
        </div>

        <div
          v-if="form.linuxdo_connect_enabled"
          class="border-t border-border pt-4"
        >
          <div class="grid grid-cols-1 gap-6">
            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.linuxdo.clientId") }}
              </label>
              <input
                v-model="form.linuxdo_connect_client_id"
                type="text"
                class="input font-mono text-sm"
                :placeholder="
                  t('admin.settings.linuxdo.clientIdPlaceholder')
                "
              />
              <p class="input-hint">
                {{ t("admin.settings.linuxdo.clientIdHint") }}
              </p>
            </div>

            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.linuxdo.clientSecret") }}
              </label>
              <input
                v-model="form.linuxdo_connect_client_secret"
                type="password"
                class="input font-mono text-sm"
                :placeholder="
                  form.linuxdo_connect_client_secret_configured
                    ? t(
                        'admin.settings.linuxdo.clientSecretConfiguredPlaceholder',
                      )
                    : t('admin.settings.linuxdo.clientSecretPlaceholder')
                "
              />
              <p class="input-hint">
                {{
                  form.linuxdo_connect_client_secret_configured
                    ? t(
                        "admin.settings.linuxdo.clientSecretConfiguredHint",
                      )
                    : t("admin.settings.linuxdo.clientSecretHint")
                }}
              </p>
            </div>

            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.linuxdo.redirectUrl") }}
              </label>
              <input
                v-model="form.linuxdo_connect_redirect_url"
                type="url"
                class="input font-mono text-sm"
                :placeholder="
                  t('admin.settings.linuxdo.redirectUrlPlaceholder')
                "
              />
              <div
                class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3"
              >
                <button
                  type="button"
                  class="btn btn-secondary btn-sm w-fit"
                  @click="setAndCopyLinuxdoRedirectUrl"
                >
                  {{ t("admin.settings.linuxdo.quickSetCopy") }}
                </button>
                <code
                  v-if="linuxdoRedirectUrlSuggestion"
                  class="code select-all break-all px-2 py-1 text-xs text-fg-muted"
                >
                  {{ linuxdoRedirectUrlSuggestion }}
                </code>
              </div>
              <p class="input-hint">
                {{ t("admin.settings.linuxdo.redirectUrlHint") }}
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Telegram Login -->
    <div class="card">
      <div class="card-header">
        <h2 class="card-title">{{ t("admin.settings.telegram.title") }}</h2>
        <p class="mt-1 text-sm text-fg-muted">
          {{ t("admin.settings.telegram.description") }}
        </p>
      </div>
      <div class="card-body space-y-5">
        <div class="flex items-center justify-between">
          <div>
            <label class="font-medium text-fg">{{ t("admin.settings.telegram.enable") }}</label>
            <p class="text-sm text-fg-muted">{{ t("admin.settings.telegram.enableHint") }}</p>
          </div>
          <Toggle v-model="form.telegram_oauth_enabled" />
        </div>

        <div v-if="form.telegram_oauth_enabled" class="border-t border-border pt-4">
          <div class="grid grid-cols-1 gap-6">
            <div>
              <label class="input-label">{{ t("admin.settings.telegram.botToken") }}</label>
              <input
                v-model="form.telegram_oauth_bot_token"
                type="password"
                class="input font-mono text-sm"
                :placeholder="
                  form.telegram_oauth_bot_token_configured
                    ? t('admin.settings.linuxdo.clientSecretConfiguredPlaceholder')
                    : '123456789:ABC...'
                "
              />
              <p class="input-hint">{{ t("admin.settings.telegram.botTokenHint") }}</p>
            </div>

            <div>
              <label class="input-label">{{ t("admin.settings.telegram.redirectUrl") }}</label>
              <input
                v-model="form.telegram_oauth_redirect_url"
                type="url"
                class="input font-mono text-sm"
                placeholder="https://example.com/auth/telegram/callback"
              />
              <div class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3">
                <button
                  type="button"
                  class="btn btn-secondary btn-sm w-fit"
                  @click="setAndCopyTelegramRedirectUrl"
                >
                  {{ t("admin.settings.linuxdo.quickSetCopy") }}
                </button>
                <code
                  v-if="telegramRedirectUrlSuggestion"
                  class="code select-all break-all px-2 py-1 text-xs text-fg-muted"
                >
                  {{ telegramRedirectUrlSuggestion }}
                </code>
              </div>
              <p class="input-hint">{{ t("admin.settings.telegram.redirectUrlHint") }}</p>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- GitHub / Google 邮箱快捷登录 -->
    <div class="card">
      <div
        class="card-header"
      >
        <h2 class="card-title">
          {{ t("admin.settings.oauthLogin.title") }}
        </h2>
        <p class="mt-1 text-sm text-fg-muted">
          {{
            t("admin.settings.oauthLogin.description")
          }}
        </p>
      </div>
      <div class="card-body space-y-6">
        <div class="grid grid-cols-1 gap-6 xl:grid-cols-2">
          <div class="border-t border-border pt-4">
            <div class="flex items-start justify-between gap-4">
              <div>
                <h3 class="font-medium text-fg">
                  GitHub
                </h3>
                <p class="mt-1 text-sm text-fg-muted">
                  {{
                    t("admin.settings.oauthLogin.githubHint")
                  }}
                </p>
              </div>
              <Toggle v-model="form.github_oauth_enabled" />
            </div>

            <div v-if="form.github_oauth_enabled" class="mt-4 space-y-4">
              <div class="border-l border-border-strong px-3 py-1 text-xs text-fg-muted">
                <i18n-t keypath="admin.settings.oauthLogin.githubGuide" tag="span" scope="global">
                  <template #link>
                    <a
                      data-testid="github-oauth-apps-guide-link"
                      href="https://github.com/settings/developers"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="font-medium text-accent hover:underline"
                    >OAuth Apps</a>
                  </template>
                </i18n-t>
              </div>

              <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
                <div>
                  <label class="input-label">Client ID</label>
                  <input
                    v-model="form.github_oauth_client_id"
                    type="text"
                    class="input font-mono text-sm"
                    placeholder="GitHub OAuth Client ID"
                  />
                </div>
                <div>
                  <label class="input-label">Client Secret</label>
                  <input
                    v-model="form.github_oauth_client_secret"
                    type="password"
                    class="input font-mono text-sm"
                    :placeholder="
                      form.github_oauth_client_secret_configured
                        ? t('admin.settings.oauthLogin.secretConfiguredPlaceholder')
                        : 'GitHub OAuth Client Secret'
                    "
                  />
                </div>
              </div>

              <div>
                <label class="input-label">
                  {{ t("admin.settings.oauthLogin.backendCallbackUrl") }}
                </label>
                <input
                  v-model="form.github_oauth_redirect_url"
                  type="url"
                  class="input font-mono text-sm"
                  placeholder="https://your-domain.com/api/v1/auth/oauth/github/callback"
                />
                <div class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3">
                  <button
                    type="button"
                    class="btn btn-secondary btn-sm w-fit"
                    @click="setAndCopyEmailOAuthRedirectUrl('github')"
                  >
                    {{ t("admin.settings.oauthLogin.generateAndCopy") }}
                  </button>
                  <code
                    v-if="githubOAuthRedirectUrlSuggestion"
                    class="code select-all break-all px-2 py-1 text-xs text-fg-muted"
                  >
                    {{ githubOAuthRedirectUrlSuggestion }}
                  </code>
                </div>
              </div>

              <div>
                <label class="input-label">
                  {{ t("admin.settings.oauthLogin.frontendCallbackUrl") }}
                </label>
                <input
                  v-model="form.github_oauth_frontend_redirect_url"
                  type="text"
                  class="input font-mono text-sm"
                  placeholder="/auth/oauth/callback"
                />
              </div>
            </div>
          </div>

          <div class="border-t border-border pt-4">
            <div class="flex items-start justify-between gap-4">
              <div>
                <h3 class="font-medium text-fg">
                  Google
                </h3>
                <p class="mt-1 text-sm text-fg-muted">
                  {{
                    t("admin.settings.oauthLogin.googleHint")
                  }}
                </p>
              </div>
              <Toggle v-model="form.google_oauth_enabled" />
            </div>

            <div v-if="form.google_oauth_enabled" class="mt-4 space-y-4">
              <div class="border-l border-border-strong px-3 py-1 text-xs text-fg-muted">
                {{
                  t("admin.settings.oauthLogin.googleGuide")
                }}
              </div>

              <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
                <div>
                  <label class="input-label">Client ID</label>
                  <input
                    v-model="form.google_oauth_client_id"
                    type="text"
                    class="input font-mono text-sm"
                    placeholder="Google OAuth Client ID"
                  />
                </div>
                <div>
                  <label class="input-label">Client Secret</label>
                  <input
                    v-model="form.google_oauth_client_secret"
                    type="password"
                    class="input font-mono text-sm"
                    :placeholder="
                      form.google_oauth_client_secret_configured
                        ? t('admin.settings.oauthLogin.secretConfiguredPlaceholder')
                        : 'Google OAuth Client Secret'
                    "
                  />
                </div>
              </div>

              <div>
                <label class="input-label">
                  {{ t("admin.settings.oauthLogin.backendCallbackUrl") }}
                </label>
                <input
                  v-model="form.google_oauth_redirect_url"
                  type="url"
                  class="input font-mono text-sm"
                  placeholder="https://your-domain.com/api/v1/auth/oauth/google/callback"
                />
                <div class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3">
                  <button
                    type="button"
                    class="btn btn-secondary btn-sm w-fit"
                    @click="setAndCopyEmailOAuthRedirectUrl('google')"
                  >
                    {{ t("admin.settings.oauthLogin.generateAndCopy") }}
                  </button>
                  <code
                    v-if="googleOAuthRedirectUrlSuggestion"
                    class="code select-all break-all px-2 py-1 text-xs text-fg-muted"
                  >
                    {{ googleOAuthRedirectUrlSuggestion }}
                  </code>
                </div>
              </div>

              <div>
                <label class="input-label">
                  {{ t("admin.settings.oauthLogin.frontendCallbackUrl") }}
                </label>
                <input
                  v-model="form.google_oauth_frontend_redirect_url"
                  type="text"
                  class="input font-mono text-sm"
                  placeholder="/auth/oauth/callback"
                />
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- WeChat Connect OAuth 登录 -->
    <div class="card">
      <div
        class="card-header"
      >
        <h2 class="card-title">
          {{ t("admin.settings.wechatConnect.title") }}
        </h2>
        <p class="mt-1 text-sm text-fg-muted">
          {{ t("admin.settings.wechatConnect.description") }}
        </p>
      </div>
      <div class="card-body space-y-5">
        <div class="flex items-center justify-between">
          <div>
            <label class="font-medium text-fg">{{
              t("admin.settings.wechatConnect.enabledLabel")
            }}</label>
            <p class="text-sm text-fg-muted">
              {{ t("admin.settings.wechatConnect.enabledHint") }}
            </p>
          </div>
          <Toggle
            v-model="form.wechat_connect_enabled"
            data-testid="wechat-connect-enabled"
          />
        </div>

        <div
          v-if="form.wechat_connect_enabled"
          class="space-y-6 border-t border-border pt-4"
        >
          <div class="space-y-4">
            <div
              class="border-t border-border pt-4"
            >
              <div class="flex items-start justify-between gap-4">
                <div>
                  <h3 class="font-medium text-fg">
                    {{ t("admin.settings.wechatConnect.pcAppTitle") }}
                  </h3>
                  <p class="mt-1 text-sm text-fg-muted">
                    {{
                      t("admin.settings.wechatConnect.pcAppHint")
                    }}
                  </p>
                </div>
                <Toggle
                  :model-value="form.wechat_connect_open_enabled"
                  data-testid="wechat-connect-open-enabled"
                  @update:model-value="handleWeChatOpenEnabledChange"
                />
              </div>
              <div
                v-if="form.wechat_connect_open_enabled"
                class="mt-4 grid grid-cols-1 gap-4 lg:grid-cols-2"
              >
                <div>
                  <label
                    class="input-label"
                  >
                    {{ t("admin.settings.wechatConnect.pcAppIdLabel") }}
                  </label>
                  <input
                    v-model="form.wechat_connect_open_app_id"
                    data-testid="wechat-connect-open-app-id"
                    type="text"
                    class="input font-mono text-sm"
                    :placeholder="
                      t('admin.settings.wechatConnect.pcAppIdPlaceholder')
                    "
                  />
                </div>
                <div>
                  <label
                    class="input-label"
                  >
                    {{ t("admin.settings.wechatConnect.pcAppSecretLabel") }}
                  </label>
                  <input
                    v-model="form.wechat_connect_open_app_secret"
                    data-testid="wechat-connect-open-app-secret"
                    type="password"
                    class="input font-mono text-sm"
                    :placeholder="
                      form.wechat_connect_open_app_secret_configured
                        ? t('admin.settings.wechatConnect.appSecretConfiguredPlaceholder')
                        : t('admin.settings.wechatConnect.pcAppSecretPlaceholder')
                    "
                  />
                </div>
              </div>
            </div>

            <div
              class="border-t border-border pt-4"
            >
              <div class="flex items-start justify-between gap-4">
                <div>
                  <h3 class="font-medium text-fg">
                    {{ t("admin.settings.wechatConnect.mpTitle") }}
                  </h3>
                  <p class="mt-1 text-sm text-fg-muted">
                    {{
                      t("admin.settings.wechatConnect.mpHint")
                    }}
                  </p>
                </div>
                <Toggle
                  :model-value="form.wechat_connect_mp_enabled"
                  data-testid="wechat-connect-mp-enabled"
                  @update:model-value="handleWeChatMPEnabledChange"
                />
              </div>
              <div
                v-if="form.wechat_connect_mp_enabled"
                class="mt-4 grid grid-cols-1 gap-4 lg:grid-cols-2"
              >
                <div>
                  <label
                    class="input-label"
                  >
                    {{ t("admin.settings.wechatConnect.mpAppIdLabel") }}
                  </label>
                  <input
                    v-model="form.wechat_connect_mp_app_id"
                    data-testid="wechat-connect-mp-app-id"
                    type="text"
                    class="input font-mono text-sm"
                    :placeholder="
                      t('admin.settings.wechatConnect.mpAppIdLabel')
                    "
                  />
                </div>
                <div>
                  <label
                    class="input-label"
                  >
                    {{
                      t("admin.settings.wechatConnect.mpAppSecretLabel")
                    }}
                  </label>
                  <input
                    v-model="form.wechat_connect_mp_app_secret"
                    data-testid="wechat-connect-mp-app-secret"
                    type="password"
                    class="input font-mono text-sm"
                    :placeholder="
                      form.wechat_connect_mp_app_secret_configured
                        ? t('admin.settings.wechatConnect.appSecretConfiguredPlaceholder')
                        : t('admin.settings.wechatConnect.mpAppSecretLabel')
                    "
                  />
                </div>
              </div>
            </div>

            <div
              class="border-t border-border pt-4"
            >
              <div class="flex items-start justify-between gap-4">
                <div>
                  <h3 class="font-medium text-fg">
                    {{ t("admin.settings.wechatConnect.mobileTitle") }}
                  </h3>
                  <p class="mt-1 text-sm text-fg-muted">
                    {{
                      t("admin.settings.wechatConnect.mobileHint")
                    }}
                  </p>
                </div>
                <Toggle
                  :model-value="form.wechat_connect_mobile_enabled"
                  data-testid="wechat-connect-mobile-enabled"
                  @update:model-value="handleWeChatMobileEnabledChange"
                />
              </div>
              <div
                v-if="form.wechat_connect_mobile_enabled"
                class="mt-4 grid grid-cols-1 gap-4 lg:grid-cols-2"
              >
                <div>
                  <label
                    class="input-label"
                  >
                    {{ t("admin.settings.wechatConnect.mobileAppIdLabel") }}
                  </label>
                  <input
                    v-model="form.wechat_connect_mobile_app_id"
                    data-testid="wechat-connect-mobile-app-id"
                    type="text"
                    class="input font-mono text-sm"
                    :placeholder="
                      t('admin.settings.wechatConnect.mobileAppIdLabel')
                    "
                  />
                </div>
                <div>
                  <label
                    class="input-label"
                  >
                    {{ t("admin.settings.wechatConnect.mobileAppSecretLabel") }}
                  </label>
                  <input
                    v-model="form.wechat_connect_mobile_app_secret"
                    data-testid="wechat-connect-mobile-app-secret"
                    type="password"
                    class="input font-mono text-sm"
                    :placeholder="
                      form.wechat_connect_mobile_app_secret_configured
                        ? t('admin.settings.wechatConnect.appSecretConfiguredPlaceholder')
                        : t('admin.settings.wechatConnect.mobileAppSecretLabel')
                    "
                  />
                </div>
              </div>
            </div>
          </div>

          <div
            v-if="
              form.wechat_connect_open_enabled &&
              (form.wechat_connect_mp_enabled ||
                form.wechat_connect_mobile_enabled)
            "
            class="border border-warning/40 bg-warning-weak px-4 py-3 text-sm text-warning-strong"
          >
            {{
              t("admin.settings.wechatConnect.sameOpenPlatformWarning")
            }}
          </div>

          <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <div>
              <label
                class="input-label"
              >
                {{
                  t("admin.settings.wechatConnect.browserRedirectUrlLabel")
                }}
              </label>
              <input
                data-testid="wechat-connect-redirect-url"
                v-model="form.wechat_connect_redirect_url"
                type="url"
                class="input font-mono text-sm"
                :placeholder="t('admin.settings.wechatConnect.redirectUrlPlaceholder')"
              />
              <p class="input-hint">
                {{
                  t("admin.settings.wechatConnect.browserRedirectUrlHint")
                }}
              </p>
              <div
                class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3"
              >
                <button
                  type="button"
                  class="btn btn-secondary btn-sm w-fit"
                  @click="setAndCopyWeChatRedirectUrl"
                >
                  {{ t("admin.settings.wechatConnect.generateAndCopy") }}
                </button>
                <code
                  v-if="wechatRedirectUrlSuggestion"
                  class="code select-all break-all px-2 py-1 text-xs text-fg-muted"
                >
                  {{ wechatRedirectUrlSuggestion }}
                </code>
              </div>
            </div>
          </div>

          <div>
            <label
              class="input-label"
            >
              {{ t("admin.settings.wechatConnect.frontendRedirectUrlLabel") }}
            </label>
            <input
              data-testid="wechat-connect-frontend-redirect-url"
              v-model="form.wechat_connect_frontend_redirect_url"
              type="text"
              class="input font-mono text-sm"
              :placeholder="t('admin.settings.wechatConnect.frontendRedirectUrlPlaceholder')"
            />
            <p class="input-hint">
              {{ t("admin.settings.wechatConnect.frontendRedirectUrlHint") }}
            </p>
          </div>
        </div>
      </div>
    </div>

    <!-- DingTalk Connect OAuth 登录 -->
    <div class="card">
      <div
        class="card-header"
      >
        <h2 class="card-title">
          {{ t("admin.settings.dingtalk.title") }}
        </h2>
        <p class="mt-1 text-sm text-fg-muted">
          {{ t("admin.settings.dingtalk.description") }}
        </p>
      </div>
      <div class="card-body space-y-5">
        <div class="flex items-center justify-between">
          <div>
            <label class="font-medium text-fg">{{
              t("admin.settings.dingtalk.enable")
            }}</label>
            <p class="text-sm text-fg-muted">
              {{ t("admin.settings.dingtalk.enableHint") }}
            </p>
          </div>
          <Toggle v-model="form.dingtalk_connect_enabled" />
        </div>

        <div
          v-if="form.dingtalk_connect_enabled"
          class="border-t border-border pt-4"
        >
          <div class="grid grid-cols-1 gap-6">
            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.dingtalk.clientId") }}
              </label>
              <input
                v-model="form.dingtalk_connect_client_id"
                type="text"
                class="input font-mono text-sm"
                :placeholder="
                  t('admin.settings.dingtalk.clientIdPlaceholder')
                "
              />
              <p class="input-hint">
                {{ t("admin.settings.dingtalk.clientIdHint") }}
              </p>
            </div>

            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.dingtalk.clientSecret") }}
              </label>
              <input
                v-model="form.dingtalk_connect_client_secret"
                type="password"
                class="input font-mono text-sm"
                :placeholder="
                  form.dingtalk_connect_client_secret_configured
                    ? t(
                        'admin.settings.dingtalk.clientSecretConfiguredPlaceholder',
                      )
                    : t('admin.settings.dingtalk.clientSecretPlaceholder')
                "
              />
              <p class="input-hint">
                {{
                  form.dingtalk_connect_client_secret_configured
                    ? t(
                        "admin.settings.dingtalk.clientSecretConfiguredHint",
                      )
                    : t("admin.settings.dingtalk.clientSecretHint")
                }}
              </p>
            </div>

            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.dingtalk.redirectUrl") }}
              </label>
              <input
                v-model="form.dingtalk_connect_redirect_url"
                type="url"
                class="input font-mono text-sm"
                :placeholder="
                  t('admin.settings.dingtalk.redirectUrlPlaceholder')
                "
              />
              <p class="input-hint">
                {{ t("admin.settings.dingtalk.redirectUrlHint") }}
              </p>
            </div>

            <!-- Corp Restriction Policy -->
            <div class="border-t border-border pt-4">
              <label class="input-label">
                {{ t("admin.settings.dingtalk.corpPolicy.label") }}
              </label>
              <p class="mb-3 text-xs text-fg-muted">
                {{ t("admin.settings.dingtalk.corpPolicy.hint") }}
              </p>
              <div class="space-y-2">
                <label class="flex cursor-pointer items-center gap-3">
                  <input
                    v-model="form.dingtalk_connect_corp_restriction_policy"
                    type="radio"
                    value="none"
                    class="h-4 w-4 text-accent"
                  />
                  <span class="text-sm text-fg">
                    {{ t("admin.settings.dingtalk.corpPolicy.none") }}
                  </span>
                </label>
                <label class="flex cursor-pointer items-center gap-3">
                  <input
                    v-model="form.dingtalk_connect_corp_restriction_policy"
                    type="radio"
                    value="internal_only"
                    class="h-4 w-4 text-accent"
                  />
                  <span class="text-sm text-fg">
                    {{ t("admin.settings.dingtalk.corpPolicy.internalOnly") }}
                  </span>
                </label>
              </div>
            </div>

            <!-- bypass_registration toggle（仅 internal_only 模式下可见可用） -->
            <div
              v-if="form.dingtalk_connect_corp_restriction_policy === 'internal_only'"
              class="flex items-center justify-between pt-4 border-t border-border"
            >
              <div>
                <label class="font-medium text-fg">{{
                  t("admin.settings.dingtalk.bypassRegistration")
                }}</label>
                <p class="text-sm text-fg-muted">
                  {{ t("admin.settings.dingtalk.bypassRegistrationHint") }}
                </p>
              </div>
              <Toggle v-model="form.dingtalk_connect_bypass_registration" />
            </div>

            <!-- 身份同步开关（仅 internal_only 模式下可见） -->
            <div
              v-if="form.dingtalk_connect_corp_restriction_policy === 'internal_only'"
              class="pt-4 border-t border-border space-y-2"
            >
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium text-fg">{{
                    t("admin.settings.dingtalk.syncDisplayName")
                  }}</label>
                  <p class="text-sm text-fg-muted">
                    {{ t("admin.settings.dingtalk.syncDisplayNameHint") }}
                  </p>
                </div>
                <Toggle v-model="form.dingtalk_connect_sync_display_name" />
              </div>
              <div v-if="form.dingtalk_connect_sync_display_name" class="space-y-2">
                <div class="flex items-center gap-2">
                  <label class="text-sm text-fg-muted whitespace-nowrap min-w-[5rem]">
                    {{ t("admin.settings.dingtalk.syncDisplayNameTarget") }}
                  </label>
                  <input
                    v-model="form.dingtalk_connect_sync_display_name_attr_key"
                    type="text"
                    placeholder="dingtalk_name"
                    class="input text-sm flex-1 max-w-xs"
                  />
                </div>
                <div class="flex items-center gap-2">
                  <label class="text-sm text-fg-muted whitespace-nowrap min-w-[5rem]">
                    {{ t("admin.settings.dingtalk.syncAttrDisplayName") }}
                  </label>
                  <input
                    v-model="form.dingtalk_connect_sync_display_name_attr_name"
                    type="text"
                    :placeholder="t('admin.settings.dingtalk.defaultAttrDisplayName')"
                    class="input text-sm flex-1 max-w-xs"
                  />
                </div>
              </div>
              <p v-if="form.dingtalk_connect_sync_display_name" class="text-xs text-fg-subtle">
                {{ t("admin.settings.dingtalk.syncDisplayNameTargetHint") }}
              </p>
            </div>
            <div
              v-if="form.dingtalk_connect_corp_restriction_policy === 'internal_only'"
              class="pt-4 border-t border-border space-y-2"
            >
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium text-fg">{{
                    t("admin.settings.dingtalk.syncCorpEmail")
                  }}</label>
                  <p class="text-sm text-fg-muted">
                    {{ t("admin.settings.dingtalk.syncCorpEmailHint") }}
                  </p>
                  <p class="text-xs text-warning mt-1">
                    {{ t("admin.settings.dingtalk.syncCorpEmailPermissionHint") }}
                  </p>
                </div>
                <Toggle v-model="form.dingtalk_connect_sync_corp_email" />
              </div>
              <div v-if="form.dingtalk_connect_sync_corp_email" class="space-y-2">
                <div class="flex items-center gap-2">
                  <label class="text-sm text-fg-muted whitespace-nowrap min-w-[5rem]">
                    {{ t("admin.settings.dingtalk.syncCorpEmailTarget") }}
                  </label>
                  <input
                    v-model="form.dingtalk_connect_sync_corp_email_attr_key"
                    type="text"
                    placeholder="dingtalk_email"
                    class="input text-sm flex-1 max-w-xs"
                  />
                </div>
                <div class="flex items-center gap-2">
                  <label class="text-sm text-fg-muted whitespace-nowrap min-w-[5rem]">
                    {{ t("admin.settings.dingtalk.syncAttrDisplayName") }}
                  </label>
                  <input
                    v-model="form.dingtalk_connect_sync_corp_email_attr_name"
                    type="text"
                    :placeholder="t('admin.settings.dingtalk.defaultAttrCorpEmail')"
                    class="input text-sm flex-1 max-w-xs"
                  />
                </div>
              </div>
              <p v-if="form.dingtalk_connect_sync_corp_email" class="text-xs text-fg-subtle">
                {{ t("admin.settings.dingtalk.syncCorpEmailTargetHint") }}
              </p>
            </div>
            <div
              v-if="form.dingtalk_connect_corp_restriction_policy === 'internal_only'"
              class="pt-4 border-t border-border space-y-2"
            >
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium text-fg">{{
                    t("admin.settings.dingtalk.syncDept")
                  }}</label>
                  <p class="text-sm text-fg-muted">
                    {{ t("admin.settings.dingtalk.syncDeptHint") }}
                  </p>
                  <p class="text-xs text-warning mt-1">
                    {{ t("admin.settings.dingtalk.syncDeptPermissionHint") }}
                  </p>
                </div>
                <Toggle v-model="form.dingtalk_connect_sync_dept" />
              </div>
              <div v-if="form.dingtalk_connect_sync_dept" class="space-y-2">
                <div class="flex items-center gap-2">
                  <label class="text-sm text-fg-muted whitespace-nowrap min-w-[5rem]">
                    {{ t("admin.settings.dingtalk.syncDeptTarget") }}
                  </label>
                  <input
                    v-model="form.dingtalk_connect_sync_dept_attr_key"
                    type="text"
                    placeholder="dingtalk_department"
                    class="input text-sm flex-1 max-w-xs"
                  />
                </div>
                <div class="flex items-center gap-2">
                  <label class="text-sm text-fg-muted whitespace-nowrap min-w-[5rem]">
                    {{ t("admin.settings.dingtalk.syncAttrDisplayName") }}
                  </label>
                  <input
                    v-model="form.dingtalk_connect_sync_dept_attr_name"
                    type="text"
                    :placeholder="t('admin.settings.dingtalk.defaultAttrDept')"
                    class="input text-sm flex-1 max-w-xs"
                  />
                </div>
              </div>
              <p v-if="form.dingtalk_connect_sync_dept" class="text-xs text-fg-subtle">
                {{ t("admin.settings.dingtalk.syncDeptTargetHint") }}
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Generic OIDC OAuth 登录 -->
    <div class="card">
      <div
        class="card-header"
      >
        <h2 class="card-title">
          {{ t("admin.settings.oidc.title") }}
        </h2>
        <p class="mt-1 text-sm text-fg-muted">
          {{ t("admin.settings.oidc.description") }}
        </p>
      </div>
      <div class="card-body space-y-5">
        <div class="flex items-center justify-between">
          <div>
            <label class="font-medium text-fg">{{
              t("admin.settings.oidc.enable")
            }}</label>
            <p class="text-sm text-fg-muted">
              {{ t("admin.settings.oidc.enableHint") }}
            </p>
          </div>
          <Toggle v-model="form.oidc_connect_enabled" />
        </div>

        <div
          v-if="form.oidc_connect_enabled"
          class="space-y-6 border-t border-border pt-4"
        >
          <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.oidc.providerName") }}
              </label>
              <input
                v-model="form.oidc_connect_provider_name"
                type="text"
                class="input"
                :placeholder="
                  t('admin.settings.oidc.providerNamePlaceholder')
                "
              />
            </div>

            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.oidc.clientId") }}
              </label>
              <input
                v-model="form.oidc_connect_client_id"
                type="text"
                class="input font-mono text-sm"
                :placeholder="
                  t('admin.settings.oidc.clientIdPlaceholder')
                "
              />
            </div>

            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.oidc.clientSecret") }}
              </label>
              <input
                v-model="form.oidc_connect_client_secret"
                type="password"
                class="input font-mono text-sm"
                :placeholder="
                  form.oidc_connect_client_secret_configured
                    ? t(
                        'admin.settings.oidc.clientSecretConfiguredPlaceholder',
                      )
                    : t('admin.settings.oidc.clientSecretPlaceholder')
                "
              />
              <p class="input-hint">
                {{
                  form.oidc_connect_client_secret_configured
                    ? t("admin.settings.oidc.clientSecretConfiguredHint")
                    : t("admin.settings.oidc.clientSecretHint")
                }}
              </p>
            </div>
          </div>

          <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.oidc.issuerUrl") }}
              </label>
              <input
                v-model="form.oidc_connect_issuer_url"
                type="url"
                class="input font-mono text-sm"
                :placeholder="
                  t('admin.settings.oidc.issuerUrlPlaceholder')
                "
              />
            </div>

            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.oidc.discoveryUrl") }}
              </label>
              <input
                v-model="form.oidc_connect_discovery_url"
                type="url"
                class="input font-mono text-sm"
                :placeholder="
                  t('admin.settings.oidc.discoveryUrlPlaceholder')
                "
              />
            </div>

            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.oidc.authorizeUrl") }}
              </label>
              <input
                v-model="form.oidc_connect_authorize_url"
                type="url"
                class="input font-mono text-sm"
                :placeholder="
                  t('admin.settings.oidc.authorizeUrlPlaceholder')
                "
              />
            </div>

            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.oidc.tokenUrl") }}
              </label>
              <input
                v-model="form.oidc_connect_token_url"
                type="url"
                class="input font-mono text-sm"
                :placeholder="
                  t('admin.settings.oidc.tokenUrlPlaceholder')
                "
              />
            </div>

            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.oidc.userinfoUrl") }}
              </label>
              <input
                v-model="form.oidc_connect_userinfo_url"
                type="url"
                class="input font-mono text-sm"
                :placeholder="
                  t('admin.settings.oidc.userinfoUrlPlaceholder')
                "
              />
            </div>

            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.oidc.jwksUrl") }}
              </label>
              <input
                v-model="form.oidc_connect_jwks_url"
                type="url"
                class="input font-mono text-sm"
                :placeholder="t('admin.settings.oidc.jwksUrlPlaceholder')"
              />
            </div>
          </div>

          <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.oidc.scopes") }}
              </label>
              <input
                v-model="form.oidc_connect_scopes"
                type="text"
                class="input font-mono text-sm"
                :placeholder="t('admin.settings.oidc.scopesPlaceholder')"
              />
              <p class="input-hint">
                {{ t("admin.settings.oidc.scopesHint") }}
              </p>
            </div>

            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.oidc.redirectUrl") }}
              </label>
              <input
                v-model="form.oidc_connect_redirect_url"
                type="url"
                class="input font-mono text-sm"
                :placeholder="
                  t('admin.settings.oidc.redirectUrlPlaceholder')
                "
              />
              <div
                class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-3"
              >
                <button
                  type="button"
                  class="btn btn-secondary btn-sm w-fit"
                  @click="setAndCopyOIDCRedirectUrl"
                >
                  {{ t("admin.settings.oidc.quickSetCopy") }}
                </button>
                <code
                  v-if="oidcRedirectUrlSuggestion"
                  class="code select-all break-all px-2 py-1 text-xs text-fg-muted"
                >
                  {{ oidcRedirectUrlSuggestion }}
                </code>
              </div>
              <p class="input-hint">
                {{ t("admin.settings.oidc.redirectUrlHint") }}
              </p>
            </div>

            <div class="lg:col-span-2">
              <label
                class="input-label"
              >
                {{ t("admin.settings.oidc.frontendRedirectUrl") }}
              </label>
              <input
                v-model="form.oidc_connect_frontend_redirect_url"
                type="text"
                class="input font-mono text-sm"
                :placeholder="
                  t('admin.settings.oidc.frontendRedirectUrlPlaceholder')
                "
              />
              <p class="input-hint">
                {{ t("admin.settings.oidc.frontendRedirectUrlHint") }}
              </p>
            </div>
          </div>

          <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.oidc.tokenAuthMethod") }}
              </label>
              <select
                v-model="form.oidc_connect_token_auth_method"
                class="input font-mono text-sm"
              >
                <option value="client_secret_post">
                  client_secret_post
                </option>
                <option value="client_secret_basic">
                  client_secret_basic
                </option>
                <option value="none">none</option>
              </select>
            </div>

            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.oidc.clockSkewSeconds") }}
              </label>
              <input
                v-model.number="form.oidc_connect_clock_skew_seconds"
                type="number"
                min="0"
                max="600"
                class="input"
              />
            </div>

            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.oidc.allowedSigningAlgs") }}
              </label>
              <input
                v-model="form.oidc_connect_allowed_signing_algs"
                type="text"
                class="input font-mono text-sm"
                :placeholder="
                  t('admin.settings.oidc.allowedSigningAlgsPlaceholder')
                "
              />
            </div>
          </div>

          <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
            <div
              class="flex items-center justify-between gap-4 border-t border-border pt-4"
            >
              <div>
                <label class="font-medium text-fg">
                  {{ t("admin.settings.oidc.usePkce") }}
                </label>
              </div>
              <Toggle
                v-model="form.oidc_connect_use_pkce"
                data-testid="oidc-connect-use-pkce"
              />
            </div>

            <div
              class="flex items-center justify-between gap-4 border-t border-border pt-4"
            >
              <div>
                <label class="font-medium text-fg">
                  {{ t("admin.settings.oidc.validateIdToken") }}
                </label>
              </div>
              <Toggle
                v-model="form.oidc_connect_validate_id_token"
                data-testid="oidc-connect-validate-id-token"
              />
            </div>

            <div
              class="flex items-center justify-between gap-4 border-t border-border pt-4"
            >
              <div>
                <label class="font-medium text-fg">
                  {{ t("admin.settings.oidc.requireEmailVerified") }}
                </label>
              </div>
              <Toggle
                v-model="form.oidc_connect_require_email_verified"
              />
            </div>
          </div>

          <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.oidc.userinfoEmailPath") }}
              </label>
              <input
                v-model="form.oidc_connect_userinfo_email_path"
                type="text"
                class="input font-mono text-sm"
                :placeholder="
                  t('admin.settings.oidc.userinfoEmailPathPlaceholder')
                "
              />
            </div>

            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.oidc.userinfoIdPath") }}
              </label>
              <input
                v-model="form.oidc_connect_userinfo_id_path"
                type="text"
                class="input font-mono text-sm"
                :placeholder="
                  t('admin.settings.oidc.userinfoIdPathPlaceholder')
                "
              />
            </div>

            <div>
              <label
                class="input-label"
              >
                {{ t("admin.settings.oidc.userinfoUsernamePath") }}
              </label>
              <input
                v-model="form.oidc_connect_userinfo_username_path"
                type="text"
                class="input font-mono text-sm"
                :placeholder="
                  t('admin.settings.oidc.userinfoUsernamePathPlaceholder')
                "
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useSettingsViewContext } from "./context";
import Icon from "@/components/icons/Icon.vue";
import Toggle from "@/components/common/Toggle.vue";

// 纯移动拆分：所有状态与方法来自 SettingsView 提供的上下文（openspec: rebuild-frontend-design-system Phase 3）
const ctx = useSettingsViewContext();
const {
  activeTab,
  adminApiKeyExists,
  adminApiKeyLoading,
  adminApiKeyMasked,
  adminApiKeyOperating,
  captchaMasterEnabled,
  captchaProviderSelection,
  commitForwardedClientIpHeaderDraft,
  commitRegistrationEmailSuffixWhitelistDraft,
  copyNewKey,
  createAdminApiKey,
  deleteAdminApiKey,
  form,
  forwardedClientIpHeaderDraft,
  githubOAuthRedirectUrlSuggestion,
  googleOAuthRedirectUrlSuggestion,
  handleForwardedClientIpHeaderKeydown,
  handleForwardedClientIpHeaderPaste,
  handleRegistrationEmailSuffixWhitelistDraftInput,
  handleRegistrationEmailSuffixWhitelistDraftKeydown,
  handleRegistrationEmailSuffixWhitelistPaste,
  handleWeChatMPEnabledChange,
  handleWeChatMobileEnabledChange,
  handleWeChatOpenEnabledChange,
  linuxdoRedirectUrlSuggestion,
  newAdminApiKey,
  oidcRedirectUrlSuggestion,
  panelRateLimitForm,
  panelRateLimitLoading,
  panelRateLimitSaving,
  regenerateAdminApiKey,
  registrationEmailSuffixWhitelistDraft,
  registrationEmailSuffixWhitelistTags,
  removeForwardedClientIpHeader,
  removeRegistrationEmailSuffixWhitelistTag,
  savePanelRateLimitSettings,
  selectCaptchaProvider,
  setAndCopyEmailOAuthRedirectUrl,
  setAndCopyLinuxdoRedirectUrl,
  setAndCopyOIDCRedirectUrl,
  setAndCopyTelegramRedirectUrl,
  setAndCopyWeChatRedirectUrl,
  t,
  telegramRedirectUrlSuggestion,
  tencentCaptchaLinks,
  wechatRedirectUrlSuggestion,
} = ctx;
</script>
