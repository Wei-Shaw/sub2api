<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.reAuthorizeAccount')"
    :width="isKiro ? 'wide' : 'normal'"
    @close="handleClose"
  >
    <div v-if="account" class="space-y-4">
      <div class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-700">
        <div class="flex items-center gap-3">
          <div
            :class="[
              'flex h-10 w-10 items-center justify-center rounded-lg bg-gradient-to-br',
              isOpenAILike
                ? 'from-green-500 to-green-600'
                : isGemini
                  ? 'from-blue-500 to-blue-600'
                  : isAntigravity
                    ? 'from-purple-500 to-purple-600'
                    : isGrok
                      ? 'from-slate-600 to-cyan-600'
                  : isKiro
                    ? 'from-amber-500 to-amber-600'
                    : isAntigravity
                      ? 'from-purple-500 to-purple-600'
                      : 'from-orange-500 to-orange-600'
            ]"
          >
            <Icon name="sparkles" size="md" class="text-white" />
          </div>
          <div>
            <span class="block font-semibold text-gray-900 dark:text-white">{{ account.name }}</span>
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{
                isOpenAI
                  ? t('admin.accounts.openaiAccount')
                  : isGemini
                    ? t('admin.accounts.geminiAccount')
                    : isAntigravity
                      ? t('admin.accounts.antigravityAccount')
                      : isGrok
                        ? t('admin.accounts.grokAccount')
                    : isKiro
                      ? t('admin.accounts.kiroAccount')
                      : isAntigravity
                        ? t('admin.accounts.antigravityAccount')
                        : t('admin.accounts.claudeCodeAccount')
              }}
            </span>
          </div>
        </div>
      </div>

      <fieldset v-if="isAnthropic" class="border-0 p-0">
        <legend class="input-label">{{ t('admin.accounts.oauth.authMethod') }}</legend>
        <div class="mt-2 flex gap-4">
          <label class="flex cursor-pointer items-center">
            <input
              v-model="addMethod"
              type="radio"
              value="oauth"
              class="mr-2 text-primary-600 focus:ring-primary-500"
            />
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.types.oauth') }}</span>
          </label>
          <label class="flex cursor-pointer items-center">
            <input
              v-model="addMethod"
              type="radio"
              value="setup-token"
              class="mr-2 text-primary-600 focus:ring-primary-500"
            />
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.setupTokenLongLived') }}</span>
          </label>
        </div>
      </fieldset>

      <div v-if="isGemini" class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-700">
        <div class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.accounts.oauth.gemini.oauthTypeLabel') }}
        </div>
        <div class="flex items-center gap-3">
          <div
            :class="[
              'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
              geminiOAuthType === 'google_one'
                ? 'bg-purple-500 text-white'
                : geminiOAuthType === 'code_assist'
                  ? 'bg-blue-500 text-white'
                  : 'bg-amber-500 text-white'
            ]"
          >
            <Icon v-if="geminiOAuthType === 'google_one'" name="user" size="sm" />
            <Icon v-else-if="geminiOAuthType === 'code_assist'" name="cloud" size="sm" />
            <Icon v-else name="sparkles" size="sm" />
          </div>
          <div>
            <span class="block text-sm font-medium text-gray-900 dark:text-white">
              {{
                geminiOAuthType === 'google_one'
                  ? 'Google One'
                  : geminiOAuthType === 'code_assist'
                    ? t('admin.accounts.gemini.oauthType.builtInTitle')
                    : t('admin.accounts.gemini.oauthType.customTitle')
              }}
            </span>
            <span class="text-xs text-gray-500 dark:text-gray-400">
              {{
                geminiOAuthType === 'google_one'
                  ? '个人账号'
                  : geminiOAuthType === 'code_assist'
                    ? t('admin.accounts.gemini.oauthType.builtInDesc')
                    : t('admin.accounts.gemini.oauthType.customDesc')
              }}
            </span>
          </div>
        </div>
      </div>

      <div v-if="isKiro" class="rounded-lg border border-amber-200 bg-amber-50 p-4 dark:border-amber-700/40 dark:bg-amber-900/20">
        <div class="mb-3 text-sm font-medium text-amber-900 dark:text-amber-100">
          {{ t('admin.accounts.oauth.kiro.authModeTitle') }}
        </div>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-3">
          <button
            type="button"
            @click="kiroAccountType = 'oauth'"
            :class="kiroModeClass('oauth')"
          >
            <div
              :class="[
                'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                kiroAccountType === 'oauth'
                  ? 'bg-amber-500 text-white'
                  : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
              ]"
            >
              <Icon name="key" size="sm" />
            </div>
            <div class="min-w-0">
              <span class="block text-sm font-medium text-gray-900 dark:text-white">
                {{ t('admin.accounts.oauth.kiro.oauthTitle') }}
              </span>
              <span class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.oauth.kiro.oauthSubtitle') }}
              </span>
            </div>
          </button>
          <button
            type="button"
            @click="kiroAccountType = 'idc'"
            :class="kiroModeClass('idc')"
          >
            <div
              :class="[
                'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                kiroAccountType === 'idc'
                  ? 'bg-blue-500 text-white'
                  : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
              ]"
            >
              <Icon name="cloud" size="sm" />
            </div>
            <div class="min-w-0">
              <span class="block text-sm font-medium text-gray-900 dark:text-white">
                {{ t('admin.accounts.oauth.kiro.idcTitle') }}
              </span>
              <span class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.oauth.kiro.idcSubtitle') }}
              </span>
            </div>
          </button>
          <button
            type="button"
            @click="kiroAccountType = 'import'"
            :class="kiroModeClass('import')"
          >
            <div
              :class="[
                'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                kiroAccountType === 'import'
                  ? 'bg-slate-700 text-white dark:bg-slate-500'
                  : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
              ]"
            >
              <Icon name="download" size="sm" />
            </div>
            <div class="min-w-0">
              <span class="block text-sm font-medium text-gray-900 dark:text-white">
                {{ t('admin.accounts.oauth.kiro.importTitle') }}
              </span>
              <span class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.accounts.oauth.kiro.importSubtitle') }}
              </span>
            </div>
          </button>
        </div>

        <div v-if="kiroAccountType === 'oauth'" class="mt-3 space-y-3">
          <div class="text-xs text-amber-800 dark:text-amber-200">
            {{ t('admin.accounts.oauth.kiro.oauthSubtitle') }}
          </div>
          <div class="grid grid-cols-2 gap-3">
            <button
              type="button"
              @click="kiroOAuthProvider = 'google'"
              :class="kiroProviderClass('google')"
            >
              <div
                :class="[
                  'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                  kiroOAuthProvider === 'google'
                    ? 'bg-amber-500 text-white'
                    : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
                ]"
              >
                <Icon name="user" size="sm" />
              </div>
              <div class="min-w-0">
                <span class="block text-sm font-medium text-gray-900 dark:text-white">
                  {{ t('admin.accounts.oauth.kiro.googleTitle') }}
                </span>
                <span class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.accounts.oauth.kiro.googleDesc') }}
                </span>
              </div>
            </button>
            <button
              type="button"
              @click="kiroOAuthProvider = 'github'"
              :class="kiroProviderClass('github')"
            >
              <div
                :class="[
                  'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
                  kiroOAuthProvider === 'github'
                    ? 'bg-slate-700 text-white dark:bg-slate-500'
                    : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
                ]"
              >
                <Icon name="terminal" size="sm" />
              </div>
              <div class="min-w-0">
                <span class="block text-sm font-medium text-gray-900 dark:text-white">
                  {{ t('admin.accounts.oauth.kiro.githubTitle') }}
                </span>
                <span class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.accounts.oauth.kiro.githubDesc') }}
                </span>
              </div>
            </button>
          </div>
        </div>

        <div v-if="kiroAccountType === 'idc'" class="mt-3 grid gap-3 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.accounts.oauth.kiro.idcStartUrlLabel') }}</label>
            <input
              v-model="kiroIDCStartUrl"
              type="text"
              class="input"
              :placeholder="t('admin.accounts.oauth.kiro.startUrlPlaceholder')"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.accounts.oauth.kiro.regionLabel') }}</label>
            <input
              v-model="kiroIDCRegion"
              type="text"
              class="input"
              :placeholder="t('admin.accounts.oauth.kiro.regionPlaceholder')"
            />
          </div>
        </div>

        <div v-if="isKiroImportMode" class="mt-3 space-y-3">
          <div>
            <label class="input-label">{{ t('admin.accounts.oauth.kiro.tokenJsonLabel') }}</label>
            <textarea
              v-model="kiroTokenJson"
              rows="7"
              class="input font-mono text-xs"
              placeholder='{"accessToken":"...","refreshToken":"..."}'
            />
            <p class="input-hint">{{ t('admin.accounts.oauth.kiro.tokenJsonHint') }}</p>
          </div>
          <div>
            <label class="input-label">{{ t('admin.accounts.oauth.kiro.deviceRegistrationLabel') }}</label>
            <textarea
              v-model="kiroDeviceRegistrationJson"
              rows="4"
              class="input font-mono text-xs"
              placeholder='{"clientId":"...","clientSecret":"..."}'
            />
          </div>
        </div>
      </div>

      <OAuthAuthorizationFlow
        v-if="!isKiroImportMode"
        ref="oauthFlowRef"
        :add-method="addMethod"
        :auth-url="currentAuthUrl"
        :session-id="currentSessionId"
        :loading="currentLoading"
        :error="currentError"
        :show-help="isAnthropic"
        :show-proxy-warning="isAnthropic"
        :show-cookie-option="isAnthropic"
        :allow-multiple="false"
        :method-label="t('admin.accounts.inputMethod')"
        :platform="oauthPlatform"
        :show-project-id="isGemini && geminiOAuthType === 'code_assist'"
        @generate-url="handleGenerateUrl"
        @cookie-auth="handleCookieAuth"
      />
    </div>

    <template #footer>
      <div v-if="account" class="flex justify-between gap-3">
        <button type="button" class="btn btn-secondary" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button
          v-if="isKiroImportMode"
          type="button"
          :disabled="currentLoading || !kiroTokenJson.trim()"
          class="btn btn-primary"
          @click="handleKiroImport"
        >
          {{ currentLoading ? t('admin.accounts.oauth.verifying') : t('admin.accounts.oauth.kiro.importAndUpdate') }}
        </button>
        <button
          v-else-if="isManualInputMethod"
          type="button"
          :disabled="!canExchangeCode"
          class="btn btn-primary"
          @click="handleExchangeCode"
        >
          <svg
            v-if="currentLoading"
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
            />
            <path
              class="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            />
          </svg>
          {{ currentLoading ? t('admin.accounts.oauth.verifying') : t('admin.accounts.oauth.completeAuth') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import OAuthAuthorizationFlow from '@/components/account/OAuthAuthorizationFlow.vue'
import { useAntigravityOAuth } from '@/composables/useAntigravityOAuth'
import {
  type AddMethod,
  type AuthInputMethod,
  useAccountOAuth
} from '@/composables/useAccountOAuth'
import { useGeminiOAuth } from '@/composables/useGeminiOAuth'
import { useGrokOAuth } from '@/composables/useGrokOAuth'
import { useKiroOAuth } from '@/composables/useKiroOAuth'
import { useOpenAIOAuth } from '@/composables/useOpenAIOAuth'
import { useAppStore } from '@/stores/app'
import type { Account, AccountPlatform } from '@/types'

interface OAuthFlowExposed {
  authCode: string
  oauthState: string
  oauthCallbackPath: string
  oauthLoginOption: string
  projectId: string
  sessionKey: string
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

const claudeOAuth = useAccountOAuth()
const openaiOAuth = useOpenAIOAuth()
const geminiOAuth = useGeminiOAuth()
const antigravityOAuth = useAntigravityOAuth()
const grokOAuth = useGrokOAuth()
const kiroOAuth = useKiroOAuth()

const oauthFlowRef = ref<OAuthFlowExposed | null>(null)

const addMethod = ref<AddMethod>('oauth')
const geminiOAuthType = ref<'code_assist' | 'google_one' | 'ai_studio'>('code_assist')
const kiroAccountType = ref<'oauth' | 'idc' | 'import'>('oauth')
const kiroOAuthProvider = ref<'google' | 'github'>('google')
const kiroIDCStartUrl = ref('https://view.awsapps.com/start')
const kiroIDCRegion = ref('us-east-1')
const kiroTokenJson = ref('')
const kiroDeviceRegistrationJson = ref('')

const isOpenAI = computed(() => props.account?.platform === 'openai')
const isOpenAILike = computed(() => isOpenAI.value)
const isGemini = computed(() => props.account?.platform === 'gemini')
const isAnthropic = computed(() => props.account?.platform === 'anthropic')
const isAntigravity = computed(() => props.account?.platform === 'antigravity')
const isGrok = computed(() => props.account?.platform === 'grok')
const isKiro = computed(() => props.account?.platform === 'kiro')

const oauthPlatform = computed<AccountPlatform>(() => {
  if (isOpenAI.value) return 'openai'
  if (isGemini.value) return 'gemini'
  if (isGrok.value) return 'grok'
  if (isKiro.value) return 'kiro'
  if (isAntigravity.value) return 'antigravity'
  return 'anthropic'
})

const currentAuthUrl = computed(() => {
  if (isOpenAILike.value) return openaiOAuth.authUrl.value
  if (isGemini.value) return geminiOAuth.authUrl.value
  if (isKiro.value) return kiroOAuth.authUrl.value
  if (isAntigravity.value) return antigravityOAuth.authUrl.value
  if (isGrok.value) return grokOAuth.authUrl.value
  return claudeOAuth.authUrl.value
})

const currentSessionId = computed(() => {
  if (isOpenAILike.value) return openaiOAuth.sessionId.value
  if (isGemini.value) return geminiOAuth.sessionId.value
  if (isKiro.value) return kiroOAuth.sessionId.value
  if (isAntigravity.value) return antigravityOAuth.sessionId.value
  if (isGrok.value) return grokOAuth.sessionId.value
  return claudeOAuth.sessionId.value
})

const currentLoading = computed(() => {
  if (isOpenAILike.value) return openaiOAuth.loading.value
  if (isGemini.value) return geminiOAuth.loading.value
  if (isKiro.value) return kiroOAuth.loading.value
  if (isAntigravity.value) return antigravityOAuth.loading.value
  if (isGrok.value) return grokOAuth.loading.value
  return claudeOAuth.loading.value
})

const currentError = computed(() => {
  if (isOpenAILike.value) return openaiOAuth.error.value
  if (isGemini.value) return geminiOAuth.error.value
  if (isKiro.value) return kiroOAuth.error.value
  if (isAntigravity.value) return antigravityOAuth.error.value
  if (isGrok.value) return grokOAuth.error.value
  return claudeOAuth.error.value
})

const isKiroImportMode = computed(() => isKiro.value && kiroAccountType.value === 'import')

const isManualInputMethod = computed(() => {
  return isOpenAILike.value || isGemini.value || isGrok.value || isKiro.value || isAntigravity.value || oauthFlowRef.value?.inputMethod === 'manual'
})

const canExchangeCode = computed(() => {
  if (isKiroImportMode.value) {
    return false
  }
  const authCode = oauthFlowRef.value?.authCode || ''
  return !!(authCode.trim() && currentSessionId.value && !currentLoading.value)
})

watch(
  () => props.show,
  (newVal) => {
    if (!newVal || !props.account) {
      resetState()
      return
    }

    if (isAnthropic.value && (props.account.type === 'oauth' || props.account.type === 'setup-token')) {
      addMethod.value = props.account.type as AddMethod
    }

    if (isGemini.value) {
      const creds = (props.account.credentials || {}) as Record<string, unknown>
      geminiOAuthType.value =
        creds.oauth_type === 'google_one'
          ? 'google_one'
          : creds.oauth_type === 'ai_studio'
            ? 'ai_studio'
            : 'code_assist'
    }

    if (isKiro.value) {
      const creds = (props.account.credentials || {}) as Record<string, unknown>
      const authMethod = typeof creds.auth_method === 'string' ? creds.auth_method : ''
      const provider = String(creds.provider || '').toLowerCase()
      kiroIDCStartUrl.value = typeof creds.start_url === 'string' && creds.start_url ? creds.start_url : 'https://view.awsapps.com/start'
      kiroIDCRegion.value = typeof creds.region === 'string' && creds.region ? creds.region : 'us-east-1'
      kiroAccountType.value = authMethod === 'idc' ? 'idc' : 'oauth'
      kiroOAuthProvider.value = provider === 'github' ? 'github' : 'google'
    }
  }
)

const resetState = () => {
  addMethod.value = 'oauth'
  geminiOAuthType.value = 'code_assist'
  kiroAccountType.value = 'oauth'
  kiroOAuthProvider.value = 'google'
  kiroIDCStartUrl.value = 'https://view.awsapps.com/start'
  kiroIDCRegion.value = 'us-east-1'
  kiroTokenJson.value = ''
  kiroDeviceRegistrationJson.value = ''
  claudeOAuth.resetState()
  openaiOAuth.resetState()
  geminiOAuth.resetState()
  antigravityOAuth.resetState()
  grokOAuth.resetState()
  kiroOAuth.resetState()
  oauthFlowRef.value?.reset()
}

const kiroModeClass = (mode: typeof kiroAccountType.value) => [
  'flex items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
  kiroAccountType.value === mode
    ? mode === 'idc'
      ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/20'
      : mode === 'import'
        ? 'border-slate-500 bg-slate-50 dark:bg-slate-900/20'
        : 'border-amber-500 bg-amber-50 dark:bg-amber-900/20'
    : mode === 'idc'
      ? 'border-gray-200 hover:border-blue-300 dark:border-dark-600 dark:hover:border-blue-700'
      : mode === 'import'
        ? 'border-gray-200 hover:border-slate-300 dark:border-dark-600 dark:hover:border-slate-700'
        : 'border-gray-200 hover:border-amber-300 dark:border-dark-600 dark:hover:border-amber-700'
]

const kiroProviderClass = (provider: typeof kiroOAuthProvider.value) => [
  'flex items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
  kiroOAuthProvider.value === provider
    ? provider === 'github'
      ? 'border-slate-500 bg-slate-50 dark:bg-slate-900/20'
      : 'border-amber-500 bg-amber-50 dark:bg-amber-900/20'
    : provider === 'github'
      ? 'border-amber-200 hover:border-slate-300 dark:border-amber-700/40 dark:hover:border-slate-700'
      : 'border-amber-200 hover:border-amber-300 dark:border-amber-700/40 dark:hover:border-amber-600'
]

const handleClose = () => {
  emit('close')
}

const buildUpdatedCredentials = (next: Record<string, unknown>) => ({
  ...((props.account?.credentials || {}) as Record<string, unknown>),
  ...next
})

const updateAccountCredentials = async (payload: {
  type: 'oauth' | 'setup-token'
  credentials: Record<string, unknown>
  extra?: Record<string, unknown>
}) => {
  if (!props.account) return

  const updatedAccount = await adminAPI.accounts.applyOAuthCredentials(props.account.id, payload)
  appStore.showSuccess(t('admin.accounts.reAuthorizedSuccess'))
  emit('reauthorized', updatedAccount)
  handleClose()
}

const handleGenerateUrl = async () => {
  if (!props.account) return

  if (isOpenAILike.value) {
    await openaiOAuth.generateAuthUrl(props.account.proxy_id)
    return
  }

  if (isGemini.value) {
    const creds = (props.account.credentials || {}) as Record<string, unknown>
    const tierId = typeof creds.tier_id === 'string' ? creds.tier_id : undefined
    const projectId = geminiOAuthType.value === 'code_assist' ? oauthFlowRef.value?.projectId : undefined
    await geminiOAuth.generateAuthUrl(props.account.proxy_id, projectId, geminiOAuthType.value, tierId)
    return
  }

  if (isGrok.value) {
    await grokOAuth.generateAuthUrl(props.account.proxy_id)
    return
  }

  if (isKiro.value) {
    if (kiroAccountType.value === 'idc') {
      await kiroOAuth.generateIDCAuthUrl({
        proxyId: props.account.proxy_id,
        startUrl: kiroIDCStartUrl.value,
        region: kiroIDCRegion.value
      })
      return
    }
    await kiroOAuth.generateAuthUrl(
      props.account.proxy_id,
      kiroOAuthProvider.value === 'github' ? 'Github' : 'Google'
    )
    return
  }

  if (isAntigravity.value) {
    await antigravityOAuth.generateAuthUrl(props.account.proxy_id)
    return
  }

  await claudeOAuth.generateAuthUrl(addMethod.value, props.account.proxy_id)
}

const handleExchangeCode = async () => {
  if (!props.account) return

  const authCode = oauthFlowRef.value?.authCode || ''
  if (!authCode.trim()) return

  if (isOpenAILike.value) {
    const sessionId = openaiOAuth.sessionId.value
    if (!sessionId) return

    const stateToUse = (oauthFlowRef.value?.oauthState || openaiOAuth.oauthState.value || '').trim()
    if (!stateToUse) {
      openaiOAuth.error.value = t('admin.accounts.oauth.authFailed')
      appStore.showError(openaiOAuth.error.value)
      return
    }

    const tokenInfo = await openaiOAuth.exchangeAuthCode(authCode.trim(), sessionId, stateToUse, props.account.proxy_id)
    if (!tokenInfo) return

    try {
      await updateAccountCredentials({
        type: 'oauth',
        credentials: buildUpdatedCredentials(openaiOAuth.buildCredentials(tokenInfo)),
        extra: openaiOAuth.buildExtraInfo(tokenInfo) as Record<string, unknown> | undefined
      })
    } catch (error: any) {
      openaiOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      appStore.showError(openaiOAuth.error.value)
    }
    return
  }

  if (isGemini.value) {
    const sessionId = geminiOAuth.sessionId.value
    if (!sessionId) return

    const stateToUse = oauthFlowRef.value?.oauthState || geminiOAuth.state.value
    if (!stateToUse) return

    const tokenInfo = await geminiOAuth.exchangeAuthCode({
      code: authCode.trim(),
      sessionId,
      state: stateToUse,
      proxyId: props.account.proxy_id,
      oauthType: geminiOAuthType.value,
      tierId: typeof (props.account.credentials as any)?.tier_id === 'string'
        ? ((props.account.credentials as any).tier_id as string)
        : undefined
    })
    if (!tokenInfo) return

    try {
      await updateAccountCredentials({
        type: 'oauth',
        credentials: buildUpdatedCredentials(geminiOAuth.buildCredentials(tokenInfo))
      })
    } catch (error: any) {
      geminiOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      appStore.showError(geminiOAuth.error.value)
    }
    return
  }

  if (isKiro.value) {
    const sessionId = kiroOAuth.sessionId.value
    if (!sessionId) return

    const stateToUse = oauthFlowRef.value?.oauthState || kiroOAuth.state.value
    if (!stateToUse) return

    const tokenInfo = await kiroOAuth.exchangeAuthCode({
      code: authCode.trim(),
      sessionId,
      state: stateToUse,
      callbackPath: oauthFlowRef.value?.oauthCallbackPath || '',
      loginOption: oauthFlowRef.value?.oauthLoginOption || '',
      proxyId: props.account.proxy_id
    })
    if (!tokenInfo) return

    try {
      await updateAccountCredentials({
        type: 'oauth',
        credentials: buildUpdatedCredentials(kiroOAuth.buildCredentials(tokenInfo))
      })
    } catch (error: any) {
      kiroOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      appStore.showError(kiroOAuth.error.value)
    }
    return
  }

  if (isAntigravity.value) {
    const sessionId = antigravityOAuth.sessionId.value
    if (!sessionId) return

    const stateToUse = oauthFlowRef.value?.oauthState || antigravityOAuth.state.value
    if (!stateToUse) return

    const tokenInfo = await antigravityOAuth.exchangeAuthCode({
      code: authCode.trim(),
      sessionId,
      state: stateToUse,
      proxyId: props.account.proxy_id
    })
    if (!tokenInfo) return

    try {
      await updateAccountCredentials({
        type: 'oauth',
        credentials: buildUpdatedCredentials(antigravityOAuth.buildCredentials(tokenInfo))
      })
    } catch (error: any) {
      antigravityOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      appStore.showError(antigravityOAuth.error.value)
    }
    return
  }

  if (isGrok.value) {
    const sessionId = grokOAuth.sessionId.value
    if (!sessionId) return

    const stateFromInput = oauthFlowRef.value?.oauthState || ''
    const stateToUse = stateFromInput || grokOAuth.state.value
    if (!stateToUse) return

    const tokenInfo = await grokOAuth.exchangeAuthCode({
      code: authCode.trim(),
      sessionId,
      state: stateToUse,
      proxyId: props.account.proxy_id
    })
    if (!tokenInfo) return

    const credentials = grokOAuth.buildCredentials(tokenInfo)
    const extra = grokOAuth.buildExtraInfo(tokenInfo)

    try {
      const updatedAccount = await adminAPI.accounts.applyOAuthCredentials(props.account.id, {
        type: 'oauth',
        credentials,
        extra
      })

      appStore.showSuccess(t('admin.accounts.reAuthorizedSuccess'))
      emit('reauthorized', updatedAccount)
      handleClose()
    } catch (error: any) {
      grokOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      appStore.showError(grokOAuth.error.value)
    }
    return
  }

  const sessionId = claudeOAuth.sessionId.value
  if (!sessionId) return

  claudeOAuth.loading.value = true
  claudeOAuth.error.value = ''

  try {
    const proxyConfig = props.account.proxy_id ? { proxy_id: props.account.proxy_id } : {}
    const endpoint =
      addMethod.value === 'oauth'
        ? '/admin/accounts/exchange-code'
        : '/admin/accounts/exchange-setup-token-code'

    const tokenInfo = await adminAPI.accounts.exchangeCode(endpoint, {
      session_id: sessionId,
      code: authCode.trim(),
      ...proxyConfig
    })

    await updateAccountCredentials({
      type: addMethod.value,
      credentials: buildUpdatedCredentials(tokenInfo),
      extra: claudeOAuth.buildExtraInfo(tokenInfo)
    })
  } catch (error: any) {
    claudeOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
    appStore.showError(claudeOAuth.error.value)
  } finally {
    claudeOAuth.loading.value = false
  }
}

const handleKiroImport = async () => {
  if (!props.account || !isKiroImportMode.value || !kiroTokenJson.value.trim()) return

  const tokenInfo = await kiroOAuth.importToken(
    kiroTokenJson.value,
    kiroDeviceRegistrationJson.value || undefined
  )
  if (!tokenInfo) return

  try {
    await updateAccountCredentials({
      type: 'oauth',
      credentials: buildUpdatedCredentials(kiroOAuth.buildCredentials(tokenInfo))
    })
  } catch (error: any) {
    kiroOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
    appStore.showError(kiroOAuth.error.value)
  }
}

const handleCookieAuth = async (sessionKey: string) => {
  if (!props.account || isOpenAILike.value || isKiro.value) return

  claudeOAuth.loading.value = true
  claudeOAuth.error.value = ''

  try {
    const proxyConfig = props.account.proxy_id ? { proxy_id: props.account.proxy_id } : {}
    const endpoint =
      addMethod.value === 'oauth'
        ? '/admin/accounts/cookie-auth'
        : '/admin/accounts/setup-token-cookie-auth'

    const tokenInfo = await adminAPI.accounts.exchangeCode(endpoint, {
      session_id: '',
      code: sessionKey.trim(),
      ...proxyConfig
    })

    await updateAccountCredentials({
      type: addMethod.value,
      credentials: buildUpdatedCredentials(tokenInfo),
      extra: claudeOAuth.buildExtraInfo(tokenInfo)
    })
  } catch (error: any) {
    claudeOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.cookieAuthFailed')
  } finally {
    claudeOAuth.loading.value = false
  }
}
</script>
