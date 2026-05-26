<template>
  <div class="space-y-5">
    <!-- Name + Notes at top -->
    <AccountCommonFields v-model="commonFields" :context="context" mode="identity" />

    <!-- API Key inputs -->
    <template v-if="context.accountCategory === 'apikey'">
      <div>
        <label class="input-label">{{ t('admin.accounts.baseUrl') }}</label>
        <input v-model="apiKeyBaseUrl" type="text" class="input"
          placeholder="https://api.anthropic.com" />
        <p v-if="context.mode !== 'edit'" class="input-hint">{{ t('admin.accounts.baseUrlHint') }}</p>
      </div>
      <div v-if="context.mode !== 'edit'">
        <label class="input-label">{{ t('admin.accounts.apiKey') }}</label>
        <input v-model="apiKeyValue" type="password" required class="input font-mono"
          placeholder="sk-ant-..." />
      </div>
      <div v-else>
        <label class="input-label">{{ t('admin.accounts.apiKey') }}</label>
        <input v-model="editApiKey" type="password" class="input font-mono"
          autocomplete="new-password" data-1p-ignore data-lpignore="true" data-bwignore="true"
          placeholder="sk-ant-..." />
        <p class="input-hint">{{ t('admin.accounts.leaveEmptyToKeep') }}</p>
      </div>
      <ModelRestrictionSection platform="anthropic"
        :mode="modelRestrictionMode" :allowed-models="allowedModels" :mappings="modelMappings"
        :protocols="['anthropic']"
        @update:mode="modelRestrictionMode = $event"
        @update:allowed-models="allowedModels = $event"
        @update:mappings="modelMappings = $event" />
      <PoolModeSection :enabled="poolModeEnabled" :retry-count="poolModeRetryCount"
        @update:enabled="poolModeEnabled = $event"
        @update:retry-count="poolModeRetryCount = $event" />
      <CustomErrorCodesSection :enabled="customErrorCodesEnabled" :codes="selectedErrorCodes"
        @update:enabled="customErrorCodesEnabled = $event"
        @update:codes="selectedErrorCodes = $event" />
      <ToggleCard :label="t('admin.accounts.anthropic.apiKeyPassthrough')"
        :hint="t('admin.accounts.anthropic.apiKeyPassthroughDesc')"
        :enabled="anthropicPassthroughEnabled"
        @update:enabled="anthropicPassthroughEnabled = $event" />
      <div v-if="webSearchGlobalEnabled" class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <label class="input-label">{{ t('admin.accounts.webSearchEmulation') }}</label>
        <select v-model="webSearchEmulationMode" class="input mt-1">
          <option value="default">{{ t('admin.accounts.webSearchEmulationDefault') }}</option>
          <option value="force_on">{{ t('admin.accounts.webSearchEmulationForceOn') }}</option>
          <option value="force_off">{{ t('admin.accounts.webSearchEmulationForceOff') }}</option>
        </select>
      </div>
    </template>

    <!-- Bedrock credentials -->
    <BedrockCredentials v-if="context.accountCategory === 'bedrock'"
      :auth-mode="bedrockAuthMode" :access-key-id="bedrockAccessKeyId"
      :secret-access-key="bedrockSecretAccessKey" :session-token="bedrockSessionToken"
      :api-key-value="bedrockApiKeyValue" :region="bedrockRegion" :force-global="bedrockForceGlobal"
      @update:auth-mode="bedrockAuthMode = $event"
      @update:access-key-id="bedrockAccessKeyId = $event"
      @update:secret-access-key="bedrockSecretAccessKey = $event"
      @update:session-token="bedrockSessionToken = $event"
      @update:api-key-value="bedrockApiKeyValue = $event"
      @update:region="bedrockRegion = $event"
      @update:force-global="bedrockForceGlobal = $event" />
    <template v-if="context.accountCategory === 'bedrock'">
      <ModelRestrictionSection platform="anthropic"
        :mode="modelRestrictionMode" :allowed-models="allowedModels" :mappings="modelMappings"
        :protocols="['anthropic']"
        @update:mode="modelRestrictionMode = $event"
        @update:allowed-models="allowedModels = $event"
        @update:mappings="modelMappings = $event" />
      <PoolModeSection :enabled="poolModeEnabled" :retry-count="poolModeRetryCount"
        @update:enabled="poolModeEnabled = $event"
        @update:retry-count="poolModeRetryCount = $event" />
    </template>

    <!-- Vertex Service Account -->
    <template v-if="context.accountCategory === 'service_account'">
      <div class="rounded-lg border border-sky-200 bg-sky-50 px-3 py-2 text-xs text-sky-800 dark:border-sky-800/40 dark:bg-sky-900/20 dark:text-sky-200">
        <p>{{ t('admin.accounts.vertexAnthropicHint') }}</p>
      </div>
      <VertexServiceAccount
        :service-account-json="vertexServiceAccountJson" :project-id="vertexProjectId"
        :client-email="vertexClientEmail" :location="vertexLocation"
        @update:service-account-json="vertexServiceAccountJson = $event"
        @update:project-id="vertexProjectId = $event"
        @update:client-email="vertexClientEmail = $event"
        @update:location="vertexLocation = $event"
        @parse-error="sdk.notify.error($event)" />
      <ModelRestrictionSection platform="anthropic"
        :mode="modelRestrictionMode" :allowed-models="allowedModels" :mappings="modelMappings"
        :protocols="['anthropic']"
        @update:mode="modelRestrictionMode = $event"
        @update:allowed-models="allowedModels = $event"
        @update:mappings="modelMappings = $event" />
    </template>

    <!-- OAuth-based: Add method selector -->
    <OAuthMethodSelector v-if="context.accountCategory === 'oauth-based'"
      :add-method="addMethod" @update:add-method="addMethod = $event as AddMethod" />

    <!-- OAuth-based: Quota Control -->
    <OAuthQuotaSection v-if="context.accountCategory === 'oauth-based'"
      :window-cost-enabled="windowCostEnabled" :window-cost-limit="windowCostLimit"
      :window-cost-sticky-reserve="windowCostStickyReserve"
      :session-limit-enabled="sessionLimitEnabled" :max-sessions="maxSessions"
      :session-idle-timeout="sessionIdleTimeout"
      :rpm-limit-enabled="rpmLimitEnabled" :base-rpm="baseRpm"
      :rpm-strategy="rpmStrategy" :rpm-sticky-buffer="rpmStickyBuffer"
      :user-msg-queue-mode="userMsgQueueMode" :umq-mode-options="umqModeOptions"
      :tls-fingerprint-enabled="tlsFingerprintEnabled"
      :tls-fingerprint-profile-id="tlsFingerprintProfileId"
      :tls-fingerprint-profiles="tlsFingerprintProfiles"
      :session-id-masking-enabled="sessionIdMaskingEnabled"
      :cacheTTLOverrideEnabled="cacheTTLOverrideEnabled"
      :cacheTTLOverrideTarget="cacheTTLOverrideTarget"
      :custom-base-url-enabled="customBaseUrlEnabled" :custom-base-url="customBaseUrl"
      @update:window-cost-enabled="windowCostEnabled = $event"
      @update:window-cost-limit="windowCostLimit = $event"
      @update:window-cost-sticky-reserve="windowCostStickyReserve = $event"
      @update:session-limit-enabled="sessionLimitEnabled = $event"
      @update:max-sessions="maxSessions = $event"
      @update:session-idle-timeout="sessionIdleTimeout = $event"
      @update:rpm-limit-enabled="rpmLimitEnabled = $event"
      @update:base-rpm="baseRpm = $event"
      @update:rpm-strategy="rpmStrategy = $event"
      @update:rpm-sticky-buffer="rpmStickyBuffer = $event"
      @update:user-msg-queue-mode="userMsgQueueMode = $event"
      @update:tls-fingerprint-enabled="tlsFingerprintEnabled = $event"
      @update:tls-fingerprint-profile-id="tlsFingerprintProfileId = $event"
      @update:session-id-masking-enabled="sessionIdMaskingEnabled = $event"
      @update:cacheTTLOverrideEnabled="cacheTTLOverrideEnabled = $event"
      @update:cacheTTLOverrideTarget="cacheTTLOverrideTarget = $event"
      @update:custom-base-url-enabled="customBaseUrlEnabled = $event"
      @update:custom-base-url="customBaseUrl = $event" />

    <!-- Sync to Stream (all Anthropic except Bedrock) -->
    <div v-if="context.accountCategory !== 'bedrock'"
      class="border-t border-gray-200 pt-4 dark:border-dark-600">
      <div class="flex items-center justify-between">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.anthropic.syncToStream') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.anthropic.syncToStreamDesc') }}
          </p>
        </div>
        <select v-model="syncToStreamMode" class="input w-24 text-sm">
          <option value="default">{{ t('admin.accounts.anthropic.syncToStreamDefault') }}</option>
          <option value="enabled">{{ t('admin.accounts.anthropic.syncToStreamEnabled') }}</option>
        </select>
      </div>
    </div>

    <!-- Intercept warmup -->
    <ToggleCard v-if="context.accountCategory !== 'service_account'"
      :label="t('admin.accounts.interceptWarmupRequests')"
      :hint="t('admin.accounts.interceptWarmupRequestsDesc')"
      :enabled="interceptWarmupRequests"
      @update:enabled="interceptWarmupRequests = $event" />

    <!-- Temp Unsched -->
    <TempUnschedSection :enabled="tempUnschedEnabled" :rules="tempUnschedRules"
      @update:enabled="tempUnschedEnabled = $event"
      @update:rules="tempUnschedRules = $event" />

    <!-- Common operational fields (proxy, concurrency, quota, groups, expiry) -->
    <AccountCommonFields v-model="commonFields" :context="context" mode="settings" />
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  ModelRestrictionSection, PoolModeSection, CustomErrorCodesSection,
  TempUnschedSection, ToggleCard, VertexServiceAccount, AccountCommonFields,
} from '@sub2api/plugin-sdk'
import type { PlatformFormContext, CommonAccountFields, SdkAccount } from '@sub2api/plugin-sdk'
import { getSdk } from '../api/sdk'
import BedrockCredentials from '../components/BedrockCredentials.vue'
import OAuthMethodSelector from '../components/OAuthMethodSelector.vue'
import OAuthQuotaSection from '../components/OAuthQuotaSection.vue'
import { useAnthropicForm } from './useAnthropicForm'
import type { AddMethod } from '../composables/useAccountOAuth'

const props = defineProps<{ context: PlatformFormContext }>()
const { t } = useI18n()
const sdk = getSdk()

const form = useAnthropicForm()
const {
  addMethod, apiKeyBaseUrl, apiKeyValue, editApiKey,
  bedrockAuthMode, bedrockAccessKeyId, bedrockSecretAccessKey,
  bedrockSessionToken, bedrockRegion, bedrockForceGlobal, bedrockApiKeyValue,
  vertexServiceAccountJson, vertexProjectId, vertexClientEmail, vertexLocation,
  windowCostEnabled, windowCostLimit, windowCostStickyReserve,
  sessionLimitEnabled, maxSessions, sessionIdleTimeout,
  rpmLimitEnabled, baseRpm, rpmStrategy, rpmStickyBuffer,
  userMsgQueueMode, umqModeOptions,
  tlsFingerprintEnabled, tlsFingerprintProfileId, tlsFingerprintProfiles,
  sessionIdMaskingEnabled, cacheTTLOverrideEnabled, cacheTTLOverrideTarget,
  customBaseUrlEnabled, customBaseUrl,
  anthropicPassthroughEnabled, webSearchEmulationMode, syncToStreamMode, webSearchGlobalEnabled,
  interceptWarmupRequests,
  modelRestrictionMode, allowedModels, modelMappings,
  poolModeEnabled, poolModeRetryCount,
  customErrorCodesEnabled, selectedErrorCodes,
  tempUnschedEnabled, tempUnschedRules,
  oauth, oauthConfig, validate, getPayload, isOAuthFlow, reset,
  handleOAuthExchange, handleCookieAuth,
  loadTlsProfiles, loadWebSearchEnabled,
  initFromAccount, getEditPayload,
} = form

const commonFields = ref<CommonAccountFields>({
  name: '',
  notes: '',
  proxy_id: null,
  concurrency: 10,
  load_factor: null,
  priority: 1,
  rate_multiplier: 1,
  expires_at: null,
  auto_pause_on_expired: true,
  group_ids: [],
  quota_enabled: false,
  quota_limit: null,
  quota_daily_limit: null,
  quota_weekly_limit: null,
})

onMounted(() => {
  loadTlsProfiles()
  loadWebSearchEnabled()
})

defineExpose({
  validate: () => validate(props.context.accountCategory, props.context.mode),
  getPayload: () => {
    const payload = getPayload(props.context.accountCategory)
    payload.common = commonFields.value
    return payload
  },
  isOAuthFlow: () => isOAuthFlow(props.context.accountCategory),
  reset: () => {
    reset()
    commonFields.value = {
      name: '', notes: '',
      proxy_id: null, concurrency: 10, load_factor: null, priority: 1,
      rate_multiplier: 1, expires_at: null, auto_pause_on_expired: true,
      group_ids: [], quota_enabled: false, quota_limit: null,
      quota_daily_limit: null, quota_weekly_limit: null,
    }
  },
  initFromAccount: (account: Record<string, unknown>) => {
    initFromAccount(account as SdkAccount)
    const extra = (account.extra ?? {}) as Record<string, unknown>
    commonFields.value = {
      name: (account.name as string) ?? '',
      notes: (account.notes as string) ?? '',
      proxy_id: (account.proxy_id as number | null) ?? null,
      concurrency: (account.concurrency as number) ?? 10,
      load_factor: (account.load_factor as number | null) ?? null,
      priority: (account.priority as number) ?? 1,
      rate_multiplier: (account.rate_multiplier as number) ?? 1,
      expires_at: (account.expires_at as number | null) ?? null,
      auto_pause_on_expired: (account.auto_pause_on_expired as boolean) ?? true,
      group_ids: (account.group_ids as number[]) ?? [],
      quota_enabled: !!(extra.quota_limit || extra.quota_daily_limit || extra.quota_weekly_limit),
      quota_limit: (extra.quota_limit as number) ?? null,
      quota_daily_limit: (extra.quota_daily_limit as number) ?? null,
      quota_weekly_limit: (extra.quota_weekly_limit as number) ?? null,
    }
  },
  getEditPayload: (account: SdkAccount) => {
    const payload = getEditPayload(account)
    payload.common = commonFields.value
    return payload
  },
  oauthConfig,
  getOAuthState: () => ({
    authUrl: oauth.authUrl.value,
    sessionId: oauth.sessionId.value,
    loading: oauth.loading.value,
    error: oauth.error.value,
  }),
  generateOAuthUrl: (proxyId: number | null) =>
    oauth.generateAuthUrl(addMethod.value, proxyId),
  resetOAuth: () => oauth.resetState(),
  handleOAuthExchange,
  handleCookieAuth,
})
</script>


