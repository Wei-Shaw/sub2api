<template>
  <div v-show="activeTab === 'security'" class="space-y-6">
    <!-- Registration Settings -->
    <div class="card">
      <div
        class="border-b border-line-subtle px-6 py-4"
      >
        <h2 class="text-lg font-semibold text-ink">
          {{ t("admin.settings.registration.title") }}
        </h2>
        <p class="mt-1 text-sm text-ink-secondary">
          {{ t("admin.settings.registration.description") }}
        </p>
      </div>
      <div class="space-y-5 p-6">
        <!-- Enable Registration -->
        <div class="flex items-center justify-between">
          <div>
            <label class="font-medium text-ink">{{
              t("admin.settings.registration.enableRegistration")
            }}</label>
            <p class="text-sm text-ink-secondary">
              {{
                t("admin.settings.registration.enableRegistrationHint")
              }}
            </p>
          </div>
          <Toggle v-model="form.registration_enabled" />
        </div>

        <!-- Email Verification -->
        <div
          class="flex items-center justify-between border-t border-line-subtle pt-4"
        >
          <div>
            <label class="font-medium text-ink">{{
              t("admin.settings.registration.emailVerification")
            }}</label>
            <p class="text-sm text-ink-secondary">
              {{ t("admin.settings.registration.emailVerificationHint") }}
            </p>
          </div>
          <Toggle v-model="form.email_verify_enabled" />
        </div>

        <!-- Email Suffix Whitelist -->
        <div class="border-t border-line-subtle pt-4">
          <label class="font-medium text-ink">{{
            t("admin.settings.registration.emailSuffixWhitelist")
          }}</label>
          <p class="mt-1 text-sm text-ink-secondary">
            {{
              t("admin.settings.registration.emailSuffixWhitelistHint")
            }}
          </p>
          <div
            class="mt-3 rounded-sm border border-line bg-surface p-2"
          >
            <div class="flex flex-wrap items-center gap-2">
              <span
                v-for="suffix in registrationEmailSuffixWhitelistTags"
                :key="suffix"
                class="inline-flex items-center gap-1 rounded bg-surface-sunken px-2 py-1 text-xs font-mono text-ink-secondary"
              >
                <span>{{ suffix }}</span>
                <button
                  type="button"
                  class="rounded-full text-ink-secondary hover:bg-surface-hover hover:text-ink-secondary"
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
                class="flex min-w-[220px] flex-1 items-center gap-1 rounded border border-transparent px-2 py-1 focus-within:border-accent dark:focus-within:border-accent"
              >
                <input
                  v-model="registrationEmailSuffixWhitelistDraft"
                  type="text"
                  class="w-full bg-transparent text-sm font-mono text-ink outline-none placeholder:text-ink-tertiary dark:placeholder:text-ink-tertiary"
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
          <p class="mt-2 text-xs text-ink-secondary">
            {{
              t(
                "admin.settings.registration.emailSuffixWhitelistInputHint",
              )
            }}
          </p>
        </div>

        <!-- Email Domain Quota -->
        <div
          class="flex items-center justify-between border-t border-line-subtle pt-4"
        >
          <div>
            <label class="font-medium text-ink">{{
              t("admin.settings.registration.emailDomainQuota")
            }}</label>
            <p class="text-sm text-ink-secondary">
              {{ t("admin.settings.registration.emailDomainQuotaHint") }}
            </p>
          </div>
          <Toggle
            v-model="form.registration_email_domain_quota_enabled"
          />
        </div>

        <!-- Promo Code -->
        <div
          class="flex items-center justify-between border-t border-line-subtle pt-4"
        >
          <div>
            <label class="font-medium text-ink">{{
              t("admin.settings.registration.promoCode")
            }}</label>
            <p class="text-sm text-ink-secondary">
              {{ t("admin.settings.registration.promoCodeHint") }}
            </p>
          </div>
          <Toggle v-model="form.promo_code_enabled" />
        </div>

        <!-- Invitation Code -->
        <div
          class="flex items-center justify-between border-t border-line-subtle pt-4"
        >
          <div>
            <label class="font-medium text-ink">{{
              t("admin.settings.registration.invitationCode")
            }}</label>
            <p class="text-sm text-ink-secondary">
              {{ t("admin.settings.registration.invitationCodeHint") }}
            </p>
          </div>
          <Toggle v-model="form.invitation_code_enabled" />
        </div>
        <!-- Password Reset - Only show when email verification is enabled -->
        <div
          v-if="form.email_verify_enabled"
          class="flex items-center justify-between border-t border-line-subtle pt-4"
        >
          <div>
            <label class="font-medium text-ink">{{
              t("admin.settings.registration.passwordReset")
            }}</label>
            <p class="text-sm text-ink-secondary">
              {{ t("admin.settings.registration.passwordResetHint") }}
            </p>
          </div>
          <Toggle v-model="form.password_reset_enabled" />
        </div>
        <!-- Frontend URL - Only show when password reset is enabled -->
        <div
          v-if="form.email_verify_enabled && form.password_reset_enabled"
          class="border-t border-line-subtle pt-4"
        >
          <label
            class="mb-2 block text-sm font-medium text-ink-secondary"
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
          <p class="mt-1.5 text-xs text-ink-secondary">
            {{ t("admin.settings.registration.frontendUrlHint") }}
          </p>
        </div>

        <!-- TOTP 2FA -->
        <div
          class="flex items-center justify-between border-t border-line-subtle pt-4"
        >
          <div>
            <label class="font-medium text-ink">{{
              t("admin.settings.registration.totp")
            }}</label>
            <p class="text-sm text-ink-secondary">
              {{ t("admin.settings.registration.totpHint") }}
            </p>
            <!-- Warning when encryption key not configured -->
            <p
              v-if="!form.totp_encryption_key_configured"
              class="mt-2 text-sm text-warn"
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
          class="border-t border-line-subtle pt-4"
          data-testid="passkey-settings"
        >
          <div class="flex items-start justify-between gap-4">
            <div>
              <label class="font-medium text-ink">{{
                t("admin.settings.security.passkey")
              }}</label>
              <p class="text-sm text-ink-secondary">
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
            class="mt-3 rounded-sm border px-3 py-2 text-sm"
            :class="
 form.passkey_configured
 ? 'border-success/40 bg-green-50 text-success dark:border-green-900 dark:bg-green-950/40'
 : 'border-amber-200 bg-amber-50 text-warn dark:border-amber-900 dark:bg-amber-950/40'
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
          class="flex items-center justify-between border-t border-line-subtle pt-4"
        >
          <div>
            <label class="font-medium text-ink">{{
              t("admin.settings.security.stepUp")
            }}</label>
            <p class="text-sm text-ink-secondary">
              {{ t("admin.settings.security.stepUpHint") }}
            </p>
          </div>
          <Toggle v-model="form.step_up_enabled" />
        </div>

        <!-- 会话 IP/UA 绑定 -->
        <div
          class="flex items-center justify-between border-t border-line-subtle pt-4"
        >
          <div>
            <label class="font-medium text-ink">{{
              t("admin.settings.security.sessionBinding")
            }}</label>
            <p class="text-sm text-ink-secondary">
              {{ t("admin.settings.security.sessionBindingHint") }}
            </p>
          </div>
          <Toggle v-model="form.session_binding_enabled" />
        </div>

        <!-- 审计日志保留天数 -->
        <div
          class="flex items-center justify-between border-t border-line-subtle pt-4"
        >
          <div>
            <label class="font-medium text-ink">{{
              t("admin.settings.security.auditRetention")
            }}</label>
            <p class="text-sm text-ink-secondary">
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
        class="border-b border-line-subtle px-6 py-4"
      >
        <h2 class="text-lg font-semibold text-ink">
          {{ t("admin.settings.apiKeyAcl.title") }}
        </h2>
        <p class="mt-1 text-sm text-ink-secondary">
          {{ t("admin.settings.apiKeyAcl.description") }}
        </p>
      </div>
      <div class="space-y-5 p-6">
        <div class="flex items-center justify-between gap-4">
          <div>
            <label class="font-medium text-ink">
              {{ t("admin.settings.apiKeyAcl.trustForwardedIp") }}
            </label>
            <p class="text-sm text-ink-secondary">
              {{ t("admin.settings.apiKeyAcl.trustForwardedIpHint") }}
            </p>
          </div>
          <Toggle v-model="form.api_key_acl_trust_forwarded_ip" />
        </div>

        <div
          v-if="form.api_key_acl_trust_forwarded_ip"
          class="border-t border-line-subtle pt-4"
        >
          <label
            for="forwarded-client-ip-headers"
            class="font-medium text-ink"
          >
            {{ t("admin.settings.apiKeyAcl.forwardedClientIpHeaders") }}
          </label>
          <p class="mt-1 text-sm text-ink-secondary">
            {{ t("admin.settings.apiKeyAcl.forwardedClientIpHeadersHint") }}
          </p>
          <div
            class="mt-3 rounded-sm border border-line bg-surface p-2"
          >
            <div class="flex flex-wrap items-center gap-2">
              <span
                v-for="header in form.forwarded_client_ip_headers"
                :key="header"
                data-testid="forwarded-client-ip-header-tag"
                class="inline-flex items-center gap-1 rounded bg-surface-sunken px-2 py-1 text-xs font-mono text-ink-secondary"
              >
                <span>{{ header }}</span>
                <button
                  type="button"
                  class="rounded-full text-ink-secondary hover:bg-surface-hover hover:text-ink-secondary"
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
                class="flex min-w-[220px] flex-1 items-center gap-1 rounded border border-transparent px-2 py-1 focus-within:border-accent dark:focus-within:border-accent"
              >
                <input
                  id="forwarded-client-ip-headers"
                  v-model="forwardedClientIpHeaderDraft"
                  data-testid="forwarded-client-ip-headers-input"
                  type="text"
                  class="w-full bg-transparent text-sm font-mono text-ink outline-none placeholder:text-ink-tertiary dark:placeholder:text-ink-tertiary"
                  :placeholder="t('admin.settings.apiKeyAcl.forwardedClientIpHeadersPlaceholder')"
                  @keydown="handleForwardedClientIpHeaderKeydown"
                  @blur="commitForwardedClientIpHeaderDraft"
                  @paste="handleForwardedClientIpHeaderPaste"
                />
              </div>
            </div>
          </div>
          <p class="mt-2 text-xs text-ink-secondary">
            {{ t("admin.settings.apiKeyAcl.forwardedClientIpHeadersRiskHint") }}
          </p>
        </div>
      </div>
    </div>

    <!-- Panel API Rate Limit Settings -->
    <div class="card">
      <div
        class="border-b border-line-subtle px-6 py-4"
      >
        <div class="flex items-center gap-2">
          <Icon
            name="shield"
            size="md"
            class="text-accent"
          />
          <h2 class="text-lg font-semibold text-ink">
            {{ t("admin.settings.panelRateLimit.title") }}
          </h2>
        </div>
        <p class="mt-1 text-sm text-ink-secondary">
          {{ t("admin.settings.panelRateLimit.description") }}
        </p>
      </div>
      <div class="space-y-5 p-6">
        <div
          v-if="panelRateLimitLoading"
          class="flex items-center gap-2 text-ink-secondary"
        >
          <div
            class="h-4 w-4 animate-spin rounded-full border-b-2 border-accent-line"
          ></div>
          {{ t("common.loading") }}
        </div>

        <template v-else>
          <!-- 计数维度说明：按账号计数，反代部署无误伤 -->
          <div
            class="rounded-sm border border border-info/40 bg-info-tint p-4"
          >
            <div class="flex items-start">
              <Icon
                name="infoCircle"
                size="md"
                class="mt-0.5 flex-shrink-0 text-info"
              />
              <p class="ml-3 text-sm text-info">
                {{ t("admin.settings.panelRateLimit.proxySafeNote") }}
              </p>
            </div>
          </div>

          <div class="flex items-center justify-between">
            <div>
              <label class="font-medium text-ink">{{
                t("admin.settings.panelRateLimit.enabled")
              }}</label>
              <p class="text-sm text-ink-secondary">
                {{ t("admin.settings.panelRateLimit.enabledHint") }}
              </p>
            </div>
            <Toggle v-model="panelRateLimitForm.enabled" />
          </div>

          <div
            v-if="panelRateLimitForm.enabled"
            class="space-y-5 border-t border-line-subtle pt-4"
          >
            <div class="grid grid-cols-1 gap-6 sm:grid-cols-2">
              <div>
                <label
                  class="mb-2 block text-sm font-medium text-ink-secondary"
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
                  <span class="text-sm text-ink-secondary">
                    {{ t("admin.settings.panelRateLimit.perMinute") }}
                  </span>
                </div>
                <p class="mt-1.5 text-xs text-ink-secondary">
                  {{ t("admin.settings.panelRateLimit.userRpmHint") }}
                </p>
              </div>

              <div>
                <label
                  class="mb-2 block text-sm font-medium text-ink-secondary"
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
                  <span class="text-sm text-ink-secondary">
                    {{ t("admin.settings.panelRateLimit.perMinute") }}
                  </span>
                </div>
                <p class="mt-1.5 text-xs text-ink-secondary">
                  {{ t("admin.settings.panelRateLimit.heavyRpmHint") }}
                </p>
              </div>

              <div>
                <label
                  class="mb-2 block text-sm font-medium text-ink-secondary"
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
                  <span class="text-sm text-ink-secondary">
                    {{ t("admin.settings.panelRateLimit.perMinute") }}
                  </span>
                </div>
                <p class="mt-1.5 text-xs text-ink-secondary">
                  {{ t("admin.settings.panelRateLimit.publicIpRpmHint") }}
                </p>
              </div>
            </div>

            <div
              class="flex items-center justify-between border-t border-line-subtle pt-4"
            >
              <div>
                <label class="font-medium text-ink">{{
                  t("admin.settings.panelRateLimit.exemptAdmin")
                }}</label>
                <p class="text-sm text-ink-secondary">
                  {{ t("admin.settings.panelRateLimit.exemptAdminHint") }}
                </p>
              </div>
              <Toggle v-model="panelRateLimitForm.exempt_admin" />
            </div>
          </div>

          <div
            class="flex justify-end border-t border-line-subtle pt-4"
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
        class="border-b border-line-subtle px-6 py-4"
      >
        <h2 class="text-lg font-semibold text-ink">
          {{ t("admin.settings.captcha.title") }}
        </h2>
        <p class="mt-1 text-sm text-ink-secondary">
          {{ t("admin.settings.captcha.description") }}
        </p>
      </div>
      <div class="space-y-5 p-6">
        <!-- Enable Captcha -->
        <div class="flex items-center justify-between">
          <div>
            <label class="font-medium text-ink">{{
              t("admin.settings.captcha.enable")
            }}</label>
            <p class="text-sm text-ink-secondary">
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
          class="border-t border-line-subtle pt-4"
        >
          <!-- Provider Selector -->
          <div class="mb-6">
            <label
              class="mb-2 block text-sm font-medium text-ink-secondary"
            >
              {{ t("admin.settings.captcha.provider") }}
            </label>
            <div
              class="grid grid-cols-3 gap-2 rounded-sm bg-surface-sunken p-1"
            >
              <button
                type="button"
                data-testid="captcha-provider-turnstile"
                class="inline-flex items-center justify-center gap-2 rounded-sm px-3 py-2 text-sm font-medium transition-colors"
                :class="
 captchaProviderSelection === 'turnstile'
 ? 'bg-surface text-accent shadow-sm '
 : 'text-ink-secondary hover:text-gray-900 '
"
                @click="selectCaptchaProvider('turnstile')"
              >
                {{ t("admin.settings.captcha.providerTurnstile") }}
              </button>
              <button
                type="button"
                data-testid="captcha-provider-tencent"
                class="inline-flex items-center justify-center gap-2 rounded-sm px-3 py-2 text-sm font-medium transition-colors"
                :class="
 captchaProviderSelection === 'tencent'
 ? 'bg-surface text-accent shadow-sm '
 : 'text-ink-secondary hover:text-gray-900 '
"
                @click="selectCaptchaProvider('tencent')"
              >
                {{ t("admin.settings.captcha.providerTencent") }}
              </button>
              <button
                type="button"
                data-testid="captcha-provider-aliyun"
                class="inline-flex items-center justify-center gap-2 rounded-sm px-3 py-2 text-sm font-medium transition-colors"
                :class="
 captchaProviderSelection === 'aliyun'
 ? 'bg-surface text-accent shadow-sm '
 : 'text-ink-secondary hover:text-gray-900 '
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
                class="mb-2 block text-sm font-medium text-ink-secondary"
              >
                {{ t("admin.settings.turnstile.siteKey") }}
              </label>
              <input
                v-model="form.turnstile_site_key"
                type="text"
                class="input font-mono text-sm"
                placeholder="0x4AAAAAAA..."
              />
              <p class="mt-1.5 text-xs text-ink-secondary">
                {{ t("admin.settings.turnstile.siteKeyHint") }}
                <a
                  href="https://dash.cloudflare.com/"
                  target="_blank"
                  class="text-accent hover:text-accent-hover"
                  >{{
                    t("admin.settings.turnstile.cloudflareDashboard")
                  }}</a
                >
              </p>
            </div>
            <div>
              <label
                class="mb-2 block text-sm font-medium text-ink-secondary"
              >
                {{ t("admin.settings.turnstile.secretKey") }}
              </label>
              <input
                v-model="form.turnstile_secret_key"
                type="password"
                class="input font-mono text-sm"
                placeholder="0x4AAAAAAA..."
              />
              <p class="mt-1.5 text-xs text-ink-secondary">
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
              <label class="mb-2 block text-sm font-medium text-ink-secondary">
                {{ t("admin.settings.tencentCaptcha.region") }}
              </label>
              <div class="grid grid-cols-2 gap-2 rounded-sm bg-surface-sunken p-1">
                <button
                  type="button"
                  data-testid="tencent-captcha-region-cn"
                  class="inline-flex items-center justify-center rounded-sm px-3 py-1.5 text-sm font-medium transition-colors"
                  :class="
 form.tencent_captcha_region !== 'intl'
 ? 'bg-surface text-accent shadow-sm '
 : 'text-ink-secondary hover:text-gray-900 '
"
                  @click="form.tencent_captcha_region = 'cn'"
                >
                  {{ t("admin.settings.tencentCaptcha.regionCn") }}
                </button>
                <button
                  type="button"
                  data-testid="tencent-captcha-region-intl"
                  class="inline-flex items-center justify-center rounded-sm px-3 py-1.5 text-sm font-medium transition-colors"
                  :class="
 form.tencent_captcha_region === 'intl'
 ? 'bg-surface text-accent shadow-sm '
 : 'text-ink-secondary hover:text-gray-900 '
"
                  @click="form.tencent_captcha_region = 'intl'"
                >
                  {{ t("admin.settings.tencentCaptcha.regionIntl") }}
                </button>
              </div>
              <p class="mt-1.5 text-xs text-ink-secondary">
                {{ t("admin.settings.tencentCaptcha.regionHint") }}
              </p>
            </div>
            <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
              <div class="md:col-span-2">
                <h3 class="text-sm font-semibold text-ink">
                  {{ t("admin.settings.tencentCaptcha.appCredentialsTitle") }}
                </h3>
                <p class="mt-1 text-xs text-ink-secondary">
                  {{ t("admin.settings.tencentCaptcha.appCredentialsHint") }}
                </p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-ink-secondary">
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
                <label class="mb-2 block text-sm font-medium text-ink-secondary">
                  {{ t("admin.settings.tencentCaptcha.appSecretKey") }}
                </label>
                <input
                  v-model="form.tencent_captcha_app_secret_key"
                  type="password"
                  autocomplete="new-password"
                  class="input font-mono text-sm"
                  :placeholder="t('admin.settings.tencentCaptcha.keepExisting')"
                />
                <p class="mt-1.5 text-xs text-ink-secondary">
                  {{ form.tencent_captcha_app_secret_key_configured ? t("admin.settings.tencentCaptcha.configured") : t("admin.settings.tencentCaptcha.required") }}
                </p>
              </div>
              <div class="border-t border-line-subtle pt-5 md:col-span-2">
                <h3 class="text-sm font-semibold text-ink">
                  {{ t("admin.settings.tencentCaptcha.cloudCredentialsTitle") }}
                </h3>
                <p class="mt-1 text-xs text-ink-secondary">
                  {{ t("admin.settings.tencentCaptcha.cloudCredentialsHint") }}
                </p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-ink-secondary">
                  {{ t("admin.settings.tencentCaptcha.cloudSecretId") }}
                </label>
                <input
                  v-model="form.tencent_captcha_cloud_secret_id"
                  type="password"
                  autocomplete="new-password"
                  class="input font-mono text-sm"
                  :placeholder="t('admin.settings.tencentCaptcha.keepExisting')"
                />
                <p class="mt-1.5 text-xs text-ink-secondary">
                  {{ form.tencent_captcha_cloud_secret_id_configured ? t("admin.settings.tencentCaptcha.configured") : t("admin.settings.tencentCaptcha.required") }}
                </p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-ink-secondary">
                  {{ t("admin.settings.tencentCaptcha.cloudSecretKey") }}
                </label>
                <input
                  v-model="form.tencent_captcha_cloud_secret_key"
                  type="password"
                  autocomplete="new-password"
                  class="input font-mono text-sm"
                  :placeholder="t('admin.settings.tencentCaptcha.keepExisting')"
                />
                <p class="mt-1.5 text-xs text-ink-secondary">
                  {{ form.tencent_captcha_cloud_secret_key_configured ? t("admin.settings.tencentCaptcha.configured") : t("admin.settings.tencentCaptcha.required") }}
                </p>
              </div>
            </div>
            <p class="mt-5 text-xs text-ink-secondary">
              {{ t("admin.settings.tencentCaptcha.camPermissionHint") }}
            </p>
            <p class="mt-2 text-xs text-ink-secondary">
              {{ t("admin.settings.tencentCaptcha.aidEncryptedHint") }}
            </p>
            <div class="mt-3 flex flex-wrap gap-x-4 gap-y-2 text-sm">
              <a
                :href="tencentCaptchaLinks.console"
                target="_blank"
                rel="noopener noreferrer"
                class="text-accent hover:text-accent-hover"
              >
                {{ t("admin.settings.tencentCaptcha.openCaptchaConsole") }}
              </a>
              <a
                :href="tencentCaptchaLinks.cloudKeys"
                target="_blank"
                rel="noopener noreferrer"
                class="text-accent hover:text-accent-hover"
              >
                {{ t("admin.settings.tencentCaptcha.createCloudKeys") }}
              </a>
              <a
                :href="tencentCaptchaLinks.webDocs"
                target="_blank"
                rel="noopener noreferrer"
                class="text-accent hover:text-accent-hover"
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
                  class="mb-2 block text-sm font-medium text-ink-secondary"
                >
                  {{ t("admin.settings.aliyunCaptcha.region") }}
                </label>
                <div
                  class="grid grid-cols-2 gap-2 rounded-sm bg-surface-sunken p-1"
                >
                  <button
                    type="button"
                    class="inline-flex items-center justify-center rounded-sm px-3 py-1.5 text-sm font-medium transition-colors"
                    :class="
 form.aliyun_captcha_region !== 'sgp'
 ? 'bg-surface text-accent shadow-sm '
 : 'text-ink-secondary hover:text-gray-900 '
"
                    @click="form.aliyun_captcha_region = 'cn'"
                  >
                    {{ t("admin.settings.aliyunCaptcha.regionCn") }}
                  </button>
                  <button
                    type="button"
                    class="inline-flex items-center justify-center rounded-sm px-3 py-1.5 text-sm font-medium transition-colors"
                    :class="
 form.aliyun_captcha_region === 'sgp'
 ? 'bg-surface text-accent shadow-sm '
 : 'text-ink-secondary hover:text-gray-900 '
"
                    @click="form.aliyun_captcha_region = 'sgp'"
                  >
                    {{ t("admin.settings.aliyunCaptcha.regionSgp") }}
                  </button>
                </div>
                <p class="mt-1.5 text-xs text-ink-secondary">
                  {{ t("admin.settings.aliyunCaptcha.regionHint") }}
                </p>
              </div>
              <div>
                <label
                  class="mb-2 block text-sm font-medium text-ink-secondary"
                >
                  {{ t("admin.settings.aliyunCaptcha.prefix") }}
                </label>
                <input
                  v-model="form.aliyun_captcha_prefix"
                  type="text"
                  class="input font-mono text-sm"
                  placeholder="14xxxxx"
                />
                <p class="mt-1.5 text-xs text-ink-secondary">
                  {{ t("admin.settings.aliyunCaptcha.prefixHint") }}
                </p>
              </div>
            </div>
            <div>
              <label
                class="mb-2 block text-sm font-medium text-ink-secondary"
              >
                {{ t("admin.settings.aliyunCaptcha.sceneId") }}
              </label>
              <input
                v-model="form.aliyun_captcha_scene_id"
                type="text"
                class="input font-mono text-sm"
                placeholder="1cxxxxxx"
              />
              <p class="mt-1.5 text-xs text-ink-secondary">
                {{ t("admin.settings.aliyunCaptcha.sceneIdHint") }}
              </p>
            </div>
            <div>
              <label
                class="mb-2 block text-sm font-medium text-ink-secondary"
              >
                {{ t("admin.settings.aliyunCaptcha.accessKeyId") }}
              </label>
              <input
                v-model="form.aliyun_captcha_access_key_id"
                type="text"
                class="input font-mono text-sm"
                placeholder="LTAI..."
              />
              <p class="mt-1.5 text-xs text-ink-secondary">
                {{ t("admin.settings.aliyunCaptcha.accessKeyIdHint") }}
              </p>
            </div>
            <div>
              <label
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
              <p class="mt-1.5 text-xs text-ink-secondary">
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
        class="border-b border-line-subtle px-6 py-4"
      >
        <h2 class="text-lg font-semibold text-ink">
          {{ t("admin.settings.linuxdo.title") }}
        </h2>
        <p class="mt-1 text-sm text-ink-secondary">
          {{ t("admin.settings.linuxdo.description") }}
        </p>
      </div>
      <div class="space-y-5 p-6">
        <div class="flex items-center justify-between">
          <div>
            <label class="font-medium text-ink">{{
              t("admin.settings.linuxdo.enable")
            }}</label>
            <p class="text-sm text-ink-secondary">
              {{ t("admin.settings.linuxdo.enableHint") }}
            </p>
          </div>
          <Toggle v-model="form.linuxdo_connect_enabled" />
        </div>

        <div
          v-if="form.linuxdo_connect_enabled"
          class="border-t border-line-subtle pt-4"
        >
          <div class="grid grid-cols-1 gap-6">
            <div>
              <label
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
              <p class="mt-1.5 text-xs text-ink-secondary">
                {{ t("admin.settings.linuxdo.clientIdHint") }}
              </p>
            </div>

            <div>
              <label
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
              <p class="mt-1.5 text-xs text-ink-secondary">
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
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
                  class="select-all break-all rounded bg-surface-sunken px-2 py-1 font-mono text-xs text-ink-secondary"
                >
                  {{ linuxdoRedirectUrlSuggestion }}
                </code>
              </div>
              <p class="mt-1.5 text-xs text-ink-secondary">
                {{ t("admin.settings.linuxdo.redirectUrlHint") }}
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- GitHub / Google 邮箱快捷登录 -->
    <div class="card">
      <div
        class="border-b border-line-subtle px-6 py-4"
      >
        <h2 class="text-lg font-semibold text-ink">
          {{ localText("邮箱快捷登录", "Email OAuth Sign-in") }}
        </h2>
        <p class="mt-1 text-sm text-ink-secondary">
          {{
            localText(
              "开启 GitHub 或 Google 邮箱授权登录后，系统会读取已验证邮箱，存在则直接登录，不存在则自动注册。",
              "After GitHub or Google email OAuth is enabled, the system reads a verified email, signs in matching users, and auto-registers missing users.",
            )
          }}
        </p>
      </div>
      <div class="space-y-6 p-6">
        <div class="grid grid-cols-1 gap-6 xl:grid-cols-2">
          <div class="rounded-sm border border-line p-4">
            <div class="flex items-start justify-between gap-4">
              <div>
                <h3 class="font-medium text-ink">
                  GitHub
                </h3>
                <p class="mt-1 text-sm text-ink-secondary">
                  {{
                    localText(
                      "GitHub OAuth App 需要 read:user user:email 权限，回调地址填写下方后端地址。",
                      "GitHub OAuth App needs read:user user:email scopes. Use the backend callback URL below.",
                    )
                  }}
                </p>
              </div>
              <Toggle v-model="form.github_oauth_enabled" />
            </div>

            <div v-if="form.github_oauth_enabled" class="mt-4 space-y-4">
              <div class="rounded-sm bg-surface-sunken px-3 py-2 text-xs text-ink-secondary">
                <template v-if="isZhLocale">
                  开通引导：GitHub Settings → Developer settings →
                  <a
                    data-testid="github-oauth-apps-guide-link"
                    href="https://github.com/settings/developers"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="font-medium text-accent hover:underline"
                  >OAuth Apps</a>
                  → New OAuth App；Homepage URL 填站点域名，Authorization callback URL 填下面的后端回调地址。
                </template>
                <template v-else>
                  Setup guide: GitHub Settings → Developer settings →
                  <a
                    data-testid="github-oauth-apps-guide-link"
                    href="https://github.com/settings/developers"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="font-medium text-accent hover:underline"
                  >OAuth Apps</a>
                  → New OAuth App. Use your site origin as Homepage URL and the backend callback URL below as Authorization callback URL.
                </template>
              </div>

              <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
                <div>
                  <label class="mb-2 block text-sm font-medium text-ink-secondary">Client ID</label>
                  <input
                    v-model="form.github_oauth_client_id"
                    type="text"
                    class="input font-mono text-sm"
                    placeholder="GitHub OAuth Client ID"
                  />
                </div>
                <div>
                  <label class="mb-2 block text-sm font-medium text-ink-secondary">Client Secret</label>
                  <input
                    v-model="form.github_oauth_client_secret"
                    type="password"
                    class="input font-mono text-sm"
                    :placeholder="
                      form.github_oauth_client_secret_configured
                        ? localText('密钥已配置，留空以保留当前值。', 'Secret configured. Leave empty to keep the current value.')
                        : 'GitHub OAuth Client Secret'
                    "
                  />
                </div>
              </div>

              <div>
                <label class="mb-2 block text-sm font-medium text-ink-secondary">
                  {{ localText("后端回调地址", "Backend Callback URL") }}
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
                    {{ localText("生成并复制", "Generate and copy") }}
                  </button>
                  <code
                    v-if="githubOAuthRedirectUrlSuggestion"
                    class="select-all break-all rounded bg-surface-sunken px-2 py-1 font-mono text-xs text-ink-secondary"
                  >
                    {{ githubOAuthRedirectUrlSuggestion }}
                  </code>
                </div>
              </div>

              <div>
                <label class="mb-2 block text-sm font-medium text-ink-secondary">
                  {{ localText("前端回跳地址", "Frontend Callback URL") }}
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

          <div class="rounded-sm border border-line p-4">
            <div class="flex items-start justify-between gap-4">
              <div>
                <h3 class="font-medium text-ink">
                  Google
                </h3>
                <p class="mt-1 text-sm text-ink-secondary">
                  {{
                    localText(
                      "Google OAuth 客户端需要 openid email profile 范围，并在凭据里登记后端回调地址。",
                      "Google OAuth client needs openid email profile scopes and the backend callback URL registered in credentials.",
                    )
                  }}
                </p>
              </div>
              <Toggle v-model="form.google_oauth_enabled" />
            </div>

            <div v-if="form.google_oauth_enabled" class="mt-4 space-y-4">
              <div class="rounded-sm bg-surface-sunken px-3 py-2 text-xs text-ink-secondary">
                {{
                  localText(
                    "开通引导：Google Cloud Console → APIs & Services → OAuth consent screen 完成同意屏幕；Credentials → Create Credentials → OAuth client ID，类型选择 Web application，并把下面地址加入 Authorized redirect URIs。",
                    "Setup guide: Google Cloud Console → APIs & Services → OAuth consent screen, then Credentials → Create Credentials → OAuth client ID, choose Web application, and add the URL below to Authorized redirect URIs.",
                  )
                }}
              </div>

              <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
                <div>
                  <label class="mb-2 block text-sm font-medium text-ink-secondary">Client ID</label>
                  <input
                    v-model="form.google_oauth_client_id"
                    type="text"
                    class="input font-mono text-sm"
                    placeholder="Google OAuth Client ID"
                  />
                </div>
                <div>
                  <label class="mb-2 block text-sm font-medium text-ink-secondary">Client Secret</label>
                  <input
                    v-model="form.google_oauth_client_secret"
                    type="password"
                    class="input font-mono text-sm"
                    :placeholder="
                      form.google_oauth_client_secret_configured
                        ? localText('密钥已配置，留空以保留当前值。', 'Secret configured. Leave empty to keep the current value.')
                        : 'Google OAuth Client Secret'
                    "
                  />
                </div>
              </div>

              <div>
                <label class="mb-2 block text-sm font-medium text-ink-secondary">
                  {{ localText("后端回调地址", "Backend Callback URL") }}
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
                    {{ localText("生成并复制", "Generate and copy") }}
                  </button>
                  <code
                    v-if="googleOAuthRedirectUrlSuggestion"
                    class="select-all break-all rounded bg-surface-sunken px-2 py-1 font-mono text-xs text-ink-secondary"
                  >
                    {{ googleOAuthRedirectUrlSuggestion }}
                  </code>
                </div>
              </div>

              <div>
                <label class="mb-2 block text-sm font-medium text-ink-secondary">
                  {{ localText("前端回跳地址", "Frontend Callback URL") }}
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
        class="border-b border-line-subtle px-6 py-4"
      >
        <h2 class="text-lg font-semibold text-ink">
          {{ t("admin.settings.wechatConnect.title") }}
        </h2>
        <p class="mt-1 text-sm text-ink-secondary">
          {{ t("admin.settings.wechatConnect.description") }}
        </p>
      </div>
      <div class="space-y-5 p-6">
        <div class="flex items-center justify-between">
          <div>
            <label class="font-medium text-ink">{{
              t("admin.settings.wechatConnect.enabledLabel")
            }}</label>
            <p class="text-sm text-ink-secondary">
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
          class="space-y-6 border-t border-line-subtle pt-4"
        >
          <div class="space-y-4">
            <div
              class="rounded-sm border border-line p-4"
            >
              <div class="flex items-start justify-between gap-4">
                <div>
                  <h3 class="font-medium text-ink">
                    {{ localText("PC 应用", "PC App") }}
                  </h3>
                  <p class="mt-1 text-sm text-ink-secondary">
                    {{
                      localText(
                        "桌面浏览器通过微信开放平台扫码登录。可与公众号或移动应用同时存在。",
                        "Desktop browsers sign in through WeChat Open Platform QR login. This can coexist with Official Account or Mobile App.",
                      )
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
                    class="mb-2 block text-sm font-medium text-ink-secondary"
                  >
                    {{ localText("PC AppID", "PC App ID") }}
                  </label>
                  <input
                    v-model="form.wechat_connect_open_app_id"
                    data-testid="wechat-connect-open-app-id"
                    type="text"
                    class="input font-mono text-sm"
                    :placeholder="
                      localText(
                        '微信开放平台 PC 应用 AppID',
                        'WeChat Open Platform PC App ID',
                      )
                    "
                  />
                </div>
                <div>
                  <label
                    class="mb-2 block text-sm font-medium text-ink-secondary"
                  >
                    {{ localText("PC AppSecret", "PC App Secret") }}
                  </label>
                  <input
                    v-model="form.wechat_connect_open_app_secret"
                    data-testid="wechat-connect-open-app-secret"
                    type="password"
                    class="input font-mono text-sm"
                    :placeholder="
                      form.wechat_connect_open_app_secret_configured
                        ? localText(
                            '密钥已配置，留空以保留当前值。',
                            'Secret configured. Leave empty to keep the current value.',
                          )
                        : localText(
                            '微信开放平台 PC 应用 AppSecret',
                            'WeChat Open Platform PC App Secret',
                          )
                    "
                  />
                </div>
              </div>
            </div>

            <div
              class="rounded-sm border border-line p-4"
            >
              <div class="flex items-start justify-between gap-4">
                <div>
                  <h3 class="font-medium text-ink">
                    {{ localText("公众号", "Official Account") }}
                  </h3>
                  <p class="mt-1 text-sm text-ink-secondary">
                    {{
                      localText(
                        "仅在微信内浏览器可用；非微信环境下会显示不可用。",
                        "Only available inside the WeChat browser. It is shown as unavailable outside WeChat.",
                      )
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
                    class="mb-2 block text-sm font-medium text-ink-secondary"
                  >
                    {{ localText("公众号 AppID", "Official Account App ID") }}
                  </label>
                  <input
                    v-model="form.wechat_connect_mp_app_id"
                    data-testid="wechat-connect-mp-app-id"
                    type="text"
                    class="input font-mono text-sm"
                    :placeholder="
                      localText(
                        '公众号 AppID',
                        'Official Account App ID',
                      )
                    "
                  />
                </div>
                <div>
                  <label
                    class="mb-2 block text-sm font-medium text-ink-secondary"
                  >
                    {{
                      localText(
                        "公众号 AppSecret",
                        "Official Account App Secret",
                      )
                    }}
                  </label>
                  <input
                    v-model="form.wechat_connect_mp_app_secret"
                    data-testid="wechat-connect-mp-app-secret"
                    type="password"
                    class="input font-mono text-sm"
                    :placeholder="
                      form.wechat_connect_mp_app_secret_configured
                        ? localText(
                            '密钥已配置，留空以保留当前值。',
                            'Secret configured. Leave empty to keep the current value.',
                          )
                        : localText(
                            '公众号 AppSecret',
                            'Official Account App Secret',
                          )
                    "
                  />
                </div>
              </div>
            </div>

            <div
              class="rounded-sm border border-line p-4"
            >
              <div class="flex items-start justify-between gap-4">
                <div>
                  <h3 class="font-medium text-ink">
                    {{ localText("移动应用", "Mobile App") }}
                  </h3>
                  <p class="mt-1 text-sm text-ink-secondary">
                    {{
                      localText(
                        "原生移动端通过微信 SDK 唤起授权，网页端不会直接发起该流程。",
                        "Native mobile clients start authorization through the WeChat SDK. The web UI does not launch this flow directly.",
                      )
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
                    class="mb-2 block text-sm font-medium text-ink-secondary"
                  >
                    {{ localText("移动应用 AppID", "Mobile App ID") }}
                  </label>
                  <input
                    v-model="form.wechat_connect_mobile_app_id"
                    data-testid="wechat-connect-mobile-app-id"
                    type="text"
                    class="input font-mono text-sm"
                    :placeholder="
                      localText(
                        '移动应用 AppID',
                        'Mobile App ID',
                      )
                    "
                  />
                </div>
                <div>
                  <label
                    class="mb-2 block text-sm font-medium text-ink-secondary"
                  >
                    {{ localText("移动应用 AppSecret", "Mobile App Secret") }}
                  </label>
                  <input
                    v-model="form.wechat_connect_mobile_app_secret"
                    data-testid="wechat-connect-mobile-app-secret"
                    type="password"
                    class="input font-mono text-sm"
                    :placeholder="
                      form.wechat_connect_mobile_app_secret_configured
                        ? localText(
                            '密钥已配置，留空以保留当前值。',
                            'Secret configured. Leave empty to keep the current value.',
                          )
                        : localText(
                            '移动应用 AppSecret',
                            'Mobile App Secret',
                          )
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
            class="rounded-sm border border border-warn/40 bg-warn-tint px-4 py-3 text-sm text-warn"
          >
            {{
              localText(
                "如果同时启用 PC 应用和公众号/移动应用，这些应用需要挂在同一个微信开放平台主体下，否则 UnionID 无法稳定归并账号。",
                "When PC App is enabled together with Official Account or Mobile App, they should belong to the same WeChat Open Platform account so UnionID can merge identities reliably.",
              )
            }}
          </div>

          <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <div>
              <label
                class="mb-2 block text-sm font-medium text-ink-secondary"
              >
                {{
                  localText(
                    "浏览器回调地址",
                    "Browser Redirect URL",
                  )
                }}
              </label>
              <input
                data-testid="wechat-connect-redirect-url"
                v-model="form.wechat_connect_redirect_url"
                type="url"
                class="input font-mono text-sm"
                :placeholder="t('admin.settings.wechatConnect.redirectUrlPlaceholder')"
              />
              <p class="mt-1.5 text-xs text-ink-secondary">
                {{
                  localText(
                    "用于 PC 应用和公众号的网页回调。移动应用走原生 SDK 时不直接使用这个浏览器回调。",
                    "Used by PC App and Official Account browser callbacks. Native mobile SDK flows do not start from this browser callback directly.",
                  )
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
                  class="select-all break-all rounded bg-surface-sunken px-2 py-1 font-mono text-xs text-ink-secondary"
                >
                  {{ wechatRedirectUrlSuggestion }}
                </code>
              </div>
            </div>
          </div>

          <div>
            <label
              class="mb-2 block text-sm font-medium text-ink-secondary"
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
            <p class="mt-1.5 text-xs text-ink-secondary">
              {{ t("admin.settings.wechatConnect.frontendRedirectUrlHint") }}
            </p>
          </div>
        </div>
      </div>
    </div>

    <!-- DingTalk Connect OAuth 登录 -->
    <div class="card">
      <div
        class="border-b border-line-subtle px-6 py-4"
      >
        <h2 class="text-lg font-semibold text-ink">
          {{ t("admin.settings.dingtalk.title") }}
        </h2>
        <p class="mt-1 text-sm text-ink-secondary">
          {{ t("admin.settings.dingtalk.description") }}
        </p>
      </div>
      <div class="space-y-5 p-6">
        <div class="flex items-center justify-between">
          <div>
            <label class="font-medium text-ink">{{
              t("admin.settings.dingtalk.enable")
            }}</label>
            <p class="text-sm text-ink-secondary">
              {{ t("admin.settings.dingtalk.enableHint") }}
            </p>
          </div>
          <Toggle v-model="form.dingtalk_connect_enabled" />
        </div>

        <div
          v-if="form.dingtalk_connect_enabled"
          class="border-t border-line-subtle pt-4"
        >
          <div class="grid grid-cols-1 gap-6">
            <div>
              <label
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
              <p class="mt-1.5 text-xs text-ink-secondary">
                {{ t("admin.settings.dingtalk.clientIdHint") }}
              </p>
            </div>

            <div>
              <label
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
              <p class="mt-1.5 text-xs text-ink-secondary">
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
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
              <p class="mt-1.5 text-xs text-ink-secondary">
                {{ t("admin.settings.dingtalk.redirectUrlHint") }}
              </p>
            </div>

            <!-- Corp Restriction Policy -->
            <div class="border-t border-line-subtle pt-4">
              <label class="mb-2 block text-sm font-medium text-ink-secondary">
                {{ t("admin.settings.dingtalk.corpPolicy.label") }}
              </label>
              <p class="mb-3 text-xs text-ink-secondary">
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
                  <span class="text-sm text-ink-secondary">
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
                  <span class="text-sm text-ink-secondary">
                    {{ t("admin.settings.dingtalk.corpPolicy.internalOnly") }}
                  </span>
                </label>
              </div>
            </div>

            <!-- bypass_registration toggle（仅 internal_only 模式下可见可用） -->
            <div
              v-if="form.dingtalk_connect_corp_restriction_policy === 'internal_only'"
              class="flex items-center justify-between pt-4 border-t border-line-subtle"
            >
              <div>
                <label class="font-medium text-ink">{{
                  t("admin.settings.dingtalk.bypassRegistration")
                }}</label>
                <p class="text-sm text-ink-secondary">
                  {{ t("admin.settings.dingtalk.bypassRegistrationHint") }}
                </p>
              </div>
              <Toggle v-model="form.dingtalk_connect_bypass_registration" />
            </div>

            <!-- 身份同步开关（仅 internal_only 模式下可见） -->
            <div
              v-if="form.dingtalk_connect_corp_restriction_policy === 'internal_only'"
              class="pt-4 border-t border-line-subtle space-y-2"
            >
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium text-ink">{{
                    t("admin.settings.dingtalk.syncDisplayName")
                  }}</label>
                  <p class="text-sm text-ink-secondary">
                    {{ t("admin.settings.dingtalk.syncDisplayNameHint") }}
                  </p>
                </div>
                <Toggle v-model="form.dingtalk_connect_sync_display_name" />
              </div>
              <div v-if="form.dingtalk_connect_sync_display_name" class="space-y-2">
                <div class="flex items-center gap-2">
                  <label class="text-sm text-ink-secondary whitespace-nowrap min-w-[5rem]">
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
                  <label class="text-sm text-ink-secondary whitespace-nowrap min-w-[5rem]">
                    {{ t("admin.settings.dingtalk.syncAttrDisplayName") }}
                  </label>
                  <input
                    v-model="form.dingtalk_connect_sync_display_name_attr_name"
                    type="text"
                    :placeholder="localText('钉钉姓名', 'DingTalk Name')"
                    class="input text-sm flex-1 max-w-xs"
                  />
                </div>
              </div>
              <p v-if="form.dingtalk_connect_sync_display_name" class="text-xs text-ink-tertiary">
                {{ t("admin.settings.dingtalk.syncDisplayNameTargetHint") }}
              </p>
            </div>
            <div
              v-if="form.dingtalk_connect_corp_restriction_policy === 'internal_only'"
              class="pt-4 border-t border-line-subtle space-y-2"
            >
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium text-ink">{{
                    t("admin.settings.dingtalk.syncCorpEmail")
                  }}</label>
                  <p class="text-sm text-ink-secondary">
                    {{ t("admin.settings.dingtalk.syncCorpEmailHint") }}
                  </p>
                  <p class="text-xs text-warn mt-1">
                    {{ t("admin.settings.dingtalk.syncCorpEmailPermissionHint") }}
                  </p>
                </div>
                <Toggle v-model="form.dingtalk_connect_sync_corp_email" />
              </div>
              <div v-if="form.dingtalk_connect_sync_corp_email" class="space-y-2">
                <div class="flex items-center gap-2">
                  <label class="text-sm text-ink-secondary whitespace-nowrap min-w-[5rem]">
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
                  <label class="text-sm text-ink-secondary whitespace-nowrap min-w-[5rem]">
                    {{ t("admin.settings.dingtalk.syncAttrDisplayName") }}
                  </label>
                  <input
                    v-model="form.dingtalk_connect_sync_corp_email_attr_name"
                    type="text"
                    :placeholder="localText('钉钉企业邮箱', 'DingTalk Corporate Email')"
                    class="input text-sm flex-1 max-w-xs"
                  />
                </div>
              </div>
              <p v-if="form.dingtalk_connect_sync_corp_email" class="text-xs text-ink-tertiary">
                {{ t("admin.settings.dingtalk.syncCorpEmailTargetHint") }}
              </p>
            </div>
            <div
              v-if="form.dingtalk_connect_corp_restriction_policy === 'internal_only'"
              class="pt-4 border-t border-line-subtle space-y-2"
            >
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium text-ink">{{
                    t("admin.settings.dingtalk.syncDept")
                  }}</label>
                  <p class="text-sm text-ink-secondary">
                    {{ t("admin.settings.dingtalk.syncDeptHint") }}
                  </p>
                  <p class="text-xs text-warn mt-1">
                    {{ t("admin.settings.dingtalk.syncDeptPermissionHint") }}
                  </p>
                </div>
                <Toggle v-model="form.dingtalk_connect_sync_dept" />
              </div>
              <div v-if="form.dingtalk_connect_sync_dept" class="space-y-2">
                <div class="flex items-center gap-2">
                  <label class="text-sm text-ink-secondary whitespace-nowrap min-w-[5rem]">
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
                  <label class="text-sm text-ink-secondary whitespace-nowrap min-w-[5rem]">
                    {{ t("admin.settings.dingtalk.syncAttrDisplayName") }}
                  </label>
                  <input
                    v-model="form.dingtalk_connect_sync_dept_attr_name"
                    type="text"
                    :placeholder="localText('钉钉部门', 'DingTalk Department')"
                    class="input text-sm flex-1 max-w-xs"
                  />
                </div>
              </div>
              <p v-if="form.dingtalk_connect_sync_dept" class="text-xs text-ink-tertiary">
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
        class="border-b border-line-subtle px-6 py-4"
      >
        <h2 class="text-lg font-semibold text-ink">
          {{ t("admin.settings.oidc.title") }}
        </h2>
        <p class="mt-1 text-sm text-ink-secondary">
          {{ t("admin.settings.oidc.description") }}
        </p>
      </div>
      <div class="space-y-5 p-6">
        <div class="flex items-center justify-between">
          <div>
            <label class="font-medium text-ink">{{
              t("admin.settings.oidc.enable")
            }}</label>
            <p class="text-sm text-ink-secondary">
              {{ t("admin.settings.oidc.enableHint") }}
            </p>
          </div>
          <Toggle v-model="form.oidc_connect_enabled" />
        </div>

        <div
          v-if="form.oidc_connect_enabled"
          class="space-y-6 border-t border-line-subtle pt-4"
        >
          <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
            <div>
              <label
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
              <p class="mt-1.5 text-xs text-ink-secondary">
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
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
                class="mb-2 block text-sm font-medium text-ink-secondary"
              >
                {{ t("admin.settings.oidc.scopes") }}
              </label>
              <input
                v-model="form.oidc_connect_scopes"
                type="text"
                class="input font-mono text-sm"
                :placeholder="t('admin.settings.oidc.scopesPlaceholder')"
              />
              <p class="mt-1.5 text-xs text-ink-secondary">
                {{ t("admin.settings.oidc.scopesHint") }}
              </p>
            </div>

            <div>
              <label
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
                  class="select-all break-all rounded bg-surface-sunken px-2 py-1 font-mono text-xs text-ink-secondary"
                >
                  {{ oidcRedirectUrlSuggestion }}
                </code>
              </div>
              <p class="mt-1.5 text-xs text-ink-secondary">
                {{ t("admin.settings.oidc.redirectUrlHint") }}
              </p>
            </div>

            <div class="lg:col-span-2">
              <label
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
              <p class="mt-1.5 text-xs text-ink-secondary">
                {{ t("admin.settings.oidc.frontendRedirectUrlHint") }}
              </p>
            </div>
          </div>

          <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
            <div>
              <label
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
              class="flex items-center justify-between rounded border border-line px-4 py-3"
            >
              <div>
                <label class="font-medium text-ink">
                  {{ t("admin.settings.oidc.usePkce") }}
                </label>
              </div>
              <Toggle
                v-model="form.oidc_connect_use_pkce"
                data-testid="oidc-connect-use-pkce"
              />
            </div>

            <div
              class="flex items-center justify-between rounded border border-line px-4 py-3"
            >
              <div>
                <label class="font-medium text-ink">
                  {{ t("admin.settings.oidc.validateIdToken") }}
                </label>
              </div>
              <Toggle
                v-model="form.oidc_connect_validate_id_token"
                data-testid="oidc-connect-validate-id-token"
              />
            </div>

            <div
              class="flex items-center justify-between rounded border border-line px-4 py-3"
            >
              <div>
                <label class="font-medium text-ink">
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
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
                class="mb-2 block text-sm font-medium text-ink-secondary"
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
import Icon from "@/components/icons/Icon.vue";
import Toggle from "@/components/common/Toggle.vue";
import { useSettingsFormContext } from "../context";

const {
  activeTab,
  captchaMasterEnabled,
  captchaProviderSelection,
  commitForwardedClientIpHeaderDraft,
  commitRegistrationEmailSuffixWhitelistDraft,
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
  isZhLocale,
  linuxdoRedirectUrlSuggestion,
  localText,
  oidcRedirectUrlSuggestion,
  panelRateLimitForm,
  panelRateLimitLoading,
  panelRateLimitSaving,
  registrationEmailSuffixWhitelistDraft,
  registrationEmailSuffixWhitelistTags,
  removeForwardedClientIpHeader,
  removeRegistrationEmailSuffixWhitelistTag,
  savePanelRateLimitSettings,
  selectCaptchaProvider,
  setAndCopyEmailOAuthRedirectUrl,
  setAndCopyLinuxdoRedirectUrl,
  setAndCopyOIDCRedirectUrl,
  setAndCopyWeChatRedirectUrl,
  t,
  tencentCaptchaLinks,
  wechatRedirectUrlSuggestion,
} = useSettingsFormContext();
</script>
