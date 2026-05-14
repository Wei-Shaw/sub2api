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
import { resolveWeComOAuthStart } from '@/api/auth'
import { useAppStore } from '@/stores'
import { resolveAffiliateReferralCode, storeOAuthAffiliateCode } from '@/utils/oauthAffiliate'

const props = defineProps<{
  disabled?: boolean
  affCode?: string
}>()

const appStore = useAppStore()
const route = useRoute()
const { t } = useI18n()
const providerName = computed(() => t('auth.wecomProviderName'))
const resolvedStart = computed(() => resolveWeComOAuthStart(appStore.cachedPublicSettings))
const buttonDisabled = computed(() => props.disabled || !resolvedStart.value.enabled)
const disabledHint = computed(() => {
  if (props.disabled || resolvedStart.value.enabled) {
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
  const apiBase = (import.meta.env.VITE_API_BASE_URL as string | undefined) || '/api/v1'
  const normalized = apiBase.replace(/\/$/, '')
  const startURL = `${normalized}/auth/oauth/wecom/start?mode=${resolvedStart.value.mode}&redirect=${encodeURIComponent(redirectTo)}`
  window.location.href = startURL
}
</script>
