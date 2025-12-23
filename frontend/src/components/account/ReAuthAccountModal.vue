<template>
  <Modal
    :show="show"
    :title="t('admin.accounts.reAuthorizeAccount')"
    size="lg"
    @close="handleClose"
  >
    <div v-if="account" class="space-y-5">
      <!-- Account Info -->
      <div class="rounded-lg border border-gray-200 dark:border-dark-600 bg-gray-50 dark:bg-dark-700 p-4">
        <div class="flex items-center gap-3">
          <div :class="[
            'flex h-10 w-10 items-center justify-center rounded-lg bg-gradient-to-br',
            isOpenAI ? 'from-green-500 to-green-600' : 'from-orange-500 to-orange-600'
          ]">
            <svg class="w-5 h-5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09z" />
            </svg>
          </div>
          <div>
            <span class="block font-semibold text-gray-900 dark:text-white">{{ account.name }}</span>
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{ isOpenAI ? t('admin.accounts.openaiAccount') : t('admin.accounts.claudeCodeAccount') }}
            </span>
          </div>
        </div>
      </div>

      <!-- Add Method Selection (Claude only) -->
      <div v-if="!isOpenAI">
        <label class="input-label">{{ t('admin.accounts.oauth.authMethod') }}</label>
        <div class="flex gap-4 mt-2">
          <label class="flex cursor-pointer items-center">
            <input
              v-model="addMethod"
              type="radio"
              value="oauth"
              class="mr-2 text-primary-600 focus:ring-primary-500"
            />
            <span class="text-sm text-gray-700 dark:text-gray-300">Oauth</span>
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
      </div>

      <!-- OAuth Authorization Section -->
      <OAuthAuthorizationFlow
        ref="oauthFlowRef"
        :add-method="addMethod"
        :auth-url="currentAuthUrl"
        :session-id="currentSessionId"
        :loading="currentLoading"
        :error="currentError"
        :show-help="!isOpenAI"
        :show-proxy-warning="!isOpenAI"
        :show-cookie-option="!isOpenAI"
        :allow-multiple="false"
        :method-label="t('admin.accounts.inputMethod')"
        :platform="isOpenAI ? 'openai' : 'anthropic'"
        @generate-url="handleGenerateUrl"
        @cookie-auth="handleCookieAuth"
      />

      <div class="flex justify-between gap-3 pt-4">
        <button
          type="button"
          class="btn btn-secondary"
          @click="handleClose"
        >
          {{ t('common.cancel') }}
        </button>
        <button
          v-if="isManualInputMethod"
          type="button"
          :disabled="!canExchangeCode"
          class="btn btn-primary"
          @click="handleExchangeCode"
        >
          <svg
            v-if="currentLoading"
            class="animate-spin -ml-1 mr-2 h-4 w-4"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
          {{ currentLoading ? t('admin.accounts.oauth.verifying') : t('admin.accounts.oauth.completeAuth') }}
        </button>
      </div>
    </div>
  </Modal>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { useAccountOAuth, type AddMethod, type AuthInputMethod } from '@/composables/useAccountOAuth'
import { useOpenAIOAuth } from '@/composables/useOpenAIOAuth'
import type { Account } from '@/types'
import Modal from '@/components/common/Modal.vue'
import OAuthAuthorizationFlow from './OAuthAuthorizationFlow.vue'

// Type for exposed OAuthAuthorizationFlow component
// Note: defineExpose automatically unwraps refs, so we use the unwrapped types
interface OAuthFlowExposed {
  authCode: string
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
  reauthorized: []
}>()

const appStore = useAppStore()
const { t } = useI18n()

// OAuth composables - use both Claude and OpenAI
const claudeOAuth = useAccountOAuth()
const openaiOAuth = useOpenAIOAuth()

// Refs
const oauthFlowRef = ref<OAuthFlowExposed | null>(null)

// State
const addMethod = ref<AddMethod>('oauth')

// Computed - check if this is an OpenAI account
const isOpenAI = computed(() => props.account?.platform === 'openai')

// Computed - current OAuth state based on platform
const currentAuthUrl = computed(() => isOpenAI.value ? openaiOAuth.authUrl.value : claudeOAuth.authUrl.value)
const currentSessionId = computed(() => isOpenAI.value ? openaiOAuth.sessionId.value : claudeOAuth.sessionId.value)
const currentLoading = computed(() => isOpenAI.value ? openaiOAuth.loading.value : claudeOAuth.loading.value)
const currentError = computed(() => isOpenAI.value ? openaiOAuth.error.value : claudeOAuth.error.value)

// Computed
const isManualInputMethod = computed(() => {
  // OpenAI always uses manual input (no cookie auth option)
  return isOpenAI.value || oauthFlowRef.value?.inputMethod === 'manual'
})

const canExchangeCode = computed(() => {
  const authCode = oauthFlowRef.value?.authCode || ''
  const sessionId = isOpenAI.value ? openaiOAuth.sessionId.value : claudeOAuth.sessionId.value
  const loading = isOpenAI.value ? openaiOAuth.loading.value : claudeOAuth.loading.value
  return authCode.trim() && sessionId && !loading
})

// Watchers
watch(() => props.show, (newVal) => {
  if (newVal && props.account) {
    // Initialize addMethod based on current account type (Claude only)
    if (!isOpenAI.value && (props.account.type === 'oauth' || props.account.type === 'setup-token')) {
      addMethod.value = props.account.type as AddMethod
    }
  } else {
    resetState()
  }
})

// Methods
const resetState = () => {
  addMethod.value = 'oauth'
  claudeOAuth.resetState()
  openaiOAuth.resetState()
  oauthFlowRef.value?.reset()
}

const handleClose = () => {
  emit('close')
}

const handleGenerateUrl = async () => {
  if (!props.account) return

  if (isOpenAI.value) {
    await openaiOAuth.generateAuthUrl(props.account.proxy_id)
  } else {
    await claudeOAuth.generateAuthUrl(addMethod.value, props.account.proxy_id)
  }
}

const handleExchangeCode = async () => {
  if (!props.account) return

  const authCode = oauthFlowRef.value?.authCode || ''
  if (!authCode.trim()) return

  if (isOpenAI.value) {
    // OpenAI OAuth flow
    const sessionId = openaiOAuth.sessionId.value
    if (!sessionId) return

    const tokenInfo = await openaiOAuth.exchangeAuthCode(authCode.trim(), sessionId, props.account.proxy_id)
    if (!tokenInfo) return

    // Build credentials and extra info
    const credentials = openaiOAuth.buildCredentials(tokenInfo)
    const extra = openaiOAuth.buildExtraInfo(tokenInfo)

    try {
      // Update account with new credentials
      await adminAPI.accounts.update(props.account.id, {
        type: 'oauth', // OpenAI OAuth is always 'oauth' type
        credentials,
        extra
      })

      // Clear error status after successful re-authorization
      await adminAPI.accounts.clearError(props.account.id)

      appStore.showSuccess(t('admin.accounts.reAuthorizedSuccess'))
      emit('reauthorized')
      handleClose()
    } catch (error: any) {
      openaiOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      appStore.showError(openaiOAuth.error.value)
    }
  } else {
    // Claude OAuth flow
    const sessionId = claudeOAuth.sessionId.value
    if (!sessionId) return

    claudeOAuth.loading.value = true
    claudeOAuth.error.value = ''

    try {
      const proxyConfig = props.account.proxy_id ? { proxy_id: props.account.proxy_id } : {}
      const endpoint = addMethod.value === 'oauth'
        ? '/admin/accounts/exchange-code'
        : '/admin/accounts/exchange-setup-token-code'

      const tokenInfo = await adminAPI.accounts.exchangeCode(endpoint, {
        session_id: sessionId,
        code: authCode.trim(),
        ...proxyConfig
      })

      const extra = claudeOAuth.buildExtraInfo(tokenInfo)

      // Update account with new credentials and type
      await adminAPI.accounts.update(props.account.id, {
        type: addMethod.value, // Update type based on selected method
        credentials: tokenInfo,
        extra
      })

      // Clear error status after successful re-authorization
      await adminAPI.accounts.clearError(props.account.id)

      appStore.showSuccess(t('admin.accounts.reAuthorizedSuccess'))
      emit('reauthorized')
      handleClose()
    } catch (error: any) {
      claudeOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      appStore.showError(claudeOAuth.error.value)
    } finally {
      claudeOAuth.loading.value = false
    }
  }
}

const handleCookieAuth = async (sessionKey: string) => {
  if (!props.account || isOpenAI.value) return

  claudeOAuth.loading.value = true
  claudeOAuth.error.value = ''

  try {
    const proxyConfig = props.account.proxy_id ? { proxy_id: props.account.proxy_id } : {}
    const endpoint = addMethod.value === 'oauth'
      ? '/admin/accounts/cookie-auth'
      : '/admin/accounts/setup-token-cookie-auth'

    const tokenInfo = await adminAPI.accounts.exchangeCode(endpoint, {
      session_id: '',
      code: sessionKey.trim(),
      ...proxyConfig
    })

    const extra = claudeOAuth.buildExtraInfo(tokenInfo)

    // Update account with new credentials and type
    await adminAPI.accounts.update(props.account.id, {
      type: addMethod.value, // Update type based on selected method
      credentials: tokenInfo,
      extra
    })

    // Clear error status after successful re-authorization
    await adminAPI.accounts.clearError(props.account.id)

    appStore.showSuccess(t('admin.accounts.reAuthorizedSuccess'))
    emit('reauthorized')
    handleClose()
  } catch (error: any) {
    claudeOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.cookieAuthFailed')
  } finally {
    claudeOAuth.loading.value = false
  }
}
</script>
