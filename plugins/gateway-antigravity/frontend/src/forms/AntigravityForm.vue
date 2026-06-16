<template>
  <div class="space-y-5">
    <!-- Name + Notes at top -->
    <AccountCommonFields v-model="commonFields" :context="context" mode="identity" />

    <!-- Upstream config -->
    <div v-if="antigravityAccountType === 'upstream'" class="space-y-4">
      <div>
        <label class="input-label">{{ t('admin.accounts.upstream.baseUrl') }}</label>
        <input v-model="upstreamBaseUrl" type="text" :required="context.mode !== 'edit'" class="input"
          placeholder="https://cloudcode-pa.googleapis.com" />
        <p class="input-hint">{{ t('admin.accounts.upstream.baseUrlHint') }}</p>
      </div>
      <div v-if="context.mode !== 'edit'">
        <label class="input-label">{{ t('admin.accounts.upstream.apiKey') }}</label>
        <input v-model="upstreamApiKey" type="password" required class="input font-mono"
          autocomplete="new-password" data-1p-ignore data-lpignore="true" data-bwignore="true"
          placeholder="sk-..." />
        <p class="input-hint">{{ t('admin.accounts.upstream.apiKeyHint') }}</p>
      </div>
      <div v-else>
        <label class="input-label">{{ t('admin.accounts.upstream.apiKey') }}</label>
        <input v-model="editUpstreamApiKey" type="password" class="input font-mono"
          autocomplete="new-password" data-1p-ignore data-lpignore="true" data-bwignore="true"
          placeholder="sk-..." />
        <p class="input-hint">{{ t('admin.accounts.leaveEmptyToKeep') }}</p>
      </div>
    </div>

    <!-- Model mapping (mapping-only, no whitelist toggle) -->
    <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
      <label class="input-label">{{ t('admin.accounts.modelRestriction') }}</label>
      <div class="mb-3 rounded-lg bg-purple-50 p-3 dark:bg-purple-900/20">
        <p class="text-xs text-purple-700 dark:text-purple-400">
          {{ t('admin.accounts.mapRequestModels') }}
        </p>
      </div>

      <div v-if="antigravityModelMappings.length > 0" class="mb-3 space-y-2">
        <div v-for="(mapping, index) in antigravityModelMappings" :key="getMappingKey(mapping)"
          class="space-y-1">
          <div class="flex items-center gap-2">
            <input v-model="mapping.from" type="text"
              :class="['input flex-1', !isValidWildcardPattern(mapping.from) ? 'border-red-500 dark:border-red-500' : '']"
              :placeholder="t('admin.accounts.requestModel')" />
            <Icon name="arrowRight" size="sm" class="flex-shrink-0 text-gray-400" />
            <input v-model="mapping.to" type="text"
              :class="['input flex-1', mapping.to.includes('*') ? 'border-red-500 dark:border-red-500' : '']"
              :placeholder="t('admin.accounts.actualModel')" />
            <button type="button" @click="removeModelMapping(index)"
              class="btn-ghost-danger rounded-lg p-2 transition-colors">
              <Icon name="trash" size="sm" />
            </button>
          </div>
          <p v-if="!isValidWildcardPattern(mapping.from)" class="text-semantic-danger text-xs">
            {{ t('admin.accounts.wildcardOnlyAtEnd') }}
          </p>
          <p v-if="mapping.to.includes('*')" class="text-semantic-danger text-xs">
            {{ t('admin.accounts.targetNoWildcard') }}
          </p>
        </div>
      </div>

      <button type="button" @click="addModelMapping()"
        class="mb-3 w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-gray-600 transition-colors hover:border-gray-400 hover:text-gray-700 dark:border-dark-500 dark:text-gray-400 dark:hover:border-dark-400 dark:hover:text-gray-300">
        + {{ t('admin.accounts.addMapping') }}
      </button>

      <div class="flex flex-wrap gap-2">
        <button v-for="preset in antigravityPresetMappings" :key="preset.label" type="button"
          @click="addPresetMapping(preset.from, preset.to)"
          :class="['rounded-lg px-3 py-1 text-xs transition-colors', preset.color]">
          + {{ preset.label }}
        </button>
      </div>
    </div>

    <!-- Intercept warmup -->
    <ToggleCard
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

    <!-- Mixed scheduling + Allow overages -->
    <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
      <CheckboxWithTooltip
        v-model="mixedScheduling"
        :label="t('admin.accounts.mixedScheduling')"
        :tooltip="t('admin.accounts.mixedSchedulingTooltip')"
      />
      <CheckboxWithTooltip
        v-model="allowOverages"
        :label="t('admin.accounts.allowOverages')"
        :tooltip="t('admin.accounts.allowOveragesTooltip')"
        class="mt-3"
      />
    </div>

    <AccountCommonFields v-model="commonFields" :context="context" mode="settings" />
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Icon,
  ToggleCard,
  TempUnschedSection,
  AccountCommonFields,
  createStableObjectKeyResolver,
  type PlatformFormContext,
  type ModelMapping,
  type CommonAccountFields,
} from '@sub2api/plugin-sdk'
import CheckboxWithTooltip from './CheckboxWithTooltip.vue'
import { useAntigravityForm } from './useAntigravityForm'
import { isValidWildcardPattern } from './modelMapping'

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
  antigravityAccountType, upstreamBaseUrl, upstreamApiKey, editUpstreamApiKey,
  antigravityModelMappings, antigravityPresetMappings,
  mixedScheduling, allowOverages, interceptWarmupRequests,
  antigravityOAuth, oauthConfig, validate, getPayload, isOAuthFlow, reset,
  handleOAuthExchange, handleRefreshToken,
  addModelMapping, removeModelMapping, addPresetMapping,
  tempUnschedEnabled, tempUnschedRules,
  initFromAccount, getEditPayload,
} = useAntigravityForm(commonFields)

watch(() => props.context.accountTypeId, (id) => {
  antigravityAccountType.value = id === 'upstream' ? 'upstream' : 'oauth'
}, { immediate: true })

const getMappingKey = createStableObjectKeyResolver<ModelMapping>('ag-mapping')

defineExpose({
  validate: () => validate(props.context.mode),
  getPayload: () => getPayload(),
  isOAuthFlow: () => isOAuthFlow(),
  reset: () => reset(),
  initFromAccount,
  getEditPayload,
  oauthConfig,
  getOAuthState: () => ({
    authUrl: antigravityOAuth.authUrl.value,
    sessionId: antigravityOAuth.sessionId.value,
    loading: antigravityOAuth.loading.value,
    error: antigravityOAuth.error.value,
  }),
  generateOAuthUrl: (proxyId: number | null) =>
    antigravityOAuth.generateAuthUrl(proxyId),
  resetOAuth: () => antigravityOAuth.resetState(),
  handleOAuthExchange,
  handleRefreshToken,
})
</script>
