<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.reAuthorizeAccount')"
    width="normal"
    @close="handleClose"
  >
    <div v-if="account" class="space-y-4">
      <!-- Account Info -->
      <div
        class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-700"
      >
        <div class="flex items-center gap-3">
          <div
            :class="[
              'flex h-10 w-10 items-center justify-center rounded-lg bg-gradient-to-br',
              gradientClass
            ]"
          >
            <PlatformIcon :platform="account.platform" size="md" class="text-white" />
          </div>
          <div>
            <span class="block font-semibold text-gray-900 dark:text-white">{{
              account.name
            }}</span>
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{ platformAccountLabel }}
            </span>
          </div>
        </div>
      </div>

      <!-- Platform form component (provides OAuth methods; visible for non-OAuth flows) -->
      <component
        v-show="showCredentialForm"
        :is="resolvedFormComponent"
        ref="platformFormRef"
        :context="platformFormContext"
        v-bind="platformFormExtraProps"
      />

      <!-- OAuth Authorization Flow (only for OAuth-capable platforms) -->
      <OAuthAuthorizationFlow
        v-if="hasOAuthFlow"
        ref="oauthFlowRef"
        :add-method="addMethod"
        :auth-url="oauthState.authUrl"
        :session-id="oauthState.sessionId"
        :loading="oauthState.loading"
        :error="oauthState.error"
        :show-help="oauthCfg?.showHelp ?? false"
        :show-proxy-warning="oauthCfg?.showProxyWarning ?? false"
        :show-cookie-option="oauthCfg?.showCookieOption ?? false"
        :show-refresh-token-option="oauthCfg?.showRefreshTokenOption ?? false"
        :show-mobile-refresh-token-option="oauthCfg?.showMobileRefreshTokenOption ?? false"
        :show-session-token-option="oauthCfg?.showSessionTokenOption ?? false"
        :show-access-token-option="oauthCfg?.showAccessTokenOption ?? false"
        :show-important-notice="oauthCfg?.showImportantNotice ?? false"
        :show-state-warning="oauthCfg?.showStateWarning ?? false"
        :i18n-prefix="oauthCfg?.i18nPrefix ?? ''"
        :allow-multiple="false"
        :method-label="t('admin.accounts.inputMethod')"
        :platform="account.platform"
        :show-project-id="oauthCfg?.showProjectId ?? false"
        @generate-url="handleGenerateUrl"
        @cookie-auth="handleCookieAuth"
        @validate-refresh-token="handleRefreshToken"
        @validate-mobile-refresh-token="handleMobileRefreshToken"
        @validate-session-token="handleSessionToken"
      />
    </div>

    <template #footer>
      <div v-if="account" class="flex justify-between gap-3">
        <button type="button" class="btn btn-secondary" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button
          v-if="showCredentialForm"
          type="button"
          :disabled="saving"
          class="btn btn-primary"
          @click="handleSaveCredentials"
        >
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
        <button
          v-else-if="isManualInputMethod"
          type="button"
          :disabled="!canExchangeCode"
          class="btn btn-primary"
          @click="handleExchangeCode"
        >
          <svg
            v-if="oauthState.loading"
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
          {{
            oauthState.loading
              ? t('admin.accounts.oauth.verifying')
              : t('admin.accounts.oauth.completeAuth')
          }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, shallowRef, computed, watch, nextTick, type Component } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { AddMethod, AuthInputMethod } from './forms/types'
import { usePlatforms } from '@/composables/usePlatforms'
import { resolveFormComponentAsync } from './forms/platformFormRegistry'
import type {
  PlatformFormContext, PlatformFormExposed,
  OAuthFlowConfig, OAuthComposableState,
} from './forms/types'
import type { Account, CreateAccountRequest } from '@/types'
import { BaseDialog } from '@sub2api/plugin-sdk'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import OAuthAuthorizationFlow from './OAuthAuthorizationFlow.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { platformGradientClass as platformGradientClassFn, platformLabel } from '@/utils/platformColors'

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

interface Props {
  show: boolean
  account: Account | null
}

const props = defineProps<Props>()
const emit = defineEmits<{
  close: []
  reauthorized: [account: Account]
}>()

const appStore = useAppStore()
const { t } = useI18n()
const { getPlatformDecl } = usePlatforms()

// ---------------------------------------------------------------------------
// Platform form component (delegates OAuth to the right platform)
// ---------------------------------------------------------------------------
const platformFormRef = ref<PlatformFormExposed | null>(null)
const resolvedFormComponent = shallowRef<Component | null>(null)

watch(
  () => props.account?.platform,
  async (platform) => {
    if (!platform) {
      resolvedFormComponent.value = null
      return
    }
    resolvedFormComponent.value = await resolveFormComponentAsync(platform)
  },
  { immediate: true },
)

// Re-init form after async component load completes (fixes race with initializeFromAccount)
watch(resolvedFormComponent, (comp) => {
  if (comp && props.show && props.account) {
    nextTick(() => { platformFormRef.value?.initFromAccount?.(props.account!) })
  }
})

const platformFormContext = computed<PlatformFormContext>(() => ({
  accountCategory: resolveAccountCategory(),
  accountTypeId: props.account?.type ?? 'oauth',
  proxyId: props.account?.proxy_id ?? null,
  mode: 'edit',
}))

const platformFormExtraProps = computed(() => ({
  platform: props.account?.platform || 'anthropic',
}))

// Refs
const oauthFlowRef = ref<OAuthFlowExposed | null>(null)

// State
const addMethod = ref<AddMethod>('oauth')
const saving = ref(false)

// ---------------------------------------------------------------------------
// OAuth delegation (mirror CreateAccountModal pattern)
// ---------------------------------------------------------------------------
const defaultOAuthState: OAuthComposableState = {
  authUrl: '', sessionId: '', loading: false, error: ''
}

const oauthState = computed<OAuthComposableState>(() =>
  platformFormRef.value?.getOAuthState?.() ?? defaultOAuthState
)

const oauthCfg = computed<OAuthFlowConfig | undefined>(() =>
  platformFormRef.value?.oauthConfig
)

const hasOAuthFlow = computed(() =>
  platformFormRef.value?.isOAuthFlow?.() ?? false
)

/** Show credential form for non-OAuth platforms (e.g. plugin apikey) */
const showCredentialForm = computed(() =>
  !!props.account && !hasOAuthFlow.value
)

// ---------------------------------------------------------------------------
// Theme / display (driven by PlatformDeclaration, no hardcoded platform checks)
// ---------------------------------------------------------------------------
const gradientClass = computed(() => {
  const p = props.account?.platform ?? ''
  const decl = props.account ? getPlatformDecl(p) : undefined
  return platformGradientClassFn(p, decl?.theme_color)
})

const platformAccountLabel = computed(() => {
  const decl = props.account ? getPlatformDecl(props.account.platform) : undefined
  const name = decl?.display_name || platformLabel(props.account?.platform ?? '')
  return `${name} ${t('admin.accounts.account')}`
})

// ---------------------------------------------------------------------------
// Computed helpers
// ---------------------------------------------------------------------------
const isManualInputMethod = computed(() =>
  oauthFlowRef.value?.inputMethod === 'manual'
)

const canExchangeCode = computed(() => {
  const code = oauthFlowRef.value?.authCode || ''
  return code.trim().length > 0 && !!oauthState.value.sessionId && !oauthState.value.loading
})

// ---------------------------------------------------------------------------
// Watchers
// ---------------------------------------------------------------------------
watch(
  () => props.show,
  (newVal) => {
    if (newVal && props.account) {
      initializeFromAccount(props.account)
    } else {
      resetState()
    }
  }
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
function resolveAccountCategory(): string {
  const type = props.account?.type
  if (type === 'oauth' || type === 'setup-token') return 'oauth-based'
  if (type === 'bedrock') return 'bedrock'
  if (type === 'service_account') return 'service_account'
  return 'apikey'
}

function initializeFromAccount(account: Account) {
  // Set addMethod from existing account type for the OAuth flow
  if (account.type === 'oauth' || account.type === 'setup-token') {
    addMethod.value = account.type as AddMethod
  }
  // Delegate all platform-specific initialization to the form component
  void nextTick(() => {
    platformFormRef.value?.initFromAccount?.(account)
  })
}

// ---------------------------------------------------------------------------
// Methods
// ---------------------------------------------------------------------------
const resetState = () => {
  addMethod.value = 'oauth'
  platformFormRef.value?.reset?.()
  platformFormRef.value?.resetOAuth?.()
  oauthFlowRef.value?.reset()
}

const handleClose = () => {
  emit('close')
}

async function handleGenerateUrl() {
  await platformFormRef.value?.generateOAuthUrl?.(
    props.account?.proxy_id ?? null,
    oauthFlowRef.value?.projectId
  )
}

async function handleExchangeCode() {
  if (!props.account) return
  const code = oauthFlowRef.value?.authCode?.trim()
  if (!code) return

  const result = await platformFormRef.value?.handleOAuthExchange?.(
    code,
    oauthFlowRef.value?.oauthState,
    oauthFlowRef.value?.projectId
  )
  if (result) await finalizeOAuthResult(result)
}

async function handleCookieAuth(sessionKey: string) {
  if (!props.account) return
  const result = await platformFormRef.value?.handleCookieAuth?.(sessionKey)
  if (result) await finalizeOAuthResult(result)
}

async function handleRefreshToken(rt: string) {
  if (!props.account) return
  const result = await platformFormRef.value?.handleRefreshToken?.(rt)
  if (result) await finalizeOAuthResult(result)
}

async function handleMobileRefreshToken(rt: string) {
  if (!props.account) return
  const result = await platformFormRef.value?.handleMobileRefreshToken?.(rt)
  if (result) await finalizeOAuthResult(result)
}

async function handleSessionToken(token: string) {
  if (!props.account) return
  const result = await platformFormRef.value?.handleSessionToken?.(token)
  if (result) await finalizeOAuthResult(result)
}

async function finalizeOAuthResult(
  result: CreateAccountRequest | CreateAccountRequest[]
) {
  if (!props.account) return
  const request = Array.isArray(result) ? result[0] : result
  if (!request) return

  try {
    await adminAPI.accounts.update(props.account.id, {
      type: request.type,
      credentials: request.credentials,
      extra: request.extra,
    })
    const updatedAccount = await adminAPI.accounts.clearError(props.account.id)
    appStore.showSuccess(t('admin.accounts.reAuthorizedSuccess'))
    emit('reauthorized', updatedAccount)
    handleClose()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.accounts.oauth.authFailed')))
  }
}

/** Save credentials directly (non-OAuth flow, e.g. plugin platforms) */
async function handleSaveCredentials() {
  if (!props.account || !platformFormRef.value) return
  const validation = platformFormRef.value.validate()
  if (!validation.valid) {
    appStore.showError(validation.error || t('common.error'))
    return
  }
  saving.value = true
  try {
    const editPayload = platformFormRef.value.getEditPayload?.(props.account)
    if (!editPayload) return
    if (editPayload.error) { appStore.showError(t(editPayload.error)); return }
    await adminAPI.accounts.update(props.account.id, {
      credentials: editPayload.credentials,
      extra: editPayload.extra,
    })
    const updatedAccount = await adminAPI.accounts.clearError(props.account.id)
    appStore.showSuccess(t('admin.accounts.reAuthorizedSuccess'))
    emit('reauthorized', updatedAccount)
    handleClose()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    saving.value = false
  }
}
</script>
