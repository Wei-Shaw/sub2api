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
} from '@/types'
import { BaseDialog, ConfirmDialog } from '@sub2api/plugin-sdk'
import Icon from '@/components/icons/Icon.vue'
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
const autoPauseOnExpired = ref(true)

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

const geminiOAuthType = ref<'code_assist' | 'google_one' | 'ai_studio'>('google_one')

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
  autoPauseOnExpired.value = true
  geminiOAuthType.value = 'code_assist'
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
</script>
