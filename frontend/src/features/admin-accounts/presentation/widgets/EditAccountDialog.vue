<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.editAccount')"
    width="wide"
    @close="handleClose"
  >
    <form
      v-if="account"
      id="edit-account-form"
      @submit.prevent="handleSubmit"
      class="space-y-5"
    >
      <EditAccountCredentialFields :context="editAccountCredentialContext" />

      <GrokModelDiagnosticsPanel v-if="show && account.platform === 'grok'" :account-id="account.id" />

      <OpenAIImageSizeTiersField v-if="account?.platform === 'openai'" v-model="openAIImageSizeTiers" />

      <EditAccountAdvancedFields :context="editAccountAdvancedContext" />

      <EditAccountPolicyFields :context="editAccountPolicyContext" />

    </form>

    <template #footer>
      <div v-if="account" class="flex justify-end gap-3">
        <button @click="handleClose" type="button" class="btn btn-secondary">
          {{ t('common.cancel') }}
        </button>
        <button
          type="submit"
          form="edit-account-form"
          :disabled="submitting"
          class="btn btn-primary"
          data-tour="account-form-submit"
        >
          <svg
            v-if="submitting"
            class="-ml-1 mr-2 h-4 w-4 animate-spin"
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
          {{ submitting ? t('admin.accounts.updating') : t('common.update') }}
        </button>
      </div>
    </template>
  </BaseDialog>

  <!-- Mixed Channel Warning Dialog -->
  <ConfirmDialog
    :show="showMixedChannelWarning"
    :title="t('admin.accounts.mixedChannelWarningTitle')"
    :message="mixedChannelWarningMessageText"
    :confirm-text="t('common.confirm')"
    :cancel-text="t('common.cancel')"
    :danger="true"
    @confirm="handleMixedChannelConfirm"
    @cancel="handleMixedChannelCancel"
  />
</template>

<script setup lang="ts">
import GrokModelDiagnosticsPanel from './GrokModelDiagnosticsPanel.vue'
import { supportsAccountQuota } from '@/features/admin-accounts/presentation/accountQuota'

import { ref, reactive, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/core/stores/appStore'
import { useAuthStore } from '@/features/auth'
import { syncUpstreamModels } from '@/features/admin-accounts/data/datasources/adminAccountActions'
import {
  getWebSearchEmulationConfig
} from '@/features/admin-settings/data/datasources/adminSettingsQueries'
import {
  list as listTLSFingerprintProfiles
} from '@/features/admin-settings/data/datasources/tlsFingerprintProfileDatasource'
import { useQuotaNotifyState } from '@/features/admin-accounts/presentation/composables/useQuotaNotifyState'
import type {
  Account,
  Proxy,
  AdminGroup,
  OpenAICompactMode,
  OpenAIResponsesMode,
  OpenAIEndpointCapability,
  OllamaCloudUsageState
} from '@/types'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import ConfirmDialog from '@/common/widgets/feedback/ConfirmDialog.vue'
import {
  buildPlanTypeOptions,
  readPlanType,
  isCustomGrokBaseUrl,
  isHeaderOverrideCapable,
  splitHeaderOverridesObject,
  HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY,
  HEADER_OVERRIDES_CREDENTIAL_KEY,
  type HeaderOverrideRow
} from '@/features/admin-accounts/presentation/credentialsBuilder'
import { formatDateTime, formatDateTimeLocalInput, parseDateTimeLocalInput } from '@/core/utils/format'
import { createStableObjectKeyResolver } from '@/core/utils/stableObjectKey'
import {
  CN_ACCOUNT_PLATFORM_LABELS,
  VERTEX_LOCATION_OPTIONS,
  defaultCNAdaptiveBaseURLs,
  defaultOpenCodeProtocolRules,
  defaultAPIKeyBaseURL,
  isCNAccountPlatform,
  type CNAdaptiveBaseURLs,
  type OpenCodeProtocolRule,
} from '@/core/constants/account'
import {
  OPENAI_WS_MODE_CTX_POOL,
  OPENAI_WS_MODE_OFF,
  OPENAI_WS_MODE_PASSTHROUGH,
  OPENAI_WS_MODE_HTTP_BRIDGE,
  resolveOpenAIWSModeConcurrencyHintKey,
  type OpenAIWSMode,
  resolveOpenAIWSModeFromExtra
} from '@/core/utils/openaiWsMode'
import {
  getPresetMappingsByPlatform,
  commonErrorCodes,
  buildModelMappingObject,
  splitModelMappingObject,
  isValidWildcardPattern
} from '@/features/admin-accounts/presentation/composables/useModelWhitelist'
import EditAccountCredentialFields from './edit/EditAccountCredentialFields.vue'
import EditAccountAdvancedFields from './edit/EditAccountAdvancedFields.vue'
import OpenAIImageSizeTiersField from './OpenAIImageSizeTiersField.vue'
import { OPENAI_IMAGE_SIZE_TIERS, readOpenAIImageSizeTiers, type OpenAIImageSizeTier } from '../openaiImageSizeTiers'
import EditAccountPolicyFields from './edit/EditAccountPolicyFields.vue'
import { useEditAccountSubmission } from '../composables/useEditAccountSubmission'
import { useCPATestConnection } from '../composables/useCPATestConnection'
import {
  DEFAULT_POOL_MODE_RETRY_COUNT,
  DEFAULT_POOL_MODE_RETRY_STATUS_CODES,
  MAX_POOL_MODE_RETRY_COUNT,
  addEmptyModelMapping,
  addPresetModelMapping,
  buildTempUnschedRules,
  createTempUnschedRule,
  parseTempUnschedRules,
  formatPoolModeRetryStatusCodes,
  getCodexFingerprintModeOptions,
  isOpenAIPersonalAccessTokenCredentials,
  moveTempUnschedRule as moveTempUnschedRuleInPlace,
  normalizeCodexFingerprintMode,
  normalizeOpenAIEndpointCapabilities,
  normalizePoolModeRetryCount,
  readCodexWebSearchEnabled,
  readOpenAIEndpointCapabilities,
  removeModelMapping as removeModelMappingAt,
  type CodexFingerprintMode,
  type ModelMapping,
  type TempUnschedRuleForm,
} from '@/features/admin-accounts/presentation/accountFormPolicy'
import type {
  EditAccountAdvancedContext,
  EditAccountCredentialContext,
  EditAccountPolicyContext,
} from '@/features/admin-accounts/presentation/accountEditorContext'
import { hydrateCNAccountEditor } from '@/features/admin-accounts/presentation/hydrateCNAccountEditor'

interface Props {
  show: boolean
  account: Account | null
  proxies: Proxy[]
  groups: AdminGroup[]
}

const props = defineProps<Props>()
const activeAccount = computed(() => props.account as Account)
const availableGroups = computed(() => props.groups)
const availableProxies = computed(() => props.proxies)
const emit = defineEmits<{
  close: []
  updated: [account: Account]
}>()

const { t } = useI18n()
const appStore = useAppStore()
const notifications = {
  showError: (message: string) => appStore.showError(message),
  showSuccess: (message: string) => appStore.showSuccess(message),
}
const authStore = useAuthStore()
const isSimpleMode = computed(() => authStore.isSimpleMode)

// Spark 影子账号(parent_account_id 非空):代理恒继承母账号,不可独立编辑(外审 B/P1),
// 故隐藏代理选择器。
const isSparkShadow = computed(() => props.account?.parent_account_id != null)

const handleOllamaCloudUsageUpdated = (state: OllamaCloudUsageState) => {
  if (props.account) emit('updated', { ...props.account, ollama_cloud_usage: state })
}

// Platform-specific hint for Base URL
const baseUrlHint = computed(() => {
  if (!props.account) return t('admin.accounts.baseUrlHint')
  if (props.account.platform === 'openai') return t('admin.accounts.openai.baseUrlHint')
  if (props.account.platform === 'gemini') return t('admin.accounts.gemini.baseUrlHint')
  if (props.account.platform === 'grok') return ''
  if (props.account.platform === 'multimodal') return t('admin.accounts.multimodal.baseUrlHint')
  if (isCNAccountPlatform(props.account.platform)) {
    return t('admin.accounts.cnProvider.baseUrlHint', {
      provider: CN_ACCOUNT_PLATFORM_LABELS[props.account.platform],
    })
  }
  return t('admin.accounts.baseUrlHint')
})

const antigravityPresetMappings = computed(() => getPresetMappingsByPlatform('antigravity'))
const bedrockPresets = computed(() => getPresetMappingsByPlatform('bedrock'))

// Model mapping type
// State
const submitting = ref(false)
const editBaseUrl = ref('https://api.anthropic.com')
const editApiKey = ref('')
const editCNAccountMode = ref<'payg' | 'coding'>('payg')
const editCNAPIProtocol = ref<'chat_completions' | 'adaptive' | 'anthropic' | 'responses'>('chat_completions')
const editOpenCodeAccountMode = ref<'zen' | 'go'>('go')
const editCNAdaptiveBaseURLs = ref<CNAdaptiveBaseURLs>(defaultCNAdaptiveBaseURLs('kimi', 'payg'))
const editOpenCodeProtocolRules = ref<OpenCodeProtocolRule[]>(defaultOpenCodeProtocolRules('go'))
const editZhipuOrganization = ref('')
const editZhipuProject = ref('')
// Bedrock credentials
const editBedrockAccessKeyId = ref('')
const editBedrockSecretAccessKey = ref('')
const editBedrockSessionToken = ref('')
const editBedrockRegion = ref('')
const editBedrockForceGlobal = ref(false)
const editBedrockApiKeyValue = ref('')
const editVertexProjectId = ref('')
const editVertexClientEmail = ref('')
const editVertexLocation = ref('us-central1')
const isBedrockAPIKeyMode = computed(() =>
  props.account?.type === 'bedrock' &&
  (props.account?.credentials as Record<string, unknown>)?.auth_mode === 'apikey'
)
const modelMappings = ref<ModelMapping[]>([])
const openAICompactModelMappings = ref<ModelMapping[]>([])
const modelRestrictionMode = ref<'whitelist' | 'mapping'>('whitelist')
const allowedModels = ref<string[]>([])
const CPA_SNAPSHOT_INTERVAL_SECONDS = 90
const MAX_CPA_CONCURRENCY_PER_CREDENTIAL = 10000
const GROK_CLIENT_TOOL_CACHE_EXTRA_KEY = 'grok_client_tool_cache_enabled'
const poolModeEnabled = ref(false)
const poolModeRetryCount = ref(DEFAULT_POOL_MODE_RETRY_COUNT)
const poolModeRetryStatusCodesInput = ref('')
const cpaModeEnabled = ref(false)
const cpaUseBaseUrl = ref(true)
const cpaManagementUrl = ref('')
const cpaManagementKey = ref('')
const cpaConcurrencyPerCredential = ref(10)
const cpaExcludeAbnormalCredentials = ref(false)
const { isTestingCPA, testCPAConnection } = useCPATestConnection({
  account: () => props.account, cpaConcurrencyPerCredential, cpaExcludeAbnormalCredentials,
  cpaManagementKey, cpaManagementUrl, cpaUseBaseUrl, editBaseUrl, notifications, t,
})

const customErrorCodesEnabled = ref(false)
const selectedErrorCodes = ref<number[]>([])
const customErrorCodeInput = ref<number | null>(null)
const headerOverrideEnabled = ref(false)
const headerOverrideRows = ref<HeaderOverrideRow[]>([])

const headerOverrideCapable = computed(
  () => !!props.account && isHeaderOverrideCapable(props.account.platform, props.account.type)
)

// Grok OAuth 自定义上游地址（仅转发端点；OAuth 授权/令牌刷新不受影响）
const grokOAuthCustomBaseUrlEnabled = ref(false)
const grokOAuthBaseUrl = ref('')
// Grok Free OAuth accounts use client-tool prompt caching by default. Keep an
// explicit false in the account extra as the opt-out signal.
const grokClientToolCacheEnabled = ref(true)

const interceptWarmupRequests = ref(false)
const autoPauseOnExpired = ref(false)
const autoPause5hThreshold = ref<number | null>(null)
const autoPause7dThreshold = ref<number | null>(null)
const autoPause5hDisabled = ref(false)
const autoPause7dDisabled = ref(false)
const upstreamBillingAutoProbeEnabled = ref(false)
const upstreamBillingRateSyncEnabled = ref(false)
const autoDisableOnUpstreamInsufficientBalance = ref(false)

watch(upstreamBillingRateSyncEnabled, (enabled) => {
  if (enabled) upstreamBillingAutoProbeEnabled.value = true
})
watch(upstreamBillingAutoProbeEnabled, (enabled) => {
  if (!enabled) upstreamBillingRateSyncEnabled.value = false
})
const mixedScheduling = ref(false) // For antigravity accounts: enable mixed scheduling
const allowOverages = ref(false) // For antigravity accounts: enable AI Credits overages
const antigravityProjectId = ref('')
const antigravityModelRestrictionMode = ref<'whitelist' | 'mapping'>('whitelist')
const antigravityWhitelistModels = ref<string[]>([])
const antigravityModelMappings = ref<ModelMapping[]>([])
const isSyncingAntigravityUpstream = ref(false)
const tempUnschedEnabled = ref(false)
const tempUnschedRules = ref<TempUnschedRuleForm[]>([])
const getModelMappingKey = createStableObjectKeyResolver<ModelMapping>('edit-model-mapping')
const getOpenAICompactModelMappingKey = createStableObjectKeyResolver<ModelMapping>('edit-openai-compact-model-mapping')
const getAntigravityModelMappingKey = createStableObjectKeyResolver<ModelMapping>('edit-antigravity-model-mapping')
const getTempUnschedRuleKey = createStableObjectKeyResolver<TempUnschedRuleForm>('edit-temp-unsched-rule')

const showMixedChannelWarning = ref(false)
const mixedChannelWarningDetails = ref<{ groupName: string; currentPlatform: string; otherPlatform: string } | null>(
  null
)
const mixedChannelWarningRawMessage = ref('')
const mixedChannelWarningAction = ref<(() => Promise<void>) | null>(null)
const antigravityMixedChannelConfirmed = ref(false)

// Quota control state (Anthropic OAuth/SetupToken only)
const windowCostEnabled = ref(false)
const windowCostLimit = ref<number | null>(null)
const windowCostStickyReserve = ref<number | null>(null)
const sessionLimitEnabled = ref(false)
const maxSessions = ref<number | null>(null)
const sessionIdleTimeout = ref<number | null>(null)
const rpmLimitEnabled = ref(false)
const baseRpm = ref<number | null>(null)
const rpmStrategy = ref<'tiered' | 'sticky_exempt'>('tiered')
const rpmStickyBuffer = ref<number | null>(null)
const userMsgQueueMode = ref('')
const umqModeOptions = computed(() => [
  { value: '', label: t('admin.accounts.quotaControl.rpmLimit.umqModeOff') },
  { value: 'throttle', label: t('admin.accounts.quotaControl.rpmLimit.umqModeThrottle') },
  { value: 'serialize', label: t('admin.accounts.quotaControl.rpmLimit.umqModeSerialize') },
])
const tlsFingerprintEnabled = ref(false)
const tlsFingerprintProfileId = ref<number | null>(null)
const tlsFingerprintProfiles = ref<{ id: number; name: string }[]>([])
const sessionIdMaskingEnabled = ref(false)
const cacheTTLOverrideEnabled = ref(false)
const cacheTTLOverrideTarget = ref<string>('5m')
const customBaseUrlEnabled = ref(false)
const customBaseUrl = ref('')

// OpenAI 自动透传开关（OAuth/API Key）
const openaiPassthroughEnabled = ref(false)
const openAIImageSizeTiers = ref<OpenAIImageSizeTier[]>([...OPENAI_IMAGE_SIZE_TIERS])
// OAuth-only compatibility switch; default preserves namespace declarations.
const openaiFlattenNamespacesEnabled = ref(false)
const openAILongContextBillingEnabled = ref(false)
// OpenAI 订阅档位（Plus/Pro/Free）手动覆盖值,存于 credentials.plan_type;'' 表示清空/自动识别
const editPlanType = ref<string>('')
const openAICompactMode = ref<OpenAICompactMode>('auto')
const excelBpsEnabled = ref(false)
const openAIResponsesMode = ref<OpenAIResponsesMode>('auto')
const openAIEndpointCapabilities = ref<OpenAIEndpointCapability[]>(['chat_completions', 'embeddings', 'decisions'])
const openAIForceImageAPIEnabled = ref(false)
const codexWebSearchEnabled = ref(true)
const openaiOAuthResponsesWebSocketV2Mode = ref<OpenAIWSMode>(OPENAI_WS_MODE_OFF)
const openaiAPIKeyResponsesWebSocketV2Mode = ref<OpenAIWSMode>(OPENAI_WS_MODE_OFF)
const codexFingerprintMode = ref<CodexFingerprintMode>('off')
const codexPrewarmContinuationEnabled = ref(false)
const codexThinkingTagNormalizationEnabled = ref(false)
const codexCLIOnlyEnabled = ref(false)
const codexCLIOnlyAppServerEnabled = ref(false)
type CodexImageToolMode = 'inherit' | 'enabled' | 'disabled' | 'block'
const codexImageToolMode = ref<CodexImageToolMode>('inherit')
type AnthropicAPIKeyAuthScheme = 'x_api_key' | 'authorization_bearer'
const anthropicPassthroughEnabled = ref(false)
const anthropicAPIKeyAuthScheme = ref<AnthropicAPIKeyAuthScheme>('x_api_key')
const webSearchEmulationMode = ref('default')
const webSearchGlobalEnabled = ref(false)
const {
  globalEnabled: quotaNotifyGlobalEnabled,
  state: quotaNotifyState,
  loadGlobalState: loadQuotaNotifyGlobal,
  loadFromExtra: loadQuotaNotifyFromExtra,
  writeToExtra: writeQuotaNotifyToExtra,
  reset: resetQuotaNotify,
} = useQuotaNotifyState()

// Load global feature states once
getWebSearchEmulationConfig().then(cfg => {
  webSearchGlobalEnabled.value = cfg?.enabled === true && (cfg?.providers?.length ?? 0) > 0
}).catch(() => { webSearchGlobalEnabled.value = false })

loadQuotaNotifyGlobal()
const editQuotaLimit = ref<number | null>(null)
const editQuotaDailyLimit = ref<number | null>(null)
const editQuotaWeeklyLimit = ref<number | null>(null)
const editDailyResetMode = ref<'rolling' | 'fixed' | null>(null)
const editDailyResetHour = ref<number | null>(null)
const editWeeklyResetMode = ref<'rolling' | 'fixed' | null>(null)
const editWeeklyResetDay = ref<number | null>(null)
const editWeeklyResetHour = ref<number | null>(null)
const editResetTimezone = ref<string | null>(null)
const openAIWSModeOptions = computed<Array<{ value: OpenAIWSMode; label: string }>>(() => [
  { value: OPENAI_WS_MODE_OFF, label: t('admin.accounts.openai.wsModeOff') },
  { value: OPENAI_WS_MODE_CTX_POOL, label: t('admin.accounts.openai.wsModeCtxPool') },
  { value: OPENAI_WS_MODE_PASSTHROUGH, label: t('admin.accounts.openai.wsModePassthrough') },
  { value: OPENAI_WS_MODE_HTTP_BRIDGE, label: t('admin.accounts.openai.wsModeHttpBridge') }
])
const codexFingerprintModeOptions = computed(() => getCodexFingerprintModeOptions(t))
const openaiResponsesWebSocketV2Mode = computed({
  get: () => {
    if (props.account?.type === 'apikey') {
      return openaiAPIKeyResponsesWebSocketV2Mode.value
    }
    return openaiOAuthResponsesWebSocketV2Mode.value
  },
  set: (mode: OpenAIWSMode) => {
    if (props.account?.type === 'apikey') {
      openaiAPIKeyResponsesWebSocketV2Mode.value = mode
      return
    }
    openaiOAuthResponsesWebSocketV2Mode.value = mode
  }
})
const openAIWSModeConcurrencyHintKey = computed(() =>
  resolveOpenAIWSModeConcurrencyHintKey(openaiResponsesWebSocketV2Mode.value)
)
const codexImageToolOptions = computed<Array<{
  value: CodexImageToolMode
  label: string
  description: string
  selectedCardClass: string
  selectedDotClass: string
}>>(() => [
  {
    value: 'inherit',
    label: t('admin.accounts.openai.codexImageToolInherit'),
    description: t('admin.accounts.openai.codexImageToolInheritDesc'),
    selectedCardClass: 'border-sky-300 bg-sky-50 text-sky-900 shadow-sm ring-1 ring-sky-200 dark:border-sky-700 dark:bg-sky-900/25 dark:text-sky-100 dark:ring-sky-800',
    selectedDotClass: 'border-sky-500 bg-sky-500 text-white'
  },
  {
    value: 'enabled',
    label: t('admin.accounts.openai.codexImageToolEnabled'),
    description: t('admin.accounts.openai.codexImageToolEnabledDesc'),
    selectedCardClass: 'border-emerald-300 bg-emerald-50 text-emerald-900 shadow-sm ring-1 ring-emerald-200 dark:border-emerald-700 dark:bg-emerald-900/25 dark:text-emerald-100 dark:ring-emerald-800',
    selectedDotClass: 'border-emerald-500 bg-emerald-500 text-white'
  },
  {
    value: 'disabled',
    label: t('admin.accounts.openai.codexImageToolDisabled'),
    description: t('admin.accounts.openai.codexImageToolDisabledDesc'),
    selectedCardClass: 'border-amber-300 bg-amber-50 text-amber-900 shadow-sm ring-1 ring-amber-200 dark:border-amber-700 dark:bg-amber-900/25 dark:text-amber-100 dark:ring-amber-800',
    selectedDotClass: 'border-amber-500 bg-amber-500 text-white'
  },
  {
    value: 'block',
    label: t('admin.accounts.openai.codexImageToolBlock'),
    description: t('admin.accounts.openai.codexImageToolBlockDesc'),
    selectedCardClass: 'border-rose-300 bg-rose-50 text-rose-900 shadow-sm ring-1 ring-rose-200 dark:border-rose-700 dark:bg-rose-900/25 dark:text-rose-100 dark:ring-rose-800',
    selectedDotClass: 'border-rose-500 bg-rose-500 text-white'
  }
])
const codexImageToolBadgeLabel = computed(() => {
  switch (codexImageToolMode.value) {
    case 'enabled':
      return t('admin.accounts.openai.codexImageToolBadgeEnabled')
    case 'disabled':
      return t('admin.accounts.openai.codexImageToolBadgeDisabled')
    case 'block':
      return t('admin.accounts.openai.codexImageToolBadgeBlock')
    default:
      return t('admin.accounts.openai.codexImageToolBadgeInherit')
  }
})
const codexImageToolBadgeClass = computed(() => {
  switch (codexImageToolMode.value) {
    case 'enabled':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
    case 'disabled':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300'
    case 'block':
      return 'bg-rose-100 text-rose-700 dark:bg-rose-900/40 dark:text-rose-300'
    default:
      return 'bg-slate-100 text-slate-600 dark:bg-dark-600 dark:text-slate-300'
  }
})
const openAICompactModeOptions = computed<Array<{ value: OpenAICompactMode; label: string }>>(() => [
  { value: 'auto', label: t('admin.accounts.openai.compactModeAuto') },
  { value: 'force_on', label: t('admin.accounts.openai.compactModeForceOn') },
  { value: 'force_off', label: t('admin.accounts.openai.compactModeForceOff') }
])
// OpenAI 订阅档位手动覆盖选项(清空 + Plus/Pro/Free;别名/自定义值友好显示且保留 canonical)
const planTypeOptions = computed(() =>
  buildPlanTypeOptions(editPlanType.value, t('admin.accounts.openai.planTypeClear'))
)
const openAIResponsesModeOptions = computed<Array<{ value: OpenAIResponsesMode; label: string }>>(() => [
  { value: 'auto', label: t('admin.accounts.openai.responsesModeAuto') },
  { value: 'force_responses', label: t('admin.accounts.openai.responsesModeForceResponses') },
  { value: 'force_chat_completions', label: t('admin.accounts.openai.responsesModeForceChatCompletions') }
])
const openAITextEndpointCapabilityLabel = computed(() => {
  if (openAIResponsesMode.value === 'force_responses') {
    return t('admin.accounts.openai.capabilityResponses')
  }
  if (openAIResponsesMode.value === 'force_chat_completions') {
    return t('admin.accounts.openai.capabilityChatCompletions')
  }
  const extra = props.account?.extra as Record<string, unknown> | undefined
  if (extra?.openai_responses_supported === true) {
    return t('admin.accounts.openai.capabilityResponsesAuto')
  }
  if (extra?.openai_responses_supported === false) {
    return t('admin.accounts.openai.capabilityChatCompletionsAuto')
  }
  return t('admin.accounts.openai.capabilityTextAuto')
})
const openAIEndpointCapabilityOptions = computed<{ value: OpenAIEndpointCapability; label: string }[]>(() => [
  { value: 'chat_completions', label: openAITextEndpointCapabilityLabel.value },
  { value: 'embeddings', label: t('admin.accounts.openai.capabilityEmbeddings') },
  { value: 'decisions', label: t('admin.accounts.openai.capabilityDecisions') }
])
const openAITextGenerationCapabilityEnabled = computed(() =>
  openAIEndpointCapabilities.value.includes('chat_completions')
)

const isOpenAIPersonalAccessTokenAccount = computed(() =>
  props.account?.platform === 'openai' &&
  props.account?.type === 'oauth' &&
  isOpenAIPersonalAccessTokenCredentials(props.account.credentials as Record<string, unknown> | undefined)
)

const applyCodexWebSearchCapability = (credentials: Record<string, unknown>) => {
  if (codexWebSearchEnabled.value) {
    delete credentials.openai_capabilities
    return
  }
  credentials.openai_capabilities = ['chat_completions']
}

const toggleOpenAIEndpointCapability = (capability: OpenAIEndpointCapability, event?: Event) => {
  if (openAIEndpointCapabilities.value.includes(capability)) {
    if (openAIEndpointCapabilities.value.length <= 1) {
      const input = event?.target as HTMLInputElement | null
      if (input) input.checked = true
      return
    }
    openAIEndpointCapabilities.value = openAIEndpointCapabilities.value.filter(
      (value) => value !== capability
    )
    if (!openAITextGenerationCapabilityEnabled.value) {
      openAIResponsesMode.value = 'auto'
    }
    return
  }
  openAIEndpointCapabilities.value = normalizeOpenAIEndpointCapabilities([
    ...openAIEndpointCapabilities.value,
    capability
  ])
}

const applyOpenAIEndpointCapabilities = (credentials: Record<string, unknown>) => {
  const capabilities = normalizeOpenAIEndpointCapabilities(openAIEndpointCapabilities.value)
  if (capabilities.length === 3) {
    delete credentials.openai_capabilities
    return
  }
  credentials.openai_capabilities = capabilities
}
const normalizeOpenAIResponsesMode = (mode: unknown): OpenAIResponsesMode => {
  if (mode === 'force_responses' || mode === 'force_chat_completions') {
    return mode
  }
  return 'auto'
}
const isOpenAIModelRestrictionDisabled = computed(() =>
  props.account?.platform === 'openai' && openaiPassthroughEnabled.value
)
const openAIResponsesStatusKey = computed(() => {
  if (openAIResponsesMode.value === 'force_responses') {
    return 'admin.accounts.openai.responsesStatusForcedResponses'
  }
  if (openAIResponsesMode.value === 'force_chat_completions') {
    return 'admin.accounts.openai.responsesStatusForcedChatCompletions'
  }
  const extra = props.account?.extra as Record<string, unknown> | undefined
  if (extra?.openai_responses_supported === true) {
    return 'admin.accounts.openai.responsesStatusAutoSupported'
  }
  if (extra?.openai_responses_supported === false) {
    return 'admin.accounts.openai.responsesStatusAutoUnsupported'
  }
  return 'admin.accounts.openai.responsesStatusAutoUnknown'
})
const openAICompactStatusKey = computed(() => {
  const extra = props.account?.extra as Record<string, unknown> | undefined
  if (!props.account || props.account.platform !== 'openai') return ''
  const mode = typeof extra?.openai_compact_mode === 'string' ? extra.openai_compact_mode : 'auto'
  if (mode === 'force_on') return 'admin.accounts.openai.compactSupported'
  if (mode === 'force_off') return 'admin.accounts.openai.compactUnsupported'
  if (typeof extra?.openai_compact_supported === 'boolean') {
    return extra.openai_compact_supported
      ? 'admin.accounts.openai.compactSupported'
      : 'admin.accounts.openai.compactUnsupported'
  }
  return 'admin.accounts.openai.compactAuto'
})

// Computed: current preset mappings based on platform
const presetMappings = computed(() => getPresetMappingsByPlatform(props.account?.platform || 'anthropic'))
const tempUnschedPresets = computed(() => [
  {
    label: t('admin.accounts.tempUnschedulable.presets.overloadLabel'),
    rule: {
      error_code: 529,
      keywords: 'overloaded, too many',
      duration_minutes: 60,
      description: t('admin.accounts.tempUnschedulable.presets.overloadDesc')
    }
  },
  {
    label: t('admin.accounts.tempUnschedulable.presets.rateLimitLabel'),
    rule: {
      error_code: 429,
      keywords: 'rate limit, too many requests',
      duration_minutes: 10,
      description: t('admin.accounts.tempUnschedulable.presets.rateLimitDesc')
    }
  },
  {
    label: t('admin.accounts.tempUnschedulable.presets.unavailableLabel'),
    rule: {
      error_code: 503,
      keywords: 'unavailable, maintenance',
      duration_minutes: 30,
      description: t('admin.accounts.tempUnschedulable.presets.unavailableDesc')
    }
  }
])

// Computed: default base URL based on platform
const defaultBaseUrl = computed(() => {
    return props.account ? defaultAPIKeyBaseURL(props.account.platform) : 'https://api.anthropic.com'
})

const mixedChannelWarningMessageText = computed(() => {
  if (mixedChannelWarningDetails.value) {
    return t('admin.accounts.mixedChannelWarning', mixedChannelWarningDetails.value)
  }
  return mixedChannelWarningRawMessage.value
})

const form = reactive({
  name: '',
  notes: '',
  proxy_id: null as number | null,
  concurrency: 1,
  load_factor: null as number | null,
  priority: 1,
  rate_multiplier: 1,
  status: 'active' as 'active' | 'inactive' | 'error',
  group_ids: [] as number[],
  expires_at: null as number | null
})

const statusOptions = computed<Array<{ value: Account['status']; label: string }>>(() => {
  const options: Array<{ value: Account['status']; label: string }> = [
    { value: 'active', label: t('common.active') },
    { value: 'inactive', label: t('common.inactive') }
  ]
  if (form.status === 'error') {
    options.push({ value: 'error', label: t('admin.accounts.status.error') })
  }
  return options
})

const expiresAtInput = computed({
  get: () => formatDateTimeLocalInput(form.expires_at),
  set: (value: string) => {
    form.expires_at = parseDateTimeLocalInput(value)
  }
})

// Watchers
const loadModelRestrictionFromMapping = (rawMapping?: Record<string, unknown>) => {
  const parsed = splitModelMappingObject(rawMapping)
  allowedModels.value = parsed.allowedModels
  modelMappings.value = parsed.modelMappings
  modelRestrictionMode.value =
    parsed.modelMappings.length > 0 && parsed.allowedModels.length === 0
      ? 'mapping'
      : 'whitelist'
}

const buildModelRestrictionMapping = () =>
  buildModelMappingObject('combined', allowedModels.value, modelMappings.value)

const applyOpenAIModelMappingCredentials = (credentials: Record<string, unknown>) => {
  const shouldApplyModelMapping = !openaiPassthroughEnabled.value

  if (shouldApplyModelMapping) {
    const modelMapping = buildModelRestrictionMapping()
    if (modelMapping) {
      credentials.model_mapping = modelMapping
    } else {
      delete credentials.model_mapping
    }
  } else if (!credentials.model_mapping) {
    delete credentials.model_mapping
  }

  const compactModelMapping = buildModelMappingObject('mapping', [], openAICompactModelMappings.value)
  if (compactModelMapping) {
    credentials.compact_model_mapping = compactModelMapping
  } else {
    delete credentials.compact_model_mapping
  }
}

const syncFormFromAccount = (newAccount: Account | null) => {
  if (!newAccount) {
    return
  }
  antigravityMixedChannelConfirmed.value = false
  showMixedChannelWarning.value = false
  mixedChannelWarningDetails.value = null
  mixedChannelWarningRawMessage.value = ''
  mixedChannelWarningAction.value = null
  form.name = newAccount.name
  form.notes = newAccount.notes || ''
  form.proxy_id = newAccount.proxy_id
  form.concurrency = newAccount.concurrency
  form.load_factor = newAccount.load_factor ?? null
  form.priority = newAccount.priority
  form.rate_multiplier = newAccount.rate_multiplier ?? 1
  form.status = (newAccount.status === 'active' || newAccount.status === 'inactive' || newAccount.status === 'error')
    ? newAccount.status
    : 'active'
  form.group_ids = newAccount.group_ids || []
  form.expires_at = newAccount.expires_at ?? null

  // Load intercept warmup requests setting (applies to all account types)
  const credentials = newAccount.credentials as Record<string, unknown> | undefined
  interceptWarmupRequests.value = credentials?.intercept_warmup_requests === true
  autoPauseOnExpired.value = newAccount.auto_pause_on_expired === true
  editVertexProjectId.value = ''
  editVertexClientEmail.value = ''
  editVertexLocation.value = 'us-central1'
  editZhipuOrganization.value = ''
  editZhipuProject.value = ''
  editOpenCodeProtocolRules.value = defaultOpenCodeProtocolRules('go')
  antigravityProjectId.value =
    newAccount.platform === 'antigravity' &&
    newAccount.type === 'oauth' &&
    typeof credentials?.antigravity_project_id === 'string'
      ? credentials.antigravity_project_id.trim()
      : ''

  // Load mixed scheduling setting (only for antigravity accounts)
  mixedScheduling.value = false
  allowOverages.value = false
	const extra = newAccount.extra as Record<string, unknown> | undefined
	excelBpsEnabled.value = false
	mixedScheduling.value = extra?.mixed_scheduling === true
	allowOverages.value = extra?.allow_overages === true
	autoPause5hThreshold.value = typeof extra?.auto_pause_5h_threshold === 'number' ? extra.auto_pause_5h_threshold * 100 : null
	autoPause7dThreshold.value = typeof extra?.auto_pause_7d_threshold === 'number' ? extra.auto_pause_7d_threshold * 100 : null
	autoPause5hDisabled.value = extra?.auto_pause_5h_disabled === true
	autoPause7dDisabled.value = extra?.auto_pause_7d_disabled === true
	upstreamBillingAutoProbeEnabled.value = extra?.upstream_billing_probe_enabled === true
	upstreamBillingRateSyncEnabled.value =
		upstreamBillingAutoProbeEnabled.value &&
		extra?.upstream_billing_rate_sync_enabled === true
	autoDisableOnUpstreamInsufficientBalance.value = extra?.auto_disable_on_upstream_insufficient_balance === true

  // Load OpenAI passthrough toggle (OpenAI OAuth/SetupToken/API Key)
  openaiPassthroughEnabled.value = false
  openAIImageSizeTiers.value = readOpenAIImageSizeTiers(extra)
  openaiFlattenNamespacesEnabled.value = false
  openAILongContextBillingEnabled.value = false
  editPlanType.value = ''
  openAICompactMode.value = 'auto'
  openAIResponsesMode.value = 'auto'
  openAIEndpointCapabilities.value = ['chat_completions', 'embeddings', 'decisions']
  openAIForceImageAPIEnabled.value = false
  codexWebSearchEnabled.value = true
  openAICompactModelMappings.value = []
  openaiOAuthResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
  openaiAPIKeyResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
  codexFingerprintMode.value = 'off'
  codexPrewarmContinuationEnabled.value = codexCLIOnlyEnabled.value = codexCLIOnlyAppServerEnabled.value = false
  codexThinkingTagNormalizationEnabled.value = false
  codexImageToolMode.value = 'inherit'
  anthropicPassthroughEnabled.value = false
  anthropicAPIKeyAuthScheme.value = 'x_api_key'
  webSearchEmulationMode.value = 'default'
  if (newAccount.platform === 'openai' && (newAccount.type === 'oauth' || newAccount.type === 'setup-token' || newAccount.type === 'apikey')) {
    openaiPassthroughEnabled.value = extra?.openai_passthrough === true || extra?.openai_oauth_passthrough === true
    openaiFlattenNamespacesEnabled.value =
      newAccount.type === 'oauth' && extra?.openai_responses_flatten_namespaces === true
    const longContextBillingValue = extra?.openai_long_context_billing_enabled
    openAILongContextBillingEnabled.value = longContextBillingValue === true
    // plan_type 手动覆盖仅 OAuth 有实际调度语义(IsOpenAIChatGPTSubscription 要求 oauth),故只对 oauth 回填
    editPlanType.value = newAccount.type === 'oauth'
      ? readPlanType(newAccount.credentials as Record<string, unknown> | undefined)
      : ''
    openAICompactMode.value = (extra?.openai_compact_mode as OpenAICompactMode) || 'auto'
    if (newAccount.type === 'apikey') {
      openAIResponsesMode.value = normalizeOpenAIResponsesMode(extra?.openai_responses_mode)
      openAIForceImageAPIEnabled.value = extra?.openai_force_image_api === true
      openAIEndpointCapabilities.value = readOpenAIEndpointCapabilities(
        newAccount.credentials as Record<string, unknown> | undefined
      )
      if (!openAITextGenerationCapabilityEnabled.value) {
        openAIResponsesMode.value = 'auto'
      }
    }
    const codexImageGenerationBridgeValue = typeof extra?.codex_image_generation_bridge === 'boolean'
      ? extra.codex_image_generation_bridge
      : extra?.codex_image_generation_bridge_enabled
    if (extra?.codex_image_generation_explicit_tool_policy === 'strip') {
      codexImageToolMode.value = 'block'
    } else if (codexImageGenerationBridgeValue === true) {
      codexImageToolMode.value = 'enabled'
    } else if (codexImageGenerationBridgeValue === false) {
      codexImageToolMode.value = 'disabled'
    }
    openaiOAuthResponsesWebSocketV2Mode.value = resolveOpenAIWSModeFromExtra(extra, {
      modeKey: 'openai_oauth_responses_websockets_v2_mode',
      enabledKey: 'openai_oauth_responses_websockets_v2_enabled',
      fallbackEnabledKeys: ['responses_websockets_v2_enabled', 'openai_ws_enabled'],
      defaultMode: OPENAI_WS_MODE_OFF
    })
    openaiAPIKeyResponsesWebSocketV2Mode.value = resolveOpenAIWSModeFromExtra(extra, {
      modeKey: 'openai_apikey_responses_websockets_v2_mode',
      enabledKey: 'openai_apikey_responses_websockets_v2_enabled',
      fallbackEnabledKeys: ['responses_websockets_v2_enabled', 'openai_ws_enabled'],
      defaultMode: OPENAI_WS_MODE_OFF
    })
    codexFingerprintMode.value = newAccount.type === 'oauth'
      ? normalizeCodexFingerprintMode(extra?.codex_fingerprint_mode)
      : 'off'
    codexPrewarmContinuationEnabled.value = newAccount.type === 'oauth' && extra?.codex_prewarm_continuation_enabled === true
    codexThinkingTagNormalizationEnabled.value = newAccount.type === 'apikey' && extra?.codex_thinking_tag_normalization_enabled === true
    if (newAccount.type === 'oauth' || newAccount.type === 'setup-token') {
      codexCLIOnlyEnabled.value = extra?.codex_cli_only === true
      codexCLIOnlyAppServerEnabled.value =
        extra?.codex_cli_only_allow_app_server === true
    }
    const credentials = newAccount.credentials as Record<string, unknown> | undefined
    const excelBpsEligible =
      newAccount.type === 'oauth' &&
      newAccount.parent_account_id == null &&
      !isOpenAIPersonalAccessTokenCredentials(credentials) &&
      credentials?.auth_mode !== 'agentIdentity'
    excelBpsEnabled.value = excelBpsEligible && (
      typeof extra?.excel_bps_enabled === 'boolean'
        ? extra.excel_bps_enabled
        : extra?.openai_excel_bps === true
    )
    if (newAccount.type === 'oauth' && isOpenAIPersonalAccessTokenCredentials(credentials)) {
      codexWebSearchEnabled.value = readCodexWebSearchEnabled(credentials)
    }
    const compactMappings = credentials?.compact_model_mapping as Record<string, string> | undefined
    if (compactMappings && typeof compactMappings === 'object') {
      openAICompactModelMappings.value = Object.entries(compactMappings).map(([from, to]) => ({ from, to }))
    }
  }
  if (newAccount.platform === 'anthropic' && newAccount.type === 'apikey') {
    anthropicPassthroughEnabled.value = extra?.anthropic_passthrough === true
    anthropicAPIKeyAuthScheme.value = extra?.anthropic_apikey_auth_scheme === 'authorization_bearer'
      ? 'authorization_bearer'
      : 'x_api_key'
  }
  if ((newAccount.platform === 'anthropic' || newAccount.platform === 'openai') && newAccount.type === 'apikey') {
    // 三态：string "default"/"enabled"/"disabled"，向后兼容旧 bool
    const wsVal = extra?.web_search_emulation
    if (wsVal === 'enabled' || wsVal === 'disabled') {
      webSearchEmulationMode.value = wsVal
    } else if (wsVal === true) {
      webSearchEmulationMode.value = 'enabled'
    } else {
      webSearchEmulationMode.value = 'default'
    }
  }

  // Load quota limit for apikey/bedrock accounts (bedrock quota is also loaded in its own branch above)
  if (supportsAccountQuota(newAccount.type)) {
    const quotaVal = extra?.quota_limit as number | undefined
    editQuotaLimit.value = (quotaVal && quotaVal > 0) ? quotaVal : null
    const dailyVal = extra?.quota_daily_limit as number | undefined
    editQuotaDailyLimit.value = (dailyVal && dailyVal > 0) ? dailyVal : null
    const weeklyVal = extra?.quota_weekly_limit as number | undefined
    editQuotaWeeklyLimit.value = (weeklyVal && weeklyVal > 0) ? weeklyVal : null
    // Load quota reset mode config
    editDailyResetMode.value = (extra?.quota_daily_reset_mode as 'rolling' | 'fixed') || null
    editDailyResetHour.value = (extra?.quota_daily_reset_hour as number) ?? null
    editWeeklyResetMode.value = (extra?.quota_weekly_reset_mode as 'rolling' | 'fixed') || null
    editWeeklyResetDay.value = (extra?.quota_weekly_reset_day as number) ?? null
    editWeeklyResetHour.value = (extra?.quota_weekly_reset_hour as number) ?? null
    editResetTimezone.value = (extra?.quota_reset_timezone as string) || null
    // Load quota notify config
    loadQuotaNotifyFromExtra(extra)
  } else {
    editQuotaLimit.value = null
    editQuotaDailyLimit.value = null
    editQuotaWeeklyLimit.value = null
    editDailyResetMode.value = null
    editDailyResetHour.value = null
    editWeeklyResetMode.value = null
    editWeeklyResetDay.value = null
    editWeeklyResetHour.value = null
    editResetTimezone.value = null
    resetQuotaNotify()
  }

  // Load antigravity model mapping (Antigravity 只支持映射模式)
  if (newAccount.platform === 'antigravity') {
    const credentials = newAccount.credentials as Record<string, unknown> | undefined

    // Antigravity 始终使用映射模式
    antigravityModelRestrictionMode.value = 'mapping'
    antigravityWhitelistModels.value = []

    // 从 model_mapping 读取映射配置
    const rawAgMapping = credentials?.model_mapping as Record<string, string> | undefined
    if (rawAgMapping && typeof rawAgMapping === 'object') {
      const entries = Object.entries(rawAgMapping)
      // 无论是白名单样式(key===value)还是真正的映射，都统一转换为映射列表
      antigravityModelMappings.value = entries.map(([from, to]) => ({ from, to }))
    } else {
      // 兼容旧数据：从 model_whitelist 读取，转换为映射格式
      const rawWhitelist = credentials?.model_whitelist
      if (Array.isArray(rawWhitelist) && rawWhitelist.length > 0) {
        antigravityModelMappings.value = rawWhitelist
          .map((v) => String(v).trim())
          .filter((v) => v.length > 0)
          .map((m) => ({ from: m, to: m }))
      } else {
        antigravityModelMappings.value = []
      }
    }
  } else {
    antigravityModelRestrictionMode.value = 'mapping'
    antigravityWhitelistModels.value = []
    antigravityModelMappings.value = []
  }

  // Load quota control settings (Anthropic OAuth/SetupToken only)
  loadQuotaControlSettings(newAccount)

  loadTempUnschedRules(credentials)

  // Load header override state (anthropic/openai apikey + grok apikey/oauth)
  headerOverrideEnabled.value = false
  headerOverrideRows.value = []
  if (newAccount.credentials && isHeaderOverrideCapable(newAccount.platform, newAccount.type)) {
    const overrideCreds = newAccount.credentials as Record<string, unknown>
    headerOverrideEnabled.value = overrideCreds[HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY] === true
    headerOverrideRows.value = splitHeaderOverridesObject(
      overrideCreds[HEADER_OVERRIDES_CREDENTIAL_KEY]
    )
  }

  // Load Grok OAuth custom upstream URL state（存储的官方地址视同未定制）
  grokOAuthCustomBaseUrlEnabled.value = false
  grokOAuthBaseUrl.value = ''
  const grokClientToolCacheSetting =
    newAccount.platform === 'grok' && newAccount.type === 'oauth'
      ? newAccount.extra?.[GROK_CLIENT_TOOL_CACHE_EXTRA_KEY]
      : undefined
  grokClientToolCacheEnabled.value =
    newAccount.platform === 'grok' &&
    newAccount.type === 'oauth' &&
    (grokClientToolCacheSetting === undefined || grokClientToolCacheSetting === true)
  if (newAccount.platform === 'grok' && newAccount.type === 'oauth' && newAccount.credentials) {
    const grokCreds = newAccount.credentials as Record<string, unknown>
    if (isCustomGrokBaseUrl(grokCreds.base_url)) {
      grokOAuthCustomBaseUrlEnabled.value = true
      grokOAuthBaseUrl.value = (grokCreds.base_url as string).trim()
    }
  }

  // Initialize API Key fields for apikey type
  if (newAccount.type === 'apikey' && newAccount.credentials) {
    const credentials = newAccount.credentials as Record<string, unknown>
    const platformDefaultUrl = defaultAPIKeyBaseURL(newAccount.platform)
    const cnState = hydrateCNAccountEditor(newAccount.platform, credentials)
    if (cnState) {
      editCNAccountMode.value = cnState.accountMode
      editOpenCodeAccountMode.value = cnState.openCodeAccountMode
      editCNAPIProtocol.value = cnState.protocol
      editCNAdaptiveBaseURLs.value = cnState.adaptiveBaseURLs
      editOpenCodeProtocolRules.value = cnState.openCodeProtocolRules
      editZhipuOrganization.value = cnState.zhipuOrganization
      editZhipuProject.value = cnState.zhipuProject
      editBaseUrl.value = cnState.baseURL
    } else {
      editBaseUrl.value = (credentials.base_url as string) || platformDefaultUrl
    }

    // Load model mappings and detect mode
    loadModelRestrictionFromMapping(credentials.model_mapping as Record<string, unknown> | undefined)

    // Load pool mode
    poolModeEnabled.value = credentials.pool_mode === true
    poolModeRetryCount.value = normalizePoolModeRetryCount(
      Number(credentials.pool_mode_retry_count ?? DEFAULT_POOL_MODE_RETRY_COUNT)
    )
    poolModeRetryStatusCodesInput.value = formatPoolModeRetryStatusCodes(credentials.pool_mode_retry_status_codes)

    cpaModeEnabled.value = credentials.cpa_mode === true
    cpaUseBaseUrl.value = typeof credentials.cpa_management_url !== 'string' || !credentials.cpa_management_url.trim()
    cpaManagementUrl.value = cpaUseBaseUrl.value ? '' : credentials.cpa_management_url as string
    const storedCPAConcurrency = Number(credentials.cpa_concurrency_per_credential ?? 10)
    cpaConcurrencyPerCredential.value = Number.isInteger(storedCPAConcurrency) && storedCPAConcurrency > 0
      ? Math.min(storedCPAConcurrency, MAX_CPA_CONCURRENCY_PER_CREDENTIAL)
      : 10
    cpaExcludeAbnormalCredentials.value = credentials.cpa_exclude_abnormal_credentials === true
    cpaManagementKey.value = ''

    // Load custom error codes
    customErrorCodesEnabled.value = credentials.custom_error_codes_enabled === true
    const existingErrorCodes = credentials.custom_error_codes as number[] | undefined
    if (existingErrorCodes && Array.isArray(existingErrorCodes)) {
      selectedErrorCodes.value = [...existingErrorCodes]
    } else {
      selectedErrorCodes.value = []
    }

  } else if (newAccount.type === 'bedrock' && newAccount.credentials) {
    const bedrockCreds = newAccount.credentials as Record<string, unknown>
    const authMode = (bedrockCreds.auth_mode as string) || 'sigv4'
    editBedrockRegion.value = (bedrockCreds.aws_region as string) || ''
    editBedrockForceGlobal.value = (bedrockCreds.aws_force_global as string) === 'true'

    if (authMode === 'apikey') {
      editBedrockApiKeyValue.value = ''
    } else {
      editBedrockAccessKeyId.value = (bedrockCreds.aws_access_key_id as string) || ''
      editBedrockSecretAccessKey.value = ''
      editBedrockSessionToken.value = ''
    }

    // Load pool mode for bedrock
    poolModeEnabled.value = bedrockCreds.pool_mode === true
    const retryCount = bedrockCreds.pool_mode_retry_count
    poolModeRetryCount.value = (typeof retryCount === 'number' && retryCount >= 0) ? retryCount : DEFAULT_POOL_MODE_RETRY_COUNT
    poolModeRetryStatusCodesInput.value = formatPoolModeRetryStatusCodes(bedrockCreds.pool_mode_retry_status_codes)

    // Load quota limits for bedrock
    const bedrockExtra = (newAccount.extra as Record<string, unknown>) || {}
    editQuotaLimit.value = typeof bedrockExtra.quota_limit === 'number' ? bedrockExtra.quota_limit : null
    editQuotaDailyLimit.value = typeof bedrockExtra.quota_daily_limit === 'number' ? bedrockExtra.quota_daily_limit : null
    editQuotaWeeklyLimit.value = typeof bedrockExtra.quota_weekly_limit === 'number' ? bedrockExtra.quota_weekly_limit : null
    // Load quota notify for bedrock
    loadQuotaNotifyFromExtra(bedrockExtra)

    // Load model mappings for bedrock
    loadModelRestrictionFromMapping(bedrockCreds.model_mapping as Record<string, unknown> | undefined)
  } else if (newAccount.type === 'upstream' && newAccount.credentials) {
    const credentials = newAccount.credentials as Record<string, unknown>
    editBaseUrl.value = (credentials.base_url as string) || ''
  } else if ((newAccount.platform === 'gemini' || newAccount.platform === 'anthropic') && newAccount.type === 'service_account' && newAccount.credentials) {
    const credentials = newAccount.credentials as Record<string, unknown>
    editVertexProjectId.value = (credentials.project_id as string) || ''
    editVertexClientEmail.value = (credentials.client_email as string) || ''
    editVertexLocation.value = (credentials.location as string) || (credentials.vertex_location as string) || 'us-central1'

    // Load model mappings for service_account
    loadModelRestrictionFromMapping(credentials.model_mapping as Record<string, unknown> | undefined)
  } else {
      const platformDefaultUrl = defaultAPIKeyBaseURL(newAccount.platform)
    editBaseUrl.value = platformDefaultUrl

    // Load model mappings for OpenAI/Grok OAuth accounts
    if ((newAccount.platform === 'openai' || newAccount.platform === 'grok') && newAccount.credentials) {
      const oauthCredentials = newAccount.credentials as Record<string, unknown>
      loadModelRestrictionFromMapping(oauthCredentials.model_mapping as Record<string, unknown> | undefined)
    } else {
      modelRestrictionMode.value = 'whitelist'
      modelMappings.value = []
      allowedModels.value = []
    }
    poolModeEnabled.value = false
    poolModeRetryCount.value = DEFAULT_POOL_MODE_RETRY_COUNT
    poolModeRetryStatusCodesInput.value = ''
    cpaModeEnabled.value = false
    cpaUseBaseUrl.value = true
    cpaManagementUrl.value = ''
    cpaManagementKey.value = ''
    cpaConcurrencyPerCredential.value = 10
    cpaExcludeAbnormalCredentials.value = false
    customErrorCodesEnabled.value = false
    selectedErrorCodes.value = []
  }
  editApiKey.value = ''
}

async function loadTLSProfiles() {
  try {
    const profiles = await listTLSFingerprintProfiles()
    tlsFingerprintProfiles.value = profiles.map(p => ({ id: p.id, name: p.name }))
  } catch {
    tlsFingerprintProfiles.value = []
  }
}

watch(
  [() => props.show, () => props.account],
  ([show, newAccount], [wasShow, previousAccount]) => {
    if (!show || !newAccount) {
      return
    }
    if (!wasShow || newAccount !== previousAccount) {
      syncFormFromAccount(newAccount)
      loadTLSProfiles()
    }
  },
  { immediate: true }
)

// Model mapping helpers
const addModelMapping = () => {
  addEmptyModelMapping(modelMappings.value)
}

const removeModelMapping = (index: number) => {
  removeModelMappingAt(modelMappings.value, index)
}

const addPresetMapping = (from: string, to: string) => {
  if (!addPresetModelMapping(modelMappings.value, from, to)) {
    appStore.showInfo(t('admin.accounts.mappingExists', { model: from }))
  }
}

const addAntigravityModelMapping = () => {
  addEmptyModelMapping(antigravityModelMappings.value)
}

const addOpenAICompactModelMapping = () => {
  addEmptyModelMapping(openAICompactModelMappings.value)
}

const removeOpenAICompactModelMapping = (index: number) => {
  removeModelMappingAt(openAICompactModelMappings.value, index)
}

const removeAntigravityModelMapping = (index: number) => {
  removeModelMappingAt(antigravityModelMappings.value, index)
}

const addAntigravityPresetMapping = (from: string, to: string) => {
  if (!addPresetModelMapping(antigravityModelMappings.value, from, to)) {
    appStore.showInfo(t('admin.accounts.mappingExists', { model: from }))
  }
}

const syncAntigravityUpstreamModels = async () => {
  if (!props.account?.id || isSyncingAntigravityUpstream.value) return

  isSyncingAntigravityUpstream.value = true
  try {
    const result = await syncUpstreamModels(props.account.id)
    const upstreamModels = result.models.map((model) => model.trim()).filter(Boolean)
    if (upstreamModels.length === 0) {
      appStore.showInfo(t('admin.accounts.syncUpstreamModelsEmpty'))
      return
    }

    let addedCount = 0
    for (const model of upstreamModels) {
      const exists = antigravityModelMappings.value.some((mapping) => mapping.from === model)
      if (!exists) {
        antigravityModelMappings.value.push({ from: model, to: model })
        addedCount += 1
      }
    }

    if (addedCount > 0) {
      appStore.showSuccess(t('admin.accounts.syncUpstreamModelsSuccess', { count: addedCount, total: upstreamModels.length }))
    } else {
      appStore.showInfo(t('admin.accounts.syncUpstreamModelsNoChanges', { count: upstreamModels.length }))
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : t('admin.accounts.syncUpstreamModelsFailed')
    appStore.showError(t('admin.accounts.syncUpstreamModelsError', { message }))
  } finally {
    isSyncingAntigravityUpstream.value = false
  }
}

// Error code toggle helper
const toggleErrorCode = (code: number) => {
  const index = selectedErrorCodes.value.indexOf(code)
  if (index === -1) {
    // Adding code - check for 429/529 warning
    if (code === 429) {
      if (!confirm(t('admin.accounts.customErrorCodes429Warning'))) {
        return
      }
    } else if (code === 529) {
      if (!confirm(t('admin.accounts.customErrorCodes529Warning'))) {
        return
      }
    }
    selectedErrorCodes.value.push(code)
  } else {
    selectedErrorCodes.value.splice(index, 1)
  }
}

// Add custom error code from input
const addCustomErrorCode = () => {
  const code = customErrorCodeInput.value
  if (code === null || code < 100 || code > 599) {
    appStore.showError(t('admin.accounts.invalidErrorCode'))
    return
  }
  if (selectedErrorCodes.value.includes(code)) {
    appStore.showInfo(t('admin.accounts.errorCodeExists'))
    return
  }
  // Check for 429/529 warning
  if (code === 429) {
    if (!confirm(t('admin.accounts.customErrorCodes429Warning'))) {
      return
    }
  } else if (code === 529) {
    if (!confirm(t('admin.accounts.customErrorCodes529Warning'))) {
      return
    }
  }
  selectedErrorCodes.value.push(code)
  customErrorCodeInput.value = null
}

// Remove error code
const removeErrorCode = (code: number) => {
  const index = selectedErrorCodes.value.indexOf(code)
  if (index !== -1) {
    selectedErrorCodes.value.splice(index, 1)
  }
}

const addTempUnschedRule = (preset?: TempUnschedRuleForm) => {
  tempUnschedRules.value.push(createTempUnschedRule(preset))
}

const removeTempUnschedRule = (index: number) => {
  removeModelMappingAt(tempUnschedRules.value, index)
}

const moveTempUnschedRule = (index: number, direction: number) => {
  moveTempUnschedRuleInPlace(tempUnschedRules.value, index, direction)
}

const applyTempUnschedConfig = (credentials: Record<string, unknown>) => {
  if (!tempUnschedEnabled.value) {
    delete credentials.temp_unschedulable_enabled
    delete credentials.temp_unschedulable_rules
    return true
  }

  const rules = buildTempUnschedRules(tempUnschedRules.value)
  if (rules.length === 0) {
    appStore.showError(t('admin.accounts.tempUnschedulable.rulesInvalid'))
    return false
  }

  credentials.temp_unschedulable_enabled = true
  credentials.temp_unschedulable_rules = rules
  return true
}

function loadTempUnschedRules(credentials?: Record<string, unknown>) {
  tempUnschedEnabled.value = credentials?.temp_unschedulable_enabled === true
  tempUnschedRules.value = parseTempUnschedRules(credentials?.temp_unschedulable_rules)
}

// Load quota control settings from account (Anthropic OAuth/SetupToken only)
function loadQuotaControlSettings(account: Account) {
  const extra = account.extra as Record<string, unknown> | undefined
  // Reset all quota control state first
  windowCostEnabled.value = false
  windowCostLimit.value = null
  windowCostStickyReserve.value = null
  sessionLimitEnabled.value = false
  maxSessions.value = null
  sessionIdleTimeout.value = null
  rpmLimitEnabled.value = false
  baseRpm.value = null
  rpmStrategy.value = 'tiered'
  rpmStickyBuffer.value = null
  userMsgQueueMode.value = ''
  tlsFingerprintEnabled.value = false
  tlsFingerprintProfileId.value = null
  sessionIdMaskingEnabled.value = false
  cacheTTLOverrideEnabled.value = false
  cacheTTLOverrideTarget.value = '5m'
  customBaseUrlEnabled.value = false
  customBaseUrl.value = ''

  const tlsFingerprintEligible =
    (account.platform === 'anthropic' && (account.type === 'oauth' || account.type === 'setup-token')) ||
    (account.platform === 'openai' && account.type === 'oauth')

  // TLS fingerprint settings are also available for OpenAI OAuth/Codex accounts.
  if (tlsFingerprintEligible) {
    const storedEnabled = account.enable_tls_fingerprint === true
    tlsFingerprintEnabled.value = storedEnabled || extra?.enable_tls_fingerprint === true
    const storedProfileID = account.tls_fingerprint_profile_id ?? extra?.tls_fingerprint_profile_id
    tlsFingerprintProfileId.value = normalizeTLSFingerprintProfileID(storedProfileID)
  }

  // Custom model-request relay is supported by Anthropic OAuth/SetupToken and
  // OpenAI OAuth/Codex.  Keep this independent from the Anthropic-only quota
  // controls below so OpenAI OAuth accounts can edit it as well.
  const customRelayEligible =
    (account.platform === 'anthropic' &&
      (account.type === 'oauth' || account.type === 'setup-token')) ||
    (account.platform === 'openai' && account.type === 'oauth')
  if (customRelayEligible && account.custom_base_url_enabled === true) {
    customBaseUrlEnabled.value = true
    customBaseUrl.value = account.custom_base_url || ''
  }

  // Remaining quota controls apply only to Anthropic OAuth/SetupToken accounts.
  if (account.platform !== 'anthropic') {
    return
  }

  // Window cost / session limit only apply to Anthropic OAuth/SetupToken accounts
  if (account.type !== 'oauth' && account.type !== 'setup-token') {
    return
  }

  // Load from extra field (via backend DTO fields)
  if (account.window_cost_limit != null && account.window_cost_limit > 0) {
    windowCostEnabled.value = true
    windowCostLimit.value = account.window_cost_limit
    windowCostStickyReserve.value = account.window_cost_sticky_reserve ?? 10
  }

  if (account.max_sessions != null && account.max_sessions > 0) {
    sessionLimitEnabled.value = true
    maxSessions.value = account.max_sessions
    sessionIdleTimeout.value = account.session_idle_timeout_minutes ?? 5
  }

  // RPM limit
  if (account.base_rpm != null && account.base_rpm > 0) {
    rpmLimitEnabled.value = true
    baseRpm.value = account.base_rpm
    rpmStrategy.value = (account.rpm_strategy as 'tiered' | 'sticky_exempt') || 'tiered'
    rpmStickyBuffer.value = account.rpm_sticky_buffer ?? null
  }

  // UMQ mode（独立于 RPM 加载，防止编辑无 RPM 账号时丢失已有配置）
  userMsgQueueMode.value = account.user_msg_queue_mode ?? ''

  // Load session ID masking setting
  if (account.session_id_masking_enabled === true) {
    sessionIdMaskingEnabled.value = true
  }

  // Load cache TTL override setting
  if (account.cache_ttl_override_enabled === true) {
    cacheTTLOverrideEnabled.value = true
    cacheTTLOverrideTarget.value = account.cache_ttl_override_target || '5m'
  }

}



function normalizeTLSFingerprintProfileID(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null
  const parsed = typeof value === 'number' ? value : Number(value)
  if (!Number.isInteger(parsed) || (parsed !== -1 && parsed <= 0)) return null
  return parsed
}

const {
  handleClose,
  handleMixedChannelCancel,
  handleMixedChannelConfirm,
  handleSubmit,
} = useEditAccountSubmission({
  account: () => props.account, allowOverages, anthropicAPIKeyAuthScheme,
  anthropicPassthroughEnabled, antigravityMixedChannelConfirmed, antigravityModelMappings,
  antigravityProjectId, applyCodexWebSearchCapability, applyOpenAIEndpointCapabilities,
  applyOpenAIModelMappingCredentials, applyTempUnschedConfig,
  autoDisableOnUpstreamInsufficientBalance, autoPause5hDisabled, autoPause5hThreshold,
  autoPause7dDisabled, autoPause7dThreshold, autoPauseOnExpired, baseRpm,
  buildModelRestrictionMapping, cacheTTLOverrideEnabled, cacheTTLOverrideTarget,
  codexFingerprintMode, codexPrewarmContinuationEnabled, codexThinkingTagNormalizationEnabled, codexCLIOnlyAppServerEnabled, codexCLIOnlyEnabled, codexImageToolMode,
  cpaConcurrencyPerCredential, cpaExcludeAbnormalCredentials, cpaManagementKey, cpaManagementUrl, cpaModeEnabled,
  cpaUseBaseUrl,
  customBaseUrl, customBaseUrlEnabled, customErrorCodesEnabled, defaultBaseUrl,
  editApiKey, editBaseUrl, editCNAccountMode, editCNAPIProtocol, editCNAdaptiveBaseURLs, editOpenCodeAccountMode, editOpenCodeProtocolRules, editZhipuOrganization, editZhipuProject, editBedrockAccessKeyId, editBedrockApiKeyValue,
  editBedrockForceGlobal, editBedrockRegion, editBedrockSecretAccessKey,
  editBedrockSessionToken, editDailyResetHour, editDailyResetMode, editPlanType,
  editQuotaDailyLimit, editQuotaLimit, editQuotaWeeklyLimit, editResetTimezone,
  editVertexClientEmail, editVertexLocation, editVertexProjectId, editWeeklyResetDay,
  editWeeklyResetHour, editWeeklyResetMode, form, grokClientToolCacheEnabled,
  grokClientToolCacheExtraKey: GROK_CLIENT_TOOL_CACHE_EXTRA_KEY, grokOAuthBaseUrl,
  grokOAuthCustomBaseUrlEnabled, headerOverrideEnabled, headerOverrideRows,
  interceptWarmupRequests, isBedrockAPIKeyMode, isOpenAIPersonalAccessTokenAccount,
  isSparkShadow, maxCPAConcurrencyPerCredential: MAX_CPA_CONCURRENCY_PER_CREDENTIAL,
  maxSessions, mixedChannelWarningAction, mixedChannelWarningDetails,
  mixedChannelWarningRawMessage, mixedScheduling, notifications,
  onClose: () => emit('close'), onUpdated: (account) => emit('updated', account),
  excelBpsEnabled, openAICompactMode, openAICompactModelMappings, openAIForceImageAPIEnabled,
  openAILongContextBillingEnabled, openAIResponsesMode,
  openAITextGenerationCapabilityEnabled, openaiAPIKeyResponsesWebSocketV2Mode,
  openaiOAuthResponsesWebSocketV2Mode, openaiFlattenNamespacesEnabled,
  openaiPassthroughEnabled, openAIImageSizeTiers, poolModeEnabled,
  poolModeRetryCount, poolModeRetryStatusCodesInput, rpmLimitEnabled, rpmStickyBuffer,
  rpmStrategy, selectedErrorCodes, sessionIdMaskingEnabled, sessionIdleTimeout,
  sessionLimitEnabled, showMixedChannelWarning, submitting, t, tlsFingerprintEnabled,
  tlsFingerprintProfileId, upstreamBillingAutoProbeEnabled, userMsgQueueMode,
  upstreamBillingRateSyncEnabled,
  webSearchEmulationMode, windowCostEnabled, windowCostLimit, windowCostStickyReserve,
  writeQuotaNotifyToExtra,
})

const editAccountCredentialContext = {
  CPA_SNAPSHOT_INTERVAL_SECONDS, DEFAULT_POOL_MODE_RETRY_COUNT, DEFAULT_POOL_MODE_RETRY_STATUS_CODES,
  MAX_CPA_CONCURRENCY_PER_CREDENTIAL, MAX_POOL_MODE_RETRY_COUNT, VERTEX_LOCATION_OPTIONS,
  account: activeAccount, addAntigravityModelMapping, addAntigravityPresetMapping, addCustomErrorCode,
  addModelMapping, addPresetMapping, allowedModels, antigravityModelMappings,
  antigravityPresetMappings, antigravityProjectId, autoDisableOnUpstreamInsufficientBalance,
  baseUrlHint, bedrockPresets, commonErrorCodes, cpaConcurrencyPerCredential, cpaExcludeAbnormalCredentials, cpaManagementKey,
  cpaManagementUrl, cpaModeEnabled, cpaUseBaseUrl, customErrorCodeInput, customErrorCodesEnabled, editApiKey,
  editBaseUrl, editCNAccountMode, editCNAPIProtocol, editCNAdaptiveBaseURLs, editOpenCodeAccountMode, editOpenCodeProtocolRules, editZhipuOrganization, editZhipuProject, editBedrockAccessKeyId, editBedrockApiKeyValue, editBedrockForceGlobal,
  editBedrockRegion, editBedrockSecretAccessKey, editBedrockSessionToken, editVertexLocation,
  editVertexProjectId, form, getAntigravityModelMappingKey, getModelMappingKey,
  grokClientToolCacheEnabled, grokOAuthBaseUrl, grokOAuthCustomBaseUrlEnabled,
  headerOverrideCapable, headerOverrideEnabled, headerOverrideRows, isBedrockAPIKeyMode,
  isOpenAIModelRestrictionDisabled, isSyncingAntigravityUpstream, isTestingCPA, isValidWildcardPattern,
  modelMappings, modelRestrictionMode, poolModeEnabled, poolModeRetryCount,
  poolModeRetryStatusCodesInput, presetMappings, removeAntigravityModelMapping, removeErrorCode,
  removeModelMapping, selectedErrorCodes, syncAntigravityUpstreamModels, t, testCPAConnection,
  toggleErrorCode,
} satisfies EditAccountCredentialContext

const editAccountAdvancedContext = {
  account: activeAccount, addTempUnschedRule, anthropicAPIKeyAuthScheme,
  anthropicPassthroughEnabled, codexFingerprintMode, codexFingerprintModeOptions,
  codexPrewarmContinuationEnabled, codexThinkingTagNormalizationEnabled, codexCLIOnlyAppServerEnabled, codexCLIOnlyEnabled,
  codexImageToolBadgeClass, codexImageToolBadgeLabel, codexImageToolMode, codexImageToolOptions,
  codexWebSearchEnabled, editDailyResetHour, editDailyResetMode, editQuotaDailyLimit,
  editQuotaLimit, editQuotaWeeklyLimit, editResetTimezone, editWeeklyResetDay,
  editWeeklyResetHour, editWeeklyResetMode, expiresAtInput, form, getTempUnschedRuleKey,
  handleOllamaCloudUsageUpdated, interceptWarmupRequests, isOpenAIPersonalAccessTokenAccount,
  isSparkShadow, moveTempUnschedRule, openAIEndpointCapabilities, openAIEndpointCapabilityOptions,
  openAIForceImageAPIEnabled, openAILongContextBillingEnabled, openAIResponsesMode,
  openAIResponsesModeOptions, openAIResponsesStatusKey, openAITextGenerationCapabilityEnabled,
  openAIWSModeConcurrencyHintKey, openAIWSModeOptions, openaiPassthroughEnabled,
  openaiFlattenNamespacesEnabled,
  openaiResponsesWebSocketV2Mode, proxies: availableProxies, quotaNotifyGlobalEnabled,
  quotaNotifyState, removeTempUnschedRule, t, tempUnschedEnabled, tempUnschedPresets,
  tempUnschedRules, toggleOpenAIEndpointCapability, upstreamBillingAutoProbeEnabled,
  upstreamBillingRateSyncEnabled,
  webSearchEmulationMode, webSearchGlobalEnabled,
} satisfies EditAccountAdvancedContext

const editAccountPolicyContext = {
  account: activeAccount, addOpenAICompactModelMapping, allowOverages, isSimpleMode,
  autoPause5hDisabled, autoPause5hThreshold, autoPause7dDisabled, autoPause7dThreshold,
  autoPauseOnExpired, baseRpm, cacheTTLOverrideEnabled, cacheTTLOverrideTarget, customBaseUrl,
  customBaseUrlEnabled, editPlanType, excelBpsEnabled, form, formatDateTime, getOpenAICompactModelMappingKey,
  groups: availableGroups, isOpenAIPersonalAccessTokenAccount, isSparkShadow, maxSessions, mixedScheduling, openAICompactMode,
  openAICompactModeOptions, openAICompactModelMappings, openAICompactStatusKey, planTypeOptions,
  removeOpenAICompactModelMapping, rpmLimitEnabled, rpmStickyBuffer, rpmStrategy,
  sessionIdMaskingEnabled, sessionIdleTimeout, sessionLimitEnabled, statusOptions, t,
  tlsFingerprintEnabled, tlsFingerprintProfileId, tlsFingerprintProfiles, umqModeOptions,
  userMsgQueueMode, windowCostEnabled, windowCostLimit, windowCostStickyReserve,
} satisfies EditAccountPolicyContext
</script>
