<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.createAccount')"
    width="wide"
    @close="handleClose"
  >
    <!-- Step Indicator for OAuth accounts -->
    <div v-if="isOAuthFlow" class="mb-6 flex items-center justify-center">
      <div class="flex items-center space-x-4">
        <div class="flex items-center">
          <div
            :class="[
              'flex h-8 w-8 items-center justify-center rounded-full text-sm font-semibold',
              step >= 1 ? 'bg-primary-500 text-white' : 'bg-gray-200 text-gray-500 dark:bg-dark-600'
            ]"
          >
            1
          </div>
          <span class="ml-2 text-sm font-medium text-gray-700 dark:text-gray-300">{{
            t('admin.accounts.oauth.authMethod')
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
            oauthStepTitle
          }}</span>
        </div>
      </div>
    </div>

    <!-- Step 1: Form -->
    <form
      v-if="step === 1"
      id="create-account-form"
      @submit.prevent="handleSubmit"
      class="space-y-5"
    >
      <!-- Name -->
      <div>
        <label class="input-label">{{ t('admin.accounts.accountName') }}</label>
        <input
          v-model="form.name"
          type="text"
          required
          class="input"
          :placeholder="t('admin.accounts.enterAccountName')"
          data-tour="account-form-name"
        />
      </div>
      <!-- Notes -->
      <div>
        <label class="input-label">{{ t('admin.accounts.notes') }}</label>
        <textarea
          v-model="form.notes"
          rows="3"
          class="input"
          :placeholder="t('admin.accounts.notesPlaceholder')"
        ></textarea>
        <p class="input-hint">{{ t('admin.accounts.notesHint') }}</p>
      </div>

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

      <!-- Dynamic Account Type Selection -->
      <div v-if="currentPlatformDecl && currentPlatformDecl.account_types.length > 0">
        <label class="input-label">{{ t('admin.accounts.accountType') }}</label>
        <div
          class="mt-2 grid gap-3"
          :class="currentPlatformDecl.account_types.length <= 2 ? 'grid-cols-2' : 'grid-cols-3'"
          data-tour="account-form-type"
        >
          <button
            v-for="at in currentPlatformDecl.account_types"
            :key="at.type"
            type="button"
            @click="onAccountTypeSelect(at)"
            :class="[
              'flex items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
              selectedAccountTypeId === at.type
                ? 'bg-primary-50 dark:bg-primary-900/20'
                : 'border-gray-200 hover:border-gray-300 dark:border-dark-600 dark:hover:border-dark-500'
            ]"
            :style="selectedAccountTypeId === at.type && currentPlatformDecl.theme_color ? { borderColor: currentPlatformDecl.theme_color } : {}"
          >
            <div
              :class="[
                'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg transition-colors',
                selectedAccountTypeId === at.type
                  ? 'text-white'
                  : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
              ]"
              :style="selectedAccountTypeId === at.type && currentPlatformDecl.theme_color ? { backgroundColor: currentPlatformDecl.theme_color } : {}"
            >
              <PlatformIcon v-if="at.icon_svg" :icon-svg="at.icon_svg" size="sm" />
              <Icon v-else name="key" size="sm" />
            </div>
            <div class="min-w-0">
              <span class="block text-sm font-medium text-gray-900 dark:text-white">{{ at.display_name }}</span>
              <span v-if="at.description" class="block truncate text-xs text-gray-500 dark:text-gray-400">{{ at.description }}</span>
            </div>
            <span
              v-if="at.badge_label"
              class="ml-auto shrink-0 rounded-full px-2 py-0.5 text-xs font-medium"
              :class="selectedAccountTypeId === at.type ? 'bg-white/80 text-gray-700' : 'bg-gray-100 text-gray-600 dark:bg-dark-600 dark:text-gray-400'"
            >{{ at.badge_label }}</span>
          </button>
        </div>
      </div>

      <!-- Dynamic Platform Form Body -->
      <component
        :is="platformFormComponent"
        ref="platformFormRef"
        :context="platformFormContext"
        v-bind="platformFormExtraProps"
      />

      <!-- Common fields -->
      <div>
        <label class="input-label">{{ t('admin.accounts.proxy') }}</label>
        <ProxySelector v-model="form.proxy_id" :proxies="proxies" />
      </div>
      <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <div>
          <label class="input-label">{{ t('admin.accounts.concurrency') }}</label>
          <input v-model.number="form.concurrency" type="number" min="1" class="input"
            @input="form.concurrency = Math.max(1, form.concurrency || 1)" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.loadFactor') }}</label>
          <input v-model.number="form.load_factor" type="number" min="1"
            class="input" :placeholder="String(form.concurrency || 1)"
            @input="form.load_factor = (form.load_factor && form.load_factor >= 1) ? form.load_factor : null" />
          <p class="input-hint">{{ t('admin.accounts.loadFactorHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.priority') }}</label>
          <input v-model.number="form.priority" type="number" min="1" class="input" data-tour="account-form-priority" />
          <p class="input-hint">{{ t('admin.accounts.priorityHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.billingRateMultiplier') }}</label>
          <input v-model.number="form.rate_multiplier" type="number" min="0" step="0.001" class="input" />
          <p class="input-hint">{{ t('admin.accounts.billingRateMultiplierHint') }}</p>
        </div>
      </div>
      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <label class="input-label">{{ t('admin.accounts.expiresAt') }}</label>
        <input v-model="expiresAtInput" type="datetime-local" class="input" />
        <p class="input-hint">{{ t('admin.accounts.expiresAtHint') }}</p>
      </div>
      <div>
        <div class="flex items-center justify-between">
          <div>
            <label class="input-label mb-0">{{ t('admin.accounts.autoPauseOnExpired') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.autoPauseOnExpiredDesc') }}</p>
          </div>
          <button type="button" @click="autoPauseOnExpired = !autoPauseOnExpired"
            :class="['relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2', autoPauseOnExpired ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600']">
            <span :class="['pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out', autoPauseOnExpired ? 'translate-x-5' : 'translate-x-0']" />
          </button>
        </div>
      </div>
      <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
        <GroupSelector v-if="!authStore.isSimpleMode" v-model="form.group_ids" :groups="groups" :platform="form.platform" data-tour="account-form-groups" />
      </div>
    </form>
    <!-- Step 2: OAuth Authorization -->
    <div v-else class="space-y-5">
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
        :platform="form.platform"
        :show-project-id="oauthCfg?.showProjectId ?? false"
        @generate-url="handleGenerateUrl"
        @cookie-auth="handleCookieAuth"
        @validate-refresh-token="handleRefreshToken"
        @validate-mobile-refresh-token="handleMobileRefreshToken"
        @validate-session-token="handleSessionToken"
      />
    </div>

    <!-- Footer -->
    <template #footer>
      <div v-if="step === 1" class="flex justify-end gap-3">
        <button @click="handleClose" type="button" class="btn btn-secondary">{{ t('common.cancel') }}</button>
        <button type="submit" form="create-account-form" :disabled="submitting" class="btn btn-primary" data-tour="account-form-submit">
          <svg v-if="submitting" class="-ml-1 mr-2 h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
          </svg>
          {{ isOAuthFlow ? t('common.next') : submitting ? t('admin.accounts.creating') : t('common.create') }}
        </button>
      </div>
      <div v-else class="flex justify-between gap-3">
        <button type="button" class="btn btn-secondary" @click="goBackToBasicInfo">{{ t('common.back') }}</button>
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
import { ref, reactive, computed, watch, onMounted, type Component } from 'vue'
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
import ProxySelector from '@/components/common/ProxySelector.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'
import OAuthAuthorizationFlow from './OAuthAuthorizationFlow.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import { usePlatforms } from '@/composables/usePlatforms'
import { resolvePlatformForm } from './forms/platformFormRegistry'
import type {
  PlatformFormContext, PlatformFormExposed,
  OAuthFlowConfig, OAuthComposableState,
} from './forms/types'
import type { AddMethod, AuthInputMethod } from '@/composables/useAccountOAuth'
import { formatDateTimeLocalInput, parseDateTimeLocalInput } from '@/utils/format'
import { extractApiErrorMessage } from '@/utils/apiError'

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
const autoPauseOnExpired = ref(true)
const accountCategory = ref<'oauth-based' | 'apikey' | 'bedrock' | 'service_account'>('oauth-based')
const addMethod = ref<AddMethod>('oauth')
const antigravityAccountType = ref<'oauth' | 'upstream'>('oauth')

const form = reactive({
  name: '',
  notes: '',
  platform: 'anthropic' as AccountPlatform,
  type: 'oauth' as AccountType,
  credentials: {} as Record<string, unknown>,
  proxy_id: null as number | null,
  concurrency: 10,
  load_factor: null as number | null,
  priority: 1,
  rate_multiplier: 1,
  group_ids: [] as number[],
  expires_at: null as number | null,
})

// ---------------------------------------------------------------------------
// Dynamic platform form component
// ---------------------------------------------------------------------------
const platformFormRef = ref<PlatformFormExposed | null>(null)
const platformFormComponent = computed<Component>(() => resolvePlatformForm(form.platform))

const platformFormContext = computed<PlatformFormContext>(() => ({
  accountCategory: accountCategory.value,
  accountTypeId: selectedAccountTypeId.value,
  proxyId: form.proxy_id,
}))

const platformFormExtraProps = computed(() => {
  if (!BUILTIN_PLATFORMS.has(form.platform)) return { platform: form.platform }
  return {}
})

// ---------------------------------------------------------------------------
// Account type selection
// ---------------------------------------------------------------------------
const selectedAccountTypeId = computed(() => {
  if (BUILTIN_PLATFORMS.has(form.platform)) {
    if (form.platform === 'antigravity') return antigravityAccountType.value
    if (accountCategory.value === 'oauth-based') {
      return form.platform === 'anthropic' ? addMethod.value : 'oauth'
    }
    if (accountCategory.value === 'bedrock') return 'bedrock'
    if (accountCategory.value === 'service_account') return 'service_account'
    return 'apikey'
  }
  return form.type
})

function onAccountTypeSelect(at: { type: string }) {
  const tp = at.type
  if (BUILTIN_PLATFORMS.has(form.platform)) {
    if (form.platform === 'antigravity') {
      antigravityAccountType.value = tp as typeof antigravityAccountType.value
      accountCategory.value = tp === 'oauth' ? 'oauth-based' : 'apikey'
      return
    }
    if (tp === 'oauth' || tp === 'setup-token') {
      accountCategory.value = 'oauth-based'
      if (form.platform === 'anthropic') addMethod.value = tp as AddMethod
    } else if (tp === 'bedrock') {
      accountCategory.value = 'bedrock'
    } else if (tp === 'service_account') {
      accountCategory.value = 'service_account'
    } else {
      accountCategory.value = 'apikey'
    }
  } else {
    form.type = tp
  }
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
const expiresAtInput = computed({
  get: () => formatDateTimeLocalInput(form.expires_at),
  set: (value: string) => { form.expires_at = parseDateTimeLocalInput(value) },
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

function buildCommonFields(): Partial<CreateAccountRequest> {
  return {
    name: form.name,
    notes: form.notes || undefined,
    proxy_id: form.proxy_id || undefined,
    concurrency: form.concurrency || 1,
    load_factor: form.load_factor ?? undefined,
    priority: form.priority || 1,
    rate_multiplier: form.rate_multiplier,
    group_ids: form.group_ids,
    expires_at: form.expires_at,
    auto_pause_on_expired: autoPauseOnExpired.value,
  }
}

async function handleSubmit() {
  if (!form.name.trim()) { appStore.showError(t('admin.accounts.pleaseEnterAccountName')); return }
  const validation = platformFormRef.value?.validate()
  if (validation && !validation.valid) { appStore.showError(validation.error || t('common.error')); return }
  const payload = platformFormRef.value?.getPayload()
  if (!payload) return
  if (payload.needsOAuthFlow) {
    const canContinue = await ensureAntigravityMixedChannelConfirmed(async () => { step.value = 2 })
    if (!canContinue) return
    step.value = 2
    return
  }
  const request: CreateAccountRequest = {
    ...buildCommonFields(),
    platform: form.platform,
    type: payload.typeOverride || form.type,
    credentials: payload.credentials,
    extra: payload.extra,
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

async function finalizeOAuthResult(result: CreateAccountRequest | CreateAccountRequest[]) {
  const requests = Array.isArray(result) ? result : [result]
  const common = buildCommonFields()
  for (let i = 0; i < requests.length; i++) {
    const req = { ...requests[i] }
    const baseName = req.name || form.name
    req.name = requests.length > 1 ? `${baseName} #${i + 1}` : baseName
    req.proxy_id = common.proxy_id
    req.concurrency = common.concurrency
    req.load_factor = common.load_factor
    req.priority = common.priority
    req.rate_multiplier = common.rate_multiplier
    req.group_ids = common.group_ids
    req.expires_at = common.expires_at
    req.auto_pause_on_expired = common.auto_pause_on_expired
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
  [accountCategory, addMethod, antigravityAccountType, () => form.platform],
  ([category, method, agType]) => {
    if (form.platform === 'antigravity' && agType === 'upstream') { form.type = 'apikey'; return }
    if (form.platform === 'anthropic' && category === 'bedrock') { form.type = 'bedrock' as AccountType; return }
    if ((form.platform === 'gemini' || form.platform === 'anthropic') && category === 'service_account') {
      form.type = 'service_account' as AccountType
    } else if (category === 'oauth-based') {
      form.type = method as AccountType
    } else {
      form.type = 'apikey'
    }
  },
  { immediate: true },
)

watch(() => form.platform, (newPlatform) => {
  if (newPlatform !== 'gemini' && newPlatform !== 'anthropic' && accountCategory.value === 'service_account')
    accountCategory.value = 'oauth-based'
  if (newPlatform !== 'anthropic' && accountCategory.value === 'bedrock')
    accountCategory.value = 'oauth-based'
  if (newPlatform === 'antigravity') {
    accountCategory.value = 'oauth-based'
    antigravityAccountType.value = 'oauth'
  }
  if (!BUILTIN_PLATFORMS.has(newPlatform)) {
    const decl = getPlatformDecl(newPlatform)
    if (decl?.account_types.length) form.type = decl.account_types[0].type
  }
})

// ---------------------------------------------------------------------------
// Reset / Close / Navigation
// ---------------------------------------------------------------------------
function resetForm() {
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
  antigravityAccountType.value = 'oauth'
  autoPauseOnExpired.value = true
  antigravityMixedChannelConfirmed.value = false
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

function goBackToBasicInfo() {
  step.value = 1
  platformFormRef.value?.resetOAuth?.()
  oauthFlowRef.value?.reset()
}
</script>