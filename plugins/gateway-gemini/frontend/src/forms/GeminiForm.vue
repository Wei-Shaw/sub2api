<template>
  <div class="space-y-5">
    <!-- Name + Notes at top -->
    <AccountCommonFields v-model="commonFields" :context="context" mode="identity" />

    <!-- Gemini help button -->
    <div class="flex justify-end">
      <button type="button" @click="showGeminiHelpDialog = true"
        class="btn-ghost-info flex items-center gap-1 rounded px-2 py-1 text-xs">
        <Icon name="questionCircle" size="sm" />
        {{ t('admin.accounts.gemini.helpButton') }}
      </button>
    </div>

    <!-- apikey note -->
    <div v-if="context.accountCategory === 'apikey'"
      class="rounded-lg border border-purple-200 bg-purple-50 px-3 py-2 text-xs text-purple-800 dark:border-purple-800/40 dark:bg-purple-900/20 dark:text-purple-200">
      <p>{{ t('admin.accounts.gemini.accountType.apiKeyNote') }}</p>
      <div class="mt-2 flex flex-wrap gap-2">
        <a :href="geminiHelpLinks.apiKey" target="_blank" rel="noreferrer"
          class="text-semantic-info font-medium hover:underline">
          {{ t('admin.accounts.gemini.accountType.apiKeyLink') }}
        </a>
      </div>
    </div>

    <!-- service_account note -->
    <div v-if="context.accountCategory === 'service_account'"
      class="rounded-lg border border-sky-200 bg-sky-50 px-3 py-2 text-xs text-sky-800 dark:border-sky-800/40 dark:bg-sky-900/20 dark:text-sky-200">
      <p>{{ t('admin.accounts.vertexGeminiHint') }}</p>
    </div>

    <!-- OAuth sub-type selector -->
    <GeminiOAuthTypeSelector
      v-if="context.accountCategory === 'oauth-based'"
      :oauth-type="geminiOAuthType"
      :ai-studio-enabled="geminiAIStudioOAuthEnabled"
      :show-advanced="showAdvancedOAuth"
      :help-links="geminiHelpLinks"
      @select="selectOAuthType($event)"
      @toggle-advanced="showAdvancedOAuth = !showAdvancedOAuth"
    />

    <!-- Tier selection (not for service_account) -->
    <GeminiTierSelector
      v-if="context.accountCategory !== 'service_account'"
      :account-category="context.accountCategory"
      :oauth-type="geminiOAuthType"
      :tier-google-one="geminiTierGoogleOne"
      :tier-gcp="geminiTierGcp"
      :tier-ai-studio="geminiTierAIStudio"
      @update:tier-google-one="onTierGoogleOneChange"
      @update:tier-gcp="onTierGcpChange"
      @update:tier-ai-studio="onTierAiStudioChange"
    />

    <!-- Vertex Service Account -->
    <VertexServiceAccount
      v-if="context.accountCategory === 'service_account'"
      :service-account-json="vertexServiceAccountJson"
      :project-id="vertexProjectId"
      :client-email="vertexClientEmail"
      :location="vertexLocation"
      @update:service-account-json="vertexServiceAccountJson = $event"
      @update:project-id="vertexProjectId = $event"
      @update:client-email="vertexClientEmail = $event"
      @update:location="vertexLocation = $event"
      @parse-error="notifyError($event)"
    />

    <!-- apikey sections -->
    <template v-if="context.accountCategory === 'apikey'">
      <div>
        <label class="input-label">{{ t('admin.accounts.baseUrl') }}</label>
        <input v-model="apiKeyBaseUrl" type="text" class="input" placeholder="https://generativelanguage.googleapis.com" />
        <p v-if="context.mode !== 'edit'" class="input-hint">{{ t('admin.accounts.gemini.baseUrlHint') }}</p>
      </div>
      <div v-if="context.mode !== 'edit'">
        <label class="input-label">{{ t('admin.accounts.apiKey') }}</label>
        <input v-model="apiKeyValue" type="password" required class="input font-mono" placeholder="AIza..." />
      </div>
      <div v-else>
        <label class="input-label">{{ t('admin.accounts.apiKey') }}</label>
        <input v-model="editApiKey" type="password" class="input font-mono"
          autocomplete="new-password" data-1p-ignore data-lpignore="true" data-bwignore="true"
          placeholder="AIza..." />
        <p class="input-hint">{{ t('admin.accounts.leaveEmptyToKeep') }}</p>
      </div>
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

    <!-- Model restriction for apikey and service_account -->
    <ModelRestrictionSection
      v-if="context.accountCategory === 'apikey' || context.accountCategory === 'service_account'"
      platform="gemini"
      :mode="modelRestrictionMode"
      :allowed-models="allowedModels"
      :mappings="modelMappings"
      :protocols="['gemini']"
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

    <AccountCommonFields v-model="commonFields" :context="context" mode="settings" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Icon,
  ModelRestrictionSection,
  PoolModeSection,
  CustomErrorCodesSection,
  TempUnschedSection,
  VertexServiceAccount,
  AccountCommonFields,
  type PlatformFormContext,
  type CommonAccountFields,
} from '@sub2api/plugin-sdk'
import { getSdk } from '../api/sdk'
import GeminiOAuthTypeSelector from './GeminiOAuthTypeSelector.vue'
import GeminiTierSelector from './GeminiTierSelector.vue'
import { useGeminiForm } from './useGeminiForm'

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

function notifyError(message: string) {
  getSdk().notify.error(message)
}

function onTierGoogleOneChange(v: string) {
  geminiTierGoogleOne.value = v as typeof geminiTierGoogleOne.value
}
function onTierGcpChange(v: string) {
  geminiTierGcp.value = v as typeof geminiTierGcp.value
}
function onTierAiStudioChange(v: string) {
  geminiTierAIStudio.value = v as typeof geminiTierAIStudio.value
}

const {
  apiKeyBaseUrl, apiKeyValue, editApiKey,
  geminiOAuthType, geminiAIStudioOAuthEnabled,
  showAdvancedOAuth, showGeminiHelpDialog,
  geminiTierGoogleOne, geminiTierGcp, geminiTierAIStudio,
  vertexServiceAccountJson, vertexProjectId, vertexClientEmail, vertexLocation,
  modelRestrictionMode, allowedModels, modelMappings,
  poolModeEnabled, poolModeRetryCount,
  customErrorCodesEnabled, selectedErrorCodes,
  tempUnschedEnabled, tempUnschedRules,
  geminiHelpLinks, geminiOAuth, geminiSelectedTier, oauthConfig,
  selectOAuthType, checkAIStudioCapability,
  validate, getPayload, reset, handleOAuthExchange,
  initFromAccount, getEditPayload
} = useGeminiForm(commonFields)

onMounted(() => {
  if (props.context.accountCategory === 'oauth-based') {
    checkAIStudioCapability()
  }
})

const reactiveOAuthConfig = computed(() => ({
  ...oauthConfig,
  showProjectId: geminiOAuthType.value === 'code_assist'
}))

defineExpose({
  validate: () => validate(props.context.accountCategory, props.context.mode),
  getPayload: () => getPayload(props.context.accountCategory),
  isOAuthFlow: () => props.context.accountCategory === 'oauth-based',
  reset: () => reset(),
  initFromAccount,
  getEditPayload,
  get oauthConfig() { return reactiveOAuthConfig.value },
  getOAuthState: () => ({
    authUrl: geminiOAuth.authUrl.value,
    sessionId: geminiOAuth.sessionId.value,
    loading: geminiOAuth.loading.value,
    error: geminiOAuth.error.value
  }),
  generateOAuthUrl: (proxyId: number | null, projectId?: string) =>
    geminiOAuth.generateAuthUrl(proxyId, projectId, geminiOAuthType.value, geminiSelectedTier.value),
  resetOAuth: () => geminiOAuth.resetState(),
  handleOAuthExchange
})
</script>
