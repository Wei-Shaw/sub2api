<template>
  <div class="space-y-5">
    <!-- apikey: API Key credentials + Model Restriction + Pool Mode + Custom Error Codes -->
    <template v-if="context.accountCategory === 'apikey'">
      <div>
        <label class="input-label">{{ t('admin.accounts.baseUrl') }}</label>
        <input v-model="apiKeyBaseUrl" type="text" class="input" placeholder="https://api.openai.com" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.apiKey') }}</label>
        <input v-model="apiKeyValue" type="password" required class="input font-mono" placeholder="sk-..." />
      </div>
      <ModelRestrictionSection
        platform="openai"
        :mode="modelRestrictionMode"
        :allowed-models="allowedModels"
        :mappings="modelMappings"
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

    <!-- Compact mode + compact model mapping -->
    <div class="border-t border-gray-200 pt-4 dark:border-dark-600 space-y-4">
      <div class="flex items-center justify-between">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.openai.compactMode') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.openai.compactModeDesc') }}
          </p>
        </div>
        <div class="w-44">
          <Select v-model="openAICompactMode" :options="openAICompactModeOptions" />
        </div>
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.openai.compactModelMapping') }}</label>
        <p class="input-hint">{{ t('admin.accounts.openai.compactModelMappingDesc') }}</p>
        <div v-if="openAICompactModelMappings.length > 0" class="mb-3 space-y-2">
          <div v-for="(mapping, index) in openAICompactModelMappings"
            :key="getCompactKey(mapping)" class="flex items-center gap-2">
            <input v-model="mapping.from" type="text" class="input flex-1"
              :placeholder="t('admin.accounts.fromModel')" />
            <span class="text-gray-400">&rarr;</span>
            <input v-model="mapping.to" type="text" class="input flex-1"
              :placeholder="t('admin.accounts.toModel')" />
            <button type="button" @click="openAICompactModelMappings.splice(index, 1)"
              class="text-red-500 hover:text-red-700">
              <Icon name="trash" size="sm" />
            </button>
          </div>
        </div>
        <button type="button"
          @click="openAICompactModelMappings.push({ from: '', to: '' })"
          class="btn btn-secondary text-sm">
          + {{ t('admin.accounts.addMapping') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { createStableObjectKeyResolver } from '@/utils/stableObjectKey'
import { Select } from '@sub2api/plugin-sdk'
import type { OpenAIWSMode } from '@/utils/openaiWsMode'
import Icon from '@/components/icons/Icon.vue'
import ModelRestrictionSection from '../widgets/ModelRestrictionSection.vue'
import PoolModeSection from '../widgets/PoolModeSection.vue'
import CustomErrorCodesSection from '../widgets/CustomErrorCodesSection.vue'
import TempUnschedSection from '../widgets/TempUnschedSection.vue'
import ToggleCard from '../widgets/ToggleCard.vue'
import { useOpenAIForm } from './useOpenAIForm'
import type { PlatformFormContext } from './types'

interface ModelMapping { from: string; to: string }

const props = defineProps<{ context: PlatformFormContext }>()
const { t } = useI18n()
const {
  apiKeyBaseUrl, apiKeyValue,
  openaiPassthroughEnabled, openAICompactMode,
  codexCLIOnlyEnabled, openAICompactModelMappings,
  modelRestrictionMode, allowedModels, modelMappings,
  poolModeEnabled, poolModeRetryCount,
  customErrorCodesEnabled, selectedErrorCodes,
  tempUnschedEnabled, tempUnschedRules,
  openAICompactModeOptions, openAIWSModeOptions, isModelRestrictionDisabled,
  openaiOAuth, oauthConfig, getWSMode, setWSMode, wsModeHintKey,
  validate, getPayload, reset,
  handleOAuthExchange, handleRefreshToken, handleMobileRefreshToken,
  initFromAccount, getEditPayload
} = useOpenAIForm()

const getCompactKey = createStableObjectKeyResolver<ModelMapping>('openai-compact-mm')

defineExpose({
  validate: () => validate(props.context.accountCategory),
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
