<template>
  <div class="space-y-4">
    <button type="button" :disabled="buttonDisabled" class="btn btn-secondary w-full" @click="startLogin">
      <span
        class="mr-2 inline-flex h-5 w-5 items-center justify-center rounded-full bg-green-100 text-xs font-semibold text-green-700 dark:bg-green-900/30 dark:text-green-300"
      >
        W
      </span>
      {{ t('auth.oidc.signIn', { providerName }) }}
    </button>

    <p
      v-if="disabledHint"
      data-testid="wechat-oauth-hint"
      class="text-sm text-amber-600 dark:text-amber-400"
    >
      {{ disabledHint }}
    </p>

    <div v-if="showDivider" class="flex items-center gap-3">
      <div class="h-px flex-1 bg-gray-200 dark:bg-dark-700"></div>
      <span class="text-xs text-gray-500 dark:text-dark-400">
        {{ t('auth.oauthOrContinue') }}
      </span>
      <div class="h-px flex-1 bg-gray-200 dark:bg-dark-700"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { resolveWeChatOAuthStart } from '@/api/auth'
import { useAppStore } from '@/stores'

const props = withDefaults(defineProps<{
  disabled?: boolean
  showDivider?: boolean
}>(), {
  showDivider: true,
})

const appStore = useAppStore()
const route = useRoute()
const { t, locale } = useI18n()
const providerName = computed(() => t('auth.wechatProviderName'))

function localizeWeChatHint(zh: string, en: string): string {
  return locale.value.startsWith('zh') ? zh : en
}

const resolvedStart = computed(() => resolveWeChatOAuthStart(appStore.cachedPublicSettings))
const buttonDisabled = computed(() => props.disabled || resolvedStart.value.mode === null)
const disabledHint = computed(() => {
  if (props.disabled) {
    return ''
  }
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
  }
})

onMounted(() => {
  if (!appStore.cachedPublicSettings && !appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})

function startLogin(): void {
  if (buttonDisabled.value || !resolvedStart.value.mode) {
    return
  }
  const redirectTo = (route.query.redirect as string) || '/dashboard'
  const apiBase = (import.meta.env.VITE_API_BASE_URL as string | undefined) || '/api/v1'
  const normalized = apiBase.replace(/\/$/, '')
  const mode = resolvedStart.value.mode
  const startURL = `${normalized}/auth/oauth/wechat/start?mode=${mode}&redirect=${encodeURIComponent(redirectTo)}`
  window.location.href = startURL
}
</script>
