<template>
  <!--
    This one keeps its own shell rather than moving into `AuthLayout`. Its
    widest state is the manual code/state/URL panel used to hand an authorization
    result back to an admin flow, which does not fit `AuthLayout`'s 26rem column,
    and the brand lockup would be noise on a machine-facing screen. Same ground,
    same hairline panel, no duplicated chrome.
  -->
  <div class="min-h-screen bg-canvas px-4 py-10">
    <div class="mx-auto max-w-2xl border border-line bg-surface p-6">
      <CallbackStatusPanel
        v-if="isProcessing"
        status="working"
        :status-label="t('common.processing')"
        :title="t('auth.oauth.callbackTitle')"
        :description="t('auth.oauth.callbackHint')"
      />

      <CallbackStatusPanel
        v-else-if="needsRegistrationCompletion"
        :title="t('auth.oidc.callbackTitle', { providerName })"
        :description="registrationHint"
      >
        <!--
          Every field here was a bare `<input>` under a detached `<label>` with
          no `for`/`id` pair — the label was decoration, not a label. `FormField`
          owns the pairing, `aria-describedby`, and the reserved message row.
        -->
        <div class="space-y-4 border-t border-line pt-5">
          <FormField id="oauth-complete-email" :label="t('auth.emailLabel')">
            <input
              id="oauth-complete-email"
              class="input"
              type="email"
              :value="registrationEmail"
              readonly
              disabled
            />
          </FormField>

          <FormField id="oauth-complete-password" :label="t('auth.passwordLabel')">
            <template #default="{ describedBy }">
              <input
                id="oauth-complete-password"
                v-model="password"
                type="password"
                class="input"
                :aria-describedby="describedBy"
                :placeholder="t('auth.createPasswordPlaceholder')"
                :disabled="isSubmitting"
                autocomplete="new-password"
                @keyup.enter="handleSubmitRegistration"
              />
            </template>
          </FormField>

          <FormField id="oauth-complete-confirm-password" :label="t('auth.confirmPassword')">
            <template #default="{ describedBy }">
              <input
                id="oauth-complete-confirm-password"
                v-model="confirmPassword"
                type="password"
                class="input"
                :aria-describedby="describedBy"
                :placeholder="t('auth.confirmPasswordPlaceholder')"
                :disabled="isSubmitting"
                autocomplete="new-password"
                @keyup.enter="handleSubmitRegistration"
              />
            </template>
          </FormField>

          <FormField
            v-if="invitationRequired"
            id="oauth-complete-invitation-code"
            :label="t('auth.invitationCodeLabel')"
          >
            <template #default="{ describedBy }">
              <input
                id="oauth-complete-invitation-code"
                v-model="invitationCode"
                type="text"
                class="input"
                :aria-describedby="describedBy"
                :placeholder="t('auth.invitationCodePlaceholder')"
                :disabled="isSubmitting"
                @keyup.enter="handleSubmitRegistration"
              />
            </template>
          </FormField>

          <!-- Form-level failure: the word carries it, `danger` only reinforces. -->
          <p v-if="registrationError" role="alert" class="text-sm text-danger">
            {{ registrationError }}
          </p>

          <!--
            The label used to swap to "Processing…" mid-press, changing the
            button's width under the cursor. `Button` keeps the label's box,
            overlays the spinner and sets `aria-busy`.
          -->
          <Button
            type="button"
            tone="accent"
            variant="solid"
            size="md"
            block
            :loading="isSubmitting"
            :disabled="isSubmitting || !canSubmitRegistration"
            @click="handleSubmitRegistration"
          >
            {{ t('auth.oidc.completeRegistration') }}
          </Button>
        </div>
      </CallbackStatusPanel>

      <CallbackStatusPanel
        v-else-if="invalidCallback"
        status="error"
        :status-label="t('common.error')"
        :title="t('auth.oauth.invalidCallbackTitle')"
        :description="t('auth.oauth.invalidCallbackHint')"
      >
        <div class="border-t border-line pt-5">
          <Button
            type="button"
            tone="accent"
            variant="solid"
            size="md"
            @click="router.replace('/login')"
          >
            {{ t('auth.backToLogin') }}
          </Button>
        </div>
      </CallbackStatusPanel>

      <CallbackStatusPanel
        v-else
        :title="t('auth.oauth.callbackTitle')"
        :description="t('auth.oauth.callbackHint')"
      >
        <div class="space-y-4 border-t border-line pt-5">
          <FormField id="oauth-callback-code" :label="t('auth.oauth.code')">
            <div class="flex items-start gap-2">
              <input
                id="oauth-callback-code"
                class="input min-w-0 flex-1 font-mono"
                :value="code"
                readonly
              />
              <Button
                class="shrink-0"
                type="button"
                variant="outline"
                size="md"
                :disabled="!code"
                @click="copy(code)"
              >
                {{ t('common.copy') }}
              </Button>
            </div>
          </FormField>

          <FormField id="oauth-callback-state" :label="t('auth.oauth.state')">
            <div class="flex items-start gap-2">
              <input
                id="oauth-callback-state"
                class="input min-w-0 flex-1 font-mono"
                :value="state"
                readonly
              />
              <Button
                class="shrink-0"
                type="button"
                variant="outline"
                size="md"
                :disabled="!state"
                @click="copy(state)"
              >
                {{ t('common.copy') }}
              </Button>
            </div>
          </FormField>

          <FormField id="oauth-callback-full-url" :label="t('auth.oauth.fullUrl')">
            <div class="flex items-start gap-2">
              <input
                id="oauth-callback-full-url"
                class="input min-w-0 flex-1 font-mono text-xs"
                :value="fullUrl"
                readonly
              />
              <Button
                class="shrink-0"
                type="button"
                variant="outline"
                size="md"
                :disabled="!fullUrl"
                @click="copy(fullUrl)"
              >
                {{ t('common.copy') }}
              </Button>
            </div>
          </FormField>
        </div>
      </CallbackStatusPanel>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
// By path, not through `components/common/index.ts`: the barrel re-exports
// LocaleSwitcher, which pulls `createI18n` into the graph and breaks the specs
// that mock `vue-i18n` with a partial factory.
import Button from '@/components/common/Button.vue'
import FormField from '@/components/common/FormField.vue'
import CallbackStatusPanel from '@/components/auth/CallbackStatusPanel.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore, useAuthStore } from '@/stores'
import { apiClient } from '@/api/client'
import { buildApiUrl } from '@/api/url'
import {
  exchangePendingOAuthCompletion,
  persistOAuthTokenContext,
  type OAuthTokenResponse
} from '@/api/auth'
import {
  clearAllAffiliateReferralCodes,
  loadOAuthAffiliateCode,
  oauthAffiliatePayload
} from '@/utils/oauthAffiliate'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const appStore = useAppStore()
const authStore = useAuthStore()
const isProcessing = ref(false)
const isSubmitting = ref(false)
const needsRegistrationCompletion = ref(false)
const invitationRequired = ref(false)
const registrationEmail = ref('')
const password = ref('')
const confirmPassword = ref('')
const invitationCode = ref('')
const registrationError = ref('')
const pendingProvider = ref<'github' | 'google'>('github')
const redirectTo = ref('/dashboard')
const invalidCallback = ref(false)
const EMAIL_OAUTH_PENDING_PROVIDER_KEY = 'email_oauth_pending_provider'

type EmailOAuthPendingCompletion = Partial<OAuthTokenResponse> & {
  error?: string
  provider?: string
  redirect?: string
  email?: string
  resolved_email?: string
  invitation_required?: boolean
}

const code = computed(() => (route.query.code as string) || '')
const state = computed(() => (route.query.state as string) || '')
const error = computed(
  () => (route.query.error as string) || (route.query.error_description as string) || ''
)

const fullUrl = computed(() => {
  if (typeof window === 'undefined') return ''
  return window.location.href
})
const providerName = computed(() =>
  pendingProvider.value === 'google' ? 'Google' : 'GitHub'
)
const registrationHint = computed(() =>
  invitationRequired.value
    ? t('auth.oidc.invitationRequired', { providerName: providerName.value })
    : t('auth.oidc.completeRegistration')
)
const canSubmitRegistration = computed(() => {
  if (!registrationEmail.value.trim()) return false
  if (password.value.length < 6) return false
  if (password.value !== confirmPassword.value) return false
  if (invitationRequired.value && !invitationCode.value.trim()) return false
  return true
})

function parseFragmentParams(): URLSearchParams {
  const raw = typeof window !== 'undefined' ? window.location.hash : ''
  const hash = raw.startsWith('#') ? raw.slice(1) : raw
  return new URLSearchParams(hash)
}

function readTokenResponse(params: URLSearchParams): OAuthTokenResponse | null {
  const accessToken = params.get('access_token')?.trim() || ''
  if (!accessToken) return null

  const response: OAuthTokenResponse = { access_token: accessToken }
  const refreshToken = params.get('refresh_token')?.trim() || ''
  if (refreshToken) response.refresh_token = refreshToken
  const expiresIn = Number.parseInt(params.get('expires_in')?.trim() || '', 10)
  if (Number.isFinite(expiresIn) && expiresIn > 0) response.expires_in = expiresIn
  const tokenType = params.get('token_type')?.trim() || ''
  if (tokenType) response.token_type = tokenType
  return response
}

function sanitizeRedirectPath(path: string | null | undefined): string {
  if (!path) return '/dashboard'
  if (!path.startsWith('/')) return '/dashboard'
  if (path.startsWith('//')) return '/dashboard'
  if (path.includes('://')) return '/dashboard'
  if (path.includes('\n') || path.includes('\r')) return '/dashboard'
  return path
}

function readPendingEmailOAuthProvider(): 'github' | 'google' | null {
  if (typeof window === 'undefined') return null
  const provider = window.sessionStorage.getItem(EMAIL_OAUTH_PENDING_PROVIDER_KEY)
  if (provider === 'github' || provider === 'google') return provider
  return null
}

function redirectProviderCallbackToBackend(provider: 'github' | 'google'): void {
  if (typeof window === 'undefined') return
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(route.query)) {
    if (Array.isArray(value)) {
      value.forEach((item) => {
        if (item != null) params.append(key, String(item))
      })
    } else if (value != null) {
      params.set(key, String(value))
    }
  }
  const suffix = params.toString() ? `?${params.toString()}` : ''
  window.location.href = buildApiUrl(`/auth/oauth/${provider}/callback${suffix}`)
}

async function finalizeTokenResponse(tokenResponse: OAuthTokenResponse, redirect: string) {
  persistOAuthTokenContext(tokenResponse)
  await authStore.setToken(tokenResponse.access_token)
  if (typeof window !== 'undefined') {
    window.sessionStorage.removeItem(EMAIL_OAUTH_PENDING_PROVIDER_KEY)
  }
  clearAllAffiliateReferralCodes()
  appStore.showSuccess(t('auth.loginSuccess'))
  await router.replace(sanitizeRedirectPath(redirect))
}

function hasOAuthTokenResponse(value: Partial<OAuthTokenResponse>): value is OAuthTokenResponse {
  return typeof value.access_token === 'string' && value.access_token.trim() !== ''
}

async function resumePendingEmailOAuth() {
  isProcessing.value = true
  try {
    const completion = await exchangePendingOAuthCompletion() as EmailOAuthPendingCompletion
    const completionRedirect = completion.redirect || '/dashboard'
    if (hasOAuthTokenResponse(completion)) {
      await finalizeTokenResponse(completion, completionRedirect)
      return
    }

    const provider = String(completion.provider || '').toLowerCase()
    if (provider === 'github' || provider === 'google') {
      pendingProvider.value = provider
    }
    redirectTo.value = sanitizeRedirectPath(completionRedirect)

    if (completion.error === 'invitation_required' || completion.error === 'registration_completion_required') {
      invitationRequired.value = completion.error === 'invitation_required' || completion.invitation_required === true
      registrationEmail.value = String(completion.resolved_email || completion.email || '').trim()
      needsRegistrationCompletion.value = true
      isProcessing.value = false
      return
    }

    appStore.showError(completion.error || t('auth.loginFailed'))
  } catch (e: unknown) {
    const err = e as { message?: string; response?: { data?: { message?: string } } }
    const message = err.response?.data?.message || err.message || t('auth.loginFailed')
    appStore.showError(message)
    invalidCallback.value = true
  } finally {
    if (!needsRegistrationCompletion.value) {
      isProcessing.value = false
    }
  }
}

async function handleSubmitRegistration() {
  registrationError.value = ''
  if (!registrationEmail.value.trim()) {
    registrationError.value = t('auth.emailRequired')
    return
  }
  if (password.value.length < 6) {
    registrationError.value = t('auth.passwordMinLength')
    return
  }
  if (password.value !== confirmPassword.value) {
    registrationError.value = t('auth.passwordsDoNotMatch')
    return
  }
  const code = invitationCode.value.trim()
  if (invitationRequired.value && !code) return

  isSubmitting.value = true
  try {
    const payload: { password: string; invitation_code?: string; aff_code?: string } = {
      password: password.value,
      ...oauthAffiliatePayload(loadOAuthAffiliateCode())
    }
    if (invitationRequired.value) {
      payload.invitation_code = code
    }
    const { data } = await apiClient.post<OAuthTokenResponse>(
      `/auth/oauth/${pendingProvider.value}/complete-registration`,
      payload
    )
    await finalizeTokenResponse(data, redirectTo.value)
  } catch (e: unknown) {
    const err = e as { message?: string; response?: { data?: { message?: string } } }
    registrationError.value =
      err.response?.data?.message || err.message || t('auth.oidc.completeRegistrationFailed')
  } finally {
    isSubmitting.value = false
  }
}

onMounted(async () => {
  const params = parseFragmentParams()
  const tokenResponse = readTokenResponse(params)
  const fragmentError = params.get('error') || ''
  const fragmentErrorDescription =
    params.get('error_description') || params.get('error_message') || ''

  if (fragmentError) {
    appStore.showError(fragmentErrorDescription || fragmentError)
    return
  }
  if (!tokenResponse) {
    if (route.path === '/auth/oauth/callback') {
      const pendingEmailOAuthProvider = readPendingEmailOAuthProvider()
      if (pendingEmailOAuthProvider && code.value && state.value) {
        redirectProviderCallbackToBackend(pendingEmailOAuthProvider)
        return
      }
      await resumePendingEmailOAuth()
    }
    return
  }

  isProcessing.value = true
  try {
    await finalizeTokenResponse(tokenResponse, params.get('redirect') || '/dashboard')
  } catch (error: unknown) {
    const message = (error as { message?: string })?.message || t('auth.loginFailed')
    appStore.showError(message)
    isProcessing.value = false
  }
})

watch(
  error,
  (message) => {
    if (message) {
      appStore.showError(message)
    }
  },
  { immediate: true }
)

const copy = (value: string) => {
  if (!value) return
  copyToClipboard(value)
}
</script>
