<template>
  <div class="space-y-4">
    <button type="button" :disabled="buttonDisabled" class="btn btn-secondary w-full" @click="startLogin">
      <span
        class="mr-2 inline-flex h-5 w-5 items-center justify-center rounded-full bg-emerald-100 text-xs font-semibold text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300"
      >
        企
      </span>
      {{ t('auth.oidc.signIn', { providerName }) }}
    </button>

    <p v-if="disabledHint" class="text-sm text-amber-600 dark:text-amber-400">
      {{ disabledHint }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { resolveWeComOAuthStart } from '@/api/auth'
import { resolveAffiliateReferralCode, storeOAuthAffiliateCode } from '@/utils/oauthAffiliate'
import { resolveOAuthPromoCode } from '@/utils/oauthPromoCode'

const props = defineProps<{
  disabled?: boolean
  affCode?: string
}>()

const appStore = useAppStore()
const route = useRoute()
const { t } = useI18n()
const providerName = computed(() => t('auth.wecomProviderName'))
const wecomEnabled = computed(() => appStore.cachedPublicSettings?.wecom_oauth_enabled === true)
const buttonDisabled = computed(() => props.disabled || !wecomEnabled.value)
const disabledHint = computed(() => {
  if (props.disabled || wecomEnabled.value) {
    return ''
  }
  return t('auth.oauthFlow.wecomNotConfigured')
})

onMounted(() => {
  if (!appStore.cachedPublicSettings && !appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})

function startLogin(): void {
  if (buttonDisabled.value) {
    return
  }
  const redirectTo = (route.query.redirect as string) || '/dashboard'
  storeOAuthAffiliateCode(resolveAffiliateReferralCode(props.affCode, route.query.aff, route.query.aff_code))
  const promoCode = resolveOAuthPromoCode(route.query.promo, route.query.promo_code)
  const params = new URLSearchParams({ redirect: redirectTo })
  if (promoCode) {
    params.set('promo_code', promoCode)
  }
  const start = resolveWeComOAuthStart(appStore.cachedPublicSettings)
  if (start.mode === 'webview') {
    const apiBase = (import.meta.env.VITE_API_BASE_URL as string | undefined) || '/api/v1'
    const normalized = apiBase.replace(/\/$/, '')
    window.location.href = `${normalized}/auth/oauth/wecom/start?mode=webview&${params.toString()}`
    return
  }
  window.location.href = `/auth/wecom/mobile?${params.toString()}`
}
</script>
