<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.createAccount')"
    width="wide"
    @close="handleClose"
  >
    <CreateAccountStepIndicator :context="createAccountStepIndicatorContext" />

    <!-- Step 1: Basic Info -->
    <form
      v-if="step === 1"
      id="create-account-form"
      @submit.prevent="handleSubmit"
      class="space-y-5"
    >
      <CreateAccountPlatformFields :context="createAccountPlatformContext" />

      <CreateAccountCredentialFields :context="createAccountCredentialContext" />

      <OpenAIImageSizeTiersField v-if="form.platform === 'openai'" v-model="openAIImageSizeTiers" />

      <CreateAccountAdvancedFields :context="createAccountAdvancedContext" />

    </form>

    <!-- Step 2: OAuth Authorization -->
    <div v-else class="space-y-5">
      <OAuthAuthorizationFlow
        ref="oauthFlowRef"
        :add-method="form.platform === 'anthropic' ? addMethod : 'oauth'"
        :auth-url="currentAuthUrl"
        :session-id="currentSessionId"
        :loading="currentOAuthLoading"
        :error="currentOAuthError"
        :show-help="form.platform === 'anthropic'"
        :show-proxy-warning="form.platform !== 'openai' && form.platform !== 'grok' && !!form.proxy_id"
        :allow-multiple="form.platform === 'anthropic'"
        :show-cookie-option="form.platform === 'anthropic'"
        :show-refresh-token-option="form.platform === 'openai' || form.platform === 'antigravity' || form.platform === 'grok'"
        :show-mobile-refresh-token-option="form.platform === 'openai'"
        :show-session-token-option="false"
        :show-access-token-option="false"
        :show-codex-session-import-option="form.platform === 'openai'"
        :show-agent-identity-option="form.platform === 'openai'"
        :show-codex-pat-option="form.platform === 'openai'"
        :show-sso-option="form.platform === 'grok'"
        :show-manual-option="true"
        :initial-input-method="'manual'"
        :platform="form.platform"
        :show-project-id="geminiOAuthType === 'code_assist'"
        @generate-url="handleGenerateUrl"
        @cookie-auth="handleCookieAuth"
        @validate-refresh-token="handleValidateRefreshToken"
        @validate-mobile-refresh-token="handleOpenAIValidateMobileRT"
        @validate-session-token="handleValidateSessionToken"
        @import-codex-session="handleOpenAIImportCodexSession"
        @import-codex-pat="handleOpenAIImportCodexPAT"
        @import-sso="handleGrokImportSSO"
      />

    </div>

    <template #footer>
      <CreateAccountDialogFooter :context="createAccountFooterContext" />
    </template>
  </BaseDialog>

      <GeminiAccountHelpDialog :context="geminiAccountHelpContext" />

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
import { ref, reactive, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/core/stores/appStore'
import {
  claudeModels,
  getPresetMappingsByPlatform,
  commonErrorCodes,
  buildModelMappingObject,
  fetchAntigravityDefaultMappings,
  isValidWildcardPattern
} from '@/features/admin-accounts/presentation/composables/useModelWhitelist'
import { useAuthStore } from '@/features/auth'
import {
  checkMixedChannelRisk,
  createAccount,
  probeUpstreamBilling
} from '@/features/admin-accounts/data/datasources/adminAccountActions'
import {
  getWebSearchEmulationConfig
} from '@/features/admin-settings/data/datasources/adminSettingsQueries'
import {
  list as listTLSFingerprintProfiles
} from '@/features/admin-settings/data/datasources/tlsFingerprintProfileDatasource'
import { useQuotaNotifyState } from '@/features/admin-accounts/presentation/composables/useQuotaNotifyState'
import {
  useAccountOAuth,
  type AddMethod
} from '@/features/admin-accounts/presentation/composables/useAccountOAuth'
import { useOpenAIOAuth } from '@/features/admin-accounts/presentation/composables/useOpenAIOAuth'
import { useGeminiOAuth } from '@/features/admin-accounts/presentation/composables/useGeminiOAuth'
import { useAntigravityOAuth } from '@/features/admin-accounts/presentation/composables/useAntigravityOAuth'
import { useGrokOAuth } from '@/features/admin-accounts/presentation/composables/useGrokOAuth'
import {
  useCreateAccountOAuthActions,
  type OAuthFlowExposed,
} from '@/features/admin-accounts/presentation/composables/useCreateAccountOAuthActions'
import { useCreateAccountEditorPolicy } from '@/features/admin-accounts/presentation/composables/useCreateAccountEditorPolicy'
import type {
  Proxy,
  AdminGroup,
  AccountPlatform,
  AccountType,
  OpenAICompactMode,
  OpenAIResponsesMode,
  OpenAIEndpointCapability
} from '@/types'
import type {
  CheckMixedChannelResponse,
  CreateAccountRequest
} from '@/features/admin-accounts/data/dtos/adminAccountDtos'
import BaseDialog from '@/common/widgets/feedback/BaseDialog.vue'
import ConfirmDialog from '@/common/widgets/feedback/ConfirmDialog.vue'
import {
  applyHeaderOverride,
  applyInterceptWarmup,
  isHeaderOverrideCapable,
  validateHeaderOverrideRows,
  type HeaderOverrideRow
} from '@/features/admin-accounts/presentation/credentialsBuilder'
import { formatDateTimeLocalInput, parseDateTimeLocalInput } from '@/core/utils/format'
import { createStableObjectKeyResolver } from '@/core/utils/stableObjectKey'
import {
  VERTEX_LOCATION_OPTIONS,
  accountAPIKeyFieldHint,
  defaultAPIKeyBaseURL,
} from '@/core/constants/account'
import {
  OPENAI_WS_MODE_CTX_POOL,
  OPENAI_WS_MODE_OFF,
  OPENAI_WS_MODE_PASSTHROUGH,
  OPENAI_WS_MODE_HTTP_BRIDGE,
  resolveOpenAIWSModeConcurrencyHintKey,
  type OpenAIWSMode
} from '@/core/utils/openaiWsMode'
import OAuthAuthorizationFlow from './OAuthAuthorizationFlow.vue'
import CreateAccountPlatformFields from './create/CreateAccountPlatformFields.vue'
import CreateAccountCredentialFields from './create/CreateAccountCredentialFields.vue'
import CreateAccountAdvancedFields from './create/CreateAccountAdvancedFields.vue'
import OpenAIImageSizeTiersField from './OpenAIImageSizeTiersField.vue'
import { buildCreateOpenAIExtra } from '../accountCreateOpenAIExtra'
import { OPENAI_IMAGE_SIZE_TIERS, type OpenAIImageSizeTier } from '../openaiImageSizeTiers'
import CreateAccountDialogFooter from './create/CreateAccountDialogFooter.vue'
import CreateAccountStepIndicator from './create/CreateAccountStepIndicator.vue'
import GeminiAccountHelpDialog from './create/GeminiAccountHelpDialog.vue'
import {
  DEFAULT_POOL_MODE_RETRY_COUNT,
  DEFAULT_POOL_MODE_RETRY_STATUS_CODES,
  MAX_POOL_MODE_RETRY_COUNT,
  getCodexFingerprintModeOptions,
  buildAPIKeySearchExtra,
  normalizePoolModeRetryCount,
  parsePoolModeRetryStatusCodes,
  type CodexFingerprintMode,
  type ModelMapping,
  type TempUnschedRuleForm,
} from '@/features/admin-accounts/presentation/accountFormPolicy'
import type {
  CreateAccountAdvancedContext,
  CreateAccountCredentialContext,
  CreateAccountFooterContext,
  CreateAccountPlatformContext,
  CreateAccountStepIndicatorContext,
  GeminiAccountHelpContext,
} from '@/features/admin-accounts/presentation/accountEditorContext'
import { isUpstreamBillingProbeEligible } from '@/features/admin-accounts/presentation/upstreamBillingProbeEligibility'
import { applyCreateCNProviderCredentials } from '@/features/admin-accounts/presentation/createCNProviderCredentials'
import { useCNAccountEditorState } from '@/features/admin-accounts/presentation/composables/useCNAccountEditorState'

const { t } = useI18n()
const authStore = useAuthStore()
const isSimpleMode = computed(() => authStore.isSimpleMode)

const oauthStepTitle = computed(() => {
  if (form.platform === 'openai') return t('admin.accounts.oauth.openai.title')
  if (form.platform === 'gemini') return t('admin.accounts.oauth.gemini.title')
  if (form.platform === 'antigravity') return t('admin.accounts.oauth.antigravity.title')
  if (form.platform === 'grok') return t('admin.accounts.oauth.grok.title')
  return t('admin.accounts.oauth.title')
})

// Platform-specific hints for API Key type
const baseUrlHint = computed(() => accountAPIKeyFieldHint(t, form.platform, 'baseUrl'))
const apiKeyHint = computed(() => accountAPIKeyFieldHint(t, form.platform, 'apiKey'))

interface Props {
  show: boolean
  proxies: Proxy[]
  groups: AdminGroup[]
}

const props = defineProps<Props>()
const availableGroups = computed(() => props.groups)
const availableProxies = computed(() => props.proxies)
const emit = defineEmits<{
  close: []
  created: []
}>()

const appStore = useAppStore()
const notifications = {
  showError: (message: string) => appStore.showError(message),
  showInfo: (message: string) => appStore.showInfo(message),
  showSuccess: (message: string) => appStore.showSuccess(message),
  showWarning: (message: string) => appStore.showWarning(message),
}

// OAuth composables
const oauth = useAccountOAuth() // For Anthropic OAuth
const openaiOAuth = useOpenAIOAuth() // For OpenAI OAuth
const geminiOAuth = useGeminiOAuth() // For Gemini OAuth
const antigravityOAuth = useAntigravityOAuth() // For Antigravity OAuth
const grokOAuth = useGrokOAuth() // For Grok OAuth

// Computed: current OAuth state for template binding
const currentAuthUrl = computed(() => {
  if (form.platform === 'openai') return openaiOAuth.authUrl.value
  if (form.platform === 'gemini') return geminiOAuth.authUrl.value
  if (form.platform === 'antigravity') return antigravityOAuth.authUrl.value
  if (form.platform === 'grok') return grokOAuth.authUrl.value
  return oauth.authUrl.value
})

const currentSessionId = computed(() => {
  if (form.platform === 'openai') return openaiOAuth.sessionId.value
  if (form.platform === 'gemini') return geminiOAuth.sessionId.value
  if (form.platform === 'antigravity') return antigravityOAuth.sessionId.value
  if (form.platform === 'grok') return grokOAuth.sessionId.value
  return oauth.sessionId.value
})

const currentOAuthLoading = computed(() => {
  if (form.platform === 'openai') return openaiOAuth.loading.value
  if (form.platform === 'gemini') return geminiOAuth.loading.value
  if (form.platform === 'antigravity') return antigravityOAuth.loading.value
  if (form.platform === 'grok') return grokOAuth.loading.value
  return oauth.loading.value
})

const currentOAuthError = computed(() => {
  if (form.platform === 'openai') return openaiOAuth.error.value
  if (form.platform === 'gemini') return geminiOAuth.error.value
  if (form.platform === 'antigravity') return antigravityOAuth.error.value
  if (form.platform === 'grok') return grokOAuth.error.value
  return oauth.error.value
})

// Refs
const oauthFlowRef = ref<OAuthFlowExposed | null>(null)

// State
const step = ref(1)
const submitting = ref(false)
const accountCategory = ref<'oauth-based' | 'apikey' | 'bedrock' | 'service_account'>('oauth-based') // UI selection for account category
const addMethod = ref<AddMethod>('oauth') // For oauth-based: 'oauth' or 'setup-token'
const apiKeyBaseUrl = ref('https://api.anthropic.com')
const apiKeyValue = ref('')
const upstreamBillingAutoProbeEnabled = ref(true)
const autoDisableOnUpstreamInsufficientBalance = ref(false)

const syncPreviewCredentials = computed(() => {
  if (!apiKeyValue.value) return undefined
  return {
    platform: form.platform,
    type: form.type,
    base_url: apiKeyBaseUrl.value || undefined,
    api_key: apiKeyValue.value
  }
})

const editQuotaLimit = ref<number | null>(null)
const editQuotaDailyLimit = ref<number | null>(null)
const editQuotaWeeklyLimit = ref<number | null>(null)
const editDailyResetMode = ref<'rolling' | 'fixed' | null>(null)
const editDailyResetHour = ref<number | null>(null)
const editWeeklyResetMode = ref<'rolling' | 'fixed' | null>(null)
const editWeeklyResetDay = ref<number | null>(null)
const editWeeklyResetHour = ref<number | null>(null)
const editResetTimezone = ref<string | null>(null)
const modelMappings = ref<ModelMapping[]>([])
const openAICompactModelMappings = ref<ModelMapping[]>([])
const modelRestrictionMode = ref<'whitelist' | 'mapping'>('whitelist')
const allowedModels = ref<string[]>([])
const poolModeEnabled = ref(false)
const poolModeRetryCount = ref(DEFAULT_POOL_MODE_RETRY_COUNT)
const poolModeRetryStatusCodesInput = ref('')
const customErrorCodesEnabled = ref(false)
const selectedErrorCodes = ref<number[]>([])
const customErrorCodeInput = ref<number | null>(null)
const headerOverrideEnabled = ref(false)
const headerOverrideRows = ref<HeaderOverrideRow[]>([])

// Grok OAuth：自定义上游地址（base_url 仅改写转发端点，OAuth 授权/刷新不受影响）
const grokOAuthCustomBaseUrlEnabled = ref(false)
const grokOAuthBaseUrl = ref('')

// Grok OAuth 三条创建路径（授权码/RT 批量/SSO 批量）共用的前置校验。
// 授权码路径必须在兑换 code 之前调用，避免校验失败时白白消耗一次性授权码。
const validateGrokOAuthUpstreamConfig = (): boolean => {
  if (grokOAuthCustomBaseUrlEnabled.value) {
    const trimmed = grokOAuthBaseUrl.value.trim()
    if (!trimmed) {
      appStore.showError(t('admin.accounts.grokCustomBaseUrl.required'))
      return false
    }
    if (!/^https?:\/\//i.test(trimmed)) {
      appStore.showError(t('admin.accounts.grokCustomBaseUrl.invalid'))
      return false
    }
  }
  if (headerOverrideEnabled.value) {
    const headerError = validateHeaderOverrideRows(headerOverrideRows.value)
    if (headerError) {
      appStore.showError(t(`admin.accounts.headerOverride.${headerError}`))
      return false
    }
  }
  return true
}

// 把已通过校验的自定义上游地址与请求头覆写写入 credentials
const applyGrokOAuthUpstreamConfig = (credentials: Record<string, unknown>) => {
  if (grokOAuthCustomBaseUrlEnabled.value) {
    credentials.base_url = grokOAuthBaseUrl.value.trim()
  }
  applyHeaderOverride(credentials, headerOverrideEnabled.value, headerOverrideRows.value, 'create')
}
const interceptWarmupRequests = ref(false)
const autoPauseOnExpired = ref(true)
const openaiPassthroughEnabled = ref(false)
const openAIImageSizeTiers = ref<OpenAIImageSizeTier[]>([...OPENAI_IMAGE_SIZE_TIERS])
// OAuth-only compatibility switch; default preserves namespace declarations.
const openaiFlattenNamespacesEnabled = ref(false)
const openAILongContextBillingEnabled = ref(false)
const openAILongContextBillingTouched = ref(false)
const openAICompactMode = ref<OpenAICompactMode>('auto')
const openAIResponsesMode = ref<OpenAIResponsesMode>('auto')
const openAIEndpointCapabilities = ref<OpenAIEndpointCapability[]>(['chat_completions', 'embeddings', 'decisions'])
const openAIForceImageAPIEnabled = ref(false)
const openaiOAuthResponsesWebSocketV2Mode = ref<OpenAIWSMode>(OPENAI_WS_MODE_OFF)
const openaiAPIKeyResponsesWebSocketV2Mode = ref<OpenAIWSMode>(OPENAI_WS_MODE_OFF)
const codexFingerprintMode = ref<CodexFingerprintMode>('off')
const codexPrewarmContinuationEnabled = ref(false)
const codexThinkingTagNormalizationEnabled = ref(false)
const codexCLIOnlyEnabled = ref(false)
const codexCLIOnlyAppServerEnabled = ref(false)
type AnthropicAPIKeyAuthScheme = 'x_api_key' | 'authorization_bearer'
const anthropicPassthroughEnabled = ref(false)
const anthropicAPIKeyAuthScheme = ref<AnthropicAPIKeyAuthScheme>('x_api_key')
const webSearchEmulationMode = ref('default')
const webSearchGlobalEnabled = ref(false)

const toggleOpenAILongContextBilling = () => {
  openAILongContextBillingEnabled.value = !openAILongContextBillingEnabled.value
  openAILongContextBillingTouched.value = true
}
const {
  globalEnabled: quotaNotifyGlobalEnabled,
  state: quotaNotifyState,
  loadGlobalState: loadQuotaNotifyGlobal,
  writeToExtra: writeQuotaNotifyToExtra,
} = useQuotaNotifyState()

// Load global feature states once
getWebSearchEmulationConfig().then(cfg => {
  webSearchGlobalEnabled.value = cfg?.enabled === true && (cfg?.providers?.length ?? 0) > 0
}).catch(() => { webSearchGlobalEnabled.value = false })

loadQuotaNotifyGlobal()
const mixedScheduling = ref(false) // For antigravity accounts: enable mixed scheduling
const allowOverages = ref(false) // For antigravity accounts: enable AI Credits overages
const antigravityAccountType = ref<'oauth' | 'upstream'>('oauth') // For antigravity: oauth or upstream
const antigravityProjectId = ref('')
const upstreamBaseUrl = ref('') // For upstream type: base URL
const upstreamApiKey = ref('') // For upstream type: API key
const antigravityModelRestrictionMode = ref<'whitelist' | 'mapping'>('whitelist')
const antigravityWhitelistModels = ref<string[]>([])
const antigravityModelMappings = ref<ModelMapping[]>([])
const antigravityPresetMappings = computed(() => getPresetMappingsByPlatform('antigravity'))
const bedrockPresets = computed(() => getPresetMappingsByPlatform('bedrock'))

// Bedrock credentials
const bedrockAuthMode = ref<'sigv4' | 'apikey'>('sigv4')
const bedrockAccessKeyId = ref('')
const bedrockSecretAccessKey = ref('')
const bedrockSessionToken = ref('')
const bedrockRegion = ref('us-east-1')
const bedrockForceGlobal = ref(false)
const bedrockApiKeyValue = ref('')
const vertexServiceAccountFileInput = ref<HTMLInputElement | null>(null)
const vertexServiceAccountJson = ref('')
const vertexProjectId = ref('')
const vertexClientEmail = ref('')
const vertexLocation = ref('global')
const vertexServiceAccountDragActive = ref(false)
const tempUnschedEnabled = ref(false)
const tempUnschedRules = ref<TempUnschedRuleForm[]>([])
const getModelMappingKey = createStableObjectKeyResolver<ModelMapping>('create-model-mapping')
const getOpenAICompactModelMappingKey = createStableObjectKeyResolver<ModelMapping>('create-openai-compact-model-mapping')
const getAntigravityModelMappingKey = createStableObjectKeyResolver<ModelMapping>('create-antigravity-model-mapping')
const getTempUnschedRuleKey = createStableObjectKeyResolver<TempUnschedRuleForm>('create-temp-unsched-rule')
const geminiOAuthType = ref<'code_assist' | 'google_one' | 'ai_studio'>('google_one')
const geminiAIStudioOAuthEnabled = ref(false)
const openAICompactModeOptions = computed<Array<{ value: OpenAICompactMode; label: string }>>(() => [
  { value: 'auto', label: t('admin.accounts.openai.compactModeAuto') },
  { value: 'force_on', label: t('admin.accounts.openai.compactModeForceOn') },
  { value: 'force_off', label: t('admin.accounts.openai.compactModeForceOff') }
])
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

const normalizeOpenAIEndpointCapabilities = (values: OpenAIEndpointCapability[]) => {
  const allowed: OpenAIEndpointCapability[] = ['chat_completions', 'embeddings', 'decisions']
  const selected = allowed.filter((value) => values.includes(value))
  return selected.length > 0 ? selected : allowed
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

function buildAntigravityExtra(): Record<string, unknown> | undefined {
  const extra: Record<string, unknown> = {}
  if (mixedScheduling.value) extra.mixed_scheduling = true
  if (allowOverages.value) extra.allow_overages = true
  return Object.keys(extra).length > 0 ? extra : undefined
}

const buildOpenAICompactModelMapping = () =>
  buildModelMappingObject('mapping', [], openAICompactModelMappings.value)

const showMixedChannelWarning = ref(false)
const mixedChannelWarningDetails = ref<{ groupName: string; currentPlatform: string; otherPlatform: string } | null>(
  null
)
const mixedChannelWarningRawMessage = ref('')
const mixedChannelWarningAction = ref<(() => Promise<void>) | null>(null)
const antigravityMixedChannelConfirmed = ref(false)
const showAdvancedOAuth = ref(false)
const showGeminiHelpDialog = ref(false)

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

// Gemini tier selection (used as fallback when auto-detection is unavailable/fails)
const geminiTierGoogleOne = ref<'google_one_free' | 'google_ai_pro' | 'google_ai_ultra'>('google_one_free')
const geminiTierGcp = ref<'gcp_standard' | 'gcp_enterprise'>('gcp_standard')
const geminiTierAIStudio = ref<'aistudio_free' | 'aistudio_paid'>('aistudio_free')

const geminiSelectedTier = computed(() => {
  if (form.platform !== 'gemini') return ''
  if (accountCategory.value === 'apikey') return geminiTierAIStudio.value
  switch (geminiOAuthType.value) {
    case 'google_one':
      return geminiTierGoogleOne.value
    case 'code_assist':
      return geminiTierGcp.value
    default:
      return geminiTierAIStudio.value
  }
})

const openAIWSModeOptions = computed<Array<{ value: OpenAIWSMode; label: string }>>(() => [
  { value: OPENAI_WS_MODE_OFF, label: t('admin.accounts.openai.wsModeOff') },
  { value: OPENAI_WS_MODE_CTX_POOL, label: t('admin.accounts.openai.wsModeCtxPool') },
  { value: OPENAI_WS_MODE_PASSTHROUGH, label: t('admin.accounts.openai.wsModePassthrough') },
  { value: OPENAI_WS_MODE_HTTP_BRIDGE, label: t('admin.accounts.openai.wsModeHttpBridge') }
])
const codexFingerprintModeOptions = computed(() => getCodexFingerprintModeOptions(t))

const openaiResponsesWebSocketV2Mode = computed({
  get: () => {
    if (form.platform === 'openai' && accountCategory.value === 'apikey') {
      return openaiAPIKeyResponsesWebSocketV2Mode.value
    }
    return openaiOAuthResponsesWebSocketV2Mode.value
  },
  set: (mode: OpenAIWSMode) => {
    if (form.platform === 'openai' && accountCategory.value === 'apikey') {
      openaiAPIKeyResponsesWebSocketV2Mode.value = mode
      return
    }
    openaiOAuthResponsesWebSocketV2Mode.value = mode
  }
})

const openAIWSModeConcurrencyHintKey = computed(() =>
  resolveOpenAIWSModeConcurrencyHintKey(openaiResponsesWebSocketV2Mode.value)
)

const isOpenAIModelRestrictionDisabled = computed(() =>
  form.platform === 'openai' && openaiPassthroughEnabled.value
)

const mixedChannelWarningMessageText = computed(() => {
  if (mixedChannelWarningDetails.value) {
    return t('admin.accounts.mixedChannelWarning', mixedChannelWarningDetails.value)
  }
  return mixedChannelWarningRawMessage.value
})

const geminiQuotaDocs = {
  codeAssist: 'https://developers.google.com/gemini-code-assist/resources/quotas',
  aiStudio: 'https://ai.google.dev/pricing',
  vertex: 'https://cloud.google.com/vertex-ai/generative-ai/docs/quotas'
}

const geminiHelpLinks = {
  apiKey: 'https://aistudio.google.com/app/apikey',
  aiStudioPricing: 'https://ai.google.dev/pricing',
  gcpProject: 'https://console.cloud.google.com/welcome/new',
  geminiWebActivation: 'https://gemini.google.com/gems/create?hl=en-US&pli=1',
  countryCheck: 'https://policies.google.com/terms',
  countryChange: 'https://policies.google.com/country-association-form'
}

// Computed: current preset mappings based on platform
const presetMappings = computed(() => getPresetMappingsByPlatform(form.platform))
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

const form = reactive({
  name: '',
  notes: '',
  platform: 'anthropic' as AccountPlatform,
  type: 'oauth' as AccountType, // Will be 'oauth', 'setup-token', or 'apikey'
  credentials: {} as Record<string, unknown>,
  proxy_id: null as number | null,
  concurrency: 10,
  load_factor: null as number | null,
  priority: 1,
  rate_multiplier: 1,
  group_ids: [] as number[],
  expires_at: null as number | null
})

const {
  cnAccountMode, cnAPIProtocol, cnAdaptiveBaseURLs, openCodeAccountMode,
  openCodeProtocolRules, resetCNAccountEditorState, zhipuOrganization, zhipuProject,
} = useCNAccountEditorState(() => form.platform)

// Helper to check if current type needs OAuth flow
const isOAuthFlow = computed(() => {
  // Antigravity upstream 类型不需要 OAuth 流程
  if (form.platform === 'antigravity' && antigravityAccountType.value === 'upstream') {
    return false
  }
  // Bedrock 类型不需要 OAuth 流程
  if (form.platform === 'anthropic' && accountCategory.value === 'bedrock') {
    return false
  }
  return accountCategory.value === 'oauth-based'
})

const isGrokSSOInputMethod = computed(() => form.platform === 'grok' && oauthFlowRef.value?.inputMethod === 'sso_cookie')

const isManualInputMethod = computed(() => {
  return oauthFlowRef.value?.inputMethod === 'manual'
})

const expiresAtInput = computed({
  get: () => formatDateTimeLocalInput(form.expires_at),
  set: (value: string) => {
    form.expires_at = parseDateTimeLocalInput(value)
  }
})

const canExchangeCode = computed(() => {
  const authCode = oauthFlowRef.value?.authCode || ''
  if (form.platform === 'openai') {
    return authCode.trim() && openaiOAuth.sessionId.value && !openaiOAuth.loading.value
  }
  if (form.platform === 'gemini') {
    return authCode.trim() && geminiOAuth.sessionId.value && !geminiOAuth.loading.value
  }
  if (form.platform === 'antigravity') {
    return authCode.trim() && antigravityOAuth.sessionId.value && !antigravityOAuth.loading.value
  }
  if (form.platform === 'grok') {
    return authCode.trim() && grokOAuth.sessionId.value && !grokOAuth.loading.value
  }
  return authCode.trim() && oauth.sessionId.value && !oauth.loading.value
})

const {
  addAntigravityModelMapping,
  addAntigravityPresetMapping,
  addCustomErrorCode,
  addModelMapping,
  addOpenAICompactModelMapping,
  addPresetMapping,
  addTempUnschedRule,
  applyTempUnschedConfig,
  handleSelectGeminiOAuthType,
  moveTempUnschedRule,
  removeAntigravityModelMapping,
  removeErrorCode,
  removeModelMapping,
  removeOpenAICompactModelMapping,
  removeTempUnschedRule,
  toggleErrorCode,
} = useCreateAccountEditorPolicy({
  accountCategory, addMethod, allowOverages, allowedModels, anthropicAPIKeyAuthScheme,
  anthropicPassthroughEnabled, antigravityAccountType, antigravityModelMappings,
  antigravityModelRestrictionMode, antigravityOAuth, antigravityProjectId,
  antigravityWhitelistModels, apiKeyBaseUrl, bedrockAccessKeyId, bedrockApiKeyValue,
  bedrockAuthMode, bedrockForceGlobal, bedrockRegion, bedrockSecretAccessKey,
    bedrockSessionToken, codexPrewarmContinuationEnabled, codexThinkingTagNormalizationEnabled, codexCLIOnlyAppServerEnabled, codexCLIOnlyEnabled,
  customErrorCodeInput, form, geminiAIStudioOAuthEnabled, geminiOAuth, geminiOAuthType,
  grokOAuth, grokOAuthBaseUrl, grokOAuthCustomBaseUrlEnabled, headerOverrideEnabled,
  headerOverrideRows, interceptWarmupRequests,
  loadTLSFingerprintProfiles: listTLSFingerprintProfiles,
  modelMappings, modelRestrictionMode, notifications, oauth,
  openAICompactModelMappings, openAIEndpointCapabilities, openAIForceImageAPIEnabled,
  openaiAPIKeyResponsesWebSocketV2Mode, openaiOAuth, openaiOAuthResponsesWebSocketV2Mode,
  openaiFlattenNamespacesEnabled, openaiPassthroughEnabled, resetForm: () => resetForm(),
  selectedErrorCodes,
  show: () => props.show, t, tempUnschedEnabled, tempUnschedRules, tlsFingerprintProfiles,
  vertexClientEmail, vertexLocation, vertexProjectId, vertexServiceAccountJson,
  webSearchEmulationMode,
})
const needsMixedChannelCheck = (platform: AccountPlatform) => platform === 'antigravity' || platform === 'anthropic'

const buildMixedChannelDetails = (resp?: CheckMixedChannelResponse) => {
  const details = resp?.details
  if (!details) {
    return null
  }
  return {
    groupName: details.group_name || 'Unknown',
    currentPlatform: details.current_platform || 'Unknown',
    otherPlatform: details.other_platform || 'Unknown'
  }
}

const clearMixedChannelDialog = () => {
  showMixedChannelWarning.value = false
  mixedChannelWarningDetails.value = null
  mixedChannelWarningRawMessage.value = ''
  mixedChannelWarningAction.value = null
}

const openMixedChannelDialog = (opts: {
  response?: CheckMixedChannelResponse
  message?: string
  onConfirm: () => Promise<void>
}) => {
  mixedChannelWarningDetails.value = buildMixedChannelDetails(opts.response)
  mixedChannelWarningRawMessage.value =
    opts.message || opts.response?.message || t('admin.accounts.failedToCreate')
  mixedChannelWarningAction.value = opts.onConfirm
  showMixedChannelWarning.value = true
}

const withAntigravityConfirmFlag = (payload: CreateAccountRequest): CreateAccountRequest => {
  if (needsMixedChannelCheck(payload.platform) && antigravityMixedChannelConfirmed.value) {
    return {
      ...payload,
      confirm_mixed_channel_risk: true
    }
  }
  const cloned = { ...payload }
  delete cloned.confirm_mixed_channel_risk
  return cloned
}

const ensureAntigravityMixedChannelConfirmed = async (onConfirm: () => Promise<void>): Promise<boolean> => {
  if (!needsMixedChannelCheck(form.platform)) {
    return true
  }
  if (antigravityMixedChannelConfirmed.value) {
    return true
  }

  try {
    const result = await checkMixedChannelRisk({
      platform: form.platform,
      group_ids: form.group_ids
    })
    if (!result.has_risk) {
      return true
    }
    openMixedChannelDialog({
      response: result,
      onConfirm: async () => {
        antigravityMixedChannelConfirmed.value = true
        await onConfirm()
      }
    })
    return false
  } catch (error: any) {
    appStore.showError(error.response?.data?.message || error.response?.data?.detail || t('admin.accounts.failedToCreate'))
    return false
  }
}

const submitCreateAccount = async (payload: CreateAccountRequest) => {
  submitting.value = true
  try {
    const account = await createAccount(withAntigravityConfirmFlag(payload))
    if (
      isUpstreamBillingProbeEligible(payload.platform, payload.type) &&
      payload.upstream_billing_probe_enabled === true
    ) {
      try {
        await probeUpstreamBilling(account.id)
      } catch {
        appStore.showWarning(t('admin.accounts.upstreamBilling.probeFailed'))
      }
    }
    appStore.showSuccess(t('admin.accounts.accountCreated'))
    emit('created')
    handleClose()
  } catch (error: any) {
    if (error.response?.status === 409 && error.response?.data?.error === 'mixed_channel_warning' && needsMixedChannelCheck(form.platform)) {
      openMixedChannelDialog({
        message: error.response?.data?.message,
        onConfirm: async () => {
          antigravityMixedChannelConfirmed.value = true
          await submitCreateAccount(payload)
        }
      })
      return
    }
    appStore.showError(error.response?.data?.message || error.response?.data?.detail || t('admin.accounts.failedToCreate'))
  } finally {
    submitting.value = false
  }
}

// Methods
const resetForm = () => {
  step.value = 1
  form.name = ''
  form.notes = ''
  form.platform = 'anthropic'
  form.type = 'oauth'
  form.credentials = {}
  form.proxy_id = null
  form.concurrency = 10
  form.load_factor = null
  form.priority = 1
  form.rate_multiplier = 1
  form.group_ids = []
  form.expires_at = null
  accountCategory.value = 'oauth-based'
  addMethod.value = 'oauth'
  apiKeyBaseUrl.value = 'https://api.anthropic.com'
  apiKeyValue.value = ''
  resetCNAccountEditorState()
  upstreamBillingAutoProbeEnabled.value = true
  autoDisableOnUpstreamInsufficientBalance.value = false
  editQuotaLimit.value = null
  editQuotaDailyLimit.value = null
  editQuotaWeeklyLimit.value = null
  editDailyResetMode.value = null
  editDailyResetHour.value = null
  editWeeklyResetMode.value = null
  editWeeklyResetDay.value = null
  editWeeklyResetHour.value = null
  editResetTimezone.value = null
  modelMappings.value = []
  openAICompactModelMappings.value = []
  modelRestrictionMode.value = 'whitelist'
  allowedModels.value = [...claudeModels] // Default fill related models

  antigravityModelRestrictionMode.value = 'mapping'
  antigravityWhitelistModels.value = []
  fetchAntigravityDefaultMappings().then(mappings => {
    antigravityModelMappings.value = [...mappings]
  })
  poolModeEnabled.value = false
  poolModeRetryCount.value = DEFAULT_POOL_MODE_RETRY_COUNT
  poolModeRetryStatusCodesInput.value = ''
  customErrorCodesEnabled.value = false
  selectedErrorCodes.value = []
  customErrorCodeInput.value = null
  headerOverrideEnabled.value = false
  headerOverrideRows.value = []
  grokOAuthCustomBaseUrlEnabled.value = false
  grokOAuthBaseUrl.value = ''
  interceptWarmupRequests.value = false
  autoPauseOnExpired.value = true
  openaiPassthroughEnabled.value = false
  openAIImageSizeTiers.value = [...OPENAI_IMAGE_SIZE_TIERS]
  openaiFlattenNamespacesEnabled.value = false
  openAILongContextBillingEnabled.value = false
  openAILongContextBillingTouched.value = false
  openAICompactMode.value = 'auto'
  openAIResponsesMode.value = 'auto'
  openAIEndpointCapabilities.value = ['chat_completions', 'embeddings', 'decisions']
  openAIForceImageAPIEnabled.value = false
  openaiOAuthResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
  openaiAPIKeyResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
  codexFingerprintMode.value = 'off'
  codexPrewarmContinuationEnabled.value = false
  codexThinkingTagNormalizationEnabled.value = false
  codexCLIOnlyEnabled.value = false
  codexCLIOnlyAppServerEnabled.value = false
  anthropicPassthroughEnabled.value = false
  anthropicAPIKeyAuthScheme.value = 'x_api_key'
  webSearchEmulationMode.value = 'default'
  // Reset quota control state
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
  allowOverages.value = false
  antigravityAccountType.value = 'oauth'
  antigravityProjectId.value = ''
  upstreamBaseUrl.value = ''
  upstreamApiKey.value = ''
  vertexServiceAccountJson.value = ''
  vertexProjectId.value = ''
  vertexClientEmail.value = ''
  vertexLocation.value = 'global'
  tempUnschedEnabled.value = false
  tempUnschedRules.value = []
  geminiOAuthType.value = 'code_assist'
  geminiTierGoogleOne.value = 'google_one_free'
  geminiTierGcp.value = 'gcp_standard'
  geminiTierAIStudio.value = 'aistudio_free'
  oauth.resetState()
  openaiOAuth.resetState()
  geminiOAuth.resetState()
  antigravityOAuth.resetState()
  grokOAuth.resetState()
  oauthFlowRef.value?.reset()
  antigravityMixedChannelConfirmed.value = false
  clearMixedChannelDialog()
}

const handleClose = () => {
  antigravityMixedChannelConfirmed.value = false
  clearMixedChannelDialog()
  emit('close')
}

const buildOpenAIExtra = (base?: Record<string, unknown>): Record<string, unknown> | undefined =>
  buildCreateOpenAIExtra({
    form,
    openAIImageSizeTiers, accountCategory, openaiOAuthResponsesWebSocketV2Mode,
    openaiAPIKeyResponsesWebSocketV2Mode, openaiPassthroughEnabled, openaiFlattenNamespacesEnabled,
    codexPrewarmContinuationEnabled, codexFingerprintMode, customBaseUrlEnabled,
    customBaseUrl, codexThinkingTagNormalizationEnabled, openAILongContextBillingEnabled,
    codexCLIOnlyEnabled, codexCLIOnlyAppServerEnabled, openAICompactMode,
    openAITextGenerationCapabilityEnabled, openAIResponsesMode, openAIForceImageAPIEnabled,
  }, base)

const buildOpenAICodexImportExtra = (): Record<string, unknown> | undefined => {
  const extra = buildOpenAIExtra()
  if (!extra) {
    return undefined
  }
  if (!openAILongContextBillingTouched.value) {
    delete extra.openai_long_context_billing_enabled
  }
  return Object.keys(extra).length > 0 ? extra : undefined
}

const buildAnthropicExtra = (base?: Record<string, unknown>): Record<string, unknown> | undefined =>
  buildAPIKeySearchExtra(base, form.platform, accountCategory.value, webSearchEmulationMode.value,
    anthropicPassthroughEnabled.value, anthropicAPIKeyAuthScheme.value)

const buildUpstreamInsufficientBalanceExtra = (
  base?: Record<string, unknown>
): Record<string, unknown> | undefined => {
  if (accountCategory.value !== 'apikey') {
    return base
  }
  const extra: Record<string, unknown> = { ...(base || {}) }
  if (autoDisableOnUpstreamInsufficientBalance.value) {
    extra.auto_disable_on_upstream_insufficient_balance = true
  } else {
    delete extra.auto_disable_on_upstream_insufficient_balance
  }
  return Object.keys(extra).length > 0 ? extra : undefined
}

// Helper function to create account with mixed channel warning handling
const doCreateAccount = async (payload: CreateAccountRequest) => {
  const canContinue = await ensureAntigravityMixedChannelConfirmed(async () => {
    await submitCreateAccount(payload)
  })
  if (!canContinue) {
    return
  }
  await submitCreateAccount(payload)
}

// Handle mixed channel warning confirmation
const handleMixedChannelConfirm = async () => {
  const action = mixedChannelWarningAction.value
  if (!action) {
    clearMixedChannelDialog()
    return
  }
  clearMixedChannelDialog()
  submitting.value = true
  try {
    await action()
  } finally {
    submitting.value = false
  }
}

const handleMixedChannelCancel = () => {
  clearMixedChannelDialog()
}

const applyVertexServiceAccountJson = (value: string) => {
  const raw = value.trim()
  if (!raw) {
    vertexProjectId.value = ''
    vertexClientEmail.value = ''
    return false
  }
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>
    const projectId = typeof parsed.project_id === 'string' ? parsed.project_id.trim() : ''
    const clientEmail = typeof parsed.client_email === 'string' ? parsed.client_email.trim() : ''
    const privateKey = typeof parsed.private_key === 'string' ? parsed.private_key.trim() : ''
    if (!projectId || !clientEmail || !privateKey) {
      appStore.showError(t('admin.accounts.vertexSaJsonMissingFields'))
      return false
    }
    vertexProjectId.value = projectId
    vertexClientEmail.value = clientEmail
    vertexServiceAccountJson.value = JSON.stringify(parsed)
    return true
  } catch {
    appStore.showError(t('admin.accounts.vertexSaJsonInvalid'))
    return false
  }
}

const parseVertexServiceAccountJson = () => applyVertexServiceAccountJson(vertexServiceAccountJson.value)

const handleVertexServiceAccountFile = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  try {
    applyVertexServiceAccountJson(await file.text())
  } finally {
    input.value = ''
  }
}

const handleVertexServiceAccountDrop = async (event: DragEvent) => {
  vertexServiceAccountDragActive.value = false
  const file = event.dataTransfer?.files?.[0]
  if (!file) return
  applyVertexServiceAccountJson(await file.text())
}

const handleSubmit = async () => {
  // For OAuth-based type, handle OAuth flow (goes to step 2)
  if (isOAuthFlow.value) {
    if (!isGrokSSOInputMethod.value && !form.name.trim()) {
      appStore.showError(t('admin.accounts.pleaseEnterAccountName'))
      return
    }
    const canContinue = await ensureAntigravityMixedChannelConfirmed(async () => {
      step.value = 2
    })
    if (!canContinue) {
      return
    }
    step.value = 2
    return
  }

  // For Bedrock type, create directly
  if (form.platform === 'anthropic' && accountCategory.value === 'bedrock') {
    if (!form.name.trim()) {
      appStore.showError(t('admin.accounts.pleaseEnterAccountName'))
      return
    }

    const credentials: Record<string, unknown> = {
      auth_mode: bedrockAuthMode.value,
      aws_region: bedrockRegion.value.trim() || 'us-east-1',
    }

    if (bedrockAuthMode.value === 'sigv4') {
      if (!bedrockAccessKeyId.value.trim()) {
        appStore.showError(t('admin.accounts.bedrockAccessKeyIdRequired'))
        return
      }
      if (!bedrockSecretAccessKey.value.trim()) {
        appStore.showError(t('admin.accounts.bedrockSecretAccessKeyRequired'))
        return
      }
      credentials.aws_access_key_id = bedrockAccessKeyId.value.trim()
      credentials.aws_secret_access_key = bedrockSecretAccessKey.value.trim()
      if (bedrockSessionToken.value.trim()) {
        credentials.aws_session_token = bedrockSessionToken.value.trim()
      }
    } else {
      if (!bedrockApiKeyValue.value.trim()) {
        appStore.showError(t('admin.accounts.bedrockApiKeyRequired'))
        return
      }
      credentials.api_key = bedrockApiKeyValue.value.trim()
    }

    if (bedrockForceGlobal.value) {
      credentials.aws_force_global = 'true'
    }

    // Model mapping
    const modelMapping = buildModelMappingObject(
      modelRestrictionMode.value, allowedModels.value, modelMappings.value
    )
    if (modelMapping) {
      credentials.model_mapping = modelMapping
    }

    // Pool mode
    if (poolModeEnabled.value) {
      credentials.pool_mode = true
      credentials.pool_mode_retry_count = normalizePoolModeRetryCount(poolModeRetryCount.value)
      const parsedRetryStatusCodes = parsePoolModeRetryStatusCodes(poolModeRetryStatusCodesInput.value)
      if (parsedRetryStatusCodes.length > 0) {
        credentials.pool_mode_retry_status_codes = parsedRetryStatusCodes
      }
    }

    applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')

    await createAccountAndFinish('anthropic', 'bedrock' as AccountType, credentials)
    return
  }

  // For Antigravity upstream type, create directly
  if (form.platform === 'antigravity' && antigravityAccountType.value === 'upstream') {
    if (!form.name.trim()) {
      appStore.showError(t('admin.accounts.pleaseEnterAccountName'))
      return
    }
    if (!upstreamBaseUrl.value.trim()) {
      appStore.showError(t('admin.accounts.upstream.pleaseEnterBaseUrl'))
      return
    }
    if (!upstreamApiKey.value.trim()) {
      appStore.showError(t('admin.accounts.upstream.pleaseEnterApiKey'))
      return
    }

    // Build upstream credentials (and optional model restriction)
    const credentials: Record<string, unknown> = {
      base_url: upstreamBaseUrl.value.trim(),
      api_key: upstreamApiKey.value.trim()
    }

    // Antigravity 只使用映射模式
    const antigravityModelMapping = buildModelMappingObject(
      'mapping',
      [],
      antigravityModelMappings.value
    )
    if (antigravityModelMapping) {
      credentials.model_mapping = antigravityModelMapping
    }

    applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')

    const extra = buildAntigravityExtra()
    await createAccountAndFinish(form.platform, 'apikey', credentials, extra)
    return
  }

  if ((form.platform === 'gemini' || form.platform === 'anthropic') && accountCategory.value === 'service_account') {
    if (!form.name.trim()) {
      appStore.showError(t('admin.accounts.pleaseEnterAccountName'))
      return
    }
    if (!parseVertexServiceAccountJson()) {
      return
    }
    if (!vertexLocation.value.trim()) {
      appStore.showError(t('admin.accounts.vertexLocationRequired'))
      return
    }
    const credentials: Record<string, unknown> = {
      service_account_json: vertexServiceAccountJson.value.trim(),
      project_id: vertexProjectId.value.trim(),
      client_email: vertexClientEmail.value.trim(),
      location: vertexLocation.value.trim(),
      tier_id: 'vertex'
    }
    await createAccountAndFinish(form.platform, 'service_account' as AccountType, credentials)
    return
  }

  // For apikey type, create directly
  if (!apiKeyValue.value.trim()) {
    appStore.showError(t('admin.accounts.pleaseEnterApiKey'))
    return
  }
  // 多模态平台没有内置上游端点，Base URL 必须由管理员填写
  if (form.platform === 'multimodal' && !apiKeyBaseUrl.value.trim()) {
    appStore.showError(t('admin.accounts.pleaseEnterBaseUrl'))
    return
  }

  // Determine default base URL based on platform
    const defaultBaseUrl = defaultAPIKeyBaseURL(form.platform)

  // Build credentials with optional model mapping
    const credentials: Record<string, unknown> = {
    base_url: apiKeyBaseUrl.value.trim() || defaultBaseUrl,
    api_key: apiKeyValue.value.trim()
    }
  applyCreateCNProviderCredentials(credentials, {
    platform: form.platform, cnAccountMode: cnAccountMode.value, cnAPIProtocol: cnAPIProtocol.value,
    adaptiveBaseURLs: cnAdaptiveBaseURLs.value, openCodeAccountMode: openCodeAccountMode.value,
    openCodeProtocolRules: openCodeProtocolRules.value, zhipuOrganization: zhipuOrganization.value,
    zhipuProject: zhipuProject.value,
  })
  if (form.platform === 'gemini') {
    credentials.tier_id = geminiTierAIStudio.value
  }

  // Add model mapping if configured（OpenAI 开启自动透传时不应用）
  if (!isOpenAIModelRestrictionDisabled.value) {
    const modelMapping = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value)
    if (modelMapping) {
      credentials.model_mapping = modelMapping
    }
  }
  if (form.platform === 'openai') {
    applyOpenAIEndpointCapabilities(credentials)
    const compactModelMapping = buildOpenAICompactModelMapping()
    if (compactModelMapping) {
      credentials.compact_model_mapping = compactModelMapping
    }
  }

  // Add pool mode if enabled
  if (poolModeEnabled.value) {
    credentials.pool_mode = true
    credentials.pool_mode_retry_count = normalizePoolModeRetryCount(poolModeRetryCount.value)
    const parsedRetryStatusCodes = parsePoolModeRetryStatusCodes(poolModeRetryStatusCodesInput.value)
    if (parsedRetryStatusCodes.length > 0) {
      credentials.pool_mode_retry_status_codes = parsedRetryStatusCodes
    }
  }

  // Add custom error codes if enabled
  if (customErrorCodesEnabled.value) {
    credentials.custom_error_codes_enabled = true
    credentials.custom_error_codes = [...selectedErrorCodes.value]
  }

  // Add header override if enabled (anthropic/openai/grok apikey)
  if (isHeaderOverrideCapable(form.platform, 'apikey')) {
    if (headerOverrideEnabled.value) {
      const headerError = validateHeaderOverrideRows(headerOverrideRows.value)
      if (headerError) {
        appStore.showError(t(`admin.accounts.headerOverride.${headerError}`))
        return
      }
    }
    applyHeaderOverride(credentials, headerOverrideEnabled.value, headerOverrideRows.value, 'create')
  }

  applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
  if (!applyTempUnschedConfig(credentials)) {
    return
  }

  form.credentials = credentials
  const extra = buildUpstreamInsufficientBalanceExtra(buildAnthropicExtra(buildOpenAIExtra()))

  await doCreateAccount({
    ...form,
    group_ids: form.group_ids,
    extra,
    upstream_billing_probe_enabled:
      isUpstreamBillingProbeEligible(form.platform, form.type)
        ? upstreamBillingAutoProbeEnabled.value
        : undefined,
    auto_pause_on_expired: autoPauseOnExpired.value
  })
}

const {
  createAccountAndFinish,
  goBackToBasicInfo,
  handleCookieAuth,
  handleExchangeCode,
  handleGenerateUrl,
  handleGrokImportSSO,
  handleOpenAIImportCodexPAT,
  handleOpenAIImportCodexSession,
  handleOpenAIValidateMobileRT,
  handleValidateRefreshToken,
  handleValidateSessionToken,
} = useCreateAccountOAuthActions({
  addMethod, allowedModels, antigravityModelMappings, antigravityOAuth, antigravityProjectId,
  apiKeyBaseUrl, applyGrokOAuthUpstreamConfig, applyOpenAIEndpointCapabilities,
  applyTempUnschedConfig, autoPauseOnExpired, baseRpm, buildAntigravityExtra,
  buildOpenAICodexImportExtra, buildOpenAICompactModelMapping, buildOpenAIExtra,
  cacheTTLOverrideEnabled, cacheTTLOverrideTarget, customBaseUrl, customBaseUrlEnabled,
  doCreateAccount, editDailyResetHour, editDailyResetMode, editQuotaDailyLimit,
  editQuotaLimit, editQuotaWeeklyLimit, editResetTimezone, editWeeklyResetDay,
  editWeeklyResetHour, editWeeklyResetMode, form, geminiOAuth, geminiOAuthType,
  geminiSelectedTier, grokOAuth, handleClose, interceptWarmupRequests,
  isOpenAIModelRestrictionDisabled, maxSessions, modelMappings, modelRestrictionMode,
  notifications, oauth, oauthFlowRef, onCreated: () => emit('created'),
  openaiOAuth, rpmLimitEnabled, rpmStickyBuffer, rpmStrategy, sessionIdMaskingEnabled,
  sessionIdleTimeout, sessionLimitEnabled, step, t, tempUnschedEnabled, tempUnschedRules,
  tlsFingerprintEnabled, tlsFingerprintProfileId, userMsgQueueMode,
  validateGrokOAuthUpstreamConfig, windowCostEnabled, windowCostLimit,
  windowCostStickyReserve, withAntigravityConfirmFlag, writeQuotaNotifyToExtra,
})
const createAccountPlatformContext = {
  VERTEX_LOCATION_OPTIONS, accountCategory, addAntigravityModelMapping, addAntigravityPresetMapping,
  antigravityAccountType, antigravityModelMappings, antigravityPresetMappings, antigravityProjectId,
  form, geminiAIStudioOAuthEnabled, geminiHelpLinks, geminiOAuthType, geminiTierAIStudio,
  geminiTierGcp, geminiTierGoogleOne, getAntigravityModelMappingKey, handleSelectGeminiOAuthType,
  handleVertexServiceAccountDrop, handleVertexServiceAccountFile, isGrokSSOInputMethod,
  isValidWildcardPattern, removeAntigravityModelMapping, showAdvancedOAuth, showGeminiHelpDialog,
  t, upstreamApiKey, upstreamBaseUrl, vertexClientEmail, vertexLocation, vertexProjectId,
  vertexServiceAccountDragActive, vertexServiceAccountFileInput,
} satisfies CreateAccountPlatformContext

const createAccountCredentialContext = {
  DEFAULT_POOL_MODE_RETRY_COUNT, DEFAULT_POOL_MODE_RETRY_STATUS_CODES, MAX_POOL_MODE_RETRY_COUNT,
  accountCategory, addCustomErrorCode, addMethod, addModelMapping, addPresetMapping, allowedModels,
  apiKeyBaseUrl, apiKeyHint, apiKeyValue, autoDisableOnUpstreamInsufficientBalance, baseUrlHint,
  cnAccountMode, cnAPIProtocol, cnAdaptiveBaseURLs,
  bedrockAccessKeyId, bedrockApiKeyValue, bedrockAuthMode, bedrockForceGlobal, bedrockPresets,
  bedrockRegion, bedrockSecretAccessKey, bedrockSessionToken, commonErrorCodes,
  customErrorCodeInput, customErrorCodesEnabled, editDailyResetHour, editDailyResetMode,
  editQuotaDailyLimit, editQuotaLimit, editQuotaWeeklyLimit, editResetTimezone, editWeeklyResetDay,
  editWeeklyResetHour, editWeeklyResetMode, form, geminiTierAIStudio, getModelMappingKey,
  grokOAuthBaseUrl, grokOAuthCustomBaseUrlEnabled, headerOverrideEnabled, headerOverrideRows,
  isHeaderOverrideCapable, isOAuthFlow, isOpenAIModelRestrictionDisabled, modelMappings,
  modelRestrictionMode, poolModeEnabled, poolModeRetryCount, poolModeRetryStatusCodesInput,
  openCodeAccountMode, openCodeProtocolRules, zhipuOrganization, zhipuProject,
  presetMappings, quotaNotifyGlobalEnabled, quotaNotifyState, removeErrorCode, removeModelMapping,
  selectedErrorCodes, syncPreviewCredentials, t, toggleErrorCode, upstreamBillingAutoProbeEnabled,
} satisfies CreateAccountCredentialContext

const createAccountAdvancedContext = {
  accountCategory, addOpenAICompactModelMapping, addTempUnschedRule, allowOverages,
  anthropicAPIKeyAuthScheme, anthropicPassthroughEnabled, isSimpleMode, autoPauseOnExpired, baseRpm,
    cacheTTLOverrideEnabled, cacheTTLOverrideTarget, codexFingerprintMode,
    codexFingerprintModeOptions, codexPrewarmContinuationEnabled, codexThinkingTagNormalizationEnabled,
    codexCLIOnlyAppServerEnabled, codexCLIOnlyEnabled,
  customBaseUrl, customBaseUrlEnabled, expiresAtInput, form, getOpenAICompactModelMappingKey,
  getTempUnschedRuleKey, groups: availableGroups, interceptWarmupRequests, maxSessions,
  mixedScheduling, moveTempUnschedRule, openAICompactMode, openAICompactModeOptions,
  openAICompactModelMappings, openAIEndpointCapabilities, openAIEndpointCapabilityOptions,
  openAIForceImageAPIEnabled, openAILongContextBillingEnabled, openAIResponsesMode,
  openAIResponsesModeOptions, openAITextGenerationCapabilityEnabled, openAIWSModeConcurrencyHintKey,
  openAIWSModeOptions, openaiPassthroughEnabled, openaiFlattenNamespacesEnabled,
  openaiResponsesWebSocketV2Mode,
  proxies: availableProxies, removeOpenAICompactModelMapping, removeTempUnschedRule,
  rpmLimitEnabled, rpmStickyBuffer, rpmStrategy, sessionIdMaskingEnabled, sessionIdleTimeout,
  sessionLimitEnabled, t, tempUnschedEnabled, tempUnschedPresets, tempUnschedRules,
  tlsFingerprintEnabled, tlsFingerprintProfileId, tlsFingerprintProfiles,
  toggleOpenAIEndpointCapability, toggleOpenAILongContextBilling, umqModeOptions, userMsgQueueMode,
  webSearchEmulationMode, webSearchGlobalEnabled, windowCostEnabled, windowCostLimit,
  windowCostStickyReserve,
} satisfies CreateAccountAdvancedContext

const createAccountFooterContext = {
  canExchangeCode, currentOAuthLoading, goBackToBasicInfo, handleClose, handleExchangeCode,
  isManualInputMethod, isOAuthFlow, step, submitting, t,
} satisfies CreateAccountFooterContext

const createAccountStepIndicatorContext = {
  isOAuthFlow, oauthStepTitle, step, t,
} satisfies CreateAccountStepIndicatorContext

const geminiAccountHelpContext = {
  geminiHelpLinks, geminiQuotaDocs, showGeminiHelpDialog, t,
} satisfies GeminiAccountHelpContext
</script>
