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
import { resolveWeChatOAuthStart REDACTED from '@/api/auth'
import { useAppStore REDACTED from '@/stores'

const props = withDefaults(defineProps<{
  disabled?: boolean
  showDivider?: boolean
REDACTED>(), {
  showDivider: true,
REDACTED)

const appStore = useAppStore()
const route = useRoute()
const { locale, t REDACTED = useI18n()
const providerName = 'WeChat'

const resolvedStart = computed(() => resolveWeChatOAuthStart(appStore.cachedPublicSettings))
const buttonDisabled = computed(() => props.disabled || resolvedStart.value.mode === null)
const disabledHint = computed(() => {
  if (props.disabled) {
    return ''
  REDACTED
  switch (resolvedStart.value.unavailableReason) {
    case 'external_browser_required':
      return localizeWeChatHint(
        '当前仅配置网站微信登录，请在系统浏览器中打开此页面后再继续。',
        'This site only has WeChat website login configured. Open this page in your browser to continue.',
      )
    case 'wechat_browser_required':
      return localizeWeChatHint(
        '当前仅配置微信内登录，请在微信中打开此页面后再继续。',
        'This site only has WeChat in-app login configured. Open this page inside WeChat to continue.',
      )
    case 'not_configured':
      return localizeWeChatHint(
        '管理员尚未配置微信登录。',
        'WeChat sign-in is not configured yet.',
      )
    default:
      return ''
  REDACTED
REDACTED)

function localizeWeChatHint(zh: string, en: string): string {
  return locale.value.toLowerCase().startsWith('zh') ? zh : en
REDACTED

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
  const apiBase = (import.meta.env.VITE_API_BASE_URL as string | undefined) || '/api/v1'
  const normalized = apiBase.replace(/\/$/, '')
  const mode = resolvedStart.value.mode
  const startURL = `${normalizedREDACTED/auth/oauth/wechat/start?mode=${modeREDACTED&redirect=${encodeURIComponent(redirectTo)REDACTED`
  window.location.href = startURL
REDACTED
</script>
