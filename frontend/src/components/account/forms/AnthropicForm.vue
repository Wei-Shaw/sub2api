<template>
  <div class="space-y-5">
    <!-- API Key inputs (create mode only - edit modal has its own) -->
    <template v-if="context.accountCategory === 'apikey' && context.mode !== 'edit'">
      <div>
        <label class="input-label">{{ t('admin.accounts.baseUrl') }}</label>
        <input v-model="apiKeyBaseUrl" type="text" class="input"
          placeholder="https://api.anthropic.com" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.apiKey') }}</label>
        <input v-model="apiKeyValue" type="password" required class="input font-mono"
          placeholder="sk-ant-..." />
      </div>
      <ModelRestrictionSection
        platform="anthropic"
        :mode="modelRestrictionMode"
        :allowed-models="allowedModels"
        :mappings="modelMappings"
        @update:mode="modelRestrictionMode = $event"
        @update:allowed-models="allowedModels = $event"
        @update:mappings="modelMappings = $event"
      />
      <PoolModeSection
        :enabled="poolModeEnabled"
        :retry-count="poolModeRetryCount"
        @update:enabled="poolModeEnabled = $event"
        @update:retry-count="poolModeRetryCount = $event"
      />
      <CustomErrorCodesSection
        :enabled="customErrorCodesEnabled"
        :codes="selectedErrorCodes"
        @update:enabled="customErrorCodesEnabled = $event"
        @update:codes="selectedErrorCodes = $event"
      />
      <ToggleCard
        :label="t('admin.accounts.anthropic.apiKeyPassthrough')"
        :hint="t('admin.accounts.anthropic.apiKeyPassthroughDesc')"
        :enabled="anthropicPassthroughEnabled"
        @update:enabled="anthropicPassthroughEnabled = $event"
      />
      <!-- Web search emulation -->
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
    <BedrockCredentials
      v-if="context.accountCategory === 'bedrock'"
      :auth-mode="bedrockAuthMode"
      :access-key-id="bedrockAccessKeyId"
      :secret-access-key="bedrockSecretAccessKey"
      :session-token="bedrockSessionToken"
      :api-key-value="bedrockApiKeyValue"
      :region="bedrockRegion"
      :force-global="bedrockForceGlobal"
      @update:auth-mode="bedrockAuthMode = $event"
      @update:access-key-id="bedrockAccessKeyId = $event"
      @update:secret-access-key="bedrockSecretAccessKey = $event"
      @update:session-token="bedrockSessionToken = $event"
      @update:api-key-value="bedrockApiKeyValue = $event"
      @update:region="bedrockRegion = $event"
      @update:force-global="bedrockForceGlobal = $event"
    />
    <template v-if="context.accountCategory === 'bedrock'">
      <ModelRestrictionSection
        platform="anthropic"
        :mode="modelRestrictionMode"
        :allowed-models="allowedModels"
        :mappings="modelMappings"
        @update:mode="modelRestrictionMode = $event"
        @update:allowed-models="allowedModels = $event"
        @update:mappings="modelMappings = $event"
      />
      <PoolModeSection
        :enabled="poolModeEnabled"
        :retry-count="poolModeRetryCount"
        @update:enabled="poolModeEnabled = $event"
        @update:retry-count="poolModeRetryCount = $event"
      />
    </template>

    <!-- Vertex Service Account -->
    <template v-if="context.accountCategory === 'service_account'">
      <div class="rounded-lg border border-sky-200 bg-sky-50 px-3 py-2 text-xs text-sky-800 dark:border-sky-800/40 dark:bg-sky-900/20 dark:text-sky-200">
        <p>{{ t('admin.accounts.vertexAnthropicHint') }}</p>
      </div>
      <VertexServiceAccount
        :service-account-json="vertexServiceAccountJson"
        :project-id="vertexProjectId"
        :client-email="vertexClientEmail"
        :location="vertexLocation"
        @update:service-account-json="vertexServiceAccountJson = $event"
        @update:project-id="vertexProjectId = $event"
        @update:client-email="vertexClientEmail = $event"
        @update:location="vertexLocation = $event"
        @parse-error="appStore.showError($event)"
      />
      <ModelRestrictionSection
        platform="anthropic"
        :mode="modelRestrictionMode"
        :allowed-models="allowedModels"
        :mappings="modelMappings"
        @update:mode="modelRestrictionMode = $event"
        @update:allowed-models="allowedModels = $event"
        @update:mappings="modelMappings = $event"
      />
    </template>

    <!-- OAuth-based: Add method selector -->
    <div v-if="context.accountCategory === 'oauth-based'" class="space-y-3">
      <label class="input-label">{{ t('admin.accounts.addMethod') }}</label>
      <div class="flex gap-4">
        <label class="flex cursor-pointer items-center">
          <input v-model="addMethod" type="radio" value="oauth"
            class="mr-2 text-primary-600 focus:ring-primary-500" />
          <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.types.oauth') }}</span>
        </label>
        <label class="flex cursor-pointer items-center">
          <input v-model="addMethod" type="radio" value="setup-token"
            class="mr-2 text-primary-600 focus:ring-primary-500" />
          <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.setupTokenLongLived') }}</span>
        </label>
      </div>
    </div>

    <!-- OAuth-based: Quota Control -->
    <template v-if="context.accountCategory === 'oauth-based'">
      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <h3 class="text-sm font-medium text-gray-900 dark:text-gray-100">
          {{ t('admin.accounts.quotaControl.title') }}
        </h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.quotaControl.hint') }}
        </p>
      </div>

      <!-- Window Cost Limit -->
      <ToggleCard
        :label="t('admin.accounts.quotaControl.windowCost.label')"
        :hint="t('admin.accounts.quotaControl.windowCost.hint')"
        :enabled="windowCostEnabled"
        @update:enabled="windowCostEnabled = $event"
      >
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.windowCost.limit') }}</label>
            <div class="relative">
              <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500">$</span>
              <input v-model.number="windowCostLimit" type="number" min="0" step="1"
                class="input pl-7" :placeholder="t('admin.accounts.quotaControl.windowCost.limitPlaceholder')" />
            </div>
            <p class="input-hint">{{ t('admin.accounts.quotaControl.windowCost.limitHint') }}</p>
          </div>
          <div>
            <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.windowCost.stickyReserve') }}</label>
            <div class="relative">
              <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500">$</span>
              <input v-model.number="windowCostStickyReserve" type="number" min="0" step="1"
                class="input pl-7" :placeholder="t('admin.accounts.quotaControl.windowCost.stickyReservePlaceholder')" />
            </div>
            <p class="input-hint">{{ t('admin.accounts.quotaControl.windowCost.stickyReserveHint') }}</p>
          </div>
        </div>
      </ToggleCard>

      <!-- Session Limit -->
      <ToggleCard
        :label="t('admin.accounts.quotaControl.sessionLimit.label')"
        :hint="t('admin.accounts.quotaControl.sessionLimit.hint')"
        :enabled="sessionLimitEnabled"
        @update:enabled="sessionLimitEnabled = $event"
      >
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.sessionLimit.maxSessions') }}</label>
            <input v-model.number="maxSessions" type="number" min="1"
              class="input" :placeholder="t('admin.accounts.quotaControl.sessionLimit.maxSessionsPlaceholder')" />
            <p class="input-hint">{{ t('admin.accounts.quotaControl.sessionLimit.maxSessionsHint') }}</p>
          </div>
          <div>
            <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.sessionLimit.idleTimeout') }}</label>
            <div class="relative">
              <input v-model.number="sessionIdleTimeout" type="number" min="1"
                class="input pr-8" :placeholder="t('admin.accounts.quotaControl.sessionLimit.idleTimeoutPlaceholder')" />
              <span class="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-gray-500">min</span>
            </div>
            <p class="input-hint">{{ t('admin.accounts.quotaControl.sessionLimit.idleTimeoutHint') }}</p>
          </div>
        </div>
      </ToggleCard>

      <!-- RPM Limit -->
      <ToggleCard
        :label="t('admin.accounts.quotaControl.rpmLimit.label')"
        :hint="t('admin.accounts.quotaControl.rpmLimit.hint')"
        :enabled="rpmLimitEnabled"
        @update:enabled="rpmLimitEnabled = $event"
      >
        <div class="space-y-4">
          <div>
            <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.rpmLimit.baseRpm') }}</label>
            <input v-model.number="baseRpm" type="number" min="1" max="1000"
              class="input" :placeholder="t('admin.accounts.quotaControl.rpmLimit.baseRpmPlaceholder')" />
            <p class="input-hint">{{ t('admin.accounts.quotaControl.rpmLimit.baseRpmHint') }}</p>
          </div>

          <div>
            <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.rpmLimit.strategy') }}</label>
            <div class="flex gap-2">
              <button type="button" @click="rpmStrategy = 'tiered'"
                :class="['flex-1 rounded-lg px-3 py-2 text-sm font-medium transition-all',
                  rpmStrategy === 'tiered'
                    ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500']"
              >
                {{ t('admin.accounts.quotaControl.rpmLimit.strategyTiered') }}
              </button>
              <button type="button" @click="rpmStrategy = 'sticky_exempt'"
                :class="['flex-1 rounded-lg px-3 py-2 text-sm font-medium transition-all',
                  rpmStrategy === 'sticky_exempt'
                    ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500']"
              >
                {{ t('admin.accounts.quotaControl.rpmLimit.strategyStickyExempt') }}
              </button>
            </div>
            <p class="input-hint">{{ t('admin.accounts.quotaControl.rpmLimit.strategyHint') }}</p>
          </div>

          <div v-if="rpmStrategy === 'tiered'">
            <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.rpmLimit.stickyBuffer') }}</label>
            <input v-model.number="rpmStickyBuffer" type="number" min="1" step="1"
              class="input" :placeholder="t('admin.accounts.quotaControl.rpmLimit.stickyBufferPlaceholder')" />
            <p class="input-hint">{{ t('admin.accounts.quotaControl.rpmLimit.stickyBufferHint') }}</p>
          </div>
        </div>
      </ToggleCard>

      <!-- User Message Queue Mode (independent of RPM toggle) -->
      <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
        <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.rpmLimit.userMsgQueue') }}</label>
        <p class="mt-1 mb-2 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.quotaControl.rpmLimit.userMsgQueueHint') }}
        </p>
        <div class="flex space-x-2">
          <button type="button" v-for="opt in umqModeOptions" :key="opt.value"
            @click="userMsgQueueMode = userMsgQueueMode === opt.value ? '' : opt.value"
            :class="['px-3 py-1.5 text-sm rounded-md border transition-colors',
              userMsgQueueMode === opt.value
                ? 'bg-primary-600 text-white border-primary-600'
                : 'bg-white dark:bg-dark-700 text-gray-700 dark:text-gray-300 border-gray-300 dark:border-dark-500 hover:bg-gray-50 dark:hover:bg-dark-600']"
          >
            {{ opt.label }}
          </button>
        </div>
      </div>

      <!-- TLS Fingerprint -->
      <ToggleCard
        :label="t('admin.accounts.quotaControl.tlsFingerprint.label')"
        :hint="t('admin.accounts.quotaControl.tlsFingerprint.hint')"
        :enabled="tlsFingerprintEnabled"
        @update:enabled="tlsFingerprintEnabled = $event"
      >
        <select v-model="tlsFingerprintProfileId" class="input">
          <option :value="null">{{ t('admin.accounts.quotaControl.tlsFingerprint.defaultProfile') }}</option>
          <option v-if="tlsFingerprintProfiles.length > 0" :value="-1">
            {{ t('admin.accounts.quotaControl.tlsFingerprint.randomProfile') }}
          </option>
          <option v-for="p in tlsFingerprintProfiles" :key="p.id" :value="p.id">
            {{ p.name }}
          </option>
        </select>
      </ToggleCard>

      <!-- Session ID Masking -->
      <ToggleCard
        :label="t('admin.accounts.quotaControl.sessionIdMasking.label')"
        :hint="t('admin.accounts.quotaControl.sessionIdMasking.hint')"
        :enabled="sessionIdMaskingEnabled"
        @update:enabled="sessionIdMaskingEnabled = $event"
      />

      <!-- Cache TTL Override -->
      <ToggleCard
        :label="t('admin.accounts.quotaControl.cacheTTLOverride.label')"
        :hint="t('admin.accounts.quotaControl.cacheTTLOverride.hint')"
        :enabled="cacheTTLOverrideEnabled"
        @update:enabled="cacheTTLOverrideEnabled = $event"
      >
        <div>
          <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.cacheTTLOverride.target') }}</label>
          <select v-model="cacheTTLOverrideTarget" class="input">
            <option value="5m">5m</option>
            <option value="1h">1h</option>
          </select>
          <p class="input-hint">{{ t('admin.accounts.quotaControl.cacheTTLOverride.targetHint') }}</p>
        </div>
      </ToggleCard>

      <!-- Custom Base URL -->
      <ToggleCard
        :label="t('admin.accounts.quotaControl.customBaseUrl.label')"
        :hint="t('admin.accounts.quotaControl.customBaseUrl.hint')"
        :enabled="customBaseUrlEnabled"
        @update:enabled="customBaseUrlEnabled = $event"
      >
        <div>
          <input v-model="customBaseUrl" type="text" class="input"
            placeholder="https://relay.example.com" />
          <p class="input-hint">{{ t('admin.accounts.quotaControl.customBaseUrl.urlHint') }}</p>
        </div>
      </ToggleCard>
    </template>

    <!-- Intercept warmup -->
    <ToggleCard
      v-if="context.accountCategory !== 'service_account'"
      :label="t('admin.accounts.interceptWarmupRequests')"
      :hint="t('admin.accounts.interceptWarmupRequestsDesc')"
      :enabled="interceptWarmupRequests"
      @update:enabled="interceptWarmupRequests = $event"
    />

    <!-- Temp Unsched -->
    <TempUnschedSection
      :enabled="tempUnschedEnabled"
      :rules="tempUnschedRules"
      @update:enabled="tempUnschedEnabled = $event"
      @update:rules="tempUnschedRules = $event"
    />
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import ModelRestrictionSection from '../widgets/ModelRestrictionSection.vue'
import PoolModeSection from '../widgets/PoolModeSection.vue'
import CustomErrorCodesSection from '../widgets/CustomErrorCodesSection.vue'
import TempUnschedSection from '../widgets/TempUnschedSection.vue'
import ToggleCard from '../widgets/ToggleCard.vue'
import BedrockCredentials from '../widgets/BedrockCredentials.vue'
import VertexServiceAccount from '../widgets/VertexServiceAccount.vue'
import { useAnthropicForm } from './useAnthropicForm'
import type { PlatformFormContext } from './types'

const props = defineProps<{ context: PlatformFormContext }>()
const { t } = useI18n()
const appStore = useAppStore()

const {
  addMethod,
  apiKeyBaseUrl, apiKeyValue,
  bedrockAuthMode, bedrockAccessKeyId, bedrockSecretAccessKey,
  bedrockSessionToken, bedrockRegion, bedrockForceGlobal, bedrockApiKeyValue,
  vertexServiceAccountJson, vertexProjectId, vertexClientEmail, vertexLocation,
  windowCostEnabled, windowCostLimit, windowCostStickyReserve,
  sessionLimitEnabled, maxSessions, sessionIdleTimeout,
  rpmLimitEnabled, baseRpm, rpmStrategy, rpmStickyBuffer,
  userMsgQueueMode, umqModeOptions,
  tlsFingerprintEnabled, tlsFingerprintProfileId, tlsFingerprintProfiles,
  sessionIdMaskingEnabled,
  cacheTTLOverrideEnabled, cacheTTLOverrideTarget,
  customBaseUrlEnabled, customBaseUrl,
  anthropicPassthroughEnabled, webSearchEmulationMode, webSearchGlobalEnabled,
  interceptWarmupRequests,
  modelRestrictionMode, allowedModels, modelMappings,
  poolModeEnabled, poolModeRetryCount,
  customErrorCodesEnabled, selectedErrorCodes,
  tempUnschedEnabled, tempUnschedRules,
  oauth, oauthConfig, validate, getPayload, isOAuthFlow, reset,
  handleOAuthExchange, handleCookieAuth,
  loadTlsProfiles, loadWebSearchEnabled,
  initFromAccount, getEditPayload
} = useAnthropicForm()

onMounted(() => {
  loadTlsProfiles()
  loadWebSearchEnabled()
})

defineExpose({
  validate: () => validate(props.context.accountCategory),
  getPayload: () => getPayload(props.context.accountCategory),
  isOAuthFlow: () => isOAuthFlow(props.context.accountCategory),
  reset: () => reset(),
  initFromAccount,
  getEditPayload,
  oauthConfig,
  getOAuthState: () => ({
    authUrl: oauth.authUrl.value,
    sessionId: oauth.sessionId.value,
    loading: oauth.loading.value,
    error: oauth.error.value
  }),
  generateOAuthUrl: (proxyId: number | null) =>
    oauth.generateAuthUrl(addMethod.value, proxyId),
  resetOAuth: () => oauth.resetState(),
  handleOAuthExchange,
  handleCookieAuth
})
</script>