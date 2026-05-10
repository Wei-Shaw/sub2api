<template>
  <div class="space-y-5">
    <!-- Gemini help button -->
    <div class="flex justify-end">
      <button type="button" @click="showGeminiHelpDialog = true"
        class="flex items-center gap-1 rounded px-2 py-1 text-xs text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20">
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
          class="font-medium text-blue-600 hover:underline dark:text-blue-400">
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
    <div v-if="context.accountCategory !== 'service_account'" class="mt-4">
      <label class="input-label">{{ t('admin.accounts.gemini.tier.label') }}</label>
      <div class="mt-2">
        <select v-if="context.accountCategory === 'oauth-based' && geminiOAuthType === 'google_one'"
          v-model="geminiTierGoogleOne" class="input">
          <option value="google_one_free">{{ t('admin.accounts.gemini.tier.googleOne.free') }}</option>
          <option value="google_ai_pro">{{ t('admin.accounts.gemini.tier.googleOne.pro') }}</option>
          <option value="google_ai_ultra">{{ t('admin.accounts.gemini.tier.googleOne.ultra') }}</option>
        </select>
        <select v-else-if="context.accountCategory === 'oauth-based' && geminiOAuthType === 'code_assist'"
          v-model="geminiTierGcp" class="input">
          <option value="gcp_standard">{{ t('admin.accounts.gemini.tier.gcp.standard') }}</option>
          <option value="gcp_enterprise">{{ t('admin.accounts.gemini.tier.gcp.enterprise') }}</option>
        </select>
        <select v-else v-model="geminiTierAIStudio" class="input">
          <option value="aistudio_free">{{ t('admin.accounts.gemini.tier.aiStudio.free') }}</option>
          <option value="aistudio_paid">{{ t('admin.accounts.gemini.tier.aiStudio.paid') }}</option>
        </select>
      </div>
      <p class="input-hint">
        {{ context.accountCategory === 'apikey'
          ? t('admin.accounts.gemini.tier.aiStudioHint')
          : t('admin.accounts.gemini.tier.hint') }}
      </p>
    </div>

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
      @parse-error="appStore.showError($event)"
    />

    <!-- apikey sections -->
    <template v-if="context.accountCategory === 'apikey'">
      <template v-if="context.mode !== 'edit'">
        <div>
          <label class="input-label">{{ t('admin.accounts.baseUrl') }}</label>
          <input v-model="apiKeyBaseUrl" type="text" class="input" placeholder="https://generativelanguage.googleapis.com" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.apiKey') }}</label>
          <input v-model="apiKeyValue" type="password" required class="input font-mono" placeholder="AIza..." />
        </div>
      </template>
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
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import Icon from '@/components/icons/Icon.vue'
import ModelRestrictionSection from '../widgets/ModelRestrictionSection.vue'
import PoolModeSection from '../widgets/PoolModeSection.vue'
import CustomErrorCodesSection from '../widgets/CustomErrorCodesSection.vue'
import TempUnschedSection from '../widgets/TempUnschedSection.vue'
import VertexServiceAccount from '../widgets/VertexServiceAccount.vue'
import GeminiOAuthTypeSelector from './GeminiOAuthTypeSelector.vue'
import { useGeminiForm } from './useGeminiForm'
import type { PlatformFormContext } from './types'

const props = defineProps<{ context: PlatformFormContext }>()
const { t } = useI18n()
const appStore = useAppStore()
const {
  apiKeyBaseUrl, apiKeyValue,
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
} = useGeminiForm()

onMounted(() => {
  if (props.context.accountCategory === 'oauth-based') {
    checkAIStudioCapability()
  }
})

defineExpose({
  validate: () => validate(props.context.accountCategory),
  getPayload: () => getPayload(props.context.accountCategory),
  isOAuthFlow: () => props.context.accountCategory === 'oauth-based',
  reset: () => reset(),
  initFromAccount,
  getEditPayload,
  oauthConfig: {
    ...oauthConfig,
    showProjectId: geminiOAuthType.value === 'code_assist'
  },
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
