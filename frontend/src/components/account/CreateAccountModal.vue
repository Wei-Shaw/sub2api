<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.createAccount')"
    width="wide"
    @close="handleClose"
  >
    <!-- Step Indicator (shown from Step 2 onwards) -->
    <div v-if="step > 1" class="mb-6 flex items-center justify-center">
      <div class="flex items-center space-x-4">
        <div class="flex items-center">
          <div
            class="flex h-8 w-8 items-center justify-center rounded-full text-sm font-semibold bg-primary-500 text-white"
          >
            1
          </div>
          <span class="ml-2 text-sm font-medium text-gray-700 dark:text-gray-300">{{
            t('admin.accounts.selectPlatform')
          }}</span>
        </div>
        <div class="h-0.5 w-8 bg-gray-300 dark:bg-dark-600" />
        <div class="flex items-center">
          <div
            :class="[
              'flex h-8 w-8 items-center justify-center rounded-full text-sm font-semibold',
              step >= 2 ? 'bg-primary-500 text-white' : 'bg-gray-200 text-gray-500 dark:bg-dark-600'
            ]"
          >
            2
          </div>
          <span class="ml-2 text-sm font-medium text-gray-700 dark:text-gray-300">{{
            t('admin.accounts.accountDetails')
          }}</span>
        </div>
        <template v-if="isOAuthFlow">
          <div class="h-0.5 w-8 bg-gray-300 dark:bg-dark-600" />
          <div class="flex items-center">
            <div
              :class="[
                'flex h-8 w-8 items-center justify-center rounded-full text-sm font-semibold',
                step >= 3 ? 'bg-primary-500 text-white' : 'bg-gray-200 text-gray-500 dark:bg-dark-600'
              ]"
            >
              3
            </div>
            <span class="ml-2 text-sm font-medium text-gray-700 dark:text-gray-300">{{
              oauthStepTitle
            }}</span>
          </div>
        </template>
      </div>
    </div>

    <!-- Step 1: Platform & Type Selection -->
    <div v-if="step === 1" class="space-y-5">
      <!-- Platform Selection -->
      <div>
        <label class="input-label">{{ t('admin.accounts.platform') }}</label>
        <div class="mt-2 flex rounded-lg bg-gray-100 p-1 dark:bg-dark-700" data-tour="account-form-platform">
          <button
            v-for="pp in allPlatforms"
            :key="pp.platform"
            type="button"
            @click="form.platform = pp.platform"
            :class="[
              'flex flex-1 items-center justify-center gap-2 rounded-md px-4 py-2.5 text-sm font-medium transition-all',
              form.platform === pp.platform
                ? 'bg-white shadow-sm dark:bg-dark-600'
                : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-200'
            ]"
            :style="form.platform === pp.platform && pp.theme_color ? { color: pp.theme_color } : {}"
          >
            <PlatformIcon :platform="pp.platform" :icon-svg="pp.icon_svg" size="sm" />
            {{ pp.display_name }}
          </button>
        </div>
      </div>

      <!-- Dynamic Account Type Selection (grouped by category) -->
      <div v-if="groupedTypeCards.length > 0">
        <label class="input-label">{{ t('admin.accounts.accountType') }}</label>
        <div
          class="mt-2 grid gap-3"
          :class="groupedTypeCards.length <= 2 ? 'grid-cols-2' : 'grid-cols-3'"
          data-tour="account-form-type"
        >
          <button
            v-for="group in groupedTypeCards"
            :key="group.category"
            type="button"
            @click="onCategoryCardSelect(group)"
            :class="[
              'flex items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
              accountCategory === group.category
                ? 'bg-primary-50 dark:bg-primary-900/20'
                : 'border-gray-200 hover:border-gray-300 dark:border-dark-600 dark:hover:border-dark-500'
            ]"
            :style="accountCategory === group.category && currentPlatformDecl?.theme_color ? { borderColor: currentPlatformDecl.theme_color } : {}"
          >
            <div
              :class="[
                'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg transition-colors',
                accountCategory === group.category
                  ? 'text-white'
                  : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
              ]"
              :style="accountCategory === group.category && currentPlatformDecl?.theme_color ? { backgroundColor: currentPlatformDecl.theme_color } : {}"
            >
              <PlatformIcon v-if="group.iconSvg" :icon-svg="group.iconSvg" size="sm" />
              <Icon v-else name="key" size="sm" />
            </div>
            <div class="min-w-0">
              <span class="block text-sm font-medium text-gray-900 dark:text-white">{{ group.displayName }}</span>
              <span v-if="group.description" class="block truncate text-xs text-gray-500 dark:text-gray-400">{{ group.description }}</span>
            </div>
            <span
              v-if="group.badgeLabel"
              class="ml-auto shrink-0 rounded-full px-2 py-0.5 text-xs font-medium"
              :class="accountCategory === group.category ? 'bg-white/80 text-gray-700' : 'bg-gray-100 text-gray-600 dark:bg-dark-600 dark:text-gray-400'"
            >{{ group.badgeLabel }}</span>
          </button>
        </div>
      </div>
    </div>

    <!-- Step 2: Plugin Form -->
    <div v-else-if="step === 2" class="space-y-5">
      <!-- Platform/type badge (read-only) + back link -->
      <div class="flex items-center justify-between rounded-lg bg-gray-50 px-4 py-2.5 dark:bg-dark-700">
        <div class="flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-300">
          <PlatformIcon
            :platform="form.platform"
            :icon-svg="currentPlatformDecl?.icon_svg"
            size="sm"
          />
          <span>{{ currentPlatformDecl?.display_name }}</span>
          <span class="text-gray-400">&mdash;</span>
          <span>{{ selectedGroupDisplayName }}</span>
        </div>
        <button
          type="button"
          class="text-sm text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
          @click="goBackToStep1"
        >
          {{ t('common.change') }}
        </button>
      </div>
      <!-- Plugin form -->
      <component
        v-if="resolvedFormComponent"
        :is="resolvedFormComponent"
        ref="platformFormRef"
        :context="platformFormContext"
        v-bind="platformFormExtraProps"
      />
      <div v-else class="text-center text-gray-500 py-8">
        {{ t('admin.accounts.loadingForm') }}
      </div>
    </div>

    <!-- Step 3: OAuth Authorization -->
    <div v-else-if="step === 3" class="space-y-5">
      <OAuthAuthorizationFlow
        ref="oauthFlowRef"
        :add-method="oauthAddMethod"
        :auth-url="oauthState.authUrl"
        :session-id="oauthState.sessionId"
        :loading="oauthState.loading"
        :error="oauthState.error"
        :show-help="oauthCfg?.showHelp ?? false"
        :show-proxy-warning="(oauthCfg?.showProxyWarning ?? true) && !!form.proxy_id"
        :allow-multiple="oauthCfg?.allowMultiple ?? false"
        :show-cookie-option="oauthCfg?.showCookieOption ?? false"
        :show-refresh-token-option="oauthCfg?.showRefreshTokenOption ?? false"
        :show-mobile-refresh-token-option="oauthCfg?.showMobileRefreshTokenOption ?? false"
        :show-session-token-option="oauthCfg?.showSessionTokenOption ?? false"
        :show-access-token-option="oauthCfg?.showAccessTokenOption ?? false"
        :show-codex-session-import-option="oauthCfg?.showCodexSessionImportOption ?? false"
        :show-important-notice="oauthCfg?.showImportantNotice ?? false"
        :show-state-warning="oauthCfg?.showStateWarning ?? false"
        :i18n-prefix="oauthCfg?.i18nPrefix ?? ''"
        :platform="form.platform"
        :show-project-id="oauthCfg?.showProjectId ?? false"
        @generate-url="handleGenerateUrl"
        @cookie-auth="handleCookieAuth"
        @validate-refresh-token="handleRefreshToken"
        @validate-mobile-refresh-token="handleMobileRefreshToken"
        @validate-session-token="handleSessionToken"
        @import-codex-session="handleCodexSessionImport"
      />
    </div>

    <!-- Footer -->
    <template #footer>
      <!-- Step 1: Cancel + Next -->
      <div v-if="step === 1" class="flex justify-end gap-3">
        <button @click="handleClose" type="button" class="btn btn-secondary">{{ t('common.cancel') }}</button>
        <button
          type="button"
          :disabled="formLoading || !selectedAccountTypeId"
          class="btn btn-primary"
          data-tour="account-form-submit"
          @click="goToStep2"
        >
          <svg v-if="formLoading" class="-ml-1 mr-2 h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
          </svg>
          {{ t('common.next') }}
        </button>
      </div>
      <!-- Step 2: Back + Create/Next -->
      <div v-else-if="step === 2" class="flex justify-between gap-3">
        <button type="button" class="btn btn-secondary" @click="goBackToStep1">{{ t('common.back') }}</button>
        <button
          type="button"
          :disabled="submitting"
          class="btn btn-primary"
          @click="handleSubmit"
        >
          <svg v-if="submitting" class="-ml-1 mr-2 h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
          </svg>
          {{ isOAuthFlow ? t('common.next') : submitting ? t('admin.accounts.creating') : t('common.create') }}
        </button>
      </div>
      <!-- Step 3: Back + Exchange code -->
      <div v-else class="flex justify-between gap-3">
        <button type="button" class="btn btn-secondary" @click="goBackToStep2">{{ t('common.back') }}</button>
        <button v-if="isManualInputMethod" type="button" :disabled="!canExchangeCode" class="btn btn-primary" @click="handleExchangeCode">
          <svg v-if="oauthState.loading" class="-ml-1 mr-2 h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
          </svg>
          {{ oauthState.loading ? t('admin.accounts.oauth.verifying') : t('admin.accounts.oauth.completeAuth') }}
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
import { ref, reactive, computed, watch, shallowRef, onMounted, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { adminAPI } from '@/api/admin'
import type {
  Proxy, AdminGroup, AccountPlatform, AccountType,
  CheckMixedChannelResponse, CreateAccountRequest,
  CodexSessionImportMessage,
  OpenAICompactMode,
  OpenAIResponsesMode
} from '@/types'
import { BaseDialog, ConfirmDialog } from '@sub2api/plugin-sdk'
import Icon from '@/components/icons/Icon.vue'
import ProxySelector from '@/components/common/ProxySelector.vue'
import ProxyAdBanner from '@/components/common/ProxyAdBanner.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'
import ModelWhitelistSelector from '@/components/account/ModelWhitelistSelector.vue'
import QuotaLimitCard from '@/components/account/QuotaLimitCard.vue'
import { applyInterceptWarmup } from '@/components/account/credentialsBuilder'
import { formatDateTimeLocalInput, parseDateTimeLocalInput } from '@/utils/format'
import { createStableObjectKeyResolver } from '@/utils/stableObjectKey'
import { VERTEX_LOCATION_OPTIONS } from '@/constants/account'
import {
  OPENAI_WS_MODE_CTX_POOL,
  OPENAI_WS_MODE_OFF,
  OPENAI_WS_MODE_PASSTHROUGH,
  isOpenAIWSModeEnabled,
  resolveOpenAIWSModeConcurrencyHintKey,
  type OpenAIWSMode
} from '@/utils/openaiWsMode'
import OAuthAuthorizationFlow from './OAuthAuthorizationFlow.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import { usePlatforms } from '@/composables/usePlatforms'
import { resolveFormComponentAsync } from './forms/platformFormRegistry'
import type {
  CommonAccountFields,
  PlatformFormContext, PlatformFormExposed,
  OAuthFlowConfig, OAuthComposableState,
  AddMethod, AuthInputMethod,
} from './forms/types'
import { extractApiErrorMessage } from '@/utils/apiError'
import { applyQuotaToExtra } from '@sub2api/plugin-sdk'

// ---------------------------------------------------------------------------
// OAuthAuthorizationFlow exposed interface
// ---------------------------------------------------------------------------
interface OAuthFlowExposed {
  authCode: string
  oauthState: string
  projectId: string
  sessionKey: string
  refreshToken: string
  sessionToken: string
  codexSession: string
  inputMethod: AuthInputMethod
  reset: () => void
}

// ---------------------------------------------------------------------------
// Props / Emits
// ---------------------------------------------------------------------------
interface Props {
  show: boolean
  proxies: Proxy[]
  groups: AdminGroup[]
}
const props = defineProps<Props>()
const emit = defineEmits<{ close: []; created: [] }>()

// ---------------------------------------------------------------------------
// Stores
// ---------------------------------------------------------------------------
const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

// ---------------------------------------------------------------------------
// Platform declarations
// ---------------------------------------------------------------------------
const { platforms, fetchPlatforms, getPlatformDecl } = usePlatforms()

const BUILTIN_PLATFORMS = new Set(['anthropic', 'openai', 'gemini', 'antigravity'])

const BUILTIN_PLATFORM_FALLBACKS: {
  platform: string; display_name: string; icon_svg: string
  theme_color: string; sort_order: number; account_types: never[]; plugin_name: string
}[] = [
  { platform: 'anthropic', display_name: 'Anthropic', icon_svg: '', theme_color: '#ea580c', sort_order: 1, account_types: [], plugin_name: '' },
  { platform: 'openai', display_name: 'OpenAI', icon_svg: '', theme_color: '#10b981', sort_order: 2, account_types: [], plugin_name: '' },
  { platform: 'gemini', display_name: 'Gemini', icon_svg: '', theme_color: '#2563eb', sort_order: 3, account_types: [], plugin_name: '' },
  { platform: 'antigravity', display_name: 'Antigravity', icon_svg: '', theme_color: '#7c3aed', sort_order: 4, account_types: [], plugin_name: '' },
]

const allPlatforms = computed(() => {
  const fromApi = [...platforms.value].sort((a, b) => a.sort_order - b.sort_order)
  return fromApi.length > 0 ? fromApi : BUILTIN_PLATFORM_FALLBACKS
})
const currentPlatformDecl = computed(() => getPlatformDecl(form.platform))

// ---------------------------------------------------------------------------
// Core state
// ---------------------------------------------------------------------------
const step = ref(1)
const submitting = ref(false)
const formLoading = ref(false)
const apiKeyBaseUrl = ref('https://api.anthropic.com')
const apiKeyValue = ref('')
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
const DEFAULT_POOL_MODE_RETRY_COUNT = 3
const MAX_POOL_MODE_RETRY_COUNT = 10
const DEFAULT_POOL_MODE_RETRY_STATUS_CODES = [401, 403, 429]
const poolModeEnabled = ref(false)
const poolModeRetryCount = ref(DEFAULT_POOL_MODE_RETRY_COUNT)
const poolModeRetryStatusCodesInput = ref('')

function parsePoolModeRetryStatusCodes(input: string): number[] {
  if (!input || !input.trim()) return []
  const seen = new Set<number>()
  const out: number[] = []
  for (const token of input.split(/[,\s]+/)) {
    const trimmed = token.trim()
    if (!trimmed) continue
    const n = Number(trimmed)
    if (!Number.isFinite(n) || !Number.isInteger(n)) continue
    if (n < 100 || n > 599) continue
    if (seen.has(n)) continue
    seen.add(n)
    out.push(n)
  }
  return out.sort((a, b) => a - b)
}
const customErrorCodesEnabled = ref(false)
const selectedErrorCodes = ref<number[]>([])
const customErrorCodeInput = ref<number | null>(null)
const interceptWarmupRequests = ref(false)
const autoPauseOnExpired = ref(true)
const openaiPassthroughEnabled = ref(false)
const openAICompactMode = ref<OpenAICompactMode>('auto')
const openAIResponsesMode = ref<OpenAIResponsesMode>('auto')
const openaiOAuthResponsesWebSocketV2Mode = ref<OpenAIWSMode>(OPENAI_WS_MODE_OFF)
const openaiAPIKeyResponsesWebSocketV2Mode = ref<OpenAIWSMode>(OPENAI_WS_MODE_OFF)
const codexCLIOnlyEnabled = ref(false)
const anthropicPassthroughEnabled = ref(false)
const webSearchEmulationMode = ref('default')
const webSearchGlobalEnabled = ref(false)
const {
  globalEnabled: quotaNotifyGlobalEnabled,
  state: quotaNotifyState,
  loadGlobalState: loadQuotaNotifyGlobal,
  writeToExtra: writeQuotaNotifyToExtra,
} = useQuotaNotifyState()

const accountCategory = ref<string>('oauth-based')
const addMethod = ref<AddMethod>('oauth')

const form = reactive({
  platform: 'anthropic' as AccountPlatform,
  type: 'oauth' as AccountType,
  credentials: {} as Record<string, unknown>,
  proxy_id: null as number | null,
  group_ids: [] as number[],
})

// Cached from plugin form payload before transitioning to OAuth Step 3
// Stores ALL common fields so they survive the v-if destruction of the plugin form component
const cachedCommonFields = ref<CommonAccountFields | null>(null)

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
const openAICompactModeOptions = computed(() => [
  { value: 'auto', label: t('admin.accounts.openai.compactModeAuto') },
  { value: 'force_on', label: t('admin.accounts.openai.compactModeForceOn') },
  { value: 'force_off', label: t('admin.accounts.openai.compactModeForceOff') }
])
const openAIResponsesModeOptions = computed(() => [
  { value: 'auto', label: t('admin.accounts.openai.responsesModeAuto') },
  { value: 'force_responses', label: t('admin.accounts.openai.responsesModeForceResponses') },
  { value: 'force_chat_completions', label: t('admin.accounts.openai.responsesModeForceChatCompletions') }
])

// ---------------------------------------------------------------------------
// Account type -> category mapping (data-driven, no platform name checks)
// ---------------------------------------------------------------------------
const OAUTH_TYPE_IDS = new Set(['oauth', 'setup-token'])

function typeIdToCategory(typeId: string): string {
  if (OAUTH_TYPE_IDS.has(typeId)) return 'oauth-based'
  return typeId
}

// ---------------------------------------------------------------------------
// Account type selection — grouped by category
// ---------------------------------------------------------------------------
const selectedAccountTypeId = ref<string>('oauth')

const groupedTypeCards = computed(() => {
  const decl = currentPlatformDecl.value
  if (!decl?.account_types.length) return []
  const map = new Map<string, {
    category: string; displayName: string; description: string
    badgeLabel: string; iconSvg?: string; sortOrder: number; defaultTypeId: string
  }>()
  for (const at of decl.account_types) {
    const cat = typeIdToCategory(at.type)
    const existing = map.get(cat)
    if (existing) {
      existing.displayName += ' / ' + at.display_name
      existing.description = ''
      existing.badgeLabel = ''
    } else {
      map.set(cat, {
        category: cat, displayName: at.display_name,
        description: at.description || '', badgeLabel: at.badge_label || '',
        iconSvg: at.icon_svg, sortOrder: at.sort_order, defaultTypeId: at.type,
      })
    }
  }
  return [...map.values()].sort((a, b) => a.sortOrder - b.sortOrder)
})

const selectedGroupDisplayName = computed(() =>
  groupedTypeCards.value.find(g => g.category === accountCategory.value)?.displayName ?? ''
)

// ---------------------------------------------------------------------------
// Dynamic platform form component (resolved async at Step 2 transition)
// ---------------------------------------------------------------------------
const platformFormRef = ref<PlatformFormExposed | null>(null)
const resolvedFormComponent = shallowRef<Component | null>(null)

const platformFormContext = computed<PlatformFormContext>(() => ({
  accountCategory: accountCategory.value,
  accountTypeId: selectedAccountTypeId.value,
  proxyId: form.proxy_id,
  mode: 'create',
  hostData: {
    proxies: props.proxies,
    groups: props.groups,
    isSimpleMode: authStore.isSimpleMode,
    quotaNotifyGlobalEnabled: false,
    platform: form.platform,
    compatiblePlatforms: getPlatformDecl(form.platform)?.compatible_gateways ?? [],
  },
}))

const platformFormExtraProps = computed(() => ({ platform: form.platform }))

function onCategoryCardSelect(group: typeof groupedTypeCards.value[number]) {
  const cat = group.category as typeof accountCategory.value
  accountCategory.value = cat
  selectedAccountTypeId.value = group.defaultTypeId
  addMethod.value = cat === 'oauth-based' ? 'oauth' : 'oauth'
  if (!BUILTIN_PLATFORMS.has(form.platform)) {
    form.type = group.defaultTypeId
  }
}

// ---------------------------------------------------------------------------
// Step navigation
// ---------------------------------------------------------------------------
async function goToStep2() {
  if (!selectedAccountTypeId.value) {
    appStore.showError(t('admin.accounts.pleaseSelectType'))
    return
  }
  formLoading.value = true
  try {
    resolvedFormComponent.value = await resolveFormComponentAsync(form.platform)
    if (!resolvedFormComponent.value) {
      appStore.showError(t('admin.accounts.failedToLoadForm'))
      return
    }
    step.value = 2
  } finally {
    formLoading.value = false
  }
}

function goBackToStep1() {
  platformFormRef.value?.reset?.()
  step.value = 1
}

function goBackToStep2() {
  platformFormRef.value?.resetOAuth?.()
  oauthFlowRef.value?.reset()
  step.value = 2
}

// ---------------------------------------------------------------------------
// OAuth delegation
// ---------------------------------------------------------------------------
const oauthFlowRef = ref<OAuthFlowExposed | null>(null)
const defaultOAuthState: OAuthComposableState = { authUrl: '', sessionId: '', loading: false, error: '' }

const oauthState = computed<OAuthComposableState>(() =>
  platformFormRef.value?.getOAuthState?.() ?? defaultOAuthState)
const oauthCfg = computed<OAuthFlowConfig | undefined>(() =>
  platformFormRef.value?.oauthConfig)
const oauthAddMethod = computed<AddMethod>(() => addMethod.value)

const oauthStepTitle = computed(() => t('admin.accounts.oauth.platformAuthTitle'))

// ---------------------------------------------------------------------------
// Computed helpers
// ---------------------------------------------------------------------------
const isOAuthFlow = computed(() => platformFormRef.value?.isOAuthFlow?.() ?? false)
const isManualInputMethod = computed(() => oauthFlowRef.value?.inputMethod === 'manual')
const canExchangeCode = computed(() => {
  const code = oauthFlowRef.value?.authCode || ''
  return code.trim().length > 0 && !!oauthState.value.sessionId && !oauthState.value.loading
})

// ---------------------------------------------------------------------------
// Mixed channel warning
// ---------------------------------------------------------------------------
const showMixedChannelWarning = ref(false)
const mixedChannelWarningDetails = ref<{
  groupName: string; currentPlatform: string; otherPlatform: string
} | null>(null)
const mixedChannelWarningRawMessage = ref('')
const mixedChannelWarningAction = ref<(() => Promise<void>) | null>(null)
const antigravityMixedChannelConfirmed = ref(false)

const mixedChannelWarningMessageText = computed(() => {
  if (mixedChannelWarningDetails.value)
    return t('admin.accounts.mixedChannelWarning', mixedChannelWarningDetails.value)
  return mixedChannelWarningRawMessage.value
})

const needsMixedChannelCheck = computed(() =>
  oauthCfg.value?.needsMixedChannelCheck ?? false)

function clearMixedChannelDialog() {
  showMixedChannelWarning.value = false
  mixedChannelWarningDetails.value = null
  mixedChannelWarningRawMessage.value = ''
  mixedChannelWarningAction.value = null
}

function openMixedChannelDialog(opts: {
  response?: CheckMixedChannelResponse; message?: string
  onConfirm: () => Promise<void>
}) {
  const details = opts.response?.details
  mixedChannelWarningDetails.value = details
    ? { groupName: details.group_name || 'Unknown', currentPlatform: details.current_platform || 'Unknown', otherPlatform: details.other_platform || 'Unknown' }
    : null
  mixedChannelWarningRawMessage.value = opts.message || opts.response?.message || t('admin.accounts.failedToCreate')
  mixedChannelWarningAction.value = opts.onConfirm
  showMixedChannelWarning.value = true
}

async function handleMixedChannelConfirm() {
  const action = mixedChannelWarningAction.value
  if (!action) { clearMixedChannelDialog(); return }
  clearMixedChannelDialog()
  submitting.value = true
  try { await action() } finally { submitting.value = false }
}

function handleMixedChannelCancel() { clearMixedChannelDialog() }

function withAntigravityConfirmFlag(payload: CreateAccountRequest): CreateAccountRequest {
  if (needsMixedChannelCheck.value && antigravityMixedChannelConfirmed.value) {
    return { ...payload, confirm_mixed_channel_risk: true }
  }
  const cloned = { ...payload }
  delete cloned.confirm_mixed_channel_risk
  return cloned
}

async function ensureAntigravityMixedChannelConfirmed(onConfirm: () => Promise<void>): Promise<boolean> {
  if (!needsMixedChannelCheck.value || antigravityMixedChannelConfirmed.value) return true
  try {
    const result = await adminAPI.accounts.checkMixedChannelRisk({ platform: form.platform, group_ids: form.group_ids })
    if (!result.has_risk) return true
    openMixedChannelDialog({
      response: result,
      onConfirm: async () => { antigravityMixedChannelConfirmed.value = true; await onConfirm() },
    })
    return false
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.accounts.failedToCreate')))
    return false
  }
}

// ---------------------------------------------------------------------------
// Submit / create account
// ---------------------------------------------------------------------------
async function submitCreateAccount(payload: CreateAccountRequest) {
  submitting.value = true
  try {
    await adminAPI.accounts.create(withAntigravityConfirmFlag(payload))
    appStore.showSuccess(t('admin.accounts.accountCreated'))
    emit('created')
    handleClose()
  } catch (err: unknown) {
    const errObj = err as { response?: { status?: number; data?: { error?: string; message?: string; detail?: string } } }
    if (errObj.response?.status === 409 && errObj.response?.data?.error === 'mixed_channel_warning' && needsMixedChannelCheck.value) {
      openMixedChannelDialog({
        message: errObj.response?.data?.message,
        onConfirm: async () => { antigravityMixedChannelConfirmed.value = true; await submitCreateAccount(payload) },
      })
      return
    }
    appStore.showError(extractApiErrorMessage(err, t('admin.accounts.failedToCreate')))
  } finally {
    submitting.value = false
  }
}

async function doCreateAccount(payload: CreateAccountRequest) {
  const canContinue = await ensureAntigravityMixedChannelConfirmed(() => submitCreateAccount(payload))
  if (!canContinue) return
  await submitCreateAccount(payload)
}

async function handleSubmit() {
  const validation = platformFormRef.value?.validate()
  if (validation && !validation.valid) { appStore.showError(validation.error || t('common.error')); return }
  const payload = platformFormRef.value?.getPayload()
  if (!payload) return
  const commonName = payload.common?.name?.trim() || ''
  const commonNotes = payload.common?.notes?.trim() || ''
  if (!commonName) { appStore.showError(t('admin.accounts.pleaseEnterAccountName')); return }
  if (payload.needsOAuthFlow) {
    cachedCommonFields.value = payload.common ? { ...payload.common } : null
    if (payload.typeOverride) addMethod.value = payload.typeOverride as AddMethod
    const canContinue = await ensureAntigravityMixedChannelConfirmed(async () => { step.value = 3 })
    if (!canContinue) return
    step.value = 3
    return
  }
  const resolvedType = payload.typeOverride || form.type
  // Merge quota fields from common into extra (backend expects them in extra)
  const extra = { ...(payload.extra || {}) }
  if (payload.common) {
    applyQuotaToExtra(extra, {
      quotaLimit: payload.common.quota_enabled ? payload.common.quota_limit : null,
      quotaDailyLimit: payload.common.quota_enabled ? payload.common.quota_daily_limit : null,
      quotaWeeklyLimit: payload.common.quota_enabled ? payload.common.quota_weekly_limit : null,
      dailyResetMode: null, dailyResetHour: null,
      weeklyResetMode: null, weeklyResetDay: null, weeklyResetHour: null,
      resetTimezone: null,
    })
  }
  const request: CreateAccountRequest = {
    name: commonName,
    notes: commonNotes || undefined,
    platform: form.platform,
    type: resolvedType,
    credentials: payload.credentials,
    extra,
    ...(payload.common ? {
      proxy_id: payload.common.proxy_id,
      concurrency: payload.common.concurrency,
      load_factor: payload.common.load_factor,
      priority: payload.common.priority,
      rate_multiplier: payload.common.rate_multiplier,
      expires_at: payload.common.expires_at,
      auto_pause_on_expired: payload.common.auto_pause_on_expired,
      group_ids: payload.common.group_ids,
    } : {}),
  } as CreateAccountRequest
  await doCreateAccount(request)
}

// ---------------------------------------------------------------------------
// OAuth handlers (delegate to form component)
// ---------------------------------------------------------------------------
async function handleGenerateUrl() {
  await platformFormRef.value?.generateOAuthUrl?.(form.proxy_id, oauthFlowRef.value?.projectId)
}

async function handleExchangeCode() {
  const code = oauthFlowRef.value?.authCode?.trim()
  if (!code) return
  const result = await platformFormRef.value?.handleOAuthExchange?.(code, oauthFlowRef.value?.oauthState, oauthFlowRef.value?.projectId)
  if (result) await finalizeOAuthResult(result)
}

async function handleCookieAuth(key: string) {
  const result = await platformFormRef.value?.handleCookieAuth?.(key)
  if (result) await finalizeOAuthResult(result)
}

async function handleRefreshToken(rt: string) {
  const result = await platformFormRef.value?.handleRefreshToken?.(rt)
  if (result) await finalizeOAuthResult(result)
}

async function handleMobileRefreshToken(rt: string) {
  const result = await platformFormRef.value?.handleMobileRefreshToken?.(rt)
  if (result) await finalizeOAuthResult(result)
}

async function handleSessionToken(token: string) {
  const result = await platformFormRef.value?.handleSessionToken?.(token)
  if (result) await finalizeOAuthResult(result)
}

async function handleCodexSessionImport(content: string) {
  const trimmed = content.trim()
  if (!trimmed) return

  try {
    const common = cachedCommonFields.value
    // Build extra with quota fields if enabled
    const codexExtra: Record<string, unknown> = {}
    if (common) {
      applyQuotaToExtra(codexExtra, {
        quotaLimit: common.quota_enabled ? common.quota_limit : null,
        quotaDailyLimit: common.quota_enabled ? common.quota_daily_limit : null,
        quotaWeeklyLimit: common.quota_enabled ? common.quota_weekly_limit : null,
        dailyResetMode: null, dailyResetHour: null,
        weeklyResetMode: null, weeklyResetDay: null, weeklyResetHour: null,
        resetTimezone: null,
      })
    }
    const result = await adminAPI.accounts.importCodexSession({
      content: trimmed,
      name: common?.name || '',
      notes: common?.notes || undefined,
      proxy_id: common?.proxy_id ?? undefined,
      concurrency: common?.concurrency,
      load_factor: common?.load_factor ?? undefined,
      priority: common?.priority,
      rate_multiplier: common?.rate_multiplier,
      group_ids: common?.group_ids,
      expires_at: common?.expires_at,
      auto_pause_on_expired: common?.auto_pause_on_expired,
      extra: Object.keys(codexExtra).length > 0 ? codexExtra : undefined,
      update_existing: true,
    })

    const params = { created: result.created, updated: result.updated, skipped: result.skipped, failed: result.failed }
    if (result.created + result.updated > 0 && result.failed === 0) {
      appStore.showSuccess(t("admin.accounts.oauth.openai.codexSessionImportSuccess", params))
      emit("created")
      handleClose()
    } else if (result.failed > 0 && result.created + result.updated > 0) {
      appStore.showWarning(t("admin.accounts.oauth.openai.codexSessionImportPartial", params))
      emit("created")
    } else if (result.failed > 0) {
      appStore.showError(t("admin.accounts.oauth.openai.codexSessionImportFailed"))
    }
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t("admin.accounts.oauth.openai.codexSessionImportFailed")))
  }
}

async function finalizeOAuthResult(result: CreateAccountRequest | CreateAccountRequest[]) {
  const requests = Array.isArray(result) ? result : [result]
  const common = cachedCommonFields.value
  for (let i = 0; i < requests.length; i++) {
    const req = { ...requests[i] }
    const baseName = req.name || common?.name || ''
    req.name = requests.length > 1 ? `${baseName} #${i + 1}` : baseName
    if (common) {
      req.notes = common.notes || undefined
      req.proxy_id = common.proxy_id
      req.concurrency = common.concurrency
      req.load_factor = common.load_factor
      req.priority = common.priority
      req.rate_multiplier = common.rate_multiplier
      req.group_ids = common.group_ids
      req.expires_at = common.expires_at
      req.auto_pause_on_expired = common.auto_pause_on_expired
      // Merge quota fields into extra
      const extra = { ...(req.extra || {}) } as Record<string, unknown>
      applyQuotaToExtra(extra, {
        quotaLimit: common.quota_enabled ? common.quota_limit : null,
        quotaDailyLimit: common.quota_enabled ? common.quota_daily_limit : null,
        quotaWeeklyLimit: common.quota_enabled ? common.quota_weekly_limit : null,
        dailyResetMode: null, dailyResetHour: null,
        weeklyResetMode: null, weeklyResetDay: null, weeklyResetHour: null,
        resetTimezone: null,
      })
      req.extra = extra
    }
    await submitCreateAccount(req)
  }
}

// ---------------------------------------------------------------------------
// Watchers
// ---------------------------------------------------------------------------
onMounted(() => { fetchPlatforms() })

watch(() => props.show, (newVal) => {
  if (newVal) fetchPlatforms()
  else resetForm()
})

watch(
  [accountCategory, addMethod, () => selectedAccountTypeId.value, () => form.platform],
  ([category, method]) => {
    if (!BUILTIN_PLATFORMS.has(form.platform)) return
    if (category === 'oauth-based') {
      form.type = method as AccountType
    } else {
      form.type = selectedAccountTypeId.value as AccountType
    }
  },
  { immediate: true },
)

watch(() => form.platform, (newPlatform) => {
  const decl = getPlatformDecl(newPlatform)
  if (decl?.account_types.length) {
    const firstType = decl.account_types[0]
    const cat = typeIdToCategory(firstType.type)
    accountCategory.value = cat
    selectedAccountTypeId.value = firstType.type
    addMethod.value = 'oauth'
    if (!BUILTIN_PLATFORMS.has(newPlatform)) {
      form.type = firstType.type
    }
  } else {
    accountCategory.value = 'oauth-based'
    addMethod.value = 'oauth'
    selectedAccountTypeId.value = 'oauth'
  }
})

// ---------------------------------------------------------------------------
// Reset / Close
// ---------------------------------------------------------------------------
function resetForm() {
  step.value = 1
  cachedCommonFields.value = null
  form.platform = (allPlatforms.value[0]?.platform || 'anthropic') as AccountPlatform
  form.type = 'oauth'
  form.credentials = {}
  form.proxy_id = null
  form.group_ids = []
  accountCategory.value = 'oauth-based'
  addMethod.value = 'oauth'
  selectedAccountTypeId.value = 'oauth'
  apiKeyBaseUrl.value = 'https://api.anthropic.com'
  apiKeyValue.value = ''
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
  interceptWarmupRequests.value = false
  autoPauseOnExpired.value = true
  openaiPassthroughEnabled.value = false
  openAICompactMode.value = 'auto'
  openAIResponsesMode.value = 'auto'
  openaiOAuthResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
  openaiAPIKeyResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
  codexCLIOnlyEnabled.value = false
  anthropicPassthroughEnabled.value = false
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
  antigravityMixedChannelConfirmed.value = false
  resolvedFormComponent.value = null
  clearMixedChannelDialog()
  platformFormRef.value?.reset?.()
  platformFormRef.value?.resetOAuth?.()
  oauthFlowRef.value?.reset()
}

function handleClose() {
  antigravityMixedChannelConfirmed.value = false
  clearMixedChannelDialog()
  emit('close')
}

const buildOpenAIExtra = (base?: Record<string, unknown>): Record<string, unknown> | undefined => {
  if (form.platform !== 'openai') {
    return base
  }

  const extra: Record<string, unknown> = { ...(base || {}) }
  if (accountCategory.value === 'oauth-based') {
    extra.openai_oauth_responses_websockets_v2_mode = openaiOAuthResponsesWebSocketV2Mode.value
    extra.openai_oauth_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(openaiOAuthResponsesWebSocketV2Mode.value)
  } else if (accountCategory.value === 'apikey') {
    extra.openai_apikey_responses_websockets_v2_mode = openaiAPIKeyResponsesWebSocketV2Mode.value
    extra.openai_apikey_responses_websockets_v2_enabled = isOpenAIWSModeEnabled(openaiAPIKeyResponsesWebSocketV2Mode.value)
  }
  // 清理兼容旧键，统一改用分类型开关。
  delete extra.responses_websockets_v2_enabled
  delete extra.openai_ws_enabled
  if (openaiPassthroughEnabled.value) {
    extra.openai_passthrough = true
  } else {
    delete extra.openai_passthrough
    delete extra.openai_oauth_passthrough
  }

  if (accountCategory.value === 'oauth-based' && codexCLIOnlyEnabled.value) {
    extra.codex_cli_only = true
  } else {
    delete extra.codex_cli_only
  }
  if (openAICompactMode.value !== 'auto') {
    extra.openai_compact_mode = openAICompactMode.value
  } else {
    delete extra.openai_compact_mode
  }

  if (accountCategory.value === 'apikey' && openAIResponsesMode.value !== 'auto') {
    extra.openai_responses_mode = openAIResponsesMode.value
  } else {
    delete extra.openai_responses_mode
  }

  return Object.keys(extra).length > 0 ? extra : undefined
}

const buildAnthropicExtra = (base?: Record<string, unknown>): Record<string, unknown> | undefined => {
  if (form.platform !== 'anthropic' || accountCategory.value !== 'apikey') {
    return base
  }

  const extra: Record<string, unknown> = { ...(base || {}) }
  if (anthropicPassthroughEnabled.value) {
    extra.anthropic_passthrough = true
  } else {
    delete extra.anthropic_passthrough
  }
  if (webSearchEmulationMode.value === 'default') {
    delete extra.web_search_emulation
  } else {
    extra.web_search_emulation = webSearchEmulationMode.value
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

const normalizePoolModeRetryCount = (value: number) => {
  if (!Number.isFinite(value)) {
    return DEFAULT_POOL_MODE_RETRY_COUNT
  }
  const normalized = Math.trunc(value)
  if (normalized < 0) {
    return 0
  }
  if (normalized > MAX_POOL_MODE_RETRY_COUNT) {
    return MAX_POOL_MODE_RETRY_COUNT
  }
  return normalized
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
    if (!form.name.trim()) {
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

  // Determine default base URL based on platform
  const defaultBaseUrl =
    form.platform === 'openai'
      ? 'https://api.openai.com'
      : form.platform === 'gemini'
        ? 'https://generativelanguage.googleapis.com'
        : 'https://api.anthropic.com'

  // Build credentials with optional model mapping
  const credentials: Record<string, unknown> = {
    base_url: apiKeyBaseUrl.value.trim() || defaultBaseUrl,
    api_key: apiKeyValue.value.trim()
  }
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

  applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
  if (!applyTempUnschedConfig(credentials)) {
    return
  }

  form.credentials = credentials
  const extra = buildAnthropicExtra(buildOpenAIExtra())

  await doCreateAccount({
    ...form,
    group_ids: form.group_ids,
    extra,
    auto_pause_on_expired: autoPauseOnExpired.value
  })
}

const goBackToBasicInfo = () => {
  step.value = 1
  oauth.resetState()
  openaiOAuth.resetState()
  geminiOAuth.resetState()
  antigravityOAuth.resetState()
  oauthFlowRef.value?.reset()
}

const handleGenerateUrl = async () => {
  if (form.platform === 'openai') {
    await openaiOAuth.generateAuthUrl(form.proxy_id)
  } else if (form.platform === 'gemini') {
    await geminiOAuth.generateAuthUrl(
      form.proxy_id,
      oauthFlowRef.value?.projectId,
      geminiOAuthType.value,
      geminiSelectedTier.value
    )
  } else if (form.platform === 'antigravity') {
    await antigravityOAuth.generateAuthUrl(form.proxy_id)
  } else {
    await oauth.generateAuthUrl(addMethod.value, form.proxy_id)
  }
}

const handleValidateRefreshToken = (rt: string) => {
  if (form.platform === 'openai') {
    handleOpenAIValidateRT(rt)
  } else if (form.platform === 'antigravity') {
    handleAntigravityValidateRT(rt)
  }
}

const handleValidateSessionToken = (_sessionToken: string) => {
  // Session token validation removed
}

const formatDateTimeLocal = formatDateTimeLocalInput
const parseDateTimeLocal = parseDateTimeLocalInput

// Create account and handle success/failure
const createAccountAndFinish = async (
  platform: AccountPlatform,
  type: AccountType,
  credentials: Record<string, unknown>,
  extra?: Record<string, unknown>
) => {
  if (!applyTempUnschedConfig(credentials)) {
    return
  }
  // Inject quota limits for apikey/bedrock accounts
  let finalExtra = extra
  if (type === 'apikey' || type === 'bedrock') {
    const quotaExtra: Record<string, unknown> = { ...(extra || {}) }
    if (editQuotaLimit.value != null && editQuotaLimit.value > 0) {
      quotaExtra.quota_limit = editQuotaLimit.value
    }
    if (editQuotaDailyLimit.value != null && editQuotaDailyLimit.value > 0) {
      quotaExtra.quota_daily_limit = editQuotaDailyLimit.value
    }
    if (editQuotaWeeklyLimit.value != null && editQuotaWeeklyLimit.value > 0) {
      quotaExtra.quota_weekly_limit = editQuotaWeeklyLimit.value
    }
    // Quota reset mode config
    if (editDailyResetMode.value === 'fixed') {
      quotaExtra.quota_daily_reset_mode = 'fixed'
      quotaExtra.quota_daily_reset_hour = editDailyResetHour.value ?? 0
    }
    if (editWeeklyResetMode.value === 'fixed') {
      quotaExtra.quota_weekly_reset_mode = 'fixed'
      quotaExtra.quota_weekly_reset_day = editWeeklyResetDay.value ?? 1
      quotaExtra.quota_weekly_reset_hour = editWeeklyResetHour.value ?? 0
    }
    if (editDailyResetMode.value === 'fixed' || editWeeklyResetMode.value === 'fixed') {
      quotaExtra.quota_reset_timezone = editResetTimezone.value || 'UTC'
    }
    // Quota notify config
    writeQuotaNotifyToExtra(quotaExtra, 'create')
    if (Object.keys(quotaExtra).length > 0) {
      finalExtra = quotaExtra
    }
  }
  if (platform === 'openai') {
    const compactModelMapping = buildOpenAICompactModelMapping()
    if (compactModelMapping) {
      credentials.compact_model_mapping = compactModelMapping
    } else {
      delete credentials.compact_model_mapping
    }
  }
  await doCreateAccount({
    name: form.name,
    notes: form.notes,
    platform,
    type,
    credentials,
    extra: finalExtra,
    proxy_id: form.proxy_id,
    concurrency: form.concurrency,
    load_factor: form.load_factor ?? undefined,
    priority: form.priority,
    rate_multiplier: form.rate_multiplier,
    group_ids: form.group_ids,
    expires_at: form.expires_at,
    auto_pause_on_expired: autoPauseOnExpired.value
  })
}

// OpenAI OAuth 授权码兑换
const handleOpenAIExchange = async (authCode: string) => {
  const oauthClient = openaiOAuth
  if (!authCode.trim() || !oauthClient.sessionId.value) return

  oauthClient.loading.value = true
  oauthClient.error.value = ''

  try {
    const stateToUse = (oauthFlowRef.value?.oauthState || oauthClient.oauthState.value || '').trim()
    if (!stateToUse) {
      oauthClient.error.value = t('admin.accounts.oauth.authFailed')
      appStore.showError(oauthClient.error.value)
      return
    }

    const tokenInfo = await oauthClient.exchangeAuthCode(
      authCode.trim(),
      oauthClient.sessionId.value,
      stateToUse,
      form.proxy_id
    )
    if (!tokenInfo) return

    const credentials = oauthClient.buildCredentials(tokenInfo)
    const oauthExtra = oauthClient.buildExtraInfo(tokenInfo) as Record<string, unknown> | undefined
    const extra = buildOpenAIExtra(oauthExtra)
    const shouldCreateOpenAI = form.platform === 'openai'

    // Add model mapping for OpenAI OAuth accounts（透传模式下不应用）
    if (shouldCreateOpenAI && !isOpenAIModelRestrictionDisabled.value) {
      const modelMapping = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value)
      if (modelMapping) {
        credentials.model_mapping = modelMapping
      }
    }
    if (shouldCreateOpenAI) {
      const compactModelMapping = buildOpenAICompactModelMapping()
      if (compactModelMapping) {
        credentials.compact_model_mapping = compactModelMapping
      }
    }

    // 应用临时不可调度配置
    if (!applyTempUnschedConfig(credentials)) {
      return
    }

    if (shouldCreateOpenAI) {
      await adminAPI.accounts.create({
        name: form.name,
        notes: form.notes,
        platform: 'openai',
        type: 'oauth',
        credentials,
        extra,
        proxy_id: form.proxy_id,
        concurrency: form.concurrency,
        load_factor: form.load_factor ?? undefined,
        priority: form.priority,
        rate_multiplier: form.rate_multiplier,
        group_ids: form.group_ids,
        expires_at: form.expires_at,
        auto_pause_on_expired: autoPauseOnExpired.value
      })
      appStore.showSuccess(t('admin.accounts.accountCreated'))
    }

    emit('created')
    handleClose()
  } catch (error: any) {
    oauthClient.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
    appStore.showError(oauthClient.error.value)
  } finally {
    oauthClient.loading.value = false
  }
}

// OpenAI 手动 RT 批量验证和创建
// OpenAI Mobile RT client_id
const OPENAI_MOBILE_RT_CLIENT_ID = 'app_LlGpXReQgckcGGUo2JrYvtJK'

const buildOpenAICodexImportCredentialExtras = (): Record<string, unknown> | null => {
  const credentials: Record<string, unknown> = {}
  if (!isOpenAIModelRestrictionDisabled.value) {
    const modelMapping = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value)
    if (modelMapping) {
      credentials.model_mapping = modelMapping
    }
  }

  const compactModelMapping = buildOpenAICompactModelMapping()
  if (compactModelMapping) {
    credentials.compact_model_mapping = compactModelMapping
  }

  if (!applyTempUnschedConfig(credentials)) {
    return null
  }
  return credentials
}

const formatCodexImportMessages = (messages?: CodexSessionImportMessage[]) => {
  return (messages || [])
    .map((item) => {
      const name = item.name ? ` ${item.name}` : ''
      return `#${item.index}${name}: ${item.message}`
    })
    .join('\n')
}

const handleOpenAIImportCodexSession = async (content: string) => {
  const oauthClient = openaiOAuth
  const trimmed = content.trim()
  if (!trimmed) {
    oauthClient.error.value = t('admin.accounts.oauth.openai.codexSessionEmpty')
    return
  }

  const credentialExtras = buildOpenAICodexImportCredentialExtras()
  if (credentialExtras === null) {
    return
  }

  oauthClient.loading.value = true
  oauthClient.error.value = ''

  try {
    const extra = buildOpenAIExtra()
    const result = await adminAPI.accounts.importCodexSession({
      content: trimmed,
      name: form.name,
      notes: form.notes || null,
      proxy_id: form.proxy_id,
      concurrency: form.concurrency,
      load_factor: form.load_factor ?? undefined,
      priority: form.priority,
      rate_multiplier: form.rate_multiplier,
      group_ids: form.group_ids,
      expires_at: form.expires_at,
      auto_pause_on_expired: autoPauseOnExpired.value,
      credential_extras: Object.keys(credentialExtras).length > 0 ? credentialExtras : undefined,
      extra,
      update_existing: true
    })

    const successCount = result.created + result.updated
    const params = {
      created: result.created,
      updated: result.updated,
      skipped: result.skipped,
      failed: result.failed
    }

    if (successCount > 0 && result.failed === 0) {
      appStore.showSuccess(t('admin.accounts.oauth.openai.codexSessionImportSuccess', params))
      emit('created')
      handleClose()
      return
    }

    const errorText = formatCodexImportMessages(result.errors)
    const warningText = formatCodexImportMessages(result.warnings)
    oauthClient.error.value = [errorText, warningText].filter(Boolean).join('\n')

    if (result.failed === 0) {
      appStore.showWarning(t('admin.accounts.oauth.openai.codexSessionImportSuccess', params))
      return
    }

    if (successCount > 0) {
      appStore.showWarning(t('admin.accounts.oauth.openai.codexSessionImportPartial', params))
      emit('created')
      return
    }

    appStore.showError(t('admin.accounts.oauth.openai.codexSessionImportFailed'))
  } catch (error: any) {
    oauthClient.error.value =
      error.response?.data?.detail ||
      error.response?.data?.message ||
      error.message ||
      t('admin.accounts.oauth.openai.codexSessionImportFailed')
    appStore.showError(oauthClient.error.value)
  } finally {
    oauthClient.loading.value = false
  }
}

// OpenAI RT 批量验证和创建（共享逻辑）
const handleOpenAIBatchRT = async (refreshTokenInput: string, clientId?: string) => {
  const oauthClient = openaiOAuth
  if (!refreshTokenInput.trim()) return

  const refreshTokens = refreshTokenInput
    .split('\n')
    .map((rt) => rt.trim())
    .filter((rt) => rt)

  if (refreshTokens.length === 0) {
    oauthClient.error.value = t('admin.accounts.oauth.openai.pleaseEnterRefreshToken')
    return
  }

  oauthClient.loading.value = true
  oauthClient.error.value = ''

  let successCount = 0
  let failedCount = 0
  const errors: string[] = []
  const shouldCreateOpenAI = form.platform === 'openai'

  try {
    for (let i = 0; i < refreshTokens.length; i++) {
      try {
        const tokenInfo = await oauthClient.validateRefreshToken(
          refreshTokens[i],
          form.proxy_id,
          clientId
        )
        if (!tokenInfo) {
          failedCount++
          errors.push(`#${i + 1}: ${oauthClient.error.value || 'Validation failed'}`)
          oauthClient.error.value = ''
          continue
        }

        const credentials = oauthClient.buildCredentials(tokenInfo)
        if (clientId) {
          credentials.client_id = clientId
        }
        const oauthExtra = oauthClient.buildExtraInfo(tokenInfo) as Record<string, unknown> | undefined
        const extra = buildOpenAIExtra(oauthExtra)

        // Add model mapping for OpenAI OAuth accounts（透传模式下不应用）
        if (shouldCreateOpenAI && !isOpenAIModelRestrictionDisabled.value) {
          const modelMapping = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value)
          if (modelMapping) {
            credentials.model_mapping = modelMapping
          }
        }
        if (shouldCreateOpenAI) {
          const compactModelMapping = buildOpenAICompactModelMapping()
          if (compactModelMapping) {
            credentials.compact_model_mapping = compactModelMapping
          }
        }

        // Generate account name; fallback to email if name is empty (ent schema requires NotEmpty)
        const baseName = form.name || tokenInfo.email || 'OpenAI OAuth Account'
        const accountName = refreshTokens.length > 1 ? `${baseName} #${i + 1}` : baseName

        if (shouldCreateOpenAI) {
          await adminAPI.accounts.create({
            name: accountName,
            notes: form.notes,
            platform: 'openai',
            type: 'oauth',
            credentials,
            extra,
            proxy_id: form.proxy_id,
            concurrency: form.concurrency,
            load_factor: form.load_factor ?? undefined,
            priority: form.priority,
            rate_multiplier: form.rate_multiplier,
            group_ids: form.group_ids,
            expires_at: form.expires_at,
            auto_pause_on_expired: autoPauseOnExpired.value
          })
        }

        successCount++
      } catch (error: any) {
        failedCount++
        const errMsg = error.response?.data?.detail || error.message || 'Unknown error'
        errors.push(`#${i + 1}: ${errMsg}`)
      }
    }

    // Show results
    if (successCount > 0 && failedCount === 0) {
      appStore.showSuccess(
        refreshTokens.length > 1
          ? t('admin.accounts.oauth.batchSuccess', { count: successCount })
          : t('admin.accounts.accountCreated')
      )
      emit('created')
      handleClose()
    } else if (successCount > 0 && failedCount > 0) {
      appStore.showWarning(
        t('admin.accounts.oauth.batchPartialSuccess', { success: successCount, failed: failedCount })
      )
      oauthClient.error.value = errors.join('\n')
      emit('created')
    } else {
      oauthClient.error.value = errors.join('\n')
      appStore.showError(t('admin.accounts.oauth.batchFailed'))
    }
  } finally {
    oauthClient.loading.value = false
  }
}

// 手动输入 RT（Codex CLI client_id，默认）
const handleOpenAIValidateRT = (rt: string) => handleOpenAIBatchRT(rt)

// 手动输入 Mobile RT
const handleOpenAIValidateMobileRT = (rt: string) => handleOpenAIBatchRT(rt, OPENAI_MOBILE_RT_CLIENT_ID)

// Antigravity 手动 RT 批量验证和创建
const handleAntigravityValidateRT = async (refreshTokenInput: string) => {
  if (!refreshTokenInput.trim()) return

  // Parse multiple refresh tokens (one per line)
  const refreshTokens = refreshTokenInput
    .split('\n')
    .map((rt) => rt.trim())
    .filter((rt) => rt)

  if (refreshTokens.length === 0) {
    antigravityOAuth.error.value = t('admin.accounts.oauth.antigravity.pleaseEnterRefreshToken')
    return
  }

  antigravityOAuth.loading.value = true
  antigravityOAuth.error.value = ''

  let successCount = 0
  let failedCount = 0
  const errors: string[] = []

  try {
    for (let i = 0; i < refreshTokens.length; i++) {
      try {
        const tokenInfo = await antigravityOAuth.validateRefreshToken(
          refreshTokens[i],
          form.proxy_id
        )
        if (!tokenInfo) {
          failedCount++
          errors.push(`#${i + 1}: ${antigravityOAuth.error.value || 'Validation failed'}`)
          antigravityOAuth.error.value = ''
          continue
        }

        const credentials = antigravityOAuth.buildCredentials(tokenInfo)
        
        // Generate account name with index for batch
        const accountName = refreshTokens.length > 1 ? `${form.name} #${i + 1}` : form.name

        // Note: Antigravity doesn't have buildExtraInfo, so we pass empty extra or rely on credentials
        const createPayload = withAntigravityConfirmFlag({
          name: accountName,
          notes: form.notes,
          platform: 'antigravity',
          type: 'oauth',
          credentials,
          extra: {},
          proxy_id: form.proxy_id,
          concurrency: form.concurrency,
          load_factor: form.load_factor ?? undefined,
          priority: form.priority,
          rate_multiplier: form.rate_multiplier,
          group_ids: form.group_ids,
          expires_at: form.expires_at,
          auto_pause_on_expired: autoPauseOnExpired.value
        })
        await adminAPI.accounts.create(createPayload)
        successCount++
      } catch (error: any) {
        failedCount++
        const errMsg = error.response?.data?.detail || error.message || 'Unknown error'
        errors.push(`#${i + 1}: ${errMsg}`)
      }
    }

    // Show results
    if (successCount > 0 && failedCount === 0) {
      appStore.showSuccess(
        refreshTokens.length > 1
          ? t('admin.accounts.oauth.batchSuccess', { count: successCount })
          : t('admin.accounts.accountCreated')
      )
      emit('created')
      handleClose()
    } else if (successCount > 0 && failedCount > 0) {
      appStore.showWarning(
        t('admin.accounts.oauth.batchPartialSuccess', { success: successCount, failed: failedCount })
      )
      antigravityOAuth.error.value = errors.join('\n')
      emit('created')
    } else {
      antigravityOAuth.error.value = errors.join('\n')
      appStore.showError(t('admin.accounts.oauth.batchFailed'))
    }
  } finally {
    antigravityOAuth.loading.value = false
  }
}

// Gemini OAuth 授权码兑换
const handleGeminiExchange = async (authCode: string) => {
  if (!authCode.trim() || !geminiOAuth.sessionId.value) return

  geminiOAuth.loading.value = true
  geminiOAuth.error.value = ''

  try {
    const stateFromInput = oauthFlowRef.value?.oauthState || ''
    const stateToUse = stateFromInput || geminiOAuth.state.value
    if (!stateToUse) {
      geminiOAuth.error.value = t('admin.accounts.oauth.authFailed')
      appStore.showError(geminiOAuth.error.value)
      return
    }

    const tokenInfo = await geminiOAuth.exchangeAuthCode({
      code: authCode.trim(),
      sessionId: geminiOAuth.sessionId.value,
      state: stateToUse,
      proxyId: form.proxy_id,
      oauthType: geminiOAuthType.value,
      tierId: geminiSelectedTier.value
    })
    if (!tokenInfo) return

    const credentials = geminiOAuth.buildCredentials(tokenInfo)
    const extra = geminiOAuth.buildExtraInfo(tokenInfo)
    await createAccountAndFinish('gemini', 'oauth', credentials, extra)
  } catch (error: any) {
    geminiOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
    appStore.showError(geminiOAuth.error.value)
  } finally {
    geminiOAuth.loading.value = false
  }
}

// Antigravity OAuth 授权码兑换
const handleAntigravityExchange = async (authCode: string) => {
  if (!authCode.trim() || !antigravityOAuth.sessionId.value) return

  antigravityOAuth.loading.value = true
  antigravityOAuth.error.value = ''

  try {
    const stateFromInput = oauthFlowRef.value?.oauthState || ''
    const stateToUse = stateFromInput || antigravityOAuth.state.value
    if (!stateToUse) {
      antigravityOAuth.error.value = t('admin.accounts.oauth.authFailed')
      appStore.showError(antigravityOAuth.error.value)
      return
    }

    const tokenInfo = await antigravityOAuth.exchangeAuthCode({
      code: authCode.trim(),
      sessionId: antigravityOAuth.sessionId.value,
      state: stateToUse,
      proxyId: form.proxy_id
    })
		if (!tokenInfo) return

		const credentials = antigravityOAuth.buildCredentials(tokenInfo)
		applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
		// Antigravity 只使用映射模式
		const antigravityModelMapping = buildModelMappingObject(
			'mapping',
			[],
			antigravityModelMappings.value
		)
		if (antigravityModelMapping) {
			credentials.model_mapping = antigravityModelMapping
		}
		const extra = buildAntigravityExtra()
		await createAccountAndFinish('antigravity', 'oauth', credentials, extra)
  } catch (error: any) {
    antigravityOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
    appStore.showError(antigravityOAuth.error.value)
  } finally {
    antigravityOAuth.loading.value = false
  }
}

// Anthropic OAuth 授权码兑换
const handleAnthropicExchange = async (authCode: string) => {
  if (!authCode.trim() || !oauth.sessionId.value) return

  oauth.loading.value = true
  oauth.error.value = ''

  try {
    const proxyConfig = form.proxy_id ? { proxy_id: form.proxy_id } : {}
    const endpoint =
      addMethod.value === 'oauth'
        ? '/admin/accounts/exchange-code'
        : '/admin/accounts/exchange-setup-token-code'

    const tokenInfo = await adminAPI.accounts.exchangeCode(endpoint, {
      session_id: oauth.sessionId.value,
      code: authCode.trim(),
      ...proxyConfig
    })

    // Build extra with quota control settings
    const baseExtra = oauth.buildExtraInfo(tokenInfo) || {}
    const extra: Record<string, unknown> = { ...baseExtra }

    // Add window cost limit settings
    if (windowCostEnabled.value && windowCostLimit.value != null && windowCostLimit.value > 0) {
      extra.window_cost_limit = windowCostLimit.value
      extra.window_cost_sticky_reserve = windowCostStickyReserve.value ?? 10
    }

    // Add session limit settings
    if (sessionLimitEnabled.value && maxSessions.value != null && maxSessions.value > 0) {
      extra.max_sessions = maxSessions.value
      extra.session_idle_timeout_minutes = sessionIdleTimeout.value ?? 5
    }

    // Add RPM limit settings
    if (rpmLimitEnabled.value) {
      const DEFAULT_BASE_RPM = 15
      extra.base_rpm = (baseRpm.value != null && baseRpm.value > 0)
        ? baseRpm.value
        : DEFAULT_BASE_RPM
      extra.rpm_strategy = rpmStrategy.value
      if (rpmStickyBuffer.value != null && rpmStickyBuffer.value > 0) {
        extra.rpm_sticky_buffer = rpmStickyBuffer.value
      }
    }

    // UMQ mode（独立于 RPM）
    if (userMsgQueueMode.value) {
      extra.user_msg_queue_mode = userMsgQueueMode.value
    }

    // Add TLS fingerprint settings
    if (tlsFingerprintEnabled.value) {
      extra.enable_tls_fingerprint = true
      if (tlsFingerprintProfileId.value) {
        extra.tls_fingerprint_profile_id = tlsFingerprintProfileId.value
      }
    }

    // Add session ID masking settings
    if (sessionIdMaskingEnabled.value) {
      extra.session_id_masking_enabled = true
    }

    // Add cache TTL override settings
    if (cacheTTLOverrideEnabled.value) {
      extra.cache_ttl_override_enabled = true
      extra.cache_ttl_override_target = cacheTTLOverrideTarget.value
    }

    // Add custom base URL settings
    if (customBaseUrlEnabled.value && customBaseUrl.value.trim()) {
      extra.custom_base_url_enabled = true
      extra.custom_base_url = customBaseUrl.value.trim()
    }

    const credentials: Record<string, unknown> = { ...tokenInfo }
    applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
    await createAccountAndFinish(form.platform, addMethod.value as AccountType, credentials, extra)
  } catch (error: any) {
    oauth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
    appStore.showError(oauth.error.value)
  } finally {
    oauth.loading.value = false
  }
}

// 主入口：根据平台路由到对应处理函数
const handleExchangeCode = async () => {
  const authCode = oauthFlowRef.value?.authCode || ''

  switch (form.platform) {
    case 'openai':
      return handleOpenAIExchange(authCode)
    case 'gemini':
      return handleGeminiExchange(authCode)
    case 'antigravity':
      return handleAntigravityExchange(authCode)
    default:
      return handleAnthropicExchange(authCode)
  }
}

const handleCookieAuth = async (sessionKey: string) => {
  oauth.loading.value = true
  oauth.error.value = ''

  try {
    const proxyConfig = form.proxy_id ? { proxy_id: form.proxy_id } : {}
    const keys = oauth.parseSessionKeys(sessionKey)

    if (keys.length === 0) {
      oauth.error.value = t('admin.accounts.oauth.pleaseEnterSessionKey')
      return
    }

    const tempUnschedPayload = tempUnschedEnabled.value
      ? buildTempUnschedRules(tempUnschedRules.value)
      : []
    if (tempUnschedEnabled.value && tempUnschedPayload.length === 0) {
      appStore.showError(t('admin.accounts.tempUnschedulable.rulesInvalid'))
      return
    }

    const endpoint =
      addMethod.value === 'oauth'
        ? '/admin/accounts/cookie-auth'
        : '/admin/accounts/setup-token-cookie-auth'

    let successCount = 0
    let failedCount = 0
    const errors: string[] = []

    for (let i = 0; i < keys.length; i++) {
      try {
        const tokenInfo = await adminAPI.accounts.exchangeCode(endpoint, {
          session_id: '',
          code: keys[i],
          ...proxyConfig
        })

        // Build extra with quota control settings
        const baseExtra = oauth.buildExtraInfo(tokenInfo) || {}
        const extra: Record<string, unknown> = { ...baseExtra }

        // Add window cost limit settings
        if (windowCostEnabled.value && windowCostLimit.value != null && windowCostLimit.value > 0) {
          extra.window_cost_limit = windowCostLimit.value
          extra.window_cost_sticky_reserve = windowCostStickyReserve.value ?? 10
        }

        // Add session limit settings
        if (sessionLimitEnabled.value && maxSessions.value != null && maxSessions.value > 0) {
          extra.max_sessions = maxSessions.value
          extra.session_idle_timeout_minutes = sessionIdleTimeout.value ?? 5
        }

        // Add RPM limit settings
        if (rpmLimitEnabled.value) {
          const DEFAULT_BASE_RPM = 15
          extra.base_rpm = (baseRpm.value != null && baseRpm.value > 0)
            ? baseRpm.value
            : DEFAULT_BASE_RPM
          extra.rpm_strategy = rpmStrategy.value
          if (rpmStickyBuffer.value != null && rpmStickyBuffer.value > 0) {
            extra.rpm_sticky_buffer = rpmStickyBuffer.value
          }
        }

        // UMQ mode（独立于 RPM）
        if (userMsgQueueMode.value) {
          extra.user_msg_queue_mode = userMsgQueueMode.value
        }

        // Add TLS fingerprint settings
        if (tlsFingerprintEnabled.value) {
          extra.enable_tls_fingerprint = true
          if (tlsFingerprintProfileId.value) {
            extra.tls_fingerprint_profile_id = tlsFingerprintProfileId.value
          }
        }

        // Add session ID masking settings
        if (sessionIdMaskingEnabled.value) {
          extra.session_id_masking_enabled = true
        }

        // Add cache TTL override settings
        if (cacheTTLOverrideEnabled.value) {
          extra.cache_ttl_override_enabled = true
          extra.cache_ttl_override_target = cacheTTLOverrideTarget.value
        }

        // Add custom base URL settings
        if (customBaseUrlEnabled.value && customBaseUrl.value.trim()) {
          extra.custom_base_url_enabled = true
          extra.custom_base_url = customBaseUrl.value.trim()
        }

        const accountName = keys.length > 1 ? `${form.name} #${i + 1}` : form.name

        const credentials: Record<string, unknown> = { ...tokenInfo }
        applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
        if (tempUnschedEnabled.value) {
          credentials.temp_unschedulable_enabled = true
          credentials.temp_unschedulable_rules = tempUnschedPayload
        }

        await adminAPI.accounts.create({
          name: accountName,
          notes: form.notes,
          platform: form.platform,
          type: addMethod.value, // Use addMethod as type: 'oauth' or 'setup-token'
          credentials,
          extra,
          proxy_id: form.proxy_id,
          concurrency: form.concurrency,
          load_factor: form.load_factor ?? undefined,
          priority: form.priority,
          rate_multiplier: form.rate_multiplier,
          group_ids: form.group_ids,
          expires_at: form.expires_at,
          auto_pause_on_expired: autoPauseOnExpired.value
        })

        successCount++
      } catch (error: any) {
        failedCount++
        errors.push(
          t('admin.accounts.oauth.keyAuthFailed', {
            index: i + 1,
            error: error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
          })
        )
      }
    }

    if (successCount > 0) {
      appStore.showSuccess(t('admin.accounts.oauth.successCreated', { count: successCount }))
      if (failedCount === 0) {
        emit('created')
        handleClose()
      } else {
        emit('created')
      }
    }

    if (failedCount > 0) {
      oauth.error.value = errors.join('\n')
    }
  } catch (error: any) {
    oauth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.cookieAuthFailed')
  } finally {
    oauth.loading.value = false
  }
}
</script>
