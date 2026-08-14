<template>
  <div class="space-y-4">
    <Button variant="outline" size="md" block :disabled="buttonDisabled" @click="startLogin">
      <template #icon>
        <!--
          WeChat's own green, not the success token. A brand mark tinted with
          the semantic palette reads as "this provider is healthy", which is a
          statement the button is not making.
        -->
        <span
          class="inline-flex h-4 w-4 items-center justify-center bg-[#07C160] font-mono text-2xs font-semibold text-white"
          aria-hidden="true"
        >
          W
        </span>
      </template>
      {{ t('auth.oidc.signIn', { providerName }) }}
    </Button>

    <p v-if="disabledHint" data-testid="wechat-oauth-hint" class="text-xs text-warn">
      {{ disabledHint }}
    </p>

    <div v-if="showDivider" class="flex items-center gap-3">
      <span class="h-px flex-1 bg-line" aria-hidden="true"></span>
      <span class="text-2xs uppercase tracking-[0.08em] text-ink-tertiary">
        {{ t('auth.oauthOrContinue') }}
      </span>
      <span class="h-px flex-1 bg-line" aria-hidden="true"></span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Button from '@/components/common/Button.vue'
import { resolveWeChatOAuthStart, type OAuthLoginStart } from '@/api/auth'
import { useAppStore } from '@/stores'
import { resolveAffiliateReferralCode, storeOAuthAffiliateCode } from '@/utils/oauthAffiliate'

const props = withDefaults(defineProps<{
  disabled?: boolean
  affCode?: string
  showDivider?: boolean
}>(), {
  showDivider: true,
})
const emit = defineEmits<{
  start: [request: OAuthLoginStart]
}>()

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
  storeOAuthAffiliateCode(resolveAffiliateReferralCode(props.affCode, route.query.aff, route.query.aff_code))
  const mode = resolvedStart.value.mode
  emit('start', {
    provider: 'wechat',
    params: { mode, redirect: redirectTo }
  })
}
</script>
