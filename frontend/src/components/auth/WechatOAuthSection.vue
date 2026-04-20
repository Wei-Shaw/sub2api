<template>
  <div class="space-y-4">
    <button type="button" :disabled="disabled" class="btn btn-secondary w-full" @click="startLogin">
      <span
        class="mr-2 inline-flex h-5 w-5 items-center justify-center rounded-full bg-green-100 text-xs font-semibold text-green-700 dark:bg-green-900/30 dark:text-green-300"
      >
        W
      </span>
      {{ t('auth.oidc.signIn', { providerName REDACTED) REDACTEDREDACTED
    </button>

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
import { useRoute REDACTED from 'vue-router'
import { useI18n REDACTED from 'vue-i18n'

withDefaults(defineProps<{
  disabled?: boolean
  showDivider?: boolean
REDACTED>(), {
  showDivider: true,
REDACTED)

const route = useRoute()
const { t REDACTED = useI18n()

const providerName = 'WeChat'

function resolveWeChatOAuthMode(): 'open' | 'mp' {
  if (typeof navigator === 'undefined') {
    return 'open'
  REDACTED
  return /MicroMessenger/i.test(navigator.userAgent) ? 'mp' : 'open'
REDACTED

function startLogin(): void {
  const redirectTo = (route.query.redirect as string) || '/dashboard'
  const apiBase = (import.meta.env.VITE_API_BASE_URL as string | undefined) || '/api/v1'
  const normalized = apiBase.replace(/\/$/, '')
  const mode = resolveWeChatOAuthMode()
  const startURL = `${normalizedREDACTED/auth/oauth/wechat/start?mode=${modeREDACTED&redirect=${encodeURIComponent(redirectTo)REDACTED`
  window.location.href = startURL
REDACTED
</script>
