<template>
  <div class="space-y-5">
    <!-- Name + Notes at top -->
    <AccountCommonFields v-model="commonFields" :context="context" mode="identity" />

    <!-- apikey: credentials (create only) + Model Restriction + Pool Mode + Custom Error Codes -->
    <template v-if="context.accountCategory === 'apikey'">
      <div>
        <label class="input-label">{{ t('admin.accounts.baseUrl') }}</label>
        <input v-model="apiKeyBaseUrl" type="text" class="input" placeholder="https://api.openai.com" />
        <p v-if="context.mode !== 'edit'" class="input-hint">{{ t('admin.accounts.openai.baseUrlHint') }}</p>
      </div>
      <div v-if="context.mode !== 'edit'">
        <label class="input-label">{{ t('admin.accounts.apiKey') }}</label>
        <input v-model="apiKeyValue" type="password" required class="input font-mono" placeholder="sk-proj-..." />
      </div>
      <div v-else>
        <label class="input-label">{{ t('admin.accounts.apiKey') }}</label>
        <input v-model="editApiKey" type="password" class="input font-mono"
          autocomplete="new-password" data-1p-ignore data-lpignore="true" data-bwignore="true"
          placeholder="sk-proj-..." />
        <p class="input-hint">{{ t('admin.accounts.leaveEmptyToKeep') }}</p>
      </div>
      <ModelRestrictionSection
        platform="openai"
        :mode="modelRestrictionMode"
        :allowed-models="allowedModels"
        :mappings="modelMappings"
        :protocols="['openai']"
        :disabled="isModelRestrictionDisabled"
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
    </template>

    <!-- oauth-based: Model Restriction only -->
    <ModelRestrictionSection
      v-if="context.accountCategory === 'oauth-based'"
      platform="openai"
      :mode="modelRestrictionMode"
      :allowed-models="allowedModels"
      :mappings="modelMappings"
      :protocols="['openai']"
      :disabled="isModelRestrictionDisabled"
      @update:mode="modelRestrictionMode = $event"
      @update:allowed-models="allowedModels = $event"
      @update:mappings="modelMappings = $event"
    />

    <!-- Temp Unsched -->
    <TempUnschedSection
      :enabled="tempUnschedEnabled"
      :rules="tempUnschedRules"
      @update:enabled="tempUnschedEnabled = $event"
      @update:rules="tempUnschedRules = $event"
    />

    <!-- Passthrough toggle -->
    <ToggleCard
      :label="t('admin.accounts.openai.oauthPassthrough')"
      :hint="t('admin.accounts.openai.oauthPassthroughDesc')"
      :enabled="openaiPassthroughEnabled"
      @update:enabled="openaiPassthroughEnabled = $event"
    />

    <!-- WS Mode select -->
    <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
      <div class="flex items-center justify-between">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.openai.wsMode') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.openai.wsModeDesc') }}
          </p>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t(wsModeHintKey(context.accountCategory)) }}
          </p>
        </div>
        <div class="w-52">
          <Select
            :model-value="getWSMode(context.accountCategory)"
            :options="openAIWSModeOptions"
            @update:model-value="setWSMode(context.accountCategory, $event as OpenAIWSMode)"
          />
        </div>
      </div>
    </div>

    <!-- Codex CLI Only (oauth only) -->
    <ToggleCard
      v-if="context.accountCategory === 'oauth-based'"
      :label="t('admin.accounts.openai.codexCLIOnly')"
      :hint="t('admin.accounts.openai.codexCLIOnlyDesc')"
      :enabled="codexCLIOnlyEnabled"
      @update:enabled="codexCLIOnlyEnabled = $event"
    />

    <!-- Codex Image Generation Bridge -->
    <CodexImageGenBridgeSection
      :mode="codexImageGenerationBridgeMode"
      :options="codexImageGenerationBridgeOptions"
      :badge-label="codexImageGenerationBridgeBadgeLabel"
      :badge-class="codexImageGenerationBridgeBadgeClass"
      @update:mode="codexImageGenerationBridgeMode = $event"
    />

    <!-- Compact mode + compact model mapping -->
    <CompactModeSection
      :compact-mode="openAICompactMode"
      :compact-mode-options="openAICompactModeOptions"
      :compact-model-mappings="openAICompactModelMappings"
      @update:compact-mode="onCompactModeUpdate($event)"
      @update:compact-model-mappings="openAICompactModelMappings = $event"
    />

    <AccountCommonFields v-model="commonFields" :context="context" mode="settings" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Select,
  ToggleCard,
  ModelRestrictionSection,
  PoolModeSection,
  CustomErrorCodesSection,
  TempUnschedSection,
  AccountCommonFields,
  type CommonAccountFields,
} from '@sub2api/plugin-sdk'
import type { OpenAIWSMode } from '../utils/openaiWsMode'
import type { PlatformFormContext } from '@sub2api/plugin-sdk'
import { useOpenAIForm } from './useOpenAIForm'
import CodexImageGenBridgeSection from './CodexImageGenBridgeSection.vue'
import CompactModeSection from './CompactModeSection.vue'

const props = defineProps<{ context: PlatformFormContext }>()
const { t } = useI18n()

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

const {
  apiKeyBaseUrl, apiKeyValue, editApiKey,
  openaiPassthroughEnabled, openAICompactMode,
  codexCLIOnlyEnabled, codexImageGenerationBridgeMode,
  codexImageGenerationBridgeOptions, codexImageGenerationBridgeBadgeLabel, codexImageGenerationBridgeBadgeClass,
  openAICompactModelMappings,
  modelRestrictionMode, allowedModels, modelMappings,
  poolModeEnabled, poolModeRetryCount,
  customErrorCodesEnabled, selectedErrorCodes,
  tempUnschedEnabled, tempUnschedRules,
  openAICompactModeOptions, openAIWSModeOptions, isModelRestrictionDisabled,
  openaiOAuth, oauthConfig, getWSMode, setWSMode, wsModeHintKey,
  validate, getPayload, reset,
  handleOAuthExchange, handleRefreshToken, handleMobileRefreshToken,
  initFromAccount, getEditPayload
} = useOpenAIForm(commonFields)

type OpenAICompactMode = 'auto' | 'force_on' | 'force_off'
function onCompactModeUpdate(value: string) {
  openAICompactMode.value = value as OpenAICompactMode
}

defineExpose({
  validate: () => validate(props.context.accountCategory, props.context.mode),
  getPayload: () => getPayload(props.context.accountCategory),
  isOAuthFlow: () => props.context.accountCategory === 'oauth-based',
  reset: () => reset(),
  initFromAccount,
  getEditPayload,
  oauthConfig,
  getOAuthState: () => ({
    authUrl: openaiOAuth.authUrl.value,
    sessionId: openaiOAuth.sessionId.value,
    loading: openaiOAuth.loading.value,
    error: openaiOAuth.error.value
  }),
  generateOAuthUrl: (proxyId: number | null) =>
    openaiOAuth.generateAuthUrl(proxyId),
  resetOAuth: () => openaiOAuth.resetState(),
  handleOAuthExchange,
  handleRefreshToken,
  handleMobileRefreshToken
})
</script>