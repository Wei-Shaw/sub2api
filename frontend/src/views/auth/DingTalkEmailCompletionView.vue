<template>
  <AuthLayout>
    <div class="space-y-6">
      <div class="text-center">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('auth.dingtalk.createAccountTitle') REDACTEDREDACTED
        </h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ t('auth.oauthFlow.createAccountHint') REDACTEDREDACTED
        </p>
      </div>

      <PendingOAuthCreateAccountForm
        test-id-prefix="dingtalk"
        :initial-email="initialEmail"
        :is-submitting="isSubmitting"
        :error-message="accountActionError"
        @submit="handleCreateAccount"
        @switch-to-bind="handleSwitchToBind"
      />
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { ref REDACTED from 'vue'
import { useRoute, useRouter REDACTED from 'vue-router'
import { useI18n REDACTED from 'vue-i18n'
import { AuthLayout REDACTED from '@/components/layout'
import PendingOAuthCreateAccountForm, {
  type PendingOAuthCreateAccountPayload
REDACTED from '@/components/auth/PendingOAuthCreateAccountForm.vue'
import { apiClient REDACTED from '@/api/client'
import { useAuthStore, useAppStore REDACTED from '@/stores'
import {
  persistOAuthTokenContext,
  type PendingOAuthExchangeResponse
REDACTED from '@/api/auth'
import { clearAllAffiliateReferralCodes REDACTED from '@/utils/oauthAffiliate'

const route = useRoute()
const router = useRouter()
const { t REDACTED = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

const isSubmitting = ref(false)
const accountActionError = ref('')

const initialEmail = (route.query.email as string | undefined) || ''

function sanitizeRedirectPath(path: string | null | undefined): string {
  if (!path) return '/dashboard'
  if (!path.startsWith('/')) return '/dashboard'
  if (path.startsWith('//')) return '/dashboard'
  if (path.includes('://')) return '/dashboard'
  if (path.includes('\n') || path.includes('\r')) return '/dashboard'
  return path
REDACTED

function getRequestErrorMessage(error: unknown, fallback: string): string {
  const err = error as { message?: string; response?: { data?: { detail?: string; message?: string REDACTED REDACTED REDACTED
  return err.response?.data?.detail || err.response?.data?.message || err.message || fallback
REDACTED

async function handleCreateAccount(payload: PendingOAuthCreateAccountPayload) {
  accountActionError.value = ''
  if (!payload.email || !payload.password) return

  isSubmitting.value = true
  try {
    const { data REDACTED = await apiClient.post<
      PendingOAuthExchangeResponse & {
        step?: string
        redirect?: string
        existing_account_bindable?: boolean
      REDACTED
    >(
      '/auth/oauth/pending/create-account',
      {
        email: payload.email,
        password: payload.password,
        verify_code: payload.verifyCode || undefined,
        ...(payload.turnstileToken ? { turnstile_token: payload.turnstileToken REDACTED : {REDACTED),
        ...(payload.tencentCaptchaTicket
          ? {
              tencent_captcha_ticket: payload.tencentCaptchaTicket,
              tencent_captcha_randstr: payload.tencentCaptchaRandstr
          REDACTED
          : {REDACTED),
        invitation_code: payload.invitationCode || undefined
      REDACTED
    )

    const redirect = sanitizeRedirectPath(data.redirect || (route.query.redirect as string | undefined))

    if (data.access_token) {
      persistOAuthTokenContext(data)
      await authStore.setToken(data.access_token)
      clearAllAffiliateReferralCodes()
      appStore.showSuccess(t('auth.loginSuccess'))
      await router.replace(redirect)
      return
    REDACTED

    // 后端把 pending session 转到 choice 状态（用户填的 email 已在系统内）→ 跳回 callback view 走绑定流程
    if (data.step === 'choose_account_action_required' || data.existing_account_bindable === true) {
      navigateToBindLogin(payload.email)
      return
    REDACTED

    accountActionError.value = t('auth.loginFailed')
  REDACTED catch (e: unknown) {
    // 全局"开放注册"关闭且未开启钉钉企业模式豁免时，引导用户去绑定已有账户而非死路
    const err = e as { response?: { data?: { reason?: string REDACTED REDACTED REDACTED
    if (err.response?.data?.reason === 'REGISTRATION_DISABLED') {
      appStore.showInfo(t('auth.dingtalk.registrationDisabledRedirectToBind'))
      navigateToBindLogin(payload.email)
      return
    REDACTED
    accountActionError.value = getRequestErrorMessage(e, t('auth.loginFailed'))
  REDACTED finally {
    isSubmitting.value = false
  REDACTED
REDACTED

function navigateToBindLogin(email: string) {
  const query: Record<string, string> = { bind: '1' REDACTED
  if (email) query.email = email
  const redirect = route.query.redirect as string | undefined
  if (redirect) query.redirect = redirect
  router.replace({ path: '/auth/dingtalk/callback', query REDACTED)
REDACTED

function handleSwitchToBind(email: string) {
  navigateToBindLogin(email)
REDACTED
</script>
