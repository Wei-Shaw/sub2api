<template>
  <div v-show="activeTab === 'email'" class="space-y-6">
    <!-- Email disabled hint - show when email_verify_enabled is off -->
    <div v-if="!form.email_verify_enabled" class="card">
      <div class="card-body">
        <div class="flex items-start gap-3">
          <Icon
            name="mail"
            size="md"
            class="mt-0.5 flex-shrink-0 text-fg-subtle"
          />
          <div>
            <h3 class="font-medium text-fg">
              {{ t("admin.settings.emailTabDisabledTitle") }}
            </h3>
            <p class="mt-1 text-sm text-fg-muted">
              {{ t("admin.settings.emailTabDisabledHint") }}
            </p>
          </div>
        </div>
      </div>
    </div>

    <!-- SMTP Settings - Only show when email verification is enabled -->
    <div v-if="form.email_verify_enabled" class="card">
      <div
        class="flex items-center justify-between border-b border-border px-6 py-4"
      >
        <div>
          <h2 class="card-title">
            {{ t("admin.settings.smtp.title") }}
          </h2>
          <p class="mt-1 text-sm text-fg-muted">
            {{ t("admin.settings.smtp.description") }}
          </p>
        </div>
        <button
          type="button"
          @click="testSmtpConnection"
          :disabled="testingSmtp || loadFailed"
          class="btn btn-secondary btn-sm"
        >
          <svg
            v-if="testingSmtp"
            class="h-4 w-4 animate-spin"
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
            testingSmtp
              ? t("admin.settings.smtp.testing")
              : t("admin.settings.smtp.testConnection")
          }}
        </button>
      </div>
      <div class="card-body space-y-6">
        <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
          <div>
            <label
              class="input-label"
            >
              {{ t("admin.settings.smtp.host") }}
            </label>
            <input
              v-model="form.smtp_host"
              type="text"
              class="input"
              :placeholder="t('admin.settings.smtp.hostPlaceholder')"
            />
          </div>
          <div>
            <label
              class="input-label"
            >
              {{ t("admin.settings.smtp.port") }}
            </label>
            <input
              v-model.number="form.smtp_port"
              type="number"
              min="1"
              max="65535"
              class="input"
              :placeholder="t('admin.settings.smtp.portPlaceholder')"
            />
          </div>
          <div>
            <label
              class="input-label"
            >
              {{ t("admin.settings.smtp.username") }}
            </label>
            <input
              v-model="form.smtp_username"
              type="text"
              class="input"
              :placeholder="t('admin.settings.smtp.usernamePlaceholder')"
            />
          </div>
          <div>
            <label
              class="input-label"
            >
              {{ t("admin.settings.smtp.password") }}
            </label>
            <input
              v-model="form.smtp_password"
              type="password"
              class="input"
              autocomplete="new-password"
              autocapitalize="off"
              spellcheck="false"
              @keydown="smtpPasswordManuallyEdited = true"
              @paste="smtpPasswordManuallyEdited = true"
              :placeholder="
                form.smtp_password_configured
                  ? t('admin.settings.smtp.passwordConfiguredPlaceholder')
                  : t('admin.settings.smtp.passwordPlaceholder')
              "
            />
            <p class="input-hint">
              {{
                form.smtp_password_configured
                  ? t("admin.settings.smtp.passwordConfiguredHint")
                  : t("admin.settings.smtp.passwordHint")
              }}
            </p>
          </div>
          <div>
            <label
              class="input-label"
            >
              {{ t("admin.settings.smtp.fromEmail") }}
            </label>
            <input
              v-model="form.smtp_from_email"
              type="email"
              class="input"
              :placeholder="t('admin.settings.smtp.fromEmailPlaceholder')"
            />
          </div>
          <div>
            <label
              class="input-label"
            >
              {{ t("admin.settings.smtp.fromName") }}
            </label>
            <input
              v-model="form.smtp_from_name"
              type="text"
              class="input"
              :placeholder="t('admin.settings.smtp.fromNamePlaceholder')"
            />
          </div>
        </div>

        <!-- Use TLS Toggle -->
        <div
          class="flex items-center justify-between border-t border-border pt-4"
        >
          <div>
            <label class="font-medium text-fg">{{
              t("admin.settings.smtp.useTls")
            }}</label>
            <p class="text-sm text-fg-muted">
              {{ t("admin.settings.smtp.useTlsHint") }}
            </p>
          </div>
          <Toggle v-model="form.smtp_use_tls" />
        </div>
      </div>
    </div>

    <!-- Send Test Email - Only show when email verification is enabled -->
    <div v-if="form.email_verify_enabled" class="card">
      <div
        class="card-header"
      >
        <h2 class="card-title">
          {{ t("admin.settings.testEmail.title") }}
        </h2>
        <p class="mt-1 text-sm text-fg-muted">
          {{ t("admin.settings.testEmail.description") }}
        </p>
      </div>
      <div class="card-body">
        <div class="flex items-end gap-4">
          <div class="flex-1">
            <label
              class="input-label"
            >
              {{ t("admin.settings.testEmail.recipientEmail") }}
            </label>
            <input
              v-model="testEmailAddress"
              type="email"
              class="input"
              :placeholder="
                t('admin.settings.testEmail.recipientEmailPlaceholder')
              "
            />
          </div>
          <button
            type="button"
            @click="sendTestEmail"
            :disabled="
              sendingTestEmail || !testEmailAddress || loadFailed
            "
            class="btn btn-secondary"
          >
            <svg
              v-if="sendingTestEmail"
              class="h-4 w-4 animate-spin"
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
              sendingTestEmail
                ? t("admin.settings.testEmail.sending")
                : t("admin.settings.testEmail.sendTestEmail")
            }}
          </button>
        </div>
      </div>
    </div>

    <!-- 订阅到期提醒 -->
    <div class="card">
      <div
        class="card-header"
      >
        <h3 class="text-h3 font-bold text-fg">
          {{ t("admin.settings.subscriptionExpiryNotify.title") }}
        </h3>
        <p class="mt-1 text-sm text-fg-muted">
          {{ t("admin.settings.subscriptionExpiryNotify.description") }}
        </p>
      </div>
      <div class="px-6 py-6">
        <div class="flex items-center justify-between gap-4">
          <div>
            <label
              class="mb-0 block text-sm font-medium text-fg"
            >
              {{ t("admin.settings.subscriptionExpiryNotify.enabled") }}
            </label>
            <p class="mt-1 text-xs text-fg-muted">
              {{ t("admin.settings.subscriptionExpiryNotify.enabledHint") }}
            </p>
          </div>
          <Toggle v-model="form.subscription_expiry_notify_enabled" />
        </div>
      </div>
    </div>

    <EmailTemplateEditor />

    <!-- Balance Low Notification -->
    <div class="card">
      <div
        class="card-header"
      >
        <h3 class="text-h3 font-bold text-fg">
          {{ t("admin.settings.balanceNotify.title") }}
        </h3>
        <p class="mt-1 text-sm text-fg-muted">
          {{ t("admin.settings.balanceNotify.description") }}
        </p>
      </div>
      <div class="px-6 py-6 space-y-4">
        <div class="flex items-center justify-between">
          <label
            class="mb-0 block text-sm font-medium text-fg"
            >{{ t("admin.settings.balanceNotify.enabled") }}</label
          >
          <Toggle v-model="form.balance_low_notify_enabled" />
        </div>
        <div v-if="form.balance_low_notify_enabled">
          <label
            class="input-label"
            >{{ t("admin.settings.balanceNotify.threshold") }}</label
          >
          <div class="relative">
            <span
              class="absolute left-3 top-1/2 -translate-y-1/2 text-fg-subtle"
              >$</span
            >
            <input
              v-model.number="form.balance_low_notify_threshold"
              type="number"
              min="0"
              step="0.01"
              class="input pl-7"
            />
          </div>
          <p class="mt-1 text-xs text-fg-muted">
            {{ t("admin.settings.balanceNotify.thresholdHint") }}
          </p>
        </div>
        <div>
          <label
            class="input-label"
            >{{ t("admin.settings.balanceNotify.rechargeUrl") }}</label
          >
          <input
            v-model="form.balance_low_notify_recharge_url"
            type="url"
            class="input"
            :placeholder="currentOrigin"
          />
          <p class="mt-1 text-xs text-fg-muted">
            {{ t("admin.settings.balanceNotify.rechargeUrlHint") }}
          </p>
        </div>
      </div>
    </div>

    <!-- Account Quota Notification -->
    <div class="card">
      <div
        class="card-header"
      >
        <h3 class="text-h3 font-bold text-fg">
          {{ t("admin.settings.quotaNotify.title") }}
        </h3>
        <p class="mt-1 text-sm text-fg-muted">
          {{ t("admin.settings.quotaNotify.description") }}
        </p>
      </div>
      <div class="px-6 py-6 space-y-4">
        <div class="flex items-center justify-between">
          <label
            class="mb-0 block text-sm font-medium text-fg"
            >{{ t("admin.settings.quotaNotify.enabled") }}</label
          >
          <Toggle v-model="form.account_quota_notify_enabled" />
        </div>
        <div v-if="form.account_quota_notify_enabled">
          <label
            class="input-label"
            >{{ t("admin.settings.quotaNotify.emails") }}</label
          >
          <div class="space-y-2">
            <div
              v-for="(entry, index) in form.account_quota_notify_emails ||
              []"
              :key="index"
              class="flex items-center gap-2"
            >
              <label
                class="relative inline-flex items-center cursor-pointer shrink-0"
              >
                <input
                  type="checkbox"
                  :checked="!entry.disabled"
                  @change="entry.disabled = !entry.disabled"
                  class="sr-only peer"
                />
                <div
                  class="w-9 h-5 bg-border peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-border-strong after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-accent"
                ></div>
              </label>
              <input
                v-model="entry.email"
                type="email"
                class="input flex-1"
                :placeholder="
                  t('admin.settings.quotaNotify.emailPlaceholder')
                "
              />
              <button
                :aria-label="t('common.remove')"
                :title="t('common.remove')"
                @click="form.account_quota_notify_emails.splice(index, 1)"
                class="btn btn-secondary px-2"
                type="button"
              >
                <Icon name="x" size="xs" class="h-4 w-4" />
              </button>
            </div>
            <button
              @click="addQuotaNotifyEmail"
              class="btn btn-secondary btn-sm"
              type="button"
            >
              + {{ t("admin.settings.quotaNotify.addEmail") }}
            </button>
          </div>
          <p class="mt-1 text-xs text-fg-muted">
            {{ t("admin.settings.quotaNotify.emailsHint") }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useSettingsViewContext } from "./context";
import EmailTemplateEditor from "@/views/admin/settings/EmailTemplateEditor.vue";
import Icon from "@/components/icons/Icon.vue";
import Toggle from "@/components/common/Toggle.vue";

// 纯移动拆分：所有状态与方法来自 SettingsView 提供的上下文（openspec: rebuild-frontend-design-system Phase 3）
const ctx = useSettingsViewContext();
const {
  activeTab,
  addQuotaNotifyEmail,
  currentOrigin,
  form,
  loadFailed,
  sendTestEmail,
  sendingTestEmail,
  smtpPasswordManuallyEdited,
  t,
  testEmailAddress,
  testSmtpConnection,
  testingSmtp,
} = ctx;
</script>
