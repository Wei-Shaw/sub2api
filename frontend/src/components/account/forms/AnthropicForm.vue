<template>
  <div class="space-y-5">
    <!-- API Key inputs -->
    <template v-if="context.accountCategory === 'apikey'">
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
        :label="t('admin.accounts.anthropicPassthrough')"
        :hint="t('admin.accounts.anthropicPassthroughDesc')"
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
    </template>

    <!-- OAuth-based: Add method selector -->
    <div v-if="context.accountCategory === 'oauth-based'" class="space-y-3">
      <label class="input-label">{{ t('admin.accounts.addMethod') }}</label>
      <div class="flex gap-4">
        <label class="flex cursor-pointer items-center">
          <input v-model="addMethod" type="radio" value="oauth"
            class="mr-2 text-primary-600 focus:ring-primary-500" />
          <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.oauthMethod') }}</span>
        </label>
        <label class="flex cursor-pointer items-center">
          <input v-model="addMethod" type="radio" value="setup-token"
            class="mr-2 text-primary-600 focus:ring-primary-500" />
          <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.setupTokenMethod') }}</span>
        </label>
      </div>
    </div>

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
