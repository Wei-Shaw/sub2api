<template>
  <div class="space-y-4">
    <button type="button" :disabled="buttonDisabled" class="btn btn-secondary w-full" @click="startLogin">
      <span
        class="mr-2 inline-flex h-5 w-5 items-center justify-center rounded-full bg-green-100 text-xs font-semibold text-green-700 dark:bg-green-900/30 dark:text-green-300"
      >
        W
      </span>
      {{ t('auth.oidc.signIn', { providerName REDACTED) REDACTEDREDACTED
    </button>

    <p
      v-if="disabledHint"
      data-testid="wechat-oauth-hint"
      class="text-sm text-amber-600 dark:text-amber-400"
    >
      {{ disabledHint REDACTEDREDACTED
    </p>

    <div v-if="showDivider" class="flex items-center gap-3">
      <div class="h-px flex-1 bg-gray-200 dark:bg-dark-700"></div>
      <span class="text-xs text-gray-500 dark:text-dark-400">
        {{ t('auth.oauthOrContinue') REDACTEDREDACTED
      </span>
      <div class="h-px flex-1 bg-gray-200 dark:bg-dark-700"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted REDACTED from 'vue'
import { useRoute REDACTED from 'vue-router'
import { useI18n REDACTED from 'vue-i18n'
import { resolveWeChatOAuthStart, type OAuthLoginStart REDACTED from '@/api/auth'
import { useAppStore REDACTED from '@/stores'
import { resolveAffiliateReferralCode, storeOAuthAffiliateCode REDACTED from '@/utils/oauthAffiliate'

const props = withDefaults(defineProps<{
  disabled?: boolean
  affCode?: string
  showDivider?: boolean
REDACTED>(), {
  showDivider: true,
REDACTED)
const emit = defineEmits<{
  start: [request: OAuthLoginStart]
REDACTED>()

const appStore = useAppStore()
const route = useRoute()
const { t, locale REDACTED = useI18n()
const providerName = computed(() => t('auth.wechatProviderName'))

function localizeWeChatHint(zh: string, en: string): string {
  return locale.value.startsWith('zh') ? zh : en
REDACTED

const resolvedStart = computed(() => resolveWeChatOAuthStart(appStore.cachedPublicSettings))
const buttonDisabled = computed(() => props.disabled || resolvedStart.value.mode === null)
const disabledHint = computed(() => {
  if (props.disabled) {
    return ''
  REDACTED
  switch (resolvedStart.value.unavailableReason) {
    case 'external_browser_required':
      return t('auth.oauthFlow.wechatSystemBrowserOnly')
    case 'wechat_browser_required':
      return t('auth.oauthFlow.wechatBrowserOnly')
    case 'native_app_required':
      return localizeWeChatHint(
        '当前仅配置微信移动应用登录，需要在原生 App 中通过微信 SDK 发起授权。',
        'This site only has WeChat mobile app login configured. Continue from the native app through the WeChat SDK.',
      )
    case 'not_configured':
      return t('auth.oauthFlow.wechatNotConfigured')
    default:
      return ''
  REDACTED
REDACTED)

onMounted(() => {
  if (!appStore.cachedPublicSettings && !appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  REDACTED
REDACTED)

function startLogin(): void {
  if (buttonDisabled.value || !resolvedStart.value.mode) {
    return
  REDACTED
  const redirectTo = (route.query.redirect as string) || '/dashboard'
  storeOAuthAffiliateCode(resolveAffiliateReferralCode(props.affCode, route.query.aff, route.query.aff_code))
  const mode = resolvedStart.value.mode
  emit('start', {
    provider: 'wechat',
    params: { mode, redirect: redirectTo REDACTED
  REDACTED)
REDACTED
</script>
