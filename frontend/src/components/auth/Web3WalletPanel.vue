<template>
  <div class="space-y-5">
    <div
      v-if="!wallet.available.value"
      class="rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-200"
    >
      {{ t('auth.web3.walletNotFound') }}
    </div>

    <div
      v-if="wallet.connected.value"
      class="rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/60"
    >
      <p class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-dark-400">
        {{ t('auth.web3.connectedWallet') }}
      </p>
      <p class="mt-1 break-all font-mono text-sm font-semibold text-gray-900 dark:text-white">
        {{ wallet.address.value }}
      </p>
      <p class="mt-2 text-xs text-gray-500 dark:text-dark-400">
        {{ t('auth.web3.chainId', { chainId: wallet.chainID.value }) }}
      </p>
    </div>

    <template v-if="intent === 'register'">
      <div>
        <label for="web3-username" class="input-label">
          {{ t('auth.web3.usernameLabel') }}
          <span class="text-red-500" aria-hidden="true">*</span>
        </label>
        <input
          id="web3-username"
          v-model="registration.username"
          type="text"
          class="input"
          :class="{ 'border-red-500 focus:border-red-500 focus:ring-red-500': registrationUsernameTouched && registrationUsernameErrorMessage }"
          :placeholder="t('auth.web3.usernamePlaceholder')"
          :disabled="busy"
          autocomplete="nickname"
          required
          :aria-invalid="registrationUsernameTouched && Boolean(registrationUsernameErrorMessage)"
          aria-describedby="web3-username-message"
          @blur="registrationUsernameTouched = true"
        />
        <p
          id="web3-username-message"
          class="mt-1 text-xs"
          :class="registrationUsernameTouched && registrationUsernameErrorMessage ? 'text-red-600 dark:text-red-400' : 'text-gray-500 dark:text-dark-400'"
        >
          {{ registrationUsernameTouched && registrationUsernameErrorMessage ? registrationUsernameErrorMessage : t('auth.web3.usernameHint') }}
        </p>
      </div>
      <div v-if="invitationCodeEnabled">
        <label for="web3-invitation-code" class="input-label">{{ t('auth.invitationCodeLabel') }}</label>
        <input
          id="web3-invitation-code"
          v-model="registration.invitationCode"
          class="input"
          :placeholder="t('auth.invitationCodePlaceholder')"
          :disabled="busy"
        />
      </div>
      <div v-if="promoCodeEnabled">
        <label for="web3-promo-code" class="input-label">{{ t('auth.promoCodeLabel') }}</label>
        <input
          id="web3-promo-code"
          v-model="registration.promoCode"
          class="input"
          :placeholder="t('auth.promoCodePlaceholder')"
          :disabled="busy"
        />
      </div>
      <div v-if="affiliateEnabled">
        <label for="web3-aff-code" class="input-label">{{ t('auth.affiliateCodeLabel') }}</label>
        <input
          id="web3-aff-code"
          v-model="registration.affCode"
          class="input"
          :placeholder="t('auth.affiliateCodePlaceholder')"
          :disabled="busy"
        />
      </div>
    </template>

    <TurnstileWidget
      v-if="turnstileEnabled && turnstileSiteKey"
      ref="turnstileRef"
      :site-key="turnstileSiteKey"
      @verify="turnstileToken = $event"
      @expire="turnstileToken = ''"
      @error="turnstileToken = ''"
    />

    <LoginAgreementPrompt
      v-if="loginAgreementEnabled"
      :accepted="agreementAccepted"
      :documents="loginAgreementDocuments"
      :mode="loginAgreementMode"
      :updated-at="loginAgreementUpdatedAt"
      :visible="showAgreementModal"
      @accept="acceptLoginAgreement"
      @reject="rejectLoginAgreement"
      @open="showAgreementModal = true"
    />

    <button
      type="button"
      class="btn btn-primary w-full"
      :disabled="busy || !settingsLoaded || !wallet.available.value || registrationUsernameBlocked || agreementGateActive || (turnstileEnabled && !turnstileToken)"
      @click="handleWalletAction"
    >
      <span v-if="busy" class="mr-2 h-4 w-4 animate-spin rounded-full border-2 border-white/40 border-t-white"></span>
      {{ actionLabel }}
    </button>

    <div class="space-y-2 rounded-xl bg-gray-50 p-4 text-xs leading-5 text-gray-600 dark:bg-dark-800/50 dark:text-dark-300">
      <p>{{ t('auth.web3.noGas') }}</p>
      <p>{{ t('auth.web3.eoaOnly') }}</p>
      <p v-if="intent === 'register'" class="font-medium text-amber-700 dark:text-amber-300">
        {{ t('auth.web3.recoveryWarning') }}
      </p>
    </div>

    <TotpLoginModal
      v-if="show2FAModal"
      ref="totpModalRef"
      :temp-token="totpTempToken"
      :user-email-masked="totpUserLabel"
      @verify="handle2FAVerify"
      @cancel="handle2FACancel"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import LoginAgreementPrompt from '@/components/auth/LoginAgreementPrompt.vue'
import TotpLoginModal from '@/components/auth/TotpLoginModal.vue'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import { getPublicSettings, isTotp2FARequired, setRefreshToken, setTokenExpiresAt } from '@/api/auth'
import {
  createWeb3Challenge,
  verifyWeb3Login,
  verifyWeb3Registration,
  type Web3AuthIntent,
} from '@/api/web3Auth'
import { useWeb3Wallet } from '@/composables/useWeb3Wallet'
import { useAppStore, useAuthStore } from '@/stores'
import type { LoginAgreementDocument } from '@/types'
import { extractI18nErrorMessage } from '@/utils/apiError'
import {
  clearAllAffiliateReferralCodes,
  loadAffiliateReferralCode,
  resolveAffiliateReferralCode,
} from '@/utils/oauthAffiliate'
import { validateWeb3Username } from '@/utils/web3Username'

const props = defineProps<{ intent: Web3AuthIntent }>()
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()
const LOGIN_AGREEMENT_STORAGE_KEY = 'sub2api_login_agreement_consent'

const settingsLoaded = ref(false)
const registrationEnabled = ref(true)
const invitationCodeEnabled = ref(false)
const promoCodeEnabled = ref(false)
const affiliateEnabled = ref(false)
const turnstileEnabled = ref(false)
const turnstileSiteKey = ref('')
const turnstileToken = ref('')
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const loginAgreementEnabled = ref(false)
const loginAgreementMode = ref<'modal' | 'checkbox' | string>('modal')
const loginAgreementUpdatedAt = ref('')
const loginAgreementRevision = ref('')
const loginAgreementDocuments = ref<LoginAgreementDocument[]>([])
const agreementAccepted = ref(true)
const showAgreementModal = ref(false)
const challengeToken = ref('')
const show2FAModal = ref(false)
const totpTempToken = ref('')
const totpUserLabel = ref('')
const totpModalRef = ref<InstanceType<typeof TotpLoginModal> | null>(null)
const verifying = ref(false)
const registrationUsernameTouched = ref(false)

const registration = reactive({
  username: '',
  invitationCode: '',
  promoCode: '',
  affCode: '',
})

function invalidateChallenge(): void {
  challengeToken.value = ''
}

const wallet = useWeb3Wallet(invalidateChallenge)
const busy = computed(() => wallet.connecting.value || wallet.signing.value || verifying.value)
const agreementGateActive = computed(() => loginAgreementEnabled.value && !agreementAccepted.value)
const registrationUsernameValidation = computed(() => validateWeb3Username(registration.username))
const registrationUsernameErrorMessage = computed(() => {
  const error = registrationUsernameValidation.value.error
  return error ? t(`auth.web3.usernameErrors.${error}`) : ''
})
const registrationUsernameBlocked = computed(() => props.intent === 'register' && Boolean(registrationUsernameValidation.value.error))
const actionLabel = computed(() => {
  if (wallet.connecting.value) return t('auth.web3.connecting')
  if (wallet.signing.value) return t('auth.web3.waitingForSignature')
  if (verifying.value) return t('common.verifying')
  if (!wallet.connected.value) return t('auth.web3.connectWallet')
  return props.intent === 'login' ? t('auth.web3.signIn') : t('auth.web3.register')
})

onMounted(async () => {
  registration.affCode = resolveAffiliateReferralCode(route.query.aff, route.query.aff_code)
  registration.promoCode = typeof route.query.promo === 'string' ? route.query.promo : ''
  try {
    const settings = await getPublicSettings()
    registrationEnabled.value = settings.registration_enabled
    invitationCodeEnabled.value = settings.invitation_code_enabled
    promoCodeEnabled.value = settings.promo_code_enabled
    affiliateEnabled.value = settings.affiliate_enabled
    turnstileEnabled.value = settings.turnstile_enabled
    turnstileSiteKey.value = settings.turnstile_site_key || ''
    applyLoginAgreementSettings(settings)
  } catch (error) {
    appStore.showError(extractI18nErrorMessage(error, t, 'auth.errors', t('auth.web3.settingsFailed')))
  } finally {
    settingsLoaded.value = true
  }
})

async function handleWalletAction(): Promise<void> {
  if (props.intent === 'register') {
    registrationUsernameTouched.value = true
    if (registrationUsernameValidation.value.error) {
      appStore.showError(registrationUsernameErrorMessage.value)
      return
    }
  }
  if (!wallet.connected.value) {
    try {
      await wallet.connect()
    } catch (error) {
      showError(error, t('auth.web3.connectFailed'))
    }
    return
  }
  if (props.intent === 'register' && !registrationEnabled.value) {
    appStore.showError(t('auth.registrationDisabled'))
    return
  }
  if (props.intent === 'register' && invitationCodeEnabled.value && !registration.invitationCode.trim()) {
    appStore.showError(t('auth.invitationCodeRequired'))
    return
  }
  if (agreementGateActive.value) {
    if (loginAgreementMode.value !== 'checkbox') showAgreementModal.value = true
    appStore.showWarning(t('legal.loginAgreementPrompt.loginRequiredWarning'))
    return
  }
  if (turnstileEnabled.value && !turnstileToken.value) {
    appStore.showError(t('auth.completeVerification'))
    return
  }

  verifying.value = true
  try {
    const challenge = await createWeb3Challenge(props.intent, {
      address: wallet.address.value,
      chain_id: wallet.chainID.value,
      ...(turnstileToken.value ? { turnstile_token: turnstileToken.value } : {}),
    })
    challengeToken.value = challenge.challenge_token
    const signature = await wallet.signMessage(challenge.message)
    if (props.intent === 'login') {
      const response = await verifyWeb3Login({
        challenge_token: challengeToken.value,
        signature,
      })
      if (isTotp2FARequired(response)) {
        totpTempToken.value = response.temp_token || ''
        totpUserLabel.value = response.user_email_masked || ''
        show2FAModal.value = true
        return
      }
      await completeAuthentication(response.access_token, response.refresh_token, response.expires_in)
      appStore.showSuccess(t('auth.loginSuccess'))
    } else {
      const affCode = registration.affCode.trim() || loadAffiliateReferralCode()
      const response = await verifyWeb3Registration({
        challenge_token: challengeToken.value,
        signature,
        username: registrationUsernameValidation.value.username,
        ...(registration.invitationCode.trim() ? { invitation_code: registration.invitationCode.trim() } : {}),
        ...(registration.promoCode.trim() ? { promo_code: registration.promoCode.trim() } : {}),
        ...(affCode ? { aff_code: affCode } : {}),
      })
      await completeAuthentication(response.access_token, response.refresh_token, response.expires_in)
      appStore.showSuccess(t('auth.accountCreatedSuccess', { siteName: appStore.siteName || 'Sub2API' }))
    }
    clearAllAffiliateReferralCodes()
    await redirectAfterAuthentication()
  } catch (error) {
    showError(error, props.intent === 'login' ? t('auth.web3.loginFailed') : t('auth.web3.registrationFailed'))
    resetTurnstile()
  } finally {
    challengeToken.value = ''
    verifying.value = false
  }
}

async function completeAuthentication(accessToken: string, refreshToken?: string, expiresIn?: number): Promise<void> {
  if (refreshToken) setRefreshToken(refreshToken)
  if (expiresIn) setTokenExpiresAt(expiresIn)
  await authStore.setToken(accessToken)
}

async function redirectAfterAuthentication(): Promise<void> {
  const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/dashboard'
  await router.push(redirect)
}

async function handle2FAVerify(code: string): Promise<void> {
  totpModalRef.value?.setVerifying(true)
  try {
    await authStore.login2FA(totpTempToken.value, code)
    show2FAModal.value = false
    clearAllAffiliateReferralCodes()
    appStore.showSuccess(t('auth.loginSuccess'))
    await redirectAfterAuthentication()
  } catch (error) {
    const message = extractI18nErrorMessage(error, t, 'auth.errors', t('profile.totp.loginFailed'))
    totpModalRef.value?.setError(message)
    totpModalRef.value?.setVerifying(false)
  }
}

function handle2FACancel(): void {
  show2FAModal.value = false
  totpTempToken.value = ''
  totpUserLabel.value = ''
}

function resetTurnstile(): void {
  turnstileRef.value?.reset()
  turnstileToken.value = ''
}

function showError(error: unknown, fallback: string): void {
  const localCode = error instanceof Error ? error.message : ''
  const localMessages: Record<string, string> = {
    WEB3_WALLET_NOT_FOUND: t('auth.web3.walletNotFound'),
    WEB3_WALLET_ACCOUNT_MISSING: t('auth.web3.accountMissing'),
    WEB3_WALLET_NOT_CONNECTED: t('auth.web3.connectFailed'),
    WEB3_CHAIN_ID_INVALID: t('auth.web3.chainInvalid'),
  }
  appStore.showError(localMessages[localCode] || extractI18nErrorMessage(error, t, 'auth.errors', fallback))
}

function applyLoginAgreementSettings(settings: Awaited<ReturnType<typeof getPublicSettings>>): void {
  const documents = Array.isArray(settings.login_agreement_documents)
    ? settings.login_agreement_documents.filter((doc) => doc && doc.id && doc.title)
    : []
  loginAgreementDocuments.value = documents
  loginAgreementEnabled.value = Boolean(settings.login_agreement_enabled && documents.length > 0)
  loginAgreementMode.value = settings.login_agreement_mode || 'modal'
  loginAgreementUpdatedAt.value = settings.login_agreement_updated_at || ''
  loginAgreementRevision.value = settings.login_agreement_revision || `${loginAgreementUpdatedAt.value}:${documents.map((doc) => `${doc.id}:${doc.title}`).join('|')}`
  agreementAccepted.value = !loginAgreementEnabled.value || hasAcceptedLoginAgreement(loginAgreementRevision.value)
  showAgreementModal.value = loginAgreementEnabled.value && !agreementAccepted.value && loginAgreementMode.value !== 'checkbox'
}

function hasAcceptedLoginAgreement(revision: string): boolean {
  if (!revision) return false
  try {
    const parsed = JSON.parse(localStorage.getItem(LOGIN_AGREEMENT_STORAGE_KEY) || '{}') as { revision?: string }
    return parsed.revision === revision
  } catch {
    return false
  }
}

function acceptLoginAgreement(): void {
  if (loginAgreementRevision.value) {
    localStorage.setItem(LOGIN_AGREEMENT_STORAGE_KEY, JSON.stringify({
      revision: loginAgreementRevision.value,
      accepted_at: new Date().toISOString(),
    }))
  }
  agreementAccepted.value = true
  showAgreementModal.value = false
}

function rejectLoginAgreement(): void {
  localStorage.removeItem(LOGIN_AGREEMENT_STORAGE_KEY)
  agreementAccepted.value = false
  showAgreementModal.value = false
  appStore.showWarning(t('legal.loginAgreementPrompt.loginRejectedWarning'))
}
</script>
